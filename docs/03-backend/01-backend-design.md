# 03.01 Backend Design

GOV2 backend is a Go modular monolith. The default should remain easy to run, easy to understand, and easy to deploy as a single binary.

## Goals

- Keep core code framework-light.
- Make business modules testable without HTTP.
- Support SQL persistence without tying services to one database.
- Keep API contracts stable.
- Provide clear extension points for modules, jobs, and plugins.

## Current Implementation

Current packages:

- `cmd/gov2`: starts the HTTP server.
- `internal/app`: wires config, logger, store, services, HTTP router.
- `internal/config`: loads JSON and environment configuration.
- `internal/domain`: user, role, menu, dictionary, audit models.
- `internal/repository`: repository interfaces and shared repository errors.
- `internal/security`: password hashing, token signing, RBAC policy primitives.
- `internal/service`: auth, users, and system use cases.
- `internal/httpapi`: routes, middleware, request parsing, responses.
- `internal/module`: built-in module registry and module metadata.
- `internal/migration`: SQL migration runner.
- `internal/store/memory`: temporary in-memory store and seed data.
- `internal/store/sqlstore`: `database/sql` repository adapter for the PostgreSQL-style schema.
- `api/openapi.yaml`: initial OpenAPI contract draft.

The service layer now depends on repository interfaces. PostgreSQL integration coverage exists for the SQL store and SQL-backed HTTP auth/system route flows; remaining persistence work should broaden SQL edge cases and optional adapters.

## Target Package Contract

```text
internal/domain
  Pure domain models and invariants.

internal/service
  Use cases. Depends on domain and repository interfaces.

internal/repository
  Interfaces and query models.

internal/store/sql
  PostgreSQL/SQLite/MySQL implementations.

internal/httpapi
  HTTP delivery only.

internal/security
  Password, token, policy, crypto helpers.

internal/event
  Domain event dispatch and handlers.

internal/module
  Module registry and module metadata.
```

## Service Design

Services should expose use-case methods with explicit inputs:

```go
type CreateUserInput struct {
    Username string
    Password string
    RoleIDs  []uint64
}

type UserService interface {
    Create(ctx context.Context, actor Actor, input CreateUserInput) (PublicUser, error)
}
```

Rules:

- Every service method takes `context.Context`.
- Mutating methods receive actor/principal context.
- Services return domain-level errors, not HTTP status codes.
- Validation errors should be structured enough for API translation.
- Services should own transaction boundaries for multi-repository writes.

## Repository Design

Repository interfaces should be small and module-specific:

```go
type UserRepository interface {
    FindByID(ctx context.Context, id uint64) (domain.User, error)
    FindByUsername(ctx context.Context, username string) (domain.User, error)
    List(ctx context.Context, query UserQuery) (Page[domain.User], error)
    Create(ctx context.Context, user domain.User) (domain.User, error)
    Update(ctx context.Context, user domain.User) (domain.User, error)
}
```

Rules:

- Do not expose SQL builders to services.
- Use repository query structs, not ad hoc maps.
- Return domain objects or typed projections.
- Repository errors map to `not found`, `conflict`, `invalid reference`, `constraint`, `unavailable`.

## API Design

Routes:

```text
GET    /api/v1/app/config
POST   /api/v1/auth/login
POST   /api/v1/auth/logout
GET    /api/v1/auth/profile
GET    /api/v1/dashboard/summary
GET    /api/v1/system/users
POST   /api/v1/system/users
GET    /api/v1/system/users/{id}
PUT    /api/v1/system/users/{id}
PATCH  /api/v1/system/users/{id}/status
DELETE /api/v1/system/users/{id}
GET    /api/v1/system/roles
POST   /api/v1/system/roles
PUT    /api/v1/system/roles/{id}
DELETE /api/v1/system/roles/{id}
GET    /api/v1/system/menus
GET    /api/v1/system/dictionaries
GET    /api/v1/system/dictionaries/code/{code}
POST   /api/v1/system/dictionaries
PUT    /api/v1/system/dictionaries/{id}
DELETE /api/v1/system/dictionaries/{id}
GET    /api/v1/system/settings
POST   /api/v1/system/settings
PUT    /api/v1/system/settings/{id}
DELETE /api/v1/system/settings/{id}
GET    /api/v1/system/audit-logs
```

