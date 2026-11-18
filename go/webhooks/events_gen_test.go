package webhooks

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const validFixtures = "../../fixtures/webhooks/events/valid"

// sameJSON reports whether two decoded JSON values are equal, taking two
// times that are the same instant as equal however they are written.
func sameJSON(a any, b any) bool {
	switch a := a.(type) {
	case map[string]any:
		b, ok := b.(map[string]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for key, value := range a {
			other, present := b[key]
			if !present || !sameJSON(value, other) {
				return false
			}
		}
		return true
	case []any:
		b, ok := b.([]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for i := range a {
			if !sameJSON(a[i], b[i]) {
				return false
			}
		}
		return true
	case string:
		b, ok := b.(string)
		if !ok {
			return false
		}
		at, errA := time.Parse(time.RFC3339Nano, a)
		bt, errB := time.Parse(time.RFC3339Nano, b)
		if errA == nil && errB == nil {
			return at.Equal(bt)
		}
		return a == b
	}
	return reflect.DeepEqual(a, b)
}

func TestEveryValidFixtureIsReadIntoItsPayloadAndWrittenBackWhole(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob(filepath.Join(validFixtures, "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures in %s: %v", validFixtures, err)
	}
	read := map[string]bool{}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var event struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &event); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		payload, known := newPayload(event.Type)
		if !known {
			t.Errorf("%s: %s has no payload", name, event.Type)
			continue
		}
		read[event.Type] = true

		decoder := json.NewDecoder(bytes.NewReader(event.Data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(payload); err != nil {
			t.Errorf("%s does not fit %T: %v", name, payload, err)
			continue
		}
		written, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var before, after any
		if err := json.Unmarshal(event.Data, &before); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(written, &after); err != nil {
			t.Fatal(err)
		}
		if !sameJSON(before, after) {
			t.Errorf("%s changed on its way through %T:\n got %s\nfrom %s", name, payload, written, event.Data)
		}
	}

	if _, known := newPayload("billing.invoice.paid"); known {
		t.Error("an event type the schema does not have has a payload")
	}
	var schema struct {
		Events []json.RawMessage `json:"oneOf"`
	}
	raw, err := os.ReadFile("../../schema/webhooks/events.schema.json")
	if err != nil || json.Unmarshal(raw, &schema) != nil {
		t.Fatalf("read schema: %v", err)
	}
	if len(read) != len(schema.Events) {
		t.Errorf("fixtures were read as %d event types, and the schema has %d", len(read), len(schema.Events))
	}
}
