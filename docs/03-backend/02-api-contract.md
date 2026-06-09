# 03.02 API Contract

GOV2 API contracts are versioned under `/api/v1`.

## OpenAPI

The initial OpenAPI draft lives at:

- `api/openapi.yaml`

It currently documents:

- operational endpoints, including health, readiness, and metrics
- public application configuration
- auth, including login, logout, profile, profile update, and password change
- dashboard
- users, including create, update, status change, delete, and role assignment
- roles, including create, update, delete, and permission catalog
- menus, including create, update, and delete
- modules, including permissions, backend route metadata, menu entries, frontend route metadata, and migration metadata
- dictionaries, including create, update, delete, and item replacement
- settings, including JSON value create, update, and delete
- audit logs

Core JSON endpoints declare OpenAPI success response schemas for the unwrapped `data` value. User and audit-log lists use paginated response objects; role, permission, menu, module, dictionary, and setting list endpoints use typed array response schemas.

## Operational Endpoint Contract

Operational endpoints expose:

- `GET /api/v1/health` is public and reports process liveness.
- `GET /api/v1/ready` is public and reports dependency readiness.
- `GET /api/v1/metrics` is public and returns Prometheus-compatible text.

Readiness returns `200` when the configured repository store is usable and `503`
when a required dependency is unavailable. It must not expose DSNs, credentials,
or secret configuration values.

## Application Config Contract

Application configuration exposes:

- `GET /api/v1/app/config` is public.

The response is an explicit allowlist with `name`, `title`, and `environment`. `title` is read from the `site.title` setting when it is a non-empty JSON string, then falls back to the configured app name. This endpoint must not expose arbitrary settings or secrets.

## Auth Contract

Authentication exposes:

- `POST /api/v1/auth/login` is public.
- `POST /api/v1/auth/logout` requires an authenticated user.
- `GET /api/v1/auth/profile` requires an authenticated user.
- `PUT /api/v1/auth/profile` requires an authenticated user.
- `PUT /api/v1/auth/password` requires an authenticated user.

Login returns a token, token expiration timestamp, and public user data. Profile reads return the public user, role IDs, normalized permission codes with empty values removed, and a permission-filtered menu tree for the current user. If the aggregated role permissions include the `*` wildcard grant, profile permissions return `*` as the exclusive permission entry.

Login failures keep the same public `401` response shape as invalid credentials and write a `login_failed` audit log without storing plaintext passwords.

Profile update is self-service only and may update nickname, email, phone, and avatar. Duplicate profile email or phone values return HTTP `409`. Password change requires the current password, validates the new password against the server password policy, returns field-level validation errors for invalid new passwords, writes an audit log on success, and writes a `password_failed` audit log when the current password check fails.

## Dashboard Contract

Dashboard exposes:

- `GET /api/v1/dashboard/summary` requires `dashboard:view`.

Dashboard summary returns aggregate user, active-user, role, and audit-log counts for the current system store. It is intended for the admin overview page and must not expose sensitive record contents.

## User Contract

User management exposes:

- `GET /api/v1/system/users` requires `system:user:list`.
- `POST /api/v1/system/users` requires `system:user:create`.
- `GET /api/v1/system/users/{id}` requires `system:user:list`.
- `PUT /api/v1/system/users/{id}` requires `system:user:update`.
- `PATCH /api/v1/system/users/{id}/status` requires `system:user:update`.
- `DELETE /api/v1/system/users/{id}` requires `system:user:delete`.

User reads support `page`, `page_size`, `keyword`, and `status`.

User writes validate username, password on create or password update, status values, and role IDs. Status values are trimmed before validation. New passwords must be at least 8 characters and contain a non-whitespace character. Role IDs are deduplicated and must reference existing roles. Duplicate usernames, emails, and phone numbers return HTTP `409`. User update, status change, and delete operations must not remove, disable, or delete the last active user assigned to the `admin` role; these attempts return HTTP `409`.

User create, update, and status-write validation errors return HTTP `400` with field details in `data.fields`. Core system writes for roles, menus, dictionaries, and settings use the same shape:

