# cimento-sdk

SDKs for [Cimento](https://cimento.ai).

> [!NOTE]
> Nothing here is released yet. Both SDKs work; the Go module is not tagged and the TypeScript package is not published.

| Package                                         | Language   | Path                                         | What it is for                                                                             |
| ----------------------------------------------- | ---------- | -------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `github.com/cimento-ai/cimento-sdk/go/webhooks` | Go         | [`go/webhooks`](go/webhooks)                 | Verify [webhooks](https://docs.cimento.ai/webhooks/overview) and read them as typed events |
| `@cimento/webhooks`                             | TypeScript | [`typescript/webhooks`](typescript/webhooks) | The same, for Node and other JavaScript runtimes                                           |

## Layout

Each SDK is its own Go module or npm package, with its own version.

```
schema/webhooks/     the JSON Schema of every webhook event; the SDKs' types come from it
fixtures/webhooks/   signature cases and event bodies that both SDKs are tested against
internal/            tooling for this repository: the type generator and the check on the fixtures
go/webhooks/         the Go module
typescript/          a pnpm workspace
  webhooks/          the @cimento/webhooks package
```

## Developing

You need Go 1.26 or newer, Node 22 or newer with [pnpm](https://pnpm.io), and [golangci-lint](https://golangci-lint.run) v2.

```sh
cd typescript && pnpm install && cd ..
make lint
make test
```

`schema/webhooks/events.schema.json` is a copy of a file Cimento generates. Do not edit it here.

The event types in both SDKs (`events_gen.go`, `events.gen.ts`) are generated from that schema by `make generate`. Do not edit them either; CI fails when they are not what the generator writes.

## Checking the SDKs against recorded webhooks

Two targets run files of webhooks through both SDKs and fail if either refuses one, fails to read one as its type, or the two make different things of it:

```sh
make check-events EVENTS=<dir>            # every file is an event body
make check-deliveries DELIVERIES=<dir>    # recorded requests
```

A recorded request is two files with one name: `<name>.body`, the request's body byte for byte, and `<name>.json`, an object with `headers` (each name to a list of values) and `receivedAt` (when it arrived). Each is verified as a receiver would have verified it at that time, with the secrets in `CIMENTO_WEBHOOK_SECRETS`, separated by commas.

A file is also a problem when the two SDKs read it differently: another type, another time, or another value for a field of the event's type.

`make check-fixtures` holds the SDKs to each other over the shared event fixtures. `make test` and CI run it.

## Security

See [SECURITY.md](SECURITY.md).

## Licence

[MIT](LICENSE)
