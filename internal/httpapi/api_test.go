package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/expary/GOV2/internal/config"
	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/httpapi"
	"github.com/expary/GOV2/internal/module"
	"github.com/expary/GOV2/internal/security"
	"github.com/expary/GOV2/internal/service"
	"github.com/expary/GOV2/internal/store/memory"
)

type testApp struct {
	handler http.Handler
	store   *memory.Store
}

type responseEnvelope[T any] struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      T      `json:"data"`
	RequestID string `json:"request_id"`
}

type loginResult struct {
	Token string `json:"token"`
}

func TestAuthProfileAndPermissionDenied(t *testing.T) {
	app := newTestApp(t)

	adminToken := login(t, app, "admin", "admin123")
	profile := requestJSON[struct {
		Permissions []string      `json:"permissions"`
		Menus       []domain.Menu `json:"menus"`
	}](t, app, http.MethodGet, "/api/v1/auth/profile", adminToken, nil, http.StatusOK)
	if !contains(profile.Permissions, domain.PermissionAll) {
		t.Fatalf("expected admin profile to include %q, got %+v", domain.PermissionAll, profile.Permissions)
	}
	if !hasMenuPath(profile.Menus, "/dashboard") || !hasMenuPath(profile.Menus, "/system/users") {
		t.Fatalf("expected admin profile menus to include dashboard and users, got %+v", profile.Menus)
	}
	for path, component := range map[string]string{
		"/dashboard":           "DashboardView",
		"/system/users":        "UsersView",
		"/system/roles":        "RolesView",
		"/system/menus":        "MenusView",
		"/system/modules":      "ModulesView",
		"/system/dictionaries": "DictionariesView",
		"/system/settings":     "SettingsView",
		"/system/audit":        "AuditLogsView",
	} {
		assertMenuComponent(t, profile.Menus, path, component)
	}

	requestJSON[any](t, app, http.MethodGet, "/api/v1/auth/profile", "", nil, http.StatusUnauthorized)

	operatorToken := login(t, app, "operator", "admin123")
	requestJSON[any](t, app, http.MethodPost, "/api/v1/system/users", operatorToken, map[string]any{
		"username": "blocked-user",
		"password": "pass12345",
		"status":   domain.UserStatusActive,
	}, http.StatusForbidden)

	role, err := app.store.CreateRole(domain.Role{
		Name:        "Dashboard Viewer",
		Code:        "dashboard-viewer",
		Description: "Dashboard only",
		Permissions: []string{domain.PermissionDashboardView},
	})
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	hash, err := security.HashPassword("viewer123")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if _, err := app.store.CreateUser(domain.User{
		Username:     "viewer",
		PasswordHash: hash,
		RoleIDs:      []uint64{role.ID},
		Status:       domain.UserStatusActive,
	}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	viewerToken := login(t, app, "viewer", "viewer123")
	requestJSON[any](t, app, http.MethodGet, "/api/v1/system/dictionaries", viewerToken, nil, http.StatusForbidden)
	viewerDictionary := requestJSON[domain.Dictionary](t, app, http.MethodGet, "/api/v1/system/dictionaries/code/user_status", viewerToken, nil, http.StatusOK)
	if viewerDictionary.Code != "user_status" || len(viewerDictionary.Items) == 0 {
		t.Fatalf("expected authenticated dictionary read by code, got %+v", viewerDictionary)
	}
	viewerProfile := requestJSON[struct {
		Menus []domain.Menu `json:"menus"`
	}](t, app, http.MethodGet, "/api/v1/auth/profile", viewerToken, nil, http.StatusOK)
	if !hasMenuPath(viewerProfile.Menus, "/dashboard") {
		t.Fatalf("expected viewer menus to include dashboard, got %+v", viewerProfile.Menus)
	}
	if hasMenuPath(viewerProfile.Menus, "/system/users") || hasMenuPath(viewerProfile.Menus, "/system") {
		t.Fatalf("expected viewer menus to exclude system paths, got %+v", viewerProfile.Menus)
	}
}

func TestAuthProfileNormalizesRolePermissions(t *testing.T) {
	app := newTestApp(t)

	role, err := app.store.CreateRole(domain.Role{
		Name:        "Spaced Dashboard Viewer",
		Code:        "spaced-dashboard-viewer",
		Description: "Dashboard permission with storage whitespace",
		Permissions: []string{
			"",
			" " + domain.PermissionDashboardView + " ",
			domain.PermissionDashboardView,
		},
	})
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	hash, err := security.HashPassword("viewer123")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if _, err := app.store.CreateUser(domain.User{
		Username:     "spaced-viewer",
		PasswordHash: hash,
		RoleIDs:      []uint64{role.ID},
		Status:       domain.UserStatusActive,
	}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	token := login(t, app, "spaced-viewer", "viewer123")
	requestJSON[domain.DashboardSummary](t, app, http.MethodGet, "/api/v1/dashboard/summary", token, nil, http.StatusOK)
	profile := requestJSON[struct {
		Permissions []string      `json:"permissions"`
		Menus       []domain.Menu `json:"menus"`
	}](t, app, http.MethodGet, "/api/v1/auth/profile", token, nil, http.StatusOK)

	if !sameStrings(profile.Permissions, []string{domain.PermissionDashboardView}) {
		t.Fatalf("expected normalized profile permissions, got %+v", profile.Permissions)
	}
	if !hasMenuPath(profile.Menus, "/dashboard") {
		t.Fatalf("expected dashboard menu from normalized permission, got %+v", profile.Menus)
	}
}

func TestAuthProfileCollapsesWildcardPermissions(t *testing.T) {
	app := newTestApp(t)

	roles, err := app.store.ListRoles()
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	var adminRoleID uint64
	for _, role := range roles {
		if strings.EqualFold(role.Code, "admin") {
			adminRoleID = role.ID
			break
		}
	}
	if adminRoleID == 0 {
		t.Fatalf("expected seeded admin role, got %+v", roles)
	}

	dashboardRole, err := app.store.CreateRole(domain.Role{
		Name:        "Dashboard Viewer",
		Code:        "dashboard-viewer-extra",
		Description: "Dashboard only",
		Permissions: []string{domain.PermissionDashboardView},
	})
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	hash, err := security.HashPassword("viewer123")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if _, err := app.store.CreateUser(domain.User{
		Username:     "wildcard-viewer",
		PasswordHash: hash,
		RoleIDs:      []uint64{dashboardRole.ID, adminRoleID},
		Status:       domain.UserStatusActive,
	}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	token := login(t, app, "wildcard-viewer", "viewer123")
	profile := requestJSON[struct {
		Permissions []string      `json:"permissions"`
		Menus       []domain.Menu `json:"menus"`
	}](t, app, http.MethodGet, "/api/v1/auth/profile", token, nil, http.StatusOK)

	if !sameStrings(profile.Permissions, []string{domain.PermissionAll}) {
		t.Fatalf("expected wildcard profile permissions only, got %+v", profile.Permissions)
	}
	if !hasMenuPath(profile.Menus, "/system/users") {
		t.Fatalf("expected wildcard menu access, got %+v", profile.Menus)
	}
}

func TestFailedLoginWritesAuditLog(t *testing.T) {
	app := newTestApp(t)

	requestJSON[any](t, app, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": "admin",
		"password": "wrong-password",
	}, http.StatusUnauthorized)

	token := login(t, app, "admin", "admin123")
	logPage := requestJSON[domain.PageResult[domain.AuditLog]](t, app, http.MethodGet, "/api/v1/system/audit-logs?action=login_failed&resource=auth&page=1&page_size=100", token, nil, http.StatusOK)
	if logPage.Total != 1 || len(logPage.Items) != 1 {
		t.Fatalf("expected one failed login audit log, got %+v", logPage)
	}
	log := logPage.Items[0]
	if log.Actor != "admin" || log.Action != "login_failed" || log.Resource != "auth" {
		t.Fatalf("unexpected failed login audit log: %+v", log)
	}
	if log.ResourceID != itoa(log.ActorID) {
		t.Fatalf("expected failed login audit resource_id to match actor id, got %+v", log)
	}
	if strings.Contains(log.Detail, "wrong-password") {
		t.Fatalf("audit log detail must not contain plaintext password: %+v", log)
	}
}

func TestAuthSelfServiceProfileAndPassword(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")

	updated := requestJSON[domain.PublicUser](t, app, http.MethodPut, "/api/v1/auth/profile", token, map[string]string{
		"nickname": "Root Admin",
		"email":    "root@gov2.local",
		"phone":    "+1 555 0101",
		"avatar":   "https://example.com/root.png",
	}, http.StatusOK)
	if updated.Username != "admin" || updated.Nickname != "Root Admin" || updated.Email != "root@gov2.local" {
		t.Fatalf("unexpected updated profile: %+v", updated)
	}
	if len(updated.RoleIDs) == 0 || updated.Status != domain.UserStatusActive {
		t.Fatalf("expected profile update to preserve roles and status, got %+v", updated)
	}

	profile := requestJSON[struct {
		User domain.PublicUser `json:"user"`
	}](t, app, http.MethodGet, "/api/v1/auth/profile", token, nil, http.StatusOK)
	if profile.User.Nickname != "Root Admin" || profile.User.Email != "root@gov2.local" {
		t.Fatalf("expected refreshed profile data, got %+v", profile.User)
	}

	for _, newPassword := range []string{"short", "        "} {
		passwordEnvelope := requestEnvelope[struct {
			Fields []service.FieldError `json:"fields"`
		}](t, app, http.MethodPut, "/api/v1/auth/password", token, map[string]string{
			"current_password": "admin123",
			"new_password":     newPassword,
		}, http.StatusBadRequest)
		if !hasValidationField(passwordEnvelope.Data.Fields, "new_password") {
			t.Fatalf("expected new_password validation field for %q, got %+v", newPassword, passwordEnvelope.Data.Fields)
		}
	}

	requestJSON[any](t, app, http.MethodPut, "/api/v1/auth/password", token, map[string]string{
		"current_password": "wrong-password",
		"new_password":     "new-admin123",
	}, http.StatusUnauthorized)
	requestJSON[any](t, app, http.MethodPut, "/api/v1/auth/password", token, map[string]string{
		"current_password": "admin123",
		"new_password":     "new-admin123",
	}, http.StatusOK)

	requestJSON[any](t, app, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": "admin",
		"password": "admin123",
	}, http.StatusUnauthorized)
	login(t, app, "admin", "new-admin123")

	logPage := requestJSON[domain.PageResult[domain.AuditLog]](t, app, http.MethodGet, "/api/v1/system/audit-logs?resource=auth&page=1&page_size=100", token, nil, http.StatusOK)
	for _, expected := range []struct {
		action   string
		resource string
	}{
		{action: "update", resource: "auth.profile"},
		{action: "password_failed", resource: "auth.password"},
		{action: "password", resource: "auth.password"},
	} {
		if !hasAuditLog(logPage.Items, expected.action, expected.resource) {
			t.Fatalf("expected audit log action=%s resource=%s, got %+v", expected.action, expected.resource, logPage.Items)
		}
	}
	if failed, ok := findAuditLog(logPage.Items, "password_failed", "auth.password"); !ok {
		t.Fatalf("expected failed password change audit log, got %+v", logPage.Items)
	} else if strings.Contains(failed.Detail, "wrong-password") {
		t.Fatalf("audit log detail must not contain plaintext password: %+v", failed)
	} else if failed.ResourceID != itoa(profile.User.ID) {
		t.Fatalf("expected failed password audit resource_id to match user id, got %+v", failed)
	}
}

func TestAuthProfileUpdateConflictsOnDuplicateContact(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")

	hash, err := security.HashPassword("profile123")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if _, err := app.store.CreateUser(domain.User{
		Username:     "profile-contact-owner",
		Email:        "profile-conflict@example.test",
		Phone:        "15500002001",
		PasswordHash: hash,
		Status:       domain.UserStatusActive,
	}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	assertRecordConflict(t, app, http.MethodPut, "/api/v1/auth/profile", token, map[string]string{
		"nickname": "Root Admin",
		"email":    "PROFILE-CONFLICT@EXAMPLE.TEST",
		"phone":    "15500002002",
		"avatar":   "https://example.com/root.png",
	})
	assertRecordConflict(t, app, http.MethodPut, "/api/v1/auth/profile", token, map[string]string{
		"nickname": "Root Admin",
		"email":    "root-unique@example.test",
		"phone":    "15500002001",
		"avatar":   "https://example.com/root.png",
	})
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	cfg := testConfig()
	cfg.Security.AllowedOrigins = []string{"https://admin.gov2.local"}
	app := newTestAppWithConfig(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Origin", "https://admin.gov2.local")
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://admin.gov2.local" {
		t.Fatalf("expected configured CORS origin, got %q", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("expected Vary=Origin, got %q", got)
	}
	assertSecurityHeaders(t, rec)
}

func TestCORSRejectsDisallowedPreflight(t *testing.T) {
	cfg := testConfig()
	cfg.Security.AllowedOrigins = []string{"https://admin.gov2.local"}
	app := newTestAppWithConfig(t, cfg)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "https://blocked.gov2.local")
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected disallowed origin to be omitted, got %q", got)
	}
	assertSecurityHeaders(t, rec)
}

func TestCORSAllowsSupportedPreflight(t *testing.T) {
	cfg := testConfig()
	cfg.Security.AllowedOrigins = []string{"https://admin.gov2.local"}
	app := newTestAppWithConfig(t, cfg)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "https://admin.gov2.local")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, X-Request-ID")
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://admin.gov2.local" {
		t.Fatalf("expected configured CORS origin, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET,POST,PUT,PATCH,DELETE,OPTIONS" {
		t.Fatalf("unexpected allowed methods: %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Authorization,Content-Type,X-Request-ID" {
		t.Fatalf("unexpected allowed headers: %q", got)
	}
	assertSecurityHeaders(t, rec)
}

func TestCORSRejectsUnsupportedPreflightMethod(t *testing.T) {
	cfg := testConfig()
	cfg.Security.AllowedOrigins = []string{"https://admin.gov2.local"}
	app := newTestAppWithConfig(t, cfg)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "https://admin.gov2.local")
	req.Header.Set("Access-Control-Request-Method", http.MethodTrace)
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertSecurityHeaders(t, rec)
}

func TestCORSRejectsUnsupportedPreflightHeader(t *testing.T) {
	cfg := testConfig()
	cfg.Security.AllowedOrigins = []string{"https://admin.gov2.local"}
	app := newTestAppWithConfig(t, cfg)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "https://admin.gov2.local")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, X-Unsafe-Header")
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertSecurityHeaders(t, rec)
}

func TestSecurityHeadersApplyToPublicResponses(t *testing.T) {
	app := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertSecurityHeaders(t, rec)
}

func TestRequestIDPreservesValidIncomingValue(t *testing.T) {
	app := newTestApp(t)
	const requestID = "trace-123_ABC:segment.9"

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("X-Request-ID", requestID)
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Request-ID"); got != requestID {
		t.Fatalf("expected incoming request id %q to be preserved, got %q", requestID, got)
	}
	envelope := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if envelope.RequestID != requestID {
		t.Fatalf("expected response envelope request_id %q, got %+v", requestID, envelope)
	}
}

