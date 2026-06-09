package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/expary/GOV2/internal/config"
	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/module"
	"github.com/expary/GOV2/internal/repository"
	"github.com/expary/GOV2/internal/security"
	"github.com/expary/GOV2/internal/service"
)

type Options struct {
	Config   config.Config
	Logger   *slog.Logger
	Modules  *module.Registry
	Routes   []Route
	Store    repository.Store
	Services *service.Registry
	Tokens   *security.TokenManager
}

const maxJSONBodyBytes int64 = 1 << 20

var (
	errRequestBodyTooLarge      = errors.New("request body too large")
	errUnsupportedJSONMediaType = errors.New("unsupported JSON media type")
)

type Route struct {
	Method     string
	Path       string
	Public     bool
	Permission string
	Handler    http.Handler
}

type API struct {
	cfg       config.Config
	logger    *slog.Logger
	modules   *module.Registry
	routes    []Route
	httpStats *httpMetrics
	store     repository.Store
	services  *service.Registry
	tokens    *security.TokenManager
	started   time.Time
}

type principalKey struct{}

type Principal struct {
	User   domain.User
	Claims security.Claims
}

type routeSpec struct {
	Method          string
	Path            string
	Public          bool
	Permission      string
	Summary         string
	QueryParams     []string
	PathParamTypes  map[string]string
	QueryParamTypes map[string]string
	RequestSchema   string
	ResponseSchema  string
	SuccessStatus   int
	Conflict        bool
	NotFound        bool
	ValidationError bool
	Handler         func(*API) http.HandlerFunc
}

type readinessCheckResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func New(options Options) *API {
	modules := options.Modules
	if modules == nil {
		modules = module.NewRegistry()
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	routes := append([]Route(nil), options.Routes...)
	metrics := newHTTPMetrics(routesForMetrics(routes))
	return &API{
		cfg:       options.Config,
		logger:    logger,
		modules:   modules,
		routes:    routes,
		httpStats: metrics,
		store:     options.Store,
		services:  options.Services,
		tokens:    options.Tokens,
		started:   time.Now().UTC(),
	}
}

func (a *API) Router() http.Handler {
	mux := http.NewServeMux()

	for _, route := range a.routesForRouter() {
		if route.Handler == nil {
			continue
		}
		handler := route.Handler
		if route.requiresAuthentication() {
			handler = a.require(strings.TrimSpace(route.Permission), handler)
		}
		mux.Handle(route.Pattern(), handler)
	}

	mux.Handle("/", a.staticHandler())

	return a.withGlobalMiddleware(mux)
}

func (r Route) Pattern() string {
	return r.Method + " " + r.Path
}

func (r Route) requiresAuthentication() bool {
	return !r.Public || strings.TrimSpace(r.Permission) != ""
}

func (a *API) routesForRouter() []Route {
	routes := make([]Route, 0, len(apiRouteSpecs())+len(a.routes))
	for _, route := range apiRouteSpecs() {
		routes = append(routes, Route{
			Method:     route.Method,
			Path:       route.Path,
			Public:     route.Public,
			Permission: route.Permission,
			Handler:    http.Handler(route.Handler(a)),
		})
	}
	routes = append(routes, a.routes...)
	return routes
}

func routesForMetrics(extensionRoutes []Route) []Route {
	routes := make([]Route, 0, len(apiRouteSpecs())+len(extensionRoutes))
	for _, route := range apiRouteSpecs() {
		routes = append(routes, Route{
			Method: route.Method,
			Path:   route.Path,
		})
	}
	routes = append(routes, extensionRoutes...)
	return routes
}

func apiRouteSpecs() []routeSpec {
	return []routeSpec{
		{Method: http.MethodGet, Path: "/api/v1/health", Public: true, Summary: "Health check", ResponseSchema: "HealthResponse", Handler: func(a *API) http.HandlerFunc { return a.health }},
		{Method: http.MethodGet, Path: "/api/v1/ready", Public: true, Summary: "Readiness check", ResponseSchema: "ReadinessResponse", Handler: func(a *API) http.HandlerFunc { return a.ready }},
		{Method: http.MethodGet, Path: "/api/v1/metrics", Public: true, Summary: "Runtime metrics", Handler: func(a *API) http.HandlerFunc { return a.metrics }},
		{Method: http.MethodGet, Path: "/api/v1/app/config", Public: true, Summary: "Public application configuration", ResponseSchema: "AppConfig", Handler: func(a *API) http.HandlerFunc { return a.appConfig }},
		{Method: http.MethodPost, Path: "/api/v1/auth/login", Public: true, Summary: "Login", RequestSchema: "LoginRequest", ResponseSchema: "LoginResponse", Handler: func(a *API) http.HandlerFunc { return a.login }},
		{Method: http.MethodPost, Path: "/api/v1/auth/logout", Summary: "Logout", ResponseSchema: "OkResult", Handler: func(a *API) http.HandlerFunc { return a.logout }},
		{Method: http.MethodGet, Path: "/api/v1/auth/profile", Summary: "Current user profile", ResponseSchema: "AuthProfile", Handler: func(a *API) http.HandlerFunc { return a.profile }},
		{Method: http.MethodPut, Path: "/api/v1/auth/profile", Summary: "Update current user profile", RequestSchema: "UpdateProfileRequest", ResponseSchema: "PublicUser", Conflict: true, NotFound: true, Handler: func(a *API) http.HandlerFunc { return a.updateProfile }},
		{Method: http.MethodPut, Path: "/api/v1/auth/password", Summary: "Change current user password", RequestSchema: "ChangePasswordRequest", ResponseSchema: "OkResult", NotFound: true, ValidationError: true, Handler: func(a *API) http.HandlerFunc { return a.changePassword }},
		{Method: http.MethodGet, Path: "/api/v1/dashboard/summary", Permission: domain.PermissionDashboardView, Summary: "Dashboard summary", ResponseSchema: "DashboardSummary", Handler: func(a *API) http.HandlerFunc { return a.dashboard }},

		{Method: http.MethodGet, Path: "/api/v1/system/users", Permission: domain.PermissionSystemUserList, Summary: "List users", QueryParams: []string{"page", "page_size", "keyword", "status"}, QueryParamTypes: map[string]string{"page": "integer", "page_size": "integer", "keyword": "string", "status": "string"}, ResponseSchema: "UserPage", Handler: func(a *API) http.HandlerFunc { return a.listUsers }},
		{Method: http.MethodPost, Path: "/api/v1/system/users", Permission: domain.PermissionSystemUserCreate, Summary: "Create user", RequestSchema: "CreateUserRequest", ResponseSchema: "PublicUser", SuccessStatus: http.StatusCreated, Conflict: true, ValidationError: true, Handler: func(a *API) http.HandlerFunc { return a.createUser }},
		{Method: http.MethodGet, Path: "/api/v1/system/users/{id}", Permission: domain.PermissionSystemUserList, Summary: "Get user", PathParamTypes: idPathParamTypes(), ResponseSchema: "PublicUser", NotFound: true, Handler: func(a *API) http.HandlerFunc { return a.getUser }},
		{Method: http.MethodPut, Path: "/api/v1/system/users/{id}", Permission: domain.PermissionSystemUserUpdate, Summary: "Update user", PathParamTypes: idPathParamTypes(), RequestSchema: "UpdateUserRequest", ResponseSchema: "PublicUser", Conflict: true, NotFound: true, ValidationError: true, Handler: func(a *API) http.HandlerFunc { return a.updateUser }},
		{Method: http.MethodPatch, Path: "/api/v1/system/users/{id}/status", Permission: domain.PermissionSystemUserUpdate, Summary: "Update user status", PathParamTypes: idPathParamTypes(), RequestSchema: "UserStatusRequest", ResponseSchema: "PublicUser", Conflict: true, NotFound: true, ValidationError: true, Handler: func(a *API) http.HandlerFunc { return a.updateUserStatus }},
		{Method: http.MethodDelete, Path: "/api/v1/system/users/{id}", Permission: domain.PermissionSystemUserDelete, Summary: "Delete user", PathParamTypes: idPathParamTypes(), ResponseSchema: "OkResult", Conflict: true, NotFound: true, Handler: func(a *API) http.HandlerFunc { return a.deleteUser }},

		{Method: http.MethodGet, Path: "/api/v1/system/roles", Permission: domain.PermissionSystemRoleList, Summary: "List roles", ResponseSchema: "RoleList", Handler: func(a *API) http.HandlerFunc { return a.listRoles }},
		{Method: http.MethodPost, Path: "/api/v1/system/roles", Permission: domain.PermissionSystemRoleCreate, Summary: "Create role", RequestSchema: "RoleRequest", ResponseSchema: "Role", SuccessStatus: http.StatusCreated, Conflict: true, ValidationError: true, Handler: func(a *API) http.HandlerFunc { return a.createRole }},
		{Method: http.MethodPut, Path: "/api/v1/system/roles/{id}", Permission: domain.PermissionSystemRoleUpdate, Summary: "Update role", PathParamTypes: idPathParamTypes(), RequestSchema: "RoleRequest", ResponseSchema: "Role", Conflict: true, NotFound: true, ValidationError: true, Handler: func(a *API) http.HandlerFunc { return a.updateRole }},
		{Method: http.MethodDelete, Path: "/api/v1/system/roles/{id}", Permission: domain.PermissionSystemRoleDelete, Summary: "Delete role", PathParamTypes: idPathParamTypes(), ResponseSchema: "OkResult", Conflict: true, NotFound: true, Handler: func(a *API) http.HandlerFunc { return a.deleteRole }},
		{Method: http.MethodGet, Path: "/api/v1/system/permissions", Permission: domain.PermissionSystemRoleList, Summary: "List system permissions", ResponseSchema: "PermissionDefinitionList", Handler: func(a *API) http.HandlerFunc { return a.listPermissions }},

		{Method: http.MethodGet, Path: "/api/v1/system/menus", Permission: domain.PermissionSystemMenuList, Summary: "List menus", ResponseSchema: "MenuList", Handler: func(a *API) http.HandlerFunc { return a.listMenus }},
		{Method: http.MethodPost, Path: "/api/v1/system/menus", Permission: domain.PermissionSystemMenuCreate, Summary: "Create menu", RequestSchema: "MenuRequest", ResponseSchema: "Menu", SuccessStatus: http.StatusCreated, Conflict: true, NotFound: true, ValidationError: true, Handler: func(a *API) http.HandlerFunc { return a.createMenu }},
		{Method: http.MethodPut, Path: "/api/v1/system/menus/{id}", Permission: domain.PermissionSystemMenuUpdate, Summary: "Update menu", PathParamTypes: idPathParamTypes(), RequestSchema: "MenuRequest", ResponseSchema: "Menu", Conflict: true, NotFound: true, ValidationError: true, Handler: func(a *API) http.HandlerFunc { return a.updateMenu }},
		{Method: http.MethodDelete, Path: "/api/v1/system/menus/{id}", Permission: domain.PermissionSystemMenuDelete, Summary: "Delete menu", PathParamTypes: idPathParamTypes(), ResponseSchema: "OkResult", Conflict: true, NotFound: true, Handler: func(a *API) http.HandlerFunc { return a.deleteMenu }},
		{Method: http.MethodGet, Path: "/api/v1/system/modules", Permission: domain.PermissionSystemModuleList, Summary: "List registered modules", ResponseSchema: "ModuleList", Handler: func(a *API) http.HandlerFunc { return a.listModules }},

		{Method: http.MethodGet, Path: "/api/v1/system/dictionaries", Permission: domain.PermissionSystemDictionaryList, Summary: "List dictionaries", ResponseSchema: "DictionaryList", Handler: func(a *API) http.HandlerFunc { return a.listDictionaries }},
		{Method: http.MethodGet, Path: "/api/v1/system/dictionaries/code/{code}", Summary: "Get dictionary by code", PathParamTypes: map[string]string{"code": "string"}, ResponseSchema: "Dictionary", NotFound: true, Handler: func(a *API) http.HandlerFunc { return a.getDictionaryByCode }},
		{Method: http.MethodPost, Path: "/api/v1/system/dictionaries", Permission: domain.PermissionSystemDictionaryCreate, Summary: "Create dictionary", RequestSchema: "DictionaryRequest", ResponseSchema: "Dictionary", SuccessStatus: http.StatusCreated, Conflict: true, ValidationError: true, Handler: func(a *API) http.HandlerFunc { return a.createDictionary }},
		{Method: http.MethodPut, Path: "/api/v1/system/dictionaries/{id}", Permission: domain.PermissionSystemDictionaryUpdate, Summary: "Update dictionary", PathParamTypes: idPathParamTypes(), RequestSchema: "DictionaryRequest", ResponseSchema: "Dictionary", Conflict: true, NotFound: true, ValidationError: true, Handler: func(a *API) http.HandlerFunc { return a.updateDictionary }},
		{Method: http.MethodDelete, Path: "/api/v1/system/dictionaries/{id}", Permission: domain.PermissionSystemDictionaryDelete, Summary: "Delete dictionary", PathParamTypes: idPathParamTypes(), ResponseSchema: "OkResult", NotFound: true, Handler: func(a *API) http.HandlerFunc { return a.deleteDictionary }},

		{Method: http.MethodGet, Path: "/api/v1/system/settings", Permission: domain.PermissionSystemSettingList, Summary: "List settings", ResponseSchema: "SettingList", Handler: func(a *API) http.HandlerFunc { return a.listSettings }},
		{Method: http.MethodPost, Path: "/api/v1/system/settings", Permission: domain.PermissionSystemSettingCreate, Summary: "Create setting", RequestSchema: "SettingRequest", ResponseSchema: "Setting", SuccessStatus: http.StatusCreated, Conflict: true, ValidationError: true, Handler: func(a *API) http.HandlerFunc { return a.createSetting }},
		{Method: http.MethodPut, Path: "/api/v1/system/settings/{id}", Permission: domain.PermissionSystemSettingUpdate, Summary: "Update setting", PathParamTypes: idPathParamTypes(), RequestSchema: "SettingRequest", ResponseSchema: "Setting", Conflict: true, NotFound: true, ValidationError: true, Handler: func(a *API) http.HandlerFunc { return a.updateSetting }},
		{Method: http.MethodDelete, Path: "/api/v1/system/settings/{id}", Permission: domain.PermissionSystemSettingDelete, Summary: "Delete setting", PathParamTypes: idPathParamTypes(), ResponseSchema: "OkResult", NotFound: true, Handler: func(a *API) http.HandlerFunc { return a.deleteSetting }},
		{Method: http.MethodGet, Path: "/api/v1/system/audit-logs", Permission: domain.PermissionSystemAuditList, Summary: "List audit logs", QueryParams: []string{"page", "page_size", "keyword", "actor", "action", "resource", "resource_id"}, QueryParamTypes: map[string]string{"page": "integer", "page_size": "integer", "keyword": "string", "actor": "string", "action": "string", "resource": "string", "resource_id": "string"}, ResponseSchema: "AuditLogPage", Handler: func(a *API) http.HandlerFunc { return a.listAuditLogs }},
	}
}

func idPathParamTypes() map[string]string {
	return map[string]string{"id": "integer"}
}

func (a *API) withGlobalMiddleware(next http.Handler) http.Handler {
	return requestID(recordHTTPMetrics(a.httpStats)(recoverer(a.logger)(accessLog(a.logger)(securityHeaders(cors(a.cfg.Security.AllowedOrigins)(next))))))
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	writeOK(w, r, map[string]any{
		"name":        a.cfg.App.Name,
		"environment": a.cfg.App.Environment,
		"status":      "ok",
		"time":        time.Now().UTC(),
	})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := []readinessCheckResponse{
		{Name: "store", Status: "ok"},
	}
	status := http.StatusOK
	if checker, ok := a.store.(repository.HealthChecker); ok {
		if err := checker.CheckHealth(ctx); err != nil {
			checks[0].Status = "error"
			checks[0].Error = err.Error()
			status = http.StatusServiceUnavailable
		}
	} else if a.store == nil {
		checks[0].Status = "error"
		checks[0].Error = "store not configured"
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, r, status, status, http.StatusText(status), map[string]any{
		"status": checks[0].Status,
		"checks": checks,
		"time":   time.Now().UTC(),
	})
}

func (a *API) metrics(w http.ResponseWriter, r *http.Request) {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = fmt.Fprintf(w, "# HELP gov2_up GOV2 process health.\n")
	_, _ = fmt.Fprintf(w, "# TYPE gov2_up gauge\n")
	_, _ = fmt.Fprintf(w, "gov2_up 1\n")
	_, _ = fmt.Fprintf(w, "# HELP gov2_uptime_seconds Seconds since HTTP API startup.\n")
	_, _ = fmt.Fprintf(w, "# TYPE gov2_uptime_seconds counter\n")
	_, _ = fmt.Fprintf(w, "gov2_uptime_seconds %.0f\n", time.Since(a.started).Seconds())
	_, _ = fmt.Fprintf(w, "# HELP gov2_runtime_goroutines Number of goroutines.\n")
	_, _ = fmt.Fprintf(w, "# TYPE gov2_runtime_goroutines gauge\n")
	_, _ = fmt.Fprintf(w, "gov2_runtime_goroutines %d\n", runtime.NumGoroutine())
	_, _ = fmt.Fprintf(w, "# HELP gov2_runtime_memory_bytes Runtime memory gauges.\n")
	_, _ = fmt.Fprintf(w, "# TYPE gov2_runtime_memory_bytes gauge\n")
	_, _ = fmt.Fprintf(w, "gov2_runtime_memory_bytes{type=\"alloc\"} %d\n", memory.Alloc)
	_, _ = fmt.Fprintf(w, "gov2_runtime_memory_bytes{type=\"heap_alloc\"} %d\n", memory.HeapAlloc)
	_, _ = fmt.Fprintf(w, "gov2_runtime_memory_bytes{type=\"heap_idle\"} %d\n", memory.HeapIdle)
	_, _ = fmt.Fprintf(w, "# HELP gov2_runtime_gc_total Total completed garbage collections.\n")
	_, _ = fmt.Fprintf(w, "# TYPE gov2_runtime_gc_total counter\n")
	_, _ = fmt.Fprintf(w, "gov2_runtime_gc_total %d\n", memory.NumGC)
	_, _ = fmt.Fprintf(w, "# HELP gov2_http_requests_total Total HTTP requests by method, route, and status.\n")
	_, _ = fmt.Fprintf(w, "# TYPE gov2_http_requests_total counter\n")
	for _, item := range a.httpStats.snapshot() {
		_, _ = fmt.Fprintf(w,
			"gov2_http_requests_total{method=\"%s\",route=\"%s\",status=\"%d\"} %d\n",
			prometheusLabelValue(item.Key.Method),
			prometheusLabelValue(item.Key.Route),
			item.Key.Status,
			item.Count,
		)
	}
}

type appConfigResponse struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Environment string `json:"environment"`
}

