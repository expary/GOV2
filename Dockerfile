# syntax=docker/dockerfile:1

FROM node:24-alpine AS web-build
WORKDIR /src/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

FROM golang:1.22-alpine AS go-build
WORKDIR /src

ARG GOV2_VERSION=dev
ARG GOV2_COMMIT=local
ARG GOV2_BUILD_DATE=unknown

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-build /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -trimpath -ldflags="-s -w -X main.version=${GOV2_VERSION} -X main.commit=${GOV2_COMMIT} -X main.buildDate=${GOV2_BUILD_DATE}" -o /out/gov2 ./cmd/gov2

FROM alpine:3.20
WORKDIR /app

RUN apk add --no-cache ca-certificates \
	&& addgroup -S gov2 \
	&& adduser -S gov2 -G gov2

COPY --from=go-build /out/gov2 /app/gov2
COPY --from=web-build /src/web/dist /app/web/dist
COPY migrations /app/migrations
COPY config/gov2.example.json /app/config/gov2.example.json

ENV GOV2_ADDR=:8080 \
	GOV2_STATIC_DIR=/app/web/dist \
	GOV2_MIGRATIONS_DIR=/app/migrations \
	GOV2_SEEDS_DIR=/app/migrations/seeds

EXPOSE 8080

USER gov2
ENTRYPOINT ["/app/gov2"]
