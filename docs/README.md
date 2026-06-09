# GOV2 Design Documents

GOV2 文档按编号组织。阅读、维护和新增文档时都要遵守编号顺序，避免散乱。

## 00 Document Map

- [00 Document Map](00-document-map.md): 文档阅读顺序、编号规则、维护规则。

## 01 Research

- [01.01 Reference Projects](01-research/01-reference-projects.md): 参考项目列表和许可证边界。
- [01.02 Reference Evaluation Matrix](01-research/02-reference-evaluation-matrix.md): 优秀 GitHub 项目评估矩阵。

## 02 Architecture

- [02.01 System Blueprint](02-architecture/01-system-blueprint.md): GOV2 总体系统蓝图。
- [02.02 Boundaries](02-architecture/02-boundaries.md): 模块、API、数据库、许可证和 AI 边界。
- [02.03 Module System](02-architecture/03-module-system.md): 模块注册、模块元数据和扩展路线。

## 03 Backend

- [03.01 Backend Design](03-backend/01-backend-design.md): Go 后端分层、服务、仓储、API、插件方向。
- [03.02 API Contract](03-backend/02-api-contract.md): OpenAPI 草案和 API 契约规则。
- [03.03 Verification](03-backend/03-verification.md): Makefile 验证命令和 PostgreSQL 集成测试入口。

## 04 Frontend

- [04.01 Frontend Design](04-frontend/01-frontend-design.md): Vue 前端结构、路由、状态、权限渲染、UI 规则。

## 05 Database

- [05.01 Database Design](05-database/01-database-design.md): 数据库理念、表模型、迁移、索引、多租户方向。
- [05.02 Migrations](05-database/02-migrations.md): 初始迁移和种子数据说明。
- [05.03 SQL Store](05-database/03-sql-store.md): `database/sql` repository adapter 和运行时选择。

## 06 Security

- [06.01 Security And Authorization](06-security/01-security-authorization.md): 认证、Token、RBAC、审计和安全默认值。

## 07 AI

- [07.01 AI Programming Rules](07-ai/01-ai-programming-rules.md): AI 编程规则、禁止事项、变更检查清单。

## 08 Roadmap

- [08.01 Roadmap](08-roadmap/01-roadmap.md): 从 MVP 到完整框架的阶段路线。

## 09 CI

- [09.01 CI](09-ci/01-ci.md): GitHub Actions workflow 和自动化验证规则。

## 10 Deployment

- [10.01 Docker Deployment](10-deployment/01-docker.md): Dockerfile、本地完整 Compose 栈和生产部署注意事项。

## 99 ADR

- [99.0001 Framework Principles](99-adr/0001-framework-principles.md): 第一份架构决策记录。

## Reference Style

GOV2 可以参考优秀开源项目的高层设计思想，但实现必须原创、MIT 兼容，不能复制外部项目代码、UI、数据库结构、生成文件或私有约定。
