# cimento-sdk

Public SDKs for Cimento. Each SDK is its own Go module (`go/<name>`) or npm package (`typescript/<name>`).

## This repository is public

- Nothing internal goes in: no internal hostnames, no tenant or customer data, no secrets, nothing copied from a private repository.
- Fixtures and examples use made-up ids and `example.com` addresses.

## The event schema

- `schema/webhooks/events.schema.json` belongs to Cimento's API, which generates it. `make sync-schema` copies it. Never edit the copy by hand.
- `x-known-values` lists the values a text field takes today. The set is open: an SDK must accept a value that is not listed.
- An SDK must accept an event type it does not know, and a field it does not know.

## Generated files

- `go/webhooks/events_gen.go`, `typescript/webhooks/src/events.gen.ts` and `typescript/webhooks/test/fixtures.gen.test.ts` are written by `make generate` from the schema and the fixtures. Never edit them.
- To change a type: the schema changes in Cimento's API, then `make sync-schema`, then `make generate`. Commit all of it together.
- The generator, `internal/gen`, fails on a schema it has no mapping for. Teach it the new construct for both languages; do not work around it in one.

## Both SDKs behave the same

A change to how one SDK verifies or parses is a change to both. They are tested against the same shared fixtures, in `fixtures/webhooks`, which has a README.

- A change in behaviour starts with a fixture. Then both SDKs are made to pass it.
- An event fixture is a request body byte for byte. Do not reformat one.
- `internal/fixtures` checks the fixtures against the schema and against themselves. It shares no code with the SDKs; keep it that way.

## `internal/`

Repository tooling, in a Go module of its own. It may have dependencies. The SDK modules may not.

## Go

- Standard library only. No `init()`.
- One explicit type per parameter: `(a string, b string)`, not `(a, b string)`.
- Sentinel errors (`var ErrX = errors.New(...)`, wrapped with `%w`) so callers can use `errors.Is`.
- Few comments. Comment what the code cannot say.
- Tests use the standard library and `t.Parallel()`.

## TypeScript

- No runtime dependencies. Use Web Crypto, not `node:crypto`, so the package runs outside Node.
- pnpm. `pnpm lint` in `typescript/` checks eslint and prettier; `pnpm -r lint` typechecks, builds and checks each package as it would be published.

## Commits

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org), with the ticket as a `Refs` footer:

```
feat(webhooks): verify the signature of a delivery

Why the change was made, when the subject does not say it.

Refs: CIM-1234
```

- Types: `feat`, `fix`, `docs`, `test`, `refactor`, `build`, `ci`, `chore`. The scope is optional; use the SDK's name (`webhooks`) or the part of the repository (`gen`, `fixtures`).
- A change that breaks an SDK's users has `!` after the type or scope and a `BREAKING CHANGE:` footer.
- A pull request's title is written the same way, since it becomes the commit when the pull request is squashed.

## Checks

`make lint` and `make test` from the root run what CI runs.
