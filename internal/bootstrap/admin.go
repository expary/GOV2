package bootstrap

import (
	"errors"
	"strconv"
	"strings"

	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/repository"
	"github.com/expary/GOV2/internal/security"
	"github.com/expary/GOV2/internal/service"
)

var (
	ErrAdminAlreadyExists = errors.New("administrator user already exists")
	ErrAdminRoleNotFound  = errors.New("administrator role not found; run migrations and seeds first")
	ErrAdminUserNotFound  = errors.New("administrator user not found")
	ErrInvalidAdminInput  = errors.New("invalid administrator input")
)

type AdminInput struct {
	Username string
	Password string
	Nickname string
	Email    string
	Phone    string
	Avatar   string
}

type AdminPasswordInput struct {
	Username string
	Password string
}

func CreateInitialAdmin(store repository.Store, input AdminInput) (domain.PublicUser, error) {
	username := strings.TrimSpace(input.Username)
	if username == "" {
		username = "admin"
	}
	if err := security.ValidatePasswordPolicy(input.Password); err != nil {
		return domain.PublicUser{}, ErrInvalidAdminInput
	}

	roles, err := store.ListRoles()
	if err != nil {
		return domain.PublicUser{}, err
	}
	adminRole, ok := findAdminRole(roles)
	if !ok {
		return domain.PublicUser{}, ErrAdminRoleNotFound
	}
	hasAdmin, err := hasAdminUser(store, adminRole.ID)
	if err != nil {
		return domain.PublicUser{}, err
	}
	if hasAdmin {
		return domain.PublicUser{}, ErrAdminAlreadyExists
	}

	registry := service.NewRegistry(store, nil)
	user, err := registry.Users.Create(service.CreateUserInput{
		Username: username,
		Password: input.Password,
		Nickname: strings.TrimSpace(input.Nickname),
		Email:    strings.TrimSpace(input.Email),
		Phone:    strings.TrimSpace(input.Phone),
		Avatar:   strings.TrimSpace(input.Avatar),
		RoleIDs:  []uint64{adminRole.ID},
		Status:   domain.UserStatusActive,
	})
	if err != nil {
		return domain.PublicUser{}, err
	}

	if _, err := store.AddAuditLog(domain.AuditLog{
		Actor:      "system",
		Action:     "bootstrap_admin",
		Resource:   "system.user",
		ResourceID: strconv.FormatUint(user.ID, 10),
		Detail:     "initial administrator created",
	}); err != nil {
		return domain.PublicUser{}, err
	}
	return user, nil
}

func ResetAdminPassword(store repository.Store, input AdminPasswordInput) (domain.PublicUser, error) {
	username := strings.TrimSpace(input.Username)
	if username == "" {
		username = "admin"
	}
	if err := security.ValidatePasswordPolicy(input.Password); err != nil {
		return domain.PublicUser{}, ErrInvalidAdminInput
	}

	roles, err := store.ListRoles()
	if err != nil {
		return domain.PublicUser{}, err
	}
	adminRole, ok := findAdminRole(roles)
	if !ok {
		return domain.PublicUser{}, ErrAdminRoleNotFound
	}

	user, err := store.FindUserByUsername(username)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return domain.PublicUser{}, ErrAdminUserNotFound
		}
		return domain.PublicUser{}, err
	}
	if !userHasRole(user, adminRole.ID) {
		return domain.PublicUser{}, ErrAdminUserNotFound
	}

	registry := service.NewRegistry(store, nil)
	updated, err := registry.Users.Update(user.ID, service.UpdateUserInput{
		Username: username,
		Password: input.Password,
		Nickname: user.Nickname,
		Email:    user.Email,
		Phone:    user.Phone,
		Avatar:   user.Avatar,
		RoleIDs:  user.RoleIDs,
		Status:   user.Status,
	})
	if err != nil {
		return domain.PublicUser{}, err
	}

	if _, err := store.AddAuditLog(domain.AuditLog{
		Actor:      "system",
		Action:     "reset_admin_password",
		Resource:   "system.user",
		ResourceID: strconv.FormatUint(updated.ID, 10),
		Detail:     "administrator password reset",
	}); err != nil {
		return domain.PublicUser{}, err
	}
	return updated, nil
}

func findAdminRole(roles []domain.Role) (domain.Role, bool) {
	for _, role := range roles {
		if strings.EqualFold(strings.TrimSpace(role.Code), "admin") {
			return role, true
		}
	}
	return domain.Role{}, false
}

func hasAdminUser(store repository.Store, roleID uint64) (bool, error) {
	page := 1
	for {
		users, total, err := store.ListUsers(repository.UserQuery{Page: page, PageSize: repository.MaxPageSize})
		if err != nil {
			return false, err
		}
		for _, user := range users {
			for _, assignedRoleID := range user.RoleIDs {
				if assignedRoleID == roleID {
					return true, nil
				}
			}
		}
		if page*repository.MaxPageSize >= total || len(users) == 0 {
			return false, nil
		}
		page++
	}
}

func userHasRole(user domain.User, roleID uint64) bool {
	for _, assignedRoleID := range user.RoleIDs {
		if assignedRoleID == roleID {
			return true
		}
	}
	return false
}
