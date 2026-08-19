# Primer LMS + TV — build, test, and codegen entry points.

# Load local service credentials for content-ingest. Keep .env git-ignored.
ifneq (,$(wildcard .env))
include .env
export
endif

COVER_MIN := 85
# Module coverage floors (Identity starts lower; Studio matches root COVER_MIN).
STUDIO_COVER_MIN := 85
IDENTITY_COVER_MIN := 80

.PHONY: all build test cover openapi openapi-tv client web bundle docker docker-tv deploy \
	dev-db dev-db-tv migrate migrate-tv lint tv-build tv-test tv-server \
	tv-client tv-web tv-bundle ingest-build ingest-plan ingest-review ingest-apply design-system \
	activity-validate activity-publish student-build student-deploy student-acceptance \
	student-stub student-harness \
	workstation-package workstation-check update-student-vendor-hash \
	investor-web investor-web-dev investor-web-test investor-web-ci \
	foundation-check agent-runtime-check \
	studio-build studio-test studio-cover studio-openapi studio-client studio-web \
	studio-e2e studio-e2e-go dev-db-studio migrate-studio \
	identity-build identity-test identity-cover identity-openapi identity-test-oauth \
	identity-e2e identity-live-stytch dev-db-identity migrate-identity

all: build openapi openapi-tv client tv-client

## Generate and validate cross-platform design tokens and the review preview.
design-system:
	python3 design-system/build.py

## Build the investor pitch site (regenerates design-system tokens first).
investor-web: design-system
	cd investor-web && npm run build

## Run the investor pitch Vite dev server.
investor-web-dev:
	cd investor-web && npm run dev

## Investor data, launch, a11y, and unit checks (no production build).
investor-web-test:
	cd investor-web && npm test

## CI sequence: design-system → tests → typecheck/lint → build → bundle budgets.
investor-web-ci: design-system
	cd investor-web && npm run gate

## Build the server binaries.
build:
	cd server && go build ./...

## Run the full test suite (integration tests use a PostgreSQL testcontainer).
test:
	cd server && go test ./... -count=1

## Run tests with coverage and enforce the minimum threshold.
cover:
	cd server && go test ./internal/... -count=1 -coverprofile=coverage.out -coverpkg=./internal/...
	cd server && go tool cover -func=coverage.out | tail -1
	@cd server && total=$$(go tool cover -func=coverage.out | tail -1 | awk '{print $$3}' | tr -d '%'); \
	if [ $$(echo "$$total < $(COVER_MIN)" | bc) -eq 1 ]; then \
		echo "FAIL: coverage $$total% is below $(COVER_MIN)%"; exit 1; \
	else \
		echo "OK: coverage $$total% >= $(COVER_MIN)%"; \
	fi

## Generate the LMS OpenAPI spec from API type signatures.
openapi:
	cd server && go run ./cmd/openapi-gen -service lms -out ../web/openapi.yaml

## Generate the TV OpenAPI spec from API type signatures.
openapi-tv:
	cd server && go run ./cmd/openapi-gen -service tv -out ../tv-web/openapi.yaml

## Generate the TypeScript client from the OpenAPI spec (build-time codegen).
client: openapi
	cd web && npm run generate:client

## Build the admin SPA (regenerates the client first).
web: client
	cd web && npm run build

## Copy the built SPA into the server for an embedded local build.
bundle: web
	rm -rf server/internal/spa/dist
	cp -r web/dist server/internal/spa/dist
	cd server && go build ./cmd/primer-server

## Build the deployment image (SPA bundled into the server binary).
docker:
	docker build -t primer-lms .

## Build the TV server deployment image (TV admin SPA bundled in).
docker-tv:
	docker build -f Dockerfile.tv -t primer-tv .

## Build, push, and deploy to the Nomad fleet (primer.fleet.clark.team).
## Requires deploy/.env — see deploy/.env.example.
deploy:
	./deploy/deploy.sh

## Start a local PostgreSQL for development.
dev-db:
	docker run -d --name primer-pg -p 5432:5432 \
		-e POSTGRES_USER=primer -e POSTGRES_PASSWORD=primer -e POSTGRES_DB=primer \
		postgres:17-alpine

## Create the TV database inside the local PostgreSQL.
dev-db-tv:
	docker exec primer-pg createdb -U primer primer_tv

## Apply LMS migrations to the dev database.
migrate:
	cd server && go run ./cmd/migrate -service lms up

