package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/repository"
	"github.com/expary/GOV2/internal/security"
)

var (
	ErrNotFound = repository.ErrNotFound
	ErrConflict = repository.ErrConflict
)

type Store struct {
	mu sync.RWMutex

	nextUserID       uint64
	nextRoleID       uint64
	nextMenuID       uint64
	nextDictionaryID uint64
	nextSettingID    uint64
	nextAuditLogID   uint64

	users        map[uint64]domain.User
	roles        map[uint64]domain.Role
	menus        map[uint64]domain.Menu
	dictionaries map[uint64]domain.Dictionary
	settings     map[uint64]domain.Setting
	auditLogs    map[uint64]domain.AuditLog
}

func NewStore() *Store {
	return &Store{
		nextUserID:       1,
		nextRoleID:       1,
		nextMenuID:       1,
		nextDictionaryID: 1,
		nextSettingID:    1,
		nextAuditLogID:   1,
		users:            map[uint64]domain.User{},
		roles:            map[uint64]domain.Role{},
		menus:            map[uint64]domain.Menu{},
		dictionaries:     map[uint64]domain.Dictionary{},
		settings:         map[uint64]domain.Setting{},
		auditLogs:        map[uint64]domain.AuditLog{},
	}
}

func (s *Store) CheckHealth(context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return nil
}

