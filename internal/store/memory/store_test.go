package memory

import (
	"sort"
	"strings"
	"testing"

	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/module"
	"github.com/expary/GOV2/internal/repository"
)

func TestStoreSeedAndListUsers(t *testing.T) {
	store := NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	users, total, err := store.ListUsers(repository.UserQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if total < 2 {
		t.Fatalf("expected seeded users, got total=%d", total)
	}
	if len(users) == 0 || users[0].PasswordHash == "" {
		t.Fatal("expected internal users to include password hash")
	}
}

func TestNormalizePageUsesRepositoryPaginationContract(t *testing.T) {
	page, pageSize := normalizePage(0, repository.MaxPageSize+1)
	if page != 1 || pageSize != repository.MaxPageSize {
		t.Fatalf("normalizePage() = (%d, %d), want (1, %d)", page, pageSize, repository.MaxPageSize)
	}
}

func TestStoreCreateUserConflict(t *testing.T) {
	store := NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	_, err := store.CreateUser(domain.User{
		Username: "admin",
		Status:   domain.UserStatusActive,
	})
	if err != ErrConflict {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestStoreUserUniqueEmailAndPhoneConflicts(t *testing.T) {
	store := NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	first, err := store.CreateUser(domain.User{
		Username: "unique-contact-one",
		Email:    "one@example.test",
		Phone:    "15500000001",
		Status:   domain.UserStatusActive,
	})
	if err != nil {
		t.Fatalf("CreateUser(first) error = %v", err)
	}
	second, err := store.CreateUser(domain.User{
		Username: "unique-contact-two",
		Email:    "two@example.test",
		Phone:    "15500000002",
		Status:   domain.UserStatusActive,
	})
	if err != nil {
		t.Fatalf("CreateUser(second) error = %v", err)
	}

	if _, err := store.CreateUser(domain.User{
		Username: "duplicate-email",
		Email:    "ONE@EXAMPLE.TEST",
		Status:   domain.UserStatusActive,
	}); err != ErrConflict {
		t.Fatalf("expected duplicate email conflict, got %v", err)
	}
	if _, err := store.CreateUser(domain.User{
		Username: "duplicate-phone",
		Phone:    "15500000001",
		Status:   domain.UserStatusActive,
	}); err != ErrConflict {
		t.Fatalf("expected duplicate phone conflict, got %v", err)
	}
	if _, err := store.UpdateUser(second.ID, domain.User{
		Username: second.Username,
		Email:    "ONE@EXAMPLE.TEST",
		Phone:    second.Phone,
		Status:   domain.UserStatusActive,
	}); err != ErrConflict {
		t.Fatalf("expected update email conflict, got %v", err)
	}
	if _, err := store.UpdateUser(second.ID, domain.User{
		Username: second.Username,
		Email:    second.Email,
		Phone:    "15500000001",
		Status:   domain.UserStatusActive,
	}); err != ErrConflict {
		t.Fatalf("expected update phone conflict, got %v", err)
	}
	if _, err := store.UpdateUser(first.ID, domain.User{
		Username: first.Username,
		Email:    "ONE@EXAMPLE.TEST",
		Phone:    "15500000001",
		Status:   domain.UserStatusActive,
	}); err != nil {
		t.Fatalf("expected same user contact update to pass, got %v", err)
	}
}

func TestStoreMenuCRUD(t *testing.T) {
	store := NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	parent, err := store.CreateMenu(domain.Menu{
		Title:     "Reports",
		Name:      "reports",
		Path:      "/reports",
		Component: "Layout",
		Sort:      90,
	})
	if err != nil {
		t.Fatalf("CreateMenu(parent) error = %v", err)
	}

	child, err := store.CreateMenu(domain.Menu{
		ParentID:   parent.ID,
		Title:      "Sales Reports",
		Name:       "reports-sales",
		Path:       "/reports/sales",
		Component:  "SalesReportsView",
		Permission: "reports:sales:list",
		Sort:       91,
	})
	if err != nil {
		t.Fatalf("CreateMenu(child) error = %v", err)
	}
	if child.ParentID != parent.ID {
		t.Fatalf("expected child parent_id=%d, got %d", parent.ID, child.ParentID)
	}

	updated, err := store.UpdateMenu(child.ID, domain.Menu{
		ParentID:   parent.ID,
		Title:      "Revenue Reports",
		Name:       "reports-revenue",
		Path:       "/reports/revenue",
		Component:  "RevenueReportsView",
		Permission: "reports:revenue:list",
		Sort:       92,
		Hidden:     true,
	})
	if err != nil {
		t.Fatalf("UpdateMenu(child) error = %v", err)
	}
	if updated.Title != "Revenue Reports" || !updated.Hidden {
		t.Fatalf("unexpected updated menu: %+v", updated)
	}

	if err := store.DeleteMenu(parent.ID); err != ErrConflict {
		t.Fatalf("expected parent delete conflict, got %v", err)
	}
	if _, err := store.UpdateMenu(parent.ID, domain.Menu{
		ParentID:  child.ID,
		Title:     parent.Title,
		Name:      parent.Name,
		Path:      parent.Path,
		Component: parent.Component,
		Sort:      parent.Sort,
	}); err != ErrConflict {
		t.Fatalf("expected cycle conflict, got %v", err)
	}
	if err := store.DeleteMenu(child.ID); err != nil {
		t.Fatalf("DeleteMenu(child) error = %v", err)
	}
	if err := store.DeleteMenu(parent.ID); err != nil {
		t.Fatalf("DeleteMenu(parent) error = %v", err)
	}
}

func TestStoreSeedMenuComponentsMatchFrontendViews(t *testing.T) {
	store := NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	want := map[string]string{
		"/dashboard":           "DashboardView",
		"/system/users":        "UsersView",
		"/system/roles":        "RolesView",
		"/system/menus":        "MenusView",
		"/system/modules":      "ModulesView",
		"/system/dictionaries": "DictionariesView",
		"/system/settings":     "SettingsView",
		"/system/audit":        "AuditLogsView",
	}
	menus, err := store.ListMenus()
	if err != nil {
		t.Fatalf("ListMenus() error = %v", err)
	}
	for path, component := range want {
		menu, ok := findMenuByPath(menus, path)
		if !ok {
			t.Fatalf("expected seeded menu path %s", path)
		}
		if menu.Component != component {
			t.Fatalf("seeded menu %s component = %q, want %q", path, menu.Component, component)
		}
	}
}

func TestStoreSeedRolesMatchBuiltInDefaults(t *testing.T) {
	store := NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	roles, err := store.ListRoles()
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	got := map[string][]string{}
	for _, role := range roles {
		got[strings.ToLower(role.Code)] = sortedStrings(role.Permissions)
	}
	want := map[string][]string{
		"admin":    {domain.PermissionAll},
		"operator": sortedStrings(operatorDefaultPermissions()),
	}

	assertStringKeys(t, "memory seed roles", stringKeys(got), stringKeys(want))
	for code, wantPermissions := range want {
		if strings.Join(got[code], "\n") != strings.Join(wantPermissions, "\n") {
			t.Fatalf("memory seed role %q permissions = %+v, want %+v", code, got[code], wantPermissions)
		}
	}
}

func TestStoreSeedMenusMatchBuiltInModuleRegistry(t *testing.T) {
	store := NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	menus, err := store.ListMenus()
	if err != nil {
		t.Fatalf("ListMenus() error = %v", err)
	}
	got := memoryMenusByName(menus, "")
	want := map[string]memorySeedMenu{}
	registry := module.NewRegistry(module.BuiltInModules()...)
	for _, menu := range registry.Menus() {
		want[menu.Name] = memorySeedMenu{
			Name:       menu.Name,
			Parent:     menu.Parent,
			Title:      menu.Title,
			Path:       menu.Path,
			Icon:       menu.Icon,
			Component:  menu.Component,
			Permission: menu.Permission,
			Sort:       menu.Sort,
			Hidden:     menu.Hidden,
		}
	}

	assertStringKeys(t, "memory seed menus", stringKeys(got), stringKeys(want))
	for name, wantMenu := range want {
		if gotMenu := got[name]; gotMenu != wantMenu {
			t.Fatalf("memory seed menu %q = %+v, want %+v", name, gotMenu, wantMenu)
		}
	}
}

func TestBuildMenuTreeSortsBySortThenID(t *testing.T) {
	menus := buildMenuTree([]domain.Menu{
		{ID: 20, Title: "Root 20", Sort: 10},
		{ID: 10, Title: "Root 10", Sort: 10},
		{ID: 202, ParentID: 20, Title: "Child 202", Sort: 5},
		{ID: 201, ParentID: 20, Title: "Child 201", Sort: 5},
	})

	if len(menus) != 2 {
		t.Fatalf("expected two root menus, got %+v", menus)
	}
	if menus[0].ID != 10 || menus[1].ID != 20 {
		t.Fatalf("expected roots to sort by sort then id, got ids %d, %d", menus[0].ID, menus[1].ID)
	}
	if len(menus[1].Children) != 2 {
		t.Fatalf("expected two children for root 20, got %+v", menus[1].Children)
	}
	if menus[1].Children[0].ID != 201 || menus[1].Children[1].ID != 202 {
		t.Fatalf("expected children to sort by sort then id, got ids %d, %d", menus[1].Children[0].ID, menus[1].Children[1].ID)
	}
}

func TestStoreDeleteAssignedRoleConflict(t *testing.T) {
	store := NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	admin, err := store.FindUserByUsername("admin")
	if err != nil {
		t.Fatalf("FindUserByUsername(admin) error = %v", err)
	}
	if len(admin.RoleIDs) == 0 {
		t.Fatal("expected seeded admin role")
	}

	if err := store.DeleteRole(admin.RoleIDs[0]); err != ErrConflict {
		t.Fatalf("expected assigned role conflict, got %v", err)
	}
}

func findMenuByPath(menus []domain.Menu, path string) (domain.Menu, bool) {
	for _, menu := range menus {
		if menu.Path == path {
			return menu, true
		}
		if child, ok := findMenuByPath(menu.Children, path); ok {
			return child, true
		}
	}
	return domain.Menu{}, false
}

func operatorDefaultPermissions() []string {
	return []string{
		domain.PermissionDashboardView,
		domain.PermissionSystemUserList,
		domain.PermissionSystemRoleList,
		domain.PermissionSystemMenuList,
		domain.PermissionSystemModuleList,
		domain.PermissionSystemDictionaryList,
		domain.PermissionSystemSettingList,
		domain.PermissionSystemAuditList,
	}
}

type memorySeedMenu struct {
	Name       string
	Parent     string
	Title      string
	Path       string
	Icon       string
	Component  string
	Permission string
	Sort       int
	Hidden     bool
}

func memoryMenusByName(menus []domain.Menu, parent string) map[string]memorySeedMenu {
	items := map[string]memorySeedMenu{}
	for _, menu := range menus {
		items[menu.Name] = memorySeedMenu{
			Name:       menu.Name,
			Parent:     parent,
			Title:      menu.Title,
			Path:       menu.Path,
			Icon:       menu.Icon,
			Component:  menu.Component,
			Permission: menu.Permission,
			Sort:       menu.Sort,
			Hidden:     menu.Hidden,
		}
		for name, child := range memoryMenusByName(menu.Children, menu.Name) {
			items[name] = child
		}
	}
	return items
}

func assertStringKeys(t *testing.T, label string, got []string, want []string) {
	t.Helper()

	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s mismatch:\ngot:\n%s\nwant:\n%s", label, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func sortedStrings(values []string) []string {
	items := append([]string(nil), values...)
	sort.Strings(items)
	return items
}

func stringKeys[V any](items map[string]V) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	return keys
}

func TestStoreDictionaryCRUD(t *testing.T) {
	store := NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	created, err := store.CreateDictionary(domain.Dictionary{
		Code: "ticket_priority",
		Name: "Ticket Priority",
		Items: []domain.DictionaryItem{
			{Label: "High", Value: "high", Sort: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateDictionary() error = %v", err)
	}

	updated, err := store.UpdateDictionary(created.ID, domain.Dictionary{
		Code: "ticket_priority",
		Name: "Ticket Priority Updated",
		Items: []domain.DictionaryItem{
			{Label: "Low", Value: "low", Sort: 2},
		},
	})
	if err != nil {
		t.Fatalf("UpdateDictionary() error = %v", err)
	}
	if updated.Name != "Ticket Priority Updated" || len(updated.Items) != 1 || updated.Items[0].Value != "low" {
		t.Fatalf("unexpected updated dictionary: %+v", updated)
	}

	if err := store.DeleteDictionary(created.ID); err != nil {
		t.Fatalf("DeleteDictionary() error = %v", err)
	}
	if err := store.DeleteDictionary(created.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStoreSettingCRUD(t *testing.T) {
	store := NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	created, err := store.CreateSetting(domain.Setting{
		Key:         "feature.enabled",
		Value:       []byte(`true`),
		Description: "Feature flag",
	})
	if err != nil {
		t.Fatalf("CreateSetting() error = %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected created setting id, got %+v", created)
	}
	if _, err := store.CreateSetting(domain.Setting{Key: "FEATURE.ENABLED", Value: []byte(`false`)}); err != ErrConflict {
		t.Fatalf("expected case-insensitive setting key conflict, got %v", err)
	}

	updated, err := store.UpdateSetting(created.ID, domain.Setting{
		Key:         "feature.enabled",
		Value:       []byte(`false`),
		Description: "Updated feature flag",
	})
	if err != nil {
		t.Fatalf("UpdateSetting() error = %v", err)
	}
	if string(updated.Value) != "false" || updated.Description != "Updated feature flag" {
		t.Fatalf("unexpected updated setting: %+v", updated)
	}

	if err := store.DeleteSetting(created.ID); err != nil {
		t.Fatalf("DeleteSetting() error = %v", err)
	}
	if err := store.DeleteSetting(created.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStoreListAuditLogsFilters(t *testing.T) {
	store := NewStore()
	if _, err := store.AddAuditLog(domain.AuditLog{
		Actor:      "admin",
		Action:     "create",
		Resource:   "system.user",
		ResourceID: "42",
		IP:         "127.0.0.1",
		UserAgent:  "test-agent",
		Detail:     "created user 42",
	}); err != nil {
		t.Fatalf("AddAuditLog(create) error = %v", err)
	}
	if _, err := store.AddAuditLog(domain.AuditLog{
		Actor:      "operator",
		Action:     "delete",
		Resource:   "system.setting",
		ResourceID: "7",
		IP:         "10.0.0.8",
		UserAgent:  "test-agent",
		Detail:     "deleted setting 7",
	}); err != nil {
		t.Fatalf("AddAuditLog(delete) error = %v", err)
	}

	cases := []struct {
		name  string
		query repository.AuditLogQuery
		want  string
	}{
		{name: "actor", query: repository.AuditLogQuery{Actor: "adm", Page: 1, PageSize: 10}, want: "system.user"},
		{name: "action", query: repository.AuditLogQuery{Action: "delete", Page: 1, PageSize: 10}, want: "system.setting"},
		{name: "resource", query: repository.AuditLogQuery{Resource: "user", Page: 1, PageSize: 10}, want: "system.user"},
		{name: "resource id", query: repository.AuditLogQuery{ResourceID: "42", Page: 1, PageSize: 10}, want: "system.user"},
		{name: "keyword", query: repository.AuditLogQuery{Keyword: "10.0.0.8", Page: 1, PageSize: 10}, want: "system.setting"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs, total, err := store.ListAuditLogs(tc.query)
			if err != nil {
				t.Fatalf("ListAuditLogs() error = %v", err)
			}
			if total != 1 || len(logs) != 1 {
				t.Fatalf("expected one audit log, got total=%d logs=%+v", total, logs)
			}
			if logs[0].Resource != tc.want {
				t.Fatalf("expected resource %q, got %+v", tc.want, logs[0])
			}
		})
	}
}
