// Loads the package as it is built, both ways it can be loaded, and has each
// verify a delivery from the shared fixtures. The tests run the source; this
// runs what would be published.
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";

const { cases } = JSON.parse(
  readFileSync(
    new URL("../../../fixtures/webhooks/signatures.json", import.meta.url),
    "utf8",
  ),
);
const delivery = cases[1];

const builds = {
  "dist/index.js": await import("../dist/index.js"),
  "dist/index.cjs": createRequire(import.meta.url)("../dist/index.cjs"),
};
for (const [
  file,
  { Webhook, WebhookVerificationError, eventTypes },
] of Object.entries(builds)) {
  const webhook = new Webhook(delivery.secrets[0], {
    now: () => new Date(delivery.now * 1000),
  });
  const event = await webhook.verify(delivery.body, delivery.headers);
  if (!event.known || !eventTypes.includes(event.type)) {
    throw new Error(`${file}: the delivery was not read as a known event`);
  }
  const refused = await webhook
    .verify(delivery.body + " ", delivery.headers)
    .catch((error) => error);
  if (
    !(refused instanceof WebhookVerificationError) ||
    refused.code !== "invalid_signature"
  ) {
    throw new Error(`${file}: a changed body was not refused`);
  }
  console.log(`${file}: verified ${event.type}`);
}
