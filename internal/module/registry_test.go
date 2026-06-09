package module

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/expary/GOV2/internal/domain"
)

func TestRegistryListsModulesAndCopiesMetadata(t *testing.T) {
	registry := NewRegistry(
		Module{
			Name:        "billing",
			Title:       "Billing",
			Version:     "0.1.0",
			Description: "Billing module",
			Permissions: []domain.PermissionDefinition{
				{Code: "billing:invoice:list", Name: "List invoices", Module: "billing"},
			},
			Backend: []BackendRoute{
				{Name: "billing-invoices-list", Method: "GET", Path: "/api/v1/billing/invoices", Permission: "billing:invoice:list", Summary: "List billing invoices"},
			},
			Menus: []MenuEntry{
				{Name: "billing", Title: "Billing", Path: "/billing", Permission: "billing:invoice:list"},
			},
			Routes: []FrontendRoute{
				{Name: "billing", Path: "/billing", Component: "BillingView", Permission: "billing:invoice:list"},
			},
			Migrations: []MigrationSet{
				{Driver: "postgres", Dir: "modules/billing/migrations"},
			},
		},
		Module{Name: "audit", Title: "Audit", Version: "0.1.0"},
	)

	list := registry.List()
	if len(list) != 2 || list[0].Name != "audit" || list[1].Name != "billing" {
		t.Fatalf("expected sorted module list, got %+v", list)
	}

	list[1].Permissions[0].Code = "mutated"
	list[1].Backend[0].Name = "mutated"
	list[1].Menus[0].Name = "mutated"
	list[1].Routes[0].Name = "mutated"
	list[1].Migrations[0].Dir = "mutated"

	again := registry.List()
	billing := again[1]
	if billing.Permissions[0].Code != "billing:invoice:list" {
		t.Fatalf("expected permissions copy, got %+v", billing.Permissions)
	}
	if billing.Backend[0].Name != "billing-invoices-list" {
		t.Fatalf("expected backend route copy, got %+v", billing.Backend)
	}
	if billing.Menus[0].Name != "billing" {
		t.Fatalf("expected menu copy, got %+v", billing.Menus)
	}
	if billing.Routes[0].Name != "billing" {
		t.Fatalf("expected route copy, got %+v", billing.Routes)
	}
	if billing.Migrations[0].Dir != "modules/billing/migrations" {
		t.Fatalf("expected migration copy, got %+v", billing.Migrations)
	}
}

func TestRegistryAggregatesModuleExtensionMetadata(t *testing.T) {
	registry := NewRegistry(Module{
		Name: "inventory",
		Permissions: []domain.PermissionDefinition{
			{Code: "inventory:item:list", Name: "List inventory items", Module: "inventory"},
		},
		Backend: []BackendRoute{
			{Name: "inventory-items-list", Method: "GET", Path: "/api/v1/inventory/items", Permission: "inventory:item:list", Summary: "List inventory items"},
		},
		Menus: []MenuEntry{
			{Name: "inventory", Title: "Inventory", Path: "/inventory", Permission: "inventory:item:list"},
		},
		Routes: []FrontendRoute{
			{Name: "inventory", Path: "/inventory", Component: "InventoryView", Permission: "inventory:item:list"},
		},
		Migrations: []MigrationSet{
			{Driver: "postgres", Dir: "modules/inventory/migrations"},
		},
	})

	if got := registry.Permissions(); len(got) != 1 || got[0].Code != "inventory:item:list" {
		t.Fatalf("unexpected permissions: %+v", got)
	}
	if got := registry.BackendRoutes(); len(got) != 1 || got[0].Path != "/api/v1/inventory/items" {
		t.Fatalf("unexpected backend routes: %+v", got)
	}
	if got := registry.Menus(); len(got) != 1 || got[0].Name != "inventory" {
		t.Fatalf("unexpected menus: %+v", got)
	}
	if got := registry.FrontendRoutes(); len(got) != 1 || got[0].Component != "InventoryView" {
		t.Fatalf("unexpected frontend routes: %+v", got)
	}
	if got := registry.Migrations(); len(got) != 1 || got[0].Dir != "modules/inventory/migrations" {
		t.Fatalf("unexpected migrations: %+v", got)
	}
}

func TestNewValidatedRegistryAcceptsBuiltInModules(t *testing.T) {
	registry, err := NewValidatedRegistry(BuiltInModules()...)
	if err != nil {
		t.Fatalf("NewValidatedRegistry(BuiltInModules) error = %v", err)
	}
	if !hasBackendRoute(registry.BackendRoutes(), "system-users-list") {
		t.Fatalf("expected validated registry to include built-in backend routes")
	}
}

