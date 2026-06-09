package repository

import (
	"context"
	"errors"

	"github.com/expary/GOV2/internal/domain"
)

var (
	ErrNotFound         = errors.New("record not found")
	ErrConflict         = errors.New("record already exists")
	ErrInvalidReference = errors.New("invalid record reference")
	ErrConstraint       = errors.New("record constraint violation")
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

func NormalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	return page, pageSize
}

type UserQuery struct {
	Keyword  string
	Status   string
	Page     int
	PageSize int
}

type AuditLogQuery struct {
	Keyword    string
	Actor      string
	Action     string
	Resource   string
	ResourceID string
	Page       int
	PageSize   int
}

type Store interface {
	UserStore
	RoleStore
	MenuStore
	DictionaryStore
	SettingStore
	AuditLogStore
	DashboardStore
}

type UserStore interface {
	FindUserByUsername(username string) (domain.User, error)
	GetUser(id uint64) (domain.User, error)
	ListUsers(query UserQuery) ([]domain.User, int, error)
	CreateUser(user domain.User) (domain.User, error)
	UpdateUser(id uint64, update domain.User) (domain.User, error)
	UpdateUserStatus(id uint64, status string) (domain.User, error)
	DeleteUser(id uint64) error
	TouchLastLogin(id uint64) error
}

type RoleStore interface {
	ListRoles() ([]domain.Role, error)
	GetRole(id uint64) (domain.Role, error)
	CreateRole(role domain.Role) (domain.Role, error)
	UpdateRole(id uint64, update domain.Role) (domain.Role, error)
	DeleteRole(id uint64) error
}

type MenuStore interface {
	GetMenu(id uint64) (domain.Menu, error)
	ListMenus() ([]domain.Menu, error)
	CreateMenu(menu domain.Menu) (domain.Menu, error)
	UpdateMenu(id uint64, update domain.Menu) (domain.Menu, error)
	DeleteMenu(id uint64) error
}

type DictionaryStore interface {
	ListDictionaries() ([]domain.Dictionary, error)
	GetDictionaryByCode(code string) (domain.Dictionary, error)
	CreateDictionary(dictionary domain.Dictionary) (domain.Dictionary, error)
	UpdateDictionary(id uint64, update domain.Dictionary) (domain.Dictionary, error)
	DeleteDictionary(id uint64) error
}

type SettingStore interface {
	ListSettings() ([]domain.Setting, error)
	CreateSetting(setting domain.Setting) (domain.Setting, error)
	UpdateSetting(id uint64, update domain.Setting) (domain.Setting, error)
	DeleteSetting(id uint64) error
}

type AuditLogStore interface {
	AddAuditLog(log domain.AuditLog) (domain.AuditLog, error)
	ListAuditLogs(query AuditLogQuery) ([]domain.AuditLog, int, error)
}

type DashboardStore interface {
	Summary() (domain.DashboardSummary, error)
}

type HealthChecker interface {
	CheckHealth(ctx context.Context) error
}
