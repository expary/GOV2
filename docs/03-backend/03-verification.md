# 03.03 Verification

GOV2 keeps verification commands in `Makefile`.

## Standard Commands

```bash
make tidy
make test
make test-postgres
make web-api-check
make web-lint
make web-build
make docs-check
make validate
```

Docker smoke commands:

```bash
make docker-build
make docker-up
make docker-logs
make docker-down
```

## Route-Level Tests

`make test` includes HTTP route tests under:

- `internal/httpapi/api_test.go`
- `internal/httpapi/openapi_contract_test.go`
- `internal/httpapi/postgres_integration_test.go` when `GOV2_TEST_POSTGRES_DSN` is set

These tests exercise:

- login and current profile
- public health, readiness, and metrics endpoints
- HTTP request metrics for public, protected, and unknown API routes using
  normalized route labels
- baseline browser security headers on public, CORS error, and extension-route
  responses
- CORS preflight rejection for unsupported request methods and headers
- JSON request media type checks, body size limits, malformed JSON error
  handling, and trailing JSON value rejection
- OpenAPI method/path coverage against the backend route registry
- OpenAPI operation summaries against backend route metadata for generated and
  browsed API references
- OpenAPI public-route `security: []` declarations
- OpenAPI public routes do not declare `x-permission`
- OpenAPI `x-permission` declarations against registered backend permissions
- OpenAPI exact `401 UnauthorizedError` responses for authenticated routes and
  exact `403 ForbiddenError` responses for permission-protected routes
- OpenAPI `404 NotFoundError` responses against backend route metadata
- OpenAPI `409 ConflictError` responses against backend route metadata
- OpenAPI `required: true` declarations for request-body routes
- OpenAPI `application/json` content declarations for request-body routes
- OpenAPI request-body presence and generated schemas against backend route metadata
- OpenAPI documented `2xx` success responses for generated frontend clients
- OpenAPI success status codes against backend route metadata
- OpenAPI supported success response media types for generated frontend clients
- OpenAPI success response schemas against backend route metadata
- OpenAPI direct success response schema references against declared components
- OpenAPI non-text success responses declare generated frontend schemas
- OpenAPI schema properties generate concrete frontend types or explicitly
  document arbitrary JSON values
- OpenAPI array schemas declare generated frontend item types
- OpenAPI `400`, `413`, and `415` responses for JSON request-body routes,
  without `413` or `415` on routes that do not read JSON request bodies
- OpenAPI `400 BadRequestError` or `400 ValidationError` response components
  against backend route metadata for request-body routes
- OpenAPI `413 PayloadTooLargeError` and `415 UnsupportedMediaTypeError`
  response components for JSON request-body routes
- OpenAPI response component, response schema, request body schema, nested schema,
  and parameter references against declared components
- OpenAPI standard error response components against `application/json` and the
  standard envelope schemas
- OpenAPI component schema `required` fields against declared object properties
- OpenAPI path placeholders against required path parameter declarations
- OpenAPI query parameter declarations against backend route metadata
- OpenAPI path/query parameter schema types against backend route metadata and
  frontend generated runtime checks
- OpenAPI routes generate unique frontend endpoint and request wrapper names
- missing-token rejection
- permission denial for low-privilege users
- admin user write endpoints
- HTTP user and self-service password routes returning field-level errors for short and whitespace-only passwords
- admin menu write endpoints
- admin dictionary write endpoints
- admin setting write endpoints
- registered module extension metadata
- module registry validation for invalid or duplicate names, duplicate route,
  menu, permission, and migration metadata, permission records assigned to the
  wrong owning module or wrong permission-code namespace, unknown permission
  references, permission references owned by a different module, unknown menu
  parents, menu parents owned by a different module, missing backend route
  summaries, unsupported or non-uppercase backend HTTP methods, backend routes
  outside `/api/v1` or the owning module API namespace, public routes that
  declare permissions, non-absolute menu or frontend route paths, menu or
  frontend route paths outside the owning module path namespace, and
  menu/frontend route component mismatches
- built-in module backend route metadata against registered HTTP route specs and
  OpenAPI operation summaries
- built-in module permission references against registered permission metadata
- role permission catalog behavior, including registered module permissions
  appearing in `/system/permissions`, assignable registered module permissions,
  and field-level rejection for unknown role permission codes
- RBAC and profile permission normalization for trimmed, duplicate, and empty
  role permission entries, including exclusive wildcard profile aggregation
- built-in frontend route metadata against actual Vue Router protected routes
- SQL seed permission, role assignment, no-default-user, user-status dictionary,
  and menu metadata against backend permission, domain, and module registries
- SQL startup gating that skips automatic seed data and development-user
  bootstrap in production
- memory-store seed role permissions and menu metadata against built-in module defaults
- compile-time extension route middleware and RBAC behavior, including
  non-empty permissions overriding an accidentally public route flag
- invalid system module input
- conflict and not-found responses for system module writes
- explicit SQL read-error propagation for list and dashboard summary methods
- explicit SQL affected-row error propagation for not-found write detection
- explicit SQL audit-write error propagation
- audit log creation for core system writes
- SQL-backed login, profile, RBAC, core system writes, and audit-log filtering when PostgreSQL integration is enabled

Service tests also cover login storage lookup failures so repository outages are
propagated as service errors instead of being reported as bad credentials.
Repository, HTTP, memory-store, and SQL-store tests cover the shared pagination
normalization contract: invalid pages start at `1`, invalid page sizes default to
`20`, and page sizes are capped at `100`.