func TestRequestIDReplacesInvalidIncomingValue(t *testing.T) {
	app := newTestApp(t)
	invalid := strings.Repeat("x", 129)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("X-Request-ID", invalid)
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	requestID := rec.Header().Get("X-Request-ID")
	if requestID == "" || requestID == invalid {
		t.Fatalf("expected invalid request id to be replaced, got %q", requestID)
	}
	envelope := decodeEnvelope[map[string]any](t, rec.Body.Bytes())
	if envelope.RequestID != requestID {
		t.Fatalf("expected envelope request_id to match header %q, got %+v", requestID, envelope)
	}
}

func TestReadyEndpointReportsStoreStatus(t *testing.T) {
	app := newTestApp(t)

	ready := requestJSON[struct {
		Status string `json:"status"`
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"checks"`
	}](t, app, http.MethodGet, "/api/v1/ready", "", nil, http.StatusOK)
	if ready.Status != "ok" || len(ready.Checks) != 1 || ready.Checks[0].Name != "store" || ready.Checks[0].Status != "ok" {
		t.Fatalf("unexpected readiness response: %+v", ready)
	}
}

func TestReadyEndpointReportsMissingStore(t *testing.T) {
	api := httpapi.New(httpapi.Options{
		Config: testConfig(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil)
	rec := httptest.NewRecorder()
	api.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d body=%s", rec.Code, rec.Body.String())
	}
	envelope := decodeEnvelope[struct {
		Status string `json:"status"`
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"checks"`
	}](t, rec.Body.Bytes())
	if envelope.Code != http.StatusServiceUnavailable || envelope.Data.Status != "error" {
		t.Fatalf("unexpected readiness envelope: %+v", envelope)
	}
	if len(envelope.Data.Checks) != 1 || envelope.Data.Checks[0].Name != "store" || envelope.Data.Checks[0].Status != "error" || envelope.Data.Checks[0].Error == "" {
		t.Fatalf("unexpected readiness checks: %+v", envelope.Data.Checks)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	app := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected health status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/system/users/123", nil)
	rec = httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected protected route status 401, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	rec = httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected unknown API route status 404, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	rec = httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("expected text/plain content type, got %q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"gov2_up 1",
		"gov2_uptime_seconds",
		"gov2_runtime_goroutines",
		"gov2_runtime_memory_bytes",
		"gov2_runtime_gc_total",
		`gov2_http_requests_total{method="GET",route="/api/v1/health",status="200"} 1`,
		`gov2_http_requests_total{method="GET",route="/api/v1/system/users/{id}",status="401"} 1`,
		`gov2_http_requests_total{method="GET",route="unknown",status="404"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected metrics body to contain %q, got %s", want, body)
		}
	}
}

func TestStaticHandlerServesOnlyStaticRoot(t *testing.T) {
	parent := t.TempDir()
	staticDir := filepath.Join(parent, "dist")
	if err := os.Mkdir(staticDir, 0o755); err != nil {
		t.Fatalf("create static dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("spa index"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "asset.txt"), []byte("asset body"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	if err := os.WriteFile(filepath.Join(parent, "outside.txt"), []byte("outside secret"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	cfg := testConfig()
	cfg.Web.StaticDir = staticDir
	app := newTestAppWithConfig(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "/asset.txt", nil)
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "asset body" {
		t.Fatalf("expected static asset response, status=%d body=%q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec = httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "spa index" {
		t.Fatalf("expected SPA fallback response, status=%d body=%q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/../outside.txt", nil)
	rec = httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "outside secret") {
		t.Fatalf("static handler served a file outside the static root: %q", rec.Body.String())
	}

	for _, path := range []string{"/api", "/api/missing"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		rec = httptest.NewRecorder()
		app.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: expected API 404, got %d body=%q", path, rec.Code, rec.Body.String())
		}
		envelope := decodeEnvelope[any](t, rec.Body.Bytes())
		if envelope.Message != "api route not found" {
			t.Fatalf("%s: expected API 404 envelope, got %+v", path, envelope)
		}
	}
}

func TestExtensionRoutesUseGlobalMiddlewareAndPermissions(t *testing.T) {
	app := newTestAppWithRoutes(t, []httpapi.Route{
		{
			Method: http.MethodGet,
			Path:   "/api/v1/ext/public",
			Public: true,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true}`))
			}),
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/v1/ext/users",
			Permission: domain.PermissionSystemUserCreate,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeExtensionJSON(w, map[string]bool{"ok": true})
			}),
		},
		{
			Method:     http.MethodGet,
			Path:       "/api/v1/ext/public-with-permission",
			Public:     true,
			Permission: domain.PermissionSystemUserCreate,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeExtensionJSON(w, map[string]bool{"ok": true})
			}),
		},
		{
			Method: http.MethodGet,
			Path:   "/api/v1/ext/auth-only",
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeExtensionJSON(w, map[string]bool{"ok": true})
			}),
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ext/public", nil)
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected public extension route status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatalf("expected extension route to receive global request id middleware")
	}
	assertSecurityHeaders(t, rec)

	requestJSON[any](t, app, http.MethodGet, "/api/v1/ext/auth-only", "", nil, http.StatusUnauthorized)
	operatorToken := login(t, app, "operator", "admin123")
	requestJSON[map[string]bool](t, app, http.MethodGet, "/api/v1/ext/auth-only", operatorToken, nil, http.StatusOK)

	requestJSON[any](t, app, http.MethodGet, "/api/v1/ext/users", "", nil, http.StatusUnauthorized)
	requestJSON[any](t, app, http.MethodGet, "/api/v1/ext/users", operatorToken, nil, http.StatusForbidden)
	adminToken := login(t, app, "admin", "admin123")
	requestJSON[map[string]bool](t, app, http.MethodGet, "/api/v1/ext/users", adminToken, nil, http.StatusOK)

	requestJSON[any](t, app, http.MethodGet, "/api/v1/ext/public-with-permission", "", nil, http.StatusUnauthorized)
	requestJSON[any](t, app, http.MethodGet, "/api/v1/ext/public-with-permission", operatorToken, nil, http.StatusForbidden)
	requestJSON[map[string]bool](t, app, http.MethodGet, "/api/v1/ext/public-with-permission", adminToken, nil, http.StatusOK)
}

