package webhooks

import "errors"

// The reasons a webhook is refused. Match them with errors.Is; the error
// returned wraps one of them with detail that never includes a secret or a
// signature.
var (
	// ErrMalformedSecret is a signing secret that is not "whsec_" followed
	// by the base64 of a key.
	ErrMalformedSecret = errors.New("webhooks: signing secret is malformed")
	// ErrMissingHeader is a request without one of the webhook-id,
	// webhook-timestamp and webhook-signature headers.
	ErrMissingHeader = errors.New("webhooks: a webhook header is missing")
	// ErrInvalidTimestamp is a webhook-timestamp that is not a whole
	// number of seconds.
	ErrInvalidTimestamp = errors.New("webhooks: webhook-timestamp is not a number of seconds")
	// ErrTimestampTooOld is a webhook sent longer ago than the tolerance.
	// A replayed request fails this way.
	ErrTimestampTooOld = errors.New("webhooks: webhook-timestamp is too old")
	// ErrTimestampTooNew is a webhook dated further ahead of this
	// machine's clock than the tolerance.
	ErrTimestampTooNew = errors.New("webhooks: webhook-timestamp is too far in the future")
	// ErrInvalidSignature is a request none of whose signatures was made
	// with a secret the Verifier holds, over the body it was given.
	ErrInvalidSignature = errors.New("webhooks: no signature matches")
	// ErrInvalidEvent is a body that is not an event: not a JSON object
	// with an id, a type, a time and a data object.
	ErrInvalidEvent = errors.New("webhooks: body is not an event")
)
