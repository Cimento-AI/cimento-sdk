import { bytesOf, isTime, type Body } from "./encoding";
import { WebhookVerificationError } from "./errors";
import type { EventType, KnownWebhookEvent } from "./events.gen";
import { eventPayloads } from "./shapes.gen";
import { fits, isObject } from "./validate";

/**
 * An event this version of the package cannot type: its type was added to
 * Cimento since, or its data does not fit its type. It is still a verified
 * event from Cimento, and answering 2xx is right.
 */
export interface UnknownWebhookEvent {
  known: false;
  id: string;
  type: string;
  occurredAt: string;
  data: Record<string, unknown>;
}

/**
 * One thing that happened in Cimento. Check `known` first: when it is true,
 * `type` says which `data` the event has.
 *
 * `id` is the same on every attempt to deliver the event, so it is what to
 * de-duplicate on. `occurredAt` is when it happened, not when it was sent.
 */
export type WebhookEvent = KnownWebhookEvent | UnknownWebhookEvent;

function invalid(why: string): WebhookVerificationError {
  return new WebhookVerificationError(
    "invalid_event",
    `body is not an event: ${why}`,
  );
}

/**
 * Reads a body as an event without checking who sent it. It is for tests and
 * for bodies already verified; a receiver should call `Webhook.verify`.
 *
 * An event of a type this version does not know is not an error, and nor is a
 * field it does not know.
 */
export function parseEvent(body: Body): WebhookEvent {
  let parsed: unknown;
  try {
    parsed = JSON.parse(
      typeof body === "string" ? body : new TextDecoder().decode(bytesOf(body)),
    );
  } catch {
    throw invalid("it is not JSON");
  }
  if (!isObject(parsed)) {
    throw invalid("it is not an object");
  }
  const { id, type, occurredAt, data } = parsed;
  if (typeof id !== "string" || id === "") {
    throw invalid("it has no id");
  }
  if (typeof type !== "string" || type === "") {
    throw invalid("it has no type");
  }
  if (typeof occurredAt !== "string" || !isTime(occurredAt)) {
    throw invalid("its occurredAt is not a time");
  }
  if (!isObject(data)) {
    throw invalid("its data is not an object");
  }

  const payload = Object.hasOwn(eventPayloads, type)
    ? eventPayloads[type as EventType]
    : undefined;
  if (payload !== undefined && fits(data, payload)) {
    // fits has checked data against the payload of this type.
    const event = { known: true, id, type, occurredAt, data };
    return event as unknown as KnownWebhookEvent;
  }
  return { known: false, id, type, occurredAt, data };
}
