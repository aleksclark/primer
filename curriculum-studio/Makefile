# Curriculum Studio module-local targets (root Makefile is F0-owned).
# Root Makefile / go.work / CI remain F0-owned — do not duplicate root edits here.
#
# Root wiring (F0-owned names):
#   make migrate-studio  → go run ./cmd/migrate up (STUDIO_DATABASE_URL required)
#   make dev-db-studio   → deferred (no coherent Compose surface; no hollow compose)

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

STUDIO_ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
CONTRACTS := $(STUDIO_ROOT)/contracts
GATES := $(STUDIO_ROOT)/tools/contract-gates

.PHONY: help \
	build test cover test-race e2e \
	migrate migrate-up migrate-down migrate-status freeze-check freeze-write db-test \
	studio-test studio-cover studio-db-pytest studio-backup-drill \
	studio-mcp-e2e studio-mcp-conformance studio-mcp-coverage studio-mcp-external-e2e \
	contracts-validate \
	contracts-ownership \
	contracts-buf-generate \
	clients-go-grpc-build clients-go-rest-generate clients-go-rest-build \
	contracts-openapi-emit \
	clients-ts-rest-generate clients-ts-rest-build \
	clients-generate \
	contracts-parity \
	contracts-gates contracts-ci contracts-conformance gates-red-proof \
	contracts-spikes

help:
	@echo "Curriculum Studio module targets:"
	@echo "  build                   go build cmd/studio-server -> ../bin/studio-server"
	@echo "  test / cover            Go unit tests and internal coverage"
	@echo "  test-race               go test -race ./..."
	@echo "  e2e                     process E2E (health/ready/SIGTERM/config)"
	@echo "  migrate / migrate-*     Studio goose CLI (D1)"
	@echo "  freeze-check/write      migration freeze inventory"
	@echo "  db-test                 Python schema/freeze suite"
	@echo "  contracts-validate      offline buf/protoc/OpenAPI validate"
	@echo "  contracts-ownership     ownership + layout scan (C1)"
	@echo "  contracts-buf-generate  buf generate via local pinned plugins (C4)"
	@echo "  clients-go-grpc-build   generate stubs then go test clients/go-grpc (C4)"
	@echo "  contracts-openapi-emit  openapi-gen offline emit to contracts/.tmp (C6)"
	@echo "  clients-go-rest-build   generate ogen client then go test clients/go-rest (C7)"
	@echo "  clients-ts-rest-build   generate openapi-fetch types then strict tsc (C7)"
	@echo "  clients-generate        emit + fan-out all client generation (C4/C7)"
	@echo "  contracts-parity        closed-enum parity gate (C2)"
	@echo "  contracts-gates         policy gates incl. no-tracked-gen (C1/C10)"
	@echo "  contracts-conformance   generated-client loopback matrix + coverage (C11)"
	@echo "  studio-mcp-e2e          P19 + E12 MCP conformance tests"
	@echo "  studio-mcp-conformance  E12-only C12 MCP conformance suite"
	@echo "  studio-mcp-coverage     emit mcp-coverage.json evidence artifact"
	@echo "  studio-mcp-external-e2e external Streamable HTTP client (BLOCKED if mcporter absent)"
	@echo "  contracts-spikes        hard-type qualification spikes (C3)"

# --- Service shell (S1) ---

build:
	mkdir -p "$(STUDIO_ROOT)/../bin"
	go build -o "$(STUDIO_ROOT)/../bin/studio-server" ./cmd/studio-server

# --- DB / persistence (D1/D2) ---

test:
	go test ./... -count=1

test-race:
	go test ./... -race -count=1

e2e:
	go test ./internal/testutil/e2e/... -count=1 -timeout 10m

cover:
	go test ./internal/... -count=1 -coverprofile=/tmp/studio-cover.out -coverpkg=./internal/...
	go tool cover -func=/tmp/studio-cover.out | tail -1

migrate migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-status:
	go run ./cmd/migrate status

freeze-check:
	go run ./cmd/migrate -check-freeze

freeze-write:
	go run ./cmd/migrate -write-freeze

db-test studio-db-pytest:
	cd db && python3 -m pytest tests -q

studio-test:
	go test ./... -count=1

studio-mcp-e2e:
	GOWORK=off go test ./internal/mcp/... -v -run 'TestP19|TestE12' -count=1 -timeout 60s

studio-mcp-conformance:
	GOWORK=off go test ./internal/mcp/conformance/... -v -count=1 -timeout 60s

studio-mcp-coverage:
	GOWORK=off go test ./internal/mcp/conformance/... -run TestE12_10_CoverageMatrix -count=1 -timeout 30s

# External-client conformance is opt-in because mcporter/Hermes is not a Go
# dependency and is not guaranteed in every build image. Never silently pass.
studio-mcp-external-e2e:
	bash "$(STUDIO_ROOT)/scripts/mcp-external-e2e.sh"