```json
{
  "code": 400,
  "message": "invalid input",
  "data": {
    "fields": [
      { "field": "username", "message": "Username is required" }
    ]
  },
  "request_id": "..."
}
```

New modules should use the same field error shape when a form can map errors back to inputs.

## Role Contract

Role management exposes:

- `GET /api/v1/system/roles` requires `system:role:list`.
- `POST /api/v1/system/roles` requires `system:role:create`.
- `PUT /api/v1/system/roles/{id}` requires `system:role:update`.
- `DELETE /api/v1/system/roles/{id}` requires `system:role:delete`.
- `GET /api/v1/system/permissions` requires `system:role:list`.

Role writes validate required name and code fields. Permission codes are
trimmed, empty values are discarded, duplicates are removed, and `*` is stored
as the exclusive wildcard grant when present. Non-wildcard permission codes must
be registered by the active module permission catalog; unknown permissions
return field-level validation under `permissions`. Duplicate role codes return
HTTP `409`. The repository prevents deleting a role that is still assigned to
any non-deleted user, which also returns HTTP `409`.
Role create and update return field-level validation errors for invalid form input.

## Module Contract

Module listing exposes:

- `GET /api/v1/system/modules` requires `system:module:list`.

Module responses describe compile-time extension metadata: permissions, backend routes, menu entries, frontend routes, and migration sets. This endpoint is informational for operators and future module tooling; it does not dynamically enable routes or run migrations by itself.

Compile-time extension handlers can be registered through `httpapi.Options.Routes`. These handlers are not part of the static OpenAPI file until the module author documents them, but they use the same authentication, permission, and global middleware behavior as built-in routes. A non-empty extension route `Permission` always makes the route authenticated and RBAC-protected, even if `Public` is also set.

## System Menu Contract

Menu management is part of the RBAC surface:

- `GET /api/v1/system/menus` requires `system:menu:list`.
- `POST /api/v1/system/menus` requires `system:menu:create`.
- `PUT /api/v1/system/menus/{id}` requires `system:menu:update`.
- `DELETE /api/v1/system/menus/{id}` requires `system:menu:delete`.

Menu writes validate required title, name, and path fields. When `permission` is set, it must reference a permission in the active backend permission catalog. The service rejects paths that do not start with `/`, self-parenting, and parent-child cycles. Duplicate menu names return HTTP `409`. The repository prevents deleting a menu that still has children, which also returns HTTP `409`.
Menu create and update return field-level validation errors for invalid form input.

## Dictionary Contract

Dictionary management exposes:

- `GET /api/v1/system/dictionaries` requires `system:dictionary:list`.
- `GET /api/v1/system/dictionaries/code/{code}` requires an authenticated user.
- `POST /api/v1/system/dictionaries` requires `system:dictionary:create`.
- `PUT /api/v1/system/dictionaries/{id}` requires `system:dictionary:update`.
- `DELETE /api/v1/system/dictionaries/{id}` requires `system:dictionary:delete`.

Dictionary reads by code are intended for business pages that need label/value options without dictionary management access.

Dictionary writes validate required code and name fields. Dictionary item labels and values are required when items are provided, and item values must be unique within the dictionary. Duplicate dictionary codes return HTTP `409`.
Dictionary create and update return field-level validation errors for invalid form input.

## Setting Contract

Setting management exposes:

- `GET /api/v1/system/settings` requires `system:setting:list`.
- `POST /api/v1/system/settings` requires `system:setting:create`.
- `PUT /api/v1/system/settings/{id}` requires `system:setting:update`.
- `DELETE /api/v1/system/settings/{id}` requires `system:setting:delete`.

Setting writes validate required keys and require `value` to be valid JSON. Empty values default to `{}`. Duplicate setting keys return HTTP `409`.
Setting create and update return field-level validation errors for invalid form input.

## Audit Log Contract

Audit log management exposes:

- `GET /api/v1/system/audit-logs` requires `system:audit:list`.

