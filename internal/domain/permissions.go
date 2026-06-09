package domain

const (
	PermissionAll = "*"

	PermissionDashboardView = "dashboard:view"

	PermissionSystemUserList   = "system:user:list"
	PermissionSystemUserCreate = "system:user:create"
	PermissionSystemUserUpdate = "system:user:update"
	PermissionSystemUserDelete = "system:user:delete"

	PermissionSystemRoleList   = "system:role:list"
	PermissionSystemRoleCreate = "system:role:create"
	PermissionSystemRoleUpdate = "system:role:update"
	PermissionSystemRoleDelete = "system:role:delete"

	PermissionSystemMenuList   = "system:menu:list"
	PermissionSystemMenuCreate = "system:menu:create"
	PermissionSystemMenuUpdate = "system:menu:update"
	PermissionSystemMenuDelete = "system:menu:delete"

	PermissionSystemModuleList = "system:module:list"

	PermissionSystemDictionaryList   = "system:dictionary:list"
	PermissionSystemDictionaryCreate = "system:dictionary:create"
	PermissionSystemDictionaryUpdate = "system:dictionary:update"
	PermissionSystemDictionaryDelete = "system:dictionary:delete"

	PermissionSystemSettingList   = "system:setting:list"
	PermissionSystemSettingCreate = "system:setting:create"
	PermissionSystemSettingUpdate = "system:setting:update"
	PermissionSystemSettingDelete = "system:setting:delete"

	PermissionSystemAuditList = "system:audit:list"
)

type PermissionDefinition struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Module      string `json:"module"`
	Description string `json:"description"`
}

func SystemPermissions() []PermissionDefinition {
	return []PermissionDefinition{
		{Code: PermissionDashboardView, Name: "View dashboard", Module: "dashboard", Description: "View dashboard summary"},
		{Code: PermissionSystemUserList, Name: "List users", Module: "system", Description: "View system users"},
		{Code: PermissionSystemUserCreate, Name: "Create users", Module: "system", Description: "Create system users"},
		{Code: PermissionSystemUserUpdate, Name: "Update users", Module: "system", Description: "Update system users"},
		{Code: PermissionSystemUserDelete, Name: "Delete users", Module: "system", Description: "Delete system users"},
		{Code: PermissionSystemRoleList, Name: "List roles", Module: "system", Description: "View roles"},
		{Code: PermissionSystemRoleCreate, Name: "Create roles", Module: "system", Description: "Create roles"},
		{Code: PermissionSystemRoleUpdate, Name: "Update roles", Module: "system", Description: "Update roles"},
		{Code: PermissionSystemRoleDelete, Name: "Delete roles", Module: "system", Description: "Delete roles"},
		{Code: PermissionSystemMenuList, Name: "List menus", Module: "system", Description: "View menus"},
		{Code: PermissionSystemMenuCreate, Name: "Create menus", Module: "system", Description: "Create menus"},
		{Code: PermissionSystemMenuUpdate, Name: "Update menus", Module: "system", Description: "Update menus"},
		{Code: PermissionSystemMenuDelete, Name: "Delete menus", Module: "system", Description: "Delete menus"},
		{Code: PermissionSystemModuleList, Name: "List modules", Module: "system", Description: "View registered modules"},
		{Code: PermissionSystemDictionaryList, Name: "List dictionaries", Module: "system", Description: "View dictionaries"},
		{Code: PermissionSystemDictionaryCreate, Name: "Create dictionaries", Module: "system", Description: "Create dictionaries"},
		{Code: PermissionSystemDictionaryUpdate, Name: "Update dictionaries", Module: "system", Description: "Update dictionaries"},
		{Code: PermissionSystemDictionaryDelete, Name: "Delete dictionaries", Module: "system", Description: "Delete dictionaries"},
		{Code: PermissionSystemSettingList, Name: "List settings", Module: "system", Description: "View system settings"},
		{Code: PermissionSystemSettingCreate, Name: "Create settings", Module: "system", Description: "Create system settings"},
		{Code: PermissionSystemSettingUpdate, Name: "Update settings", Module: "system", Description: "Update system settings"},
		{Code: PermissionSystemSettingDelete, Name: "Delete settings", Module: "system", Description: "Delete system settings"},
		{Code: PermissionSystemAuditList, Name: "List audit logs", Module: "system", Description: "View audit logs"},
	}
}
