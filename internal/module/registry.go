package module

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/expary/GOV2/internal/domain"
)

type Module struct {
	Name        string                        `json:"name"`
	Title       string                        `json:"title"`
	Version     string                        `json:"version"`
	Description string                        `json:"description"`
	Permissions []domain.PermissionDefinition `json:"permissions"`
	Backend     []BackendRoute                `json:"backend_routes"`
	Menus       []MenuEntry                   `json:"menus"`
	Routes      []FrontendRoute               `json:"frontend_routes"`
	Migrations  []MigrationSet                `json:"migrations"`
}

type BackendRoute struct {
	Name       string `json:"name"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Public     bool   `json:"public"`
	Permission string `json:"permission"`
	Summary    string `json:"summary"`
}

type MenuEntry struct {
	Name       string `json:"name"`
	Parent     string `json:"parent"`
	Title      string `json:"title"`
	Path       string `json:"path"`
	Icon       string `json:"icon"`
	Component  string `json:"component"`
	Permission string `json:"permission"`
	Sort       int    `json:"sort"`
	Hidden     bool   `json:"hidden"`
}

type FrontendRoute struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Component  string `json:"component"`
	Title      string `json:"title"`
	Permission string `json:"permission"`
}

type MigrationSet struct {
	Driver   string `json:"driver"`
	Dir      string `json:"dir"`
	SeedsDir string `json:"seeds_dir"`
}

type Registry struct {
	modules map[string]Module
}

var supportedBackendMethods = map[string]bool{
	"DELETE":  true,
	"GET":     true,
	"HEAD":    true,
	"OPTIONS": true,
	"PATCH":   true,
	"POST":    true,
	"PUT":     true,
}

const ModuleNamePattern = `^[a-z][a-z0-9_]*$`

var moduleNamePattern = regexp.MustCompile(ModuleNamePattern)

func IsValidName(name string) bool {
	return moduleNamePattern.MatchString(name)
}

func NewRegistry(modules ...Module) *Registry {
	registry := &Registry{modules: map[string]Module{}}
	for _, mod := range modules {
		registry.Register(mod)
	}
	return registry
}

func NewValidatedRegistry(modules ...Module) (*Registry, error) {
	if err := ValidateModules(modules...); err != nil {
		return nil, err
	}
	return NewRegistry(modules...), nil
}

func (r *Registry) Register(mod Module) {
	if mod.Name == "" {
		return
	}
	r.modules[mod.Name] = cloneModule(mod)
}

func (r *Registry) List() []Module {
	items := make([]Module, 0, len(r.modules))
	for _, mod := range r.modules {
		items = append(items, cloneModule(mod))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

func (r *Registry) Permissions() []domain.PermissionDefinition {
	items := make([]domain.PermissionDefinition, 0)
	for _, mod := range r.List() {
		items = append(items, mod.Permissions...)
	}
	return items
}

func (r *Registry) BackendRoutes() []BackendRoute {
	items := make([]BackendRoute, 0)
	for _, mod := range r.List() {
		items = append(items, mod.Backend...)
	}
	return items
}

func (r *Registry) Menus() []MenuEntry {
	items := make([]MenuEntry, 0)
	for _, mod := range r.List() {
		items = append(items, mod.Menus...)
	}
	return items
}

func (r *Registry) FrontendRoutes() []FrontendRoute {
	items := make([]FrontendRoute, 0)
	for _, mod := range r.List() {
		items = append(items, mod.Routes...)
	}
	return items
}

func (r *Registry) Migrations() []MigrationSet {
	items := make([]MigrationSet, 0)
	for _, mod := range r.List() {
		items = append(items, mod.Migrations...)
	}
	return items
}

func ValidateModules(modules ...Module) error {
	moduleNames := map[string]string{}
	permissionCodes := map[string]string{}
	menuNames := map[string]string{}

	for _, mod := range modules {
		moduleName := strings.TrimSpace(mod.Name)
		if moduleName == "" {
			return fmt.Errorf("module name is required")
		}
		if !IsValidName(mod.Name) {
			return fmt.Errorf("module name %q must match %s", mod.Name, ModuleNamePattern)
		}
		if err := requireUnique(moduleNames, mod.Name, mod.Name, "module name"); err != nil {
			return err
		}
		for _, permission := range mod.Permissions {
			permissionCode := strings.TrimSpace(permission.Code)
			permissionModule := strings.TrimSpace(permission.Module)
			if permissionCode == "" {
				return fmt.Errorf("module %s permission code is required", mod.Name)
			}
			if permissionModule == "" {
				return fmt.Errorf("module %s permission %s must declare a module", mod.Name, permission.Code)
			}
			if permissionModule != moduleName {
				return fmt.Errorf("module %s permission %s module = %q, want %q", mod.Name, permission.Code, permission.Module, moduleName)
			}
			if !strings.HasPrefix(permissionCode, moduleName+":") {
				return fmt.Errorf("module %s permission %s must use module namespace %q", mod.Name, permission.Code, moduleName+":")
			}
			if err := requireUnique(permissionCodes, permissionCode, mod.Name, "permission code"); err != nil {
				return err
			}
		}
		for _, menu := range mod.Menus {
			if strings.TrimSpace(menu.Name) == "" {
				return fmt.Errorf("module %s menu name is required", mod.Name)
			}
			if err := requireUnique(menuNames, menu.Name, mod.Name, "menu name"); err != nil {
				return err
			}
		}
	}

	backendNames := map[string]string{}
	backendPatterns := map[string]string{}
	menuPaths := map[string]string{}
	frontendNames := map[string]string{}
	frontendPaths := map[string]string{}
	frontendByPath := map[string]FrontendRoute{}
	migrationKeys := map[string]string{}
	for _, mod := range modules {
		moduleName := strings.TrimSpace(mod.Name)
		backendPathNamespace := "/api/v1/" + moduleName
		frontendPathNamespace := "/" + moduleName
		for _, route := range mod.Backend {
			method := strings.TrimSpace(route.Method)
			path := strings.TrimSpace(route.Path)
			if strings.TrimSpace(route.Name) == "" {
				return fmt.Errorf("module %s backend route name is required", mod.Name)
			}
			if method == "" || path == "" {
				return fmt.Errorf("module %s backend route %s must declare method and path", mod.Name, route.Name)
			}
			if !supportedBackendMethods[method] {
				return fmt.Errorf("module %s backend route %s must use a supported uppercase HTTP method", mod.Name, route.Name)
			}
			if path != "/api/v1" && !strings.HasPrefix(path, "/api/v1/") {
				return fmt.Errorf("module %s backend route %s must use an /api/v1 path", mod.Name, route.Name)
			}
			if !hasPathNamespace(path, backendPathNamespace) {
				return fmt.Errorf("module %s backend route %s must use module API namespace %q", mod.Name, route.Name, backendPathNamespace)
			}
			if strings.TrimSpace(route.Summary) == "" {
				return fmt.Errorf("module %s backend route %s must declare a summary", mod.Name, route.Name)
			}
			if err := requireUnique(backendNames, route.Name, mod.Name, "backend route name"); err != nil {
				return err
			}
			if err := requireUnique(backendPatterns, method+" "+path, mod.Name, "backend route pattern"); err != nil {
				return err
			}
			if route.Public && strings.TrimSpace(route.Permission) != "" {
				return fmt.Errorf("module %s backend route %s is public and must not declare a permission", mod.Name, route.Name)
			}
			if err := requireOwnedPermission(permissionCodes, route.Permission, moduleName, "module "+mod.Name+" backend route "+route.Name); err != nil {
				return err
			}
		}
		for _, menu := range mod.Menus {
			menuPath := strings.TrimSpace(menu.Path)
			if menuPath == "" {
				return fmt.Errorf("module %s menu %s must declare a path", mod.Name, menu.Name)
			}
			if !strings.HasPrefix(menuPath, "/") {
				return fmt.Errorf("module %s menu %s must use an absolute path", mod.Name, menu.Name)
			}
			if !hasPathNamespace(menuPath, frontendPathNamespace) {
				return fmt.Errorf("module %s menu %s must use module path namespace %q", mod.Name, menu.Name, frontendPathNamespace)
			}
			if err := requireUnique(menuPaths, menuPath, mod.Name, "menu path"); err != nil {
				return err
			}
			parentName := strings.TrimSpace(menu.Parent)
			if parentName != "" {
				parentModule, ok := menuNames[parentName]
				if !ok {
					return fmt.Errorf("module %s menu %s references unknown parent menu %q", mod.Name, menu.Name, parentName)
				}
				if parentModule != moduleName {
					return fmt.Errorf("module %s menu %s references parent menu %q owned by module %s", mod.Name, menu.Name, parentName, parentModule)
				}
			}
			if err := requireOwnedPermission(permissionCodes, menu.Permission, moduleName, "module "+mod.Name+" menu "+menu.Name); err != nil {
				return err
			}
		}
		for _, route := range mod.Routes {
			routePath := strings.TrimSpace(route.Path)
			if strings.TrimSpace(route.Name) == "" {
				return fmt.Errorf("module %s frontend route name is required", mod.Name)
			}
			if routePath == "" || strings.TrimSpace(route.Component) == "" {
				return fmt.Errorf("module %s frontend route %s must declare path and component", mod.Name, route.Name)
			}
			if !strings.HasPrefix(routePath, "/") {
				return fmt.Errorf("module %s frontend route %s must use an absolute path", mod.Name, route.Name)
			}
			if !hasPathNamespace(routePath, frontendPathNamespace) {
				return fmt.Errorf("module %s frontend route %s must use module path namespace %q", mod.Name, route.Name, frontendPathNamespace)
			}
			if err := requireUnique(frontendNames, route.Name, mod.Name, "frontend route name"); err != nil {
				return err
			}
			if err := requireUnique(frontendPaths, routePath, mod.Name, "frontend route path"); err != nil {
				return err
			}
			frontendByPath[routePath] = route
			if err := requireOwnedPermission(permissionCodes, route.Permission, moduleName, "module "+mod.Name+" frontend route "+route.Name); err != nil {
				return err
			}
		}
		for _, migration := range mod.Migrations {
			if strings.TrimSpace(migration.Driver) == "" || strings.TrimSpace(migration.Dir) == "" {
				return fmt.Errorf("module %s migration set must declare driver and dir", mod.Name)
			}
			if err := requireUnique(migrationKeys, migration.Driver+" "+migration.Dir, mod.Name, "migration set"); err != nil {
				return err
			}
		}
	}
	for _, mod := range modules {
		for _, menu := range mod.Menus {
			route, ok := frontendByPath[strings.TrimSpace(menu.Path)]
			if !ok {
				continue
			}
			if strings.TrimSpace(menu.Component) != strings.TrimSpace(route.Component) {
				return fmt.Errorf("module %s menu %s component %q does not match frontend route %s component %q for path %q", mod.Name, menu.Name, menu.Component, route.Name, route.Component, strings.TrimSpace(menu.Path))
			}
		}
	}
	return nil
}

func hasPathNamespace(path, namespace string) bool {
	return path == namespace || strings.HasPrefix(path, namespace+"/")
}

func BuiltInModules() []Module {
	permissions := domain.SystemPermissions()
	dashboardPermissions := make([]domain.PermissionDefinition, 0)
	systemPermissions := make([]domain.PermissionDefinition, 0)
	for _, permission := range permissions {
		if permission.Module == "dashboard" {
			dashboardPermissions = append(dashboardPermissions, permission)
			continue
		}
		systemPermissions = append(systemPermissions, permission)
	}
	return []Module{
		{
			Name:        "dashboard",
			Title:       "Dashboard",
			Version:     "0.1.0",
			Description: "Operational summary and system overview",
			Permissions: dashboardPermissions,
			Backend: []BackendRoute{
				{Name: "dashboard-summary", Method: "GET", Path: "/api/v1/dashboard/summary", Permission: domain.PermissionDashboardView, Summary: "Dashboard summary"},
			},
			Menus: []MenuEntry{
				{Name: "dashboard", Title: "Dashboard", Path: "/dashboard", Icon: "layout-dashboard", Component: "DashboardView", Permission: domain.PermissionDashboardView, Sort: 10},
			},
			Routes: []FrontendRoute{
				{Name: "dashboard", Path: "/dashboard", Component: "DashboardView", Title: "Dashboard", Permission: domain.PermissionDashboardView},
			},
		},
		{
			Name:        "system",
			Title:       "System Management",
			Version:     "0.1.0",
			Description: "Users, roles, menus, dictionaries, settings, modules, and audit logs",
			Permissions: systemPermissions,
			Backend: []BackendRoute{
				{Name: "system-users-list", Method: "GET", Path: "/api/v1/system/users", Permission: domain.PermissionSystemUserList, Summary: "List users"},
				{Name: "system-users-create", Method: "POST", Path: "/api/v1/system/users", Permission: domain.PermissionSystemUserCreate, Summary: "Create user"},
				{Name: "system-users-read", Method: "GET", Path: "/api/v1/system/users/{id}", Permission: domain.PermissionSystemUserList, Summary: "Get user"},
				{Name: "system-users-update", Method: "PUT", Path: "/api/v1/system/users/{id}", Permission: domain.PermissionSystemUserUpdate, Summary: "Update user"},
				{Name: "system-users-status", Method: "PATCH", Path: "/api/v1/system/users/{id}/status", Permission: domain.PermissionSystemUserUpdate, Summary: "Update user status"},
				{Name: "system-users-delete", Method: "DELETE", Path: "/api/v1/system/users/{id}", Permission: domain.PermissionSystemUserDelete, Summary: "Delete user"},
				{Name: "system-roles-list", Method: "GET", Path: "/api/v1/system/roles", Permission: domain.PermissionSystemRoleList, Summary: "List roles"},
				{Name: "system-roles-create", Method: "POST", Path: "/api/v1/system/roles", Permission: domain.PermissionSystemRoleCreate, Summary: "Create role"},
				{Name: "system-roles-update", Method: "PUT", Path: "/api/v1/system/roles/{id}", Permission: domain.PermissionSystemRoleUpdate, Summary: "Update role"},
				{Name: "system-roles-delete", Method: "DELETE", Path: "/api/v1/system/roles/{id}", Permission: domain.PermissionSystemRoleDelete, Summary: "Delete role"},
				{Name: "system-permissions-list", Method: "GET", Path: "/api/v1/system/permissions", Permission: domain.PermissionSystemRoleList, Summary: "List system permissions"},
				{Name: "system-menus-list", Method: "GET", Path: "/api/v1/system/menus", Permission: domain.PermissionSystemMenuList, Summary: "List menus"},
				{Name: "system-menus-create", Method: "POST", Path: "/api/v1/system/menus", Permission: domain.PermissionSystemMenuCreate, Summary: "Create menu"},
				{Name: "system-menus-update", Method: "PUT", Path: "/api/v1/system/menus/{id}", Permission: domain.PermissionSystemMenuUpdate, Summary: "Update menu"},
				{Name: "system-menus-delete", Method: "DELETE", Path: "/api/v1/system/menus/{id}", Permission: domain.PermissionSystemMenuDelete, Summary: "Delete menu"},
				{Name: "system-modules-list", Method: "GET", Path: "/api/v1/system/modules", Permission: domain.PermissionSystemModuleList, Summary: "List registered modules"},
				{Name: "system-dictionaries-list", Method: "GET", Path: "/api/v1/system/dictionaries", Permission: domain.PermissionSystemDictionaryList, Summary: "List dictionaries"},
				{Name: "system-dictionaries-code", Method: "GET", Path: "/api/v1/system/dictionaries/code/{code}", Summary: "Get dictionary by code"},
				{Name: "system-dictionaries-create", Method: "POST", Path: "/api/v1/system/dictionaries", Permission: domain.PermissionSystemDictionaryCreate, Summary: "Create dictionary"},
				{Name: "system-dictionaries-update", Method: "PUT", Path: "/api/v1/system/dictionaries/{id}", Permission: domain.PermissionSystemDictionaryUpdate, Summary: "Update dictionary"},
				{Name: "system-dictionaries-delete", Method: "DELETE", Path: "/api/v1/system/dictionaries/{id}", Permission: domain.PermissionSystemDictionaryDelete, Summary: "Delete dictionary"},
				{Name: "system-settings-list", Method: "GET", Path: "/api/v1/system/settings", Permission: domain.PermissionSystemSettingList, Summary: "List settings"},
				{Name: "system-settings-create", Method: "POST", Path: "/api/v1/system/settings", Permission: domain.PermissionSystemSettingCreate, Summary: "Create setting"},
				{Name: "system-settings-update", Method: "PUT", Path: "/api/v1/system/settings/{id}", Permission: domain.PermissionSystemSettingUpdate, Summary: "Update setting"},
				{Name: "system-settings-delete", Method: "DELETE", Path: "/api/v1/system/settings/{id}", Permission: domain.PermissionSystemSettingDelete, Summary: "Delete setting"},
				{Name: "system-audit-list", Method: "GET", Path: "/api/v1/system/audit-logs", Permission: domain.PermissionSystemAuditList, Summary: "List audit logs"},
			},
			Menus: []MenuEntry{
				{Name: "system", Title: "System", Path: "/system", Icon: "settings", Component: "Layout", Sort: 20},
				{Name: "system-users", Parent: "system", Title: "Users", Path: "/system/users", Icon: "users", Component: "UsersView", Permission: domain.PermissionSystemUserList, Sort: 21},
				{Name: "system-roles", Parent: "system", Title: "Roles", Path: "/system/roles", Icon: "shield", Component: "RolesView", Permission: domain.PermissionSystemRoleList, Sort: 22},
				{Name: "system-menus", Parent: "system", Title: "Menus", Path: "/system/menus", Icon: "menu", Component: "MenusView", Permission: domain.PermissionSystemMenuList, Sort: 23},
				{Name: "system-modules", Parent: "system", Title: "Modules", Path: "/system/modules", Icon: "boxes", Component: "ModulesView", Permission: domain.PermissionSystemModuleList, Sort: 24},
				{Name: "system-dictionaries", Parent: "system", Title: "Dictionaries", Path: "/system/dictionaries", Icon: "book", Component: "DictionariesView", Permission: domain.PermissionSystemDictionaryList, Sort: 25},
				{Name: "system-settings", Parent: "system", Title: "Settings", Path: "/system/settings", Icon: "sliders-horizontal", Component: "SettingsView", Permission: domain.PermissionSystemSettingList, Sort: 26},
				{Name: "system-audit", Parent: "system", Title: "Audit Logs", Path: "/system/audit", Icon: "history", Component: "AuditLogsView", Permission: domain.PermissionSystemAuditList, Sort: 27},
			},
			Routes: []FrontendRoute{
				{Name: "system-users", Path: "/system/users", Component: "UsersView", Title: "Users", Permission: domain.PermissionSystemUserList},
				{Name: "system-roles", Path: "/system/roles", Component: "RolesView", Title: "Roles", Permission: domain.PermissionSystemRoleList},
				{Name: "system-menus", Path: "/system/menus", Component: "MenusView", Title: "Menus", Permission: domain.PermissionSystemMenuList},
				{Name: "system-modules", Path: "/system/modules", Component: "ModulesView", Title: "Modules", Permission: domain.PermissionSystemModuleList},
				{Name: "system-dictionaries", Path: "/system/dictionaries", Component: "DictionariesView", Title: "Dictionaries", Permission: domain.PermissionSystemDictionaryList},
				{Name: "system-settings", Path: "/system/settings", Component: "SettingsView", Title: "Settings", Permission: domain.PermissionSystemSettingList},
				{Name: "system-audit", Path: "/system/audit", Component: "AuditLogsView", Title: "Audit Logs", Permission: domain.PermissionSystemAuditList},
			},
			Migrations: []MigrationSet{
				{Driver: "postgres", Dir: "migrations", SeedsDir: "migrations/seeds"},
			},
		},
	}
}

func cloneModule(mod Module) Module {
	mod.Permissions = append([]domain.PermissionDefinition{}, mod.Permissions...)
	mod.Backend = append([]BackendRoute{}, mod.Backend...)
	mod.Menus = append([]MenuEntry{}, mod.Menus...)
	mod.Routes = append([]FrontendRoute{}, mod.Routes...)
	mod.Migrations = append([]MigrationSet{}, mod.Migrations...)
	return mod
}

func requireUnique(seen map[string]string, key, owner, kind string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	if existingOwner, ok := seen[key]; ok {
		return fmt.Errorf("duplicate %s %q in module %s; already used by module %s", kind, key, owner, existingOwner)
	}
	seen[key] = owner
	return nil
}

func requireOwnedPermission(known map[string]string, permission, expectedModule, owner string) error {
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return nil
	}
	permissionModule, ok := known[permission]
	if !ok {
		return fmt.Errorf("%s references unknown permission %q", owner, permission)
	}
	if permissionModule != expectedModule {
		return fmt.Errorf("%s references permission %q owned by module %s", owner, permission, permissionModule)
	}
	return nil
}
