package service

import (
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/repository"
	"github.com/expary/GOV2/internal/security"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrDisabledUser       = errors.New("user is disabled")
	ErrInvalidInput       = errors.New("invalid input")
	ErrLastAdministrator  = errors.New("last active administrator cannot be removed")
)

type Registry struct {
	Auth   *AuthService
	Users  *UserService
	System *SystemService
}

func NewRegistry(store repository.Store, tokens *security.TokenManager, permissions ...[]domain.PermissionDefinition) *Registry {
	return &Registry{
		Auth:   &AuthService{store: store, tokens: tokens},
		Users:  &UserService{store: store},
		System: &SystemService{store: store, permissionCatalog: newPermissionCatalog(permissions...)},
	}
}

type AuthService struct {
	store  repository.Store
	tokens *security.TokenManager
}

type LoginInput struct {
	Username  string
	Password  string
	IP        string
	UserAgent string
}

type LoginResult struct {
	Token     string            `json:"token"`
	ExpiresAt int64             `json:"expires_at"`
	User      domain.PublicUser `json:"user"`
}

type UpdateProfileInput struct {
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Avatar   string `json:"avatar"`
}

type ChangePasswordInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *AuthService) Login(input LoginInput) (LoginResult, error) {
	username := strings.TrimSpace(input.Username)
	if username == "" || input.Password == "" {
		if err := s.auditLoginFailure(domain.User{}, username, input, "missing username or password"); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, ErrInvalidCredentials
	}

	user, err := s.store.FindUserByUsername(username)
	if err != nil {
		if !errors.Is(err, repository.ErrNotFound) {
			return LoginResult{}, err
		}
		if err := s.auditLoginFailure(domain.User{}, username, input, "invalid credentials"); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, ErrInvalidCredentials
	}
	if user.Status != domain.UserStatusActive {
		if err := s.auditLoginFailure(user, username, input, "user is disabled"); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, ErrDisabledUser
	}
	if !security.VerifyPassword(input.Password, user.PasswordHash) {
		if err := s.auditLoginFailure(user, username, input, "invalid credentials"); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, ErrInvalidCredentials
	}

	token, claims, err := s.tokens.Sign(user.ID, user.Username, user.RoleIDs)
	if err != nil {
		return LoginResult{}, err
	}
	if err := s.store.TouchLastLogin(user.ID); err != nil {
		return LoginResult{}, err
	}
	if _, err := s.store.AddAuditLog(domain.AuditLog{
		ActorID:    user.ID,
		Actor:      user.Username,
		Action:     "login",
		Resource:   "auth",
		ResourceID: strconv.FormatUint(user.ID, 10),
		IP:         normalizeIP(input.IP),
		UserAgent:  input.UserAgent,
		Detail:     "user logged in",
	}); err != nil {
		return LoginResult{}, err
	}
	user, err = s.store.GetUser(user.ID)
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		Token:     token,
		ExpiresAt: claims.ExpiresAt,
		User:      user.Public(),
	}, nil
}

func (s *AuthService) auditLoginFailure(user domain.User, username string, input LoginInput, detail string) error {
	actor := strings.TrimSpace(username)
	if actor == "" {
		actor = "unknown"
	}
	resourceID := ""
	if user.ID != 0 {
		resourceID = strconv.FormatUint(user.ID, 10)
	}
	_, err := s.store.AddAuditLog(domain.AuditLog{
		ActorID:    user.ID,
		Actor:      actor,
		Action:     "login_failed",
		Resource:   "auth",
		ResourceID: resourceID,
		IP:         normalizeIP(input.IP),
		UserAgent:  input.UserAgent,
		Detail:     detail,
	})
	return err
}

func (s *AuthService) UpdateProfile(userID uint64, input UpdateProfileInput) (domain.PublicUser, error) {
	user, err := s.store.GetUser(userID)
	if err != nil {
		return domain.PublicUser{}, err
	}
	if user.Status != domain.UserStatusActive {
		return domain.PublicUser{}, ErrDisabledUser
	}

	updated, err := s.store.UpdateUser(user.ID, domain.User{
		Username: user.Username,
		Nickname: strings.TrimSpace(input.Nickname),
		Email:    strings.TrimSpace(input.Email),
		Phone:    strings.TrimSpace(input.Phone),
		Avatar:   strings.TrimSpace(input.Avatar),
		RoleIDs:  append([]uint64(nil), user.RoleIDs...),
		Status:   user.Status,
	})
	if err != nil {
		return domain.PublicUser{}, err
	}
	return updated.Public(), nil
}

