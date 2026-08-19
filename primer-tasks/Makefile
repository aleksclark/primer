# Primer Tasks product-local developer targets.
# The lifecycle wrapper is the authority for Compose project isolation.

DEV := ./scripts/dev

.PHONY: check up dev status endpoints logs down destroy \
	build test cover openapi clients tasks-client tasks-typecheck tasks-lint \
	web tasks-web android e2e

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

build:
	go build ./...

test:
	go test ./... -count=1

cover:
	mkdir -p build
	go test ./... -count=1 -coverprofile=build/coverage.out

doc: openapi

openapi:
	mkdir -p build
	go run ./cmd/openapi-gen -out build/openapi.yaml

clients: openapi
	npm --prefix clients/typescript ci --no-audit --no-fund
	npm --prefix clients/typescript run generate

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

android:
	cd android && ./gradlew assembleDebug

e2e:
	go test ./e2e/... -count=1
