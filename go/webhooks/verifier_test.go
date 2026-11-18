package webhooks

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// outcomes maps what a signature fixture expects to the error that says it.
var outcomes = map[string]error{
	"ok":                nil,
	"missing_header":    ErrMissingHeader,
	"invalid_timestamp": ErrInvalidTimestamp,
	"timestamp_too_old": ErrTimestampTooOld,
	"timestamp_too_new": ErrTimestampTooNew,
	"invalid_signature": ErrInvalidSignature,
	"malformed_secret":  ErrMalformedSecret,
}

// verifierFor is the Verifier a fixture describes, at the time it describes.
func verifierFor(c signatureCase) (*Verifier, error) {
	options := []Option{WithClock(func() time.Time { return time.Unix(c.Now, 0) })}
	if c.ToleranceSeconds != nil {
		options = append(options, WithTolerance(time.Duration(*c.ToleranceSeconds)*time.Second))
	}
	if len(c.Secrets) == 0 {
		return NewVerifier("", options...)
	}
	return NewVerifier(c.Secrets[0], append(options, WithAdditionalSecrets(c.Secrets[1:]...))...)
}

func headerOf(c signatureCase) http.Header {
	header := http.Header{}
	for name, value := range c.Headers {
		header.Set(name, value)
	}
	return header
}

func TestVerifySignatureGivesWhatEveryFixtureExpects(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}
	for _, c := range signatureCases(t) {
		want, known := outcomes[c.Expect]
		if !known {
			t.Fatalf("%s: this package has no error for %q", c.Name, c.Expect)
		}
		seen[c.Expect] = true

		verifier, err := verifierFor(c)
		if err == nil {
			err = verifier.VerifySignature([]byte(c.Body), headerOf(c))
		}
		if want == nil && err != nil {
			t.Errorf("%s: %v", c.Name, err)
		}
		if want != nil && !errors.Is(err, want) {
			t.Errorf("%s: got %v, want %v", c.Name, err, want)
		}
		for outcome, other := range outcomes {
			if other != nil && other != want && errors.Is(err, other) {
				t.Errorf("%s: the error is also %s", c.Name, outcome)
			}
		}
		if err != nil {
			for _, secret := range c.Secrets {
				if len(secret) > len(secretPrefix) && strings.Contains(err.Error(), secret[len(secretPrefix):]) {
					t.Errorf("%s: the error repeats a secret", c.Name)
				}
			}
			if signature := c.Headers[headerSignature]; signature != "" && strings.Contains(err.Error(), signature) {
				t.Errorf("%s: the error repeats the signature", c.Name)
			}
		}
	}
	for outcome := range outcomes {
		if !seen[outcome] {
			t.Errorf("no fixture expects %s", outcome)
		}
	}
}

func TestVerifyReadsTheEventOfADeliveryItTrusts(t *testing.T) {
	t.Parallel()

	read := 0
	for _, c := range signatureCases(t) {
		if c.Expect != "ok" || !strings.Contains(c.Name, "delivery") {
			continue
		}
		verifier, err := verifierFor(c)
		if err != nil {
			t.Fatal(err)
		}
		event, err := verifier.Verify([]byte(c.Body), headerOf(c))
		if err != nil {
			t.Errorf("%s: %v", c.Name, err)
			continue
		}
		if event.ID != c.Headers[headerID] || event.Data == nil {
			t.Errorf("%s: read as %+v", c.Name, event)
		}
		read++
	}
	if read < 2 {
		t.Fatalf("only %d fixtures hold a delivery", read)
	}
}

func TestVerifyDoesNotReadABodyItDoesNotTrust(t *testing.T) {
	t.Parallel()

	for _, c := range signatureCases(t) {
		if c.Expect == "ok" || c.Expect == "malformed_secret" {
			continue
		}
		verifier, err := verifierFor(c)
		if err != nil {
			t.Fatal(err)
		}
		if event, err := verifier.Verify([]byte(c.Body), headerOf(c)); event != nil || !errors.Is(err, outcomes[c.Expect]) {
			t.Errorf("%s: got %+v, %v", c.Name, event, err)
		}
	}
}

func TestVerifyRefusesATrustedBodyThatIsNotAnEvent(t *testing.T) {
	t.Parallel()

	reference := signatureCases(t)[0]
	verifier, err := verifierFor(reference)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.VerifySignature([]byte(reference.Body), headerOf(reference)); err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify([]byte(reference.Body), headerOf(reference)); !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("got %v", err)
	}
}

func TestTheDefaultsAreFiveMinutesAndTheMachinesClock(t *testing.T) {
	t.Parallel()

	delivery := signatureCases(t)[1]
	verifier, err := NewVerifier(delivery.Secrets[0])
	if err != nil {
		t.Fatal(err)
	}
	if verifier.tolerance != 300 {
		t.Errorf("the tolerance is %d seconds", verifier.tolerance)
	}
	// The fixture was signed long before this test runs.
	if err := verifier.VerifySignature([]byte(delivery.Body), headerOf(delivery)); !errors.Is(err, ErrTimestampTooOld) {
		t.Errorf("got %v", err)
	}

	negative, err := NewVerifier(delivery.Secrets[0], WithTolerance(-time.Hour), WithClock(func() time.Time { return time.Unix(delivery.Now, 0) }))
	if err != nil {
		t.Fatal(err)
	}
	if err := negative.VerifySignature([]byte(delivery.Body), headerOf(delivery)); !errors.Is(err, ErrTimestampTooOld) {
		t.Errorf("a negative tolerance: got %v", err)
	}
}
