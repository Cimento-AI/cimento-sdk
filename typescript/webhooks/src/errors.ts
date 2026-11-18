/**
 * Why a webhook was refused. The first seven are the reasons `verify` and
 * `parseEvent` give. The last three come only from `fetchHandler`, for a
 * request it refused before verifying it.
 */
export type WebhookVerificationErrorCode =
  | "malformed_secret"
  | "missing_header"
  | "invalid_timestamp"
  | "timestamp_too_old"
  | "timestamp_too_new"
  | "invalid_signature"
  | "invalid_event"
  | "method_not_allowed"
  | "body_too_large"
  | "body_unreadable";

/**
 * The error for a webhook that cannot be trusted or is not an event. `code`
 * says which. The message never includes a secret or a signature.
 */
export class WebhookVerificationError extends Error {
  readonly code: WebhookVerificationErrorCode;

  constructor(code: WebhookVerificationErrorCode, message: string) {
    super(message);
    this.name = "WebhookVerificationError";
    this.code = code;
  }
}