`/api/v1/app/config` is public but allowlisted. It exposes only safe presentation fields such as app name, app title, and environment, with `site.title` driving the frontend brand.

Future rules:

- Generate OpenAPI from route metadata or maintain a checked-in OpenAPI spec.
- Keep API DTOs separate from database records.
- Use `page` and `page_size` for offset pagination in admin tables.
- Audit logs accept `keyword`, `actor`, `action`, and `resource` filters in addition to pagination.
- Use cursor pagination for high-volume audit/events later.

## Authentication

MVP uses signed bearer tokens. Production target:

- Access token with short TTL.
- Refresh token stored server-side or revocable.
- Password policy and password history.
- Optional OAuth/OIDC provider adapters.
- Session audit trail.

Authenticated users can update their own profile fields and change their own password through auth endpoints. These self-service writes must not require system user management permissions and must write audit records.

Password hashing must remain slow and salted. Secrets must come from environment/config, not source code.

## Authorization

RBAC remains the default:

```text
role -> permissions[]
user -> roles[]
route -> required permission
```

Future policy extension:

```text
Can(actor, action, resource, context) bool
```

The route-level permission check is necessary but not sufficient for resource ownership checks. Business services must enforce row/resource-level rules when needed.

## Errors

Use typed domain/service errors:

- `ErrInvalidInput`
- `ErrNotFound`
- `ErrConflict`
- `ErrPermissionDenied`
- `ErrUnauthenticated`

HTTP handlers translate these into status codes. Services must not call `http.Error`.

Field-level validation errors use `service.ValidationError`, which still unwraps to `ErrInvalidInput`. HTTP responses keep the standard envelope and place field details under `data.fields[]`.

## Background Jobs

Initial background jobs can run in-process:

- audit cleanup
- token cleanup
- notification dispatch
- report export

Design future job interface:

```go
type Job interface {
    Name() string
    Run(ctx context.Context) error
}
```

Jobs must be idempotent when retried.

## Plugin Direction

Plugins should start as compile-time modules, not dynamic binary plugins. Dynamic plugins can be added only after the stable extension API is clear.

Module registration should include:

- name
- version
- permissions
- routes
- migrations
- seed data
- frontend route metadata

Current module metadata covers permissions, backend route metadata, menus, frontend routes, and migration sets. Application startup validates built-in module metadata for uniqueness and reference integrity before building the HTTP API. `httpapi.Options.Routes` provides the compile-time HTTP handler registration point for module delivery packages without editing the built-in route list.

## Observability

Backend must keep:

- structured logs
- request IDs
- audit logs
- health endpoint
- readiness endpoint
- `/metrics`

Incoming `X-Request-ID` values are accepted only when they are bounded token-like values. Invalid or oversized IDs are replaced so response headers, envelopes, and logs remain safe to consume.

All HTTP responses pass through shared browser security headers:
`X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`,
`Referrer-Policy: no-referrer`, and `Cross-Origin-Resource-Policy:
same-origin`. These headers also apply to extension routes registered through
`httpapi.Options.Routes`.

`/api/v1/health` reports process liveness. `/api/v1/ready` checks required
dependencies, including the configured repository store, and returns `503` when
the process should not receive traffic yet.

`/metrics` exposes Prometheus-compatible process gauges and counters for uptime,
goroutines, memory, garbage collections, and HTTP request totals labeled by
method, normalized route pattern, and status code. Dynamic API routes use the
registered pattern such as `/api/v1/system/users/{id}` so high-cardinality IDs do
not appear in metric labels.

Future:

- trace IDs
- slow query logs
- background job history

## Testing

Minimum backend test layers:

- Unit tests for security and policy.
- Service tests with fake repositories.
- Repository tests with real SQL database or SQLite.
- HTTP integration tests with seeded app.

Before release:

```bash
go test ./...
```