func (s *AuthService) ChangePassword(userID uint64, input ChangePasswordInput) error {
	fields := []FieldError{}
	if input.CurrentPassword == "" {
		fields = append(fields, FieldError{Field: "current_password", Message: "Current password is required"})
	}
	if input.NewPassword == "" {
		fields = append(fields, FieldError{Field: "new_password", Message: "New password is required"})
	} else if err := security.ValidatePasswordPolicy(input.NewPassword); err != nil {
		fields = append(fields, FieldError{Field: "new_password", Message: err.Error()})
	}
	if len(fields) > 0 {
		return NewValidationError(fields...)
	}

	user, err := s.store.GetUser(userID)
	if err != nil {
		return err
	}
	if user.Status != domain.UserStatusActive {
		return ErrDisabledUser
	}
	if !security.VerifyPassword(input.CurrentPassword, user.PasswordHash) {
		return ErrInvalidCredentials
	}

	passwordHash, err := security.HashPassword(input.NewPassword)
	if err != nil {
		return err
	}
	_, err = s.store.UpdateUser(user.ID, domain.User{
		Username:     user.Username,
		Nickname:     user.Nickname,
		Email:        user.Email,
		Phone:        user.Phone,
		Avatar:       user.Avatar,
		PasswordHash: passwordHash,
		RoleIDs:      append([]uint64(nil), user.RoleIDs...),
		Status:       user.Status,
	})
	return err
}

type UserService struct {
	store repository.Store
}

type CreateUserInput struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	Nickname string   `json:"nickname"`
	Email    string   `json:"email"`
	Phone    string   `json:"phone"`
	Avatar   string   `json:"avatar"`
	RoleIDs  []uint64 `json:"role_ids"`
	Status   string   `json:"status"`
}

type UpdateUserInput struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	Nickname string   `json:"nickname"`
	Email    string   `json:"email"`
	Phone    string   `json:"phone"`
	Avatar   string   `json:"avatar"`
	RoleIDs  []uint64 `json:"role_ids"`
	Status   string   `json:"status"`
}

