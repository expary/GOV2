# 10.01 Docker Deployment

GOV2 provides a local Docker path for running the Go API, built Vue assets, and PostgreSQL together.

## Files

- `Dockerfile`: multi-stage build for Vue assets and the Go server binary.
- `.dockerignore`: keeps local caches, node modules, generated frontend output, logs, and private config out of the build context.
- `deployments/docker-compose.yml`: full local stack with GOV2 and PostgreSQL.
- `deployments/docker-compose.postgres.yml`: PostgreSQL-only helper for database testing.

## Local Full Stack

```bash
make docker-up
```

The full stack exposes:

- GOV2: `http://localhost:8080`
- health check: `http://localhost:8080/api/v1/health`
- readiness check: `http://localhost:8080/api/v1/ready`
- runtime metrics: `http://localhost:8080/api/v1/metrics`
- PostgreSQL: `localhost:5432`

The health endpoint reports process liveness. The readiness endpoint verifies
that required dependencies such as the configured store are usable before the
process receives traffic.

The metrics endpoint emits Prometheus-compatible process metrics and HTTP
request totals labeled by method, normalized route, and status code.

The local stack uses:

- `GOV2_ENVIRONMENT=development`
- `GOV2_STORAGE_DRIVER=pgx`
- `GOV2_AUTO_MIGRATE=true`

Development mode runs migrations, seed files, and development bootstrap data. The default account remains:

- `admin / admin123`

Stop the stack with:

```bash
make docker-down
```

View application logs with:

```bash
make docker-logs
```

## Image Build

```bash
make docker-build
```

The runtime image contains:

- `/app/gov2`
- `/app/web/dist`
- `/app/migrations`
- `/app/config/gov2.example.json`

The image defaults `GOV2_STATIC_DIR`, `GOV2_MIGRATIONS_DIR`, and `GOV2_SEEDS_DIR` to the container paths.

## Release Package

Build a local binary and release archive with:

```bash
make build
make package
bin/gov2 version
```

The archive is written to:

- `dist/gov2-<version>-<os>-<arch>.tar.gz`

It includes the `gov2` binary, built Vue assets, migrations, the example config, OpenAPI, README, and license.

Set release metadata with:

```bash
make package VERSION=0.1.0 COMMIT=<git-sha>
```

## Production Notes

For production deployment, set:

- `GOV2_ENVIRONMENT=production`
- `GOV2_TOKEN_SECRET` to a non-placeholder value with at least 32 characters
- `GOV2_CORS_ALLOWED_ORIGINS` to explicit browser origins, for example `https://admin.example.com`
- `GOV2_STORAGE_DRIVER=pgx`
- `GOV2_STORAGE_DSN` to the production PostgreSQL DSN

Production startup rejects unsafe token secrets, wildcard CORS origins, and `memory` storage.

Optional runtime tuning can be supplied through:

- `GOV2_APP_NAME`
- `GOV2_ADDR`
- `GOV2_SERVER_READ_TIMEOUT`
- `GOV2_SERVER_WRITE_TIMEOUT`
- `GOV2_SERVER_IDLE_TIMEOUT`
- `GOV2_TOKEN_TTL`
- `GOV2_TOKEN_ISSUER`

Do not use the development default account in production. Run migrations and seed data deliberately, then create the first administrator before exposing the service:

```bash
export GOV2_ENVIRONMENT=production
export GOV2_CORS_ALLOWED_ORIGINS='https://admin.example.com'
export GOV2_STORAGE_DRIVER=pgx
export GOV2_STORAGE_DSN='postgres://gov2:gov2@localhost:5432/gov2?sslmode=disable'
export GOV2_ADMIN_USERNAME=admin
export GOV2_ADMIN_PASSWORD='replace-with-a-strong-password'

gov2 migrate up
gov2 migrate seed
gov2 admin create
```

`gov2 admin create` requires the seeded `admin` role, enforces the normal password policy, and rejects creation when an administrator already exists.

If the administrator password is lost, reset it without granting new roles:

```bash
export GOV2_STORAGE_DRIVER=pgx
export GOV2_STORAGE_DSN='postgres://gov2:gov2@localhost:5432/gov2?sslmode=disable'
export GOV2_ADMIN_USERNAME=admin
export GOV2_ADMIN_PASSWORD='replace-with-a-new-strong-password'

gov2 admin reset-password
```

The reset command only updates users that already have the `admin` role.
