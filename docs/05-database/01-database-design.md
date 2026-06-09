# 05.01 Database Design

GOV2 should become SQL-backed while keeping the current memory store as a development and test adapter.

## Philosophy

- PostgreSQL-first for production.
- SQLite-supported for local development and single-binary demos.
- MySQL-compatible adapter can be added after PostgreSQL schema stabilizes.
- Schema changes are migration-driven and reviewable.
- Database records are not API contracts.

## Core Tables

Target table model:

```text
gov2_users
gov2_roles
gov2_user_roles
gov2_permissions
gov2_role_permissions
gov2_menus
gov2_dictionaries
gov2_dictionary_items
gov2_audit_logs
gov2_settings
gov2_sessions
```

Use the `gov2_` prefix for built-in system tables. Business modules may use module prefixes.

## Common Columns

Most mutable tables should include:

```text
id              bigint primary key
created_at      timestamptz not null
updated_at      timestamptz not null
deleted_at      timestamptz null
created_by      bigint null
updated_by      bigint null
version         bigint not null default 1
```

Rules:

- `deleted_at` enables soft delete for business/system records.
- Audit logs are append-only and do not need soft delete.
- `version` supports optimistic locking when needed.

## User Tables

```text
gov2_users
  id
  username unique
  nickname
  email unique nullable
  phone unique nullable
  avatar
  password_hash
  status
  last_login_at
  created_at
  updated_at
  deleted_at
```

Rules:

- Never return `password_hash` in API responses.
- Normalize username uniqueness case-insensitively when the database supports it.
- Status values should come from dictionary or enum-like constants.

## Role and Permission Tables

```text
gov2_roles
  id
  name
  code unique
  description
  created_at
  updated_at
  deleted_at

gov2_permissions
  id
  code unique
  name
  module
  description

gov2_user_roles
  user_id
  role_id

gov2_role_permissions
  role_id
  permission_id
```

Permission codes are stable framework contracts. Renaming a permission is a breaking change for menus, frontend rendering, and user expectations.

## Menu Tables

```text
gov2_menus
  id
  parent_id
  title
  name
  path
  icon
  component
  permission_code
  sort
  hidden
  created_at
  updated_at
```

Rules:

- Menus reference permission codes, not permission IDs, to make seed data readable.
- `component` is frontend metadata and should not be trusted for security.
- Seeded menu `component` values should match the frontend route metadata for the same path.
- Menu trees are sorted by `sort`, then `id`.

## Dictionary Tables

```text
gov2_dictionaries
  id
  code
  name
  description
  created_at
  updated_at
  deleted_at

gov2_dictionary_items
  id
  dictionary_id
  label
  value
  sort
  status
```

Dictionaries provide display/config data. They should not replace domain invariants for critical security or workflow states.

Dictionary `code` uniqueness is case-insensitive for active rows. Soft-deleted dictionary codes may be reused.

## Setting Tables

```text
gov2_settings
  id
  key
  value_json
  description
  created_at
  updated_at
```

Settings are operational configuration records. Values are JSON so modules can store structured configuration without schema changes. Setting keys are case-insensitively unique.

## Audit Logs

```text
gov2_audit_logs
  id
  actor_id
  actor
  action
  resource
  resource_id
  ip
  user_agent
  detail_json
  created_at
```

Rules:

- Append-only.
- Query by created time descending.
- Index actor, action, resource, and created time.
- Avoid storing secrets or raw tokens in details.

## Sessions

Future production sessions:

```text
gov2_sessions
  id
  user_id
  refresh_token_hash
  user_agent
  ip
  expires_at
  revoked_at
  created_at
```

Refresh tokens must be hashed before storage.

## Indexing

Baseline indexes:

```text
unique lower(username)
unique lower(email) where email is not null
gov2_user_roles(user_id, role_id)
gov2_role_permissions(role_id, permission_id)
gov2_menus(parent_id, sort)
gov2_audit_logs(created_at desc)
gov2_audit_logs(actor_id, created_at desc)
gov2_audit_logs(action, created_at desc)
gov2_audit_logs(resource, created_at desc)
```

Avoid adding speculative indexes before query patterns exist.

## Migrations

Migration rules:

- One migration per logical change.
- Migrations are immutable after release.
- Down migrations are useful in development but production rollback should prefer forward fixes.
- Seeds are separate from schema migrations and idempotent.
- Every migration should be safe to run in CI.

Suggested layout:

```text
migrations
  000001_init.up.sql
  000001_init.down.sql
  seeds
    system.sql
```

## Multi-Tenancy

Do not add multi-tenancy prematurely. Design with a future path:

- Add `tenant_id` to business tables when needed.
- System-level tables can be tenant-scoped or global by module.
- Tenant isolation must be enforced in repository queries and service authorization.
- Prefer row-level tenant separation first; database-per-tenant is an advanced deployment option.

## Data Access Rules

- Services never build SQL strings.
- Repositories never return password hashes unless a use case explicitly requires authentication.
- High-volume or user-generated list APIs must paginate. Small bounded metadata
  and configuration lists, such as roles, menus, modules, dictionaries, settings,
  and permission catalogs, may keep array responses when the API contract says so
  and the repository returns a deterministic order.
- Repository pagination uses the shared GOV2 contract: invalid pages start at
  `1`, invalid or missing page sizes default to `20`, and page sizes are capped
  at `100` across memory and SQL adapters.
- Development memory seed role permissions and menus must stay aligned with the
  built-in module registry so memory and SQL development environments expose the
  same system defaults.
- Exports must enforce permission and row limits.
- Deleting users should usually disable or soft delete, not hard delete.

## Current Gap

Current code keeps `internal/store/memory` as the development and unit-test adapter and includes a PostgreSQL-style `database/sql` adapter under `internal/store/sqlstore`. SQL list and dashboard summary reads now propagate database errors instead of returning empty collections on storage failure. The next persistence milestones are broader SQL edge-case hardening, production bootstrap review, and optional SQLite support after PostgreSQL behavior stabilizes.
