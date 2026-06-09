# 05.03 SQL Store

GOV2 now includes a `database/sql` repository adapter:

- `internal/store/sqlstore/store.go`

## Scope

The SQL store implements the current `repository.Store` interface:

- users, including role assignment, service-level role ID validation, and
  username/email/phone uniqueness conflicts
- roles, including write conflict handling and assigned-role delete protection
- menus, including create, update, delete, parent validation, and cycle protection
- dictionaries, including item replacement and soft delete
- settings, including JSONB values
- audit logs
- dashboard summary

It uses PostgreSQL-style SQL and the schema from `migrations/000001_init.up.sql`.

## Driver State

The adapter uses `database/sql`. GOV2 registers pgx through:

- `internal/store/postgres/driver.go`

Use `storage.driver=pgx`.

Next persistence steps:

1. Run `go mod tidy` and `go test ./...` in an environment with Go.
2. Run integration tests against PostgreSQL.
3. Expand route and service coverage around SQL edge cases.
4. Review optional local-development adapters after PostgreSQL behavior stabilizes.

## Transaction Boundary

SQL writes that update multiple tables use a shared transaction helper in `internal/store/sqlstore/store.go`.

The helper is used by:

- user create/update with role assignments
- role create/update with permission assignments
- menu create/update with registered permission validation
- dictionary create/update with item replacement

This keeps rollback, commit, and write-error mapping consistent while leaving single-table writes simple.

## Write Error Mapping

The SQL store maps common PostgreSQL write errors into repository errors:

- unique violation `23505` -> `repository.ErrConflict`
- foreign key violation `23503` -> `repository.ErrInvalidReference`
- not-null, check, invalid text, and other integrity constraint errors -> `repository.ErrConstraint`

HTTP handlers return `409` for conflicts and `400` for invalid references or constraint violations.

Read paths check row iteration and nested relation-loading errors before returning data. List and dashboard summary methods return repository errors to the service layer so unavailable SQL reads become normal API errors instead of silently returning empty or partially populated data. Write paths that depend on affected-row counts propagate `RowsAffected()` errors before deciding whether a target row was missing. Audit inserts also return database errors, keeping required security audit records from being silently dropped.

## Runtime Selection

Application startup now selects storage by config:

- `storage.driver=memory`: use in-memory development store. Driver values are
  trimmed and memory detection is case-insensitive; production startup rejects
  this driver.
- any other driver: open `database/sql`, optionally run migrations, then use SQL store.
- `pgx`: PostgreSQL through `github.com/jackc/pgx/v5/stdlib`.
- SQL DSN values are trimmed before opening the database, and blank values are
  rejected by both server startup and CLI SQL commands.

`storage.auto_migrate=true` runs migrations on startup. In non-production environments it also runs seed files and bootstraps development users when needed.
The built-in seed creates `site.title` only when that setting is missing, so
repeat seed runs do not reset an operator-customized application title.
Production detection is case-insensitive and trims whitespace before deciding whether to skip seed files and development user bootstrap.
Role and menu writes reference the seeded permission catalog. SQL persistence
rejects unknown permission codes instead of creating ad hoc permission records.

## Development Bootstrap

In non-production SQL mode, GOV2 can create the development users:

- `admin / admin123`
- `operator / admin123`

Development bootstrap also rechecks the built-in `admin` and `operator` roles
and restores their default permissions if role rows already exist but permission
assignments are incomplete. It does not create default users when any active
user already exists.

Production environments must not rely on these defaults.

## Initial Administrator

Production SQL mode does not create a default login user. After migrations and seed data have run, create the first administrator with:

```bash
export GOV2_STORAGE_DRIVER=pgx
export GOV2_STORAGE_DSN='postgres://gov2:gov2@localhost:5432/gov2?sslmode=disable'
export GOV2_ADMIN_USERNAME=admin
export GOV2_ADMIN_PASSWORD='replace-with-a-strong-password'
go run ./cmd/gov2 admin create
```

The command requires the seeded `admin` role, applies the same password policy as user management, and rejects creation when an administrator user already exists.

If the administrator password is lost, reset an existing administrator account with:

```bash
export GOV2_STORAGE_DRIVER=pgx
export GOV2_STORAGE_DSN='postgres://gov2:gov2@localhost:5432/gov2?sslmode=disable'
export GOV2_ADMIN_USERNAME=admin
export GOV2_ADMIN_PASSWORD='replace-with-a-new-strong-password'
go run ./cmd/gov2 admin reset-password
```

The reset command only updates a user that already has the `admin` role and applies the same password policy as user management.

## Integration Tests

GOV2 has optional PostgreSQL integration tests for the SQL store and SQL-backed HTTP API routes:

```bash
make postgres-up
export GOV2_TEST_POSTGRES_DSN='postgres://gov2:gov2@localhost:5432/gov2?sslmode=disable'
make test
```

Focused command:

```bash
make test-postgres
```

Without `GOV2_TEST_POSTGRES_DSN`, PostgreSQL integration tests are skipped.

The focused PostgreSQL command verifies migrations, seed data, repeat seed runs preserving existing `site.title`, development bootstrap credentials, user lifecycle behavior, user username/email/phone uniqueness conflicts, password updates with role replacement, role permission replacement and assigned-role delete protection, menu hierarchy and cycle protection, dictionary item replacement, setting JSON persistence and key conflicts, audit-log filtering, and dashboard summary counts against the real SQL adapter. It also runs a SQL-backed HTTP route flow for login/profile, RBAC denial, dashboard access, core system writes, audit-log filters, and delete-conflict behavior.