func (s *UserService) List(filter repository.UserQuery) (domain.PageResult[domain.PublicUser], error) {
	users, total, err := s.store.ListUsers(filter)
	if err != nil {
		return domain.PageResult[domain.PublicUser]{}, err
	}
	items := make([]domain.PublicUser, 0, len(users))
	for _, user := range users {
		items = append(items, user.Public())
	}
	filter.Page, filter.PageSize = normalizePage(filter.Page, filter.PageSize)
	return domain.PageResult[domain.PublicUser]{
		Items:    items,
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

func (s *UserService) Get(id uint64) (domain.PublicUser, error) {
	user, err := s.store.GetUser(id)
	if err != nil {
		return domain.PublicUser{}, err
	}
	return user.Public(), nil
}

func (s *UserService) Create(input CreateUserInput) (domain.PublicUser, error) {
	username := strings.TrimSpace(input.Username)
	fields := []FieldError{}
	if username == "" {
		fields = append(fields, FieldError{Field: "username", Message: "Username is required"})
	}
	if input.Password == "" {
		fields = append(fields, FieldError{Field: "password", Message: "Password is required"})
	} else if err := security.ValidatePasswordPolicy(input.Password); err != nil {
		fields = append(fields, FieldError{Field: "password", Message: err.Error()})
	}
	if len(fields) > 0 {
		return domain.PublicUser{}, NewValidationError(fields...)
	}
	status, err := normalizeUserStatus(input.Status)
	if err != nil {
		return domain.PublicUser{}, NewValidationError(FieldError{Field: "status", Message: "Status must be active or disabled"})
	}
	roleIDs, err := validateRoleIDs(s.store, input.RoleIDs)
	if err != nil {
		return domain.PublicUser{}, err
	}
	passwordHash, err := security.HashPassword(input.Password)
	if err != nil {
		return domain.PublicUser{}, err
	}

	user, err := s.store.CreateUser(domain.User{
		Username:     username,
		Nickname:     strings.TrimSpace(input.Nickname),
		Email:        strings.TrimSpace(input.Email),
		Phone:        strings.TrimSpace(input.Phone),
		Avatar:       strings.TrimSpace(input.Avatar),
		PasswordHash: passwordHash,
		RoleIDs:      roleIDs,
		Status:       status,
	})
	if err != nil {
		return domain.PublicUser{}, err
	}
	return user.Public(), nil
}

func (s *UserService) Update(id uint64, input UpdateUserInput) (domain.PublicUser, error) {
	username := strings.TrimSpace(input.Username)
	if username == "" {
		return domain.PublicUser{}, NewValidationError(FieldError{Field: "username", Message: "Username is required"})
	}
	status, err := normalizeUserStatus(input.Status)
	if err != nil {
		return domain.PublicUser{}, NewValidationError(FieldError{Field: "status", Message: "Status must be active or disabled"})
	}
	roleIDs, err := validateRoleIDs(s.store, input.RoleIDs)
	if err != nil {
		return domain.PublicUser{}, err
	}
	existing, err := s.store.GetUser(id)
	if err != nil {
		return domain.PublicUser{}, err
	}
	removesLastAdmin, err := s.removesLastActiveAdministrator(existing, roleIDs, status)
	if err != nil {
		return domain.PublicUser{}, err
	}
	if removesLastAdmin {
		return domain.PublicUser{}, ErrLastAdministrator
	}

	update := domain.User{
		Username: username,
		Nickname: strings.TrimSpace(input.Nickname),
		Email:    strings.TrimSpace(input.Email),
		Phone:    strings.TrimSpace(input.Phone),
		Avatar:   strings.TrimSpace(input.Avatar),
		RoleIDs:  roleIDs,
		Status:   status,
	}
	if input.Password != "" {
		if err := security.ValidatePasswordPolicy(input.Password); err != nil {
			return domain.PublicUser{}, NewValidationError(FieldError{Field: "password", Message: err.Error()})
		}
		passwordHash, err := security.HashPassword(input.Password)
		if err != nil {
			return domain.PublicUser{}, err
		}
		update.PasswordHash = passwordHash
	}

	user, err := s.store.UpdateUser(id, update)
	if err != nil {
		return domain.PublicUser{}, err
	}
	return user.Public(), nil
}

func (s *UserService) SetStatus(id uint64, status string) (domain.PublicUser, error) {
	status = strings.TrimSpace(status)
	if status != domain.UserStatusActive && status != domain.UserStatusDisabled {
		return domain.PublicUser{}, NewValidationError(FieldError{Field: "status", Message: "Status must be active or disabled"})
	}
	existing, err := s.store.GetUser(id)
	if err != nil {
		return domain.PublicUser{}, err
	}
	removesLastAdmin, err := s.removesLastActiveAdministrator(existing, existing.RoleIDs, status)
	if err != nil {
		return domain.PublicUser{}, err
	}
	if removesLastAdmin {
		return domain.PublicUser{}, ErrLastAdministrator
	}
	user, err := s.store.UpdateUserStatus(id, status)
	if err != nil {
		return domain.PublicUser{}, err
	}
	return user.Public(), nil
}

func (s *UserService) Delete(id uint64) error {
	existing, err := s.store.GetUser(id)
	if err != nil {
		return err
	}
	removesLastAdmin, err := s.removesLastActiveAdministrator(existing, []uint64{}, domain.UserStatusDisabled)
	if err != nil {
		return err
	}
	if removesLastAdmin {
		return ErrLastAdministrator
	}
	return s.store.DeleteUser(id)
}

type SystemService struct {
	store             repository.Store
	permissionCatalog []domain.PermissionDefinition
}

type RoleInput struct {
	Name        string   `json:"name"`
	Code        string   `json:"code"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

type MenuInput struct {
	ParentID   uint64 `json:"parent_id"`
	Title      string `json:"title"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Icon       string `json:"icon"`
	Component  string `json:"component"`
	Permission string `json:"permission"`
	Sort       int    `json:"sort"`
	Hidden     bool   `json:"hidden"`
}

type DictionaryInput struct {
	Code  string                  `json:"code"`
	Name  string                  `json:"name"`
	Items []domain.DictionaryItem `json:"items"`
}

type SettingInput struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	Description string          `json:"description"`
}

func (s *SystemService) Dashboard() (domain.DashboardSummary, error) {
	return s.store.Summary()
}

func (s *SystemService) ListRoles() ([]domain.Role, error) {
	return s.store.ListRoles()
}

func (s *SystemService) Permissions() []domain.PermissionDefinition {
	return clonePermissionDefinitions(s.permissions())
}

func (s *SystemService) CreateRole(input RoleInput) (domain.Role, error) {
	role, err := s.normalizeRoleInput(input)
	if err != nil {
		return domain.Role{}, err
	}
	return s.store.CreateRole(role)
}

func (s *SystemService) UpdateRole(id uint64, input RoleInput) (domain.Role, error) {
	role, err := s.normalizeRoleInput(input)
	if err != nil {
		return domain.Role{}, err
	}
	return s.store.UpdateRole(id, role)
}

func (s *SystemService) DeleteRole(id uint64) error {
	return s.store.DeleteRole(id)
}

func (s *SystemService) Menus() ([]domain.Menu, error) {
	return s.store.ListMenus()
}

func (s *SystemService) CreateMenu(input MenuInput) (domain.Menu, error) {
	menu, err := s.normalizeMenuInput(input)
	if err != nil {
		return domain.Menu{}, err
	}
	return s.store.CreateMenu(menu)
}

func (s *SystemService) UpdateMenu(id uint64, input MenuInput) (domain.Menu, error) {
	menu, err := s.normalizeMenuInput(input)
	if err != nil {
		return domain.Menu{}, err
	}
	menus, err := s.store.ListMenus()
	if err != nil {
		return domain.Menu{}, err
	}
	if menu.ParentID == id || menuTreeContainsDescendant(menus, id, menu.ParentID) {
		return domain.Menu{}, NewValidationError(FieldError{
			Field:   "parent_id",
			Message: "Parent menu cannot be itself or a descendant",
		})
	}
	return s.store.UpdateMenu(id, menu)
}

func (s *SystemService) DeleteMenu(id uint64) error {
	return s.store.DeleteMenu(id)
}

func (s *SystemService) Dictionaries() ([]domain.Dictionary, error) {
	return s.store.ListDictionaries()
}

func (s *SystemService) DictionaryByCode(code string) (domain.Dictionary, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return domain.Dictionary{}, ErrInvalidInput
	}
	return s.store.GetDictionaryByCode(code)
}

