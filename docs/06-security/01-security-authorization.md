# 06.01 Security And Authorization

Security is a first-class framework layer. GOV2 must enforce permissions on the backend even when the frontend hides routes or buttons.

## Scope

This document covers:

- authentication
- sessions and tokens
- RBAC
- future ABAC/resource policies
- audit logs
- security defaults

## Authentication

MVP:

- username and password login
- salted password hash
- minimum password policy for user creation, admin password reset, and self-service password change
- signed bearer token
- bearer token verification checks signature, issuer, expiration, required claims, and the signed GOV2 HS256 header
- audit logging for successful and failed login attempts
- login treats missing users and bad passwords as invalid credentials, while
  storage lookup failures propagate as service errors instead of being hidden as
  credential failures
- self-service password change with current password verification and success/failure audit logging

Production target:

- short-lived access token
- refresh token stored as a hash
- session revocation
- password reset flow
- optional OAuth/OIDC adapters
- login history and suspicious login audit

## Authorization

Default model:

```text
user -> roles -> permissions
route -> required permission
```

Permission code format:

```text
module:resource:action
```

Examples:

```text
system:user:list
system:user:create
system:role:update
system:audit:list
```

Registered module permissions must use the owning module namespace prefix. For
example, permissions declared by the `inventory` module must start with
`inventory:`.
Module route, menu, and frontend route metadata must reference permissions owned
by the same module; cross-module permission references are rejected during module
registry validation.
Role writes must reference permissions from the active module permission catalog,
except for the explicit `*` wildcard grant. Unknown role permission codes are
rejected before persistence so frontend typos or stale clients cannot create
latent grants for future routes.
Menu writes with a non-empty permission must also reference the active backend
permission catalog. Unknown menu permission codes are rejected before
persistence so stale navigation metadata cannot create latent grants for future
routes.
RBAC policy evaluation and profile permission aggregation trim permission codes
and ignore empty values before matching or returning them to clients. Profile
permission aggregation returns the `*` wildcard grant as the exclusive
permission entry when any assigned role grants it.

## Policy Extension

RBAC is enough for the first system framework version. Resource-level policies should use a future interface:

```text
Can(actor, action, resource, context) -> decision
```

This keeps the door open for ABAC or Casbin-compatible adapters without forcing them into MVP.

## Frontend Permission Rules

Frontend may:

- hide routes
- hide buttons
- show disabled states
- improve navigation clarity

Frontend must not:

- be trusted as the permission enforcement layer
- infer permissions from token internals
- invent permission codes not registered by backend modules

## Audit

Audit logs are required for:

- login and logout
- user creation/update/delete
- role and permission changes
- menu changes
- dictionary/settings changes
- failed sensitive operations when practical

Current MVP route handlers write audit records for successful login/logout, failed login attempts, successful and failed self-service password changes, and successful core system mutations across users, roles, menus, dictionaries, and settings. Audit writes return errors; security-sensitive flows must not report success when their required audit record cannot be persisted.

Audit logs must not store:

- plaintext passwords
- raw access tokens
- raw refresh tokens
- secret config values

Failed login and failed password-change audit logs record actor, IP, user agent, and a reason summary, but never submitted passwords.

## Default Security Rules

- Development default credentials are allowed only for local use.
- Production must set `GOV2_TOKEN_SECRET` to a non-placeholder value with at least 32 characters.
- Production must not use `storage.driver=memory`.
- Set `GOV2_ENVIRONMENT=production` in deployment environments.
- Production environment detection trims whitespace and is case-insensitive; values such as `Production` still use production safety rules.
- Production must create the first administrator through `gov2 admin create`; development default users are not bootstrapped in production.
- `gov2 admin reset-password` can recover an existing administrator password but must not elevate non-admin users.
- User management must not delete, disable, or remove the admin role from the last active administrator.
- Development CORS may use `*`; production CORS must use explicit origins or disable cross-origin access.
- CORS preflight requests must use an allowed origin, supported HTTP method, and
  supported request headers.
- Password hashes are never returned by API.
- New or changed passwords must be at least 8 characters and contain at least
  one non-whitespace character.
- Disabled users cannot authenticate.
- Server-side RBAC checks are mandatory for protected routes.
- Compile-time extension routes with a non-empty permission are protected by
  bearer authentication and RBAC even if `Public` is set.
- Authenticated API routes must document exact `401 UnauthorizedError`
  responses, and routes protected by a named permission must document exact
  `403 ForbiddenError` responses.
- JSON request endpoints require a JSON media type before decoding.
- JSON request bodies are capped at 1 MiB before decoding.
- JSON request bodies must contain a single top-level value; trailing JSON values
  after the first value are rejected.
- JSON request endpoints must document `400`, exact `413 PayloadTooLargeError`,
  and exact `415 UnsupportedMediaTypeError` responses for bad requests, size,
  and media-type failures. Decode-only `400` responses use `BadRequestError`;
  field-level service validation routes use `ValidationError`.
- All HTTP responses include baseline browser security headers for content type
  sniffing, frame embedding, referrer leakage, and same-origin resource policy.

Application startup enforces the production token-secret, memory-store, and CORS wildcard rules.

## Testing

Security tests should cover:

- password verification
- token expiration and signature validation
- route permission denial
- audit log creation for sensitive writes
- production startup rejection for unsafe defaults
- CORS allowlist behavior, rejected production wildcard configuration, and
  unsupported preflight method/header rejection
- JSON request media type, body size, and trailing JSON value rejection
- baseline security headers on normal responses, CORS errors, and extension routes
- disabled user login denial
- role permission matching
- sensitive fields absent from API responses