func TestAPIRouterWorksWithNilLogger(t *testing.T) {
	store := memory.NewStore()
	tokens := security.NewTokenManager("test-secret", time.Hour, "gov2-test")
	services := service.NewRegistry(store, tokens)
	api := httpapi.New(httpapi.Options{
		Config:   testConfig(),
		Store:    store,
		Services: services,
		Tokens:   tokens,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	api.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected nil-logger API health status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatalf("expected nil-logger API to still run global middleware")
	}
}

func TestExtensionRoutePanicIsRecoveredWithNilLogger(t *testing.T) {
	store := memory.NewStore()
	tokens := security.NewTokenManager("test-secret", time.Hour, "gov2-test")
	services := service.NewRegistry(store, tokens)
	api := httpapi.New(httpapi.Options{
		Config:   testConfig(),
		Store:    store,
		Services: services,
		Tokens:   tokens,
		Routes: []httpapi.Route{
			{
				Method: http.MethodGet,
				Path:   "/api/v1/ext/panic",
				Public: true,
				Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					panic("extension route panic")
				}),
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ext/panic", nil)
	rec := httptest.NewRecorder()
	api.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected recovered panic status 500, got %d body=%s", rec.Code, rec.Body.String())
	}
	envelope := decodeEnvelope[any](t, rec.Body.Bytes())
	if envelope.Message != "internal server error" || envelope.RequestID == "" {
		t.Fatalf("expected recovered panic envelope with request id, got %+v", envelope)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	metricsRec := httptest.NewRecorder()
	api.Router().ServeHTTP(metricsRec, metricsReq)
	if body := metricsRec.Body.String(); !strings.Contains(body, `gov2_http_requests_total{method="GET",route="/api/v1/ext/panic",status="500"} 1`) {
		t.Fatalf("expected recovered panic request metric, got %s", body)
	}
}

func TestSystemModulesExposeExtensionMetadata(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")

	modules := requestJSON[[]module.Module](t, app, http.MethodGet, "/api/v1/system/modules", token, nil, http.StatusOK)
	dashboard, ok := findModule(modules, "dashboard")
	if !ok {
		t.Fatalf("expected dashboard module, got %+v", modules)
	}
	if len(dashboard.Permissions) == 0 || len(dashboard.Menus) == 0 || len(dashboard.Routes) == 0 {
		t.Fatalf("expected dashboard module extension metadata, got %+v", dashboard)
	}

	system, ok := findModule(modules, "system")
	if !ok {
		t.Fatalf("expected system module, got %+v", modules)
	}
	if len(system.Permissions) == 0 || len(system.Menus) == 0 || len(system.Routes) == 0 || len(system.Migrations) == 0 {
		t.Fatalf("expected system module extension metadata, got %+v", system)
	}
}

func TestSystemPermissionsUseRegisteredModuleCatalog(t *testing.T) {
	registry, err := module.NewValidatedRegistry(append(module.BuiltInModules(), module.Module{
		Name:        "inventory",
		Title:       "Inventory",
		Version:     "0.1.0",
		Description: "Inventory operations",
		Permissions: []domain.PermissionDefinition{
			{Code: "inventory:item:list", Name: "List inventory items", Module: "inventory"},
		},
		Backend: []module.BackendRoute{
			{Name: "inventory-items-list", Method: http.MethodGet, Path: "/api/v1/inventory/items", Permission: "inventory:item:list", Summary: "List inventory items"},
		},
		Menus: []module.MenuEntry{
			{Name: "inventory", Title: "Inventory", Path: "/inventory", Component: "InventoryView", Permission: "inventory:item:list"},
		},
		Routes: []module.FrontendRoute{
			{Name: "inventory", Path: "/inventory", Component: "InventoryView", Title: "Inventory", Permission: "inventory:item:list"},
		},
	})...)
	if err != nil {
		t.Fatalf("build module registry: %v", err)
	}
	app := newTestAppWithModules(t, registry)
	token := login(t, app, "admin", "admin123")

	permissions := requestJSON[[]domain.PermissionDefinition](t, app, http.MethodGet, "/api/v1/system/permissions", token, nil, http.StatusOK)
	if !hasPermissionDefinition(permissions, "inventory:item:list") {
		t.Fatalf("expected registered module permission in permission catalog, got %+v", permissions)
	}

	role := requestJSON[domain.Role](t, app, http.MethodPost, "/api/v1/system/roles", token, map[string]any{
		"name":        "Inventory Viewer",
		"code":        "inventory-viewer",
		"permissions": []string{"inventory:item:list"},
	}, http.StatusCreated)
	if !contains(role.Permissions, "inventory:item:list") {
		t.Fatalf("expected role to include registered module permission, got %+v", role)
	}

	type validationData struct {
		Fields []service.FieldError `json:"fields"`
	}
	envelope := requestEnvelope[validationData](t, app, http.MethodPost, "/api/v1/system/roles", token, map[string]any{
		"name":        "Unknown Permission",
		"code":        "unknown-permission",
		"permissions": []string{"unknown:item:list"},
	}, http.StatusBadRequest)
	if !hasValidationField(envelope.Data.Fields, "permissions") {
		t.Fatalf("expected permissions validation field, got %+v", envelope.Data.Fields)
	}
}

func TestAppConfigEndpointUsesSiteTitleSetting(t *testing.T) {
	app := newTestApp(t)

	config := requestJSON[struct {
		Name        string `json:"name"`
		Title       string `json:"title"`
		Environment string `json:"environment"`
	}](t, app, http.MethodGet, "/api/v1/app/config", "", nil, http.StatusOK)
	if config.Name != "GOV2" || config.Title != "GOV2" || config.Environment != "test" {
		t.Fatalf("unexpected app config: %+v", config)
	}

	token := login(t, app, "admin", "admin123")
	settings := requestJSON[[]domain.Setting](t, app, http.MethodGet, "/api/v1/system/settings", token, nil, http.StatusOK)
	var siteTitleID uint64
	for _, setting := range settings {
		if strings.EqualFold(setting.Key, "site.title") {
			siteTitleID = setting.ID
			break
		}
	}
	if siteTitleID == 0 {
		t.Fatalf("expected seeded site.title setting, got %+v", settings)
	}

	requestJSON[domain.Setting](t, app, http.MethodPut, "/api/v1/system/settings/"+itoa(siteTitleID), token, map[string]any{
		"key":         "site.title",
		"value":       "GOV2 Console",
		"description": "Displayed application title",
	}, http.StatusOK)
	updated := requestJSON[struct {
		Title string `json:"title"`
	}](t, app, http.MethodGet, "/api/v1/app/config", "", nil, http.StatusOK)
	if updated.Title != "GOV2 Console" {
		t.Fatalf("expected updated public site title, got %+v", updated)
	}
}

func TestUserValidationErrorsIncludeFields(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")

	envelope := requestEnvelope[struct {
		Fields []service.FieldError `json:"fields"`
	}](t, app, http.MethodPost, "/api/v1/system/users", token, map[string]any{
		"username": "",
		"password": "",
		"status":   domain.UserStatusActive,
	}, http.StatusBadRequest)
	if envelope.Message != "invalid input" {
		t.Fatalf("expected invalid input message, got %+v", envelope)
	}
	for _, field := range []string{"username", "password"} {
		if !hasValidationField(envelope.Data.Fields, field) {
			t.Fatalf("expected validation field %q, got %+v", field, envelope.Data.Fields)
		}
	}

	whitespacePasswordEnvelope := requestEnvelope[struct {
		Fields []service.FieldError `json:"fields"`
	}](t, app, http.MethodPost, "/api/v1/system/users", token, map[string]any{
		"username": "whitespace-password-user",
		"password": "        ",
		"status":   domain.UserStatusActive,
	}, http.StatusBadRequest)
	if !hasValidationField(whitespacePasswordEnvelope.Data.Fields, "password") {
		t.Fatalf("expected password validation field, got %+v", whitespacePasswordEnvelope.Data.Fields)
	}

	roles, err := app.store.ListRoles()
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	if len(roles) == 0 {
		t.Fatal("expected seeded roles")
	}
	user := requestJSON[domain.PublicUser](t, app, http.MethodPost, "/api/v1/system/users", token, map[string]any{
		"username": "validation-user",
		"password": "pass12345",
		"role_ids": []uint64{roles[0].ID},
		"status":   domain.UserStatusActive,
	}, http.StatusCreated)
	statusEnvelope := requestEnvelope[struct {
		Fields []service.FieldError `json:"fields"`
	}](t, app, http.MethodPatch, "/api/v1/system/users/"+itoa(user.ID)+"/status", token, map[string]string{
		"status": "archived",
	}, http.StatusBadRequest)
	if !hasValidationField(statusEnvelope.Data.Fields, "status") {
		t.Fatalf("expected status validation field, got %+v", statusEnvelope.Data.Fields)
	}
	normalizedStatus := requestJSON[domain.PublicUser](t, app, http.MethodPatch, "/api/v1/system/users/"+itoa(user.ID)+"/status", token, map[string]string{
		"status": " \t" + domain.UserStatusDisabled + "\n",
	}, http.StatusOK)
	if normalizedStatus.Status != domain.UserStatusDisabled {
		t.Fatalf("expected normalized disabled status, got %+v", normalizedStatus)
	}
	for _, password := range []string{"short", "        "} {
		passwordEnvelope := requestEnvelope[struct {
			Fields []service.FieldError `json:"fields"`
		}](t, app, http.MethodPut, "/api/v1/system/users/"+itoa(user.ID), token, map[string]any{
			"username": user.Username,
			"password": password,
			"role_ids": user.RoleIDs,
			"status":   user.Status,
		}, http.StatusBadRequest)
		if !hasValidationField(passwordEnvelope.Data.Fields, "password") {
			t.Fatalf("expected password validation field for %q, got %+v", password, passwordEnvelope.Data.Fields)
		}
	}
}

func TestUserWriteProtectsLastActiveAdministrator(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")

	admin, err := app.store.FindUserByUsername("admin")
	if err != nil {
		t.Fatalf("FindUserByUsername(admin) error = %v", err)
	}

	statusEnvelope := requestEnvelope[any](t, app, http.MethodPatch, "/api/v1/system/users/"+itoa(admin.ID)+"/status", token, map[string]string{
		"status": domain.UserStatusDisabled,
	}, http.StatusConflict)
	if statusEnvelope.Message != "last active administrator cannot be removed" {
		t.Fatalf("expected last admin conflict message, got %+v", statusEnvelope)
	}

	updateEnvelope := requestEnvelope[any](t, app, http.MethodPut, "/api/v1/system/users/"+itoa(admin.ID), token, map[string]any{
		"username": admin.Username,
		"nickname": admin.Nickname,
		"email":    admin.Email,
		"phone":    admin.Phone,
		"avatar":   admin.Avatar,
		"role_ids": []uint64{},
		"status":   domain.UserStatusActive,
	}, http.StatusConflict)
	if updateEnvelope.Message != "last active administrator cannot be removed" {
		t.Fatalf("expected last admin conflict message, got %+v", updateEnvelope)
	}

	deleteEnvelope := requestEnvelope[any](t, app, http.MethodDelete, "/api/v1/system/users/"+itoa(admin.ID), token, nil, http.StatusConflict)
	if deleteEnvelope.Message != "last active administrator cannot be removed" {
		t.Fatalf("expected last admin conflict message, got %+v", deleteEnvelope)
	}
}

func TestSystemValidationErrorsIncludeFields(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")

	type validationData struct {
		Fields []service.FieldError `json:"fields"`
	}
	tests := []struct {
		name   string
		method string
		path   string
		body   any
		field  string
	}{
		{
			name:   "role",
			method: http.MethodPost,
			path:   "/api/v1/system/roles",
			body: map[string]any{
				"name": "",
				"code": "",
			},
			field: "name",
		},
		{
			name:   "menu",
			method: http.MethodPost,
			path:   "/api/v1/system/menus",
			body: map[string]any{
				"title": "Reports",
				"name":  "reports",
				"path":  "reports",
			},
			field: "path",
		},
		{
			name:   "menu permission",
			method: http.MethodPost,
			path:   "/api/v1/system/menus",
			body: map[string]any{
				"title":      "Reports",
				"name":       "reports",
				"path":       "/reports",
				"permission": "reports:list",
			},
			field: "permission",
		},
		{
			name:   "dictionary",
			method: http.MethodPost,
			path:   "/api/v1/system/dictionaries",
			body: map[string]any{
				"code": "duplicate_items",
				"name": "Duplicate Items",
				"items": []map[string]any{
					{"label": "Active", "value": "active"},
					{"label": "Active Again", "value": "ACTIVE"},
				},
			},
			field: "items",
		},
		{
			name:   "setting",
			method: http.MethodPost,
			path:   "/api/v1/system/settings",
			body: map[string]any{
				"key":   "",
				"value": true,
			},
			field: "key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := requestEnvelope[validationData](t, app, tt.method, tt.path, token, tt.body, http.StatusBadRequest)
			if envelope.Message != "invalid input" {
				t.Fatalf("expected invalid input message, got %+v", envelope)
			}
			if !hasValidationField(envelope.Data.Fields, tt.field) {
				t.Fatalf("expected validation field %q, got %+v", tt.field, envelope.Data.Fields)
			}
		})
	}
}

func TestSystemWriteEndpoints(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")

	roles, err := app.store.ListRoles()
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	if len(roles) == 0 {
		t.Fatal("expected seeded roles")
	}

	user := requestJSON[domain.PublicUser](t, app, http.MethodPost, "/api/v1/system/users", token, map[string]any{
		"username": "api-user",
		"password": "pass12345",
		"nickname": "API User",
		"role_ids": []uint64{roles[0].ID},
		"status":   domain.UserStatusActive,
	}, http.StatusCreated)
	if user.ID == 0 {
		t.Fatalf("expected created user id, got %+v", user)
	}

	updatedUser := requestJSON[domain.PublicUser](t, app, http.MethodPut, "/api/v1/system/users/"+itoa(user.ID), token, map[string]any{
		"username": "api-user",
		"nickname": "API User Updated",
		"role_ids": []uint64{roles[0].ID},
		"status":   domain.UserStatusActive,
	}, http.StatusOK)
	if updatedUser.Nickname != "API User Updated" {
		t.Fatalf("expected updated nickname, got %+v", updatedUser)
	}
	requestJSON[domain.PublicUser](t, app, http.MethodPatch, "/api/v1/system/users/"+itoa(user.ID)+"/status", token, map[string]string{
		"status": domain.UserStatusDisabled,
	}, http.StatusOK)
	requestJSON[any](t, app, http.MethodDelete, "/api/v1/system/users/"+itoa(user.ID), token, nil, http.StatusOK)

	menu := requestJSON[domain.Menu](t, app, http.MethodPost, "/api/v1/system/menus", token, map[string]any{
		"title":      "Reports",
		"name":       "reports",
		"path":       "/reports",
		"component":  "ReportsView",
		"permission": domain.PermissionDashboardView,
		"sort":       90,
	}, http.StatusCreated)
	if menu.ID == 0 {
		t.Fatalf("expected created menu id, got %+v", menu)
	}
	updatedMenu := requestJSON[domain.Menu](t, app, http.MethodPut, "/api/v1/system/menus/"+itoa(menu.ID), token, map[string]any{
		"title":      "Reports Updated",
		"name":       "reports",
		"path":       "/reports",
		"component":  "ReportsView",
		"permission": domain.PermissionDashboardView,
		"sort":       91,
	}, http.StatusOK)
	if updatedMenu.Title != "Reports Updated" {
		t.Fatalf("expected updated menu title, got %+v", updatedMenu)
	}
	requestJSON[any](t, app, http.MethodDelete, "/api/v1/system/menus/"+itoa(menu.ID), token, nil, http.StatusOK)

	dictionary := requestJSON[domain.Dictionary](t, app, http.MethodPost, "/api/v1/system/dictionaries", token, map[string]any{
		"code": "api_status",
		"name": "API Status",
		"items": []map[string]any{
			{"label": "Open", "value": "open", "sort": 1},
		},
	}, http.StatusCreated)
	if dictionary.ID == 0 || len(dictionary.Items) != 1 {
		t.Fatalf("expected created dictionary with items, got %+v", dictionary)
	}
	updatedDictionary := requestJSON[domain.Dictionary](t, app, http.MethodPut, "/api/v1/system/dictionaries/"+itoa(dictionary.ID), token, map[string]any{
		"code": "api_status",
		"name": "API Status Updated",
		"items": []map[string]any{
			{"label": "Closed", "value": "closed", "sort": 2},
		},
	}, http.StatusOK)
	if updatedDictionary.Name != "API Status Updated" || updatedDictionary.Items[0].Value != "closed" {
		t.Fatalf("expected updated dictionary, got %+v", updatedDictionary)
	}
	requestJSON[any](t, app, http.MethodDelete, "/api/v1/system/dictionaries/"+itoa(dictionary.ID), token, nil, http.StatusOK)

	setting := requestJSON[domain.Setting](t, app, http.MethodPost, "/api/v1/system/settings", token, map[string]any{
		"key":         "api.setting",
		"value":       map[string]bool{"enabled": true},
		"description": "API setting",
	}, http.StatusCreated)
	if setting.ID == 0 {
		t.Fatalf("expected created setting id, got %+v", setting)
	}
	updatedSetting := requestJSON[domain.Setting](t, app, http.MethodPut, "/api/v1/system/settings/"+itoa(setting.ID), token, map[string]any{
		"key":         "api.setting",
		"value":       map[string]bool{"enabled": false},
		"description": "Updated API setting",
	}, http.StatusOK)
	if updatedSetting.Description != "Updated API setting" {
		t.Fatalf("expected updated setting, got %+v", updatedSetting)
	}
	requestJSON[any](t, app, http.MethodDelete, "/api/v1/system/settings/"+itoa(setting.ID), token, nil, http.StatusOK)

	logPage := requestJSON[domain.PageResult[domain.AuditLog]](t, app, http.MethodGet, "/api/v1/system/audit-logs?page=1&page_size=100", token, nil, http.StatusOK)
	for _, expected := range []struct {
		action   string
		resource string
	}{
		{action: "create", resource: "system.user"},
		{action: "status", resource: "system.user"},
		{action: "delete", resource: "system.user"},
		{action: "create", resource: "system.menu"},
		{action: "update", resource: "system.dictionary"},
		{action: "delete", resource: "system.setting"},
	} {
		if !hasAuditLog(logPage.Items, expected.action, expected.resource) {
			t.Fatalf("expected audit log action=%s resource=%s, got %+v", expected.action, expected.resource, logPage.Items)
		}
	}

	userCreateLogs := requestJSON[domain.PageResult[domain.AuditLog]](t, app, http.MethodGet, "/api/v1/system/audit-logs?action=create&resource=system.user&resource_id="+itoa(user.ID)+"&page=1&page_size=100", token, nil, http.StatusOK)
	if userCreateLogs.Total != 1 || len(userCreateLogs.Items) != 1 {
		t.Fatalf("expected one filtered user create audit log, got %+v", userCreateLogs)
	}
	if userCreateLogs.Items[0].Action != "create" || userCreateLogs.Items[0].Resource != "system.user" || userCreateLogs.Items[0].ResourceID != itoa(user.ID) {
		t.Fatalf("unexpected filtered user create audit log: %+v", userCreateLogs.Items[0])
	}

	settingDeleteLogs := requestJSON[domain.PageResult[domain.AuditLog]](t, app, http.MethodGet, "/api/v1/system/audit-logs?keyword=deleted%20setting&resource_id="+itoa(setting.ID)+"&page=1&page_size=100", token, nil, http.StatusOK)
	if settingDeleteLogs.Total != 1 || len(settingDeleteLogs.Items) != 1 {
		t.Fatalf("expected one filtered setting delete audit log, got %+v", settingDeleteLogs)
	}
	if settingDeleteLogs.Items[0].Action != "delete" || settingDeleteLogs.Items[0].Resource != "system.setting" || settingDeleteLogs.Items[0].ResourceID != itoa(setting.ID) {
		t.Fatalf("unexpected filtered setting delete audit log: %+v", settingDeleteLogs.Items[0])
	}
}

func TestSystemWriteConflictResponses(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")

	roles, err := app.store.ListRoles()
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	if len(roles) == 0 {
		t.Fatal("expected seeded roles")
	}

	userOne := requestJSON[domain.PublicUser](t, app, http.MethodPost, "/api/v1/system/users", token, map[string]any{
		"username": "conflict-user-one",
		"password": "pass12345",
		"email":    "conflict-one@example.test",
		"phone":    "15500001001",
		"role_ids": []uint64{roles[0].ID},
		"status":   domain.UserStatusActive,
	}, http.StatusCreated)
	userTwo := requestJSON[domain.PublicUser](t, app, http.MethodPost, "/api/v1/system/users", token, map[string]any{
		"username": "conflict-user-two",
		"password": "pass12345",
		"email":    "conflict-two@example.test",
		"phone":    "15500001002",
		"role_ids": []uint64{roles[0].ID},
		"status":   domain.UserStatusActive,
	}, http.StatusCreated)
	assertRecordConflict(t, app, http.MethodPost, "/api/v1/system/users", token, map[string]any{
		"username": "CONFLICT-USER-ONE",
		"password": "pass12345",
		"status":   domain.UserStatusActive,
	})
	assertRecordConflict(t, app, http.MethodPost, "/api/v1/system/users", token, map[string]any{
		"username": "conflict-user-email",
		"password": "pass12345",
		"email":    "CONFLICT-ONE@EXAMPLE.TEST",
		"status":   domain.UserStatusActive,
	})
	assertRecordConflict(t, app, http.MethodPost, "/api/v1/system/users", token, map[string]any{
		"username": "conflict-user-phone",
		"password": "pass12345",
		"phone":    "15500001001",
		"status":   domain.UserStatusActive,
	})
	assertRecordConflict(t, app, http.MethodPut, "/api/v1/system/users/"+itoa(userTwo.ID), token, map[string]any{
		"username": "CONFLICT-USER-ONE",
		"email":    userTwo.Email,
		"phone":    userTwo.Phone,
		"role_ids": userTwo.RoleIDs,
		"status":   domain.UserStatusActive,
	})
	assertRecordConflict(t, app, http.MethodPut, "/api/v1/system/users/"+itoa(userTwo.ID), token, map[string]any{
		"username": userTwo.Username,
		"email":    "CONFLICT-ONE@EXAMPLE.TEST",
		"phone":    userTwo.Phone,
		"role_ids": userTwo.RoleIDs,
		"status":   domain.UserStatusActive,
	})
	assertRecordConflict(t, app, http.MethodPut, "/api/v1/system/users/"+itoa(userTwo.ID), token, map[string]any{
		"username": userTwo.Username,
		"email":    userTwo.Email,
		"phone":    "15500001001",
		"role_ids": userTwo.RoleIDs,
		"status":   domain.UserStatusActive,
	})
	if userOne.ID == 0 {
		t.Fatalf("expected first conflict user id, got %+v", userOne)
	}

	roleOne := requestJSON[domain.Role](t, app, http.MethodPost, "/api/v1/system/roles", token, map[string]any{
		"name": "Conflict Role One",
		"code": "conflict-role-one",
	}, http.StatusCreated)
	roleTwo := requestJSON[domain.Role](t, app, http.MethodPost, "/api/v1/system/roles", token, map[string]any{
		"name": "Conflict Role Two",
		"code": "conflict-role-two",
	}, http.StatusCreated)
	assertRecordConflict(t, app, http.MethodPost, "/api/v1/system/roles", token, map[string]any{
		"name": "Duplicate Conflict Role",
		"code": "CONFLICT-ROLE-ONE",
	})
	assertRecordConflict(t, app, http.MethodPut, "/api/v1/system/roles/"+itoa(roleTwo.ID), token, map[string]any{
		"name": "Conflict Role Two",
		"code": "CONFLICT-ROLE-ONE",
	})
	requestJSON[domain.PublicUser](t, app, http.MethodPost, "/api/v1/system/users", token, map[string]any{
		"username": "conflict-role-user",
		"password": "pass12345",
		"role_ids": []uint64{roleOne.ID},
		"status":   domain.UserStatusActive,
	}, http.StatusCreated)
	assertRecordConflict(t, app, http.MethodDelete, "/api/v1/system/roles/"+itoa(roleOne.ID), token, nil)

	menuOne := requestJSON[domain.Menu](t, app, http.MethodPost, "/api/v1/system/menus", token, map[string]any{
		"title":     "Conflict Menu One",
		"name":      "conflict-menu-one",
		"path":      "/conflict-menu-one",
		"component": "ConflictMenuOneView",
	}, http.StatusCreated)
	menuTwo := requestJSON[domain.Menu](t, app, http.MethodPost, "/api/v1/system/menus", token, map[string]any{
		"title":     "Conflict Menu Two",
		"name":      "conflict-menu-two",
		"path":      "/conflict-menu-two",
		"component": "ConflictMenuTwoView",
	}, http.StatusCreated)
	assertRecordConflict(t, app, http.MethodPost, "/api/v1/system/menus", token, map[string]any{
		"title":     "Duplicate Conflict Menu",
		"name":      "CONFLICT-MENU-ONE",
		"path":      "/conflict-menu-duplicate",
		"component": "ConflictMenuDuplicateView",
	})
	assertRecordConflict(t, app, http.MethodPut, "/api/v1/system/menus/"+itoa(menuTwo.ID), token, map[string]any{
		"title":     "Conflict Menu Two",
		"name":      "CONFLICT-MENU-ONE",
		"path":      "/conflict-menu-two",
		"component": "ConflictMenuTwoView",
	})
	requestJSON[domain.Menu](t, app, http.MethodPost, "/api/v1/system/menus", token, map[string]any{
		"parent_id": menuOne.ID,
		"title":     "Conflict Menu Child",
		"name":      "conflict-menu-child",
		"path":      "/conflict-menu-one/child",
		"component": "ConflictMenuChildView",
	}, http.StatusCreated)
	assertRecordConflict(t, app, http.MethodDelete, "/api/v1/system/menus/"+itoa(menuOne.ID), token, nil)

	dictionaryOne := requestJSON[domain.Dictionary](t, app, http.MethodPost, "/api/v1/system/dictionaries", token, map[string]any{
		"code": "conflict_dictionary_one",
		"name": "Conflict Dictionary One",
	}, http.StatusCreated)
	dictionaryTwo := requestJSON[domain.Dictionary](t, app, http.MethodPost, "/api/v1/system/dictionaries", token, map[string]any{
		"code": "conflict_dictionary_two",
		"name": "Conflict Dictionary Two",
	}, http.StatusCreated)
	assertRecordConflict(t, app, http.MethodPost, "/api/v1/system/dictionaries", token, map[string]any{
		"code": "CONFLICT_DICTIONARY_ONE",
		"name": "Duplicate Conflict Dictionary",
	})
	assertRecordConflict(t, app, http.MethodPut, "/api/v1/system/dictionaries/"+itoa(dictionaryTwo.ID), token, map[string]any{
		"code": "CONFLICT_DICTIONARY_ONE",
		"name": "Conflict Dictionary Two",
	})
	if dictionaryOne.ID == 0 {
		t.Fatalf("expected first conflict dictionary id, got %+v", dictionaryOne)
	}

	settingOne := requestJSON[domain.Setting](t, app, http.MethodPost, "/api/v1/system/settings", token, map[string]any{
		"key":   "conflict.setting.one",
		"value": true,
	}, http.StatusCreated)
	settingTwo := requestJSON[domain.Setting](t, app, http.MethodPost, "/api/v1/system/settings", token, map[string]any{
		"key":   "conflict.setting.two",
		"value": false,
	}, http.StatusCreated)
	assertRecordConflict(t, app, http.MethodPost, "/api/v1/system/settings", token, map[string]any{
		"key":   "CONFLICT.SETTING.ONE",
		"value": true,
	})
	assertRecordConflict(t, app, http.MethodPut, "/api/v1/system/settings/"+itoa(settingTwo.ID), token, map[string]any{
		"key":   "CONFLICT.SETTING.ONE",
		"value": false,
	})
	if settingOne.ID == 0 {
		t.Fatalf("expected first conflict setting id, got %+v", settingOne)
	}
}

func TestJSONRequestBodyLimit(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")
	body := `{"username":"` + strings.Repeat("x", 1<<20) + `","password":"pass12345"}`

	envelope := requestRawEnvelope[any](t, app, http.MethodPost, "/api/v1/system/users", token, body, http.StatusRequestEntityTooLarge)
	if envelope.Message != "request body too large" {
		t.Fatalf("expected request body too large message, got %+v", envelope)
	}
}

func TestMalformedJSONRequestBodyRemainsBadRequest(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")

	envelope := requestRawEnvelope[any](t, app, http.MethodPost, "/api/v1/system/users", token, `{"username":`, http.StatusBadRequest)
	if envelope.Message != "invalid request body" {
		t.Fatalf("expected invalid request body message, got %+v", envelope)
	}
}

func TestJSONRequestBodyRequiresJSONContentType(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")
	body := `{"username":"wrong-media","password":"pass12345"}`

	for _, tt := range []struct {
		name        string
		contentType string
	}{
		{name: "missing"},
		{name: "text", contentType: "text/plain"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			envelope := requestRawEnvelopeWithContentType[any](t, app, http.MethodPost, "/api/v1/system/users", token, tt.contentType, body, http.StatusUnsupportedMediaType)
			if envelope.Message != "unsupported media type" {
				t.Fatalf("expected unsupported media type message, got %+v", envelope)
			}
		})
	}
}

func TestJSONRequestBodyAllowsCompatibleContentTypes(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")

	for _, tt := range []struct {
		contentType string
		username    string
	}{
		{contentType: "application/json; charset=utf-8", username: "json-charset"},
		{contentType: "application/vnd.gov2+json", username: "json-structured"},
	} {
		t.Run(tt.contentType, func(t *testing.T) {
			body := `{"username":"` + tt.username + `","password":"pass12345"}`
			user := requestRawEnvelopeWithContentType[domain.PublicUser](t, app, http.MethodPost, "/api/v1/system/users", token, tt.contentType, body, http.StatusCreated).Data
			if user.Username != tt.username {
				t.Fatalf("expected created user from %s body, got %+v", tt.contentType, user)
			}
		})
	}
}

func TestTrailingJSONRequestBodyIsRejected(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")
	body := `{"username":"extra-json","password":"pass12345"}{"username":"second"}`

	envelope := requestRawEnvelope[any](t, app, http.MethodPost, "/api/v1/system/users", token, body, http.StatusBadRequest)
	if envelope.Message != "invalid request body" {
		t.Fatalf("expected invalid request body message, got %+v", envelope)
	}
}

func TestTrailingJSONWhitespaceIsAllowed(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")
	body := `{"username":"whitespace-json","password":"pass12345"}  ` + "\n\t"

	user := requestRawEnvelope[domain.PublicUser](t, app, http.MethodPost, "/api/v1/system/users", token, body, http.StatusCreated).Data
	if user.Username != "whitespace-json" {
		t.Fatalf("expected created user from body with trailing whitespace, got %+v", user)
	}
}

func TestSystemWriteEndpointErrors(t *testing.T) {
	app := newTestApp(t)
	token := login(t, app, "admin", "admin123")

	requestJSON[any](t, app, http.MethodPost, "/api/v1/system/menus", token, map[string]any{
		"title":     "Broken Reports",
		"name":      "broken-reports",
		"path":      "reports",
		"component": "ReportsView",
	}, http.StatusBadRequest)

	parent := requestJSON[domain.Menu](t, app, http.MethodPost, "/api/v1/system/menus", token, map[string]any{
		"title":     "Operations",
		"name":      "operations",
		"path":      "/operations",
		"component": "Layout",
	}, http.StatusCreated)
	child := requestJSON[domain.Menu](t, app, http.MethodPost, "/api/v1/system/menus", token, map[string]any{
		"parent_id":  parent.ID,
		"title":      "Operations Queue",
		"name":       "operations-queue",
		"path":       "/operations/queue",
		"component":  "OperationsQueueView",
		"permission": domain.PermissionDashboardView,
	}, http.StatusCreated)
	if parent.ID == 0 || child.ParentID != parent.ID {
		t.Fatalf("expected parent-child menus, parent=%+v child=%+v", parent, child)
	}
	requestJSON[any](t, app, http.MethodDelete, "/api/v1/system/menus/"+itoa(parent.ID), token, nil, http.StatusConflict)
	requestJSON[any](t, app, http.MethodPut, "/api/v1/system/menus/"+itoa(parent.ID), token, map[string]any{
		"parent_id": child.ID,
		"title":     "Operations",
		"name":      "operations",
		"path":      "/operations",
		"component": "Layout",
	}, http.StatusBadRequest)

	requestJSON[any](t, app, http.MethodPost, "/api/v1/system/dictionaries", token, map[string]any{
		"code": "duplicate_items",
		"name": "Duplicate Items",
		"items": []map[string]any{
			{"label": "Enabled", "value": "enabled", "sort": 1},
			{"label": "Enabled Again", "value": "ENABLED", "sort": 2},
		},
	}, http.StatusBadRequest)

	requestJSON[domain.Setting](t, app, http.MethodPost, "/api/v1/system/settings", token, map[string]any{
		"key":   "feature.conflict",
		"value": true,
	}, http.StatusCreated)
	requestJSON[any](t, app, http.MethodPost, "/api/v1/system/settings", token, map[string]any{
		"key":   "FEATURE.CONFLICT",
		"value": false,
	}, http.StatusConflict)
	requestJSON[any](t, app, http.MethodDelete, "/api/v1/system/settings/999999", token, nil, http.StatusNotFound)
}

func newTestApp(t *testing.T) testApp {
	t.Helper()
	return newTestAppWithConfig(t, testConfig())
}

func newTestAppWithConfig(t *testing.T, cfg config.Config) testApp {
	t.Helper()
	return newTestAppWithConfigAndRoutes(t, cfg, nil)
}

func newTestAppWithRoutes(t *testing.T, routes []httpapi.Route) testApp {
	t.Helper()
	return newTestAppWithConfigAndRoutes(t, testConfig(), routes)
}

func newTestAppWithModules(t *testing.T, modules *module.Registry) testApp {
	t.Helper()
	return newTestAppWithConfigRoutesAndModules(t, testConfig(), nil, modules)
}

func newTestAppWithConfigAndRoutes(t *testing.T, cfg config.Config, routes []httpapi.Route) testApp {
	t.Helper()
	return newTestAppWithConfigRoutesAndModules(t, cfg, routes, nil)
}

func newTestAppWithConfigRoutesAndModules(t *testing.T, cfg config.Config, routes []httpapi.Route, modules *module.Registry) testApp {
	t.Helper()

	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	if modules == nil {
		modules = module.NewRegistry(module.BuiltInModules()...)
	}
	tokens := security.NewTokenManager("test-secret", time.Hour, "gov2-test")
	services := service.NewRegistry(store, tokens, modules.Permissions())
	api := httpapi.New(httpapi.Options{
		Config:   cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Modules:  modules,
		Routes:   routes,
		Store:    store,
		Services: services,
		Tokens:   tokens,
	})
	return testApp{
		handler: api.Router(),
		store:   store,
	}
}

func writeExtensionJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(responseEnvelope[any]{
		Code:    200,
		Message: "ok",
		Data:    value,
	})
}

func assertSecurityHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for header, want := range map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "no-referrer",
		"Cross-Origin-Resource-Policy": "same-origin",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Fatalf("expected %s=%q, got %q", header, want, got)
		}
	}
}

func testConfig() config.Config {
	return config.Config{
		App: config.AppConfig{
			Name:        "GOV2",
			Environment: "test",
		},
		Security: config.SecurityConfig{
			AllowedOrigins: []string{"*"},
		},
		Web: config.WebConfig{
			StaticDir: "web/dist",
		},
	}
}

func login(t *testing.T, app testApp, username, password string) string {
	t.Helper()

	result := requestJSON[loginResult](t, app, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": username,
		"password": password,
	}, http.StatusOK)
	if result.Token == "" {
		t.Fatalf("expected login token for %s", username)
	}
	return result.Token
}

func requestJSON[T any](t *testing.T, app testApp, method, path, token string, body any, wantStatus int) T {
	t.Helper()
	return requestEnvelope[T](t, app, method, path, token, body, wantStatus).Data
}

func requestEnvelope[T any](t *testing.T, app testApp, method, path, token string, body any, wantStatus int) responseEnvelope[T] {
	t.Helper()

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s: expected status %d, got %d body=%s", method, path, wantStatus, rec.Code, rec.Body.String())
	}

	var envelope responseEnvelope[T]
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return envelope
}

