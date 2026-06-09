package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/repository"
	"github.com/expary/GOV2/internal/security"
	"github.com/jackc/pgx/v5/pgconn"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) CheckHealth(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return mapWriteError(err)
	}
	defer rollback(tx)

	if err := fn(tx); err != nil {
		return mapWriteError(err)
	}
	return mapWriteError(tx.Commit())
}

func (s *Store) FindUserByUsername(username string) (domain.User, error) {
	return s.scanUser(context.Background(), `
SELECT id, username, nickname, COALESCE(email, ''), COALESCE(phone, ''), avatar, password_hash, status, created_at, updated_at, last_login_at
FROM gov2_users
WHERE lower(username) = lower($1) AND deleted_at IS NULL`, username)
}

func (s *Store) GetUser(id uint64) (domain.User, error) {
	return s.scanUser(context.Background(), `
SELECT id, username, nickname, COALESCE(email, ''), COALESCE(phone, ''), avatar, password_hash, status, created_at, updated_at, last_login_at
FROM gov2_users
WHERE id = $1 AND deleted_at IS NULL`, id)
}

func (s *Store) ListUsers(query repository.UserQuery) ([]domain.User, int, error) {
	ctx := context.Background()
	query.Page, query.PageSize = normalizePage(query.Page, query.PageSize)

	where := []string{"deleted_at IS NULL"}
	args := []any{}
	if query.Status != "" {
		args = append(args, query.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if strings.TrimSpace(query.Keyword) != "" {
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(query.Keyword))+"%")
		where = append(where, fmt.Sprintf("lower(username || ' ' || nickname || ' ' || COALESCE(email, '')) LIKE $%d", len(args)))
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	countSQL := "SELECT count(*) FROM gov2_users WHERE " + whereSQL
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	args = append(args, query.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT id, username, nickname, COALESCE(email, ''), COALESCE(phone, ''), avatar, password_hash, status, created_at, updated_at, last_login_at
FROM gov2_users
WHERE `+whereSQL+`
ORDER BY id ASC
LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := []domain.User{}
	for rows.Next() {
		user, err := scanUserRow(rows)
		if err != nil {
			return nil, 0, err
		}
		roleIDs, err := s.roleIDsForUser(ctx, user.ID)
		if err != nil {
			return nil, 0, err
		}
		user.RoleIDs = roleIDs
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func (s *Store) CreateUser(user domain.User) (domain.User, error) {
	ctx := context.Background()
	if _, err := s.FindUserByUsername(user.Username); err == nil {
		return domain.User{}, repository.ErrConflict
	} else if !errors.Is(err, repository.ErrNotFound) {
		return domain.User{}, err
	}

	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC()
		if user.Status == "" {
			user.Status = domain.UserStatusActive
		}
		err := tx.QueryRowContext(ctx, `
INSERT INTO gov2_users (username, nickname, email, phone, avatar, password_hash, status, created_at, updated_at)
VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7, $8, $8)
RETURNING id, created_at, updated_at`,
			user.Username,
			user.Nickname,
			user.Email,
			user.Phone,
			user.Avatar,
			user.PasswordHash,
			user.Status,
			now,
		).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return err
		}
		return replaceUserRoles(ctx, tx, user.ID, user.RoleIDs)
	}); err != nil {
		return domain.User{}, err
	}
	return s.GetUser(user.ID)
}

func (s *Store) UpdateUser(id uint64, update domain.User) (domain.User, error) {
	ctx := context.Background()
	if _, err := s.GetUser(id); err != nil {
		return domain.User{}, err
	}
	if existingID, found, err := s.userIDByUsername(ctx, update.Username); err != nil {
		return domain.User{}, err
	} else if found && existingID != id {
		return domain.User{}, repository.ErrConflict
	}

	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		if update.Status == "" {
			update.Status = domain.UserStatusActive
		}
		if update.PasswordHash != "" {
			if err := execRequireRowsAffected(ctx, tx, `
UPDATE gov2_users
SET username = $1, nickname = $2, email = NULLIF($3, ''), phone = NULLIF($4, ''), avatar = $5, password_hash = $6, status = $7, updated_at = $8, version = version + 1
WHERE id = $9 AND deleted_at IS NULL`,
				update.Username, update.Nickname, update.Email, update.Phone, update.Avatar, update.PasswordHash, update.Status, time.Now().UTC(), id); err != nil {
				return err
			}
		} else if err := execRequireRowsAffected(ctx, tx, `
UPDATE gov2_users
SET username = $1, nickname = $2, email = NULLIF($3, ''), phone = NULLIF($4, ''), avatar = $5, status = $6, updated_at = $7, version = version + 1
WHERE id = $8 AND deleted_at IS NULL`,
			update.Username, update.Nickname, update.Email, update.Phone, update.Avatar, update.Status, time.Now().UTC(), id); err != nil {
			return err
		}
		return replaceUserRoles(ctx, tx, id, update.RoleIDs)
	}); err != nil {
		return domain.User{}, err
	}
	return s.GetUser(id)
}

