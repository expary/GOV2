package repository

import "testing"

func TestNormalizePage(t *testing.T) {
	tests := []struct {
		name         string
		page         int
		pageSize     int
		wantPage     int
		wantPageSize int
	}{
		{name: "valid", page: 2, pageSize: 50, wantPage: 2, wantPageSize: 50},
		{name: "default invalid values", page: 0, pageSize: 0, wantPage: 1, wantPageSize: DefaultPageSize},
		{name: "cap page size", page: 1, pageSize: MaxPageSize + 1, wantPage: 1, wantPageSize: MaxPageSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, pageSize := NormalizePage(tt.page, tt.pageSize)
			if page != tt.wantPage || pageSize != tt.wantPageSize {
				t.Fatalf("NormalizePage(%d, %d) = (%d, %d), want (%d, %d)", tt.page, tt.pageSize, page, pageSize, tt.wantPage, tt.wantPageSize)
			}
		})
	}
}
