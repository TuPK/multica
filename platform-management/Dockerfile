# Build from the repository root, not from platform-management: the console
# imports @multica/ui and @multica/core, which live outside it.
#   docker build -f platform-management/Dockerfile -t multica-platform .
# The context is filtered by platform-management/Dockerfile.dockerignore.
# Keep the Go image in step with /Dockerfile and the Node image with
# /Dockerfile.web.

# --- Console stage ---
FROM node:22.23.4-alpine AS console

WORKDIR /app

# Activate the pnpm version the repository declares before it reads the lockfile.
COPY package.json ./
RUN corepack enable && \
    PNPM_VERSION="$(node -p 'require("./package.json").packageManager')" && \
    corepack prepare "$PNPM_VERSION" --activate

# Manifests first, so the install layer is reused until a dependency changes.
# Root pnpm patchedDependencies is read even without the mobile workspace.
COPY pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY apps/mobile/patches/ apps/mobile/patches/
COPY packages/core/package.json packages/core/
COPY packages/ui/package.json packages/ui/
COPY packages/tsconfig/package.json packages/tsconfig/
COPY packages/vitest-setup/package.json packages/vitest-setup/
COPY platform-management/console/package.json platform-management/console/
RUN pnpm install --frozen-lockfile --filter @multica/platform-console...

COPY packages/core/ packages/core/
COPY packages/ui/ packages/ui/
COPY packages/tsconfig/ packages/tsconfig/
COPY packages/vitest-setup/ packages/vitest-setup/
COPY platform-management/console/ platform-management/console/
RUN pnpm --filter @multica/platform-console build

# --- Build stage ---
FROM golang:1.27.3-alpine AS builder

WORKDIR /src

COPY platform-management/go.mod platform-management/go.sum ./
RUN go mod download

COPY platform-management/ ./
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/platform-server ./cmd/platform-server

# --- Runtime stage ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S -g 10001 platform \
 && adduser -S -u 10001 -G platform platform

WORKDIR /app

COPY --from=builder /out/platform-server .
COPY --from=console /app/platform-management/console/dist/ ./console/dist/

USER platform

EXPOSE 8095 30001

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -q -O /dev/null "http://127.0.0.1:${PLATFORM_CONSOLE_PORT:-30001}/healthz" || exit 1

ENTRYPOINT ["./platform-server"]
