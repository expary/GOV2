package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/expary/GOV2/internal/repository"
)

func TestPageParamsNormalizeRepositoryPagination(t *testing.T) {
	tests := []struct {
		name         string
		target       string
		wantPage     int
		wantPageSize int
	}{
		{name: "defaults", target: "/api/v1/system/users", wantPage: 1, wantPageSize: repository.DefaultPageSize},
		{name: "invalid values", target: "/api/v1/system/users?page=0&page_size=0", wantPage: 1, wantPageSize: repository.DefaultPageSize},
		{name: "cap page size", target: "/api/v1/system/users?page=2&page_size=250", wantPage: 2, wantPageSize: repository.MaxPageSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			page, pageSize := pageParams(req)
			if page != tt.wantPage || pageSize != tt.wantPageSize {
				t.Fatalf("pageParams(%q) = (%d, %d), want (%d, %d)", tt.target, page, pageSize, tt.wantPage, tt.wantPageSize)
			}
		})
	}
}
