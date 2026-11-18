# The cimento-api checkout the event schema is copied from.
CIMENTO_API ?= ../cimento-api

.PHONY: test lint sync-schema

test:
	cd go/webhooks && go vet ./... && go test -race -shuffle=on -count=1 ./...
	cd typescript && pnpm -r test

lint:
	cd go/webhooks && go mod tidy -diff && golangci-lint run ./...
	cd typescript && pnpm lint && pnpm -r lint

# The schema belongs to cimento-api. This copies it; it is never edited here.
sync-schema:
	cp $(CIMENTO_API)/webhooks/schema/events.schema.json schema/webhooks/events.schema.json