func requestRawEnvelope[T any](t *testing.T, app testApp, method, path, token string, body string, wantStatus int) responseEnvelope[T] {
	t.Helper()
	return requestRawEnvelopeWithContentType[T](t, app, method, path, token, "application/json", body, wantStatus)
}

func requestRawEnvelopeWithContentType[T any](t *testing.T, app testApp, method, path, token string, contentType string, body string, wantStatus int) responseEnvelope[T] {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s: expected status %d, got %d body=%s", method, path, wantStatus, rec.Code, rec.Body.String())
	}

	var envelope responseEnvelope[T]
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return envelope
}

func decodeEnvelope[T any](t *testing.T, data []byte) responseEnvelope[T] {
	t.Helper()

	var envelope responseEnvelope[T]
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return envelope
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func hasPermissionDefinition(values []domain.PermissionDefinition, code string) bool {
	for _, value := range values {
		if value.Code == code {
			return true
		}
	}
	return false
}

func hasAuditLog(logs []domain.AuditLog, action, resource string) bool {
	_, ok := findAuditLog(logs, action, resource)
	return ok
}

func findAuditLog(logs []domain.AuditLog, action, resource string) (domain.AuditLog, bool) {
	for _, log := range logs {
		if log.Action == action && log.Resource == resource {
			return log, true
		}
	}
	return domain.AuditLog{}, false
}

func hasMenuPath(menus []domain.Menu, path string) bool {
	_, ok := findMenuByPath(menus, path)
	return ok
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

func assertMenuComponent(t *testing.T, menus []domain.Menu, path, component string) {
	t.Helper()

	menu, ok := findMenuByPath(menus, path)
	if !ok {
		t.Fatalf("expected menu path %s, got %+v", path, menus)
	}
	if menu.Component != component {
		t.Fatalf("menu %s component = %q, want %q", path, menu.Component, component)
	}
}

func hasValidationField(fields []service.FieldError, field string) bool {
	for _, item := range fields {
		if item.Field == field {
			return true
		}
	}
	return false
}

func assertRecordConflict(t *testing.T, app testApp, method, path, token string, body any) {
	t.Helper()

	envelope := requestEnvelope[any](t, app, method, path, token, body, http.StatusConflict)
	if envelope.Message != "record already exists" {
		t.Fatalf("%s %s: expected record conflict message, got %+v", method, path, envelope)
	}
}

func findModule(modules []module.Module, name string) (module.Module, bool) {
	for _, item := range modules {
		if item.Name == name {
			return item, true
		}
	}
	return module.Module{}, false
}

func itoa(value uint64) string {
	if value == 0 {
		return "0"
	}
	var out [20]byte
	i := len(out)
	for value > 0 {
		i--
		out[i] = byte('0' + value%10)
		value /= 10
	}
	return string(out[i:])
}
