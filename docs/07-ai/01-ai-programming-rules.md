# 07.01 AI Programming Rules

GOV2 should be easy for AI tools to extend, but AI changes must stay inside clear engineering and license boundaries.

## Why This Exists

AI assistants are useful for scaffolding modules, writing tests, refactoring, and documenting decisions. They are also risky when requirements are vague, external code is copied, or architecture boundaries are ignored.

This document defines how AI should work in GOV2.

## Allowed Work

AI assistants may:

- Generate original Go and Vue code.
- Create tests and fixtures.
- Refactor code inside existing boundaries.
- Summarize public projects and extract general design principles.
- Draft migrations, API specs, and docs.
- Propose alternatives with tradeoffs.

## Forbidden Work

AI assistants must not:

- Copy code from other repositories unless the license is compatible and attribution is added.
- Copy UI assets, schemas, table names, generated clients, or test fixtures from license-incompatible projects.
- Add hidden telemetry, license gates, remote kill switches, or paid-only code paths.
- Store secrets in source files.
- Change public API contracts without updating docs.
- Rewrite unrelated files to make a narrow task easier.
- Hide failing verification.

## Prompt Boundary

When asking AI to implement a feature, prompts should specify:

- module name
- backend API
- permission code
- database tables
- frontend route
- tests to update
- docs to update

Bad prompt:

```text
参考某项目做一个用户管理。
```

Better prompt:

```text
为 GOV2 增加组织管理模块。遵守 docs/02-architecture/02-boundaries.md。
后端新增 /api/v1/system/organizations，权限码 system:organization:*。
新增 SQL migration、service tests、Vue route and table view。
不要复制外部项目代码。
```

## AI Change Checklist

Every AI-generated change should answer:

- Which module owns this code?
- Does this cross a documented boundary?
- Is the dependency direction correct?
- Did it introduce a new third-party package?
- Is the new package MIT-compatible or otherwise acceptable?
- Are API changes documented?
- Are database changes in migrations?
- Are permissions registered and enforced server-side?
- Are tests or verification commands included?

## Dependency Policy

Before adding a dependency, AI must justify:

- What problem it solves.
- Why standard library or existing code is not enough.
- License compatibility.
- Maintenance status.
- Replacement cost.

Avoid dependencies for tiny helpers, string formatting, one-off validation, or simple state management.

## Code Generation Rules

Code generation is allowed when it reduces repeatable work.

Rules:

- Generated files must be marked.
- Generator inputs must be checked in.
- Generated output should be reproducible.
- Do not manually edit generated files unless documented.
- Do not use generators that require proprietary services for normal framework development.

## API Rules for AI

When adding an endpoint:

- Add a permission code for protected operations.
- Add a request DTO and response DTO.
- Validate input.
- Return the standard response envelope.
- Add OpenAPI documentation when OpenAPI exists.
- Add frontend API usage only through the API client.

## Database Rules for AI

When adding a table:

- Use clear module prefix.
- Add audit columns where appropriate.
- Add indexes based on expected queries.
- Add migration and seed only when needed.
- Avoid hardcoded IDs except stable seed references that are documented.

## Frontend Rules for AI

When adding a page:

- Add route metadata.
- Add permission guard or permission rendering.
- Use existing layout and design tokens.
- Include loading and empty states for data views.
- Keep table columns stable on mobile through overflow, not layout jumps.
- Do not use in-app instructional text to explain how the application works.

## Review Rules

AI-generated changes should be reviewed for:

- license safety
- boundary correctness
- public API stability
- security
- tests
- user-facing UX
- performance for list endpoints

No change is complete until verification commands have run or the environment limitation is explicitly recorded.