func (s *Store) UpdateUserStatus(id uint64, status string) (domain.User, error) {
	result, err := s.db.ExecContext(context.Background(), `
UPDATE gov2_users
SET status = $1, updated_at = $2, version = version + 1
WHERE id = $3 AND deleted_at IS NULL`, status, time.Now().UTC(), id)
	if err != nil {
		return domain.User{}, err
	}
	if err := requireRowsAffected(result); err != nil {
		return domain.User{}, err
	}
	return s.GetUser(id)
}

func (s *Store) DeleteUser(id uint64) error {
	result, err := s.db.ExecContext(context.Background(), `
UPDATE gov2_users
SET deleted_at = $1, updated_at = $1, version = version + 1
WHERE id = $2 AND deleted_at IS NULL`, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if err := requireRowsAffected(result); err != nil {
		return err
	}
	return nil
}

func (s *Store) TouchLastLogin(id uint64) error {
	result, err := s.db.ExecContext(context.Background(), `
UPDATE gov2_users
SET last_login_at = $1, updated_at = $1
WHERE id = $2 AND deleted_at IS NULL`, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if err := requireRowsAffected(result); err != nil {
		return err
	}
	return nil
}

func (s *Store) ListRoles() ([]domain.Role, error) {
	rows, err := s.db.QueryContext(context.Background(), `
SELECT id, name, code, description, created_at, updated_at
FROM gov2_roles
WHERE deleted_at IS NULL
ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := []domain.Role{}
	for rows.Next() {
		role, err := scanRoleRow(rows)
		if err != nil {
			return nil, err
		}
		permissions, err := s.permissionsForRole(context.Background(), role.ID)
		if err != nil {
			return nil, err
		}
		role.Permissions = permissions
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return roles, nil
}

func (s *Store) GetRole(id uint64) (domain.Role, error) {
	role, err := s.scanRole(context.Background(), `
SELECT id, name, code, description, created_at, updated_at
FROM gov2_roles
WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return domain.Role{}, mapWriteError(err)
	}
	permissions, err := s.permissionsForRole(context.Background(), role.ID)
	if err != nil {
		return domain.Role{}, err
	}
	role.Permissions = permissions
	return role, nil
}

func (s *Store) CreateRole(role domain.Role) (domain.Role, error) {
	ctx := context.Background()
	if _, found, err := s.roleIDByCode(ctx, role.Code); err != nil {
		return domain.Role{}, err
	} else if found {
		return domain.Role{}, repository.ErrConflict
	}

	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC()
		err := tx.QueryRowContext(ctx, `
INSERT INTO gov2_roles (name, code, description, created_at, updated_at)
VALUES ($1, $2, $3, $4, $4)
RETURNING id, created_at, updated_at`, role.Name, role.Code, role.Description, now).Scan(&role.ID, &role.CreatedAt, &role.UpdatedAt)
		if err != nil {
			return err
		}
		return replaceRolePermissions(ctx, tx, role.ID, role.Permissions)
	}); err != nil {
		return domain.Role{}, err
	}
	return s.GetRole(role.ID)
}

