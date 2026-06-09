package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/httpapi"
	"github.com/expary/GOV2/internal/migration"
	"github.com/expary/GOV2/internal/module"
	"github.com/expary/GOV2/internal/security"
	"github.com/expary/GOV2/internal/service"
	_ "github.com/expary/GOV2/internal/store/postgres"
	"github.com/expary/GOV2/internal/store/sqlstore"
)

type postgresTestApp struct {
	handler http.Handler
}

func TestPostgresHTTPIntegration(t *testing.T) {
	app := newPostgresTestApp(t)
	suffix := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)

	appConfig := requestPostgresJSON[struct {
		Title string `json:"title"`
	}](t, app, http.MethodGet, "/api/v1/app/config", "", nil, http.StatusOK)
	if appConfig.Title == "" {
		t.Fatalf("expected public app config title, got %+v", appConfig)
	}

	adminToken := postgresLogin(t, app, "admin", "admin123")
	adminProfile := requestPostgresJSON[struct {
		Permissions []string      `json:"permissions"`
		Menus       []domain.Menu `json:"menus"`
	}](t, app, http.MethodGet, "/api/v1/auth/profile", adminToken, nil, http.StatusOK)
	if !contains(adminProfile.Permissions, domain.PermissionAll) {
		t.Fatalf("expected admin profile to include %q, got %+v", domain.PermissionAll, adminProfile.Permissions)
	}
	if !hasMenuPath(adminProfile.Menus, "/dashboard") || !hasMenuPath(adminProfile.Menus, "/system/users") {
		t.Fatalf("expected admin profile menus to include SQL-seeded system paths, got %+v", adminProfile.Menus)
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
		assertMenuComponent(t, adminProfile.Menus, path, component)
	}

	missingUsername := "it_missing_" + suffix
	requestPostgresJSON[any](t, app, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": missingUsername,
		"password": "wrong-password",
	}, http.StatusUnauthorized)

	role := requestPostgresJSON[domain.Role](t, app, http.MethodPost, "/api/v1/system/roles", adminToken, map[string]any{
		"name":        "Integration Dashboard Viewer",
		"code":        "it-dashboard-viewer-" + suffix,
		"description": "Created by PostgreSQL HTTP integration test",
		"permissions": []string{domain.PermissionDashboardView},
	}, http.StatusCreated)
	if role.ID == 0 || !contains(role.Permissions, domain.PermissionDashboardView) {
		t.Fatalf("expected created role with dashboard permission, got %+v", role)
	}

	user := requestPostgresJSON[domain.PublicUser](t, app, http.MethodPost, "/api/v1/system/users", adminToken, map[string]any{
		"username": "it_http_user_" + suffix,
		"password": "pass12345",
		"nickname": "Postgres HTTP User",
		"role_ids": []uint64{role.ID},
		"status":   domain.UserStatusActive,
	}, http.StatusCreated)
	if user.ID == 0 || len(user.RoleIDs) != 1 || user.RoleIDs[0] != role.ID {
		t.Fatalf("expected created SQL-backed user assigned to role %d, got %+v", role.ID, user)
	}

	viewerToken := postgresLogin(t, app, user.Username, "pass12345")
	summary := requestPostgresJSON[domain.DashboardSummary](t, app, http.MethodGet, "/api/v1/dashboard/summary", viewerToken, nil, http.StatusOK)
	if summary.UserCount < 1 || summary.RoleCount < 1 || summary.MenuCount < 1 {
		t.Fatalf("expected SQL-backed dashboard summary counts, got %+v", summary)
	}
	requestPostgresJSON[any](t, app, http.MethodGet, "/api/v1/system/users", viewerToken, nil, http.StatusForbidden)
	viewerProfile := requestPostgresJSON[struct {
		Menus []domain.Menu `json:"menus"`
	}](t, app, http.MethodGet, "/api/v1/auth/profile", viewerToken, nil, http.StatusOK)
	if !hasMenuPath(viewerProfile.Menus, "/dashboard") || hasMenuPath(viewerProfile.Menus, "/system") {
		t.Fatalf("expected dashboard-only SQL-backed profile menus, got %+v", viewerProfile.Menus)
	}

	parentMenu := requestPostgresJSON[domain.Menu](t, app, http.MethodPost, "/api/v1/system/menus", adminToken, map[string]any{
		"title":     "Integration",
		"name":      "it-http-menu-" + suffix,
		"path":      "/it-http-" + suffix,
		"component": "Layout",
		"sort":      800,
	}, http.StatusCreated)
	childMenu := requestPostgresJSON[domain.Menu](t, app, http.MethodPost, "/api/v1/system/menus", adminToken, map[string]any{
		"parent_id":  parentMenu.ID,
		"title":      "Integration Child",
		"name":       "it-http-menu-child-" + suffix,
		"path":       "/it-http-" + suffix + "/child",
		"component":  "IntegrationChildView",
		"permission": domain.PermissionDashboardView,
		"sort":       801,
	}, http.StatusCreated)
	menus := requestPostgresJSON[[]domain.Menu](t, app, http.MethodGet, "/api/v1/system/menus", adminToken, nil, http.StatusOK)
	if !hasMenuPath(menus, childMenu.Path) {
		t.Fatalf("expected SQL-backed menu tree to include child path %q, got %+v", childMenu.Path, menus)
	}
	requestPostgresJSON[any](t, app, http.MethodDelete, "/api/v1/system/menus/"+itoa(parentMenu.ID), adminToken, nil, http.StatusConflict)

	dictionary := requestPostgresJSON[domain.Dictionary](t, app, http.MethodPost, "/api/v1/system/dictionaries", adminToken, map[string]any{
		"code": "it_http_dictionary_" + suffix,
		"name": "Integration HTTP Dictionary",
		"items": []map[string]any{
			{"label": "Open", "value": "open", "sort": 1},
		},
	}, http.StatusCreated)
	updatedDictionary := requestPostgresJSON[domain.Dictionary](t, app, http.MethodPut, "/api/v1/system/dictionaries/"+itoa(dictionary.ID), adminToken, map[string]any{
		"code": dictionary.Code,
		"name": "Integration HTTP Dictionary Updated",
		"items": []map[string]any{
			{"label": "Closed", "value": "closed", "sort": 2},
		},
	}, http.StatusOK)
	if updatedDictionary.Name != "Integration HTTP Dictionary Updated" || len(updatedDictionary.Items) != 1 || updatedDictionary.Items[0].Value != "closed" {
		t.Fatalf("expected SQL-backed dictionary update with replaced items, got %+v", updatedDictionary)
	}

	setting := requestPostgresJSON[domain.Setting](t, app, http.MethodPost, "/api/v1/system/settings", adminToken, map[string]any{
		"key":         "it.http.setting." + suffix,
		"value":       map[string]bool{"enabled": true},
		"description": "Integration HTTP setting",
	}, http.StatusCreated)
	updatedSetting := requestPostgresJSON[domain.Setting](t, app, http.MethodPut, "/api/v1/system/settings/"+itoa(setting.ID), adminToken, map[string]any{
		"key":         setting.Key,
		"value":       map[string]bool{"enabled": false},
		"description": "Updated integration HTTP setting",
	}, http.StatusOK)
	if updatedSetting.Description != "Updated integration HTTP setting" || !strings.Contains(string(updatedSetting.Value), "false") {
		t.Fatalf("expected SQL-backed setting update to persist JSON value, got %+v", updatedSetting)
	}

	userCreateLogs := requestPostgresJSON[domain.PageResult[domain.AuditLog]](t, app, http.MethodGet, "/api/v1/system/audit-logs?action=create&resource=system.user&resource_id="+itoa(user.ID)+"&keyword="+url.QueryEscape("created user "+itoa(user.ID))+"&page=1&page_size=10", adminToken, nil, http.StatusOK)
	if userCreateLogs.Total != 1 || len(userCreateLogs.Items) != 1 {
		t.Fatalf("expected one SQL-backed user create audit log, got %+v", userCreateLogs)
	}
	if userCreateLogs.Items[0].ResourceID != itoa(user.ID) {
		t.Fatalf("expected SQL-backed user create resource_id %q, got %+v", itoa(user.ID), userCreateLogs.Items[0])
	}
	failedLoginLogs := requestPostgresJSON[domain.PageResult[domain.AuditLog]](t, app, http.MethodGet, "/api/v1/system/audit-logs?action=login_failed&resource=auth&actor="+url.QueryEscape(missingUsername)+"&page=1&page_size=10", adminToken, nil, http.StatusOK)
	if failedLoginLogs.Total != 1 || len(failedLoginLogs.Items) != 1 {
		t.Fatalf("expected one SQL-backed failed login audit log, got %+v", failedLoginLogs)
	}
	if strings.Contains(failedLoginLogs.Items[0].Detail, "wrong-password") {
		t.Fatalf("failed login audit log must not contain plaintext password: %+v", failedLoginLogs.Items[0])
	}

	requestPostgresJSON[any](t, app, http.MethodDelete, "/api/v1/system/roles/"+itoa(role.ID), adminToken, nil, http.StatusConflict)
	requestPostgresJSON[any](t, app, http.MethodDelete, "/api/v1/system/menus/"+itoa(childMenu.ID), adminToken, nil, http.StatusOK)
	requestPostgresJSON[any](t, app, http.MethodDelete, "/api/v1/system/menus/"+itoa(parentMenu.ID), adminToken, nil, http.StatusOK)
	requestPostgresJSON[any](t, app, http.MethodDelete, "/api/v1/system/dictionaries/"+itoa(dictionary.ID), adminToken, nil, http.StatusOK)
	requestPostgresJSON[any](t, app, http.MethodDelete, "/api/v1/system/settings/"+itoa(setting.ID), adminToken, nil, http.StatusOK)
	requestPostgresJSON[any](t, app, http.MethodDelete, "/api/v1/system/users/"+itoa(user.ID), adminToken, nil, http.StatusOK)
	requestPostgresJSON[any](t, app, http.MethodDelete, "/api/v1/system/roles/"+itoa(role.ID), adminToken, nil, http.StatusOK)
}