func (a *API) appConfig(w http.ResponseWriter, r *http.Request) {
	name := appName(a.cfg.App.Name)
	writeOK(w, r, appConfigResponse{
		Name:        name,
		Title:       a.siteTitle(name),
		Environment: strings.TrimSpace(a.cfg.App.Environment),
	})
}

func (a *API) siteTitle(fallback string) string {
	title := strings.TrimSpace(fallback)
	if title == "" {
		title = "GOV2"
	}
	if a.store == nil {
		return title
	}
	settings, err := a.store.ListSettings()
	if err != nil {
		return title
	}
	for _, setting := range settings {
		if !strings.EqualFold(setting.Key, "site.title") {
			continue
		}
		var configured string
		if err := json.Unmarshal(setting.Value, &configured); err != nil {
			return title
		}
		configured = strings.TrimSpace(configured)
		if configured == "" {
			return title
		}
		return configured
	}
	return title
}

func appName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "GOV2"
	}
	return value
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}

	result, err := a.services.Auth.Login(service.LoginInput{
		Username:  req.Username,
		Password:  req.Password,
		IP:        r.RemoteAddr,
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	principal, _ := CurrentPrincipal(r.Context())
	if _, err := a.store.AddAuditLog(domain.AuditLog{
		ActorID:    principal.User.ID,
		Actor:      principal.User.Username,
		Action:     "logout",
		Resource:   "auth",
		ResourceID: strconv.FormatUint(principal.User.ID, 10),
		IP:         remoteIP(r.RemoteAddr),
		UserAgent:  r.UserAgent(),
		Detail:     "user logged out",
	}); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, map[string]bool{"ok": true})
}

