# Primer LMS + TV — build, test, and codegen entry points.

# Load local service credentials for content-ingest. Keep .env git-ignored.
ifneq (,$(wildcard .env))
include .env
export
endif

COVER_MIN := 85

.PHONY: all build test cover openapi openapi-tv client web bundle docker docker-tv deploy \
	dev-db dev-db-tv migrate migrate-tv lint tv-build tv-test tv-server \
	tv-client tv-web tv-bundle ingest-build ingest-plan ingest-review ingest-apply design-system \
	activity-validate activity-publish student-build student-deploy \
	workstation-package workstation-check update-student-vendor-hash

all: build openapi openapi-tv client tv-client

## Generate and validate cross-platform design tokens and the review preview.
design-system:
	python3 design-system/build.py

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
