# 02.03 Module System

GOV2 modules are the main extension unit for framework and business capabilities.

## Current Implementation

The current code includes:

- `internal/module/registry.go`
- built-in module registration in `internal/app/app.go`
- module listing API: `GET /api/v1/system/modules`
- frontend module page: `/system/modules`
- module scaffold command: `gov2 module scaffold`
- module metadata validation for built-in startup registration

## Module Metadata

A module has:

- `name`
- `title`
- `version`
- `description`
- `permissions`
- backend route metadata
- menu entries
- frontend route metadata
- migration sets

This metadata is intentionally declarative. It gives GOV2 a stable compile-time extension point before adding dynamic plugins or generated wiring.
Application startup uses a validated module registry for built-in modules. The
validator rejects module names that do not match `^[a-z][a-z0-9_]*$`,
duplicate module names, duplicate permission codes, duplicate backend route
names or method/path patterns, duplicate menu names or paths, duplicate frontend
route names or paths, duplicate migration sets, permission records whose
`module` value does not match the owning module name, permission codes that do
not use the owning module namespace prefix, backend routes without summaries,
backend routes with unsupported or non-uppercase HTTP methods, backend route
paths outside `/api/v1`, backend route paths outside the owning module API
namespace, public backend routes that also declare permissions, non-absolute
menu or frontend route paths, menu or frontend route paths outside the owning
module path namespace, unknown permission references, permission references to
permissions owned by a different module, menu parent references that do not point
at a registered menu or point at a menu owned by a different module, and
menu/frontend route component mismatches for the same path.
Built-in backend route metadata must stay aligned with the registered HTTP route specs and OpenAPI operations; normal Go verification compares method, path, public flag, permission values, and OpenAPI operation summaries for protected system routes.

When a menu entry and a frontend route metadata record describe the same path, their `component` names must match the actual frontend view registration. Menu `component` values are UX metadata for navigation and future dynamic route tooling; they are not authorization inputs.
Registered backend routes for a module named `<name>` must live at
`/api/v1/<name>` or below it. Registered menu and frontend route paths must live
at `/<name>` or below it.
Backend routes, menu entries, and frontend routes with a non-empty `permission`
must reference a permission declared by the same owning module.
Menu entries with a non-empty `parent` must reference a menu declared by the
same owning module.
The active module registry also supplies the role-management permission catalog:
`GET /api/v1/system/permissions` lists registered module permissions, and role
create/update rejects non-wildcard permission codes that are not present in that
catalog.

## Built-In Modules

- `dashboard`
- `system`

The system module currently owns users, roles, menus, modules, dictionaries, settings, and audit logs.

## Module Scaffold Command

Create a starter business module with:

```bash
go run ./cmd/gov2 module scaffold --name inventory --title Inventory
```

Registered and scaffolded business module names must match
`^[a-z][a-z0-9_]*$` and scaffolded names must not use GOV2 built-in module or
API namespace names: `app`, `auth`, `audit`,
`dashboard`, `dictionaries`, `menus`, `roles`, `settings`, `system`, or `users`.

The command writes:

- `modules/<name>/backend/module.go`
- `modules/<name>/backend/module_test.go`
- `modules/<name>/frontend/<Title>View.vue`
- `modules/<name>/migrations/000001_init.up.sql`
- `modules/<name>/migrations/000001_init.down.sql`
- `modules/<name>/README.md`

Generated files are intentionally not auto-registered. The developer must review the module boundary, add route/service/repository tests, and then wire the module into application startup and frontend routing.

The generated backend module includes starter permission definitions, backend route metadata, one menu entry, one frontend route metadata record, and a PostgreSQL migration directory reference. It also includes a metadata test that runs `module.ValidateModules(Module())` so registry contract drift is caught with `go test ./modules/<name>/backend`. These are metadata contracts only; the developer still owns route handlers, service/repository implementation, migration content, and frontend view wiring.
The generated frontend starter uses existing GOV2 layout classes and should be expanded inside the normal frontend module boundaries.

HTTP delivery can register compile-time extension handlers through `httpapi.Options.Routes`. Routes with `Public: true` and an empty `Permission` skip bearer authentication but still pass through global middleware such as request IDs, access logs, CORS, and panic recovery. Routes with `Public: false` and an empty `Permission` require only an authenticated active user. Routes with a non-empty `Permission` always require bearer authentication and RBAC authorization, even if `Public` is set accidentally. `httpapi.New` uses a discard logger when no logger is supplied, so extension tests can build routers without custom logging setup.

## Future Module Contract

Modules can now describe:

- backend route metadata
- permission definitions
- menu entries
- migrations
- frontend route metadata

Future modules should also register:

- seed data
- background jobs

Dynamic binary plugins are not part of the first production target. GOV2 should stabilize compile-time module registration first.
