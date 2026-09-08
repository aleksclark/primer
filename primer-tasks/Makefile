# Primer Tasks product-local developer targets.
# The lifecycle wrapper is the authority for Compose project isolation.

DEV := ./scripts/dev

.PHONY: check up dev status endpoints logs down destroy proof \
	build test cover openapi agent-protocol clients clients-typescript tasks-client tasks-agent-compat kotlin-client tasks-typecheck tasks-lint \
	web tasks-web browser-test android e2e host-up host-down host-e2e host-proof \
	ui-mocks ui-mocks-web ui-mocks-android ui-mocks-review ui-mocks-update \
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
	go test ./... -count=1 -timeout 20m

cover:
	PRIMER_TASKS_COVERAGE_GATE=1 ../scripts/enforce-module-cover.sh . 85 tasks

doc: openapi

openapi:
	mkdir -p build
	go run ./cmd/openapi-gen -out build/openapi.yaml

agent-protocol:
	go run ./cmd/agent-protocol-gen -out build/agent-protocol.schema.json

# TypeScript qualification is independently runnable while native source is
# separately owned. The existing clients target retains its Kotlin build edge.
clients-typescript:
	npm --prefix clients/typescript ci --no-audit --no-fund
	npm --prefix clients/typescript run generate
	npm --prefix clients/typescript run typecheck
	npm --prefix clients/typescript run check:boundary
	npm --prefix clients/typescript test
	npm --prefix clients/typescript run test:generation
	go test -tags tasksclientcontract ./internal/api -run '^TestGeneratedStudentClientUsesPublicBoundary$$' -count=1

clients: clients-typescript
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

ui-mocks-web: clients
	npm --prefix web ci --no-audit --no-fund
	npm --prefix web run typecheck
	npm --prefix web run test:unit
	npm --prefix web run storybook:build
	npm --prefix web run storybook:test
	npm --prefix web run storybook:visual

ui-mocks-android: clients
	cd ../android && JAVA_HOME=$${JAVA_HOME:-/usr/lib/jvm/java-17-openjdk} ANDROID_HOME=$${ANDROID_HOME:-/opt/android-sdk} ./gradlew :feature-tasks-student:testDebugUnitTest :feature-tasks-student:assembleDebug --no-daemon --max-workers=2

ui-mocks: ui-mocks-web ui-mocks-android

ui-mocks-review:
	@test -n "$$CHROMATIC_PROJECT_TOKEN" || (echo "CHROMATIC_PROJECT_TOKEN is required"; exit 1)
	npm --prefix web run chromatic

ui-mocks-update:
	npm --prefix web run storybook:visual:update

browser-test:
	npm --prefix web run browser:test

kotlin-client: clients
	cd ../android && ./gradlew :tasks-client:test --no-daemon --max-workers=1

# Compatibility wrapper for the retired prototype APK. Student now lives in
# ../android/:feature-tasks-student; keep this target until that flow is proven.
android: kotlin-client
	cd android && ./gradlew test assembleDebug --no-daemon --max-workers=1

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
