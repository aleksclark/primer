# primer-agents module Makefile

MODULE_ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

# ── Build / test ──────────────────────────────────────────────────────────────

.PHONY: build test test-race vet

build:
	go build ./...

test:
	go test -count=1 -timeout=300s ./...

test-race:
	go test -race -count=1 -timeout=300s ./...

vet:
	go vet ./...

# ── OpenAPI generation ────────────────────────────────────────────────────────
# Emits openapi.yaml (OAS 3.1) and openapi30.yaml (OAS 3.0) from handler signatures.
# openapi30.yaml is used as input for the Go client generator (oapi-codegen).

.PHONY: openapi

openapi:
	bash scripts/generate-openapi.sh

# ── Client generation ─────────────────────────────────────────────────────────
# clients-go: regenerate client/go/client.gen.go from openapi30.yaml
# clients-ts: regenerate client/ts/types.gen.ts from openapi.yaml
# clients:    regenerate all (openapi + Go + TypeScript)

.PHONY: clients-go clients-ts clients

clients-go: openapi
	bash scripts/generate-go-client.sh

clients-ts: openapi
	bash scripts/generate-ts-client.sh

clients: clients-go clients-ts

# ── Contract / drift gates ────────────────────────────────────────────────────
# contracts-check: fail if committed openapi.yaml is stale or client has duplicate DTOs
# Runs the in-package tests (no testcontainer needed; -short skips live integrations).

.PHONY: contracts-check

contracts-check:
	go test -count=1 -timeout=60s -short \
		-run "TestOpenAPIDeterministic|TestNoDuplicateDTOs|TestNoRawTransportImports" \
		./internal/api/...

# ── Go client tests ───────────────────────────────────────────────────────────

.PHONY: test-client-go

test-client-go:
	cd client/go && GOWORK=off go test -count=1 -timeout=30s ./...

# ── TypeScript client check ───────────────────────────────────────────────────

.PHONY: check-ts

check-ts:
	cd client/ts && npx tsc --noEmit

# ── All gates ─────────────────────────────────────────────────────────────────

.PHONY: gates

gates: vet test test-race contracts-check test-client-go check-ts

# ── Help ──────────────────────────────────────────────────────────────────────

.PHONY: help

help:
	@echo "primer-agents module targets:"
	@echo "  build             go build ./..."
	@echo "  test              go test ./... (with testcontainer PostgreSQL)"
	@echo "  test-race         go test -race ./..."
	@echo "  vet               go vet ./..."
	@echo "  openapi           emit openapi.yaml + openapi30.yaml from handler signatures"
	@echo "  clients-go        regenerate client/go/client.gen.go (needs openapi)"
	@echo "  clients-ts        regenerate client/ts/types.gen.ts (needs openapi)"
	@echo "  clients           all client generation (openapi + go + ts)"
	@echo "  contracts-check   drift + no-duplicate-DTO + no-raw-transport gates"
	@echo "  test-client-go    run generated Go client contract tests"
	@echo "  check-ts          tsc --noEmit for TypeScript client"
	@echo "  gates             all of the above"
