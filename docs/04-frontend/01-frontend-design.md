# 04.01 Frontend Design

GOV2 frontend is a Vue 3 admin application built with Vite, Vue Router, Pinia, and icon components.

## Goals

- Work as a real admin console, not a marketing page.
- Keep screens dense, readable, and operational.
- Make permissions visible in routing and actions.
- Keep API contracts explicit and generated where possible.
- Allow future UI library replacement without rewriting business modules.

## Current Structure

```text
web
  index.html
  package.json
  vite.config.js
  src
    main.js
    App.vue
    api/client.js
    components/PermissionButton.vue
    components/PermissionGate.vue
    stores/app.js
    stores/preferences.js
    stores/session.js
    router/index.js
    layouts/AdminLayout.vue
    views
      AccountView.vue
      LoginView.vue
      DashboardView.vue
      UsersView.vue
      RolesView.vue
      MenusView.vue
      ModulesView.vue
      DictionariesView.vue
      SettingsView.vue
      AuditLogsView.vue
      ForbiddenView.vue
      NotFoundView.vue
    styles.css
```

## Target Structure

```text
src
  app
    providers
    config
  shared
    api
    components
    composables
    stores
    styles
    utils
  modules
    auth
    dashboard
    system-users
    system-roles
    system-menus
    system-dictionaries
    system-settings
    system-audit
  router
  layouts
```

Current flat structure is acceptable for MVP. Move to module folders when a second real business module is added.

## Routing

Rules:

- Public routes: login and future password reset.
- Protected routes: all admin routes under the main layout.
- Route metadata includes title and optional permission.
- Router guard loads profile before entering protected routes.
- Sidebar navigation is rendered from the permission-filtered menu tree returned by `/auth/profile`, with a static fallback for compatibility.
- Unknown protected routes should render a not-found view after auth.
- Forbidden and not-found views should show contextual route or permission details and only offer navigation actions the current profile can use.
- Expired authentication clears the session and returns the user to login.
- The admin shell persists sidebar and density preferences in local storage.

## API Client

The frontend API client lives at:

- `web/src/api/client.js`
- `web/src/api/endpoints.js` is generated from `api/openapi.yaml`
- `web/src/api/requests.js` is generated from `api/openapi.yaml`
- `web/src/api/types.js` is generated from `api/openapi.yaml`
- `web/src/permissions.js` is generated from `internal/domain/permissions.go`

It handles:

- JSON request/response envelopes
- successful JSON response envelope shape checks before data unwrap
- generated plain-text response handling for non-envelope operational endpoints
- structured JSON-envelope error parsing and envelope field type checks even for plain-text success endpoints
- `Content-Type: application/json` only on requests that send JSON bodies
- bearer token injection
- generated request wrapper functions for page/store calls
- generated endpoint metadata through `apiRequest(endpoint, options)`
- generated path, query, path/query parameter type, scalar path parameter, repeated query parameter item type, request-body presence, top-level body field, body field type, body enum value, body array scalar item, and body inline object array item contract checks before dispatch
- generated response schema presence, top-level object/array shape, generated field type and enum checks, generated array scalar item type checks, generated nested object field shape, generated array item object shape, paginated/list item object shape, and required response field checks after dispatch
- AbortSignal forwarding for request cancellation
- generated response schema metadata and JSDoc return types where OpenAPI declares response data schemas
- structured `ApiError` values with `status`, `code`, `requestID`, and `data`
- `validationErrorsByField(error)` for `data.fields[]` form validation responses
- automatic token removal and `gov2:auth-expired` event dispatch on authenticated `401` responses

Endpoint generation commands:

```bash
cd web
npm run api:generate
npm run api:check
```

