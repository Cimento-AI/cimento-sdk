import { describe, expect, test } from "vitest";

import { Webhook, WebhookVerificationError } from "../src/index";
import {
  codeOf,
  delivery,
  reference,
  signatureCases,
  webhookFor,
} from "./fixtures";

const outcomes = [
  "ok",
  "missing_header",
  "invalid_timestamp",
  "timestamp_too_old",
  "timestamp_too_new",
  "invalid_signature",
  "malformed_secret",
];

describe("verifySignature gives what every fixture expects", () => {
  test.each(signatureCases)("$name", async (c) => {
    expect(outcomes).toContain(c.expect);
    const plain = () => webhookFor(c).verifySignature(c.body, c.headers);
    expect(await codeOf(plain)).toBe(c.expect);

    // The same request as a Headers and with the body as bytes.
    const bytes = new TextEncoder().encode(c.body);
    const fetched = () =>
      webhookFor(c).verifySignature(bytes, new Headers(c.headers));
    expect(await codeOf(fetched)).toBe(c.expect);
    expect(
      await codeOf(() =>
        webhookFor(c).verifySignature(bytes.buffer, c.headers),
      ),
    ).toBe(c.expect);
  });

  test("every outcome has a fixture", () => {
    const expected = new Set(signatureCases.map((c) => c.expect));
    expect([...expected].sort()).toEqual([...outcomes].sort());
  });
});

test("an error never repeats a secret or a signature", async () => {
  for (const c of signatureCases) {
    let message = "";
    try {
      await webhookFor(c).verifySignature(c.body, c.headers);
    } catch (error) {
      message = (error as Error).message;
    }
    for (const secret of c.secrets) {
      if (secret.length > "whsec_".length) {
        expect(message).not.toContain(secret.slice("whsec_".length));
      }
    }
    const signature = c.headers["webhook-signature"];
    if (signature) {
      expect(message).not.toContain(signature);
    }
  }
});

test("verify reads the event of a delivery it trusts", async () => {
  const deliveries = signatureCases.filter(
    (c) => c.expect === "ok" && c.name.includes("delivery"),
  );
  expect(deliveries.length).toBeGreaterThanOrEqual(2);
  for (const c of deliveries) {
    const event = await webhookFor(c).verify(c.body, c.headers);
    expect(event.id).toBe(c.headers["webhook-id"]);
    expect(event.known).toBe(true);
  }
});

test("verify does not read a body it does not trust", async () => {
  for (const c of signatureCases) {
    if (c.expect === "ok" || c.expect === "malformed_secret") {
      continue;
    }
    expect(await codeOf(() => webhookFor(c).verify(c.body, c.headers))).toBe(
      c.expect,
    );
  }
});

test("verify refuses a trusted body that is not an event", async () => {
  const webhook = webhookFor(reference);
  await webhook.verifySignature(reference.body, reference.headers);
  expect(
    await codeOf(() => webhook.verify(reference.body, reference.headers)),
  ).toBe("invalid_event");
});

test("the defaults are five minutes and the machine's clock", async () => {
  // The fixture was signed long before this test runs.
  const webhook = new Webhook(delivery.secrets[0]!);
  expect(
    await codeOf(() =>
      webhook.verifySignature(delivery.body, delivery.headers),
    ),
  ).toBe("timestamp_too_old");

  const at = (seconds: number, toleranceSeconds?: number) =>
    codeOf(() =>
      new Webhook(delivery.secrets[0]!, {
        toleranceSeconds,
        now: () => new Date((delivery.now + seconds) * 1000),
      }).verifySignature(delivery.body, delivery.headers),
    );
  const sent = Number(delivery.headers["webhook-timestamp"]) - delivery.now;
  expect(await at(sent + 300)).toBe("ok");
  expect(await at(sent + 301)).toBe("timestamp_too_old");
  expect(await at(sent, -3600)).toBe("ok");
  expect(await at(sent + 1, -3600)).toBe("timestamp_too_old");
});

test("a tolerance or a clock that is not a number cannot turn the check off", async () => {
  for (const toleranceSeconds of [NaN, Infinity, -Infinity, "300"]) {
    expect(
      () =>
        new Webhook(delivery.secrets[0]!, {
          toleranceSeconds: toleranceSeconds as number,
        }),
    ).toThrowError(RangeError);
  }
  const stopped = new Webhook(delivery.secrets[0]!, {
    now: () => new Date(NaN),
  });
  await expect(
    stopped.verifySignature(delivery.body, delivery.headers),
  ).rejects.toThrow(RangeError);
});

test("a secret that is not a string is malformed", () => {
  for (const secret of [undefined, null, 42, {}]) {
    expect(() => new Webhook(secret as unknown as string)).toThrowError(
      WebhookVerificationError,
    );
  }
});

test("header values in a list, as Node gives repeated headers", async () => {
  const headers = Object.fromEntries(
    Object.entries(delivery.headers).map(([name, value]) => [name, [value]]),
  );
  await webhookFor(delivery).verifySignature(delivery.body, headers);
  expect(
    await codeOf(() =>
      webhookFor(delivery).verifySignature(delivery.body, {
        ...delivery.headers,
        "webhook-id": undefined,
      }),
    ),
  ).toBe("missing_header");
});

describe("sign", () => {
  const passing = signatureCases.filter((c) => c.expect === "ok");

  test.each(passing)("gives the signature of: $name", async (c) => {
    const lower = Object.fromEntries(
      Object.entries(c.headers).map(([name, value]) => [
        name.toLowerCase(),
        value,
      ]),
    );
    const sent = lower["webhook-signature"]!.split(/\s+/);
    const timestamp = new Date(Number(lower["webhook-timestamp"]) * 1000);
    const signed = await Promise.all(
      c.secrets.map((secret) =>
        new Webhook(secret).sign(lower["webhook-id"]!, timestamp, c.body),
      ),
    );
    expect(signed.some((signature) => sent.includes(signature))).toBe(true);
  });

  test("what it returns verifies, and another secret signs differently", async () => {
    const now = new Date("2027-01-02T03:04:05.999Z");
    const other = `whsec_${"QUJD".repeat(8)}`;
    const body = JSON.stringify({
      id: "evt",
      type: "webhook.test",
      occurredAt: "2027-01-02T03:04:05Z",
      data: { endpointId: "e" },
    });
    const signer = new Webhook(delivery.secrets[0]!, {
      additionalSecrets: [other],
    });
    const signature = await signer.sign("evt", now, body);
    expect(signature).toMatch(/^v1,[A-Za-z0-9+/]+=*$/);

    const receiver = new Webhook(delivery.secrets[0]!, { now: () => now });
    const event = await receiver.verify(body, {
      "webhook-id": "evt",
      "webhook-timestamp": String(Math.floor(now.getTime() / 1000)),
      "webhook-signature": signature,
    });
    expect(event.type).toBe("webhook.test");
    expect(await new Webhook(other).sign("evt", now, body)).not.toBe(signature);
  });
});
