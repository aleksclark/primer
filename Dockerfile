# syntax=docker/dockerfile:1

# ── Stage 1: generate the OpenAPI spec from the API type signatures ─────────
FROM golang:1.25-alpine AS spec
WORKDIR /src
COPY server/go.mod server/go.sum ./server/
RUN cd server && go mod download
COPY server/ ./server/
RUN cd server && go run ./cmd/openapi-gen -out /openapi.yaml

# ── Stage 2: build the admin SPA (TS client generated from the spec) ────────
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
# SPA imports ../../design-system/generated/primer.css
COPY design-system/generated/ /src/design-system/generated/
COPY --from=spec /openapi.yaml ./openapi.yaml
RUN npm run build

# ── Stage 3: build the server with the SPA embedded ─────────────────────────
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY server/go.mod server/go.sum ./server/
RUN cd server && go mod download
COPY server/ ./server/
# Replace the placeholder with the real SPA bundle before compiling.
RUN rm -rf server/internal/spa/dist
COPY --from=web /src/web/dist/ ./server/internal/spa/dist/
RUN cd server && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /primer-server ./cmd/primer-server

# ── Final: minimal runtime image ─────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /primer-server /primer-server

ENV PORT=8080
EXPOSE 8080

# Migrations are embedded and applied automatically on startup.
# Configure with DATABASE_URL, PORT, HOST, ENV, CORS_ORIGINS.
ENTRYPOINT ["/primer-server"]
