import { describe, expect, test } from "vitest";

import {
  fetchHandler,
  type WebhookEvent,
  type WebhookVerificationError,
} from "../src/index";
import {
  delivery,
  reference,
  signatureCases,
  webhookFor,
  type SignatureCase,
} from "./fixtures";

/** Sends one request to a handler for the fixture's secret and clock. */
async function serve(
  c: SignatureCase,
  init: { method?: string; body?: RequestInit["body"] } = {},
  handle: (event: WebhookEvent) => void | Promise<void> = () => {},
) {
  let handled: WebhookEvent | undefined;
  let rejected: WebhookVerificationError | undefined;
  const handler = fetchHandler(
    webhookFor(c),
    (event) => {
      handled = event;
      return handle(event);
    },
    { onReject: (_request, error) => (rejected = error) },
  );
  const method = init.method ?? "POST";
  const request = new Request("https://receiver.example.com/webhooks/cimento", {
    method,
    headers: c.headers,
    body:
      method === "GET" || method === "HEAD" ? undefined : (init.body ?? c.body),
    // Node needs this to send a stream.
    ...({ duplex: "half" } as object),
  });
  const response = await handler(request);
  return { response, handled, rejected };
}

test("answers 204 and hands over the event", async () => {
  const { response, handled, rejected } = await serve(delivery);
  expect(response.status).toBe(204);
  expect(rejected).toBeUndefined();
  expect(handled?.type).toBe("agent_hub.finding.created");
  expect(handled?.known).toBe(true);
  expect(await response.text()).toBe("");
});

test("answers 500 when the event is not handled", async () => {
  for (const handle of [
    () => {
      throw new Error("the database is down");
    },
    () => Promise.reject(new Error("the database is down")),
  ]) {
    const { response, handled, rejected } = await serve(delivery, {}, handle);
    expect(response.status).toBe(500);
    expect(handled).toBeDefined();
    expect(rejected).toBeUndefined();
    expect(await response.text()).toBe("");
  }
});

describe("refuses every fixture that does not verify", () => {
  const statuses: Record<string, number> = {
    missing_header: 400,
    invalid_timestamp: 400,
    timestamp_too_old: 401,
    timestamp_too_new: 401,
    invalid_signature: 401,
  };
  const refused = signatureCases.filter((c) => c.expect in statuses);

  test.each(refused)("$name", async (c) => {
    const { response, handled, rejected } = await serve(c);
    expect(response.status).toBe(statuses[c.expect]);
    expect(handled).toBeUndefined();
    expect(rejected?.code).toBe(c.expect);
    expect(await response.text()).toBe("");
  });
});

test("refuses a trusted body that is not an event", async () => {
  const { response, handled, rejected } = await serve(reference);
  expect(response.status).toBe(400);
  expect(handled).toBeUndefined();
  expect(rejected?.code).toBe("invalid_event");
});

test.each(["GET", "PUT", "DELETE", "HEAD"])("refuses %s", async (method) => {
  const { response, handled, rejected } = await serve(delivery, { method });
  expect(response.status).toBe(405);
  expect(response.headers.get("Allow")).toBe("POST");
  expect(handled).toBeUndefined();
  expect(rejected?.code).toBe("method_not_allowed");
  expect(rejected?.message).toContain(method);
});

test("refuses a body it cannot read whole", async () => {
  const tooLarge = await serve(delivery, {
    body: new Uint8Array((1 << 20) + 1).fill(97),
  });
  expect(tooLarge.response.status).toBe(413);
  expect(tooLarge.handled).toBeUndefined();
  expect(tooLarge.rejected?.code).toBe("body_too_large");

  const atTheLimit = await serve(delivery, {
    body: new Uint8Array(1 << 20).fill(97),
  });
  expect(atTheLimit.response.status).toBe(401);

  const broken = await serve(delivery, {
    body: new ReadableStream({
      pull(controller) {
        controller.error(new Error("connection reset"));
      },
    }),
  });
  expect(broken.response.status).toBe(400);
  expect(broken.rejected?.code).toBe("body_unreadable");

  // A body something has already read cannot be read again.
  let rejected: WebhookVerificationError | undefined;
  const handler = fetchHandler(webhookFor(delivery), () => {}, {
    onReject: (_request, error) => (rejected = error),
  });
  const used = new Request("https://receiver.example.com/", {
    method: "POST",
    headers: delivery.headers,
    body: delivery.body,
  });
  await used.text();
  expect((await handler(used)).status).toBe(400);
  expect(rejected?.code).toBe("body_unreadable");

  const empty = await serve(delivery, { body: "" });
  expect(empty.response.status).toBe(401);
  expect(empty.rejected?.code).toBe("invalid_signature");
});

test("works without onReject, and passes the request to the handler", async () => {
  let seen: string | undefined;
  const handler = fetchHandler(webhookFor(delivery), (_event, request) => {
    seen = request.url;
  });
  const refused = await handler(
    new Request("https://receiver.example.com/a", {
      method: "POST",
      body: "x",
    }),
  );
  expect(refused.status).toBe(400);
  const accepted = await handler(
    new Request("https://receiver.example.com/b", {
      method: "POST",
      headers: delivery.headers,
      body: delivery.body,
    }),
  );
  expect(accepted.status).toBe(204);
  expect(seen).toBe("https://receiver.example.com/b");
});

test("an error that is not a refusal is not swallowed", async () => {
  const broken = { verify: () => Promise.reject(new TypeError("a bug")) };
  const handler = fetchHandler(broken as never, () => {});
  await expect(
    handler(new Request("https://receiver.example.com/", { method: "POST" })),
  ).rejects.toThrow(TypeError);
});