func (s *Store) UpdateRole(id uint64, update domain.Role) (domain.Role, error) {
	ctx := context.Background()
	if _, err := s.GetRole(id); err != nil {
		return domain.Role{}, err
	}
	if existingID, found, err := s.roleIDByCode(ctx, update.Code); err != nil {
		return domain.Role{}, err
	} else if found && existingID != id {
		return domain.Role{}, repository.ErrConflict
	}

	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
UPDATE gov2_roles
SET name = $1, code = $2, description = $3, updated_at = $4, version = version + 1
WHERE id = $5 AND deleted_at IS NULL`, update.Name, update.Code, update.Description, time.Now().UTC(), id)
		if err != nil {
			return err
		}
		if err := requireRowsAffected(result); err != nil {
			return err
		}
		return replaceRolePermissions(ctx, tx, id, update.Permissions)
	}); err != nil {
		return domain.Role{}, err
	}
	return s.GetRole(id)
}

func (s *Store) DeleteRole(id uint64) error {
	var assigned int
	if err := s.db.QueryRowContext(context.Background(), `
SELECT count(*)
FROM gov2_user_roles ur
JOIN gov2_users u ON u.id = ur.user_id
WHERE ur.role_id = $1 AND u.deleted_at IS NULL`, id).Scan(&assigned); err != nil {
		return err
	}
	if assigned > 0 {
		return repository.ErrConflict
	}
	result, err := s.db.ExecContext(context.Background(), `
UPDATE gov2_roles
SET deleted_at = $1, updated_at = $1, version = version + 1
WHERE id = $2 AND deleted_at IS NULL`, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if err := requireRowsAffected(result); err != nil {
		return err
	}
	return nil
}

func (s *Store) ListMenus() ([]domain.Menu, error) {
	rows, err := s.db.QueryContext(context.Background(), `
SELECT id, COALESCE(parent_id, 0), title, name, path, icon, component, COALESCE(permission_code, ''), sort, hidden
FROM gov2_menus
WHERE deleted_at IS NULL
ORDER BY sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	menus := []domain.Menu{}
	for rows.Next() {
		menu, err := scanMenuRow(rows)
		if err != nil {
			return nil, err
		}
		menus = append(menus, menu)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return buildMenuTree(menus), nil
}

func (s *Store) GetMenu(id uint64) (domain.Menu, error) {
	return s.scanMenu(context.Background(), `
SELECT id, COALESCE(parent_id, 0), title, name, path, icon, component, COALESCE(permission_code, ''), sort, hidden
FROM gov2_menus
WHERE id = $1 AND deleted_at IS NULL`, id)
}

func (s *Store) CreateMenu(menu domain.Menu) (domain.Menu, error) {
	ctx := context.Background()
	if _, found, err := s.menuIDByName(ctx, menu.Name); err != nil {
		return domain.Menu{}, err
	} else if found {
		return domain.Menu{}, repository.ErrConflict
	}
	if err := s.ensureMenuParent(ctx, menu.ParentID); err != nil {
		return domain.Menu{}, err
	}

	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		if err := requirePermission(ctx, tx, menu.Permission); err != nil {
			return err
		}
		err := tx.QueryRowContext(ctx, `
INSERT INTO gov2_menus (parent_id, title, name, path, icon, component, permission_code, sort, hidden, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
RETURNING id`,
			nullableUint64(menu.ParentID),
			menu.Title,
			menu.Name,
			menu.Path,
			menu.Icon,
			menu.Component,
			nullableString(menu.Permission),
			menu.Sort,
			menu.Hidden,
			time.Now().UTC(),
		).Scan(&menu.ID)
		return err
	}); err != nil {
		return domain.Menu{}, err
	}
	return s.GetMenu(menu.ID)
}

func (s *Store) UpdateMenu(id uint64, update domain.Menu) (domain.Menu, error) {
	ctx := context.Background()
	if _, err := s.GetMenu(id); err != nil {
		return domain.Menu{}, err
	}
	if existingID, found, err := s.menuIDByName(ctx, update.Name); err != nil {
		return domain.Menu{}, err
	} else if found && existingID != id {
		return domain.Menu{}, repository.ErrConflict
	}
	if update.ParentID == id {
		return domain.Menu{}, repository.ErrConflict
	}
	if err := s.ensureMenuParent(ctx, update.ParentID); err != nil {
		return domain.Menu{}, err
	}
	if update.ParentID != 0 {
		hasAncestor, err := s.menuHasAncestor(ctx, update.ParentID, id)
		if err != nil {
			return domain.Menu{}, err
		}
		if hasAncestor {
			return domain.Menu{}, repository.ErrConflict
		}
	}

	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		if err := requirePermission(ctx, tx, update.Permission); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `
UPDATE gov2_menus
SET parent_id = $1, title = $2, name = $3, path = $4, icon = $5, component = $6, permission_code = $7, sort = $8, hidden = $9, updated_at = $10
WHERE id = $11 AND deleted_at IS NULL`,
			nullableUint64(update.ParentID),
			update.Title,
			update.Name,
			update.Path,
			update.Icon,
			update.Component,
			nullableString(update.Permission),
			update.Sort,
			update.Hidden,
			time.Now().UTC(),
			id,
		)
		if err != nil {
			return err
		}
		if err := requireRowsAffected(result); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return domain.Menu{}, err
	}
	return s.GetMenu(id)
}