studio-cover:
	@echo "Use root 'make studio-cover' for the enforced COVER_MIN gate (>=85%)."
	@cd .. && make studio-cover

studio-backup-drill:
	./db/scripts/backup_restore_drill.sh

# --- Contracts (C1+) ---

contracts-validate:
	cd "$(CONTRACTS)" && ./scripts/validate.sh

contracts-ownership:
	python3 "$(GATES)/ownership_scan.py" --studio-root "$(STUDIO_ROOT)"

# C4: local pinned plugins only (no BSR remote).
contracts-buf-generate:
	cd "$(CONTRACTS)" && ./scripts/generate.sh

clients-go-rest-generate: contracts-openapi-emit
	bash "$(STUDIO_ROOT)/clients/go-rest/scripts/generate.sh"

clients-go-rest-build: clients-go-rest-generate
	cd "$(STUDIO_ROOT)" && GOWORK=off go test ./clients/go-rest/... -count=1

clients-ts-rest-generate: contracts-openapi-emit
	bash "$(STUDIO_ROOT)/clients/ts-rest/scripts/generate.sh"

clients-ts-rest-build: clients-ts-rest-generate
	cd "$(STUDIO_ROOT)/clients/ts-rest" && npm ci --ignore-scripts --no-audit --no-fund && npm run build

clients-go-grpc-build: contracts-buf-generate clients-go-rest-generate
	cd "$(STUDIO_ROOT)" && GOWORK=off go test ./clients/go-grpc/... -count=1

# C6: offline Huma emission.
contracts-openapi-emit:
	mkdir -p "$(CONTRACTS)/.tmp"
	go run ./cmd/openapi-gen -out "$(CONTRACTS)/.tmp/openapi.emitted.yaml"
	if python3 -c 'import openapi_spec_validator' >/dev/null 2>&1; then \
		python3 -m openapi_spec_validator "$(CONTRACTS)/.tmp/openapi.emitted.yaml"; \
	else \
		echo "WARN: openapi-spec-validator not installed; emitted spec was generated offline"; \
	fi

# C7: emitted OpenAPI is the only REST client generation input.
clients-generate: contracts-openapi-emit contracts-buf-generate clients-go-rest-build clients-ts-rest-build
	@echo "OK: clients-generate (protobuf + Go REST + TypeScript REST)"

# Filled in C2 — keep name stable.
contracts-parity:
	bash "$(CONTRACTS)/scripts/parity.sh"

contracts-gates: contracts-ownership contracts-openapi-emit
	bash "$(CONTRACTS)/scripts/check-baseline.sh"
	bash "$(CONTRACTS)/scripts/check-buf-breaking.sh"
	bash "$(CONTRACTS)/scripts/check-openapi-breaking.sh"
	bash "$(GATES)/check_no_tracked_generated.sh"
	bash "$(GATES)/check-raw-transport.sh"
	bash "$(GATES)/check_no_tracked_generated.sh" --self-test
	bash "$(CONTRACTS)/scripts/bootstrap_local_plugins.sh" --self-test
	# Nested spike harness must not pollute production module requirements.
	@echo "==== production module go mod tidy -diff (must be empty) ===="
	cd "$(STUDIO_ROOT)" && GOWORK=off go mod tidy -diff
	@test -f "$(GATES)/spikes/go/harness/go.mod" || { echo "FAIL: missing nested harness go.mod" >&2; exit 1; }
	@grep -q '^module spike.local/harness$$' "$(GATES)/spikes/go/harness/go.mod" \
		|| { echo "FAIL: harness go.mod must declare module spike.local/harness" >&2; exit 1; }

contracts-conformance: contracts-buf-generate clients-go-rest-generate
	cd "$(STUDIO_ROOT)" && GOWORK=off go test ./internal/conformance ./clients/go-grpc/... ./clients/go-rest/... -count=1 -timeout 10m

contracts-ci: contracts-validate contracts-openapi-emit clients-generate contracts-conformance
	OPENAPI_PATH="$(CONTRACTS)/.tmp/openapi.emitted.yaml" bash "$(CONTRACTS)/scripts/parity.sh"
	bash "$(CONTRACTS)/scripts/check-buf-breaking.sh"
	bash "$(CONTRACTS)/scripts/check-openapi-breaking.sh"
	bash "$(GATES)/check-raw-transport.sh"
	cd "$(STUDIO_ROOT)" && GOWORK=off go test ./... -count=1
	bash "$(GATES)/check_no_tracked_generated.sh"
	git diff --check

# Planted reds must fail their target and leave the checkout unchanged.
gates-red-proof:
	bash "$(CONTRACTS)/scripts/red-proof.sh"

# C3 qualification spikes
contracts-spikes:
	bash "$(GATES)/spikes/run_spikes.sh"
