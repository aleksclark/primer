# Curriculum Studio module-local targets (root Makefile is F0-owned).
#
# Root wiring request: point `make migrate-studio` at:
#   cd curriculum-studio && $(MAKE) migrate
# and `make dev-db-studio` at a real curriculum_studio DB once compose exists.

.PHONY: test migrate migrate-up migrate-down migrate-status freeze-check freeze-write db-test cover

test:
	go test ./... -count=1

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

db-test:
	cd db && python3 -m pytest tests -q
