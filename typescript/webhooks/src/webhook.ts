import { bytesOf, decodeBase64, encodeBase64, type Body } from "./encoding";
import { WebhookVerificationError } from "./errors";
import { parseEvent, type WebhookEvent } from "./event";

/** How far a webhook's timestamp may be from the clock, in seconds. */
export const DEFAULT_TOLERANCE_SECONDS = 300;

type Key = Awaited<ReturnType<typeof crypto.subtle.importKey>>;

const secretPrefix = "whsec_";
const signatureVersion = "v1";

export interface WebhookOptions {
  /**
   * More secrets to accept a webhook signed with. After rotating an
   * endpoint's secret, Cimento signs with the old and the new one for 24
   * hours; pass the old one here while your deployment moves over.
   */
  additionalSecrets?: readonly string[];
  /**
   * How far a webhook's timestamp may be from the clock, in either direction.
   * `DEFAULT_TOLERANCE_SECONDS` when absent. Counted in whole seconds. It
   * must be a finite number: there is no way to turn the check off.
   */
  toleranceSeconds?: number;
  /** The clock timestamps are checked against. For tests. */
  now?: () => Date;
}

/**
 * A request's headers: a `Headers`, or an object of them such as Node's
 * `request.headers`. Names are matched without regard to case.
 */
export type WebhookHeaders =
  | { get(name: string): string | null }
  | Readonly<Record<string, string | readonly string[] | undefined>>;

function header(headers: WebhookHeaders, name: string): string {
  if (typeof headers.get === "function") {
    return (headers as { get(name: string): string | null }).get(name) ?? "";
  }
  for (const [key, value] of Object.entries(headers)) {
    if (key.toLowerCase() === name) {
      return (typeof value === "string" ? value : value?.[0]) ?? "";
    }
  }
  return "";
}

/**
 * Verifies requests sent by Cimento to one endpoint, with the endpoint's
 * signing secret, and reads them as events.
 */
export class Webhook {
  readonly #secrets: Uint8Array<ArrayBuffer>[];
  readonly #tolerance: number;
  readonly #now: () => Date;
  #keys: Promise<Key[]> | undefined;