func TestValidateModulesRejectsInvalidExtensionMetadata(t *testing.T) {
	tests := []struct {
		name    string
		modules []Module
		want    string
	}{
		{
			name: "duplicate module name",
			modules: []Module{
				validInventoryModule(),
				validInventoryModule(),
			},
			want: `duplicate module name "inventory"`,
		},
		{
			name: "invalid module name",
			modules: []Module{
				{Name: "Inventory"},
			},
			want: `module name "Inventory" must match ^[a-z][a-z0-9_]*$`,
		},
		{
			name: "module name with surrounding spaces",
			modules: []Module{
				{Name: " inventory "},
			},
			want: `module name " inventory " must match ^[a-z][a-z0-9_]*$`,
		},
		{
			name: "duplicate permission code",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Permissions = append(mod.Permissions, domain.PermissionDefinition{
						Code:   "inventory:item:list",
						Name:   "Duplicate inventory item list",
						Module: "inventory",
					})
					return mod
				}(),
			},
			want: `duplicate permission code "inventory:item:list"`,
		},
		{
			name: "permission module mismatch",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Permissions[0].Module = "billing"
					return mod
				}(),
			},
			want: `permission inventory:item:list module = "billing", want "inventory"`,
		},
		{
			name: "permission code namespace mismatch",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Permissions[0].Code = "billing:item:list"
					mod.Backend[0].Permission = "billing:item:list"
					mod.Menus[0].Permission = "billing:item:list"
					mod.Routes[0].Permission = "billing:item:list"
					return mod
				}(),
			},
			want: `permission billing:item:list must use module namespace "inventory:"`,
		},
		{
			name: "backend route unknown permission",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Backend[0].Permission = "inventory:item:archive"
					return mod
				}(),
			},
			want: `backend route inventory-items-list references unknown permission "inventory:item:archive"`,
		},
		{
			name: "backend route cross module permission",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Backend[0].Permission = "billing:item:list"
					return mod
				}(),
				validBillingModule(),
			},
			want: `backend route inventory-items-list references permission "billing:item:list" owned by module billing`,
		},
		{
			name: "backend route missing summary",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Backend[0].Summary = ""
					return mod
				}(),
			},
			want: "must declare a summary",
		},
		{
			name: "backend route lowercase method",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Backend[0].Method = "get"
					return mod
				}(),
			},
			want: "must use a supported uppercase HTTP method",
		},
		{
			name: "backend route outside API version",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Backend[0].Path = "/inventory/items"
					return mod
				}(),
			},
			want: "must use an /api/v1 path",
		},
		{
			name: "backend route outside module namespace",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Backend[0].Path = "/api/v1/catalog/items"
					return mod
				}(),
			},
			want: `backend route inventory-items-list must use module API namespace "/api/v1/inventory"`,
		},
		{
			name: "public backend route with permission",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Backend[0].Public = true
					return mod
				}(),
			},
			want: "is public and must not declare a permission",
		},
		{
			name: "unknown parent menu",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Menus[0].Parent = "missing-parent"
					return mod
				}(),
			},
			want: `references unknown parent menu "missing-parent"`,
		},
		{
			name: "cross module parent menu",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Menus[0].Parent = "billing"
					return mod
				}(),
				validBillingModule(),
			},
			want: `menu inventory references parent menu "billing" owned by module billing`,
		},
		{
			name: "relative menu path",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Menus[0].Path = "inventory"
					return mod
				}(),
			},
			want: "must use an absolute path",
		},
		{
			name: "menu outside module namespace",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Menus[0].Path = "/catalog"
					return mod
				}(),
			},
			want: `menu inventory must use module path namespace "/inventory"`,
		},
		{
			name: "menu cross module permission",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Menus[0].Permission = "billing:item:list"
					return mod
				}(),
				validBillingModule(),
			},
			want: `menu inventory references permission "billing:item:list" owned by module billing`,
		},
		{
			name: "duplicate menu path",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Menus = append(mod.Menus, MenuEntry{
						Name:       "inventory-archive",
						Title:      "Inventory Archive",
						Path:       "/inventory",
						Component:  "InventoryArchiveView",
						Permission: "inventory:item:list",
					})
					return mod
				}(),
			},
			want: `duplicate menu path "/inventory"`,
		},
		{
			name: "menu frontend route component mismatch",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Menus[0].Component = "InventoryShell"
					return mod
				}(),
			},
			want: `component "InventoryShell" does not match frontend route inventory component "InventoryView" for path "/inventory"`,
		},
		{
			name: "relative frontend route path",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Routes[0].Path = "inventory"
					return mod
				}(),
			},
			want: "must use an absolute path",
		},
		{
			name: "frontend route outside module namespace",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Routes[0].Path = "/catalog"
					return mod
				}(),
			},
			want: `frontend route inventory must use module path namespace "/inventory"`,
		},
		{
			name: "frontend route cross module permission",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Routes[0].Permission = "billing:item:list"
					return mod
				}(),
				validBillingModule(),
			},
			want: `frontend route inventory references permission "billing:item:list" owned by module billing`,
		},
		{
			name: "duplicate frontend route path",
			modules: []Module{
				func() Module {
					mod := validInventoryModule()
					mod.Routes = append(mod.Routes, FrontendRoute{
						Name:       "inventory-archive",
						Path:       "/inventory",
						Component:  "InventoryArchiveView",
						Title:      "Inventory Archive",
						Permission: "inventory:item:list",
					})
					return mod
				}(),
			},
			want: `duplicate frontend route path "/inventory"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateModules(tt.modules...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected validation error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestBuiltInModulesExposeExtensionMetadata(t *testing.T) {
	registry := NewRegistry(BuiltInModules()...)

	if !hasPermission(registry.Permissions(), domain.PermissionDashboardView) {
		t.Fatalf("expected built-in permissions to include dashboard view")
	}
	if !hasBackendRoute(registry.BackendRoutes(), "system-users-list") {
		t.Fatalf("expected built-in backend routes to include system users list")
	}
	if !hasMenu(registry.Menus(), "system-users") {
		t.Fatalf("expected built-in menus to include system users")
	}
	if !hasRoute(registry.FrontendRoutes(), "system-users") {
		t.Fatalf("expected built-in frontend routes to include system users")
	}
	if migrations := registry.Migrations(); len(migrations) != 1 || migrations[0].Dir != "migrations" || migrations[0].SeedsDir != "migrations/seeds" {
		t.Fatalf("expected built-in migration metadata, got %+v", migrations)
	}
}

func TestBuiltInMenuComponentsMatchFrontendRoutes(t *testing.T) {
	registry := NewRegistry(BuiltInModules()...)
	routesByPath := map[string]FrontendRoute{}
	for _, route := range registry.FrontendRoutes() {
		routesByPath[route.Path] = route
	}

	for _, menu := range registry.Menus() {
		route, ok := routesByPath[menu.Path]
		if !ok {
			continue
		}
		if menu.Component != route.Component {
			t.Fatalf("menu %s component = %q, route component = %q", menu.Name, menu.Component, route.Component)
		}
	}
}

func TestBuiltInModulePermissionReferencesAreRegistered(t *testing.T) {
	registry := NewRegistry(BuiltInModules()...)
	known := map[string]bool{}
	for _, permission := range registry.Permissions() {
		known[permission.Code] = true
	}

	for _, route := range registry.BackendRoutes() {
		if route.Permission != "" && !known[route.Permission] {
			t.Fatalf("backend route %s references unregistered permission %q", route.Name, route.Permission)
		}
	}
	for _, menu := range registry.Menus() {
		if menu.Permission != "" && !known[menu.Permission] {
			t.Fatalf("menu %s references unregistered permission %q", menu.Name, menu.Permission)
		}
	}
	for _, route := range registry.FrontendRoutes() {
		if route.Permission != "" && !known[route.Permission] {
			t.Fatalf("frontend route %s references unregistered permission %q", route.Name, route.Permission)
		}
	}
}

func TestBuiltInFrontendRoutesMatchVueRouter(t *testing.T) {
	registry := NewRegistry(BuiltInModules()...)
	got := vueRouterRoutes(t)
	want := map[string]vueRouterRoute{}
	for _, route := range registry.FrontendRoutes() {
		want[route.Path] = vueRouterRoute{
			Path:       route.Path,
			Component:  route.Component,
			Title:      route.Title,
			Permission: route.Permission,
		}
	}

	assertStringKeys(t, "Vue Router protected module routes", stringKeys(got), stringKeys(want))
	for path, wantRoute := range want {
		if gotRoute := got[path]; gotRoute != wantRoute {
			t.Fatalf("Vue Router route %q = %+v, want %+v", path, gotRoute, wantRoute)
		}
	}
}

func hasPermission(items []domain.PermissionDefinition, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}

type vueRouterRoute struct {
	Path       string
	Component  string
	Title      string
	Permission string
}

func vueRouterRoutes(t *testing.T) map[string]vueRouterRoute {
	t.Helper()

	data, err := os.ReadFile("../../web/src/router/index.js")
	if err != nil {
		t.Fatalf("read Vue Router source: %v", err)
	}
	source := string(data)
	permissionValues := permissionValuesByName()
	pattern := regexp.MustCompile(`\{\s*path:\s*"([^"]+)",\s*name:\s*"([^"]+)",\s*component:\s*([A-Za-z][A-Za-z0-9_]*),\s*meta:\s*\{\s*title:\s*"([^"]+)",\s*permission:\s*permissions\.([A-Za-z][A-Za-z0-9_]*)\s*\}\s*\}`)
	routes := map[string]vueRouterRoute{}
	for _, match := range pattern.FindAllStringSubmatch(source, -1) {
		permission, ok := permissionValues[match[5]]
		if !ok {
			t.Fatalf("Vue Router route %q references unknown permissions.%s", match[1], match[5])
		}
		routes["/"+match[1]] = vueRouterRoute{
			Path:       "/" + match[1],
			Component:  match[3],
			Title:      match[4],
			Permission: permission,
		}
	}
	if len(routes) == 0 {
		t.Fatal("Vue Router source did not expose protected module routes")
	}
	return routes
}