func (s *SystemService) CreateDictionary(input DictionaryInput) (domain.Dictionary, error) {
	dictionary, err := normalizeDictionaryInput(input)
	if err != nil {
		return domain.Dictionary{}, err
	}
	return s.store.CreateDictionary(dictionary)
}

func (s *SystemService) UpdateDictionary(id uint64, input DictionaryInput) (domain.Dictionary, error) {
	dictionary, err := normalizeDictionaryInput(input)
	if err != nil {
		return domain.Dictionary{}, err
	}
	return s.store.UpdateDictionary(id, dictionary)
}

func (s *SystemService) DeleteDictionary(id uint64) error {
	return s.store.DeleteDictionary(id)
}

func (s *SystemService) Settings() ([]domain.Setting, error) {
	return s.store.ListSettings()
}

func (s *SystemService) CreateSetting(input SettingInput) (domain.Setting, error) {
	setting, err := normalizeSettingInput(input)
	if err != nil {
		return domain.Setting{}, err
	}
	return s.store.CreateSetting(setting)
}

func (s *SystemService) UpdateSetting(id uint64, input SettingInput) (domain.Setting, error) {
	setting, err := normalizeSettingInput(input)
	if err != nil {
		return domain.Setting{}, err
	}
	return s.store.UpdateSetting(id, setting)
}

func (s *SystemService) DeleteSetting(id uint64) error {
	return s.store.DeleteSetting(id)
}

