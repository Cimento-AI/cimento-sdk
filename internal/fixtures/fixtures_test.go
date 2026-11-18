package fixtures

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	schemaPath     = "../../schema/webhooks/events.schema.json"
	fixturesPath   = "../../fixtures/webhooks"
	nullsSuffix    = ".nulls"
	defaultSeconds = 300
)

var outcomes = []string{
	"ok", "missing_header", "invalid_timestamp", "timestamp_too_old", "timestamp_too_new",
	"invalid_signature", "malformed_secret",
}

// property is one field of a payload as the schema describes it.
type property struct {
	Type  json.RawMessage `json:"type"`
	Ref   string          `json:"$ref"`
	Known []string        `json:"x-known-values"`
	Items *struct {
		Known []string `json:"x-known-values"`
	} `json:"items"`
}

func (p property) nullable() bool {
	var types []string
	return json.Unmarshal(p.Type, &types) == nil && slices.Contains(types, "null")
}

type payloadSchema struct {
	Required   []string            `json:"required"`
	Properties map[string]property `json:"properties"`
}

type eventSchema struct {
	Events []struct {
		Title      string `json:"title"`
		Properties struct {
			Data struct {
				Ref string `json:"$ref"`
			} `json:"data"`
		} `json:"properties"`
	} `json:"oneOf"`
	Defs map[string]payloadSchema `json:"$defs"`
}

func (s eventSchema) def(ref string) payloadSchema {
	return s.Defs[strings.TrimPrefix(ref, "#/$defs/")]
}

// payloadOf is the schema of an event type's data, and whether the schema has
// the type.
func (s eventSchema) payloadOf(eventType string) (payloadSchema, bool) {
	for _, event := range s.Events {
		if event.Title == eventType {
			return s.def(event.Properties.Data.Ref), true
		}
	}
	return payloadSchema{}, false
}

func readSchema(t *testing.T) (eventSchema, *jsonschema.Schema) {
	t.Helper()

	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var described eventSchema
	if err := json.Unmarshal(raw, &described); err != nil {
		t.Fatal(err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource("events.schema.json", document); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile("events.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return described, compiled
}

// readEvents reads the fixtures in one directory under events, by file name
// without its extension.
func readEvents(t *testing.T, directory string) map[string][]byte {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(fixturesPath, "events", directory, "*"))
	if err != nil {
		t.Fatal(err)
	}
	events := map[string][]byte{}
	for _, path := range paths {
		name, isJSON := strings.CutSuffix(filepath.Base(path), ".json")
		if !isJSON {
			t.Errorf("%s is not a .json file", path)
			continue
		}
		events[name], err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(events) == 0 {
		t.Fatalf("no fixtures in events/%s", directory)
	}
	return events
}

type envelope struct {
	ID         string                     `json:"id"`
	Type       string                     `json:"type"`
	OccurredAt string                     `json:"occurredAt"`
	Data       map[string]json.RawMessage `json:"data"`
}

// readEnvelope reads a body the way an SDK must: it is an event when it is an
// object with an id, a type, a time and a data object, whatever else it holds.
func readEnvelope(body []byte) (envelope, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return envelope{}, err
	}
	if fields == nil {
		return envelope{}, fmt.Errorf("not an object")
	}
	var event envelope
	for key, into := range map[string]any{"id": &event.ID, "type": &event.Type, "occurredAt": &event.OccurredAt, "data": &event.Data} {
		raw, present := fields[key]
		if !present || string(raw) == "null" {
			return envelope{}, fmt.Errorf("%s is missing", key)
		}
		if err := json.Unmarshal(raw, into); err != nil {
			return envelope{}, fmt.Errorf("%s: %w", key, err)
		}
	}
	if event.ID == "" || event.Type == "" {
		return envelope{}, fmt.Errorf("id or type is empty")
	}
	if _, err := time.Parse(time.RFC3339Nano, event.OccurredAt); err != nil {
		return envelope{}, fmt.Errorf("occurredAt: %w", err)
	}
	return event, nil
}

func TestEveryEventTypeHasAValidFixture(t *testing.T) {
	t.Parallel()

	schema, _ := readSchema(t)
	fixtures := readEvents(t, "valid")
	for _, event := range schema.Events {
		if _, ok := fixtures[event.Title]; !ok {
			t.Errorf("events/valid has no %s.json", event.Title)
		}
		payload := schema.def(event.Properties.Data.Ref)
		_, hasNulls := fixtures[event.Title+nullsSuffix]
		if needsNulls := len(nullablePaths(schema, payload, "")) > 0; needsNulls != hasNulls {
			t.Errorf("%s: has a field that may be null: %t; has a %s fixture: %t", event.Title, needsNulls, nullsSuffix, hasNulls)
		}
	}
	for name := range fixtures {
		eventType := strings.TrimSuffix(name, nullsSuffix)
		if _, ok := schema.payloadOf(eventType); !ok {
			t.Errorf("events/valid/%s.json is for a type the schema does not have", name)
		}
	}
}

func TestValidFixturesMatchTheSchemaExactly(t *testing.T) {
	t.Parallel()

	schema, compiled := readSchema(t)
	for name, body := range readEvents(t, "valid") {
		eventType, isNulls := strings.CutSuffix(name, nullsSuffix)
		instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if err := compiled.Validate(instance); err != nil {
			t.Errorf("%s does not match the schema:\n%v", name, err)
			continue
		}
		var fields map[string]json.RawMessage
		event, err := readEnvelope(body)
		if err != nil || json.Unmarshal(body, &fields) != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if event.Type != eventType {
			t.Errorf("%s holds a %s event", name, event.Type)
		}
		if len(fields) != 4 {
			t.Errorf("%s has envelope fields beyond id, type, occurredAt and data", name)
		}
		payload, _ := schema.payloadOf(eventType)
		for _, problem := range compare(schema, payload, event.Data, "data", isNulls) {
			t.Errorf("%s: %s", name, problem)
		}
	}
}

// nullablePaths lists the fields of a payload that may be null.
func nullablePaths(schema eventSchema, payload payloadSchema, prefix string) []string {
	var paths []string
	for _, key := range payload.Required {
		described := payload.Properties[key]
		switch {
		case described.nullable():
			paths = append(paths, prefix+key)
		case described.Ref != "":
			paths = append(paths, nullablePaths(schema, schema.def(described.Ref), prefix+key+".")...)
		}
	}
	return paths
}

// compare reports where data differs from what the schema lists: a key that is
// missing or extra, a value outside a field's known values, and, for a nulls
// fixture, a nullable field that is not null.
func compare(schema eventSchema, payload payloadSchema, data map[string]json.RawMessage, path string, allNull bool) []string {
	var problems []string
	for key := range data {
		if !slices.Contains(payload.Required, key) {
			problems = append(problems, fmt.Sprintf("%s.%s is not in the schema", path, key))
		}
	}
	for _, key := range payload.Required {
		raw, present := data[key]
		described := payload.Properties[key]
		at := path + "." + key
		if !present {
			problems = append(problems, at+" is missing")
			continue
		}
		isNull := string(raw) == "null"
		if allNull && described.nullable() && !isNull {
			problems = append(problems, at+" may be null and is not, in a nulls fixture")
		}
		known := described.Known
		if described.Items != nil {
			known = described.Items.Known
		}
		if known != nil && !isNull {
			var one string
			var many []string
			if json.Unmarshal(raw, &one) == nil {
				many = []string{one}
			} else if err := json.Unmarshal(raw, &many); err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", at, err))
			}
			for _, value := range many {
				if !slices.Contains(known, value) {
					problems = append(problems, fmt.Sprintf("%s is %q, which is not a known value", at, value))
				}
			}
		}
		if described.Ref != "" {
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(raw, &nested); err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", at, err))
				continue
			}
			problems = append(problems, compare(schema, schema.def(described.Ref), nested, at, allNull)...)
		}
	}
	return problems
}

