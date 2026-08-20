# Primer Tasks product-local developer targets.
# The lifecycle wrapper is the authority for Compose project isolation.

DEV := ./scripts/dev

.PHONY: check up dev status endpoints logs down destroy proof \
	build test cover openapi agent-protocol clients tasks-client tasks-agent-compat tasks-typecheck tasks-lint \
	web tasks-web browser-test android e2e tasks-test tasks-cover tasks-clients tasks-android tasks-e2e tasks-live-image-e2e

check:
	$(DEV) check

up dev:
	$(DEV) $@

status endpoints:
	$(DEV) $@

logs:
	$(DEV) logs

down:
	$(DEV) down

destroy:
	$(DEV) destroy

proof:
	$(DEV) proof

build:
	go build ./...

test:
	go test ./... -count=1

cover:
	PRIMER_TASKS_COVERAGE_GATE=1 ../scripts/enforce-module-cover.sh . 85 tasks

doc: openapi

openapi:
	mkdir -p build
	go run ./cmd/openapi-gen -out build/openapi.yaml

agent-protocol:
	go run ./cmd/agent-protocol-gen -out build/agent-protocol.schema.json

clients: openapi agent-protocol
	npm --prefix clients/typescript ci --no-audit --no-fund
	npm --prefix clients/typescript run generate
	node clients/kotlin/generate-client.mjs

tasks-client: clients

tasks-agent-compat: agent-protocol
	@test "$(shell grep -F 'charm.land/fantasy v0.41.1' go.mod | wc -l)" = "1"
	@test "$$(grep -R -n 'fantasy/internal\|server/internal/agent\|internal/maf' internal/agent cmd/agent-protocol-gen 2>/dev/null || true)" = ""
	@go test ./internal/agent/... ./internal/jobs/... ./internal/domain/parent

tasks-typecheck: clients
	npm --prefix web ci --no-audit --no-fund
	npm --prefix web run typecheck

tasks-lint:
	npm --prefix web ci --no-audit --no-fund
	npm --prefix web run lint

web: clients
	npm --prefix web ci --no-audit --no-fund
	npm --prefix web run build

tasks-web: web

browser-test:
	npm --prefix web run browser:test

android: clients
	cd android && ./gradlew testDebugUnitTest assembleDebug

tasks-test: test
tasks-cover: cover
tasks-clients: clients
tasks-android: android
tasks-e2e: e2e

# Live multimodal qualification is opt-in and fail-closed. Scripted fixtures
# never satisfy this target and no quality claim is made while blocked.
tasks-live-image-e2e:
	@if [ "$(TASKS_LIVE_MODEL_TEST)" != "1" ]; then echo "BLOCKED: set TASKS_LIVE_MODEL_TEST=1 with approved provider credentials and fixture"; exit 2; fi
	@echo "BLOCKED: live image qualification adapter requires explicit provider approval"; exit 2

# Keep the historical E2E entry point honest: it is the gated browser
# boundary until exploratory PASS authorizes a real runner.
e2e: browser-test
