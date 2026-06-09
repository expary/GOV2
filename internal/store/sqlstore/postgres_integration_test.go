package sqlstore_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/migration"
	"github.com/expary/GOV2/internal/repository"
	"github.com/expary/GOV2/internal/security"
	_ "github.com/expary/GOV2/internal/store/postgres"
	"github.com/expary/GOV2/internal/store/sqlstore"
)

func TestPostgresStoreIntegration(t *testing.T) {
	store, db := newPostgresIntegrationStore(t)
	suffix := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)

	t.Run("seed and user lifecycle", func(t *testing.T) {
		admin, err := store.FindUserByUsername("admin")
		if err != nil {
			t.Fatalf("find admin: %v", err)
		}
		if admin.PasswordHash == "" || len(admin.RoleIDs) == 0 {
			t.Fatalf("expected seeded admin credentials and roles, got %+v", admin)
		}

		passwordHash := testPasswordHash(t)
		username := "it_user_" + suffix
		created, err := store.CreateUser(domain.User{
			Username:     username,
			Nickname:     "Integration User",
			Email:        username + "@gov2.local",
			Phone:        "155" + suffix[len(suffix)-8:],
			PasswordHash: passwordHash,
			RoleIDs:      admin.RoleIDs,
			Status:       domain.UserStatusActive,
		})
		if err != nil {
			t.Fatalf("create user: %v", err)
		}

		if _, err := store.CreateUser(domain.User{
			Username:     username,
			PasswordHash: passwordHash,
			Status:       domain.UserStatusActive,
		}); !errors.Is(err, repository.ErrConflict) {
			t.Fatalf("expected duplicate username conflict, got %v", err)
		}
		if _, err := store.CreateUser(domain.User{
			Username:     "it_user_email_" + suffix,
			Email:        strings.ToUpper(username + "@gov2.local"),
			PasswordHash: passwordHash,
			Status:       domain.UserStatusActive,
		}); !errors.Is(err, repository.ErrConflict) {
			t.Fatalf("expected duplicate email conflict, got %v", err)
		}
		if _, err := store.CreateUser(domain.User{
			Username:     "it_user_phone_" + suffix,
			Phone:        created.Phone,
			PasswordHash: passwordHash,
			Status:       domain.UserStatusActive,
		}); !errors.Is(err, repository.ErrConflict) {
			t.Fatalf("expected duplicate phone conflict, got %v", err)
		}
		second, err := store.CreateUser(domain.User{
			Username:     "it_user_second_" + suffix,
			Email:        "it_user_second_" + suffix + "@gov2.local",
			Phone:        "156" + suffix[len(suffix)-8:],
			PasswordHash: passwordHash,
			Status:       domain.UserStatusActive,
		})
		if err != nil {
			t.Fatalf("create second user: %v", err)
		}
		if _, err := store.UpdateUser(second.ID, domain.User{
			Username: second.Username,
			Email:    strings.ToUpper(created.Email),
			Phone:    second.Phone,
			Status:   domain.UserStatusActive,
		}); !errors.Is(err, repository.ErrConflict) {
			t.Fatalf("expected update email conflict, got %v", err)
		}
		if _, err := store.UpdateUser(second.ID, domain.User{
			Username: second.Username,
			Email:    second.Email,
			Phone:    created.Phone,
			Status:   domain.UserStatusActive,
		}); !errors.Is(err, repository.ErrConflict) {
			t.Fatalf("expected update phone conflict, got %v", err)
		}
		roles, err := store.ListRoles()
		if err != nil {
			t.Fatalf("list roles for user update: %v", err)
		}
		adminRole := roleByCode(t, roles, "admin")
		operatorRole := roleByCode(t, roles, "operator")
		newPasswordHash := testPasswordHash(t)
		updated, err := store.UpdateUser(created.ID, domain.User{
			Username:     created.Username,
			Nickname:     "Integration User Updated",
			Email:        created.Email,
			Phone:        created.Phone,
			PasswordHash: newPasswordHash,
			RoleIDs:      []uint64{operatorRole.ID},
			Status:       domain.UserStatusActive,
		})
		if err != nil {
			t.Fatalf("update user password and roles: %v", err)
		}
		if updated.PasswordHash != newPasswordHash {
			t.Fatalf("expected password hash to be updated")
		}
		if !sameUint64s(updated.RoleIDs, []uint64{operatorRole.ID}) {
			t.Fatalf("expected password update to replace roles with operator role, got %+v", updated.RoleIDs)
		}
		updated, err = store.UpdateUser(created.ID, domain.User{
			Username: created.Username,
			Nickname: updated.Nickname,
			Email:    created.Email,
			Phone:    created.Phone,
			RoleIDs:  []uint64{adminRole.ID},
			Status:   domain.UserStatusActive,
		})
		if err != nil {
			t.Fatalf("update user roles without password: %v", err)
		}
		if !sameUint64s(updated.RoleIDs, []uint64{adminRole.ID}) {
			t.Fatalf("expected non-password update to replace roles with admin role, got %+v", updated.RoleIDs)
		}

		users, total, err := store.ListUsers(repository.UserQuery{Keyword: username, Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("list users: %v", err)
		}
		if total < 1 || len(users) != 1 {
			t.Fatalf("expected one matching user, got len=%d total=%d", len(users), total)
		}

		if err := store.DeleteUser(second.ID); err != nil {
			t.Fatalf("delete second user: %v", err)
		}
		if err := store.DeleteUser(created.ID); err != nil {
			t.Fatalf("delete user: %v", err)
		}
		if _, err := store.GetUser(created.ID); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("expected deleted user not found, got %v", err)
		}
	})

	t.Run("roles persist permissions and protect assigned roles", func(t *testing.T) {
		roleCode := "it_role_" + suffix
		role, err := store.CreateRole(domain.Role{
			Name:        "Integration Role",
			Code:        roleCode,
			Description: "Created by PostgreSQL integration test",
			Permissions: []string{
				domain.PermissionSystemAuditList,
				domain.PermissionSystemRoleList,
			},
		})
		if err != nil {
			t.Fatalf("create role: %v", err)
		}
		if !containsString(role.Permissions, domain.PermissionSystemRoleList) {
			t.Fatalf("expected role permissions to include %q, got %+v", domain.PermissionSystemRoleList, role.Permissions)
		}

		if _, err := store.CreateRole(domain.Role{Name: "Duplicate Role", Code: strings.ToUpper(roleCode)}); !errors.Is(err, repository.ErrConflict) {
			t.Fatalf("expected duplicate role code conflict, got %v", err)
		}
		if _, err := store.CreateRole(domain.Role{
			Name:        "Unknown Permission Role",
			Code:        "it_unknown_permission_role_" + suffix,
			Permissions: []string{"integration:role:" + suffix},
		}); !errors.Is(err, repository.ErrInvalidReference) {
			t.Fatalf("expected unknown role permission invalid reference, got %v", err)
		}

		updated, err := store.UpdateRole(role.ID, domain.Role{
			Name:        "Integration Role Updated",
			Code:        roleCode,
			Description: "Updated by PostgreSQL integration test",
			Permissions: []string{
				domain.PermissionSystemSettingList,
			},
		})
		if err != nil {
			t.Fatalf("update role: %v", err)
		}
		if updated.Name != "Integration Role Updated" || !containsString(updated.Permissions, domain.PermissionSystemSettingList) || containsString(updated.Permissions, domain.PermissionSystemRoleList) {
			t.Fatalf("expected updated role permissions to replace previous values, got %+v", updated)
		}
		if _, err := store.UpdateRole(role.ID, domain.Role{
			Name:        "Integration Role Invalid",
			Code:        roleCode,
			Description: "Unknown permission update should roll back",
			Permissions: []string{
				"integration:role:update:" + suffix,
			},
		}); !errors.Is(err, repository.ErrInvalidReference) {
			t.Fatalf("expected unknown role permission update invalid reference, got %v", err)
		}
		unchangedRole, err := store.GetRole(role.ID)
		if err != nil {
			t.Fatalf("get role after failed permission update: %v", err)
		}
		if unchangedRole.Name != updated.Name || !containsString(unchangedRole.Permissions, domain.PermissionSystemSettingList) {
			t.Fatalf("expected failed role permission update to roll back, got %+v", unchangedRole)
		}

		createdUser, err := store.CreateUser(domain.User{
			Username:     "it_role_user_" + suffix,
			PasswordHash: testPasswordHash(t),
			RoleIDs:      []uint64{role.ID},
			Status:       domain.UserStatusActive,
		})
		if err != nil {
			t.Fatalf("create role-assigned user: %v", err)
		}
		if err := store.DeleteRole(role.ID); !errors.Is(err, repository.ErrConflict) {
			t.Fatalf("expected assigned role delete conflict, got %v", err)
		}
		if err := store.DeleteUser(createdUser.ID); err != nil {
			t.Fatalf("delete role-assigned user: %v", err)
		}
		if err := store.DeleteRole(role.ID); err != nil {
			t.Fatalf("delete unassigned role: %v", err)
		}
		if _, err := store.GetRole(role.ID); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("expected deleted role not found, got %v", err)
		}
	})

	t.Run("menus persist hierarchy and block cycles", func(t *testing.T) {
		parent, err := store.CreateMenu(domain.Menu{
			Title:     "Integration",
			Name:      "it-menu-" + suffix,
			Path:      "/it-" + suffix,
			Component: "Layout",
			Sort:      700,
		})
		if err != nil {
			t.Fatalf("create parent menu: %v", err)
		}
		child, err := store.CreateMenu(domain.Menu{
			ParentID:   parent.ID,
			Title:      "Integration Child",
			Name:       "it-menu-child-" + suffix,
			Path:       "/it-" + suffix + "/child",
			Component:  "IntegrationChildView",
			Permission: domain.PermissionDashboardView,
			Sort:       701,
		})
		if err != nil {
			t.Fatalf("create child menu: %v", err)
		}
		if _, err := store.CreateMenu(domain.Menu{
			ParentID:   parent.ID,
			Title:      "Unknown Permission Child",
			Name:       "it-menu-unknown-permission-" + suffix,
			Path:       "/it-" + suffix + "/unknown-permission",
			Component:  "IntegrationUnknownPermissionView",
			Permission: "integration:menu:" + suffix,
			Sort:       703,
		}); !errors.Is(err, repository.ErrInvalidReference) {
			t.Fatalf("expected unknown menu permission invalid reference, got %v", err)
		}

		menus, err := store.ListMenus()
		if err != nil {
			t.Fatalf("list menus: %v", err)
		}
		if !menuTreeHasChild(menus, parent.ID, child.ID) {
			t.Fatalf("expected menu tree to contain parent=%d child=%d", parent.ID, child.ID)
		}
		if err := store.DeleteMenu(parent.ID); !errors.Is(err, repository.ErrConflict) {
			t.Fatalf("expected parent menu delete conflict, got %v", err)
		}
		if _, err := store.UpdateMenu(parent.ID, domain.Menu{
			ParentID:  child.ID,
			Title:     parent.Title,
			Name:      parent.Name,
			Path:      parent.Path,
			Component: parent.Component,
			Sort:      parent.Sort,
		}); !errors.Is(err, repository.ErrConflict) {
			t.Fatalf("expected menu cycle conflict, got %v", err)
		}

		updatedChild, err := store.UpdateMenu(child.ID, domain.Menu{
			ParentID:   parent.ID,
			Title:      "Integration Child Updated",
			Name:       child.Name,
			Path:       child.Path,
			Component:  child.Component,
			Permission: child.Permission,
			Sort:       702,
			Hidden:     true,
		})
		if err != nil {
			t.Fatalf("update child menu: %v", err)
		}
		if updatedChild.Title != "Integration Child Updated" || !updatedChild.Hidden {
			t.Fatalf("expected updated child menu state, got %+v", updatedChild)
		}
		if _, err := store.UpdateMenu(child.ID, domain.Menu{
			ParentID:   parent.ID,
			Title:      "Integration Child Invalid",
			Name:       child.Name,
			Path:       child.Path,
			Component:  child.Component,
			Permission: "integration:menu:update:" + suffix,
			Sort:       704,
		}); !errors.Is(err, repository.ErrInvalidReference) {
			t.Fatalf("expected unknown menu permission update invalid reference, got %v", err)
		}
		unchangedChild, err := store.GetMenu(child.ID)
		if err != nil {
			t.Fatalf("get child menu after failed permission update: %v", err)
		}
		if unchangedChild.Title != updatedChild.Title || unchangedChild.Permission != updatedChild.Permission || unchangedChild.Sort != updatedChild.Sort {
			t.Fatalf("expected failed menu permission update to roll back, got %+v", unchangedChild)
		}

		if err := store.DeleteMenu(child.ID); err != nil {
			t.Fatalf("delete child menu: %v", err)
		}
		if err := store.DeleteMenu(parent.ID); err != nil {
			t.Fatalf("delete parent menu: %v", err)
		}
	})

	t.Run("dictionaries replace items and enforce code uniqueness", func(t *testing.T) {
		code := "it_dictionary_" + suffix
		dictionary, err := store.CreateDictionary(domain.Dictionary{
			Code: code,
			Name: "Integration Dictionary",
			Items: []domain.DictionaryItem{
				{Label: "Open", Value: "open", Sort: 1},
				{Label: "Closed", Value: "closed", Sort: 2},
			},
		})
		if err != nil {
			t.Fatalf("create dictionary: %v", err)
		}
		if len(dictionary.Items) != 2 {
			t.Fatalf("expected two dictionary items, got %+v", dictionary)
		}
		if _, err := store.CreateDictionary(domain.Dictionary{Code: strings.ToUpper(code), Name: "Duplicate Dictionary"}); !errors.Is(err, repository.ErrConflict) {
			t.Fatalf("expected duplicate dictionary code conflict, got %v", err)
		}

		updated, err := store.UpdateDictionary(dictionary.ID, domain.Dictionary{
			Code: code,
			Name: "Integration Dictionary Updated",
			Items: []domain.DictionaryItem{
				{Label: "Done", Value: "done", Sort: 3},
			},
		})
		if err != nil {
			t.Fatalf("update dictionary: %v", err)
		}
		if updated.Name != "Integration Dictionary Updated" || len(updated.Items) != 1 || updated.Items[0].Value != "done" {
			t.Fatalf("expected dictionary items to be replaced, got %+v", updated)
		}
		found, err := store.GetDictionaryByCode(strings.ToUpper(code))
		if err != nil {
			t.Fatalf("get dictionary by uppercase code: %v", err)
		}
		if found.ID != dictionary.ID {
			t.Fatalf("expected dictionary lookup to return id=%d, got %+v", dictionary.ID, found)
		}

		if err := store.DeleteDictionary(dictionary.ID); err != nil {
			t.Fatalf("delete dictionary: %v", err)
		}
		if _, err := store.GetDictionaryByCode(code); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("expected deleted dictionary not found, got %v", err)
		}
	})

	t.Run("settings persist json and enforce key uniqueness", func(t *testing.T) {
		key := "it.setting." + suffix
		setting, err := store.CreateSetting(domain.Setting{
			Key:         key,
			Value:       json.RawMessage(`{"enabled":true}`),
			Description: "Integration setting",
		})
		if err != nil {
			t.Fatalf("create setting: %v", err)
		}
		if !jsonEqual(setting.Value, json.RawMessage(`{"enabled":true}`)) {
			t.Fatalf("expected created setting JSON to round-trip, got %s", string(setting.Value))
		}
		if _, err := store.CreateSetting(domain.Setting{Key: strings.ToUpper(key), Value: json.RawMessage(`false`)}); !errors.Is(err, repository.ErrConflict) {
			t.Fatalf("expected duplicate setting key conflict, got %v", err)
		}

		updated, err := store.UpdateSetting(setting.ID, domain.Setting{
			Key:         key,
			Value:       json.RawMessage(`{"enabled":false}`),
			Description: "Updated integration setting",
		})
		if err != nil {
			t.Fatalf("update setting: %v", err)
		}
		if updated.Description != "Updated integration setting" || !jsonEqual(updated.Value, json.RawMessage(`{"enabled":false}`)) {
			t.Fatalf("expected updated setting JSON and description, got %+v", updated)
		}
		if err := store.DeleteSetting(setting.ID); err != nil {
			t.Fatalf("delete setting: %v", err)
		}
		settings, err := store.ListSettings()
		if err != nil {
			t.Fatalf("list settings: %v", err)
		}
		if settingExists(settings, key) {
			t.Fatalf("expected deleted setting %q to be absent", key)
		}
	})

	t.Run("seed preserves existing site title setting", func(t *testing.T) {
		siteTitle := settingByKey(t, store, "site.title")
		customTitle := "Integration GOV2 " + suffix
		if _, err := store.UpdateSetting(siteTitle.ID, domain.Setting{
			Key:         siteTitle.Key,
			Value:       json.RawMessage(strconv.Quote(customTitle)),
			Description: siteTitle.Description,
		}); err != nil {
			t.Fatalf("update site.title before reseed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		runner := migration.NewRunner(db, "../../../migrations")
		if _, err := runner.RunSeeds(ctx, "../../../migrations/seeds"); err != nil {
			t.Fatalf("rerun seeds: %v", err)
		}

		siteTitle = settingByKey(t, store, "site.title")
		if !jsonEqual(siteTitle.Value, json.RawMessage(strconv.Quote(customTitle))) {
			t.Fatalf("expected reseed to preserve site.title %q, got %s", customTitle, string(siteTitle.Value))
		}
	})

	t.Run("audit filters and dashboard summary use persisted rows", func(t *testing.T) {
		before, err := store.Summary()
		if err != nil {
			t.Fatalf("summary before audit: %v", err)
		}
		log, err := store.AddAuditLog(domain.AuditLog{
			Actor:      "it_actor_" + suffix,
			Action:     "integration",
			Resource:   "sqlstore",
			ResourceID: "resource-" + suffix,
			IP:         "127.0.0.1",
			UserAgent:  "postgres-integration-test",
			Detail:     "integration audit " + suffix,
		})
		if err != nil {
			t.Fatalf("add audit log: %v", err)
		}
		if log.ID == 0 {
			t.Fatalf("expected audit log id, got %+v", log)
		}

		logs, total, err := store.ListAuditLogs(repository.AuditLogQuery{
			Keyword:    suffix,
			Actor:      log.Actor,
			Action:     log.Action,
			Resource:   log.Resource,
			ResourceID: log.ResourceID,
			Page:       1,
			PageSize:   10,
		})
		if err != nil {
			t.Fatalf("list audit logs: %v", err)
		}
		if total != 1 || len(logs) != 1 || logs[0].ID != log.ID || logs[0].ResourceID != log.ResourceID {
			t.Fatalf("expected one filtered audit log id=%d, total=%d logs=%+v", log.ID, total, logs)
		}

		after, err := store.Summary()
		if err != nil {
			t.Fatalf("summary after audit: %v", err)
		}
		if after.AuditLogCount != before.AuditLogCount+1 {
			t.Fatalf("expected audit log count to increment from %d to %d, got %d", before.AuditLogCount, before.AuditLogCount+1, after.AuditLogCount)
		}
		if after.UserCount < 2 || after.RoleCount < 2 || after.MenuCount < 2 {
			t.Fatalf("expected seeded summary counts, got %+v", after)
		}
	})

	t.Run("development bootstrap repairs built-in role permissions", func(t *testing.T) {
		before, err := store.Summary()
		if err != nil {
			t.Fatalf("summary before repair: %v", err)
		}
		if _, err := db.Exec(`
DELETE FROM gov2_role_permissions rp
USING gov2_roles r, gov2_permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND lower(r.code) = lower($1)
  AND p.code = $2`, "operator", domain.PermissionSystemAuditList); err != nil {
			t.Fatalf("delete operator audit permission: %v", err)
		}

		roles, err := store.ListRoles()
		if err != nil {
			t.Fatalf("list roles before repair: %v", err)
		}
		operator := roleByCode(t, roles, "operator")
		if containsString(operator.Permissions, domain.PermissionSystemAuditList) {
			t.Fatalf("expected operator audit permission to be removed before repair, got %+v", operator.Permissions)
		}

		if err := store.BootstrapDevelopmentData(); err != nil {
			t.Fatalf("bootstrap development data repair: %v", err)
		}
		after, err := store.Summary()
		if err != nil {
			t.Fatalf("summary after repair: %v", err)
		}
		if after.UserCount != before.UserCount {
			t.Fatalf("expected bootstrap repair not to create users when users already exist, before=%+v after=%+v", before, after)
		}
		roles, err = store.ListRoles()
		if err != nil {
			t.Fatalf("list roles after repair: %v", err)
		}
		operator = roleByCode(t, roles, "operator")
		if !containsString(operator.Permissions, domain.PermissionSystemAuditList) {
			t.Fatalf("expected bootstrap repair to restore operator audit permission, got %+v", operator.Permissions)
		}
	})
}

func newPostgresIntegrationStore(t *testing.T) (*sqlstore.Store, *sql.DB) {
	t.Helper()

	dsn := os.Getenv("GOV2_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set GOV2_TEST_POSTGRES_DSN to run PostgreSQL integration tests")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}

	runner := migration.NewRunner(db, "../../../migrations")
	if _, err := runner.RunUp(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	if _, err := runner.RunSeeds(ctx, "../../../migrations/seeds"); err != nil {
		t.Fatalf("run seeds: %v", err)
	}

	store := sqlstore.New(db)
	if err := store.BootstrapDevelopmentData(); err != nil {
		t.Fatalf("bootstrap development data: %v", err)
	}

	return store, db
}

func testPasswordHash(t *testing.T) string {
	t.Helper()

	passwordHash, err := security.HashPassword("pass12345")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return passwordHash
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func roleByCode(t *testing.T, roles []domain.Role, code string) domain.Role {
	t.Helper()

	for _, role := range roles {
		if strings.EqualFold(role.Code, code) {
			return role
		}
	}
	t.Fatalf("role %q not found in %+v", code, roles)
	return domain.Role{}
}

func menuTreeHasChild(menus []domain.Menu, parentID, childID uint64) bool {
	for _, menu := range menus {
		if menu.ID == parentID {
			for _, child := range menu.Children {
				if child.ID == childID {
					return true
				}
			}
		}
		if menuTreeHasChild(menu.Children, parentID, childID) {
			return true
		}
	}
	return false
}

func jsonEqual(a, b json.RawMessage) bool {
	var left any
	var right any
	if err := json.Unmarshal(a, &left); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &right); err != nil {
		return false
	}
	return isJSONEqual(left, right)
}

func isJSONEqual(a, b any) bool {
	return jsonString(a) == jsonString(b)
}

func jsonString(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func settingExists(settings []domain.Setting, key string) bool {
	for _, setting := range settings {
		if strings.EqualFold(setting.Key, key) {
			return true
		}
	}
	return false
}

func settingByKey(t *testing.T, store *sqlstore.Store, key string) domain.Setting {
	t.Helper()

	settings, err := store.ListSettings()
	if err != nil {
		t.Fatalf("list settings: %v", err)
	}
	for _, setting := range settings {
		if strings.EqualFold(setting.Key, key) {
			return setting
		}
	}
	t.Fatalf("expected setting %q to exist", key)
	return domain.Setting{}
}

func sameUint64s(left, right []uint64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