func (s *Store) Seed() error {
	now := time.Now().UTC()
	adminHash, err := security.HashPassword("admin123")
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	adminRole := s.insertRoleLocked(domain.Role{
		Name:        "Administrator",
		Code:        "admin",
		Description: "Full system access",
		Permissions: []string{domain.PermissionAll},
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	operatorRole := s.insertRoleLocked(domain.Role{
		Name:        "Operator",
		Code:        "operator",
		Description: "Daily system operations",
		Permissions: []string{
			domain.PermissionDashboardView,
			domain.PermissionSystemUserList,
			domain.PermissionSystemRoleList,
			domain.PermissionSystemMenuList,
			domain.PermissionSystemModuleList,
			domain.PermissionSystemDictionaryList,
			domain.PermissionSystemSettingList,
			domain.PermissionSystemAuditList,
		},
		CreatedAt: now,
		UpdatedAt: now,
	})

	s.insertUserLocked(domain.User{
		Username:     "admin",
		Nickname:     "GOV2 Admin",
		Email:        "admin@gov2.local",
		PasswordHash: adminHash,
		RoleIDs:      []uint64{adminRole.ID},
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	s.insertUserLocked(domain.User{
		Username:     "operator",
		Nickname:     "GOV2 Operator",
		Email:        "operator@gov2.local",
		PasswordHash: adminHash,
		RoleIDs:      []uint64{operatorRole.ID},
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	menus := []domain.Menu{
		{Title: "Dashboard", Name: "dashboard", Path: "/dashboard", Icon: "layout-dashboard", Component: "DashboardView", Permission: domain.PermissionDashboardView, Sort: 10},
		{Title: "System", Name: "system", Path: "/system", Icon: "settings", Component: "Layout", Sort: 20},
		{Title: "Users", Name: "system-users", ParentID: 2, Path: "/system/users", Icon: "users", Component: "UsersView", Permission: domain.PermissionSystemUserList, Sort: 21},
		{Title: "Roles", Name: "system-roles", ParentID: 2, Path: "/system/roles", Icon: "shield", Component: "RolesView", Permission: domain.PermissionSystemRoleList, Sort: 22},
		{Title: "Menus", Name: "system-menus", ParentID: 2, Path: "/system/menus", Icon: "menu", Component: "MenusView", Permission: domain.PermissionSystemMenuList, Sort: 23},
		{Title: "Modules", Name: "system-modules", ParentID: 2, Path: "/system/modules", Icon: "boxes", Component: "ModulesView", Permission: domain.PermissionSystemModuleList, Sort: 24},
		{Title: "Dictionaries", Name: "system-dictionaries", ParentID: 2, Path: "/system/dictionaries", Icon: "book", Component: "DictionariesView", Permission: domain.PermissionSystemDictionaryList, Sort: 25},
		{Title: "Settings", Name: "system-settings", ParentID: 2, Path: "/system/settings", Icon: "sliders-horizontal", Component: "SettingsView", Permission: domain.PermissionSystemSettingList, Sort: 26},
		{Title: "Audit Logs", Name: "system-audit", ParentID: 2, Path: "/system/audit", Icon: "history", Component: "AuditLogsView", Permission: domain.PermissionSystemAuditList, Sort: 27},
	}
	for _, menu := range menus {
		menu.ID = s.nextMenuID
		s.nextMenuID++
		s.menus[menu.ID] = menu
	}

	s.insertDictionaryLocked(domain.Dictionary{
		Code: "user_status",
		Name: "User Status",
		Items: []domain.DictionaryItem{
			{Label: "Active", Value: domain.UserStatusActive, Sort: 1},
			{Label: "Disabled", Value: domain.UserStatusDisabled, Sort: 2},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	s.insertSettingLocked(domain.Setting{
		Key:         "site.title",
		Value:       []byte(`"GOV2"`),
		Description: "Displayed application title",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	s.insertAuditLocked(domain.AuditLog{
		ActorID:   adminRole.ID,
		Actor:     "system",
		Action:    "seed",
		Resource:  "store",
		IP:        "127.0.0.1",
		UserAgent: "gov2",
		Detail:    "initial data created",
		CreatedAt: now,
	})

	return nil
}

func (s *Store) FindUserByUsername(username string) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.users {
		if strings.EqualFold(user.Username, username) {
			return cloneUser(user), nil
		}
	}
	return domain.User{}, ErrNotFound
}

func (s *Store) GetUser(id uint64) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[id]
	if !ok {
		return domain.User{}, ErrNotFound
	}
	return cloneUser(user), nil
}

func (s *Store) ListUsers(filter repository.UserQuery) ([]domain.User, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filter.Page, filter.PageSize = normalizePage(filter.Page, filter.PageSize)
	keyword := strings.ToLower(strings.TrimSpace(filter.Keyword))
	status := strings.TrimSpace(filter.Status)

	items := make([]domain.User, 0, len(s.users))
	for _, user := range s.users {
		if status != "" && user.Status != status {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(user.Username+" "+user.Nickname+" "+user.Email), keyword) {
			continue
		}
		items = append(items, cloneUser(user))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})

	total := len(items)
	return paginate(items, filter.Page, filter.PageSize), total, nil
}

func (s *Store) CreateUser(user domain.User) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.users {
		if userConflicts(existing, user, 0) {
			return domain.User{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now
	return cloneUser(s.insertUserLocked(user)), nil
}

func (s *Store) UpdateUser(id uint64, update domain.User) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[id]
	if !ok {
		return domain.User{}, ErrNotFound
	}
	for _, existing := range s.users {
		if userConflicts(existing, update, id) {
			return domain.User{}, ErrConflict
		}
	}

	user.Username = update.Username
	user.Nickname = update.Nickname
	user.Email = update.Email
	user.Phone = update.Phone
	user.Avatar = update.Avatar
	user.RoleIDs = append([]uint64(nil), update.RoleIDs...)
	if update.Status != "" {
		user.Status = update.Status
	}
	if update.PasswordHash != "" {
		user.PasswordHash = update.PasswordHash
	}
	user.UpdatedAt = time.Now().UTC()
	s.users[id] = cloneUser(user)
	return cloneUser(user), nil
}

func userConflicts(existing domain.User, candidate domain.User, candidateID uint64) bool {
	if existing.ID == candidateID {
		return false
	}
	if strings.EqualFold(existing.Username, candidate.Username) {
		return true
	}
	email := strings.TrimSpace(candidate.Email)
	if email != "" && strings.EqualFold(strings.TrimSpace(existing.Email), email) {
		return true
	}
	phone := strings.TrimSpace(candidate.Phone)
	if phone != "" && strings.TrimSpace(existing.Phone) == phone {
		return true
	}
	return false
}

func (s *Store) UpdateUserStatus(id uint64, status string) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[id]
	if !ok {
		return domain.User{}, ErrNotFound
	}
	user.Status = status
	user.UpdatedAt = time.Now().UTC()
	s.users[id] = cloneUser(user)
	return cloneUser(user), nil
}

func (s *Store) DeleteUser(id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[id]; !ok {
		return ErrNotFound
	}
	delete(s.users, id)
	return nil
}

func (s *Store) TouchLastLogin(id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[id]
	if !ok {
		return ErrNotFound
	}
	now := time.Now().UTC()
	user.LastLoginAt = &now
	user.UpdatedAt = now
	s.users[id] = cloneUser(user)
	return nil
}

func (s *Store) ListRoles() ([]domain.Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]domain.Role, 0, len(s.roles))
	for _, role := range s.roles {
		items = append(items, cloneRole(role))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func (s *Store) GetRole(id uint64) (domain.Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	role, ok := s.roles[id]
	if !ok {
		return domain.Role{}, ErrNotFound
	}
	return cloneRole(role), nil
}

func (s *Store) CreateRole(role domain.Role) (domain.Role, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.roles {
		if strings.EqualFold(existing.Code, role.Code) {
			return domain.Role{}, ErrConflict
		}
	}
	now := time.Now().UTC()
	role.CreatedAt = now
	role.UpdatedAt = now
	return cloneRole(s.insertRoleLocked(role)), nil
}

func (s *Store) UpdateRole(id uint64, update domain.Role) (domain.Role, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	role, ok := s.roles[id]
	if !ok {
		return domain.Role{}, ErrNotFound
	}
	for _, existing := range s.roles {
		if existing.ID != id && strings.EqualFold(existing.Code, update.Code) {
			return domain.Role{}, ErrConflict
		}
	}

	role.Name = update.Name
	role.Code = update.Code
	role.Description = update.Description
	role.Permissions = append([]string(nil), update.Permissions...)
	role.UpdatedAt = time.Now().UTC()
	s.roles[id] = cloneRole(role)
	return cloneRole(role), nil
}

func (s *Store) DeleteRole(id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.roles[id]; !ok {
		return ErrNotFound
	}
	for _, user := range s.users {
		for _, roleID := range user.RoleIDs {
			if roleID == id {
				return ErrConflict
			}
		}
	}
	delete(s.roles, id)
	return nil
}

func (s *Store) ListMenus() ([]domain.Menu, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]domain.Menu, 0, len(s.menus))
	for _, menu := range s.menus {
		items = append(items, cloneMenu(menu))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Sort < items[j].Sort
	})
	return buildMenuTree(items), nil
}

func (s *Store) GetMenu(id uint64) (domain.Menu, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	menu, ok := s.menus[id]
	if !ok {
		return domain.Menu{}, ErrNotFound
	}
	return cloneMenu(menu), nil
}

func (s *Store) CreateMenu(menu domain.Menu) (domain.Menu, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.menuNameExistsLocked(0, menu.Name) {
		return domain.Menu{}, ErrConflict
	}
	if menu.ParentID != 0 {
		if _, ok := s.menus[menu.ParentID]; !ok {
			return domain.Menu{}, ErrNotFound
		}
	}
	return cloneMenu(s.insertMenuLocked(menu)), nil
}

func (s *Store) UpdateMenu(id uint64, update domain.Menu) (domain.Menu, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	menu, ok := s.menus[id]
	if !ok {
		return domain.Menu{}, ErrNotFound
	}
	if s.menuNameExistsLocked(id, update.Name) {
		return domain.Menu{}, ErrConflict
	}
	if update.ParentID != 0 {
		if _, ok := s.menus[update.ParentID]; !ok {
			return domain.Menu{}, ErrNotFound
		}
		if update.ParentID == id || s.menuHasAncestorLocked(update.ParentID, id) {
			return domain.Menu{}, ErrConflict
		}
	}

	menu.ParentID = update.ParentID
	menu.Title = update.Title
	menu.Name = update.Name
	menu.Path = update.Path
	menu.Icon = update.Icon
	menu.Component = update.Component
	menu.Permission = update.Permission
	menu.Sort = update.Sort
	menu.Hidden = update.Hidden
	s.menus[id] = cloneMenu(menu)
	return cloneMenu(menu), nil
}

func (s *Store) DeleteMenu(id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.menus[id]; !ok {
		return ErrNotFound
	}
	for _, menu := range s.menus {
		if menu.ParentID == id {
			return ErrConflict
		}
	}
	delete(s.menus, id)
	return nil
}

func (s *Store) ListDictionaries() ([]domain.Dictionary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]domain.Dictionary, 0, len(s.dictionaries))
	for _, dictionary := range s.dictionaries {
		items = append(items, cloneDictionary(dictionary))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func (s *Store) GetDictionaryByCode(code string) (domain.Dictionary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	code = strings.TrimSpace(code)
	for _, dictionary := range s.dictionaries {
		if strings.EqualFold(dictionary.Code, code) {
			return cloneDictionary(dictionary), nil
		}
	}
	return domain.Dictionary{}, ErrNotFound
}

func (s *Store) CreateDictionary(dictionary domain.Dictionary) (domain.Dictionary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dictionaryCodeExistsLocked(0, dictionary.Code) {
		return domain.Dictionary{}, ErrConflict
	}
	now := time.Now().UTC()
	dictionary.CreatedAt = now
	dictionary.UpdatedAt = now
	return cloneDictionary(s.insertDictionaryLocked(dictionary)), nil
}

func (s *Store) UpdateDictionary(id uint64, update domain.Dictionary) (domain.Dictionary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dictionary, ok := s.dictionaries[id]
	if !ok {
		return domain.Dictionary{}, ErrNotFound
	}
	if s.dictionaryCodeExistsLocked(id, update.Code) {
		return domain.Dictionary{}, ErrConflict
	}

	dictionary.Code = update.Code
	dictionary.Name = update.Name
	dictionary.Items = append([]domain.DictionaryItem(nil), update.Items...)
	dictionary.UpdatedAt = time.Now().UTC()
	s.dictionaries[id] = cloneDictionary(dictionary)
	return cloneDictionary(dictionary), nil
}

func (s *Store) DeleteDictionary(id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.dictionaries[id]; !ok {
		return ErrNotFound
	}
	delete(s.dictionaries, id)
	return nil
}

func (s *Store) ListSettings() ([]domain.Setting, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]domain.Setting, 0, len(s.settings))
	for _, setting := range s.settings {
		items = append(items, cloneSetting(setting))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Key < items[j].Key
	})
	return items, nil
}

func (s *Store) CreateSetting(setting domain.Setting) (domain.Setting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.settingKeyExistsLocked(0, setting.Key) {
		return domain.Setting{}, ErrConflict
	}
	now := time.Now().UTC()
	setting.CreatedAt = now
	setting.UpdatedAt = now
	return cloneSetting(s.insertSettingLocked(setting)), nil
}

func (s *Store) UpdateSetting(id uint64, update domain.Setting) (domain.Setting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	setting, ok := s.settings[id]
	if !ok {
		return domain.Setting{}, ErrNotFound
	}
	if s.settingKeyExistsLocked(id, update.Key) {
		return domain.Setting{}, ErrConflict
	}
	setting.Key = update.Key
	setting.Value = append([]byte(nil), update.Value...)
	setting.Description = update.Description
	setting.UpdatedAt = time.Now().UTC()
	s.settings[id] = cloneSetting(setting)
	return cloneSetting(setting), nil
}

func (s *Store) DeleteSetting(id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.settings[id]; !ok {
		return ErrNotFound
	}
	delete(s.settings, id)
	return nil
}

func (s *Store) AddAuditLog(log domain.AuditLog) (domain.AuditLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	return s.insertAuditLocked(log), nil
}

func (s *Store) ListAuditLogs(filter repository.AuditLogQuery) ([]domain.AuditLog, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filter.Page, filter.PageSize = normalizePage(filter.Page, filter.PageSize)
	keyword := strings.ToLower(strings.TrimSpace(filter.Keyword))
	actor := strings.ToLower(strings.TrimSpace(filter.Actor))
	action := strings.ToLower(strings.TrimSpace(filter.Action))
	resource := strings.ToLower(strings.TrimSpace(filter.Resource))
	resourceID := strings.TrimSpace(filter.ResourceID)

	items := make([]domain.AuditLog, 0, len(s.auditLogs))
	for _, log := range s.auditLogs {
		if actor != "" && !strings.Contains(strings.ToLower(log.Actor), actor) {
			continue
		}
		if action != "" && !strings.EqualFold(log.Action, action) {
			continue
		}
		if resource != "" && !strings.Contains(strings.ToLower(log.Resource), resource) {
			continue
		}
		if resourceID != "" && log.ResourceID != resourceID {
			continue
		}
		if keyword != "" {
			searchText := strings.ToLower(log.Actor + " " + log.Action + " " + log.Resource + " " + log.ResourceID + " " + log.Detail + " " + log.IP + " " + log.UserAgent)
			if !strings.Contains(searchText, keyword) {
				continue
			}
		}
		items = append(items, log)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID > items[j].ID
	})

	total := len(items)
	return paginate(items, filter.Page, filter.PageSize), total, nil
}

func (s *Store) Summary() (domain.DashboardSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	active := 0
	for _, user := range s.users {
		if user.Status == domain.UserStatusActive {
			active++
		}
	}
	return domain.DashboardSummary{
		UserCount:       len(s.users),
		RoleCount:       len(s.roles),
		MenuCount:       len(s.menus),
		AuditLogCount:   len(s.auditLogs),
		ActiveUserCount: active,
	}, nil
}

func (s *Store) insertUserLocked(user domain.User) domain.User {
	user.ID = s.nextUserID
	s.nextUserID++
	user.RoleIDs = append([]uint64(nil), user.RoleIDs...)
	s.users[user.ID] = cloneUser(user)
	return user
}

func (s *Store) insertRoleLocked(role domain.Role) domain.Role {
	role.ID = s.nextRoleID
	s.nextRoleID++
	role.Permissions = append([]string(nil), role.Permissions...)
	s.roles[role.ID] = cloneRole(role)
	return role
}

func (s *Store) insertMenuLocked(menu domain.Menu) domain.Menu {
	menu.ID = s.nextMenuID
	s.nextMenuID++
	menu.Children = nil
	s.menus[menu.ID] = cloneMenu(menu)
	return menu
}

func (s *Store) insertDictionaryLocked(dictionary domain.Dictionary) domain.Dictionary {
	dictionary.ID = s.nextDictionaryID
	s.nextDictionaryID++
	dictionary.Items = append([]domain.DictionaryItem(nil), dictionary.Items...)
	s.dictionaries[dictionary.ID] = cloneDictionary(dictionary)
	return dictionary
}

func (s *Store) insertSettingLocked(setting domain.Setting) domain.Setting {
	setting.ID = s.nextSettingID
	s.nextSettingID++
	setting.Value = append([]byte(nil), setting.Value...)
	s.settings[setting.ID] = cloneSetting(setting)
	return setting
}

func (s *Store) insertAuditLocked(log domain.AuditLog) domain.AuditLog {
	log.ID = s.nextAuditLogID
	s.nextAuditLogID++
	s.auditLogs[log.ID] = log
	return log
}

func (s *Store) menuNameExistsLocked(ignoreID uint64, name string) bool {
	for _, existing := range s.menus {
		if existing.ID != ignoreID && strings.EqualFold(existing.Name, name) {
			return true
		}
	}
	return false
}

func (s *Store) menuHasAncestorLocked(candidateID, ancestorID uint64) bool {
	for candidateID != 0 {
		menu, ok := s.menus[candidateID]
		if !ok {
			return false
		}
		if menu.ParentID == ancestorID {
			return true
		}
		if menu.ParentID == candidateID {
			return false
		}
		candidateID = menu.ParentID
	}
	return false
}

func (s *Store) dictionaryCodeExistsLocked(ignoreID uint64, code string) bool {
	for _, existing := range s.dictionaries {
		if existing.ID != ignoreID && strings.EqualFold(existing.Code, code) {
			return true
		}
	}
	return false
}

func (s *Store) settingKeyExistsLocked(ignoreID uint64, key string) bool {
	for _, existing := range s.settings {
		if existing.ID != ignoreID && strings.EqualFold(existing.Key, key) {
			return true
		}
	}
	return false
}

func normalizePage(page, pageSize int) (int, int) {
	return repository.NormalizePage(page, pageSize)
}

func paginate[T any](items []T, page, pageSize int) []T {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []T{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func buildMenuTree(items []domain.Menu) []domain.Menu {
	nodes := make([]domain.Menu, len(items))
	copy(nodes, items)

	byID := make(map[uint64]*domain.Menu, len(nodes))
	for i := range nodes {
		nodes[i].Children = nil
		byID[nodes[i].ID] = &nodes[i]
	}

	roots := make([]domain.Menu, 0)
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

func cloneUser(user domain.User) domain.User {
	user.RoleIDs = append([]uint64(nil), user.RoleIDs...)
	if user.LastLoginAt != nil {
		lastLoginAt := *user.LastLoginAt
		user.LastLoginAt = &lastLoginAt
	}
	return user
}

func cloneRole(role domain.Role) domain.Role {
	role.Permissions = append([]string(nil), role.Permissions...)
	return role
}

func cloneMenu(menu domain.Menu) domain.Menu {
	menu.Children = append([]domain.Menu(nil), menu.Children...)
	for i := range menu.Children {
		menu.Children[i] = cloneMenu(menu.Children[i])
	}
	return menu
}

func cloneDictionary(dictionary domain.Dictionary) domain.Dictionary {
	dictionary.Items = append([]domain.DictionaryItem(nil), dictionary.Items...)
	return dictionary
}

func cloneSetting(setting domain.Setting) domain.Setting {
	setting.Value = append([]byte(nil), setting.Value...)
	return setting
}
