package sqlstore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/expary/GOV2/internal/domain"
	"github.com/expary/GOV2/internal/repository"
	"github.com/jackc/pgx/v5/pgconn"
)

const sqlstoreTestDriverName = "gov2_sqlstore_test"

var (
	sqlstoreTestDriverOnce   sync.Once
	sqlstoreTestDriverStates sync.Map
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

func TestWithTxMapsTransactionErrors(t *testing.T) {
	t.Run("begin error", func(t *testing.T) {
		state := &txTestDriverState{beginErr: &pgconn.PgError{Code: "23505"}}
		store := New(openTxTestDB(t, state))

		err := store.withTx(context.Background(), func(*sql.Tx) error {
			t.Fatal("transaction function should not run when begin fails")
			return nil
		})
		if !errors.Is(err, repository.ErrConflict) {
			t.Fatalf("expected begin error to map to conflict, got %v", err)
		}
		snapshot := state.snapshot()
		if snapshot.begins != 1 || snapshot.commits != 0 || snapshot.rollbacks != 0 {
			t.Fatalf("transaction counts = begins:%d commits:%d rollbacks:%d, want 1/0/0", snapshot.begins, snapshot.commits, snapshot.rollbacks)
		}
	})

	t.Run("function error", func(t *testing.T) {
		state := &txTestDriverState{}
		store := New(openTxTestDB(t, state))

		err := store.withTx(context.Background(), func(*sql.Tx) error {
			return &pgconn.PgError{Code: "23503"}
		})
		if !errors.Is(err, repository.ErrInvalidReference) {
			t.Fatalf("expected function error to map to invalid reference, got %v", err)
		}
		snapshot := state.snapshot()
		if snapshot.begins != 1 || snapshot.commits != 0 || snapshot.rollbacks != 1 {
			t.Fatalf("transaction counts = begins:%d commits:%d rollbacks:%d, want 1/0/1", snapshot.begins, snapshot.commits, snapshot.rollbacks)
		}
	})

	t.Run("commit error", func(t *testing.T) {
		state := &txTestDriverState{commitErr: &pgconn.PgError{Code: "23514"}}
		store := New(openTxTestDB(t, state))

		err := store.withTx(context.Background(), func(*sql.Tx) error {
			return nil
		})
		if !errors.Is(err, repository.ErrConstraint) {
			t.Fatalf("expected commit error to map to constraint, got %v", err)
		}
		snapshot := state.snapshot()
		if snapshot.begins != 1 || snapshot.commits != 1 {
			t.Fatalf("transaction counts = begins:%d commits:%d, want 1/1", snapshot.begins, snapshot.commits)
		}
	})

	t.Run("success", func(t *testing.T) {
		state := &txTestDriverState{}
		store := New(openTxTestDB(t, state))

		if err := store.withTx(context.Background(), func(*sql.Tx) error { return nil }); err != nil {
			t.Fatalf("expected successful transaction, got %v", err)
		}
		snapshot := state.snapshot()
		if snapshot.begins != 1 || snapshot.commits != 1 || snapshot.rollbacks != 0 {
			t.Fatalf("transaction counts = begins:%d commits:%d rollbacks:%d, want 1/1/0", snapshot.begins, snapshot.commits, snapshot.rollbacks)
		}
	})
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

type txTestDriverSnapshot struct {
	begins    int
	commits   int
	rollbacks int
}

type txTestDriverState struct {
	mu        sync.Mutex
	beginErr  error
	commitErr error
	begins    int
	commits   int
	rollbacks int
}

func (s *txTestDriverState) snapshot() txTestDriverSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	return txTestDriverSnapshot{
		begins:    s.begins,
		commits:   s.commits,
		rollbacks: s.rollbacks,
	}
}

func openTxTestDB(t *testing.T, state *txTestDriverState) *sql.DB {
	t.Helper()

	sqlstoreTestDriverOnce.Do(func() {
		sql.Register(sqlstoreTestDriverName, txTestDriver{})
	})

	dsn := fmt.Sprintf("%s-%p", t.Name(), state)
	sqlstoreTestDriverStates.Store(dsn, state)
	t.Cleanup(func() {
		sqlstoreTestDriverStates.Delete(dsn)
	})

	db, err := sql.Open(sqlstoreTestDriverName, dsn)
	if err != nil {
		t.Fatalf("open tx test db: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close tx test db: %v", err)
		}
	})
	return db
}

type txTestDriver struct{}

func (txTestDriver) Open(name string) (driver.Conn, error) {
	stateValue, ok := sqlstoreTestDriverStates.Load(name)
	if !ok {
		return nil, fmt.Errorf("missing tx test driver state for %q", name)
	}
	return &txTestConn{state: stateValue.(*txTestDriverState)}, nil
}

type txTestConn struct {
	state *txTestDriverState
}

func (*txTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("tx test driver does not support prepared statements")
}

func (*txTestConn) Close() error {
	return nil
}

func (c *txTestConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *txTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	c.state.begins++
	if c.state.beginErr != nil {
		return nil, c.state.beginErr
	}
	return &txTestTx{state: c.state}, nil
}

type txTestTx struct {
	state *txTestDriverState
}

func (tx *txTestTx) Commit() error {
	tx.state.mu.Lock()
	defer tx.state.mu.Unlock()

	tx.state.commits++
	return tx.state.commitErr
}

func (tx *txTestTx) Rollback() error {
	tx.state.mu.Lock()
	defer tx.state.mu.Unlock()

	tx.state.rollbacks++
	return nil
}
