package service

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/repository"
	"github.com/expary/GOV2/internal/security"
	"github.com/expary/GOV2/internal/store/memory"
)

func TestNormalizePageUsesRepositoryPaginationContract(t *testing.T) {
	page, pageSize := normalizePage(0, repository.MaxPageSize+1)
	if page != 1 || pageSize != repository.MaxPageSize {
		t.Fatalf("normalizePage() = (%d, %d), want (1, %d)", page, pageSize, repository.MaxPageSize)
	}
}

func TestUserServiceCreateRejectsUnknownRole(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	service := &UserService{store: store}
	_, err := service.Create(CreateUserInput{
		Username: "unknown-role-user",
		Password: "pass12345",
		RoleIDs:  []uint64{9999},
		Status:   domain.UserStatusActive,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	fields, ok := ValidationFields(err)
	if !ok || !hasField(fields, "role_ids") {
		t.Fatalf("expected role_ids validation field, got fields=%+v ok=%v", fields, ok)
	}
}

func TestUserServiceCreateReturnsFieldValidationErrors(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	service := &UserService{store: store}
	_, err := service.Create(CreateUserInput{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	fields, ok := ValidationFields(err)
	if !ok {
		t.Fatalf("expected validation fields, got %v", err)
	}
	for _, field := range []string{"username", "password"} {
		if !hasField(fields, field) {
			t.Fatalf("expected validation field %q, got %+v", field, fields)
		}
	}
}

func TestPasswordPolicyValidationFields(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	userService := &UserService{store: store}
	for _, password := range []string{"short", "        "} {
		_, err := userService.Create(CreateUserInput{
			Username: "invalid-password-user",
			Password: password,
			Status:   domain.UserStatusActive,
		})
		expectValidationField(t, err, "password")
	}

	admin, err := store.FindUserByUsername("admin")
	if err != nil {
		t.Fatalf("FindUserByUsername() error = %v", err)
	}
	for _, password := range []string{"short", "        "} {
		_, err = userService.Update(admin.ID, UpdateUserInput{
			Username: admin.Username,
			Password: password,
			RoleIDs:  admin.RoleIDs,
			Status:   admin.Status,
		})
		expectValidationField(t, err, "password")
	}

	authService := &AuthService{store: store}
	for _, password := range []string{"short", "        "} {
		err = authService.ChangePassword(admin.ID, ChangePasswordInput{
			CurrentPassword: "admin123",
			NewPassword:     password,
		})
		expectValidationField(t, err, "new_password")
	}
}

func TestAuthServiceAuditsFailedLogin(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	service := &AuthService{store: store}
	_, err := service.Login(LoginInput{
		Username:  "admin",
		Password:  "wrong-password",
		IP:        "10.0.0.8:44321",
		UserAgent: "service-test",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	logs, total, err := store.ListAuditLogs(repository.AuditLogQuery{
		Action:   "login_failed",
		Resource: "auth",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("expected one failed login audit log, total=%d logs=%+v", total, logs)
	}
	log := logs[0]
	if log.Actor != "admin" || log.ActorID == 0 || log.ResourceID != strconv.FormatUint(log.ActorID, 10) || log.IP != "10.0.0.8" || log.UserAgent != "service-test" {
		t.Fatalf("unexpected failed login audit log: %+v", log)
	}
	if strings.Contains(log.Detail, "wrong-password") {
		t.Fatalf("audit log detail must not contain plaintext password: %+v", log)
	}
}

func TestAuthServiceLoginPropagatesUserLookupError(t *testing.T) {
	lookupErr := errors.New("user lookup unavailable")
	store := &failingLoginStore{findUserErr: lookupErr}
	service := &AuthService{store: store}

	_, err := service.Login(LoginInput{
		Username: "admin",
		Password: "admin123",
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("expected lookup error to propagate, got %v", err)
	}
	if errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("lookup failure must not be reported as invalid credentials: %v", err)
	}
	if store.auditCount != 0 {
		t.Fatalf("expected no failed-login audit for storage lookup error, got %d", store.auditCount)
	}
}

func TestUserServiceCreateDeduplicatesRoles(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	roles, err := store.ListRoles()
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	if len(roles) == 0 {
		t.Fatal("expected seeded roles")
	}

	service := &UserService{store: store}
	user, err := service.Create(CreateUserInput{
		Username: "dedupe-role-user",
		Password: "pass12345",
		RoleIDs:  []uint64{roles[0].ID, roles[0].ID},
		Status:   domain.UserStatusActive,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(user.RoleIDs) != 1 || user.RoleIDs[0] != roles[0].ID {
		t.Fatalf("expected one deduplicated role, got %+v", user.RoleIDs)
	}
}

func TestUserServiceProtectsLastActiveAdministrator(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	admin, err := store.FindUserByUsername("admin")
	if err != nil {
		t.Fatalf("FindUserByUsername(admin) error = %v", err)
	}
	service := &UserService{store: store}

	_, err = service.SetStatus(admin.ID, domain.UserStatusDisabled)
	if !errors.Is(err, ErrLastAdministrator) {
		t.Fatalf("expected ErrLastAdministrator when disabling last admin, got %v", err)
	}

	_, err = service.Update(admin.ID, UpdateUserInput{
		Username: admin.Username,
		Nickname: admin.Nickname,
		Email:    admin.Email,
		Phone:    admin.Phone,
		Avatar:   admin.Avatar,
		RoleIDs:  []uint64{},
		Status:   domain.UserStatusActive,
	})
	if !errors.Is(err, ErrLastAdministrator) {
		t.Fatalf("expected ErrLastAdministrator when removing last admin role, got %v", err)
	}

	if err := service.Delete(admin.ID); !errors.Is(err, ErrLastAdministrator) {
		t.Fatalf("expected ErrLastAdministrator when deleting last admin, got %v", err)
	}
}

func TestUserServiceAllowsChangingOneOfMultipleAdministrators(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	admin, err := store.FindUserByUsername("admin")
	if err != nil {
		t.Fatalf("FindUserByUsername(admin) error = %v", err)
	}
	hash, err := security.HashPassword("second123")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if _, err := store.CreateUser(domain.User{
		Username:     "second-admin",
		PasswordHash: hash,
		RoleIDs:      append([]uint64(nil), admin.RoleIDs...),
		Status:       domain.UserStatusActive,
	}); err != nil {
		t.Fatalf("CreateUser(second admin) error = %v", err)
	}

	service := &UserService{store: store}
	if _, err := service.SetStatus(admin.ID, domain.UserStatusDisabled); err != nil {
		t.Fatalf("SetStatus(admin disabled) error = %v", err)
	}
}

func TestUserServiceSetStatusNormalizesInput(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	hash, err := security.HashPassword("status123")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	user, err := store.CreateUser(domain.User{
		Username:     "status-normalized",
		PasswordHash: hash,
		Status:       domain.UserStatusActive,
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	service := &UserService{store: store}
	updated, err := service.SetStatus(user.ID, " \t"+domain.UserStatusDisabled+"\n")
	if err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if updated.Status != domain.UserStatusDisabled {
		t.Fatalf("expected normalized disabled status, got %+v", updated)
	}

	_, err = service.SetStatus(user.ID, " \t\n")
	expectValidationField(t, err, "status")
}

func TestSystemServiceValidationFields(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	service := &SystemService{store: store}
	tests := []struct {
		name  string
		run   func() error
		field string
	}{
		{
			name: "role name",
			run: func() error {
				_, err := service.CreateRole(RoleInput{Code: "auditor"})
				return err
			},
			field: "name",
		},
		{
			name: "menu path",
			run: func() error {
				_, err := service.CreateMenu(MenuInput{Title: "Reports", Name: "reports", Path: "reports"})
				return err
			},
			field: "path",
		},
		{
			name: "menu permission",
			run: func() error {
				_, err := service.CreateMenu(MenuInput{
					Title:      "Reports",
					Name:       "reports",
					Path:       "/reports",
					Permission: "reports:list",
				})
				return err
			},
			field: "permission",
		},
		{
			name: "dictionary code",
			run: func() error {
				_, err := service.CreateDictionary(DictionaryInput{Name: "Ticket Priority"})
				return err
			},
			field: "code",
		},
		{
			name: "setting key",
			run: func() error {
				_, err := service.CreateSetting(SettingInput{Value: []byte(`true`)})
				return err
			},
			field: "key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}
			fields, ok := ValidationFields(err)
			if !ok || !hasField(fields, tt.field) {
				t.Fatalf("expected validation field %q, got fields=%+v ok=%v", tt.field, fields, ok)
			}
		})
	}
}

func TestSystemServiceNormalizesRolePermissions(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	service := &SystemService{store: store}
	role, err := service.CreateRole(RoleInput{
		Name: "Support",
		Code: "support",
		Permissions: []string{
			" " + domain.PermissionDashboardView + " ",
			"",
			domain.PermissionDashboardView,
			domain.PermissionSystemUserList,
		},
	})
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	want := []string{domain.PermissionDashboardView, domain.PermissionSystemUserList}
	if !sameStrings(role.Permissions, want) {
		t.Fatalf("expected normalized permissions %+v, got %+v", want, role.Permissions)
	}

	role, err = service.UpdateRole(role.ID, RoleInput{
		Name: "Support",
		Code: "support",
		Permissions: []string{
			domain.PermissionDashboardView,
			" " + domain.PermissionAll + " ",
			domain.PermissionSystemUserList,
		},
	})
	if err != nil {
		t.Fatalf("UpdateRole() error = %v", err)
	}
	if !sameStrings(role.Permissions, []string{domain.PermissionAll}) {
		t.Fatalf("expected wildcard permissions only, got %+v", role.Permissions)
	}

	_, err = service.UpdateRole(role.ID, RoleInput{
		Name: "Support",
		Code: "support",
		Permissions: []string{
			domain.PermissionAll,
			"unknown:item:list",
		},
	})
	expectValidationField(t, err, "permissions")
}

func TestSystemServiceRejectsUnknownRolePermissions(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	service := &SystemService{store: store}
	_, err := service.CreateRole(RoleInput{
		Name:        "Broken Permission",
		Code:        "broken-permission",
		Permissions: []string{"inventory:item:list"},
	})
	expectValidationField(t, err, "permissions")
}

func TestSystemServiceAllowsConfiguredModulePermissions(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	service := &SystemService{
		store: store,
		permissionCatalog: []domain.PermissionDefinition{
			{Code: "inventory:item:list", Name: "List inventory items", Module: "inventory"},
		},
	}
	role, err := service.CreateRole(RoleInput{
		Name:        "Inventory Viewer",
		Code:        "inventory-viewer",
		Permissions: []string{"inventory:item:list"},
	})
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	if !sameStrings(role.Permissions, []string{"inventory:item:list"}) {
		t.Fatalf("expected configured module permission, got %+v", role.Permissions)
	}

	menu, err := service.CreateMenu(MenuInput{
		Title:      "Inventory",
		Name:       "inventory",
		Path:       "/inventory",
		Permission: "inventory:item:list",
	})
	if err != nil {
		t.Fatalf("CreateMenu() error = %v", err)
	}
	if menu.Permission != "inventory:item:list" {
		t.Fatalf("expected configured module menu permission, got %+v", menu)
	}
}

func TestSystemServiceRejectsDuplicateDictionaryItems(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	service := &SystemService{store: store}
	_, err := service.CreateDictionary(DictionaryInput{
		Code: "duplicated_item",
		Name: "Duplicated Item",
		Items: []domain.DictionaryItem{
			{Label: "Active", Value: "active", Sort: 1},
			{Label: "Active Again", Value: "ACTIVE", Sort: 2},
		},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	fields, ok := ValidationFields(err)
	if !ok || !hasField(fields, "items") {
		t.Fatalf("expected items validation field, got fields=%+v ok=%v", fields, ok)
	}
}

func TestSystemServiceRejectsInvalidSettingJSON(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	service := &SystemService{store: store}
	_, err := service.CreateSetting(SettingInput{
		Key:   "broken.json",
		Value: []byte(`{"broken":`),
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	fields, ok := ValidationFields(err)
	if !ok || !hasField(fields, "value") {
		t.Fatalf("expected value validation field, got fields=%+v ok=%v", fields, ok)
	}
}

func expectValidationField(t *testing.T, err error, field string) {
	t.Helper()

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	fields, ok := ValidationFields(err)
	if !ok || !hasField(fields, field) {
		t.Fatalf("expected validation field %q, got fields=%+v ok=%v", field, fields, ok)
	}
}

func hasField(fields []FieldError, field string) bool {
	for _, item := range fields {
		if item.Field == field {
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

type failingLoginStore struct {
	*memory.Store
	findUserErr error
	auditCount  int
}

func (s *failingLoginStore) FindUserByUsername(string) (domain.User, error) {
	return domain.User{}, s.findUserErr
}

func (s *failingLoginStore) AddAuditLog(log domain.AuditLog) (domain.AuditLog, error) {
	s.auditCount++
	return log, nil
}
