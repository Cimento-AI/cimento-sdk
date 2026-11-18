package webhooks

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

var sentinels = []error{
	ErrMalformedSecret, ErrMissingHeader, ErrInvalidTimestamp, ErrTimestampTooOld,
	ErrTimestampTooNew, ErrInvalidSignature, ErrInvalidEvent,
}

func TestVerifyNeverGivesTheHandlersOwnError(t *testing.T) {
	t.Parallel()

	if isSentinel(ErrMethodNotAllowed) {
		t.Error("ErrMethodNotAllowed is listed as an error Verify returns")
	}
}

func isSentinel(err error) bool {
	for _, sentinel := range sentinels {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}

func FuzzParse(f *testing.F) {
	for _, directory := range []string{"valid", "open", "invalid"} {
		for _, body := range eventFixtures(f, directory) {
			f.Add(body)
		}
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		event, err := Parse(body)
		if (event == nil) == (err == nil) {
			t.Fatalf("got %+v and %v", event, err)
		}
		if err != nil && !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("an error that is not ErrInvalidEvent: %v", err)
		}
		if event != nil && (event.ID == "" || event.Type == "" || len(event.RawData) == 0) {
			t.Fatalf("an event with a part missing: %+v", event)
		}
	})
}

func FuzzVerify(f *testing.F) {
	cases := signatureCases(f)
	for _, c := range cases {
		f.Add(c.Headers[headerID], c.Headers[headerTimestamp], c.Headers[headerSignature], []byte(c.Body), c.Now)
	}
	secret := cases[1].Secrets[0]

	f.Fuzz(func(t *testing.T, id string, timestamp string, signature string, body []byte, now int64) {
		verifier, err := NewVerifier(secret, WithClock(func() time.Time { return time.Unix(now, 0) }))
		if err != nil {
			t.Fatal(err)
		}
		header := http.Header{}
		header.Set(headerID, id)
		header.Set(headerTimestamp, timestamp)
		header.Set(headerSignature, signature)

		event, err := verifier.Verify(body, header)
		if (event == nil) == (err == nil) {
			t.Fatalf("got %+v and %v", event, err)
		}
		if err != nil && !isSentinel(err) {
			t.Fatalf("an error that is none of the sentinels: %v", err)
		}
	})
}
