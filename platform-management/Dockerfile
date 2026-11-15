# Build from the platform-management directory:
#   docker build -f platform-management/Dockerfile -t multica-platform platform-management
# Keep the Go image in step with /Dockerfile.

# --- Build stage ---
FROM golang:1.27.3-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/platform-server ./cmd/platform-server
# The console build is optional until it exists; keep the directory either way.
RUN mkdir -p console/dist

# --- Runtime stage ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S -g 10001 platform \
 && adduser -S -u 10001 -G platform platform

WORKDIR /app

COPY --from=builder /out/platform-server .
COPY --from=builder /src/console/dist/ ./console/dist/

USER platform

EXPOSE 8095 30001

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -q -O /dev/null "http://127.0.0.1:${PLATFORM_CONSOLE_PORT:-30001}/healthz" || exit 1

ENTRYPOINT ["./platform-server"]