## Apply TV migrations to the TV dev database.
migrate-tv:
	cd server && go run ./cmd/migrate -service tv up

## Build just the TV server binary.
tv-build:
	cd server && go build ./cmd/tv-server

## Run the TV server test suite.
tv-test:
	cd server && go test ./internal/tv/... -count=1

## Run the TV server locally.
tv-server:
	cd server && go run ./cmd/tv-server

## Generate the TV TypeScript client from the TV OpenAPI spec.
tv-client: openapi-tv
	cd tv-web && npm run generate:client

## Build the TV admin SPA (regenerates the client first).
tv-web: tv-client
	cd tv-web && npm run build

## Copy the built TV SPA into the server for an embedded local build.
tv-bundle: tv-web
	rm -rf server/internal/tv/spa/dist
	cp -r tv-web/dist server/internal/tv/spa/dist
	cd server && go build ./cmd/tv-server

## Vet the Go code.
lint:
	cd server && go vet ./...

## Build the content-ingest binary.
ingest-build:
	cd server && go build -o ../bin/content-ingest ./cmd/content-ingest

## Show the content-ingest diff (writes review.yaml candidates + a report).
ingest-plan: ingest-build
	./bin/content-ingest plan

## Interactive TUI to pick candidates in curriculum/content-review.yaml.
ingest-review: ingest-build
	./bin/content-ingest review

## Converge Radarr/Sonarr/yt-dlp/Jellyfin/TV toward the content manifest.
ingest-apply: ingest-build
	./bin/content-ingest apply

## Validate curriculum/activities against student-client contracts (offline, no DB).
activity-validate:
	cd server && go run ./cmd/activity-validate -dir ../curriculum/activities

## Publish curriculum standards + activity revisions into the LMS database.
## Requires DATABASE_URL (see server/internal/config).
activity-publish:
	cd server && go run ./cmd/activity-publish -activities ../curriculum/activities -standards ../curriculum/standards

## TEST-ONLY: Phase 1 work-queue stub TUI. Do not deploy to workstations.
## Prefer primer-student (packaged via workstation flake) for real instruction.
student-stub:
	cd server && go build -o ../bin/primer-student-stub ./cmd/primer-student-stub

## TEST-ONLY: headless engine harness for CI/acceptance. Do not deploy.
student-harness:
	cd server && go build -o ../bin/primer-student-harness ./cmd/primer-student-harness

## API + headless acceptance smoke (requires running LMS + parent credentials).
## See scripts/student-acceptance.sh and agent_docs/runbooks/student-client-ops.md.
student-acceptance:
	./scripts/student-acceptance.sh

## Build the interactive student workstation TUI with version/commit ldflags.
STUDENT_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
STUDENT_COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
student-build:
	cd server && go build -ldflags="-s -w -X main.version=$(STUDENT_VERSION) -X main.commit=$(STUDENT_COMMIT)" \
		-o ../bin/primer-student ./cmd/primer-student

## DEPRECATED: scp a prebuilt binary to /var/lib/primer-student/bin.
## Prefer: cd workstation && ./deploy.sh  (Nix package is the default now).
## Usage: make student-deploy HOST=root@primer.local
HOST ?= root@primer.local
student-deploy: student-build
	@echo "WARNING: student-deploy is deprecated; use workstation flake package via ./deploy.sh" >&2
	ssh "$(HOST)" 'mkdir -p /var/lib/primer-student/bin && chown student:students /var/lib/primer-student /var/lib/primer-student/bin'
	scp bin/primer-student "$(HOST):/var/lib/primer-student/bin/primer-student"
	ssh "$(HOST)" 'chown student:students /var/lib/primer-student/bin/primer-student && chmod 755 /var/lib/primer-student/bin/primer-student && primer-student-health'

## Build primer-student via the workstation flake (Docker Nix when host nix is broken).
## Mounts the Primer parent tree so git worktrees resolve inside the container.
PRIMER_ROOT ?= $(shell cd "$(CURDIR)/../.." && pwd)
workstation-package:
	docker volume create primer-nix-store >/dev/null
	docker run --rm \
		-v primer-nix-store:/nix \
		-v "$(PRIMER_ROOT):$(PRIMER_ROOT)" \
		-w "$(CURDIR)/workstation" \
		-e NIX_CONFIG='experimental-features = nix-command flakes' \
		nixos/nix:2.24.11 \
		sh -c 'git config --global --add safe.directory "*" && nix build .#primer-student --option sandbox false --print-out-paths'

