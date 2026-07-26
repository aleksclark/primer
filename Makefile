# Primer LMS — build, test, and codegen entry points.

COVER_MIN := 85

.PHONY: all build test cover openapi client web bundle docker dev-db migrate lint

all: build openapi client

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

## Generate the OpenAPI spec from API type signatures.
openapi:
	cd server && go run ./cmd/openapi-gen -out ../web/openapi.yaml

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

## Start a local PostgreSQL for development.
dev-db:
	docker run -d --name primer-pg -p 5432:5432 \
		-e POSTGRES_USER=primer -e POSTGRES_PASSWORD=primer -e POSTGRES_DB=primer \
		postgres:17-alpine

## Apply migrations to the dev database.
migrate:
	cd server && go run ./cmd/migrate up

## Vet the Go code.
lint:
	cd server && go vet ./...