func (s *Store) DeleteMenu(id uint64) error {
	ctx := context.Background()
	var children int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM gov2_menus WHERE parent_id = $1 AND deleted_at IS NULL", id).Scan(&children); err != nil {
		return err
	}
	if children > 0 {
		return repository.ErrConflict
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE gov2_menus
SET deleted_at = $1, updated_at = $1
WHERE id = $2 AND deleted_at IS NULL`, time.Now().UTC(), id)
	if err != nil {
		return mapWriteError(err)
	}
	if err := requireRowsAffected(result); err != nil {
		return err
	}
	return nil
}

func (s *Store) ListDictionaries() ([]domain.Dictionary, error) {
	rows, err := s.db.QueryContext(context.Background(), `
SELECT id, code, name, created_at, updated_at
FROM gov2_dictionaries
WHERE deleted_at IS NULL
ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dictionaries := []domain.Dictionary{}
	for rows.Next() {
		var dictionary domain.Dictionary
		if err := rows.Scan(&dictionary.ID, &dictionary.Code, &dictionary.Name, &dictionary.CreatedAt, &dictionary.UpdatedAt); err != nil {
			return nil, err
		}
		items, err := s.dictionaryItems(context.Background(), dictionary.ID)
		if err != nil {
			return nil, err
		}
		dictionary.Items = items
		dictionaries = append(dictionaries, dictionary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return dictionaries, nil
}

func (s *Store) GetDictionaryByCode(code string) (domain.Dictionary, error) {
	id, found, err := s.dictionaryIDByCode(context.Background(), strings.TrimSpace(code))
	if err != nil {
		return domain.Dictionary{}, err
	}
	if !found {
		return domain.Dictionary{}, repository.ErrNotFound
	}
	return s.dictionaryByID(id)
}

func (s *Store) CreateDictionary(dictionary domain.Dictionary) (domain.Dictionary, error) {
	ctx := context.Background()
	if _, found, err := s.dictionaryIDByCode(ctx, dictionary.Code); err != nil {
		return domain.Dictionary{}, err
	} else if found {
		return domain.Dictionary{}, repository.ErrConflict
	}

	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC()
		err := tx.QueryRowContext(ctx, `
INSERT INTO gov2_dictionaries (code, name, created_at, updated_at)
VALUES ($1, $2, $3, $3)
RETURNING id, created_at, updated_at`, dictionary.Code, dictionary.Name, now).Scan(&dictionary.ID, &dictionary.CreatedAt, &dictionary.UpdatedAt)
		if err != nil {
			return err
		}
		return replaceDictionaryItems(ctx, tx, dictionary.ID, dictionary.Items)
	}); err != nil {
		return domain.Dictionary{}, err
	}
	return s.dictionaryByID(dictionary.ID)
}

