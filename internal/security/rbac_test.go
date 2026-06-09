package security

import (
	"testing"

	"github.com/expary/GOV2/internal/domain"
)

func TestPolicyAllowsWildcardAndPermission(t *testing.T) {
	policy := NewPolicy([]domain.Role{
		{ID: 1, Permissions: []string{" " + domain.PermissionAll + " "}},
		{ID: 2, Permissions: []string{"", " " + domain.PermissionSystemUserList + " ", domain.PermissionSystemUserList}},
	})

	if !policy.Allows([]uint64{1}, "anything") {
		t.Fatal("wildcard role should allow any permission")
	}
	if !policy.Allows([]uint64{2}, domain.PermissionSystemUserList) {
		t.Fatal("role should allow explicit permission")
	}
	if !policy.Allows([]uint64{2}, " "+domain.PermissionSystemUserList+" ") {
		t.Fatal("policy should trim requested permissions")
	}
	if policy.Allows([]uint64{2}, domain.PermissionSystemUserDelete) {
		t.Fatal("role should deny missing permission")
	}
	if policy.Allows([]uint64{1}, "") {
		t.Fatal("policy should not allow empty permissions")
	}
}