## Run flake checks (package build + activity-validate) in Docker Nix.
workstation-check:
	docker volume create primer-nix-store >/dev/null
	docker run --rm \
		-v primer-nix-store:/nix \
		-v "$(PRIMER_ROOT):$(PRIMER_ROOT)" \
		-w "$(CURDIR)/workstation" \
		-e NIX_CONFIG='experimental-features = nix-command flakes' \
		nixos/nix:2.24.11 \
		sh -c 'git config --global --add safe.directory "*" && nix build \
			.#checks.x86_64-linux.primer-student \
			.#checks.x86_64-linux.runtime-coreutils-basic \
			.#checks.x86_64-linux.activity-validate \
			.#checks.x86_64-linux.workstation-eval \
			--option sandbox false --print-out-paths'

## Recompute packages/primer-student.nix vendorHash after go.mod changes.
update-student-vendor-hash:
	./workstation/scripts/update-primer-student-vendor-hash.sh

# =============================================================================
# Curriculum Studio + Primer Identity foundations (delivery wave F0)
# Honest targets only: compile/test where modules exist; clear deferral otherwise.
# Binaries, migrations, coverage floors, OpenAPI clients, and E2E are owned by
# later S1/I1+ waves — do not claim them green here.
# =============================================================================

## Mechanical F0 foundation check (module paths, go.work, Make names, no coupling).
foundation-check:
	./scripts/check-f0-foundations.sh

## Verify the pinned public-preview MAF production boundary.
agent-runtime-check:
	./scripts/check-maf-runtime.sh

## Studio module unit/package tests (minimal F0 root; no business coverage claim).
studio-test:
	cd curriculum-studio && go test ./...

## Studio binary build — deferred until cmd/studio-server exists (S1).
studio-build:
	@if [ -d curriculum-studio/cmd/studio-server ]; then \
		cd curriculum-studio && go build -o ../bin/studio-server ./cmd/studio-server; \
	else \
		echo "studio-build: deferred until curriculum-studio/cmd/studio-server exists (S1)"; \
		exit 2; \
	fi

## Studio coverage gate (≥85%) — deferred until internal packages exist.
studio-cover:
	@./scripts/enforce-module-cover.sh curriculum-studio $(STUDIO_COVER_MIN) studio

## Studio OpenAPI emission from Huma handler signatures.
studio-openapi:
	$(MAKE) -C curriculum-studio contracts-openapi-emit

## Studio TS client codegen from the emitted Huma OpenAPI contract.
studio-client:
	$(MAKE) -C curriculum-studio clients-ts-rest-build

## Studio SPA build. The SPA uses the generated client and existing BFF session.
studio-web: studio-client
	@if [ ! -f curriculum-studio/web/package-lock.json ]; then echo "studio-web: missing curriculum-studio/web/package-lock.json" >&2; exit 2; fi
	cd curriculum-studio/web && npm ci --ignore-scripts --no-audit --no-fund && npm run build

## Studio shell build smoke; configured deployments add the real browser journey.
studio-e2e: studio-web
	@echo "studio-e2e: shell build verified; browser journey requires the configured Studio database and Identity fixture"

## Studio Go process E2E (S1 harness under internal/testutil/e2e).
studio-e2e-go:
	cd curriculum-studio && go test ./internal/testutil/e2e/ -count=1 -timeout 10m

## Create Studio dev database — deferred: no coherent additive Compose surface
## exists for curriculum_studio yet (F0 will not invent hollow compose). Use a
## disposable Postgres + STUDIO_DATABASE_URL with migrate-studio, or S1/D1
## compose once it lands.
dev-db-studio:
	@echo "dev-db-studio: deferred — no coherent Compose surface for curriculum_studio in F0 (refusing hollow compose); use disposable Postgres + STUDIO_DATABASE_URL with migrate-studio, or S1/D1 compose when added"; exit 2

## Apply Studio migrations (D1). Requires STUDIO_DATABASE_URL; never prints DSN.
## F0 sole ownership of this root target name — delegates to reviewed Studio CLI.
migrate-studio:
	@if [ -z "$${STUDIO_DATABASE_URL:-}" ]; then \
		echo "migrate-studio: STUDIO_DATABASE_URL is required (no fallback to DATABASE_URL)" >&2; \
		exit 2; \
	fi
	@cd curriculum-studio && go run ./cmd/migrate up

