package migration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Runner struct {
	db        *sql.DB
	dir       string
	namespace string
}

type AppliedMigration struct {
	Version   string
	Name      string
	AppliedAt time.Time
}

func NewRunner(db *sql.DB, dir string) *Runner {
	return &Runner{db: db, dir: dir}
}

func NewRunnerWithNamespace(db *sql.DB, dir string, namespace string) *Runner {
	return &Runner{db: db, dir: dir, namespace: strings.TrimSpace(namespace)}
}

func (r *Runner) RunUp(ctx context.Context) ([]AppliedMigration, error) {
	if err := r.ensureTable(ctx); err != nil {
		return nil, err
	}

	files, err := migrationFiles(r.dir, ".up.sql")
	if err != nil {
		return nil, err
	}

	applied := make([]AppliedMigration, 0)
	for _, file := range files {
		version := migrationVersionForNamespace(file.Name(), ".up.sql", r.namespace)
		exists, err := r.isApplied(ctx, version)
		if err != nil {
			return nil, err
		}
		if exists {
			continue
		}

		sqlText, err := os.ReadFile(filepath.Join(r.dir, file.Name()))
		if err != nil {
			return nil, err
		}
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, string(sqlText)); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("apply migration %s: %w", file.Name(), err)
		}
		now := time.Now().UTC()
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO gov2_schema_migrations (version, name, applied_at) VALUES ($1, $2, $3)",
			version,
			file.Name(),
			now,
		); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		applied = append(applied, AppliedMigration{Version: version, Name: file.Name(), AppliedAt: now})
	}
	return applied, nil
}

func (r *Runner) RunSeeds(ctx context.Context, dir string) ([]string, error) {
	files, err := migrationFiles(dir, ".sql")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	applied := make([]string, 0, len(files))
	for _, file := range files {
		sqlText, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			return nil, err
		}
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, string(sqlText)); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("apply seed %s: %w", file.Name(), err)
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		applied = append(applied, file.Name())
	}
	return applied, nil
}

func (r *Runner) ensureTable(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS gov2_schema_migrations (
  version TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL
)`)
	return err
}

func (r *Runner) isApplied(ctx context.Context, version string) (bool, error) {
	var value string
	err := r.db.QueryRowContext(ctx, "SELECT version FROM gov2_schema_migrations WHERE version = $1", version).Scan(&value)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, err
}

func migrationFiles(dir, suffix string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]os.DirEntry, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		if suffix == ".sql" && (strings.HasSuffix(entry.Name(), ".up.sql") || strings.HasSuffix(entry.Name(), ".down.sql")) {
			continue
		}
		files = append(files, entry)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})
	return files, nil
}

func migrationVersion(name, suffix string) string {
	return strings.TrimSuffix(name, suffix)
}

func migrationVersionForNamespace(name, suffix string, namespace string) string {
	version := migrationVersion(name, suffix)
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return version
	}
	return namespace + "/" + version
}