func (s *Store) UpdateDictionary(id uint64, update domain.Dictionary) (domain.Dictionary, error) {
	ctx := context.Background()
	if _, err := s.dictionaryByID(id); err != nil {
		return domain.Dictionary{}, err
	}
	if existingID, found, err := s.dictionaryIDByCode(ctx, update.Code); err != nil {
		return domain.Dictionary{}, err
	} else if found && existingID != id {
		return domain.Dictionary{}, repository.ErrConflict
	}

	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
UPDATE gov2_dictionaries
SET code = $1, name = $2, updated_at = $3
WHERE id = $4 AND deleted_at IS NULL`, update.Code, update.Name, time.Now().UTC(), id)
		if err != nil {
			return err
		}
		if err := requireRowsAffected(result); err != nil {
			return err
		}
		return replaceDictionaryItems(ctx, tx, id, update.Items)
	}); err != nil {
		return domain.Dictionary{}, err
	}
	return s.dictionaryByID(id)
}

func (s *Store) DeleteDictionary(id uint64) error {
	result, err := s.db.ExecContext(context.Background(), `
UPDATE gov2_dictionaries
SET deleted_at = $1, updated_at = $1
WHERE id = $2 AND deleted_at IS NULL`, time.Now().UTC(), id)
	if err != nil {
		return mapWriteError(err)
	}
	if err := requireRowsAffected(result); err != nil {
		return err
	}
	return nil
}

func (s *Store) ListSettings() ([]domain.Setting, error) {
	rows, err := s.db.QueryContext(context.Background(), `
SELECT id, "key", value_json, description, created_at, updated_at
FROM gov2_settings
ORDER BY "key" ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := []domain.Setting{}
	for rows.Next() {
		setting, err := scanSettingRow(rows)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return settings, nil
}

func (s *Store) CreateSetting(setting domain.Setting) (domain.Setting, error) {
	ctx := context.Background()
	if _, found, err := s.settingIDByKey(ctx, setting.Key); err != nil {
		return domain.Setting{}, err
	} else if found {
		return domain.Setting{}, repository.ErrConflict
	}

	now := time.Now().UTC()
	err := s.db.QueryRowContext(ctx, `
INSERT INTO gov2_settings ("key", value_json, description, created_at, updated_at)
VALUES ($1, $2::jsonb, $3, $4, $4)
RETURNING id, created_at, updated_at`, setting.Key, string(setting.Value), setting.Description, now).Scan(&setting.ID, &setting.CreatedAt, &setting.UpdatedAt)
	if err != nil {
		return domain.Setting{}, mapWriteError(err)
	}
	return s.settingByID(setting.ID)
}

func (s *Store) UpdateSetting(id uint64, update domain.Setting) (domain.Setting, error) {
	ctx := context.Background()
	if _, err := s.settingByID(id); err != nil {
		return domain.Setting{}, err
	}
	if existingID, found, err := s.settingIDByKey(ctx, update.Key); err != nil {
		return domain.Setting{}, err
	} else if found && existingID != id {
		return domain.Setting{}, repository.ErrConflict
	}

	result, err := s.db.ExecContext(ctx, `
UPDATE gov2_settings
SET "key" = $1, value_json = $2::jsonb, description = $3, updated_at = $4
WHERE id = $5`, update.Key, string(update.Value), update.Description, time.Now().UTC(), id)
	if err != nil {
		return domain.Setting{}, mapWriteError(err)
	}
	if err := requireRowsAffected(result); err != nil {
		return domain.Setting{}, err
	}
	return s.settingByID(id)
}

func (s *Store) DeleteSetting(id uint64) error {
	result, err := s.db.ExecContext(context.Background(), "DELETE FROM gov2_settings WHERE id = $1", id)
	if err != nil {
		return mapWriteError(err)
	}
	if err := requireRowsAffected(result); err != nil {
		return err
	}
	return nil
}

func (s *Store) AddAuditLog(log domain.AuditLog) (domain.AuditLog, error) {
	detail, _ := json.Marshal(map[string]string{"detail": log.Detail})
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	var actorID any
	if log.ActorID != 0 {
		actorID = log.ActorID
	}
	var id uint64
	if err := s.db.QueryRowContext(context.Background(), `
INSERT INTO gov2_audit_logs (actor_id, actor, action, resource, resource_id, ip, user_agent, detail_json, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)
RETURNING id`, actorID, log.Actor, log.Action, log.Resource, log.ResourceID, log.IP, log.UserAgent, string(detail), log.CreatedAt).Scan(&id); err != nil {
		return domain.AuditLog{}, mapWriteError(err)
	}
	log.ID = id
	return log, nil
}

func (s *Store) ListAuditLogs(query repository.AuditLogQuery) ([]domain.AuditLog, int, error) {
	ctx := context.Background()
	query.Page, query.PageSize = normalizePage(query.Page, query.PageSize)

	where := []string{"1 = 1"}
	args := []any{}
	if strings.TrimSpace(query.Actor) != "" {
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(query.Actor))+"%")
		where = append(where, fmt.Sprintf("lower(actor) LIKE $%d", len(args)))
	}
	if strings.TrimSpace(query.Action) != "" {
		args = append(args, strings.ToLower(strings.TrimSpace(query.Action)))
		where = append(where, fmt.Sprintf("lower(action) = $%d", len(args)))
	}
	if strings.TrimSpace(query.Resource) != "" {
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(query.Resource))+"%")
		where = append(where, fmt.Sprintf("lower(resource) LIKE $%d", len(args)))
	}
	if strings.TrimSpace(query.ResourceID) != "" {
		args = append(args, strings.TrimSpace(query.ResourceID))
		where = append(where, fmt.Sprintf("resource_id = $%d", len(args)))
	}
	if strings.TrimSpace(query.Keyword) != "" {
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(query.Keyword))+"%")
		where = append(where, fmt.Sprintf("lower(actor || ' ' || action || ' ' || resource || ' ' || resource_id || ' ' || ip || ' ' || user_agent || ' ' || COALESCE(detail_json->>'detail', '')) LIKE $%d", len(args)))
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM gov2_audit_logs WHERE "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	args = append(args, query.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT id, COALESCE(actor_id, 0), actor, action, resource, resource_id, ip, user_agent, COALESCE(detail_json->>'detail', ''), created_at
FROM gov2_audit_logs
WHERE `+whereSQL+`
ORDER BY id DESC
LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	logs := []domain.AuditLog{}
	for rows.Next() {
		var log domain.AuditLog
		if err := rows.Scan(&log.ID, &log.ActorID, &log.Actor, &log.Action, &log.Resource, &log.ResourceID, &log.IP, &log.UserAgent, &log.Detail, &log.CreatedAt); err != nil {
			return nil, 0, err
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func (s *Store) Summary() (domain.DashboardSummary, error) {
	var summary domain.DashboardSummary
	ctx := context.Background()
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM gov2_users WHERE deleted_at IS NULL").Scan(&summary.UserCount); err != nil {
		return domain.DashboardSummary{}, err
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM gov2_users WHERE deleted_at IS NULL AND status = $1", domain.UserStatusActive).Scan(&summary.ActiveUserCount); err != nil {
		return domain.DashboardSummary{}, err
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM gov2_roles WHERE deleted_at IS NULL").Scan(&summary.RoleCount); err != nil {
		return domain.DashboardSummary{}, err
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM gov2_menus WHERE deleted_at IS NULL").Scan(&summary.MenuCount); err != nil {
		return domain.DashboardSummary{}, err
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM gov2_audit_logs").Scan(&summary.AuditLogCount); err != nil {
		return domain.DashboardSummary{}, err
	}
	return summary, nil
}

func (s *Store) BootstrapDevelopmentData() error {
	ctx := context.Background()
	var userCount int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM gov2_users WHERE deleted_at IS NULL").Scan(&userCount); err != nil {
		return err
	}

	adminRoleID, err := s.ensureRole(ctx, "Administrator", "admin", "Full system access", []string{domain.PermissionAll})
	if err != nil {
		return err
	}
	operatorRoleID, err := s.ensureRole(ctx, "Operator", "operator", "Daily system operations", []string{
		domain.PermissionDashboardView,
		domain.PermissionSystemUserList,
		domain.PermissionSystemRoleList,
		domain.PermissionSystemMenuList,
		domain.PermissionSystemModuleList,
		domain.PermissionSystemDictionaryList,
		domain.PermissionSystemSettingList,
		domain.PermissionSystemAuditList,
	})
	if err != nil {
		return err
	}

	if userCount > 0 {
		return nil
	}

	hash, err := security.HashPassword("admin123")
	if err != nil {
		return err
	}
	if _, err := s.CreateUser(domain.User{
		Username:     "admin",
		Nickname:     "GOV2 Admin",
		Email:        "admin@gov2.local",
		PasswordHash: hash,
		RoleIDs:      []uint64{adminRoleID},
		Status:       domain.UserStatusActive,
	}); err != nil {
		return err
	}
	if _, err := s.CreateUser(domain.User{
		Username:     "operator",
		Nickname:     "GOV2 Operator",
		Email:        "operator@gov2.local",
		PasswordHash: hash,
		RoleIDs:      []uint64{operatorRoleID},
		Status:       domain.UserStatusActive,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Store) scanUser(ctx context.Context, query string, args ...any) (domain.User, error) {
	user, err := scanUserRow(s.db.QueryRowContext(ctx, query, args...))
	if err != nil {
		return domain.User{}, err
	}
	roleIDs, err := s.roleIDsForUser(ctx, user.ID)
	if err != nil {
		return domain.User{}, err
	}
	user.RoleIDs = roleIDs
	return user, nil
}

func scanUserRow(row scanner) (domain.User, error) {
	var user domain.User
	var lastLogin sql.NullTime
	err := row.Scan(&user.ID, &user.Username, &user.Nickname, &user.Email, &user.Phone, &user.Avatar, &user.PasswordHash, &user.Status, &user.CreatedAt, &user.UpdatedAt, &lastLogin)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, repository.ErrNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	return user, nil
}

func (s *Store) scanRole(ctx context.Context, query string, args ...any) (domain.Role, error) {
	role, err := scanRoleRow(s.db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Role{}, repository.ErrNotFound
	}
	return role, err
}

func scanRoleRow(row scanner) (domain.Role, error) {
	var role domain.Role
	err := row.Scan(&role.ID, &role.Name, &role.Code, &role.Description, &role.CreatedAt, &role.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Role{}, repository.ErrNotFound
	}
	return role, err
}

func (s *Store) scanMenu(ctx context.Context, query string, args ...any) (domain.Menu, error) {
	menu, err := scanMenuRow(s.db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Menu{}, repository.ErrNotFound
	}
	return menu, err
}

func scanMenuRow(row scanner) (domain.Menu, error) {
	var menu domain.Menu
	err := row.Scan(&menu.ID, &menu.ParentID, &menu.Title, &menu.Name, &menu.Path, &menu.Icon, &menu.Component, &menu.Permission, &menu.Sort, &menu.Hidden)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Menu{}, repository.ErrNotFound
	}
	return menu, err
}

func scanSettingRow(row scanner) (domain.Setting, error) {
	var setting domain.Setting
	var value []byte
	err := row.Scan(&setting.ID, &setting.Key, &value, &setting.Description, &setting.CreatedAt, &setting.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Setting{}, repository.ErrNotFound
	}
	if err != nil {
		return domain.Setting{}, err
	}
	if len(value) == 0 {
		value = []byte(`{}`)
	}
	setting.Value = append(json.RawMessage(nil), value...)
	return setting, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func (s *Store) roleIDsForUser(ctx context.Context, userID uint64) ([]uint64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT role_id FROM gov2_user_roles WHERE user_id = $1 ORDER BY role_id ASC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []uint64{}
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Store) permissionsForRole(ctx context.Context, roleID uint64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT p.code
FROM gov2_permissions p
JOIN gov2_role_permissions rp ON rp.permission_id = p.id
WHERE rp.role_id = $1
ORDER BY p.code ASC`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	permissions := []string{}
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return permissions, nil
}

func (s *Store) dictionaryItems(ctx context.Context, dictionaryID uint64) ([]domain.DictionaryItem, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT label, value, sort
FROM gov2_dictionary_items
WHERE dictionary_id = $1
ORDER BY sort ASC, id ASC`, dictionaryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []domain.DictionaryItem{}
	for rows.Next() {
		var item domain.DictionaryItem
		if err := rows.Scan(&item.Label, &item.Value, &item.Sort); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) dictionaryByID(id uint64) (domain.Dictionary, error) {
	ctx := context.Background()
	var dictionary domain.Dictionary
	err := s.db.QueryRowContext(ctx, `
SELECT id, code, name, created_at, updated_at
FROM gov2_dictionaries
WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&dictionary.ID, &dictionary.Code, &dictionary.Name, &dictionary.CreatedAt, &dictionary.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Dictionary{}, repository.ErrNotFound
	}
	if err != nil {
		return domain.Dictionary{}, err
	}
	items, err := s.dictionaryItems(ctx, dictionary.ID)
	if err != nil {
		return domain.Dictionary{}, err
	}
	dictionary.Items = items
	return dictionary, nil
}

func (s *Store) settingByID(id uint64) (domain.Setting, error) {
	return scanSettingRow(s.db.QueryRowContext(context.Background(), `
SELECT id, "key", value_json, description, created_at, updated_at
FROM gov2_settings
WHERE id = $1`, id))
}

func (s *Store) userIDByUsername(ctx context.Context, username string) (uint64, bool, error) {
	var id uint64
	err := s.db.QueryRowContext(ctx, "SELECT id FROM gov2_users WHERE lower(username) = lower($1) AND deleted_at IS NULL", username).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return 0, false, err
}

func (s *Store) roleIDByCode(ctx context.Context, code string) (uint64, bool, error) {
	var id uint64
	err := s.db.QueryRowContext(ctx, "SELECT id FROM gov2_roles WHERE lower(code) = lower($1) AND deleted_at IS NULL", code).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return 0, false, err
}

func (s *Store) menuIDByName(ctx context.Context, name string) (uint64, bool, error) {
	var id uint64
	err := s.db.QueryRowContext(ctx, "SELECT id FROM gov2_menus WHERE lower(name) = lower($1) AND deleted_at IS NULL", name).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return 0, false, err
}

func (s *Store) dictionaryIDByCode(ctx context.Context, code string) (uint64, bool, error) {
	var id uint64
	err := s.db.QueryRowContext(ctx, "SELECT id FROM gov2_dictionaries WHERE lower(code) = lower($1) AND deleted_at IS NULL", code).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return 0, false, err
}

func (s *Store) settingIDByKey(ctx context.Context, key string) (uint64, bool, error) {
	var id uint64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM gov2_settings WHERE lower("key") = lower($1)`, key).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return 0, false, err
}

func (s *Store) ensureMenuParent(ctx context.Context, parentID uint64) error {
	if parentID == 0 {
		return nil
	}
	exists, err := s.menuExists(ctx, parentID)
	if err != nil {
		return err
	}
	if !exists {
		return repository.ErrNotFound
	}
	return nil
}

func (s *Store) menuExists(ctx context.Context, id uint64) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM gov2_menus WHERE id = $1 AND deleted_at IS NULL)", id).Scan(&exists)
	return exists, err
}

func (s *Store) menuHasAncestor(ctx context.Context, candidateID, ancestorID uint64) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
WITH RECURSIVE ancestors AS (
  SELECT id, parent_id
  FROM gov2_menus
  WHERE id = $1 AND deleted_at IS NULL
  UNION ALL
  SELECT parent.id, parent.parent_id
  FROM gov2_menus parent
  JOIN ancestors child ON parent.id = child.parent_id
  WHERE parent.deleted_at IS NULL
)
SELECT EXISTS (SELECT 1 FROM ancestors WHERE id = $2)`, candidateID, ancestorID).Scan(&exists)
	return exists, err
}

func (s *Store) ensureRole(ctx context.Context, name, code, description string, permissions []string) (uint64, error) {
	if id, found, err := s.roleIDByCode(ctx, code); err != nil {
		return 0, err
	} else if found {
		err := s.withTx(ctx, func(tx *sql.Tx) error {
			for _, permission := range permissions {
				if err := assignRolePermission(ctx, tx, id, permission); err != nil {
					return err
				}
			}
			return nil
		})
		return id, err
	}
	role, err := s.CreateRole(domain.Role{
		Name:        name,
		Code:        code,
		Description: description,
		Permissions: permissions,
	})
	if err != nil {
		return 0, err
	}
	return role.ID, nil
}

func replaceUserRoles(ctx context.Context, tx *sql.Tx, userID uint64, roleIDs []uint64) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM gov2_user_roles WHERE user_id = $1", userID); err != nil {
		return err
	}
	for _, roleID := range roleIDs {
		if _, err := tx.ExecContext(ctx, "INSERT INTO gov2_user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", userID, roleID); err != nil {
			return err
		}
	}
	return nil
}

func replaceRolePermissions(ctx context.Context, tx *sql.Tx, roleID uint64, permissions []string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM gov2_role_permissions WHERE role_id = $1", roleID); err != nil {
		return err
	}
	for _, permission := range permissions {
		if err := assignRolePermission(ctx, tx, roleID, permission); err != nil {
			return err
		}
	}
	return nil
}

func assignRolePermission(ctx context.Context, tx *sql.Tx, roleID uint64, permission string) error {
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return nil
	}
	if err := requirePermission(ctx, tx, permission); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO gov2_role_permissions (role_id, permission_id)
SELECT $1, id
FROM gov2_permissions
WHERE code = $2
ON CONFLICT DO NOTHING`, roleID, permission)
	return err
}

