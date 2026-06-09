package security

import (
	"strings"

	"github.com/expary/GOV2/internal/domain"
)

type Policy struct {
	rolePermissions map[uint64]map[string]struct{}
}

func NewPolicy(roles []domain.Role) Policy {
	rolePermissions := make(map[uint64]map[string]struct{}, len(roles))
	for _, role := range roles {
		permissions := make(map[string]struct{}, len(role.Permissions))
		for _, permission := range role.Permissions {
			permission = strings.TrimSpace(permission)
			if permission == "" {
				continue
			}
			permissions[permission] = struct{}{}
		}
		rolePermissions[role.ID] = permissions
	}
	return Policy{rolePermissions: rolePermissions}
}

func (p Policy) Allows(roleIDs []uint64, permission string) bool {
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return false
	}
	for _, roleID := range roleIDs {
		permissions := p.rolePermissions[roleID]
		if permissions == nil {
			continue
		}
		if _, ok := permissions[domain.PermissionAll]; ok {
			return true
		}
		if _, ok := permissions[permission]; ok {
			return true
		}
	}
	return false
}
