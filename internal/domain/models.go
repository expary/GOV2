package domain

import (
	"encoding/json"
	"time"
)

const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

type User struct {
	ID           uint64     `json:"id"`
	Username     string     `json:"username"`
	Nickname     string     `json:"nickname"`
	Email        string     `json:"email"`
	Phone        string     `json:"phone"`
	Avatar       string     `json:"avatar"`
	PasswordHash string     `json:"-"`
	RoleIDs      []uint64   `json:"role_ids"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

type PublicUser struct {
	ID          uint64     `json:"id"`
	Username    string     `json:"username"`
	Nickname    string     `json:"nickname"`
	Email       string     `json:"email"`
	Phone       string     `json:"phone"`
	Avatar      string     `json:"avatar"`
	RoleIDs     []uint64   `json:"role_ids"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

type Role struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	Code        string    `json:"code"`
	Description string    `json:"description"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Menu struct {
	ID         uint64 `json:"id"`
	ParentID   uint64 `json:"parent_id"`
	Title      string `json:"title"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Icon       string `json:"icon"`
	Component  string `json:"component"`
	Permission string `json:"permission"`
	Sort       int    `json:"sort"`
	Hidden     bool   `json:"hidden"`
	Children   []Menu `json:"children,omitempty"`
}

type Dictionary struct {
	ID        uint64           `json:"id"`
	Code      string           `json:"code"`
	Name      string           `json:"name"`
	Items     []DictionaryItem `json:"items"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

type DictionaryItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Sort  int    `json:"sort"`
}

type Setting struct {
	ID          uint64          `json:"id"`
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	Description string          `json:"description"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type AuditLog struct {
	ID         uint64    `json:"id"`
	ActorID    uint64    `json:"actor_id"`
	Actor      string    `json:"actor"`
	Action     string    `json:"action"`
	Resource   string    `json:"resource"`
	ResourceID string    `json:"resource_id"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"user_agent"`
	Detail     string    `json:"detail"`
	CreatedAt  time.Time `json:"created_at"`
}

type PageResult[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

type DashboardSummary struct {
	UserCount       int `json:"user_count"`
	RoleCount       int `json:"role_count"`
	MenuCount       int `json:"menu_count"`
	AuditLogCount   int `json:"audit_log_count"`
	ActiveUserCount int `json:"active_user_count"`
}

func (u User) Public() PublicUser {
	roleIDs := append([]uint64(nil), u.RoleIDs...)
	return PublicUser{
		ID:          u.ID,
		Username:    u.Username,
		Nickname:    u.Nickname,
		Email:       u.Email,
		Phone:       u.Phone,
		Avatar:      u.Avatar,
		RoleIDs:     roleIDs,
		Status:      u.Status,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
		LastLoginAt: copyTimePtr(u.LastLoginAt),
	}
}

func copyTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