  /**
   * `secret` is the `whsec_...` value Cimento shows once when the endpoint
   * is created or its secret is rotated. Throws a `WebhookVerificationError`
   * with the code `malformed_secret` for a secret that is not one, and a
   * `RangeError` for a tolerance that is not a finite number.
   */
  constructor(secret: string, options: WebhookOptions = {}) {
    const secrets = [secret, ...(options.additionalSecrets ?? [])];
    this.#secrets = secrets.map((given, i) => {
      const key =
        typeof given === "string" && given.startsWith(secretPrefix)
          ? decodeBase64(given.slice(secretPrefix.length))
          : null;
      if (key === null || key.length === 0) {
        throw new WebhookVerificationError(
          "malformed_secret",
          `signing secret is malformed: secret ${i + 1} of ${secrets.length} is not "${secretPrefix}" followed by base64`,
        );
      }
      return key;
    });
    const tolerance = options.toleranceSeconds ?? DEFAULT_TOLERANCE_SECONDS;
    // NaN or Infinity would make both timestamp checks pass for any
    // timestamp, and a captured request could then be replayed for ever.
    if (typeof tolerance !== "number" || !Number.isFinite(tolerance)) {
      throw new RangeError("toleranceSeconds must be a finite number");
    }
    this.#tolerance = Math.max(0, Math.trunc(tolerance));
    this.#now = options.now ?? (() => new Date());
  }

  #importKeys(): Promise<Key[]> {
    this.#keys ??= Promise.all(
      this.#secrets.map((secret) =>
        crypto.subtle.importKey(
          "raw",
          secret,
          { name: "HMAC", hash: "SHA-256" },
          false,
          ["sign", "verify"],
        ),
      ),
    );
    return this.#keys;
  }

  /**
   * Checks that `body` was sent by Cimento and reads it as an event. Pass the
   * request's body exactly as it arrived: a body that was parsed and written
   * out again no longer matches its signature.
   *
   * Rejects with a `WebhookVerificationError` when the request cannot be
   * trusted or the body is not an event.
   */
  async verify(body: Body, headers: WebhookHeaders): Promise<WebhookEvent> {
    await this.verifySignature(body, headers);
    return parseEvent(body);
  }

  /**
   * Checks that `body` was sent by Cimento, without reading it. Most
   * receivers want `verify`.
   */
  async verifySignature(body: Body, headers: WebhookHeaders): Promise<void> {
    const id = header(headers, "webhook-id");
    const timestamp = header(headers, "webhook-timestamp");
    const signatures = header(headers, "webhook-signature");
    for (const [name, value] of [
      ["webhook-id", id],
      ["webhook-timestamp", timestamp],
      ["webhook-signature", signatures],
    ] as const) {
      if (value === "") {
        throw new WebhookVerificationError(
          "missing_header",
          `a webhook header is missing: ${name}`,
        );
      }
    }

    const sentAt = /^[+-]?\d+$/.test(timestamp) ? Number(timestamp) : NaN;
    if (!Number.isSafeInteger(sentAt)) {
      throw new WebhookVerificationError(
        "invalid_timestamp",
        "webhook-timestamp is not a number of seconds",
      );
    }
    const now = Math.floor(this.#now().getTime() / 1000);
    if (!Number.isFinite(now)) {
      throw new RangeError("the clock given as `now` returned an invalid date");
    }
    if (now - sentAt > this.#tolerance) {
      throw new WebhookVerificationError(
        "timestamp_too_old",
        `webhook-timestamp is too old: sent ${now - sentAt} seconds ago, and the tolerance is ${this.#tolerance}`,
      );
    }
    if (sentAt - now > this.#tolerance) {
      throw new WebhookVerificationError(
        "timestamp_too_new",
        `webhook-timestamp is too far in the future: dated ${sentAt - now} seconds ahead, and the tolerance is ${this.#tolerance}`,
      );
    }

    const signed = signedContent(id, timestamp, body);
    const keys = await this.#importKeys();
    // A header holds one signature for each secret Cimento signed with, and
    // may hold signatures of versions this package does not know.
    let matched = false;
    for (const signature of signatures.split(/\s+/)) {
      const comma = signature.indexOf(",");
      const given =
        signature.slice(0, Math.max(comma, 0)) === signatureVersion
          ? decodeBase64(signature.slice(comma + 1))
          : null;
      if (given === null) {
        continue;
      }
      for (const key of keys) {
        // verify compares in constant time.
        if (await crypto.subtle.verify("HMAC", key, given, signed)) {
          matched = true;
        }
      }
    }
    if (!matched) {
      throw new WebhookVerificationError(
        "invalid_signature",
        "no signature matches",
      );
    }
  }

  /**
   * Returns the signature Cimento would send in the `webhook-signature`
   * header for a request with this id, this time in `webhook-timestamp` and
   * this body: `v1,` followed by base64. It signs with the first secret.
   *
   * It is for testing a receiver: sign a body with a secret of your own and
   * send it to your handler.
   */
  async sign(id: string, timestamp: Date, body: Body): Promise<string> {
    const seconds = Math.floor(timestamp.getTime() / 1000);
    const [key] = await this.#importKeys();
    const signature = await crypto.subtle.sign(
      "HMAC",
      key!,
      signedContent(id, String(seconds), body),
    );
    return `${signatureVersion},${encodeBase64(new Uint8Array(signature))}`;
  }
}

function signedContent(
  id: string,
  timestamp: string,
  body: Body,
): Uint8Array<ArrayBuffer> {
  const prefix = new TextEncoder().encode(`${id}.${timestamp}.`);
  const bytes = bytesOf(body);
  const signed = new Uint8Array(prefix.length + bytes.length);
  signed.set(prefix);
  signed.set(bytes, prefix.length);
  return signed;
}
