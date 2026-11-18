import {
  WebhookVerificationError,
  type WebhookVerificationErrorCode,
} from "./errors";
import type { WebhookEvent } from "./event";
import type { Webhook } from "./webhook";

/** Far above any event Cimento sends. */
const maxBodyBytes = 1 << 20;

export interface FetchHandlerOptions {
  /**
   * Called with the reason each time the handler refuses a request. The
   * response never says why, so this is how to find out.
   */
  onReject?: (request: Request, error: WebhookVerificationError) => void;
}

const statuses: Partial<Record<WebhookVerificationErrorCode, number>> = {
  invalid_signature: 401,
  timestamp_too_old: 401,
  timestamp_too_new: 401,
  method_not_allowed: 405,
  body_too_large: 413,
};

/** Reads a request's body, giving up once it is over the limit. */
async function read(request: Request): Promise<Uint8Array> {
  const chunks: Uint8Array[] = [];
  let length = 0;
  if (request.body !== null) {
    try {
      // getReader throws for a body something has already read.
      const reader = request.body.getReader();
      for (;;) {
        const { done, value } = await reader.read();
        if (done) {
          break;
        }
        length += value.length;
        if (length > maxBodyBytes) {
          await reader.cancel();
          throw new WebhookVerificationError(
            "body_too_large",
            `the body is over ${maxBodyBytes} bytes`,
          );
        }
        chunks.push(value);
      }
    } catch (error) {
      if (error instanceof WebhookVerificationError) {
        throw error;
      }
      throw new WebhookVerificationError(
        "body_unreadable",
        "the body could not be read",
      );
    }
  }
  const body = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.length;
  }
  return body;
}

/**
 * Returns a function from a `Request` to a `Response` that verifies the
 * request, reads it as an event and calls `handle` with it. It suits anything
 * that speaks the Fetch API: Cloudflare Workers, Deno, Bun, Hono, a Next.js
 * route handler.
 *
 * It answers 204 when `handle` returns and 500 when it throws, which has
 * Cimento send the event again later. Cimento waits 10 seconds for the answer,
 * so `handle` should hand long work to something else. It may be called more
 * than once with the same event; de-duplicate on `event.id`.
 *
 * A request that is not a webhook from Cimento never reaches `handle`. It is
 * answered 401 for a signature that does not match or a timestamp outside the
 * tolerance, 400 for a missing header or a body that is not an event, 405 for
 * a method other than POST, and 413 for a body over 1 MiB.
 */
export function fetchHandler(
  webhook: Webhook,
  handle: (event: WebhookEvent, request: Request) => void | Promise<void>,
  options: FetchHandlerOptions = {},
): (request: Request) => Promise<Response> {
  return async (request) => {
    let event: WebhookEvent;
    try {
      if (request.method !== "POST") {
        throw new WebhookVerificationError(
          "method_not_allowed",
          `a webhook is a POST request, not ${request.method}`,
        );
      }
      event = await webhook.verify(await read(request), request.headers);
    } catch (error) {
      if (!(error instanceof WebhookVerificationError)) {
        throw error;
      }
      options.onReject?.(request, error);
      const status = statuses[error.code] ?? 400;
      return new Response(null, {
        status,
        headers: status === 405 ? { Allow: "POST" } : undefined,
      });
    }

    try {
      await handle(event, request);
    } catch {
      return new Response(null, { status: 500 });
    }
    return new Response(null, { status: 204 });
  };
}