func (a *API) profile(w http.ResponseWriter, r *http.Request) {
	principal, _ := CurrentPrincipal(r.Context())
	roles, err := a.store.ListRoles()
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	permissions := permissionsForRoles(roles, principal.User.RoleIDs)
	menus, err := a.store.ListMenus()
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, map[string]any{
		"user":        principal.User.Public(),
		"roles":       principal.User.RoleIDs,
		"permissions": permissions,
		"menus":       menusForPermissions(menus, permissions),
	})
}

func (a *API) updateProfile(w http.ResponseWriter, r *http.Request) {
	principal, _ := CurrentPrincipal(r.Context())
	var input service.UpdateProfileInput
	if err := readJSON(r, &input); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	user, err := a.services.Auth.UpdateProfile(principal.User.ID, input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "update", "auth.profile", principal.User.ID, "updated own profile"); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, user)
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	principal, _ := CurrentPrincipal(r.Context())
	var input service.ChangePasswordInput
	if err := readJSON(r, &input); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if err := a.services.Auth.ChangePassword(principal.User.ID, input); err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			if auditErr := a.auditResource(r, "password_failed", "auth.password", principal.User.ID, "password change failed: invalid current password"); auditErr != nil {
				writeServiceError(w, r, auditErr)
				return
			}
		}
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "password", "auth.password", principal.User.ID, "changed own password"); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, map[string]bool{"ok": true})
}

