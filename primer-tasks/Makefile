# Primer Tasks product-local developer targets.
# The lifecycle wrapper is the authority for Compose project isolation.

DEV := ./scripts/dev

.PHONY: check up dev status endpoints logs down destroy proof \
	build test cover openapi clients tasks-client tasks-typecheck tasks-lint \
	web tasks-web browser-test android e2e host-up host-down host-e2e host-proof \
	tasks-test tasks-cover tasks-clients tasks-android tasks-e2e tasks-release

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

tasks-release:
	go build -o ../bin/tasks-release ./cmd/tasks-release

test:
	go test ./... -count=1

cover:
	PRIMER_TASKS_COVERAGE_GATE=1 ../scripts/enforce-module-cover.sh . 85 tasks

doc: openapi

openapi:
	mkdir -p build
	go run ./cmd/openapi-gen -out build/openapi.yaml

clients: openapi
	npm --prefix clients/typescript ci --no-audit --no-fund
	npm --prefix clients/typescript run generate
	node clients/kotlin/generate-client.mjs

tasks-client: clients

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
	cd android && ./gradlew test assembleDebug

tasks-test: test
tasks-cover: cover
tasks-clients: clients
tasks-android: android
tasks-e2e: e2e

host-up:
	./scripts/host-stack up

host-down:
	./scripts/host-stack down

host-e2e: host-up
	set -a; . tmp/host-stack/endpoints.env; set +a; \
	PRIMER_TASKS_EXPLORATORY_BROWSER_PASS=1 npm --prefix web run browser:promoted; \
	status=$$?; $(MAKE) host-down; exit $$status

host-proof:
	./scripts/prove-host.sh

# Default E2E is the host/non-Compose public browser path.
e2e: host-e2e
