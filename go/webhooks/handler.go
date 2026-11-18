package webhooks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// maxBodyBytes is far above any event Cimento sends.
const maxBodyBytes = 1 << 20

type handler struct {
	webhook  *Webhook
	handle   func(ctx context.Context, event *Event) error
	rejected func(r *http.Request, err error)
}

// A HandlerOption changes how a Handler behaves.
type HandlerOption func(h *handler)

// OnReject has the Handler call rejected with the reason each time it refuses
// a request: one of this package's sentinel errors, or the error from reading
// the body. The response never says why, so this is how to find out.
func OnReject(rejected func(r *http.Request, err error)) HandlerOption {
	return func(h *handler) { h.rejected = rejected }
}

// Handler returns an http.Handler that verifies each request, reads it as an
// event and calls handle with it. It panics if wh or handle is nil.
//
// It answers 204 when handle returns nil and 500 when it returns an error,
// which has Cimento send the event again later. Cimento waits 10 seconds for
// the answer, so handle should hand long work to something else. It may be
// called more than once with the same event; de-duplicate on Event.ID.
//
// A request that is not a webhook from Cimento never reaches handle. It is
// answered 401 for a signature that does not match or a timestamp outside the
// tolerance, 400 for a missing header or a body that is not an event, 405 for
// a method other than POST, and 413 for a body over 1 MiB.
func Handler(wh *Webhook, handle func(ctx context.Context, event *Event) error, options ...HandlerOption) http.Handler {
	if wh == nil || handle == nil {
		panic("webhooks: Handler needs a Webhook and a function to handle events")
	}
	h := &handler{webhook: wh, handle: handle, rejected: func(*http.Request, error) {}}
	for _, option := range options {
		option(h)
	}
	return h
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.rejected(r, fmt.Errorf("%w, not %s", ErrMethodNotAllowed, r.Method))
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		h.rejected(r, err)
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	event, err := h.webhook.Verify(body, r.Header)
	switch {
	case err == nil:
	case errors.Is(err, ErrInvalidSignature), errors.Is(err, ErrTimestampTooOld), errors.Is(err, ErrTimestampTooNew):
		h.rejected(r, err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	default:
		h.rejected(r, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := h.handle(r.Context(), event); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
