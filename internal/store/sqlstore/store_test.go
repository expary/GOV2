package sqlstore

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/repository"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapWriteErrorClassifiesPostgresErrors(t *testing.T) {
	tests := []struct {
		name string
		code string
		want error
	}{
		{name: "unique violation", code: "23505", want: repository.ErrConflict},
		{name: "foreign key violation", code: "23503", want: repository.ErrInvalidReference},
		{name: "not null violation", code: "23502", want: repository.ErrConstraint},
		{name: "check violation", code: "23514", want: repository.ErrConstraint},
		{name: "invalid text representation", code: "22P02", want: repository.ErrConstraint},
		{name: "other integrity constraint", code: "23000", want: repository.ErrConstraint},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mapWriteError(&pgconn.PgError{Code: tt.code})
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

func TestMapWriteErrorKeepsUnknownErrors(t *testing.T) {
	original := errors.New("network unavailable")

	if err := mapWriteError(original); !errors.Is(err, original) {
		t.Fatalf("expected original error, got %v", err)
	}
	if err := mapWriteError(&pgconn.PgError{Code: "40001"}); err == nil || errors.Is(err, repository.ErrConstraint) {
		t.Fatalf("expected non-constraint postgres error to pass through, got %v", err)
	}
}

func TestReadMethodsReturnDatabaseErrors(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://invalid")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	store := New(db)

	if _, _, err := store.ListUsers(repository.UserQuery{Page: 1, PageSize: 10}); err == nil {
		t.Fatal("expected ListUsers to return database error")
	}
	if _, err := store.ListRoles(); err == nil {
		t.Fatal("expected ListRoles to return database error")
	}
	if _, err := store.ListMenus(); err == nil {
		t.Fatal("expected ListMenus to return database error")
	}
	if _, err := store.ListDictionaries(); err == nil {
		t.Fatal("expected ListDictionaries to return database error")
	}
	if _, err := store.ListSettings(); err == nil {
		t.Fatal("expected ListSettings to return database error")
	}
	if _, _, err := store.ListAuditLogs(repository.AuditLogQuery{Page: 1, PageSize: 10}); err == nil {
		t.Fatal("expected ListAuditLogs to return database error")
	}
	if _, err := store.Summary(); err == nil {
		t.Fatal("expected Summary to return database error")
	}
}

func TestAddAuditLogReturnsDatabaseError(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://invalid")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	store := New(db)

	if _, err := store.AddAuditLog(domain.AuditLog{Actor: "system", Action: "test", Resource: "audit"}); err == nil {
		t.Fatal("expected AddAuditLog to return database error")
	}
}

func TestNormalizePageUsesRepositoryPaginationContract(t *testing.T) {
	page, pageSize := normalizePage(0, repository.MaxPageSize+1)
	if page != 1 || pageSize != repository.MaxPageSize {
		t.Fatalf("normalizePage() = (%d, %d), want (1, %d)", page, pageSize, repository.MaxPageSize)
	}
}

func TestRequireRowsAffected(t *testing.T) {
	original := errors.New("rows affected unavailable")
	if err := requireRowsAffected(fakeResult{rowsAffectedErr: original}); !errors.Is(err, original) {
		t.Fatalf("expected original rows affected error, got %v", err)
	}
	if err := requireRowsAffected(fakeResult{rowsAffected: 0}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected not found for zero affected rows, got %v", err)
	}
	if err := requireRowsAffected(fakeResult{rowsAffected: 2}); err != nil {
		t.Fatalf("expected affected rows to pass, got %v", err)
	}
}

func TestBuildMenuTreeSortsBySortThenID(t *testing.T) {
	menus := buildMenuTree([]domain.Menu{
		{ID: 20, Title: "Root 20", Sort: 10},
		{ID: 10, Title: "Root 10", Sort: 10},
		{ID: 202, ParentID: 20, Title: "Child 202", Sort: 5},
		{ID: 201, ParentID: 20, Title: "Child 201", Sort: 5},
	})

	if len(menus) != 2 {
		t.Fatalf("expected two root menus, got %+v", menus)
	}
	if menus[0].ID != 10 || menus[1].ID != 20 {
		t.Fatalf("expected roots to sort by sort then id, got ids %d, %d", menus[0].ID, menus[1].ID)
	}
	if len(menus[1].Children) != 2 {
		t.Fatalf("expected two children for root 20, got %+v", menus[1].Children)
	}
	if menus[1].Children[0].ID != 201 || menus[1].Children[1].ID != 202 {
		t.Fatalf("expected children to sort by sort then id, got ids %d, %d", menus[1].Children[0].ID, menus[1].Children[1].ID)
	}
}

type fakeResult struct {
	rowsAffected    int64
	rowsAffectedErr error
}

func (r fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (r fakeResult) RowsAffected() (int64, error) {
	return r.rowsAffected, r.rowsAffectedErr
}
