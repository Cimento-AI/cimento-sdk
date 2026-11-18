# cimento-sdk

Public SDKs for Cimento. Each SDK is its own Go module (`go/<name>`) or npm package (`typescript/<name>`).

## This repository is public

- Nothing internal goes in: no internal hostnames, no tenant or customer data, no secrets, nothing copied from a private repository.
- Fixtures and examples use made-up ids and `example.com` addresses.

## The event schema

- `schema/webhooks/events.schema.json` belongs to Cimento's API, which generates it. `make sync-schema` copies it. Never edit the copy by hand.
- `x-known-values` lists the values a text field takes today. The set is open: an SDK must accept a value that is not listed.
- An SDK must accept an event type it does not know, and a field it does not know.

## Both SDKs behave the same

A change to how one SDK verifies or parses is a change to both. They are tested against the same shared fixtures.

## Go

- Standard library only. No `init()`.
- One explicit type per parameter: `(a string, b string)`, not `(a, b string)`.
- Sentinel errors (`var ErrX = errors.New(...)`, wrapped with `%w`) so callers can use `errors.Is`.
- Few comments. Comment what the code cannot say.
- Tests use the standard library and `t.Parallel()`.

## TypeScript

- No runtime dependencies. Use Web Crypto, not `node:crypto`, so the package runs outside Node.
- pnpm. `pnpm lint` in `typescript/` checks eslint and prettier; `pnpm -r lint` typechecks, builds and checks each package as it would be published.

## Checks

`make lint` and `make test` from the root run what CI runs.