## Identity module unit/package tests (minimal F0 root; no business coverage claim).
identity-test:
	cd primer-identity && go test ./...

## Identity binary build — deferred until cmd/identity-server exists (I1).
identity-build:
	@if [ -d primer-identity/cmd/identity-server ]; then \
		cd primer-identity && go build -o ../bin/identity-server ./cmd/identity-server; \
	else \
		echo "identity-build: deferred until primer-identity/cmd/identity-server exists (I1)"; \
		exit 2; \
	fi

## Identity coverage gate (≥80%) — deferred until internal packages exist (I1+; raise later).
identity-cover:
	@./scripts/enforce-module-cover.sh primer-identity $(IDENTITY_COVER_MIN) identity

## Identity OpenAPI emission + generated IB1 client — fail-closed drift check.
identity-openapi:
	@spec="$$(mktemp)"; \
	client="$$(mktemp)"; \
	trap 'rm -f "$$spec" "$$client"' EXIT; \
	( cd primer-identity && go run ./cmd/openapi-gen -out "$$spec" ) && \
	cmp -s "$$spec" primer-identity/openapi.yaml && \
	( cd primer-identity && go tool oapi-codegen -package identityclient -generate types,client -o "$$client" "$$spec" ) && \
	cmp -s "$$client" primer-identity/client/client.gen.go

## Identity OAuth / IB1 adversarial suite (E01..E10 relevant packages, race, real DB).
identity-test-oauth:
	cd primer-identity && go test -race -count=1 \
		./client \
		./cmd/openapi-gen \
		./internal/api \
		./internal/app \
		./internal/broker \
		./internal/brokerprovider \
		./internal/config \
		./internal/db \
		./internal/domain \
		./internal/keys \
		./internal/oauth \
		./internal/repo \
		./internal/secrethash \
		./internal/stateseal \
		./internal/stytch \
		./internal/stytchcache \
		./internal/token \
		./internal/testutil \
		./internal/testutil/e2e

## Identity process E2E (I1 harness under internal/testutil/e2e).
identity-e2e:
	cd primer-identity && go test ./internal/testutil/e2e/ -count=1 -timeout 10m

## Opt-in live Stytch *test-project* provider qualification (not IB8-E10 browser/webhook).
## Requires IDENTITY_LIVE_STYTCH=1 and IDENTITY_STYTCH_{PROJECT_ID,SECRET} (env=test).
## Optional: IDENTITY_LIVE_STYTCH_ENV_FILE=~/.config/primer/stytch-test.env
## Optional happy path: IDENTITY_LIVE_STYTCH_SESSION_TOKEN=<opaque session token>
## Never prints secrets. Excluded from default identity-test / identity-e2e.
identity-live-stytch:
	@if [ "$${IDENTITY_LIVE_STYTCH:-}" != "1" ]; then \
		echo "identity-live-stytch: set IDENTITY_LIVE_STYTCH=1 to run (refusing ambient credentials)" >&2; \
		exit 2; \
	fi
	cd primer-identity && go test -tags=live_stytch ./internal/testutil/live/ -count=1 -timeout 5m -v

## Create Identity dev database — deferred: no coherent additive Compose surface
## exists for primer_identity yet (F0 will not invent hollow compose). Use a
## disposable Postgres + IDENTITY_DATABASE_URL with migrate-identity, or I1
## compose once it lands.
dev-db-identity:
	@echo "dev-db-identity: deferred — no coherent Compose surface for primer_identity in F0 (refusing hollow compose); use disposable Postgres + IDENTITY_DATABASE_URL with migrate-identity, or I1 compose when added"; exit 2

## Apply Identity migrations (I1). Requires IDENTITY_DATABASE_URL (+ IDENTITY_ISSUER
## for config.Load). Never prints DSN. F0 sole ownership of this root target name.
migrate-identity:
	@if [ -z "$${IDENTITY_DATABASE_URL:-}" ]; then \
		echo "migrate-identity: IDENTITY_DATABASE_URL is required (no fallback to DATABASE_URL)" >&2; \
		exit 2; \
	fi
	@if [ -z "$${IDENTITY_ISSUER:-}" ]; then \
		echo "migrate-identity: IDENTITY_ISSUER is required by identity config.Load" >&2; \
		exit 2; \
	fi
	@cd primer-identity && go run ./cmd/identity-migrate up
