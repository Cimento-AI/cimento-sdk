// The TypeScript half of the check command in internal/check, which runs
// files of webhooks through both SDKs and compares what each makes of them.
//
//   node scripts/check.mjs events <dir>        every file is an event body
//   node scripts/check.mjs deliveries <dir>    recorded requests
//
// It prints one result per file as JSON, in the shape the Go half compares
// with its own. The package is run as it is built.
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

import {
  parseEvent,
  Webhook,
  WebhookVerificationError,
} from "../dist/index.js";

const [mode, dir] = process.argv.slice(2);
if ((mode !== "events" && mode !== "deliveries") || !dir) {
  console.error("usage: check.mjs events|deliveries <dir>");
  process.exit(2);
}

const secrets = (process.env.CIMENTO_WEBHOOK_SECRETS ?? "")
  .split(",")
  .map((secret) => secret.trim())
  .filter((secret) => secret !== "");

/** Checks a recorded request as a receiver would have when it arrived. */
async function verify(name, body) {
  const described = join(dir, name.replace(/\.body$/, ".json"));
  const { headers, receivedAt } = JSON.parse(readFileSync(described, "utf8"));
  const [secret, ...additionalSecrets] = secrets;
  const webhook = new Webhook(secret, {
    additionalSecrets,
    now: () => new Date(receivedAt),
  });
  return webhook.verify(body, headers);
}

const names = readdirSync(dir)
  .filter((name) => statSync(join(dir, name)).isFile())
  .filter((name) => mode === "events" || name.endsWith(".body"))
  .sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));

const results = [];
for (const name of names) {
  const body = readFileSync(join(dir, name));
  const result = { file: name, error: "", id: "", type: "", typed: false };
  try {
    const event =
      mode === "events" ? parseEvent(body) : await verify(name, body);
    result.id = event.id;
    result.type = event.type;
    result.typed = event.known;
  } catch (error) {
    result.error =
      error instanceof WebhookVerificationError
        ? error.code
        : `error: ${error.message}`;
  }
  results.push(result);
}
console.log(JSON.stringify(results));
