# 01.02 Reference Evaluation Matrix

This document evaluates excellent GitHub projects and converts the useful ideas into GOV2 design decisions. It is not a copy plan.

## Evaluation Criteria

Each reference is evaluated by:

- What GOV2 can learn.
- What GOV2 must not copy.
- Which GOV2 document or module is affected.
- The concrete decision GOV2 adopts.

## Matrix

| Project | Category | What GOV2 Learns | Do Not Copy | GOV2 Decision |
| --- | --- | --- | --- | --- |
| Kubernetes | API conventions | Versioned APIs, resource-oriented design, stable object contracts | Kubernetes API types, code, controller implementation | Keep `/api/v1`, stable DTOs, explicit compatibility rules |
| Grafana | Observability platform | Plugin isolation, permission-aware UI, operational dashboards | Plugin code, UI assets, internal schema | Add module registry and future plugin hooks after core APIs stabilize |
| PocketBase | Go backend product | Single-binary developer experience, embedded admin ergonomics | Database schema, API names, admin UI implementation | GOV2 should run as one Go process serving API and Vue assets |
| Supabase | Open data platform | Postgres-first product thinking, open integration ecosystem | Supabase service composition, schemas, branding | PostgreSQL is the production-first database target |
| Temporal | Workflow platform | Clear workflow/activity separation and durable execution thinking | Workflow engine internals | Background jobs in GOV2 must be idempotent and separated from HTTP handlers |
| Kratos | Go framework | Clean service boundaries, transport separation, config discipline | Project template or generated code | Keep delivery layer separate from service/domain |
| go-zero | Go framework | Pragmatic API/RPC scaffolding and generator ergonomics | Generator code and naming conventions | GOV2 can later add module scaffolding with original templates |
| Ent | Entity framework | Schema-as-code, migrations, typed query benefits | Ent schema structure unless deliberately adopted | Database design should support typed repository adapters |
| Atlas | Database tooling | Declarative migrations and schema review discipline | CLI internals or migration examples | Migrations must be reviewable, ordered, and CI-safe |
| golang-migrate | Migration tooling | Simple ordered migration workflow | Migration files from examples | Use numbered SQL migrations when SQL persistence starts |
| Casbin | Authorization | RBAC/ABAC vocabulary and policy adapter direction | Policy model files from projects | Start with RBAC, leave policy-engine adapter path |
| Ory Keto/Kratos | Identity/auth | Separation of identity, session, and authorization responsibilities | Identity server implementation | GOV2 auth must separate user, session, token, and permission concerns |
| Ant Design Pro | Enterprise admin UX | Dense admin layouts, route-level permissions, dashboard conventions | UI implementation, theme, assets | GOV2 frontend should be dense, operational, and permission-aware |
| vue-vben-admin | Vue admin framework | Vue 3 admin structure, layout, route guards, theme organization | Code, components, visual design | GOV2 frontend uses Vue 3 with module-oriented routes and stores |
| vue-element-admin | Admin dashboard | Permission routing and practical admin CRUD screens | Code, names, assets | GOV2 keeps backend-authoritative permissions and frontend UX guards |
| Directus | Data platform | Admin app around data model, roles, permissions | Schema, UI, API structure | GOV2 dictionaries/settings should be extensible without becoming data-platform-first |

## Reference-To-Document Mapping

| GOV2 Document | Main References |
| --- | --- |
| `02-architecture/01-system-blueprint.md` | Kubernetes, Grafana, PocketBase, Supabase |
| `02-architecture/02-boundaries.md` | Kratos, Kubernetes, Grafana |
| `03-backend/01-backend-design.md` | Kratos, go-zero, PocketBase, Temporal |
| `04-frontend/01-frontend-design.md` | Ant Design Pro, vue-vben-admin, vue-element-admin |
| `05-database/01-database-design.md` | Supabase, Ent, Atlas, golang-migrate |
| `06-security/01-security-authorization.md` | Casbin, Ory, Grafana |
| `07-ai/01-ai-programming-rules.md` | AGENTS-style coding rules, project-specific guardrails |

## Final Rule

References guide GOV2's thinking. GOV2 implementation must remain original, MIT-compatible, and internally consistent.