func TestOpenFixturesAreEventsAndInvalidFixturesAreNot(t *testing.T) {
	t.Parallel()

	_, compiled := readSchema(t)
	for name, body := range readEvents(t, "open") {
		if _, err := readEnvelope(body); err != nil {
			t.Errorf("open/%s is not an event: %v", name, err)
		}
	}
	for name, body := range readEvents(t, "invalid") {
		if event, err := readEnvelope(body); err == nil {
			t.Errorf("invalid/%s is an event, of type %s", name, event.Type)
		}
		instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
		if err == nil && compiled.Validate(instance) == nil {
			t.Errorf("invalid/%s matches the schema", name)
		}
	}
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

// outcome verifies a case from the Standard Webhooks rules alone. It is the
// reference the SDKs are held to, and shares no code with them.
func (c signatureCase) outcome() string {
	if len(c.Secrets) == 0 {
		return "malformed_secret"
	}
	keys := make([][]byte, 0, len(c.Secrets))
	for _, secret := range c.Secrets {
		encoded, prefixed := strings.CutPrefix(secret, "whsec_")
		key, err := base64.StdEncoding.DecodeString(encoded)
		if !prefixed || err != nil || len(key) == 0 {
			return "malformed_secret"
		}
		keys = append(keys, key)
	}

	header := map[string]string{}
	for name, value := range c.Headers {
		header[strings.ToLower(name)] = value
	}
	id, timestamp, signatures := header["webhook-id"], header["webhook-timestamp"], header["webhook-signature"]
	if id == "" || timestamp == "" || signatures == "" {
		return "missing_header"
	}
	sentAt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return "invalid_timestamp"
	}
	tolerance := int64(defaultSeconds)
	if c.ToleranceSeconds != nil {
		tolerance = *c.ToleranceSeconds
	}
	if c.Now-sentAt > tolerance {
		return "timestamp_too_old"
	}
	if sentAt-c.Now > tolerance {
		return "timestamp_too_new"
	}

	for _, signature := range strings.Fields(signatures) {
		version, encoded, _ := strings.Cut(signature, ",")
		given, err := base64.StdEncoding.DecodeString(encoded)
		if version != "v1" || err != nil {
			continue
		}
		for _, key := range keys {
			mac := hmac.New(sha256.New, key)
			mac.Write([]byte(id + "." + timestamp + "." + c.Body))
			if hmac.Equal(mac.Sum(nil), given) {
				return "ok"
			}
		}
	}
	return "invalid_signature"
}

func TestSignatureCasesExpectWhatTheRulesGive(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join(fixturesPath, "signatures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []signatureCase `json:"cases"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		t.Fatal(err)
	}

	names := map[string]bool{}
	expected := map[string]int{}
	for _, c := range file.Cases {
		if c.Name == "" || names[c.Name] {
			t.Errorf("case name %q is empty or used twice", c.Name)
		}
		names[c.Name] = true
		if !slices.Contains(outcomes, c.Expect) {
			t.Errorf("%s: %q is not an outcome", c.Name, c.Expect)
		}
		expected[c.Expect]++
		if got := c.outcome(); got != c.Expect {
			t.Errorf("%s: expects %s, and the rules give %s", c.Name, c.Expect, got)
		}
	}
	unused := slices.DeleteFunc(slices.Clone(outcomes), func(outcome string) bool { return expected[outcome] > 0 })
	sort.Strings(unused)
	if len(unused) > 0 {
		t.Errorf("no case expects %v", unused)
	}
}
