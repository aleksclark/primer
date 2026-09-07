# syntax=docker/dockerfile:1

# ── Stage 1: prepare the server module graph, including local replaces ──────
FROM golang:1.25-alpine AS source
ENV CGO_ENABLED=0
RUN apk add --no-cache git ca-certificates
WORKDIR /src
# Docker-only workspace for the server graph. Host go.work also lists
# studio/identity/tasks; those modules are not part of these images.
# Local replaces (nested inventory):
#   server/go.mod: github.com/aleksclark/primer/agents/client/go => ../primer-agents/client/go
#   primer-agents/{,client/go} go.mod: no further local replaces
#   github.com/aleksclark/primer/agents/runtime comes from the workspace module
COPY <<'EOF' /src/go.work
go 1.25.7

use (
	./server
	./primer-agents
	./primer-agents/client/go
)
EOF
ENV GOWORK=/src/go.work
COPY server/go.mod server/go.sum ./server/
COPY primer-agents/go.mod primer-agents/go.sum ./primer-agents/
COPY primer-agents/client/go/go.mod primer-agents/client/go/go.sum ./primer-agents/client/go/
RUN --mount=type=cache,target=/go/pkg/mod \
	go -C /src/server mod download && \
	go -C /src/primer-agents mod download && \
	go -C /src/primer-agents/client/go mod download
COPY server/ ./server/
COPY primer-agents/ ./primer-agents/
WORKDIR /src/server

# ── Stage 2: generate the OpenAPI spec from the API type signatures ─────────
FROM source AS spec
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go run ./cmd/openapi-gen -out /openapi.yaml

# ── Stage 3: build the admin SPA (TS client generated from the spec) ────────
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
# SPA imports ../../design-system/generated/primer.css
COPY design-system/generated/ /src/design-system/generated/
COPY --from=spec /openapi.yaml ./openapi.yaml
RUN npm run build

# ── Stage 4: build the server with the SPA embedded ─────────────────────────
FROM source AS build
# Replace the placeholder with the real SPA bundle before compiling.
RUN rm -rf internal/spa/dist
COPY --from=web /src/web/dist/ ./internal/spa/dist/
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /primer-server ./cmd/primer-server

# ── Final: minimal runtime image ─────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /primer-server /primer-server

ENV PORT=8080
EXPOSE 8080

# Migrations are embedded and applied automatically on startup.
# Configure with DATABASE_URL, PORT, HOST, ENV, CORS_ORIGINS.
ENTRYPOINT ["/primer-server"]
