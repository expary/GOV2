# GOV2 AI Programming Rules

This file is the entry point for AI coding assistants working in GOV2.

## Core Rule

GOV2 is an original, free, MIT-licensed system framework. You may learn from public open-source projects at the level of ideas, boundaries, tradeoffs, and patterns, but you must not copy source code, assets, naming, database schema, generated code, or implementation details from projects with incompatible or unclear licenses.

## Read First

Before changing code, read these documents:

- `docs/00-document-map.md`
- `docs/02-architecture/01-system-blueprint.md`
- `docs/02-architecture/02-boundaries.md`
- `docs/03-backend/01-backend-design.md`
- `docs/04-frontend/01-frontend-design.md`
- `docs/05-database/01-database-design.md`
- `docs/06-security/01-security-authorization.md`
- `docs/07-ai/01-ai-programming-rules.md`

## Engineering Boundaries

- Keep domain models independent from HTTP, SQL, Vue, and framework concerns.
- Add external dependencies only when they solve a real framework problem and are compatible with MIT distribution.
- Prefer small, testable modules over broad utility packages.
- Do not add hidden commercial restrictions, license gates, telemetry, or paid-only features.
- Do not change public API shapes without updating docs and compatibility notes.
- Do not store secrets in source files. Use config files, environment variables, or secret managers.

## Verification

For Go changes, run:

```bash
go test ./...
```

For Vue changes, run:

```bash
cd web
npm run lint
npm run build
```

If the local environment lacks Go, Node, or network access, record the exact command that could not run and why.
