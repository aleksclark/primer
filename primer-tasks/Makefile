# Primer Tasks product-local developer targets.
# The lifecycle wrapper is the authority for Compose project isolation.

DEV := ./scripts/dev

.PHONY: check up dev status endpoints logs down destroy proof \
	build test cover openapi clients tasks-client tasks-typecheck tasks-lint \
	web tasks-web browser-test android e2e

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

android:
	cd android && ./gradlew assembleDebug

# Keep the historical E2E entry point honest: it is the gated browser
# boundary until exploratory PASS authorizes a real runner.
e2e: browser-test
