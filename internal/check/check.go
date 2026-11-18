// Package main is the check command: it runs files of webhooks through both
// SDKs and reports where either fails or the two disagree.
//
//	go run ./check -root .. -events <dir>        every file is an event body
//	go run ./check -root .. -deliveries <dir>    recorded requests
//
// A recorded request is two files with one name: <name>.body, the request's
// body byte for byte, and <name>.json, an object with "headers" (each name to
// a list of values) and "receivedAt" (when it arrived, RFC 3339). The secrets
// to verify with are in CIMENTO_WEBHOOK_SECRETS, separated by commas.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cimento-ai/cimento-sdk/go/webhooks"
)

// ErrInput is a directory or a file the command cannot check at all.
var ErrInput = errors.New("cannot read what was given to check")

const (
	modeEvents     = "events"
	modeDeliveries = "deliveries"
	secretsEnv     = "CIMENTO_WEBHOOK_SECRETS"
)

// result is what one SDK made of one file. Both SDKs' results are compared
// field by field, so the TypeScript half prints exactly this.
type result struct {
	File string `json:"file"`
	// Error is why the SDK refused the file, by the shared fixtures' names
	// for the reasons, or "" if it did not.
	Error string `json:"error"`
	ID    string `json:"id"`
	Type  string `json:"type"`
	// Typed is whether the event came back as its type and not as an event
	// the SDK does not know.
	Typed bool `json:"typed"`
	// OccurredAt and Data are the event as the SDK handed it over, so that
	// what the two SDKs read, and not only that they read it, is compared.
	OccurredAt string          `json:"occurredAt"`
	Data       json.RawMessage `json:"data"`
}

// codes are the shared fixtures' names for the Go SDK's errors.
var codes = []struct {
	sentinel error
	code     string
}{
	{webhooks.ErrMalformedSecret, "malformed_secret"},
	{webhooks.ErrMissingHeader, "missing_header"},
	{webhooks.ErrInvalidTimestamp, "invalid_timestamp"},
	{webhooks.ErrTimestampTooOld, "timestamp_too_old"},
	{webhooks.ErrTimestampTooNew, "timestamp_too_new"},
	{webhooks.ErrInvalidSignature, "invalid_signature"},
	{webhooks.ErrInvalidEvent, "invalid_event"},
}

func codeOf(err error) string {
	for _, known := range codes {
		if errors.Is(err, known.sentinel) {
			return known.code
		}
	}
	return "error: " + err.Error()
}

// recorded is the .json beside a recorded request's body.
type recorded struct {
	Headers    http.Header `json:"headers"`
	ReceivedAt time.Time   `json:"receivedAt"`
}

// files lists what there is to check in dir, by name: every file for events,
// and every .body file for deliveries.
func files(mode string, dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInput, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || (mode == modeDeliveries && !strings.HasSuffix(entry.Name(), ".body")) {
			continue
		}
		names = append(names, entry.Name())
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%w: %s has nothing to check", ErrInput, dir)
	}
	sort.Strings(names)
	return names, nil
}

func secrets() []string {
	var given []string
	for _, secret := range strings.Split(os.Getenv(secretsEnv), ",") {
		if secret = strings.TrimSpace(secret); secret != "" {
			given = append(given, secret)
		}
	}
	return given
}

// checkWithGo runs every file through the Go SDK.
func checkWithGo(mode string, dir string, names []string) ([]result, error) {
	results := make([]result, 0, len(names))
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInput, err)
		}
		checked := result{File: name}
		var event *webhooks.Event
		if mode == modeEvents {
			event, err = webhooks.Parse(body)
		} else {
			event, err = verify(dir, name, body)
			if errors.Is(err, ErrInput) {
				return nil, err
			}
		}
		if err != nil {
			checked.Error = codeOf(err)
		} else {
			checked.ID, checked.Type, checked.Typed = event.ID, event.Type, event.Data != nil
			checked.OccurredAt = event.OccurredAt.UTC().Format(time.RFC3339Nano)
			checked.Data = event.RawData
			if checked.Typed {
				if checked.Data, err = json.Marshal(event.Data); err != nil {
					return nil, err
				}
			}
		}
		results = append(results, checked)
	}
	return results, nil
}

// verify checks a recorded request as a receiver would have when it arrived.
func verify(dir string, name string, body []byte) (*webhooks.Event, error) {
	described, err := os.ReadFile(filepath.Join(dir, strings.TrimSuffix(name, ".body")+".json"))
	if err != nil {
		return nil, fmt.Errorf("%w: %s has no .json beside it: %v", ErrInput, name, err)
	}
	var request recorded
	if err := json.Unmarshal(described, &request); err != nil || request.ReceivedAt.IsZero() {
		return nil, fmt.Errorf("%w: the .json beside %s has no headers and receivedAt: %v", ErrInput, name, err)
	}
	given := secrets()
	if len(given) == 0 {
		return nil, fmt.Errorf("%w: %s is not set", ErrInput, secretsEnv)
	}
	wh, err := webhooks.NewWebhook(given[0],
		webhooks.WithAdditionalSecrets(given[1:]...),
		webhooks.WithClock(func() time.Time { return request.ReceivedAt }))
	if err != nil {
		return nil, err
	}
	// The names were written as they arrived; Get wants them canonical.
	header := http.Header{}
	for headerName, values := range request.Headers {
		for _, value := range values {
			header.Add(headerName, value)
		}
	}
	return wh.Verify(body, header)
}
