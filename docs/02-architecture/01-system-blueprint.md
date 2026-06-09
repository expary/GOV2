# 02.01 System Blueprint

GOV2 is a free, open, MIT-licensed system framework for building admin systems, internal tools, SaaS back offices, and operational platforms with Go and Vue.

The framework should feel like a product-ready starter without becoming a locked commercial template. It should be easy to run as a single binary, easy to extend as modules, and safe to evolve into a larger service.

## Reference Projects

GOV2 learns from public projects at the architecture-principle level. See [Reference Projects](../01-research/01-reference-projects.md) for links and license boundaries.

- Kubernetes: stable resource-oriented APIs, versioning discipline, and declarative object conventions.
- Grafana: plugin isolation, permissions around user-facing capabilities, and operational observability.
- PocketBase: single-binary Go developer experience and pragmatic embedded-backend ergonomics.
- Supabase: Postgres-first data design, open platform mindset, and clear product APIs.
- Casbin: RBAC/ABAC policy modeling ideas.
- Ant Design Pro and other enterprise dashboards: dense, role-aware, workflow-oriented admin UX.

These references are not implementation sources. GOV2 must not copy code, assets, schemas, generated files, private conventions, or license-restricted material from them.

## Product Scope

GOV2 should provide these framework capabilities:

- Authentication: password login, token sessions, refresh-token plan, password reset plan, optional OAuth/OIDC adapters.
- Authorization: RBAC first, policy extension path for ABAC/resource permissions.
- System management: users, roles, permissions, menus, dictionaries, settings, audit logs.
- Data foundation: SQL persistence, migrations, seed data, audit fields, soft delete policy.
- API foundation: versioned REST API, typed responses, validation, OpenAPI export plan.
- Frontend foundation: Vue 3 admin shell, route guards, permission rendering, theme tokens, reusable tables/forms.
- Developer tooling: code generation, module scaffolding, test fixtures, docs, CI, Docker Compose.
- AI-ready development: explicit boundaries, module rules, prompt-safe docs, and review gates.

## Non-Goals

- GOV2 is not a low-code platform first. Low-code tools can be added after core framework stability.
- GOV2 is not tied to one UI component vendor.
- GOV2 is not a microservice framework by default. The default deployment is a modular monolith.
- GOV2 must not create paid-only core features.

## Deployment Shape

The default architecture is:

```text
Browser
  |
Vue Admin App
  |
Versioned HTTP API
  |
Go Application Services
  |
Domain + Policy + Events
  |
Repositories
  |
PostgreSQL / SQLite / MySQL-compatible adapter
```

The recommended first production target is one Go process serving:

- API routes under `/api/v1`.
- Built Vue assets from `web/dist`.
- One SQL database.
- Optional background workers in the same process.

Larger deployments can split background workers and API servers later without changing domain contracts.

## Repository Shape

Current MVP layout:

```text
cmd/gov2                 executable entrypoint
config                   config examples
internal/app             application composition
internal/config          config loading
internal/domain          core domain models
internal/httpapi         HTTP routes, middleware, responses
internal/security        password, tokens, RBAC primitives
internal/service         use-case services
internal/store/memory    temporary in-memory store
web                      Vue 3 admin app
docs                     framework design
```

Target layout after persistence and modules:

```text
cmd/gov2
internal/app
internal/domain
internal/module
internal/service
internal/repository
internal/store/sql
internal/httpapi
internal/security
internal/event
internal/plugin
pkg/gov2
web
docs
```

`pkg/gov2` should only expose stable extension APIs. Most implementation stays in `internal`.

## Module Model

A GOV2 module is a cohesive business capability:

- Domain types and rules.
- Service/use-case methods.
- Repository interfaces and SQL implementation.
- HTTP routes.
- Frontend routes/views/components.
- Permission codes.
- Optional seed data and migrations.

System modules are built in:

- `auth`
- `users`
- `roles`
- `menus`
- `dictionaries`
- `settings`
- `audit`

Business modules should follow the same structure but must not depend on HTTP or Vue internals.

## API Model

APIs are versioned from day one:

- Current base path: `/api/v1`.
- Response envelope: `code`, `message`, `data`, `request_id`.
- Resource names use plural nouns.
- Actions that are not CRUD use explicit subresources, for example `/users/{id}/status`.
- Breaking changes require a new version path or an explicit migration note.

API objects should be stable and boring. Avoid leaking database internals, password hashes, private token claims, or frontend-only state.

## Permission Model

Permission codes use namespaced verbs:

```text
dashboard:view
system:user:list
system:user:create
system:user:update
system:user:delete
system:role:list
system:menu:list
system:dictionary:list
system:setting:list
system:audit:list
```

RBAC is the default. The policy engine must allow future resource-level checks:

```text
subject + action + resource + context -> allow/deny
```

## Extensibility

GOV2 extension points should be explicit:

- Service interfaces for modules.
- Repository interfaces for persistence adapters.
- Event hooks for audit, notification, and async jobs.
- Frontend route registry for dynamic module pages.
- Permission registry for menu and button-level checks.

Avoid extension by modifying global state from random packages.

## Observability

Every production request should have:

- Request ID.
- Access log.
- Structured error.
- Actor identity when authenticated.
- Audit log for sensitive operations.

Future additions:

- Trace propagation.
- Health and readiness checks.
- Background job status.

## Security Principles

- Passwords are hashed, never encrypted or stored in plaintext.
- Token secrets must be configured outside source code.
- Admin default credentials are development-only and must be rotated in production.
- CORS defaults should be permissive only in development.
- RBAC checks happen on the server even when the frontend hides UI.
- Audit logs record security-sensitive changes.

## Roadmap Summary

1. Keep MVP running with memory store and Vue shell.
2. Introduce SQL repository interfaces and migrations.
3. Add OpenAPI and typed frontend API generation.
4. Add module scaffolding and code generation.
5. Add plugin/event extension points.
6. Add production packaging, CI, Docker Compose, and deployment docs.
