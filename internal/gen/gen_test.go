package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoNames(t *testing.T) {
	t.Parallel()

	for text, want := range map[string]string{
		"sendId":                        "SendID",
		"mcpServerId":                   "MCPServerID",
		"user":                          "User",
		"changedKeys":                   "ChangedKeys",
		"agent_hub.mcp_server.approved": "AgentHubMCPServerApproved",
		"phishing.simulation.no_action": "PhishingSimulationNoAction",
		"false_positive":                "FalsePositive",
		"telemetry.full_content":        "TelemetryFullContent",
		"unapproved_mcp_servers":        "UnapprovedMCPServers",
		"alerts.manager_copy_on_third":  "AlertsManagerCopyOnThird",
		"webhook.test":                  "WebhookTest",
	} {
		if got := goName(text); got != want {
			t.Errorf("goName(%q) = %s, want %s", text, got, want)
		}
	}
}

// schemaWith is a schema of one event whose payload has the given properties,
// all required in the order given.
func schemaWith(required string, properties string) []byte {
	return []byte(`{
		"oneOf": [{
			"title": "test.event", "description": "A test.", "x-source": "system",
			"type": "object", "required": ["id", "type", "occurredAt", "data"],
			"properties": {
				"id": {"type": "string", "format": "uuid"},
				"type": {"const": "test.event"},
				"occurredAt": {"type": "string", "format": "date-time"},
				"data": {"$ref": "#/$defs/TestPayload"}
			}
		}],
		"$defs": {
			"TestPayload": {"type": "object", "required": [` + required + `], "properties": {` + properties + `}},
			"Other": {"type": "object", "required": ["id"], "properties": {"id": {"type": "string", "format": "uuid"}}}
		}
	}`)
}

// withPayload is schemaWith's schema with its payload definition replaced.
func withPayload(definition string) []byte {
	schema := string(schemaWith(`"id"`, `"id": {"type": "string"}`))
	return []byte(strings.Replace(schema, `"type": "object", "required": ["id"], "properties": {"id": {"type": "string"}}`, definition, 1))
}

// withEvent is schemaWith's schema with its event's data property replaced,
// its id property too when one is given, and more keywords on the event.
func withEvent(data string, id string, keywords ...string) []byte {
	schema := string(schemaWith(`"id"`, `"id": {"type": "string"}`))
	schema = strings.Replace(schema, `"data": {"$ref": "#/$defs/TestPayload"}`, data, 1)
	if id != "" {
		schema = strings.Replace(schema, `"id": {"type": "string", "format": "uuid"},
				"type"`, id+`, "type"`, 1)
	}
	for _, keyword := range keywords {
		schema = strings.Replace(schema, `"title": "test.event"`, keyword+`, "title": "test.event"`, 1)
	}
	return []byte(schema)
}

// everyKind is a schema whose payload has a field of every kind the
// generator knows.
func everyKind() []byte {
	return schemaWith(
		`"id", "name", "note", "at", "until", "count", "kind", "tags", "other"`,
		`"id": {"type": "string", "format": "uuid"},
		 "name": {"type": "string"},
		 "note": {"type": ["string", "null"]},
		 "at": {"type": "string", "format": "date-time"},
		 "until": {"type": ["string", "null"], "format": "date-time"},
		 "count": {"type": "integer"},
		 "kind": {"type": ["string", "null"], "x-known-values": ["one", "two_words"]},
		 "tags": {"type": "array", "items": {"type": "string", "x-known-values": ["a.b"]}},
		 "other": {"$ref": "#/$defs/Other"}`,
	)
}