Audit log reads support `page`, `page_size`, `keyword`, `actor`, `action`, `resource`, and `resource_id`. `resource_id` matches exact resource identifiers. Keyword search covers actor, action, resource, resource ID, IP, user agent, and detail.

## Rules

- Paginated list endpoints normalize `page < 1` to `1`, default invalid or
  missing `page_size` to `20`, and cap `page_size` at `100`.
- Protected endpoints must declare a permission in `x-permission`.
- Authenticated endpoints that do not require a named permission rely on the global bearer security declaration and must not declare `security: []`.
- HTTP routes with no named permission must be intentionally authenticated-only; the RBAC policy denies empty permissions outside the HTTP route-authentication layer.
- Public endpoints must declare `security: []`.
- Public endpoints must not declare `x-permission`; otherwise generated frontend
  endpoint metadata would simultaneously mark the operation as public and
  permission-protected.
- Authenticated endpoints must document `401 UnauthorizedError`. Endpoints with
  `x-permission` must also document `403 ForbiddenError`.
- OpenAPI operations must document a non-empty `summary` matching the backend
  route registry so generated and browsed API references remain readable.
- API response envelopes use `code`, `message`, `data`, and `request_id`.
- Clients may send `X-Request-ID` with letters, digits, `-`, `_`, `.`, or `:` up to 128 characters. Missing or invalid request IDs are replaced by the server and exposed through the response header and envelope.
- JSON request endpoints require `Content-Type: application/json` or a
  compatible `application/*+json` media type. Missing, malformed, or non-JSON
  media types return HTTP `415` with message `unsupported media type`.
- JSON request bodies are limited to 1 MiB. Oversized JSON requests return HTTP
  `413` with the standard response envelope and message `request body too large`.
- JSON request bodies must contain exactly one top-level JSON value. Trailing
  whitespace is allowed, but trailing JSON values are rejected with HTTP `400`.
- OpenAPI operations with `requestBody` must document `required: true` so
  generated frontend request wrappers reject missing bodies before dispatch.
- OpenAPI operations with `requestBody` must document an `application/json`
  content entry because generated frontend request wrappers send JSON bodies.
- OpenAPI operations with `requestBody` must declare an `application/json` schema
  reference to a component schema so generated frontend request wrappers receive
  a stable request DTO type.
- OpenAPI operations with `requestBody` must document `400`, `413`, and `415`
  error responses for bad requests, body size failures, and media-type failures.
- OpenAPI request-body operations that can return field-level service validation
  errors must document `400 ValidationError`; other request-body operations must
  document `400 BadRequestError`. Backend route metadata is the source for this
  distinction.
- OpenAPI request-body operations must document `413 PayloadTooLargeError` and
  `415 UnsupportedMediaTypeError` through those exact response components.
- OpenAPI operations without `requestBody` must not document `413` or `415`
  JSON request-body error responses.
- OpenAPI operations that can hit repository or protected-resource conflicts
  must document `409 ConflictError`; operations without conflict semantics should
  not document `409`.
- OpenAPI operations that can return `repository.ErrNotFound` must document
  `404 NotFoundError`; operations without record lookup semantics should not
  document `404`.
- OpenAPI success response schemas document the unwrapped `data` value consumed by generated frontend request wrappers.
- OpenAPI operations must document at least one `2xx` success response.
- OpenAPI success response schemas may be object schemas, paginated response objects, or top-level array aliases.
- OpenAPI success responses consumed by generated frontend request wrappers must
  use `application/json`, except plain-text operational endpoints such as
  metrics, which must use `text/plain` as the only success response media type.
- OpenAPI `$ref` values under operation responses must reference declared
  `components.responses` entries.
- OpenAPI `$ref` values inside `components.responses` must reference declared
  schema components.
- Standard OpenAPI error response components must document `application/json`
  content and reference the standard envelope schemas: `ErrorEnvelope` for
  generic errors and `ValidationErrorEnvelope` for field validation errors.
- OpenAPI `$ref` values inside `components.schemas` must reference declared
  schema components.
- OpenAPI path and query parameters must declare scalar schema types supported
  by generated frontend runtime checks: `integer`, `number`, `string`, or
  `boolean`.