func (a *API) dashboard(w http.ResponseWriter, r *http.Request) {
	summary, err := a.services.System.Dashboard()
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, summary)
}

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	page, pageSize := pageParams(r)
	result, err := a.services.Users.List(repository.UserQuery{
		Keyword:  r.URL.Query().Get("keyword"),
		Status:   r.URL.Query().Get("status"),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (a *API) getUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	user, err := a.services.Users.Get(id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, user)
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	var input service.CreateUserInput
	if err := readJSON(r, &input); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	user, err := a.services.Users.Create(input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "create", "system.user", user.ID, "created user "+strconv.FormatUint(user.ID, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeCreated(w, r, user)
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input service.UpdateUserInput
	if err := readJSON(r, &input); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	user, err := a.services.Users.Update(id, input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "update", "system.user", user.ID, "updated user "+strconv.FormatUint(user.ID, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, user)
}

func (a *API) updateUserStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if err := readJSON(r, &input); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	user, err := a.services.Users.SetStatus(id, input.Status)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "status", "system.user", user.ID, "set user "+strconv.FormatUint(user.ID, 10)+" status to "+user.Status); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, user)
}

func (a *API) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.services.Users.Delete(id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "delete", "system.user", id, "deleted user "+strconv.FormatUint(id, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, map[string]bool{"ok": true})
}

func (a *API) listRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := a.services.System.ListRoles()
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, roles)
}

