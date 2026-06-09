# 08.01 Roadmap

This roadmap keeps GOV2 moving from MVP to a complete framework without copying any reference project.

## Phase 0: MVP Foundation

Status: in progress.

Deliverables:

- Go HTTP server.
- Vue 3 admin shell.
- Memory store.
- Login/profile.
- Users, roles, menus, dictionaries, settings, audit logs.
- User management CRUD with role selection and status control.
- Role management CRUD with permission catalog and assigned-role delete protection.
- Menu management CRUD with parent and cycle validation.
- Dictionary management CRUD with item replacement.
- Setting management CRUD with JSON values.
- Public app config endpoint with `site.title`-driven frontend branding.
- Framework design docs.
- Repository interfaces.
- Module registry.
- OpenAPI draft.
- OpenAPI route and permission contract test.
- Initial PostgreSQL migration SQL.
- Migration CLI entrypoint.
- `database/sql` store adapter.
- PostgreSQL pgx driver registration.
- PostgreSQL development Docker Compose file.
- Application Dockerfile and full local Docker Compose stack.
- Optional PostgreSQL integration test skeleton.
- GitHub Actions CI for local-equivalent validation.
- PostgreSQL service-backed CI integration test.
- HTTP route tests for auth, permission checks, and core system writes.
- HTTP route tests for invalid input, conflict, and not-found system write responses.
- Audit logging for core system mutations.
- Audit logging for successful and failed login attempts.
- Production startup guard for unsafe token secrets and memory storage.
- First administrator bootstrap command for SQL deployments.
- Recovery command for resetting an existing administrator password.
- SQL store transaction helper for multi-table writes.
- SQL write-error mapping for conflicts, references, and constraints.
- Environment-specific CORS policy with production wildcard rejection.
- Runtime metrics endpoint.
- Starter module scaffolding command with generated module metadata self-test.
- Validated module registry for built-in startup metadata.
- Local release package target with build metadata.
- Structured frontend API error handling and auth-expired session cleanup.
- OpenAPI-generated frontend endpoint metadata, including operation summaries, and stale-generation check.
- OpenAPI-generated frontend request wrapper functions used by pages and stores.
- OpenAPI-generated frontend JSDoc schema typedefs.
- OpenAPI-generated frontend request-body field metadata and client-side contract checks.
- OpenAPI-generated frontend response schema metadata and wrapper return JSDoc types for auth and core system JSON responses.
- OpenAPI response schema completeness check for non-text success responses.
- Backend-generated frontend permission metadata and permission usage drift check.
- Frontend API usage check for generated endpoint references and hardcoded API paths.
- Frontend PermissionGate and PermissionButton components for permission-aware rendering.
- Field-level validation error envelope and frontend core system form field mapping.
- Server-backed audit log filters and paginated frontend audit table.
- Server-backed user pagination with page-size controls and empty state.
- Dictionary-backed user status labels in filters, forms, and tables.
- Authenticated account profile update and password change.
- Minimum password length policy for user creation, admin password reset, and account password change.
- Audit logging for successful and failed account password changes.
- Permission-filtered menu tree in profile and frontend dynamic sidebar navigation.
- Menu management refreshes the current profile menu tree after successful writes.
- Frontend preferences for sidebar collapse and density, plus authenticated not-found routing.
- Contextual forbidden and not-found frontend states with permission-aware recovery actions.
- Module registry metadata for permissions, menus, frontend routes, and migration sets.
- Compile-time HTTP extension route registration with shared auth and global middleware.

Exit criteria:

- `go test ./...` passes in an environment with Go.
- `cd web && npm run lint && npm run build` passes.
- Design docs exist for architecture, boundaries, backend, frontend, database, and AI programming.

## Phase 1: SQL Persistence

Deliverables:

- PostgreSQL integration tests.
- Verified PostgreSQL integration test run in CI or a Docker-capable environment.
- SQL-backed HTTP integration tests for auth/profile, RBAC, dashboard, and core system writes.
- SQL store hardening for conflict handling and transactions.
- Optional SQLite development adapter.
- SQL seed and bootstrap review for production-safe behavior.

Exit criteria:

- Memory store remains usable for tests.
- SQL integration tests cover auth and system modules.
- Fresh database can be migrated and seeded.
- Server can run with SQL storage in a verified PostgreSQL environment.

## Phase 2: API and Frontend Contracts

Deliverables:

- OpenAPI spec.
- Typed frontend API client.
- Structured validation errors.
- Not-found and unauthorized pages.
- Permission gate/component.

Exit criteria:

- API spec matches route tests.
- Frontend no longer handcodes response shapes where generated types exist.

## Phase 3: Module Framework

Deliverables:

- Module registry.
- Backend route registry.
- Permission registry.
- Menu registry.
- Migration registry.
- Frontend route metadata registry.
- Route/menu/migration registry integration for scaffolded modules.

Exit criteria:

- A new module can be added without editing global switch statements.
- Module can register backend routes and frontend route metadata.

## Phase 4: Production Hardening

Deliverables:

- Refresh-token sessions.
- Password reset.
- Hardened production Docker deployment.
- Versioned upgrade process.

Exit criteria:

- Production deployment docs exist.
- Security-sensitive defaults are safe or clearly blocked.

## Phase 5: Advanced Framework Features

Deliverables:

- Code generator.
- Optional Casbin-compatible policy adapter.
- Background jobs.
- Notifications.
- Import/export.
- Plugin/event hooks.

Exit criteria:

- Extensions have stable public contracts.
- Core modules remain usable without optional features.