Pages and stores should call documented APIs through generated functions from `@/api/requests` instead of calling `apiRequest(...)` directly, importing generated endpoint metadata, using raw browser or third-party request transports, or hardcoding `/api/v1` paths.
Generated endpoint metadata includes method, path, OpenAPI operation summary, public access, permission, path params, query params, generated path/query parameter types, request body presence, request body schema name, response schema name, allowed top-level body fields, and required top-level body fields.
Generated request wrapper JSDoc exposes OpenAPI operation summaries, path
params, query params with generated OpenAPI parameter types, request body
schemas, optional auth override, `AbortSignal`, and response return assistance
in the JavaScript frontend.
Generated response typedefs support object schemas, paginated response objects, and top-level array aliases for list endpoints.
Generated permission metadata exposes backend-registered permission codes through `permissions.*`; the wildcard grant must use `permissions.all` rather than a hardcoded `"*"`.
`npm run api:check` verifies that generated endpoint, request wrapper, schema type, and permission metadata are current, operations document non-empty `summary` values, path/query parameters declare supported generated runtime types that Go contract tests also compare with backend route metadata, public operations do not declare `x-permission`, operations document `2xx` success responses, success responses use supported media types, non-text success responses declare response schemas, operation response component references exist, response component schema references exist, standard error response components use `application/json` and the standard envelope schemas, nested schema references exist, schema required fields are declared in their object properties, array schemas declare item types, schema properties generate concrete frontend types or explicitly document arbitrary JSON values, request-body routes document `required: true`, `application/json` content, and generated schema references, authenticated routes document exact `401 UnauthorizedError`, permission-protected routes document exact `403 ForbiddenError`, request-body routes document `400`, a standard `400 BadRequestError` or `400 ValidationError` component, exact `413 PayloadTooLargeError`, and exact `415 UnsupportedMediaTypeError`, routes without request bodies do not document `413` or `415`, endpoint body and response schemas exist, every endpoint has a generated request wrapper, each generated request wrapper keeps canonical generated imports and calls its same-named endpoint metadata, generated request wrappers keep the `options = {}` default parameter, generated request wrapper JSDoc includes the OpenAPI summary, top-level options object, generated path/query parameter types, request body type, return type, and optional `AbortSignal`, application source imports only existing generated request wrappers by name, view-level generated GET requests pass a `signal` option, all generated endpoint references and imported permission object references exist, permission literals are registered by the backend, wildcard grants use `permissions.all`, application source uses generated request wrappers instead of direct `api(...)` or `apiRequest(...)` calls, and application source does not reintroduce raw request transports, generated endpoint metadata imports, relative or aliased imports that resolve to restricted API modules, or hardcoded `/api/v1` literals.

Pages may continue to show `err.message`, while advanced flows can inspect `err.status` or `err.requestID`.

Future dynamic routes:

```js
{
  path: "/system/users",
  component: UsersView,
  meta: {
    title: "Users",
    permission: "system:user:list"
  }
}
```

For future dynamic route tooling, backend menu/profile `component` strings must match the frontend route metadata and imported view component identifiers for the same path.
Backend built-in frontend route metadata must also match the actual protected Vue Router routes; normal Go verification reads `web/src/router/index.js` and compares path, component, title, and permission metadata.

Frontend permissions improve UX only. Backend permissions remain authoritative.

## State Management

Pinia stores should be small:

- `app`: public app config, site title, brand initials, and document title support.
- `session`: token, profile, login/logout, self-service profile update, and password change.
- `preferences`: density and sidebar state.
- module stores only when multiple views share state.

Avoid global stores for one-table local state.

The `app` store loads `/api/v1/app/config` without authentication during application startup. Login and admin layout branding must read from this store instead of hardcoding the site title.

## API Client

The API client owns:

- base request behavior
- JSON parsing
- bearer token attachment
- response envelope unwrap
- error message extraction

Node-based frontend tests cover API client query building, generated endpoint
summary metadata validation, generated endpoint metadata validation, scalar path
parameter enforcement, repeated query parameter item type validation, JSON body
dispatch, successful JSON response envelope shape validation, structured JSON
error response envelope validation, generated response schema checks for
top-level response fields, field types, enum values, generated nested object
fields, generated body and response array item types, and generated list/page
item objects,
public endpoint auth suppression, AbortSignal forwarding, authenticated `401`
expiration handling, and field-level validation error mapping.
Dashboard, module metadata, and core system management list refreshes pass
AbortSignals through generated API requests so superseded refreshes and unmounted
views do not let stale responses overwrite current state.

It must not own:

- UI notifications
- route redirects
- permission decisions

Future:

- Expand OpenAPI response schemas until all frontend workflows have generated return types.
- Add retry only for idempotent GET requests.

## UI System

