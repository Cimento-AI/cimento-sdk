package main

import (
	"fmt"
	"strconv"
	"strings"
)

func tsType(described field) string {
	base := map[kind]string{
		kindText: "string", kindUUID: "string", kindTime: "string", kindInteger: "number",
		kindTextList: "string[]", kindObject: described.Object,
	}[described.Kind]
	if described.Known != nil {
		// The empty intersection keeps the known values in an editor's
		// suggestions while any other string is still accepted.
		quoted := make([]string, 0, len(described.Known)+1)
		for _, value := range described.Known {
			quoted = append(quoted, strconv.Quote(value))
		}
		base = strings.Join(append(quoted, "(string & {})"), " | ")
		if described.Kind == kindTextList {
			base = "(" + base + ")[]"
		}
	}
	if described.Nullable {
		return base + " | null"
	}
	return base
}

// tsSource is the TypeScript module of payload interfaces and event types.
func tsSource(schema model) ([]byte, error) {
	taken := names{}
	var out strings.Builder
	fmt.Fprintf(&out, "// %s\n\n", generatedLine)

	for _, described := range schema.Payloads {
		if err := taken.claim(described.Name, described.Name); err != nil {
			return nil, err
		}
		fmt.Fprintf(&out, "export interface %s {\n", described.Name)
		for _, key := range described.Fields {
			fmt.Fprintf(&out, "  %s: %s;\n", key.Key, tsType(key))
		}
		out.WriteString("}\n\n")
	}

	out.WriteString("/** The data of each event Cimento sends, by the event's type. */\nexport interface EventPayloads {\n")
	for _, described := range schema.Events {
		fmt.Fprintf(&out, "  /** %s */\n  %q: %s;\n", described.Description, described.Type, described.Payload)
	}
	out.WriteString("}\n\n")

	out.WriteString("export type EventType = keyof EventPayloads;\n\n")
	out.WriteString("/** Every event type this package knows, in the order Cimento lists them. */\nexport const eventTypes = [\n")
	for _, described := range schema.Events {
		fmt.Fprintf(&out, "  %q,\n", described.Type)
	}
	out.WriteString("] as const satisfies readonly EventType[];\n\n")

	out.WriteString("/** An event of a type this package knows. `type` says which `data` it has. */\n")
	out.WriteString("export type KnownWebhookEvent = {\n  [T in EventType]: {\n    id: string;\n    type: T;\n    occurredAt: string;\n    data: EventPayloads[T];\n  };\n}[EventType];\n")
	return []byte(out.String()), nil
}

// tsFixtureTest is a test that holds every valid fixture to the generated
// type of its event, when it is compiled.
func tsFixtureTest(schema model, fixtures []string) ([]byte, error) {
	var out strings.Builder
	fmt.Fprintf(&out, "// %s\n\n", generatedLine)
	out.WriteString("import { expect, test } from \"vitest\";\n\n")
	out.WriteString("import { eventTypes, type EventPayloads } from \"../src/events.gen\";\n")
	out.WriteString("import schema from \"../../../schema/webhooks/events.schema.json\";\n")
	for i, name := range fixtures {
		fmt.Fprintf(&out, "import fixture%d from \"../../../fixtures/webhooks/events/valid/%s.json\";\n", i, name)
	}

	out.WriteString("\n// A fixture that is not its event's payload does not compile.\nconst typed: unknown[] = [\n")
	for i, name := range fixtures {
		eventType := strings.TrimSuffix(name, ".nulls")
		known := false
		for _, described := range schema.Events {
			known = known || described.Type == eventType
		}
		if !known {
			return nil, fmt.Errorf("%w: fixture %s.json is for no event in the schema", ErrUnsupported, name)
		}
		fmt.Fprintf(&out, "  fixture%d.data satisfies EventPayloads[%q],\n", i, eventType)
	}
	out.WriteString("];\n\n")
	fmt.Fprintf(&out, "test(\"every valid fixture is typed as its event's payload\", () => {\n  expect(typed).toHaveLength(%d);\n});\n\n", len(fixtures))
	out.WriteString("test(\"the event types are the schema's, in its order\", () => {\n  expect([...eventTypes]).toEqual(schema.oneOf.map((event) => event.title));\n});\n")
	return []byte(out.String()), nil
}