func newPostgresTestApp(t *testing.T) postgresTestApp {
	t.Helper()

	dsn := os.Getenv("GOV2_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set GOV2_TEST_POSTGRES_DSN to run PostgreSQL HTTP integration tests")
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

	runner := migration.NewRunner(db, "../../migrations")
	if _, err := runner.RunUp(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	if _, err := runner.RunSeeds(ctx, "../../migrations/seeds"); err != nil {
		t.Fatalf("run seeds: %v", err)
	}

	store := sqlstore.New(db)
	if err := store.BootstrapDevelopmentData(); err != nil {
		t.Fatalf("bootstrap development data: %v", err)
	}

	tokens := security.NewTokenManager("postgres-http-test-secret", time.Hour, "gov2-postgres-http-test")
	services := service.NewRegistry(store, tokens)
	api := httpapi.New(httpapi.Options{
		Config:   testConfig(),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Modules:  module.NewRegistry(module.BuiltInModules()...),
		Store:    store,
		Services: services,
		Tokens:   tokens,
	})
	return postgresTestApp{handler: api.Router()}
}

func postgresLogin(t *testing.T, app postgresTestApp, username, password string) string {
	t.Helper()

	result := requestPostgresJSON[loginResult](t, app, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": username,
		"password": password,
	}, http.StatusOK)
	if result.Token == "" {
		t.Fatalf("expected login token for %s", username)
	}
	return result.Token
}

func requestPostgresJSON[T any](t *testing.T, app postgresTestApp, method, path, token string, body any, wantStatus int) T {
	t.Helper()
	return requestPostgresEnvelope[T](t, app, method, path, token, body, wantStatus).Data
}

func requestPostgresEnvelope[T any](t *testing.T, app postgresTestApp, method, path, token string, body any, wantStatus int) responseEnvelope[T] {
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