func replaceDictionaryItems(ctx context.Context, tx *sql.Tx, dictionaryID uint64, items []domain.DictionaryItem) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM gov2_dictionary_items WHERE dictionary_id = $1", dictionaryID); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO gov2_dictionary_items (dictionary_id, label, value, sort)
VALUES ($1, $2, $3, $4)`, dictionaryID, item.Label, item.Value, item.Sort); err != nil {
			return mapWriteError(err)
		}
	}
	return nil
}

func requirePermission(ctx context.Context, tx *sql.Tx, permission string) error {
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return nil
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM gov2_permissions WHERE code = $1)", permission).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return repository.ErrInvalidReference
	}
	return nil
}

func nullableUint64(value uint64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func buildMenuTree(items []domain.Menu) []domain.Menu {
	nodes := make([]domain.Menu, len(items))
	copy(nodes, items)

	byID := make(map[uint64]*domain.Menu, len(nodes))
	for i := range nodes {
		nodes[i].Children = nil
		byID[nodes[i].ID] = &nodes[i]
	}
	for i := range nodes {
		menu := &nodes[i]
		if menu.ParentID == 0 {
			continue
		}
		parent := byID[menu.ParentID]
		if parent != nil {
			parent.Children = append(parent.Children, *menu)
		}
	}
	roots := make([]domain.Menu, 0)
	for i := range nodes {
		if nodes[i].ParentID == 0 || byID[nodes[i].ParentID] == nil {
			roots = append(roots, nodes[i])
		}
	}
	sortMenus(roots)
	return roots
}

func sortMenus(items []domain.Menu) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Sort != items[j].Sort {
			return items[i].Sort < items[j].Sort
		}
		return items[i].ID < items[j].ID
	})
	for i := range items {
		sortMenus(items[i].Children)
	}
}

func normalizePage(page, pageSize int) (int, int) {
	return repository.NormalizePage(page, pageSize)
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}

func requireRowsAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func execRequireRowsAffected(ctx context.Context, tx *sql.Tx, query string, args ...any) error {
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	return requireRowsAffected(result)
}

func mapWriteError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23505":
		return repository.ErrConflict
	case "23503":
		return repository.ErrInvalidReference
	case "23502", "23514", "22P02":
		return repository.ErrConstraint
	default:
		if strings.HasPrefix(pgErr.Code, "23") {
			return repository.ErrConstraint
		}
	}
	return err
}
