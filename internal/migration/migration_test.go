package migration

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testDriverName = "gov2_migration_test"

var (
	testDriverOnce   sync.Once
	testDriverStates sync.Map
)

func TestRunUpRecordsMigrationInSameTransaction(t *testing.T) {
	dir := t.TempDir()
	writeTestSQL(t, dir, "000001_init.up.sql", "CREATE TABLE demo (id BIGINT);")

	state := &testDriverState{}
	db := openTestDB(t, state)

	applied, err := NewRunner(db, dir).RunUp(context.Background())
	if err != nil {
		t.Fatalf("RunUp() error = %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("RunUp() applied %d migrations, want 1", len(applied))
	}
	if applied[0].Version != "000001_init" || applied[0].Name != "000001_init.up.sql" {
		t.Fatalf("RunUp() applied = %+v, want init migration metadata", applied[0])
	}

	snapshot := state.snapshot()
	if snapshot.begins != 1 || snapshot.commits != 1 || snapshot.rollbacks != 0 {
		t.Fatalf("transaction counts = begins:%d commits:%d rollbacks:%d, want 1/1/0", snapshot.begins, snapshot.commits, snapshot.rollbacks)
	}
	if !containsStatement(snapshot.txExecs, "CREATE TABLE demo") {
		t.Fatalf("transaction statements = %v, want migration SQL", snapshot.txExecs)
	}
	if !containsStatement(snapshot.txExecs, "INSERT INTO gov2_schema_migrations") {
		t.Fatalf("transaction statements = %v, want migration record insert", snapshot.txExecs)
	}
}

func TestRunUpRollsBackWhenMigrationRecordInsertFails(t *testing.T) {
	dir := t.TempDir()
	writeTestSQL(t, dir, "000001_init.up.sql", "CREATE TABLE demo (id BIGINT);")

	state := &testDriverState{failMigrationInsert: true}
	db := openTestDB(t, state)

	applied, err := NewRunner(db, dir).RunUp(context.Background())
	if err == nil {
		t.Fatal("RunUp() error = nil, want migration record insert error")
	}
	if applied != nil {
		t.Fatalf("RunUp() applied = %v, want nil on error", applied)
	}

	snapshot := state.snapshot()
	if snapshot.begins != 1 || snapshot.commits != 0 || snapshot.rollbacks != 1 {
		t.Fatalf("transaction counts = begins:%d commits:%d rollbacks:%d, want 1/0/1", snapshot.begins, snapshot.commits, snapshot.rollbacks)
	}
}

func TestRunSeedsSkipsMigrationFilesAndCommitsEachSeed(t *testing.T) {
	dir := t.TempDir()
	writeTestSQL(t, dir, "001_system.sql", "INSERT INTO permissions (code) VALUES ('system.users.read');")
	writeTestSQL(t, dir, "002_schema.up.sql", "CREATE TABLE should_not_run (id BIGINT);")
	writeTestSQL(t, dir, "003_schema.down.sql", "DROP TABLE should_not_run;")
	writeTestSQL(t, dir, "004_notes.txt", "ignored")

	state := &testDriverState{}
	db := openTestDB(t, state)

	applied, err := NewRunner(db, t.TempDir()).RunSeeds(context.Background(), dir)
	if err != nil {
		t.Fatalf("RunSeeds() error = %v", err)
	}
	if len(applied) != 1 || applied[0] != "001_system.sql" {
		t.Fatalf("RunSeeds() applied = %v, want only 001_system.sql", applied)
	}

	snapshot := state.snapshot()
	if snapshot.begins != 1 || snapshot.commits != 1 || snapshot.rollbacks != 0 {
		t.Fatalf("transaction counts = begins:%d commits:%d rollbacks:%d, want 1/1/0", snapshot.begins, snapshot.commits, snapshot.rollbacks)
	}
	if containsStatement(snapshot.txExecs, "should_not_run") {
		t.Fatalf("transaction statements = %v, migration files should not run as seeds", snapshot.txExecs)
	}
}

func TestMigrationFilesSortsAndFiltersBySuffix(t *testing.T) {
	dir := t.TempDir()
	writeTestSQL(t, dir, "020_seed.sql", "SELECT 20;")
	writeTestSQL(t, dir, "010_seed.sql", "SELECT 10;")
	writeTestSQL(t, dir, "030_schema.up.sql", "SELECT 30;")
	writeTestSQL(t, dir, "040_schema.down.sql", "SELECT 40;")
	if err := os.Mkdir(filepath.Join(dir, "050_dir.sql"), 0o755); err != nil {
		t.Fatal(err)
	}

	files, err := migrationFiles(dir, ".sql")
	if err != nil {
		t.Fatalf("migrationFiles() error = %v", err)
	}

	names := make([]string, 0, len(files))
	for _, file := range files {
		names = append(names, file.Name())
	}
	want := []string{"010_seed.sql", "020_seed.sql"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("migrationFiles() = %v, want %v", names, want)
	}
}

func openTestDB(t *testing.T, state *testDriverState) *sql.DB {
	t.Helper()
	testDriverOnce.Do(func() {
		sql.Register(testDriverName, testDriver{})
	})
	dsn := t.Name() + "/" + time.Now().UTC().Format(time.RFC3339Nano)
	testDriverStates.Store(dsn, state)
	t.Cleanup(func() {
		testDriverStates.Delete(dsn)
	})

	db, err := sql.Open(testDriverName, dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func writeTestSQL(t *testing.T, dir, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsStatement(statements []string, want string) bool {
	for _, statement := range statements {
		if strings.Contains(statement, want) {
			return true
		}
	}
	return false
}

type testDriverSnapshot struct {
	execs     []string
	txExecs   []string
	queries   []string
	begins    int
	commits   int
	rollbacks int
}

type testDriverState struct {
	mu                  sync.Mutex
	execs               []string
	txExecs             []string
	queries             []string
	begins              int
	commits             int
	rollbacks           int
	inTx                bool
	failMigrationInsert bool
}

func (s *testDriverState) snapshot() testDriverSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	return testDriverSnapshot{
		execs:     append([]string(nil), s.execs...),
		txExecs:   append([]string(nil), s.txExecs...),
		queries:   append([]string(nil), s.queries...),
		begins:    s.begins,
		commits:   s.commits,
		rollbacks: s.rollbacks,
	}
}

type testDriver struct{}

func (testDriver) Open(name string) (driver.Conn, error) {
	stateValue, ok := testDriverStates.Load(name)
	if !ok {
		return nil, errors.New("missing test driver state")
	}
	return &testConn{state: stateValue.(*testDriverState)}, nil
}

type testConn struct {
	state *testDriverState
}

func (c *testConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepared statements are not supported by test driver")
}

func (c *testConn) Close() error {
	return nil
}

func (c *testConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *testConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	c.state.begins++
	c.state.inTx = true
	return &testTx{state: c.state}, nil
}

func (c *testConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	if c.state.inTx {
		c.state.txExecs = append(c.state.txExecs, query)
		if c.state.failMigrationInsert && strings.Contains(query, "INSERT INTO gov2_schema_migrations") {
			return nil, errors.New("migration insert failed")
		}
	} else {
		c.state.execs = append(c.state.execs, query)
	}
	return driver.RowsAffected(1), nil
}

func (c *testConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	c.state.queries = append(c.state.queries, query)
	return emptyRows{}, nil
}

type testTx struct {
	state *testDriverState
}

func (tx *testTx) Commit() error {
	tx.state.mu.Lock()
	defer tx.state.mu.Unlock()

	tx.state.commits++
	tx.state.inTx = false
	return nil
}

func (tx *testTx) Rollback() error {
	tx.state.mu.Lock()
	defer tx.state.mu.Unlock()

	tx.state.rollbacks++
	tx.state.inTx = false
	return nil
}

type emptyRows struct{}

func (emptyRows) Columns() []string {
	return []string{"version"}
}

func (emptyRows) Close() error {
	return nil
}

func (emptyRows) Next([]driver.Value) error {
	return io.EOF
}
