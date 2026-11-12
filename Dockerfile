# --- Catalog stage ---
# The server embeds template data generated from catalog/ (docs/specs/template-registry/spec.md). The
# generated files are not committed, and the generator needs only Node, so it runs in its own stage.
# Keep the Node version in step with Dockerfile.web.
FROM node:22.23.4-alpine AS catalog

WORKDIR /src
COPY catalog/ catalog/
COPY scripts/catalog/ scripts/catalog/
COPY scripts/lib/ scripts/lib/
# The generator checks the catalog's locales against the product's supported locales.
COPY packages/core/i18n/types.ts packages/core/i18n/types.ts
RUN node scripts/catalog/cli.mjs build --target go

# --- Build stage ---
FROM golang:1.27.3-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src

# Cache dependencies
COPY server/go.mod server/go.sum ./server/
RUN cd server && go mod download

# Copy server source, then the generated catalog data it embeds
COPY server/ ./server/
COPY --from=catalog /src/server/internal/catalog/generated/ ./server/internal/catalog/generated/

# Build binaries
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
RUN cd server && CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" -o bin/server ./cmd/server
RUN cd server && CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" -o bin/multica ./cmd/multica
RUN cd server && CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/migrate ./cmd/migrate
RUN cd server && CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/maintenance ./cmd/maintenance
RUN cd server && CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/backfill_task_usage_hourly ./cmd/backfill_task_usage_hourly
RUN cd server && CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/backfill_codex_usage_cache ./cmd/backfill_codex_usage_cache

# --- Runtime stage ---
FROM alpine:3.21

# git runs the repo-docs sync (knowledge_repo_sync); the sync needs it at run time.
RUN apk add --no-cache ca-certificates tzdata git

WORKDIR /app

COPY --from=builder /src/server/bin/server .
COPY --from=builder /src/server/bin/multica .
COPY --from=builder /src/server/bin/migrate .
COPY --from=builder /src/server/bin/maintenance .
COPY --from=builder /src/server/bin/backfill_task_usage_hourly .
COPY --from=builder /src/server/bin/backfill_codex_usage_cache .
COPY server/migrations/ ./migrations/
COPY LICENSE NOTICE ./
COPY docker/entrypoint.sh .
RUN sed -i 's/\r$//' entrypoint.sh && chmod +x entrypoint.sh

EXPOSE 8080

ENTRYPOINT ["./entrypoint.sh"]