func TestTheGeneratorWritesEveryKindOfField(t *testing.T) {
	t.Parallel()

	schema, err := readModel(everyKind())
	if err != nil {
		t.Fatal(err)
	}

	goTypes, err := goSource(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"EventTypeTestEvent = \"test.event\"",
		"ID    string     `json:\"id\"`",
		"Note  *string    `json:\"note\"`",
		"At    time.Time  `json:\"at\"`",
		"Until *time.Time `json:\"until\"`",
		"Count int        `json:\"count\"`",
		"Tags  []string   `json:\"tags\"`",
		"Other Other      `json:\"other\"`",
		"TestPayloadKindTwoWords = \"two_words\"",
		"TestPayloadTagsAB = \"a.b\"",
		"case EventTypeTestEvent:\n\t\treturn &TestPayload{}, true",
	} {
		if !strings.Contains(string(goTypes), want) {
			t.Errorf("the Go source lacks %q:\n%s", want, goTypes)
		}
	}

	tsTypes, err := tsSource(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"  id: string;\n  name: string;\n  note: string | null;\n  at: string;\n  until: string | null;\n  count: number;\n",
		`  kind: "one" | "two_words" | (string & {}) | null;`,
		`  tags: ("a.b" | (string & {}))[];`,
		"  other: Other;",
		"  /** A test. */\n  \"test.event\": TestPayload;",
		"    known: true;\n    id: string;\n    type: T;",
	} {
		if !strings.Contains(string(tsTypes), want) {
			t.Errorf("the TypeScript source lacks %q:\n%s", want, tsTypes)
		}
	}
}

func TestTheGeneratorDescribesEachPayloadForRunTime(t *testing.T) {
	t.Parallel()

	schema, err := readModel(everyKind())
	if err != nil {
		t.Fatal(err)
	}
	shapes := string(tsShapes(schema))
	for _, want := range []string{
		`  TestPayload: [
    ["id", "string", false],
    ["name", "string", false],
    ["note", "string", true],
    ["at", "time", false],
    ["until", "time", true],
    ["count", "integer", false],
    ["kind", "string", true],
    ["tags", "strings", false],
    ["other", { payload: "Other" }, false],
  ],`,
		"  Other: [\n    [\"id\", \"string\", false],\n  ],",
		"  \"test.event\": \"TestPayload\",\n};",
	} {
		if !strings.Contains(shapes, want) {
			t.Errorf("the shapes lack %q:\n%s", want, shapes)
		}
	}
	types, err := tsSource(schema)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(types), "payloadShapes") {
		t.Error("the module the package exports has the shapes in it")
	}
}

