package bootstrap

import (
	"errors"
	"strconv"
	"testing"

	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/repository"
	"github.com/expary/GOV2/internal/security"
	"github.com/expary/GOV2/internal/store/memory"
)

func TestCreateInitialAdminCreatesActiveAdmin(t *testing.T) {
	store := memory.NewStore()
	adminRole, err := store.CreateRole(domain.Role{
		Name:        "Administrator",
		Code:        "admin",
		Description: "Full system access",
		Permissions: []string{domain.PermissionAll},
	})
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}

	user, err := CreateInitialAdmin(store, AdminInput{
		Username: "root",
		Password: "strongpass",
		Nickname: "Root Admin",
		Email:    "root@gov2.local",
	})
	if err != nil {
		t.Fatalf("CreateInitialAdmin() error = %v", err)
	}
	if user.Username != "root" || user.Status != domain.UserStatusActive {
		t.Fatalf("unexpected admin user: %+v", user)
	}
	if len(user.RoleIDs) != 1 || user.RoleIDs[0] != adminRole.ID {
		t.Fatalf("expected admin role %d, got %+v", adminRole.ID, user.RoleIDs)
	}

	internalUser, err := store.FindUserByUsername("root")
	if err != nil {
		t.Fatalf("FindUserByUsername() error = %v", err)
	}
	if !security.VerifyPassword("strongpass", internalUser.PasswordHash) {
		t.Fatal("expected stored password hash to verify")
	}

	logs, total, err := store.ListAuditLogs(repository.AuditLogQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if total != 1 || logs[0].Action != "bootstrap_admin" {
		t.Fatalf("expected bootstrap audit log, got total=%d logs=%+v", total, logs)
	}
	if logs[0].ResourceID != strconv.FormatUint(user.ID, 10) {
		t.Fatalf("expected bootstrap audit resource_id %d, got %+v", user.ID, logs[0])
	}
}

func TestCreateInitialAdminRejectsShortPassword(t *testing.T) {
	store := memory.NewStore()
	if _, err := store.CreateRole(domain.Role{Name: "Administrator", Code: "admin"}); err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}

	for _, password := range []string{"short", "        "} {
		_, err := CreateInitialAdmin(store, AdminInput{
			Username: "admin",
			Password: password,
		})
		if !errors.Is(err, ErrInvalidAdminInput) {
			t.Fatalf("expected ErrInvalidAdminInput for %q, got %v", password, err)
		}
	}
}

func TestCreateInitialAdminRejectsMissingAdminRole(t *testing.T) {
	store := memory.NewStore()

	_, err := CreateInitialAdmin(store, AdminInput{
		Username: "admin",
		Password: "strongpass",
	})
	if !errors.Is(err, ErrAdminRoleNotFound) {
		t.Fatalf("expected ErrAdminRoleNotFound, got %v", err)
	}
}

func TestCreateInitialAdminRejectsExistingAdminUser(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	_, err := CreateInitialAdmin(store, AdminInput{
		Username: "another-admin",
		Password: "strongpass",
	})
	if !errors.Is(err, ErrAdminAlreadyExists) {
		t.Fatalf("expected ErrAdminAlreadyExists, got %v", err)
	}
}

func TestResetAdminPasswordUpdatesExistingAdmin(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	user, err := ResetAdminPassword(store, AdminPasswordInput{
		Username: "admin",
		Password: "newstrongpass",
	})
	if err != nil {
		t.Fatalf("ResetAdminPassword() error = %v", err)
	}
	if user.Username != "admin" {
		t.Fatalf("expected admin user, got %+v", user)
	}

	internalUser, err := store.FindUserByUsername("admin")
	if err != nil {
		t.Fatalf("FindUserByUsername() error = %v", err)
	}
	if !security.VerifyPassword("newstrongpass", internalUser.PasswordHash) {
		t.Fatal("expected new password hash to verify")
	}
	if security.VerifyPassword("admin123", internalUser.PasswordHash) {
		t.Fatal("expected old password to stop verifying")
	}

	logs, total, err := store.ListAuditLogs(repository.AuditLogQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if total < 1 || logs[0].Action != "reset_admin_password" {
		t.Fatalf("expected reset audit log first, got total=%d logs=%+v", total, logs)
	}
	if logs[0].ResourceID != strconv.FormatUint(user.ID, 10) {
		t.Fatalf("expected reset audit resource_id %d, got %+v", user.ID, logs[0])
	}
}

func TestResetAdminPasswordRejectsShortPassword(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	for _, password := range []string{"short", "        "} {
		_, err := ResetAdminPassword(store, AdminPasswordInput{
			Username: "admin",
			Password: password,
		})
		if !errors.Is(err, ErrInvalidAdminInput) {
			t.Fatalf("expected ErrInvalidAdminInput for %q, got %v", password, err)
		}
	}
}

func TestResetAdminPasswordRejectsNonAdminUser(t *testing.T) {
	store := memory.NewStore()
	if err := store.Seed(); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	_, err := ResetAdminPassword(store, AdminPasswordInput{
		Username: "operator",
		Password: "newstrongpass",
	})
	if !errors.Is(err, ErrAdminUserNotFound) {
		t.Fatalf("expected ErrAdminUserNotFound, got %v", err)
	}
}
