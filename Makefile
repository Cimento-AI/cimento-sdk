# The cimento-api checkout the event schema is copied from.
CIMENTO_API ?= ../cimento-api

.PHONY: test lint generate sync-schema check-events check-deliveries

test:
	cd go/webhooks && go vet ./... && go test -race -shuffle=on -count=1 ./...
	cd internal && go vet ./... && go test -race -shuffle=on -count=1 ./...
	cd typescript && pnpm -r test

lint:
	cd go/webhooks && go mod tidy -diff && golangci-lint run ./...
	cd internal && go mod tidy -diff && golangci-lint run ./...
	cd typescript && pnpm lint && pnpm -r lint

# Writes the SDKs' event types from the schema. The files it writes are
# committed, and CI fails when they are not what this produces.
generate:
	cd internal && go run ./gen -root ..

# The schema belongs to cimento-api. This copies it; it is never edited here.
sync-schema:
	cp $(CIMENTO_API)/webhooks/schema/events.schema.json schema/webhooks/events.schema.json

# Runs files of webhooks through both SDKs and fails if either refuses one,
# fails to read one as its type, or the two disagree.
#
#   make check-events EVENTS=<dir>            every file is an event body
#   make check-deliveries DELIVERIES=<dir>    recorded requests: <name>.body
#                                             and <name>.json (see internal/check)
#
# check-deliveries verifies with the secrets in CIMENTO_WEBHOOK_SECRETS,
# separated by commas.
check-events:
	@test -n "$(EVENTS)" || { echo "usage: make check-events EVENTS=<dir>"; exit 2; }
	cd typescript && pnpm --filter @cimento/webhooks build >/dev/null
	cd internal && go run ./check -root .. -events "$(abspath $(EVENTS))"

check-deliveries:
	@test -n "$(DELIVERIES)" || { echo "usage: make check-deliveries DELIVERIES=<dir>"; exit 2; }
	cd typescript && pnpm --filter @cimento/webhooks build >/dev/null
	cd internal && go run ./check -root .. -deliveries "$(abspath $(DELIVERIES))"
