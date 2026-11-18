package webhooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const fixtures = "../../fixtures/webhooks"

// eventFixtures reads the event bodies in one directory of the shared
// fixtures, by file name without its extension.
func eventFixtures(t testing.TB, directory string) map[string][]byte {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(fixtures, "events", directory, "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures in events/%s: %v", directory, err)
	}
	bodies := map[string][]byte{}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		bodies[strings.TrimSuffix(filepath.Base(path), ".json")] = body
	}
	return bodies
}

type signatureCase struct {
	Name             string            `json:"name"`
	Secrets          []string          `json:"secrets"`
	Headers          map[string]string `json:"headers"`
	Body             string            `json:"body"`
	Now              int64             `json:"now"`
	ToleranceSeconds *int64            `json:"toleranceSeconds"`
	Expect           string            `json:"expect"`
}

func signatureCases(t testing.TB) []signatureCase {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(fixtures, "signatures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []signatureCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil || len(file.Cases) == 0 {
		t.Fatalf("read signature cases: %v", err)
	}
	return file.Cases
}

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
