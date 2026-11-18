/**
 * Verify webhooks sent by Cimento and read them as typed events.
 *
 * @packageDocumentation
 */
export {
  WebhookVerificationError,
  type WebhookVerificationErrorCode,
} from "./errors";
export {
  parseEvent,
  type UnknownWebhookEvent,
  type WebhookEvent,
} from "./event";
export { eventTypes } from "./events.gen";
export type * from "./events.gen";
export { fetchHandler, type FetchHandlerOptions } from "./handler";
export {
  DEFAULT_TOLERANCE_SECONDS,
  Webhook,
  type WebhookHeaders,
  type WebhookOptions,
} from "./webhook";
export type { Body as WebhookBody } from "./encoding";