func TestTheGeneratorRefusesWhatItCannotType(t *testing.T) {
	t.Parallel()

	valid := schemaWith(`"id"`, `"id": {"type": "string"}`)
	if _, err := readModel(valid); err != nil {
		t.Fatal(err)
	}
	for name, schema := range map[string][]byte{
		"a number":                        schemaWith(`"score"`, `"score": {"type": "number"}`),
		"a format it does not know":       schemaWith(`"at"`, `"at": {"type": "string", "format": "date"}`),
		"an enum":                         schemaWith(`"kind"`, `"kind": {"type": "string", "enum": ["a"]}`),
		"a list of numbers":               schemaWith(`"counts"`, `"counts": {"type": "array", "items": {"type": "integer"}}`),
		"a list that may be null":         schemaWith(`"tags"`, `"tags": {"type": ["array", "null"], "items": {"type": "string"}}`),
		"an object that may be null":      schemaWith(`"other"`, `"other": {"type": ["object", "null"]}`),
		"a $ref to nothing":               schemaWith(`"other"`, `"other": {"$ref": "#/$defs/Missing"}`),
		"a $ref beside other keywords":    schemaWith(`"other"`, `"other": {"$ref": "#/$defs/Other", "type": "object"}`),
		"a property that is optional":     schemaWith(`"id"`, `"id": {"type": "string"}, "name": {"type": "string"}`),
		"a key that is not described":     schemaWith(`"id", "name"`, `"id": {"type": "string"}`),
		"known values on a number":        schemaWith(`"count"`, `"count": {"type": "integer", "x-known-values": ["1"]}`),
		"an enum on a list's items":       schemaWith(`"tags"`, `"tags": {"type": "array", "items": {"type": "string", "enum": ["approved"]}}`),
		"a format on a list's items":      schemaWith(`"ids"`, `"ids": {"type": "array", "items": {"type": "string", "format": "uuid"}}`),
		"a limit on a list":               schemaWith(`"tags"`, `"tags": {"type": "array", "items": {"type": "string"}, "maxItems": 3}`),
		"no events":                       []byte(`{"oneOf": [], "$defs": {}}`),
		"a payload that may be null":      withPayload(`"type": ["object", "null"], "required": [], "properties": {}`),
		"a payload that is not an object": withPayload(`"type": "string"`),
		"a payload closed to new fields":  withPayload(`"type": "object", "required": [], "properties": {}, "additionalProperties": false`),
		"an event whose data is inline":   withEvent(`"data": {"type": "object"}`, ``),
		"an event with another id":        withEvent(`"data": {"$ref": "#/$defs/TestPayload"}`, `"id": {"type": "string"}`),
		"an event with a fifth field":     withEvent(`"data": {"$ref": "#/$defs/TestPayload"}, "version": {"type": "integer"}`, ``),
		"an event that is one of several": withEvent(`"data": {"$ref": "#/$defs/TestPayload"}`, ``, `"anyOf": []`),
		"an event that may be null":       []byte(strings.Replace(string(valid), `"type": "object", "required": ["id", "type"`, `"type": ["object", "null"], "required": ["id", "type"`, 1)),
		"an event whose title is a list":  []byte(strings.Replace(string(valid), `"title": "test.event"`, `"title": ["test.event"]`, 1)),
		"an event type that is a number":  []byte(strings.Replace(string(valid), `"type": {"const": "test.event"}`, `"type": {"const": 7}`, 1)),
		"a data $ref that is a list":      withEvent(`"data": {"$ref": ["#/$defs/TestPayload"]}`, ``),
		"required that is not a list":     withPayload(`"type": "object", "required": "id", "properties": {"id": {"type": "string"}}`),
		"a format that is not text":       schemaWith(`"id"`, `"id": {"type": "string", "format": 4}`),
		"known values that are numbers":   schemaWith(`"kind"`, `"kind": {"type": "string", "x-known-values": [1, 2]}`),
		"events that are not a list":      []byte(`{"oneOf": {}, "$defs": {}}`),
		"defs that are a list":            []byte(`{"oneOf": [], "$defs": []}`),
	} {
		if _, err := readModel(schema); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestASchemaThatIsNotJSONIsNotCalledUnsupported(t *testing.T) {
	t.Parallel()

	for name, schema := range map[string]string{"a truncated file": `{"oneOf": [`, "an empty file": ``, "text": `oneOf`} {
		if _, err := readModel([]byte(schema)); !errors.Is(err, ErrNotJSON) || errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestTheGeneratorRefusesTwoNamesThatComeOutTheSame(t *testing.T) {
	t.Parallel()

	for name, schema := range map[string][]byte{
		"two keys":                      schemaWith(`"userId", "user_id"`, `"userId": {"type": "string"}, "user_id": {"type": "string"}`),
		"two values":                    schemaWith(`"kind"`, `"kind": {"type": "string", "x-known-values": ["no_action", "no.action"]}`),
		"a value that is no identifier": schemaWith(`"kind"`, `"kind": {"type": "string", "x-known-values": ["--"]}`),
	} {
		read, err := readModel(schema)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := goSource(read); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestAFixtureForNoEventIsRefused(t *testing.T) {
	t.Parallel()

	schema, err := readModel(schemaWith(`"id"`, `"id": {"type": "string"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tsFixtureTest(schema, []string{"test.event", "test.event.nulls"}); err != nil {
		t.Error(err)
	}
	if _, err := tsFixtureTest(schema, []string{"test.other"}); !errors.Is(err, ErrUnsupported) {
		t.Error(err)
	}
}

// The committed files are what the generator writes from the committed schema
// and fixtures.
func TestTheGeneratedFilesAreCurrent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, pattern := range []string{schemaFile, fixturesGlob} {
		paths, err := filepath.Glob(filepath.Join("../..", pattern))
		if err != nil || len(paths) == 0 {
			t.Fatalf("%s: nothing to copy: %v", pattern, err)
		}
		for _, path := range paths {
			relative, _ := filepath.Rel("../..", path)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(filepath.Join(root, relative)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, relative), content, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := generate(root); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{goFile, tsFile, tsShapesFile, tsTestFile} {
		written, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatal(err)
		}
		committed, err := os.ReadFile(filepath.Join("../..", file))
		if err != nil {
			t.Fatal(err)
		}
		if string(written) != string(committed) {
			t.Errorf("%s is stale: run `make generate` and commit the result", file)
		}
	}
}