func permissionValuesByName() map[string]string {
	values := map[string]string{}
	for _, permission := range domain.SystemPermissions() {
		values[permissionObjectName(permission.Code)] = permission.Code
	}
	values[permissionObjectName(domain.PermissionAll)] = domain.PermissionAll
	return values
}

func permissionObjectName(code string) string {
	if code == domain.PermissionAll {
		return "all"
	}
	parts := regexp.MustCompile(`[^A-Za-z0-9]+`).Split(code, -1)
	name := ""
	for index, part := range parts {
		if part == "" {
			continue
		}
		part = strings.ToLower(part[:1]) + part[1:]
		if index == 0 && name == "" {
			name = part
			continue
		}
		name += strings.ToUpper(part[:1]) + part[1:]
	}
	return name
}

func assertStringKeys(t *testing.T, label string, got []string, want []string) {
	t.Helper()

	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s mismatch:\ngot:\n%s\nwant:\n%s", label, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func stringKeys[V any](items map[string]V) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	return keys
}

func hasBackendRoute(items []BackendRoute, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func hasMenu(items []MenuEntry, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func hasRoute(items []FrontendRoute, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func validInventoryModule() Module {
	return Module{
		Name:        "inventory",
		Title:       "Inventory",
		Version:     "0.1.0",
		Description: "Inventory module",
		Permissions: []domain.PermissionDefinition{
			{Code: "inventory:item:list", Name: "List inventory items", Module: "inventory"},
		},
		Backend: []BackendRoute{
			{Name: "inventory-items-list", Method: "GET", Path: "/api/v1/inventory/items", Permission: "inventory:item:list", Summary: "List inventory items"},
		},
		Menus: []MenuEntry{
			{Name: "inventory", Title: "Inventory", Path: "/inventory", Component: "InventoryView", Permission: "inventory:item:list"},
		},
		Routes: []FrontendRoute{
			{Name: "inventory", Path: "/inventory", Component: "InventoryView", Title: "Inventory", Permission: "inventory:item:list"},
		},
		Migrations: []MigrationSet{
			{Driver: "postgres", Dir: "modules/inventory/migrations"},
		},
	}
}

func validBillingModule() Module {
	return Module{
		Name:        "billing",
		Title:       "Billing",
		Version:     "0.1.0",
		Description: "Billing module",
		Permissions: []domain.PermissionDefinition{
			{Code: "billing:item:list", Name: "List billing items", Module: "billing"},
		},
		Backend: []BackendRoute{
			{Name: "billing-items-list", Method: "GET", Path: "/api/v1/billing/items", Permission: "billing:item:list", Summary: "List billing items"},
		},
		Menus: []MenuEntry{
			{Name: "billing", Title: "Billing", Path: "/billing", Component: "BillingView", Permission: "billing:item:list"},
		},
		Routes: []FrontendRoute{
			{Name: "billing", Path: "/billing", Component: "BillingView", Title: "Billing", Permission: "billing:item:list"},
		},
		Migrations: []MigrationSet{
			{Driver: "postgres", Dir: "modules/billing/migrations"},
		},
	}
}
