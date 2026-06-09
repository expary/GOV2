# 02.02 Boundaries

This document defines what each layer owns and what it must not know about.

## Dependency Rule

Allowed direction:

```text
cmd -> app -> delivery -> service -> domain
                 |
                 -> repository interfaces
store/sql -> repository interfaces + domain
web -> HTTP API only
```

The inner layers must not import outer layers:

- `domain` imports no `http`, SQL driver, Vue, config loader, or logger.
- `service` may use domain types and repository/security interfaces.
- `httpapi` translates HTTP into service calls.
- `store` implements repository contracts.
- `web` talks only to documented HTTP APIs.

## Backend Boundaries

### Domain

Owns:

- Core entities.
- Business invariants.
- Domain constants.
- Domain events.

Does not own:

- HTTP request parsing.
- SQL queries.
- JSON response envelope.
- Vue route names.

### Service

Owns:

- Use cases.
- Transactions.
- Authorization orchestration.
- Validation that belongs to business workflows.

Does not own:

- Raw HTTP handlers.
- HTML or Vue state.
- Database-specific SQL syntax.

### Repository

Owns:

- Persistence contracts.
- Query options.
- Transaction boundaries exposed to services.

Does not own:

- Password policy.
- Token issuing.
- HTTP pagination envelope.

### HTTP API

Owns:

- Routing.
- Middleware.
- Request and response DTOs.
- API versioning.
- Status codes.

Does not own:

- Business decisions.
- Database mutations outside services.

## Frontend Boundaries

The Vue app owns presentation and interaction. It does not own security decisions.

Allowed:

- Hide routes/buttons based on permissions returned by `/auth/profile`.
- Render navigation from backend-provided menu metadata.
- Cache local UI state.
- Validate input for user experience.

Not allowed:

- Assume hidden UI means secure.
- Invent permission codes not registered by backend modules.
- Depend on private token internals.

## API Boundaries

Public API contracts must be documented before downstream clients depend on them.

Rules:

- New fields are allowed when optional.
- Removing fields is breaking.
- Changing field type is breaking.
- Changing error code semantics is breaking.
- Internal database IDs may be exposed only for system resources where stable IDs are acceptable.

## Database Boundaries

Database schema belongs to the backend. Frontend code must not know table names.

Rules:

- Migrations are append-only after release.
- Seed data is explicit and idempotent.
- Sensitive fields are never returned by default.
- Audit tables are write-heavy and should be queried with clear pagination.

## License Boundary

GOV2 is MIT. This creates a hard boundary:

- Do not copy code from BSL, GPL, AGPL, unknown, or commercial-source projects.
- Do not copy UI assets, icons, schemas, generated clients, or test fixtures from other projects.
- Do not port code by changing names.
- Public ideas and generic architecture patterns are acceptable.
- Any copied third-party code must have a compatible license and attribution.

## AI Boundary

AI assistants may:

- Generate original code.
- Refactor local code.
- Summarize external projects.
- Propose architecture.

AI assistants must not:

- Paste external project code unless the license is compatible and the source is attributed.
- Create broad rewrites without preserving existing user changes.
- Introduce dependencies without explaining why.
- Add hidden telemetry or paid locks.
