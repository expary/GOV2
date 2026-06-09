package httpapi

import (
	"net/http"
	"testing"

	"github.com/expary/GOV2/internal/module"
)

func TestBuiltInModuleBackendRoutesMatchHTTPRouteSpecs(t *testing.T) {
	specs := map[string]routeSpec{}
	for _, spec := range apiRouteSpecs() {
		specs[routeKey(spec.Method, spec.Path)] = spec
	}

	registry := module.NewRegistry(module.BuiltInModules()...)
	metadata := map[string]module.BackendRoute{}
	for _, route := range registry.BackendRoutes() {
		key := routeKey(route.Method, route.Path)
		if _, exists := metadata[key]; exists {
			t.Fatalf("duplicate built-in module backend route metadata %s", key)
		}
		spec, ok := specs[key]
		if !ok {
			t.Fatalf("built-in module backend route %s is not registered by HTTP route specs", key)
		}
		if route.Public != spec.Public {
			t.Fatalf("built-in module backend route %s public = %v, HTTP route spec public = %v", key, route.Public, spec.Public)
		}
		if route.Permission != spec.Permission {
			t.Fatalf("built-in module backend route %s permission = %q, HTTP route spec permission = %q", key, route.Permission, spec.Permission)
		}
		if route.Summary != spec.Summary {
			t.Fatalf("built-in module backend route %s summary = %q, HTTP route spec summary = %q", key, route.Summary, spec.Summary)
		}
		metadata[key] = route
	}

	for _, spec := range apiRouteSpecs() {
		if !requiresBuiltInModuleMetadata(spec) {
			continue
		}
		key := routeKey(spec.Method, spec.Path)
		if _, ok := metadata[key]; !ok {
			t.Fatalf("HTTP route spec %s must be described by built-in module backend route metadata", key)
		}
	}
}

func TestBuiltInModuleBackendRouteSummariesMatchOpenAPI(t *testing.T) {
	operations := loadOpenAPIOperations(t)
	registry := module.NewRegistry(module.BuiltInModules()...)

	for _, route := range registry.BackendRoutes() {
		key := routeKey(route.Method, route.Path)
		if route.Summary == "" {
			t.Fatalf("built-in module backend route %s must declare a summary", key)
		}
		operation, ok := operations[openAPIKey(route.Method, route.Path)]
		if !ok {
			t.Fatalf("built-in module backend route %s is not documented by OpenAPI", key)
		}
		if route.Summary != operation.Summary {
			t.Fatalf("built-in module backend route %s summary = %q, OpenAPI summary = %q", key, route.Summary, operation.Summary)
		}
	}
}

func requiresBuiltInModuleMetadata(spec routeSpec) bool {
	if spec.Public {
		return false
	}
	if spec.Permission == "" {
		return false
	}
	switch spec.Path {
	case "/api/v1/auth/logout", "/api/v1/auth/profile", "/api/v1/auth/password":
		return false
	}
	if spec.Method == http.MethodGet && spec.Path == "/api/v1/system/dictionaries/code/{code}" {
		return false
	}
	return true
}

func routeKey(method, routePath string) string {
	return method + " " + routePath
}
