# GOV2

GOV2 是一个完全免费、可商用、可二次开发的 Go 系统管理框架。项目参考常见后台管理系统的功能边界，但代码、目录、接口和前端实现均为原创实现，不复制 `gin-vue-admin` 的源码或资产。

框架设计文档见 [docs/README.md](docs/README.md)，文档地图见 [docs/00-document-map.md](docs/00-document-map.md)。AI 编程边界和规则见 [AGENTS.md](AGENTS.md) 与 [docs/07-ai/01-ai-programming-rules.md](docs/07-ai/01-ai-programming-rules.md)。

API 草案见 [api/openapi.yaml](api/openapi.yaml)，数据库初始迁移见 [migrations](migrations)。

## 当前版本

第一版是一个可运行的 MVP，重点把系统框架的基础打稳：

- Go 标准库 HTTP 服务，无第三方运行时依赖
- 登录认证、签名 Token、密码哈希
- 用户创建、管理员重置和自助改密统一执行最小密码长度策略
- 用户、角色、菜单、字典、审计日志 API
- 用户管理 CRUD，包含角色分配、状态切换、搜索过滤、分页和字典状态标签
- 菜单管理 CRUD，包含父级校验和循环保护
- 菜单写入后自动刷新当前用户菜单树
- 角色管理 CRUD，包含权限目录和已分配角色删除保护
- 字典管理 CRUD，包含字典项新增、编辑和删除
- 设置管理 CRUD，支持 JSON 配置值
- `site.title` 设置驱动公共应用配置和前端品牌标题
- 基于角色权限码的 RBAC
- 请求 ID、CORS、访问日志、异常恢复中间件
- 登录成功、退出和登录失败自动记录审计日志
- 核心系统写操作自动记录审计日志
- 审计日志支持分页和关键字、操作者、动作、资源、资源 ID 筛选
- Vue 3 + Vite + Pinia + Vue Router 管理台
- 登录用户可自助更新个人资料和修改密码
- 前端统一 API client，保留 request_id 并处理认证失效
- API 支持字段级校验错误，核心系统表单可映射后端字段错误
- 自助改密成功和当前密码错误都会写入安全审计
- `/auth/profile` 返回权限过滤后的菜单树，前端侧栏可动态渲染菜单
- 前端支持侧栏折叠、紧凑密度偏好和受保护 Not Found 页面
- 内存存储和 PostgreSQL `database/sql` 存储适配器
- Dockerfile 和本地完整 Docker Compose 运行栈
- GitHub Actions CI 和本地 `make validate` 验证入口
- HTTP 路由测试覆盖登录、权限拒绝和核心系统写接口

## 快速启动

需要 Go 1.22 或更高版本，以及 Node.js 20 或更高版本。

Docker 一键启动：

```bash
make docker-up
```

访问地址：

- GOV2：http://localhost:8080
- 健康检查：http://localhost:8080/api/v1/health
- 就绪检查：http://localhost:8080/api/v1/ready
- 运行指标：http://localhost:8080/api/v1/metrics

停止 Docker 栈：

```bash
make docker-down
```

后端：

```bash
go run ./cmd/gov2
```

前端开发模式：

```bash
cd web
npm install
npm run dev
```

默认访问地址：

- 前端开发服务：http://localhost:5173
- 后端服务：http://localhost:8080
- 健康检查：http://localhost:8080/api/v1/health
- 就绪检查：http://localhost:8080/api/v1/ready
- 运行指标：http://localhost:8080/api/v1/metrics

生产构建：

```bash
cd web
npm run build
cd ..
go run ./cmd/gov2
```

生产模式下，Go 服务会托管 `web/dist`。

本地发布包：

```bash
make build
make package
bin/gov2 version
```

发布包输出到 `dist/gov2-<version>-<os>-<arch>.tar.gz`。

默认账号：

- 用户名：`admin`
- 密码：`admin123`

Docker 本地栈使用 PostgreSQL、自动迁移和开发种子数据，默认账号仅用于开发体验。

## 配置

复制配置示例：

```bash
cp config/gov2.example.json config/gov2.json
```

也可以通过环境变量覆盖：