Visual direction:

- Calm operational interface.
- Dense tables.
- Clear forms.
- Compact side navigation.
- Minimal decorative elements.
- Consistent 6-8px radius.

Use icons for actions where a familiar icon exists. Use text buttons for commands that need clarity.
Disabled controls should use normal disabled affordances; only controls blocked by an in-flight request should expose `aria-busy` and a busy cursor.

Design tokens:

- `--bg`
- `--surface`
- `--ink`
- `--muted`
- `--line`
- `--accent`
- semantic colors for success, warning, danger

Future component groups:

- `DataTable`
- `SearchBar`
- `PermissionButton`
- `StatusBadge`
- `ConfirmDialog`
- `FormField`
- `PageHeader`

## Tables

Admin tables should support:

- server pagination
- keyword search
- column density
- row actions
- loading and empty states
- stable widths

High-volume tables, such as audit logs, should use cursor pagination later.

Current audit log management supports server-side keyword, actor, action, resource, and resource ID filters with page-size controls, request cancellation for superseded list refreshes, and explicit empty/error states.

Current dashboard management supports loading placeholders for summary metrics and explicit error display when summary loading fails.

Current account management supports self-service profile update and password change without system user management permissions, including field-level validation error display for password changes.

Current menu management supports:

- tree flattening for table display
- parent selection
- create and update form
- profile menu tree refresh after successful menu writes
- field-level validation error display for menu form writes
- explicit load error display for menu list refresh
- permission-gated row actions
- delete protection enforced by backend
- explicit empty state when no menus are available

Current role management supports:

- role create, update, and delete
- permission catalog loading from `/api/v1/system/permissions`
- grouped permission checkboxes
- `*` permission selection that clears narrower permissions
- field-level validation error display for role form writes
- explicit load error display for role and permission refresh
- permission-gated row actions
- explicit empty state when no roles are available

Current setting management supports:

- setting list, create, update, and delete
- JSON value editing with client-side JSON validation before API dispatch
- field-level validation error display for setting form writes
- explicit load error display for setting list refresh
- permission-gated row actions
- explicit empty state when no settings are available

Current user management supports:

- user create, update, delete, and status toggle
- role selection from the role API
- status filtering and keyword search
- server pagination, page-size controls, request cancellation for superseded list refreshes, and empty state
- user status labels and ordering from `/api/v1/system/dictionaries/code/user_status` with enum-safe fallback
- role names in tables instead of raw role IDs
- field-level validation error display for user form writes
- permission-gated row actions
- disabled UI actions for attempts to delete, disable, or remove the admin role from the last active administrator

Current dictionary management supports:

- dictionary create, update, and delete
- inline dictionary item add/remove
- item label, value, and sort editing
- field-level validation error display for dictionary form writes
- explicit load error display for dictionary list refresh
- permission-gated card actions
- explicit empty state when no dictionaries are available

Current route error states support:

- forbidden route context with the missing permission code
- not-found route context with the requested path
- permission-aware recovery links for protected admin routes

Current module management supports:

- module metadata listing from `/api/v1/system/modules`
- registered permission, backend API, menu, frontend route, and migration metadata counts
- backend API route metadata display for module operators and future tooling
- frontend route metadata display for module operators and future tooling
- explicit load error display for module metadata refresh
- explicit empty state when no modules are available

## Forms

Rules:

- Form validation exists on both frontend and backend.
- Frontend validation is for speed and clarity.
- Backend validation is authoritative.
- Error messages should map to fields when possible.

## Permission Rendering

Recommended pattern:

```vue
<PermissionButton permission="system:user:create">
  Create User
</PermissionButton>
```

`PermissionButton` wraps button actions and hides unauthorized actions by default. It also supports `mode="disable"` when a screen should show a disabled action instead of hiding it. `PermissionGate` remains available for non-button content. Both read from `session.permissions` and must not bypass backend checks.

## Build and Delivery

Development:

```bash
cd web
npm run dev
```

Production:

```bash
cd web
npm run build
```

Go serves `web/dist` by default.

## Testing

Minimum frontend gates:

```bash
cd web
npm run test
npm run lint
npm run build
```

Future:

- component tests
- router guard tests
- Playwright smoke tests for login and core pages
