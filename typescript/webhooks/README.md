# @cimento/webhooks

Verify [webhooks sent by Cimento](https://docs.cimento.ai/webhooks/overview) and read them as typed events.

> [!NOTE]
> This package is not released yet. Its API may still change.

```sh
npm install @cimento/webhooks
```

It needs Node 22 or newer, or any runtime with Web Crypto: Cloudflare Workers, Deno, Bun. It has no dependencies.

## Receive webhooks

Make a `Webhook` from your endpoint's signing secret, the `whsec_...` value Cimento shows once when you create the endpoint. If your server speaks the Fetch API (Workers, Deno, Bun, Hono, a Next.js route handler), `fetchHandler` does the rest:

```ts
import { fetchHandler, Webhook } from "@cimento/webhooks";

const webhook = new Webhook(process.env.CIMENTO_WEBHOOK_SECRET!);

export const receive = fetchHandler(
  webhook,
  async (event) => {
    if (!event.known) {
      // A type added to Cimento since this version. Answering 2xx is right.
      return;
    }
    switch (event.type) {
      case "training.assignment.completed":
        console.log(
          `${event.data.user.id} finished training, recorded by the ${event.data.completedBy}`,
        );
        break;
      case "phishing.simulation.clicked":
        console.log(`${event.data.user.id} clicked a simulation`);
        break;
    }
  },
  {
    onReject: (request, error) =>
      console.warn(`refused a webhook: ${error.code}`),
  },
);
```

`receive` takes a `Request` and answers Cimento for you:

| Answer   | When                                                                                      |
| -------- | ----------------------------------------------------------------------------------------- |
| 204      | Your function returned.                                                                   |
| 500      | Your function threw. Cimento sends the event again later.                                 |
| 401      | The signature does not match, or the timestamp is more than five minutes from your clock. |
| 400      | A webhook header is missing, or the body is not an event.                                 |
| 405, 413 | The request is not a POST, or its body is over 1 MiB.                                     |

The response never says why a request was refused. `onReject` tells you, for every refusal in the table.

Cimento waits 10 seconds for the answer and may deliver an event more than once. Hand long work to something else, and de-duplicate on `event.id`. Events can arrive out of order; `event.occurredAt` is when each one happened.

## Events

Check `event.known` first. When it is true, `event.type` is one of the event types this version knows, and narrowing on it gives `event.data` the type of that event's data. The data has been checked: every field is there, and none is null unless its type says it can be.

Cimento adds event types and fields over time. An event of a type this version does not know is still verified and handed to you, with `known` false and its data as a plain object. So is an event whose data does not fit its type. A field this version does not know is left in the data. Text fields with a set of values today, such as `completedBy`, suggest those values and accept any string, because more may be added.

Times and ids are strings, as Cimento sent them.

## Verify a request yourself

On Express or plain Node, call `verify` with the body exactly as it arrived and the request's headers:

```ts
import { createServer } from "node:http";

import { Webhook, WebhookVerificationError } from "@cimento/webhooks";

const webhook = new Webhook(process.env.CIMENTO_WEBHOOK_SECRET!);

export const server = createServer(async (request, response) => {
  // The signature is over the body as it was sent, so collect the bytes
  // and verify them before anything parses them.
  const chunks: Buffer[] = [];
  for await (const chunk of request) {
    chunks.push(chunk as Buffer);
  }
  try {
    const event = await webhook.verify(Buffer.concat(chunks), request.headers);
    console.log(event.id, event.type, event.occurredAt);
    response.writeHead(204).end();
  } catch (error) {
    if (!(error instanceof WebhookVerificationError)) {
      throw error;
    }
    const unauthorized = ["invalid_signature", "timestamp_too_old"];
    response.writeHead(unauthorized.includes(error.code) ? 401 : 400).end();
  }
});
```

The signature is over the bytes Cimento sent. A body that was parsed and written out again will not verify; with Express, use `express.raw({ type: "application/json" })` on the route.

`verify` rejects with a `WebhookVerificationError`, whose `code` says why:

| `code`                                   | Meaning                                                                   |
| ---------------------------------------- | ------------------------------------------------------------------------- |
| `missing_header`                         | `webhook-id`, `webhook-timestamp` or `webhook-signature` is missing       |
| `invalid_timestamp`                      | `webhook-timestamp` is not a number of seconds                            |
| `timestamp_too_old`, `timestamp_too_new` | the timestamp is outside the tolerance; a replayed request fails this way |
| `invalid_signature`                      | no signature matches a secret you hold                                    |
| `invalid_event`                          | the body is not an event                                                  |

`new Webhook` throws one with the code `malformed_secret` for a secret that is not `whsec_` followed by base64. `onReject` can also be given `method_not_allowed`, `body_too_large` and `body_unreadable`.

`verify` and `sign` return promises, because they use Web Crypto. That is the one difference from the `standardwebhooks` package, whose `Webhook` this one is named after.

## Rotating a secret

After you rotate an endpoint's secret, Cimento signs with the old and the new one for 24 hours. Give the webhook both while your deployment moves over:

```ts
const webhook = new Webhook(newSecret, { additionalSecrets: [oldSecret] });
```

## Testing your handler

Sign a body with a secret of your own and send it to your handler, as Cimento would:

```ts
export async function deliver(
  receive: (request: Request) => Promise<Response>,
  webhook: Webhook,
  event: { id: string; type: string; occurredAt: string; data: object },
): Promise<Response> {
  const body = JSON.stringify(event);
  const now = new Date();
  return receive(
    new Request("https://receiver.example.com/webhooks/cimento", {
      method: "POST",
      headers: {
        "webhook-id": event.id,
        "webhook-timestamp": String(Math.floor(now.getTime() / 1000)),
        "webhook-signature": await webhook.sign(event.id, now, body),
      },
      body,
    }),
  );
}
```

`sign` uses the first secret. `parseEvent` reads a body as an event without verifying it. The `now` and `toleranceSeconds` options set the clock and the tolerance a `Webhook` uses.
