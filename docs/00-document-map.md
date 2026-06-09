# 00 Document Map

GOV2 文档按编号阅读和维护。新增文档必须放入对应编号目录，不要再平铺到 `docs/` 根目录。

## Reading Order

1. [01.01 Reference Projects](01-research/01-reference-projects.md)
2. [01.02 Reference Evaluation Matrix](01-research/02-reference-evaluation-matrix.md)
3. [02.01 System Blueprint](02-architecture/01-system-blueprint.md)
4. [02.02 Boundaries](02-architecture/02-boundaries.md)
5. [02.03 Module System](02-architecture/03-module-system.md)
6. [03.01 Backend Design](03-backend/01-backend-design.md)
7. [03.02 API Contract](03-backend/02-api-contract.md)
8. [03.03 Verification](03-backend/03-verification.md)
9. [04.01 Frontend Design](04-frontend/01-frontend-design.md)
10. [05.01 Database Design](05-database/01-database-design.md)
11. [05.02 Migrations](05-database/02-migrations.md)
12. [05.03 SQL Store](05-database/03-sql-store.md)
13. [06.01 Security And Authorization](06-security/01-security-authorization.md)
14. [07.01 AI Programming Rules](07-ai/01-ai-programming-rules.md)
15. [08.01 Roadmap](08-roadmap/01-roadmap.md)
16. [09.01 CI](09-ci/01-ci.md)
17. [10.01 Docker Deployment](10-deployment/01-docker.md)
18. [99.0001 ADR: Framework Principles](99-adr/0001-framework-principles.md)

## Numbering Rules

- `00-*`: document map and index.
- `01-research`: external project research and evaluation.
- `02-architecture`: global blueprint, boundaries, module rules.
- `03-backend`: Go backend framework design.
- `04-frontend`: Vue frontend framework design.
- `05-database`: database, migrations, persistence.
- `06-security`: auth, RBAC, audit, security baseline.
- `07-ai`: AI coding rules and guardrails.
- `08-roadmap`: implementation phases.
- `09-ci`: continuous integration and automated validation.
- `10-deployment`: deployment, packaging, and runtime operation.
- `99-adr`: architecture decision records.

## Maintenance Rules

- Do not create unnumbered design docs.
- Update this map when adding a document.
- If a document changes public API, database, permission, or AI rules, update the related document too.
- Keep reference docs separate from GOV2 design decisions.
