import { readFileSync } from "node:fs";
import type { AddressInfo } from "node:net";

import { afterAll, beforeAll, expect, test, vi } from "vitest";

import { Webhook, type WebhookEvent } from "../src/index";
import { delivery } from "./fixtures";

// The examples are the README's code. They import the package by name, as a
// reader's code would; vitest.config.ts points that name at the source.

const secret = "whsec_dGVzdGluZy1vbmx5LW5vdC1hLXJlYWwtc2VjcmV0";
beforeAll(() => vi.stubEnv("CIMENTO_WEBHOOK_SECRET", secret));
afterAll(() => vi.unstubAllEnvs());

const event = {
  id: "c7a1e2b3-4d5f-4a6b-8c9d-0e1f2a3b4c5d",
  type: "training.assignment.completed",
  occurredAt: "2026-11-12T15:02:11.902Z",
  data: {
    assignmentId: "8f3b2a1c-6d5e-4f70-9a8b-7c6d5e4f3a2b",
    campaignId: "2e1d0c9b-8a7f-4e6d-b5c4-a3b2c1d0e9f8",
    user: {
      id: "7a6b5c4d-3e2f-4a1b-8c9d-0e1f2a3b4c5d",
      email: "ada.lovelace@example.com",
    },
    completedAt: "2026-11-12T15:02:11.871Z",
    completedBy: "learner",
  },
};

test("the fetch example handles a signed event and refuses a changed one", async () => {
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
  const { receive } = await import("../examples/fetch");
  const { deliver } = await import("../examples/testing");

  const accepted = await deliver(receive, new Webhook(secret), event);
  expect(accepted.status).toBe(204);
  expect(log).toHaveBeenCalledWith(
    "7a6b5c4d-3e2f-4a1b-8c9d-0e1f2a3b4c5d finished training, recorded by the learner",
  );

  const other = new Webhook(`whsec_${"QUJD".repeat(8)}`);
  const refused = await deliver(receive, other, event);
  expect(refused.status).toBe(401);
  expect(warn).toHaveBeenCalledWith("refused a webhook: invalid_signature");
  log.mockRestore();
  warn.mockRestore();
});

test("the Node example verifies the raw body of a request", async () => {
  const log = vi.spyOn(console, "log").mockImplementation(() => {});
  const { server } = await import("../examples/node");
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address() as AddressInfo;
  const url = `http://127.0.0.1:${port}/webhooks/cimento`;
  try {
    const webhook = new Webhook(secret);
    const body = JSON.stringify(event);
    const now = new Date();
    const headers = {
      "webhook-id": event.id,
      "webhook-timestamp": String(Math.floor(now.getTime() / 1000)),
      "webhook-signature": await webhook.sign(event.id, now, body),
    };
    const accepted = await fetch(url, { method: "POST", headers, body });
    expect(accepted.status).toBe(204);
    expect(log).toHaveBeenCalledWith(event.id, event.type, event.occurredAt);

    const changed = body.replace("learner", "admin");
    const refused = await fetch(url, {
      method: "POST",
      headers,
      body: changed,
    });
    expect(refused.status).toBe(401);
    const bare = await fetch(url, { method: "POST", body });
    expect(bare.status).toBe(400);
  } finally {
    await new Promise((resolve) => server.close(resolve));
    log.mockRestore();
  }
});

test("the README shows the examples as they are", () => {
  const readme = readFileSync(new URL("../README.md", import.meta.url), "utf8");
  for (const name of ["fetch", "node", "testing"]) {
    const source = readFileSync(
      new URL(`../examples/${name}.ts`, import.meta.url),
      "utf8",
    );
    const shown = /\/\/ #region readme\n([\s\S]*?)\/\/ #endregion readme/.exec(
      source,
    );
    expect(shown, name).not.toBeNull();
    expect(readme, `examples/${name}.ts`).toContain(shown![1]!.trim());
  }
});

test("a delivery from the fixtures is an event the examples' types accept", () => {
  const parsed: WebhookEvent = JSON.parse(delivery.body) as WebhookEvent;
  expect(parsed.type).toBe("agent_hub.finding.created");
});
