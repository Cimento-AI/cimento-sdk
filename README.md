# cimento-sdk

SDKs for [Cimento](https://cimento.ai).

> [!NOTE]
> Nothing here is released yet. The packages below have no API and are not published.

| Package                                         | Language   | Path                                         | What it is for                                                                             |
| ----------------------------------------------- | ---------- | -------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `github.com/cimento-ai/cimento-sdk/go/webhooks` | Go         | [`go/webhooks`](go/webhooks)                 | Verify [webhooks](https://docs.cimento.ai/webhooks/overview) and read them as typed events |
| `@cimento/webhooks`                             | TypeScript | [`typescript/webhooks`](typescript/webhooks) | The same, for Node and other JavaScript runtimes                                           |

## Layout

Each SDK is its own Go module or npm package, with its own version.

```
schema/webhooks/     the JSON Schema of every webhook event; the SDKs' types come from it
fixtures/webhooks/   signature cases and event bodies that both SDKs are tested against
internal/            tooling for this repository, such as the check on the fixtures
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

## Security

See [SECURITY.md](SECURITY.md).

## Licence

[MIT](LICENSE)
