# 05.02 Migrations

GOV2 now includes the first PostgreSQL migration draft.

## Files

- `migrations/000001_init.up.sql`
- `migrations/000001_init.down.sql`
- `migrations/000002_audit_resource_id_index.up.sql`
- `migrations/000002_audit_resource_id_index.down.sql`
- `migrations/seeds/system.sql`

## Commands

The CLI entrypoint exists now:

```bash
docker compose -f deployments/docker-compose.postgres.yml up -d

export GOV2_STORAGE_DRIVER=pgx
export GOV2_STORAGE_DSN='postgres://gov2:gov2@localhost:5432/gov2?sslmode=disable'

gov2 migrate up
gov2 migrate seed
```

The command uses `storage.driver` and `storage.dsn` from config or environment variables. GOV2 registers pgx under `storage.driver=pgx`.

Current default server storage is still `memory`. A SQL repository adapter exists under `internal/store/sqlstore`, with optional PostgreSQL integration tests covering the SQL store and SQL-backed HTTP auth/system route flows.

## Scope

The initial schema covers:

- users
- roles
- permissions
- user-role assignments
- role-permission assignments
- menus
- dictionaries
- dictionary items
- audit logs
- settings
- sessions

Mutable system tables use soft-delete-aware unique indexes where duplicate business keys should be reusable after deletion. Examples include user usernames, role codes, menu names, and dictionary codes.
Audit-log query indexes cover created time plus actor, action, resource, and
resource ID filters used by the admin audit log API.

## Seed Data

The seed SQL inserts:

- built-in permissions
- admin/operator roles
- built-in role permissions
- system menus
- user status dictionary
- default `site.title` only when the setting does not already exist

User seed data is intentionally not included in SQL because password hashing belongs to application code.
Operational settings are not reset by repeat seed runs; administrators may change
`site.title` without a later seed reverting the configured brand title.

## Rules

- Migrations are ordered and append-only after release.
- Migration numeric prefixes must be unique and contiguous from `000001`; do not
  reuse a number or leave gaps.
- Each numbered `.up.sql` migration should have a matching `.down.sql` file for
  development rollback and contract completeness.
- Seed data must be idempotent.
- Built-in permission and menu seed metadata must stay aligned with the backend
  permission definitions and module registry; normal Go tests check this
  contract.
- Each migration file is applied in a single database transaction with its schema
  migration record.
- Each seed file is applied in its own database transaction.
- Seed discovery only runs plain `*.sql` seed files and skips migration files
  ending in `.up.sql` or `.down.sql`.
- Production rollback should prefer forward fixes.
- SQL schema is not the public API contract.
