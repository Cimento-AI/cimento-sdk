import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { Webhook, type WebhookVerificationErrorCode } from "../src/index";

const root = new URL("../../../fixtures/webhooks/", import.meta.url);

/** The event bodies in one directory of the shared fixtures, by file name. */
export function eventFixtures(directory: string): Map<string, Uint8Array> {
  const path = fileURLToPath(new URL(`events/${directory}/`, root));
  const bodies = new Map<string, Uint8Array>();
  for (const file of readdirSync(path).sort()) {
    bodies.set(file.replace(/\.json$/, ""), readFileSync(`${path}${file}`));
  }
  if (bodies.size === 0) {
    throw new Error(`no fixtures in events/${directory}`);
  }
  return bodies;
}

export interface SignatureCase {
  name: string;
  secrets: string[];
  headers: Record<string, string>;
  body: string;
  now: number;
  toleranceSeconds?: number;
  expect: "ok" | WebhookVerificationErrorCode;
}

export const signatureCases: SignatureCase[] = (
  JSON.parse(readFileSync(new URL("signatures.json", root), "utf8")) as {
    cases: SignatureCase[];
  }
).cases;

/** The Webhook a fixture describes, at the time it describes. */
export function webhookFor(c: SignatureCase): Webhook {
  return new Webhook(c.secrets[0] ?? "", {
    additionalSecrets: c.secrets.slice(1),
    toleranceSeconds: c.toleranceSeconds,
    now: () => new Date(c.now * 1000),
  });
}

/** A fixture that is a real delivery: it verifies and its body is an event. */
export const delivery = signatureCases[1]!;

/** The Standard Webhooks example: it verifies and its body is not an event. */
export const reference = signatureCases[0]!;

export async function codeOf(
  attempt: () => unknown,
): Promise<"ok" | WebhookVerificationErrorCode> {
  try {
    await attempt();
    return "ok";
  } catch (error) {
    const { WebhookVerificationError } = await import("../src/index");
    if (error instanceof WebhookVerificationError) {
      return error.code;
    }
    throw error;
  }
}