- `GOV2_CONFIG`：配置文件路径
- `GOV2_APP_NAME`：应用名称
- `GOV2_ENVIRONMENT`：运行环境，例如 `development` 或 `production`
- `GOV2_ADDR`：监听地址，例如 `:8080`
- `GOV2_SERVER_READ_TIMEOUT`：HTTP 读取超时，例如 `5s`
- `GOV2_SERVER_WRITE_TIMEOUT`：HTTP 写入超时，例如 `10s`
- `GOV2_SERVER_IDLE_TIMEOUT`：HTTP 空闲连接超时，例如 `60s`
- `GOV2_TOKEN_SECRET`：Token 签名密钥，生产环境必须设置为至少 32 字符的非占位值
- `GOV2_TOKEN_TTL`：Token 有效期，例如 `2h`
- `GOV2_TOKEN_ISSUER`：Token 签发者，默认 `gov2`
- `GOV2_CORS_ALLOWED_ORIGINS`：允许的跨域来源，多个来源用逗号分隔；生产环境不能使用 `*`
- `GOV2_STORAGE_DRIVER`：存储驱动，默认 `memory`
- `GOV2_STORAGE_DSN`：数据库连接字符串
- `GOV2_MIGRATIONS_DIR`：迁移目录，默认 `migrations`
- `GOV2_SEEDS_DIR`：种子数据目录，默认 `migrations/seeds`
- `GOV2_STATIC_DIR`：前端静态目录，默认 `web/dist`
- `GOV2_ADMIN_USERNAME`：管理员初始化或密码恢复用户名，默认 `admin`
- `GOV2_ADMIN_PASSWORD`：管理员初始化或密码恢复密码，至少 8 位
- `GOV2_ADMIN_EMAIL`：初始化管理员邮箱，仅用于 `gov2 admin create`
- `GOV2_ADMIN_NICKNAME`：初始化管理员昵称，仅用于 `gov2 admin create`

## 数据库迁移

当前服务端默认仍使用 `memory` 存储。仓库已提供 PostgreSQL 迁移 SQL、迁移命令入口、`database/sql` store 适配器和 pgx 驱动注册入口。

生产环境会拒绝使用默认/占位 Token secret，也会拒绝 `memory` 存储。

```bash
make postgres-up

export GOV2_STORAGE_DRIVER=pgx
export GOV2_STORAGE_DSN='postgres://gov2:gov2@localhost:5432/gov2?sslmode=disable'

go run ./cmd/gov2 migrate up
go run ./cmd/gov2 migrate seed
GOV2_AUTO_MIGRATE=true go run ./cmd/gov2
```

生产环境不会自动创建默认管理员。完成迁移和种子数据后，用显式密码创建首个管理员：

```bash
export GOV2_ADMIN_USERNAME=admin
export GOV2_ADMIN_PASSWORD='replace-with-a-strong-password'
go run ./cmd/gov2 admin create
```

`gov2 admin create` 会复用已有 `admin` 角色，只在当前没有管理员用户时创建一个超级管理员。

如果管理员密码丢失，可以通过数据库连接重置已有管理员密码：

```bash
export GOV2_ADMIN_USERNAME=admin
export GOV2_ADMIN_PASSWORD='replace-with-a-new-strong-password'
go run ./cmd/gov2 admin reset-password
```

`gov2 admin reset-password` 只会更新已拥有 `admin` 角色的用户，不会把普通用户提升为管理员。

可选 PostgreSQL 集成测试：

```bash
export GOV2_TEST_POSTGRES_DSN='postgres://gov2:gov2@localhost:5432/gov2?sslmode=disable'
make test
```

也可以直接运行 PostgreSQL 专项集成测试：

```bash
make test-postgres
```

## 验证

```bash
make validate
```

## 模块脚手架

生成一个新业务模块的起始结构：

```bash
go run ./cmd/gov2 module scaffold --name inventory --title Inventory
```

脚手架会生成后端模块元数据、前端视图模板、迁移占位和 README。它不会自动修改全局注册代码，新模块接入前需要补路由、服务、仓储和测试。

## 许可证

GOV2 使用 MIT License。你可以免费用于个人、商业、闭源或开源项目。

## 后续路线

1. 增加更多 SQL 边界场景的路由和服务测试覆盖。
2. 增加 Casbin 或自研策略引擎的可选适配层。
3. 增加代码生成器、表单配置和低代码资源管理。
4. 增加更完整的 Vue 组件库、主题系统和动态路由。
5. 增加版本化升级流程。
