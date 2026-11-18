package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultTolerance is how far a webhook's timestamp may be from the clock.
const DefaultTolerance = 5 * time.Minute

const (
	headerID         = "webhook-id"
	headerTimestamp  = "webhook-timestamp"
	headerSignature  = "webhook-signature"
	secretPrefix     = "whsec_"
	signatureVersion = "v1"
)

// A Verifier checks that a request was sent by Cimento, with an endpoint's
// signing secret. It is safe for concurrent use.
type Verifier struct {
	keys      [][]byte
	tolerance int64
	now       func() time.Time
}

type verifierConfig struct {
	secrets   []string
	tolerance time.Duration
	now       func() time.Time
}

// An Option changes how a Verifier verifies.
type Option func(config *verifierConfig)

// WithAdditionalSecrets has the Verifier accept a webhook signed with any of
// these secrets as well. After rotating an endpoint's secret, Cimento signs
// with the old and the new one for 24 hours; pass both while your deployment
// moves from one to the other.
func WithAdditionalSecrets(secrets ...string) Option {
	return func(config *verifierConfig) { config.secrets = append(config.secrets, secrets...) }
}

// WithTolerance sets how far a webhook's timestamp may be from the clock, in
// either direction, in place of DefaultTolerance. It is counted in whole
// seconds.
func WithTolerance(tolerance time.Duration) Option {
	return func(config *verifierConfig) { config.tolerance = tolerance }
}

// WithClock sets the clock timestamps are checked against, in place of
// time.Now. It is for tests.
func WithClock(now func() time.Time) Option {
	return func(config *verifierConfig) { config.now = now }
}

// NewVerifier returns a Verifier for an endpoint's signing secret, the
// "whsec_..." value Cimento shows once when the endpoint is created or its
// secret is rotated. It returns ErrMalformedSecret for a secret that is not
// one.
func NewVerifier(secret string, options ...Option) (*Verifier, error) {
	config := verifierConfig{secrets: []string{secret}, tolerance: DefaultTolerance, now: time.Now}
	for _, option := range options {
		option(&config)
	}

	verifier := &Verifier{tolerance: max(int64(config.tolerance/time.Second), 0), now: config.now}
	for i, secret := range config.secrets {
		encoded, prefixed := strings.CutPrefix(secret, secretPrefix)
		key, err := base64.StdEncoding.DecodeString(encoded)
		if !prefixed || err != nil || len(key) == 0 {
			return nil, fmt.Errorf("%w: secret %d of %d is not %q followed by base64", ErrMalformedSecret, i+1, len(config.secrets), secretPrefix)
		}
		verifier.keys = append(verifier.keys, key)
	}
	return verifier, nil
}

// Verify checks that body was sent by Cimento and reads it as an event. Pass
// the request's body exactly as it arrived: a body that was parsed and written
// out again no longer matches its signature.
//
// It returns an error wrapping one of this package's sentinel errors when the
// request cannot be trusted or the body is not an event.
func (v *Verifier) Verify(body []byte, header http.Header) (*Event, error) {
	if err := v.VerifySignature(body, header); err != nil {
		return nil, err
	}
	return Parse(body)
}

// VerifySignature checks that body was sent by Cimento, without reading it.
// Most receivers want Verify.
func (v *Verifier) VerifySignature(body []byte, header http.Header) error {
	id, timestamp, signatures := header.Get(headerID), header.Get(headerTimestamp), header.Get(headerSignature)
	for name, value := range map[string]string{headerID: id, headerTimestamp: timestamp, headerSignature: signatures} {
		if value == "" {
			return fmt.Errorf("%w: %s", ErrMissingHeader, name)
		}
	}

	sentAt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrInvalidTimestamp
	}
	now := v.now().Unix()
	if now-sentAt > v.tolerance {
		return fmt.Errorf("%w: sent %d seconds ago, and the tolerance is %d", ErrTimestampTooOld, now-sentAt, v.tolerance)
	}
	if sentAt-now > v.tolerance {
		return fmt.Errorf("%w: dated %d seconds ahead, and the tolerance is %d", ErrTimestampTooNew, sentAt-now, v.tolerance)
	}

	expected := make([][]byte, 0, len(v.keys))
	for _, key := range v.keys {
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(id + "." + timestamp + "."))
		mac.Write(body)
		expected = append(expected, mac.Sum(nil))
	}
	// A header holds one signature for each secret Cimento signed with, and
	// may hold signatures of versions this package does not know.
	matched := false
	for _, signature := range strings.Fields(signatures) {
		version, encoded, _ := strings.Cut(signature, ",")
		given, err := base64.StdEncoding.DecodeString(encoded)
		if version != signatureVersion || err != nil {
			continue
		}
		for _, mac := range expected {
			if hmac.Equal(mac, given) {
				matched = true
			}
		}
	}
	if !matched {
		return ErrInvalidSignature
	}
	return nil
}