- OpenAPI path and query parameter names and scalar schema types must match the
  backend route registry metadata exactly, so generated frontend parameter
  validation cannot drift from HTTP route expectations.
- OpenAPI schema `required` entries must name fields declared under the same
  object schema `properties` block.
- OpenAPI schema properties must declare a generated type with `type`, `$ref`,
  `enum`, or typed `items`, unless the field explicitly documents that it holds
  any valid JSON value.
- OpenAPI array schemas must declare `items` so generated frontend typedefs do
  not silently produce `Array<unknown>`.
- High-volume or user-generated list endpoints should use paginated response objects. Small bounded metadata and configuration lists may use top-level array aliases when the contract documents them explicitly.
- Non-text OpenAPI `2xx` responses must declare a known success response schema so generated frontend wrappers do not silently fall back to `unknown`.
- Field-level validation errors use `data.fields[]` with `field` and `message`.
- Frontend API errors must preserve HTTP status and `request_id` for support and diagnostics.
- DTOs should not expose database records directly.
- Breaking changes require a versioned API path or a migration note.

## Verification

`go test ./...` includes an OpenAPI contract test that compares `api/openapi.yaml` with the backend route registry. The test checks documented method/path coverage, operation summaries against route metadata, public route security overrides, named `x-permission` values, permission registration, query parameter declarations and exact path/query parameter scalar schema types against backend route metadata, path/query parameter scalar schema types supported by generated frontend runtime checks, request-body presence and schema names against backend route metadata, success status codes against backend route metadata, success response schema names against backend route metadata, exact `401 UnauthorizedError` and `403 ForbiddenError` security response components for protected routes, `404` not-found responses and conflict responses against backend route metadata, required request bodies, `application/json` request-body content, request-body schema declarations and references, documented `2xx` success responses, supported success response media types, direct success response schema references, non-text success response schema declarations, exact `413`/`415` JSON request-body error response components, `400 BadRequestError` or `400 ValidationError` response components against backend route metadata, referenced response components, standard error response component JSON envelope schemas, nested schema component references, and schema required-field declarations.

`cd web && npm run api:check` verifies that frontend endpoint metadata, request wrapper functions, and JSDoc schema typedefs generated from `api/openapi.yaml` are current. The generated metadata and request wrapper JSDoc include OpenAPI operation summaries; generated metadata also includes documented path params, query params, generated path/query parameter types, request-body presence, request-body schema names, response schema names, and top-level request-body fields used by `apiRequest`. It fails when an operation omits a non-empty `summary`, when a path or query parameter omits a supported scalar schema type, when a public operation declares `x-permission`, when an operation omits a `2xx` success response, when a success response uses an unsupported media type, when a non-text success response omits a known response schema, when an operation references an unknown response component, when a response component references an unknown schema, when a standard error response component omits `application/json` or its standard envelope schema, when a schema component references an unknown schema, when a schema required field is not declared in the same object properties, when an array schema omits `items`, when a schema property would generate `unknown` without explicitly documenting an arbitrary JSON value, when an authenticated route omits `401 UnauthorizedError`, when a permission-protected route omits `403 ForbiddenError`, when a request-body route omits `required: true`, when a request-body route omits `application/json` content, when a request-body route omits a generated schema reference, when a request-body route omits `400`, `413`, or `415`, when a request-body route omits a `400 BadRequestError` or `400 ValidationError` response component, or when a request-body route omits exact `413 PayloadTooLargeError` or `415 UnsupportedMediaTypeError` response components.

The same check verifies that `web/src/permissions.js` is generated from `internal/domain/permissions.go`, frontend permission references, including local aliases of the generated permission object, use backend-registered permission codes, wildcard grants use `permissions.all`, and application code calls documented APIs through generated request wrappers instead of low-level transports or generated endpoint metadata.

The frontend runtime API client uses generated response schema metadata to check
the unwrapped `data` value returned by generated request wrappers. It verifies
top-level array responses are arrays, top-level object responses are objects,
and required object fields are present before page code consumes the response.