Scaffold tests cover module name format and reserved built-in namespace
rejection, generated module file layout, stable CLI output, overwrite preflight
behavior, generated backend module metadata, and the generated
`module.ValidateModules(Module())` self-test file for scaffolded modules. They
also run `go test` against a generated backend package in a temporary in-repo
module path so template import paths, compilation, and metadata self-validation
are checked together.

## Frontend API Contract

`make web-api-check` runs:

```bash
cd web && npm run api:check
```

This verifies that `web/src/api/endpoints.js`, `web/src/api/requests.js`, and `web/src/api/types.js` are current with `api/openapi.yaml`, including generated OpenAPI operation summary endpoint metadata and request wrapper JSDoc, generated path/query parameter metadata, generated path/query parameter types that Go contract tests compare with backend route metadata, unique generated endpoint names, request wrapper functions with `AbortSignal` option JSDoc, request-body schema names, response schema names, request-body field metadata, object typedefs, and array response typedefs. It also fails when an operation omits a non-empty `summary`, when a path or query parameter omits a supported scalar schema type, when a public operation declares `x-permission`, when an operation omits a `2xx` success response, when a success response uses an unsupported media type, when a non-text OpenAPI `2xx` response omits a known response schema, when an operation references an unknown response component, when a response component references an unknown schema, when a standard error response component omits `application/json` or its standard envelope schema, when a schema component references an unknown schema, when a schema required field is not declared in the same object properties, when an array schema omits `items`, when a schema property would generate `unknown` without explicitly documenting an arbitrary JSON value, when an authenticated route omits `401 UnauthorizedError`, when a permission-protected route omits `403 ForbiddenError`, when a request-body route omits `required: true`, when a request-body route omits `application/json` content, when a request-body route omits a generated schema reference, when a request-body route omits `400`, `413`, or `415`, when a request-body route omits a `400 BadRequestError` or `400 ValidationError` response component, or when a request-body route omits exact `413 PayloadTooLargeError` or `415 UnsupportedMediaTypeError` response components. It verifies that `web/src/permissions.js` is current with `internal/domain/permissions.go`. Regenerate these files with `make web-api-generate` after changing the OpenAPI contract or backend permission registry.
It also scans generated request wrappers for endpoint coverage, calls to their
same-named generated endpoint metadata, canonical generated imports, generated
path/query parameter JSDoc types, request body JSDoc, return JSDoc, OpenAPI
summary JSDoc, and `AbortSignal` option docs, then the generated `options = {}`
default parameter and top-level options JSDoc, then scans Vue and JavaScript source for unknown or
non-named
generated request wrapper imports, unknown generated endpoint references,
view-level generated GET request calls whose options object does not include a
`signal` option, unknown
endpoint body or response schemas, unknown imported permission object references,
unregistered permission literals, hardcoded wildcard permission literals instead
of `permissions.all`, direct `api(...)` or `apiRequest(...)` usage outside the
generated wrapper, generated endpoint metadata imports outside the generated
wrapper, relative or aliased imports that resolve to restricted API metadata and
transport modules, raw browser or third-party request transports, and hardcoded
`/api/v1` literals outside the generated endpoint contract.

`make web-test` runs:

```bash
cd web && npm run test
```

This uses Node's built-in test runner to cover API client query building,
generated endpoint summary metadata and request wrapper JSDoc validation,
generated endpoint metadata validation, generated path/query parameter type
validation, generated request body field, type, enum, and array scalar item
validation, JSON body dispatch, public endpoint auth
suppression, generated response schema validation for top-level values,
generated field types, generated enum values, generated nested object fields,
generated array scalar item types, generated array item objects, and generated
page/list item objects, AbortSignal forwarding,
authenticated `401` expiration handling, and field-level validation error
mapping.
Vue lint/build validation covers the user and audit-log views that pass
AbortSignals through generated API requests for superseded table refreshes, plus
the dashboard, module metadata, and core system management views that cancel
superseded list or summary refreshes.

## Documentation Links

`make docs-check` runs:

```bash
node scripts/check-doc-links.mjs
```

This checks local Markdown links and Markdown heading anchors in `README.md`,
`AGENTS.md`, and `docs/`.
It also verifies that Markdown design documents stay under numbered `docs/`
directories and are indexed from both `docs/00-document-map.md` and
`docs/README.md`.

## PostgreSQL Development Commands

```bash
make postgres-up
make migrate-up
make migrate-seed
GOV2_ADMIN_PASSWORD='replace-with-a-strong-password' make admin-create
GOV2_ADMIN_PASSWORD='replace-with-a-new-strong-password' make admin-reset-password
make run-sql
```

## Optional Integration Test

```bash
export GOV2_TEST_POSTGRES_DSN='postgres://gov2:gov2@localhost:5432/gov2?sslmode=disable'
make test
```

Focused command:

```bash
make test-postgres
```

When `GOV2_TEST_POSTGRES_DSN` is absent, PostgreSQL integration tests are skipped during normal `make test`. `make test-postgres` sets the default local development DSN from `POSTGRES_DSN`.

The focused PostgreSQL integration command runs SQL store coverage plus SQL-backed HTTP API coverage for login/profile, RBAC denial, dashboard access, core system writes, audit-log filters, and delete-conflict behavior. SQL store coverage includes user uniqueness conflicts, password updates with role replacement, role permission replacement, menu cycle checks, dictionary item replacement, setting JSON persistence, audit-log filters, and dashboard summary counts.