func (a *API) createRole(w http.ResponseWriter, r *http.Request) {
	var input service.RoleInput
	if err := readJSON(r, &input); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	role, err := a.services.System.CreateRole(input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "create", "system.role", role.ID, "created role "+strconv.FormatUint(role.ID, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeCreated(w, r, role)
}

func (a *API) updateRole(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input service.RoleInput
	if err := readJSON(r, &input); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	role, err := a.services.System.UpdateRole(id, input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "update", "system.role", role.ID, "updated role "+strconv.FormatUint(role.ID, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, role)
}

func (a *API) deleteRole(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.services.System.DeleteRole(id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "delete", "system.role", id, "deleted role "+strconv.FormatUint(id, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, map[string]bool{"ok": true})
}

func (a *API) listPermissions(w http.ResponseWriter, r *http.Request) {
	writeOK(w, r, a.services.System.Permissions())
}

func (a *API) listMenus(w http.ResponseWriter, r *http.Request) {
	menus, err := a.services.System.Menus()
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, menus)
}

func (a *API) createMenu(w http.ResponseWriter, r *http.Request) {
	var input service.MenuInput
	if err := readJSON(r, &input); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	menu, err := a.services.System.CreateMenu(input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "create", "system.menu", menu.ID, "created menu "+strconv.FormatUint(menu.ID, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeCreated(w, r, menu)
}

func (a *API) updateMenu(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input service.MenuInput
	if err := readJSON(r, &input); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	menu, err := a.services.System.UpdateMenu(id, input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "update", "system.menu", menu.ID, "updated menu "+strconv.FormatUint(menu.ID, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, menu)
}

func (a *API) deleteMenu(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.services.System.DeleteMenu(id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "delete", "system.menu", id, "deleted menu "+strconv.FormatUint(id, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, map[string]bool{"ok": true})
}

func (a *API) listModules(w http.ResponseWriter, r *http.Request) {
	writeOK(w, r, a.modules.List())
}

func (a *API) listDictionaries(w http.ResponseWriter, r *http.Request) {
	dictionaries, err := a.services.System.Dictionaries()
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, dictionaries)
}

func (a *API) getDictionaryByCode(w http.ResponseWriter, r *http.Request) {
	dictionary, err := a.services.System.DictionaryByCode(r.PathValue("code"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, dictionary)
}

func (a *API) createDictionary(w http.ResponseWriter, r *http.Request) {
	var input service.DictionaryInput
	if err := readJSON(r, &input); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	dictionary, err := a.services.System.CreateDictionary(input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "create", "system.dictionary", dictionary.ID, "created dictionary "+strconv.FormatUint(dictionary.ID, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeCreated(w, r, dictionary)
}

func (a *API) updateDictionary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input service.DictionaryInput
	if err := readJSON(r, &input); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	dictionary, err := a.services.System.UpdateDictionary(id, input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "update", "system.dictionary", dictionary.ID, "updated dictionary "+strconv.FormatUint(dictionary.ID, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, dictionary)
}

func (a *API) deleteDictionary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.services.System.DeleteDictionary(id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "delete", "system.dictionary", id, "deleted dictionary "+strconv.FormatUint(id, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, map[string]bool{"ok": true})
}

func (a *API) listSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := a.services.System.Settings()
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, settings)
}

func (a *API) createSetting(w http.ResponseWriter, r *http.Request) {
	var input service.SettingInput
	if err := readJSON(r, &input); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	setting, err := a.services.System.CreateSetting(input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "create", "system.setting", setting.ID, "created setting "+strconv.FormatUint(setting.ID, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeCreated(w, r, setting)
}

func (a *API) updateSetting(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input service.SettingInput
	if err := readJSON(r, &input); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	setting, err := a.services.System.UpdateSetting(id, input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "update", "system.setting", setting.ID, "updated setting "+strconv.FormatUint(setting.ID, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, setting)
}

func (a *API) deleteSetting(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.services.System.DeleteSetting(id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	if err := a.auditResource(r, "delete", "system.setting", id, "deleted setting "+strconv.FormatUint(id, 10)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, map[string]bool{"ok": true})
}

func (a *API) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	page, pageSize := pageParams(r)
	result, err := a.services.System.AuditLogs(repository.AuditLogQuery{
		Keyword:    r.URL.Query().Get("keyword"),
		Actor:      r.URL.Query().Get("actor"),
		Action:     r.URL.Query().Get("action"),
		Resource:   r.URL.Query().Get("resource"),
		ResourceID: r.URL.Query().Get("resource_id"),
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (a *API) require(permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := security.BearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeError(w, r, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := a.tokens.Verify(token)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "invalid bearer token")
			return
		}
		user, err := a.store.GetUser(claims.Subject)
		if err != nil || user.Status != domain.UserStatusActive {
			writeError(w, r, http.StatusUnauthorized, "invalid user")
			return
		}
		roles, err := a.store.ListRoles()
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		policy := security.NewPolicy(roles)
		if permission != "" && !policy.Allows(user.RoleIDs, permission) {
			writeError(w, r, http.StatusForbidden, "permission denied")
			return
		}

		ctx := context.WithValue(r.Context(), principalKey{}, Principal{
			User:   user,
			Claims: claims,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *API) auditResource(r *http.Request, action, resource string, resourceID uint64, detail string) error {
	principal, ok := CurrentPrincipal(r.Context())
	if !ok {
		return nil
	}
	resourceIDText := ""
	if resourceID != 0 {
		resourceIDText = strconv.FormatUint(resourceID, 10)
	}
	_, err := a.store.AddAuditLog(domain.AuditLog{
		ActorID:    principal.User.ID,
		Actor:      principal.User.Username,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceIDText,
		IP:         remoteIP(r.RemoteAddr),
		UserAgent:  r.UserAgent(),
		Detail:     detail,
	})
	return err
}

func remoteIP(value string) string {
	host, _, err := net.SplitHostPort(value)
	if err == nil {
		return host
	}
	return value
}

func (a *API) staticHandler() http.Handler {
	staticDir := a.cfg.Web.StaticDir
	fileServer := http.FileServer(http.Dir(staticDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r.URL.Path) {
			writeError(w, r, http.StatusNotFound, "api route not found")
			return
		}

		path, ok := staticFilePath(staticDir, r.URL.Path)
		if !ok {
			writeError(w, r, http.StatusNotFound, "static asset not found")
			return
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
	})
}

func isAPIPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/")
}

func staticFilePath(staticDir, requestPath string) (string, bool) {
	root, err := filepath.Abs(staticDir)
	if err != nil {
		return "", false
	}
	cleaned := strings.TrimPrefix(filepath.Clean("/"+requestPath), string(filepath.Separator))
	path := filepath.Join(root, cleaned)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return path, true
}

func CurrentPrincipal(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(Principal)
	return principal, ok
}

func permissionsForRoles(roles []domain.Role, roleIDs []uint64) []string {
	roleSet := map[uint64]struct{}{}
	for _, id := range roleIDs {
		roleSet[id] = struct{}{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, role := range roles {
		if _, ok := roleSet[role.ID]; !ok {
			continue
		}
		for _, permission := range role.Permissions {
			permission = strings.TrimSpace(permission)
			if permission == "" {
				continue
			}
			if permission == domain.PermissionAll {
				return []string{domain.PermissionAll}
			}
			if _, ok := seen[permission]; ok {
				continue
			}
			seen[permission] = struct{}{}
			out = append(out, permission)
		}
	}
	return out
}

func menusForPermissions(items []domain.Menu, permissions []string) []domain.Menu {
	allowed := map[string]struct{}{}
	for _, permission := range permissions {
		allowed[permission] = struct{}{}
	}

	var walk func([]domain.Menu) []domain.Menu
	walk = func(items []domain.Menu) []domain.Menu {
		out := make([]domain.Menu, 0, len(items))
		for _, item := range items {
			if item.Hidden {
				continue
			}
			children := walk(item.Children)
			layout := strings.EqualFold(item.Component, "layout")
			selfAllowed := menuAllowed(item.Permission, allowed) && (item.Permission != "" || !layout)
			if !selfAllowed && len(children) == 0 {
				continue
			}
			item.Children = children
			out = append(out, item)
		}
		return out
	}

	return walk(items)
}

func menuAllowed(permission string, allowed map[string]struct{}) bool {
	if permission == "" {
		return true
	}
	if _, ok := allowed[domain.PermissionAll]; ok {
		return true
	}
	_, ok := allowed[permission]
	return ok
}

func pageParams(r *http.Request) (int, int) {
	page := queryInt(r, "page", 1)
	pageSize := queryInt(r, "page_size", repository.DefaultPageSize)
	return repository.NormalizePage(page, pageSize)
}

func queryInt(r *http.Request, key string, fallback int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func pathID(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		writeError(w, r, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func readJSON(r *http.Request, out any) error {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return errUnsupportedJSONMediaType
	}

	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxJSONBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return errRequestBodyTooLarge
		}
		return err
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain a single JSON value")
		}
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return errRequestBodyTooLarge
		}
		return err
	}
	return nil
}

func isJSONContentType(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	mediaType = strings.ToLower(mediaType)
	if mediaType == "application/json" {
		return true
	}
	return strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json")
}

func writeJSONDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errUnsupportedJSONMediaType) {
		writeError(w, r, http.StatusUnsupportedMediaType, "unsupported media type")
		return
	}
	if errors.Is(err, errRequestBodyTooLarge) {
		writeError(w, r, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	writeError(w, r, http.StatusBadRequest, "invalid request body")
}

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	if fields, ok := service.ValidationFields(err); ok {
		writeErrorData(w, r, http.StatusBadRequest, "invalid input", map[string]any{
			"fields": fields,
		})
		return
	}

	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		writeError(w, r, http.StatusUnauthorized, "invalid username or password")
	case errors.Is(err, service.ErrDisabledUser):
		writeError(w, r, http.StatusForbidden, "user is disabled")
	case errors.Is(err, service.ErrInvalidInput):
		writeError(w, r, http.StatusBadRequest, "invalid input")
	case errors.Is(err, service.ErrLastAdministrator):
		writeError(w, r, http.StatusConflict, "last active administrator cannot be removed")
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "record not found")
	case errors.Is(err, repository.ErrConflict):
		writeError(w, r, http.StatusConflict, "record already exists")
	case errors.Is(err, repository.ErrInvalidReference):
		writeError(w, r, http.StatusBadRequest, "invalid record reference")
	case errors.Is(err, repository.ErrConstraint):
		writeError(w, r, http.StatusBadRequest, "record constraint violation")
	default:
		writeError(w, r, http.StatusInternalServerError, "internal server error")
	}
}