func (s *SystemService) AuditLogs(filter repository.AuditLogQuery) (domain.PageResult[domain.AuditLog], error) {
	logs, total, err := s.store.ListAuditLogs(filter)
	if err != nil {
		return domain.PageResult[domain.AuditLog]{}, err
	}
	filter.Page, filter.PageSize = normalizePage(filter.Page, filter.PageSize)
	return domain.PageResult[domain.AuditLog]{
		Items:    logs,
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

func cleanPermissions(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if value == domain.PermissionAll {
			return []string{domain.PermissionAll}
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func newPermissionCatalog(permissionSets ...[]domain.PermissionDefinition) []domain.PermissionDefinition {
	if len(permissionSets) == 0 {
		return clonePermissionDefinitions(domain.SystemPermissions())
	}
	out := make([]domain.PermissionDefinition, 0)
	seen := map[string]struct{}{}
	for _, permissions := range permissionSets {
		for _, permission := range permissions {
			code := strings.TrimSpace(permission.Code)
			if code == "" {
				continue
			}
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			out = append(out, permission)
		}
	}
	if len(out) == 0 {
		return clonePermissionDefinitions(domain.SystemPermissions())
	}
	return out
}

func (s *SystemService) permissions() []domain.PermissionDefinition {
	if s == nil || len(s.permissionCatalog) == 0 {
		return domain.SystemPermissions()
	}
	return s.permissionCatalog
}

func clonePermissionDefinitions(items []domain.PermissionDefinition) []domain.PermissionDefinition {
	return append([]domain.PermissionDefinition(nil), items...)
}

func permissionCodeSet(items []domain.PermissionDefinition) map[string]struct{} {
	out := map[string]struct{}{}
	for _, item := range items {
		code := strings.TrimSpace(item.Code)
		if code == "" {
			continue
		}
		out[code] = struct{}{}
	}
	return out
}

func (s *SystemService) normalizeRoleInput(input RoleInput) (domain.Role, error) {
	role := domain.Role{
		Name:        strings.TrimSpace(input.Name),
		Code:        strings.TrimSpace(input.Code),
		Description: strings.TrimSpace(input.Description),
		Permissions: cleanPermissions(input.Permissions),
	}
	fields := []FieldError{}
	if role.Name == "" {
		fields = append(fields, FieldError{Field: "name", Message: "Name is required"})
	}
	if role.Code == "" {
		fields = append(fields, FieldError{Field: "code", Message: "Code is required"})
	}
	knownPermissions := permissionCodeSet(s.permissions())
	for _, permission := range input.Permissions {
		permission = strings.TrimSpace(permission)
		if permission == "" || permission == domain.PermissionAll {
			continue
		}
		if _, ok := knownPermissions[permission]; !ok {
			fields = append(fields, FieldError{Field: "permissions", Message: "Permissions must be registered"})
			break
		}
	}
	if len(fields) > 0 {
		return domain.Role{}, NewValidationError(fields...)
	}
	return role, nil
}

func normalizeUserStatus(status string) (string, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		return domain.UserStatusActive, nil
	}
	if status != domain.UserStatusActive && status != domain.UserStatusDisabled {
		return "", ErrInvalidInput
	}
	return status, nil
}

func validateRoleIDs(store repository.Store, roleIDs []uint64) ([]uint64, error) {
	if len(roleIDs) == 0 {
		return []uint64{}, nil
	}
	known := map[uint64]struct{}{}
	roles, err := store.ListRoles()
	if err != nil {
		return nil, err
	}
	for _, role := range roles {
		known[role.ID] = struct{}{}
	}
	seen := map[uint64]struct{}{}
	out := make([]uint64, 0, len(roleIDs))
	for _, id := range roleIDs {
		if id == 0 {
			return nil, NewValidationError(FieldError{Field: "role_ids", Message: "Role IDs must be positive"})
		}
		if _, ok := known[id]; !ok {
			return nil, NewValidationError(FieldError{Field: "role_ids", Message: "Role does not exist"})
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

func (s *UserService) removesLastActiveAdministrator(user domain.User, nextRoleIDs []uint64, nextStatus string) (bool, error) {
	roles, err := s.store.ListRoles()
	if err != nil {
		return false, err
	}
	adminRoleID, ok := adminRoleID(roles)
	if !ok {
		return false, nil
	}
	if user.Status != domain.UserStatusActive || !roleIDsContain(user.RoleIDs, adminRoleID) {
		return false, nil
	}
	if nextStatus == domain.UserStatusActive && roleIDsContain(nextRoleIDs, adminRoleID) {
		return false, nil
	}
	hasOther, err := hasOtherActiveAdministrator(s.store, user.ID, adminRoleID)
	if err != nil {
		return false, err
	}
	return !hasOther, nil
}

func adminRoleID(roles []domain.Role) (uint64, bool) {
	for _, role := range roles {
		if strings.EqualFold(strings.TrimSpace(role.Code), "admin") {
			return role.ID, true
		}
	}
	return 0, false
}

func hasOtherActiveAdministrator(store repository.Store, userID, adminRoleID uint64) (bool, error) {
	page := 1
	for {
		users, total, err := store.ListUsers(repository.UserQuery{
			Status:   domain.UserStatusActive,
			Page:     page,
			PageSize: repository.MaxPageSize,
		})
		if err != nil {
			return false, err
		}
		for _, user := range users {
			if user.ID != userID && roleIDsContain(user.RoleIDs, adminRoleID) {
				return true, nil
			}
		}
		if page*repository.MaxPageSize >= total || len(users) == 0 {
			return false, nil
		}
		page++
	}
}

func roleIDsContain(roleIDs []uint64, want uint64) bool {
	for _, roleID := range roleIDs {
		if roleID == want {
			return true
		}
	}
	return false
}

func normalizeDictionaryInput(input DictionaryInput) (domain.Dictionary, error) {
	dictionary := domain.Dictionary{
		Code: strings.TrimSpace(input.Code),
		Name: strings.TrimSpace(input.Name),
	}
	fields := []FieldError{}
	if dictionary.Code == "" {
		fields = append(fields, FieldError{Field: "code", Message: "Code is required"})
	}
	if dictionary.Name == "" {
		fields = append(fields, FieldError{Field: "name", Message: "Name is required"})
	}
	seen := map[string]struct{}{}
	for _, item := range input.Items {
		item.Label = strings.TrimSpace(item.Label)
		item.Value = strings.TrimSpace(item.Value)
		if item.Label == "" || item.Value == "" {
			fields = append(fields, FieldError{Field: "items", Message: "Dictionary items require label and value"})
			continue
		}
		key := strings.ToLower(item.Value)
		if _, ok := seen[key]; ok {
			fields = append(fields, FieldError{Field: "items", Message: "Dictionary item values must be unique"})
			continue
		}
		seen[key] = struct{}{}
		dictionary.Items = append(dictionary.Items, item)
	}
	if len(fields) > 0 {
		return domain.Dictionary{}, NewValidationError(fields...)
	}
	return dictionary, nil
}

func normalizeSettingInput(input SettingInput) (domain.Setting, error) {
	key := strings.TrimSpace(input.Key)
	if key == "" {
		return domain.Setting{}, NewValidationError(FieldError{Field: "key", Message: "Key is required"})
	}
	value := bytesTrim(input.Value)
	if len(value) == 0 {
		value = json.RawMessage(`{}`)
	}
	if !json.Valid(value) {
		return domain.Setting{}, NewValidationError(FieldError{Field: "value", Message: "Value must be valid JSON"})
	}
	return domain.Setting{
		Key:         key,
		Value:       append(json.RawMessage(nil), value...),
		Description: strings.TrimSpace(input.Description),
	}, nil
}

func bytesTrim(value json.RawMessage) json.RawMessage {
	text := strings.TrimSpace(string(value))
	if text == "" {
		return nil
	}
	return json.RawMessage(text)
}

func (s *SystemService) normalizeMenuInput(input MenuInput) (domain.Menu, error) {
	menu := domain.Menu{
		ParentID:   input.ParentID,
		Title:      strings.TrimSpace(input.Title),
		Name:       strings.TrimSpace(input.Name),
		Path:       strings.TrimSpace(input.Path),
		Icon:       strings.TrimSpace(input.Icon),
		Component:  strings.TrimSpace(input.Component),
		Permission: strings.TrimSpace(input.Permission),
		Sort:       input.Sort,
		Hidden:     input.Hidden,
	}
	fields := []FieldError{}
	if menu.Title == "" {
		fields = append(fields, FieldError{Field: "title", Message: "Title is required"})
	}
	if menu.Name == "" {
		fields = append(fields, FieldError{Field: "name", Message: "Name is required"})
	}
	if menu.Path == "" {
		fields = append(fields, FieldError{Field: "path", Message: "Path is required"})
	}
	if menu.Path != "" && !strings.HasPrefix(menu.Path, "/") {
		fields = append(fields, FieldError{Field: "path", Message: "Path must start with /"})
	}
	if menu.Permission != "" {
		if _, ok := permissionCodeSet(s.permissions())[menu.Permission]; !ok {
			fields = append(fields, FieldError{Field: "permission", Message: "Permission must be registered"})
		}
	}
	if len(fields) > 0 {
		return domain.Menu{}, NewValidationError(fields...)
	}
	return menu, nil
}

func menuTreeContainsDescendant(items []domain.Menu, rootID, candidateID uint64) bool {
	if rootID == 0 || candidateID == 0 {
		return false
	}
	for _, item := range items {
		if item.ID == rootID {
			return menuTreeContainsID(item.Children, candidateID)
		}
		if menuTreeContainsDescendant(item.Children, rootID, candidateID) {
			return true
		}
	}
	return false
}

func menuTreeContainsID(items []domain.Menu, id uint64) bool {
	for _, item := range items {
		if item.ID == id || menuTreeContainsID(item.Children, id) {
			return true
		}
	}
	return false
}

func normalizeIP(value string) string {
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	return value
}

func normalizePage(page, pageSize int) (int, int) {
	return repository.NormalizePage(page, pageSize)
}
