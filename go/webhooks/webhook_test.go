package webhooks

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
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

// webhookFor is the Webhook a fixture describes, at the time it describes.
func webhookFor(c signatureCase) (*Webhook, error) {
	options := []Option{WithClock(func() time.Time { return time.Unix(c.Now, 0) })}
	if c.ToleranceSeconds != nil {
		options = append(options, WithTolerance(time.Duration(*c.ToleranceSeconds)*time.Second))
	}
	if len(c.Secrets) == 0 {
		return NewWebhook("", options...)
	}
	return NewWebhook(c.Secrets[0], append(options, WithAdditionalSecrets(c.Secrets[1:]...))...)
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

		wh, err := webhookFor(c)
		if err == nil {
			err = wh.VerifySignature([]byte(c.Body), headerOf(c))
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
		wh, err := webhookFor(c)
		if err != nil {
			t.Fatal(err)
		}
		event, err := wh.Verify([]byte(c.Body), headerOf(c))
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
		wh, err := webhookFor(c)
		if err != nil {
			t.Fatal(err)
		}
		if event, err := wh.Verify([]byte(c.Body), headerOf(c)); event != nil || !errors.Is(err, outcomes[c.Expect]) {
			t.Errorf("%s: got %+v, %v", c.Name, event, err)
		}
	}
}

func TestVerifyRefusesATrustedBodyThatIsNotAnEvent(t *testing.T) {
	t.Parallel()

	reference := signatureCases(t)[0]
	wh, err := webhookFor(reference)
	if err != nil {
		t.Fatal(err)
	}
	if err := wh.VerifySignature([]byte(reference.Body), headerOf(reference)); err != nil {
		t.Fatal(err)
	}
	if _, err := wh.Verify([]byte(reference.Body), headerOf(reference)); !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("got %v", err)
	}
}

func TestTheDefaultsAreFiveMinutesAndTheMachinesClock(t *testing.T) {
	t.Parallel()

	delivery := signatureCases(t)[1]
	wh, err := NewWebhook(delivery.Secrets[0])
	if err != nil {
		t.Fatal(err)
	}
	if wh.tolerance != 300 {
		t.Errorf("the tolerance is %d seconds", wh.tolerance)
	}
	// The fixture was signed long before this test runs.
	if err := wh.VerifySignature([]byte(delivery.Body), headerOf(delivery)); !errors.Is(err, ErrTimestampTooOld) {
		t.Errorf("got %v", err)
	}

	negative, err := NewWebhook(delivery.Secrets[0], WithTolerance(-time.Hour), WithClock(func() time.Time { return time.Unix(delivery.Now, 0) }))
	if err != nil {
		t.Fatal(err)
	}
	if err := negative.VerifySignature([]byte(delivery.Body), headerOf(delivery)); !errors.Is(err, ErrTimestampTooOld) {
		t.Errorf("a negative tolerance: got %v", err)
	}
}

func TestSignGivesTheSignatureOfEveryFixtureThatVerifies(t *testing.T) {
	t.Parallel()

	signed := 0
	for _, c := range signatureCases(t) {
		if c.Expect != "ok" {
			continue
		}
		sentAt, err := strconv.ParseInt(headerOf(c).Get(headerTimestamp), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		sent := strings.Fields(headerOf(c).Get(headerSignature))
		matched := false
		for _, secret := range c.Secrets {
			wh, err := NewWebhook(secret)
			if err != nil {
				t.Fatal(err)
			}
			signature, err := wh.Sign(headerOf(c).Get(headerID), time.Unix(sentAt, 0), []byte(c.Body))
			if err != nil {
				t.Fatal(err)
			}
			matched = matched || slices.Contains(sent, signature)
		}
		if !matched {
			t.Errorf("%s: no secret signs it as the header has it", c.Name)
		}
		signed++
	}
	if signed == 0 {
		t.Fatal("no fixture verifies")
	}
}

func TestWhatSignReturnsVerifies(t *testing.T) {
	t.Parallel()

	c := signatureCases(t)[1]
	now := time.Date(2027, 1, 2, 3, 4, 5, 999, time.UTC)
	signer, err := NewWebhook(c.Secrets[0], WithAdditionalSecrets("whsec_"+strings.Repeat("QUJD", 8)), WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"id":"evt","type":"webhook.test","occurredAt":"2027-01-02T03:04:05Z","data":{"endpointId":"e"}}`)
	signature, err := signer.Sign("evt", now, body)
	if err != nil || !strings.HasPrefix(signature, "v1,") || strings.Contains(signature, " ") {
		t.Fatalf("got %q, %v", signature, err)
	}

	header := http.Header{}
	header.Set(headerID, "evt")
	header.Set(headerTimestamp, strconv.FormatInt(now.Unix(), 10))
	header.Set(headerSignature, signature)
	receiver, err := NewWebhook(c.Secrets[0], WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	if event, err := receiver.Verify(body, header); err != nil || event.Type != EventTypeWebhookTest {
		t.Errorf("got %+v, %v", event, err)
	}

	other, err := NewWebhook("whsec_" + strings.Repeat("QUJD", 8))
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := other.Sign("evt", now, body); again == signature {
		t.Error("another secret gives the same signature")
	}
}
