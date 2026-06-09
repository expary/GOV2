# 99.0001 ADR: Framework Principles

## Status

Accepted.

## Context

GOV2 should become a complete Go + Vue system framework. The project may learn from excellent open-source repositories, but it must remain original, free, and MIT-licensed.

The first MVP already contains a Go HTTP API and Vue frontend, but the framework needs documented architectural principles before more features are added.

## Decision

GOV2 will use these principles:

1. Modular monolith first.
2. Go backend and Vue frontend.
3. MIT license with no hidden commercial restrictions.
4. Domain and service code stay independent from HTTP, SQL, and Vue.
5. SQL persistence is introduced through repository interfaces.
6. RBAC is the default authorization model, with a future policy extension path.
7. Public APIs are versioned under `/api/v1`.
8. Frontend permissions improve UX; backend permissions enforce security.
9. AI assistants must follow documented boundaries and license rules.

## Consequences

Positive:

- Faster single-binary development.
- Easier testing.
- Clear module ownership.
- Safer AI-generated changes.
- Better path to SQL, plugins, and generated clients.

Tradeoffs:

- More upfront documentation.
- Some MVP code must be refactored before adding large modules.
- Dynamic plugin support is delayed until extension APIs stabilize.
