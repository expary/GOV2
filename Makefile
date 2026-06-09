GOCACHE ?= /tmp/go-cache
GOMODCACHE ?= /tmp/go-mod
GOENV = GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE)

VERSION ?= 0.1.0
COMMIT ?= local
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
TARGET_OS ?= $(shell go env GOOS)
TARGET_ARCH ?= $(shell go env GOARCH)
LD_FLAGS = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

POSTGRES_DSN ?= postgres://gov2:gov2@localhost:5432/gov2?sslmode=disable

.PHONY: tidy test test-postgres web-api-generate web-api-check web-test web-lint web-build build package docs-check validate postgres-up postgres-down migrate-up migrate-seed admin-create admin-reset-password run-sql docker-build docker-up docker-down docker-logs

tidy:
	$(GOENV) go mod tidy

test:
	$(GOENV) go test ./...

test-postgres:
	GOV2_TEST_POSTGRES_DSN='$(POSTGRES_DSN)' $(GOENV) go test -p 1 ./internal/store/sqlstore ./internal/httpapi -run 'TestPostgres(Store|HTTP)Integration' -count=1

web-api-generate:
	cd web && npm run api:generate

web-api-check:
	cd web && npm run api:check

web-test:
	cd web && npm run test

web-lint:
	cd web && npm run lint

web-build:
	cd web && npm run build

build: web-build
	mkdir -p bin
	CGO_ENABLED=0 GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) $(GOENV) go build -buildvcs=false -trimpath -ldflags='$(LD_FLAGS)' -o bin/gov2 ./cmd/gov2

package: build
	mkdir -p dist
	tar -czf dist/gov2-$(VERSION)-$(TARGET_OS)-$(TARGET_ARCH).tar.gz bin/gov2 web/dist migrations config/gov2.example.json api/openapi.yaml README.md LICENSE

docs-check:
	node scripts/check-doc-links.mjs

validate: tidy test web-api-check web-test web-lint web-build docs-check

postgres-up:
	docker compose -f deployments/docker-compose.postgres.yml up -d

postgres-down:
	docker compose -f deployments/docker-compose.postgres.yml down

migrate-up:
	GOV2_STORAGE_DRIVER=pgx GOV2_STORAGE_DSN='$(POSTGRES_DSN)' $(GOENV) go run ./cmd/gov2 migrate up

migrate-seed:
	GOV2_STORAGE_DRIVER=pgx GOV2_STORAGE_DSN='$(POSTGRES_DSN)' $(GOENV) go run ./cmd/gov2 migrate seed

admin-create:
	GOV2_STORAGE_DRIVER=pgx GOV2_STORAGE_DSN='$(POSTGRES_DSN)' $(GOENV) go run ./cmd/gov2 admin create

admin-reset-password:
	GOV2_STORAGE_DRIVER=pgx GOV2_STORAGE_DSN='$(POSTGRES_DSN)' $(GOENV) go run ./cmd/gov2 admin reset-password

run-sql:
	GOV2_STORAGE_DRIVER=pgx GOV2_STORAGE_DSN='$(POSTGRES_DSN)' GOV2_AUTO_MIGRATE=true $(GOENV) go run ./cmd/gov2

docker-build:
	docker build --build-arg GOV2_VERSION=$(VERSION) --build-arg GOV2_COMMIT=$(COMMIT) --build-arg GOV2_BUILD_DATE=$(BUILD_DATE) -t gov2:$(VERSION) -t gov2:local .

docker-up:
	docker compose -f deployments/docker-compose.yml up --build -d

docker-down:
	docker compose -f deployments/docker-compose.yml down

docker-logs:
	docker compose -f deployments/docker-compose.yml logs -f gov2
