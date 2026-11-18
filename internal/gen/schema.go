package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrUnsupported is a schema this generator has no mapping for. It fails
// rather than guess, because a guess would be published in two SDKs.
var ErrUnsupported = errors.New("the schema has something the generator cannot turn into types")

const defsPrefix = "#/$defs/"

type kind int

const (
	kindText kind = iota
	kindUUID
	kindTime
	kindInteger
	kindTextList
	kindObject
)

// field is one key of a payload. Nullable applies to every kind but lists and
// objects, and Known to text and lists of text.
type field struct {
	Key      string
	Kind     kind
	Nullable bool
	Object   string
	Known    []string
}

type payload struct {
	Name   string
	Fields []field
}

type event struct {
	Type        string
	Description string
	Payload     string
}

// model is the schema as the generator needs it: events in catalog order, and
// payloads in the order the events first use them.
type model struct {
	Events   []event
	Payloads []payload
}

type rawProperty struct {
	Type   json.RawMessage `json:"type"`
	Format string          `json:"format"`
	Ref    string          `json:"$ref"`
	Items  *rawProperty    `json:"items"`
	Known  []string        `json:"x-known-values"`
	Const  string          `json:"const"`
}

type rawObject struct {
	Title       string                     `json:"title"`
	Description string                     `json:"description"`
	Required    []string                   `json:"required"`
	Properties  map[string]json.RawMessage `json:"properties"`
}

type rawSchema struct {
	Events []rawObject                `json:"oneOf"`
	Defs   map[string]json.RawMessage `json:"$defs"`
}

func readModel(schema []byte) (model, error) {
	var raw rawSchema
	if err := json.Unmarshal(schema, &raw); err != nil {
		return model{}, fmt.Errorf("read schema: %w", err)
	}
	if len(raw.Events) == 0 {
		return model{}, fmt.Errorf("%w: no events under oneOf", ErrUnsupported)
	}

	reader := modelReader{defs: raw.Defs, seen: map[string]bool{}}
	for _, described := range raw.Events {
		var data, eventType rawProperty
		if err := json.Unmarshal(described.Properties["data"], &data); err != nil {
			return model{}, fmt.Errorf("%s: data: %w", described.Title, err)
		}
		if err := json.Unmarshal(described.Properties["type"], &eventType); err != nil {
			return model{}, fmt.Errorf("%s: type: %w", described.Title, err)
		}
		name, isRef := strings.CutPrefix(data.Ref, defsPrefix)
		if described.Title == "" || eventType.Const != described.Title || !isRef {
			return model{}, fmt.Errorf("%w: event %q is not a titled envelope whose data is a $ref", ErrUnsupported, described.Title)
		}
		if err := reader.payload(name); err != nil {
			return model{}, fmt.Errorf("%s: %w", described.Title, err)
		}
		reader.model.Events = append(reader.model.Events, event{Type: described.Title, Description: described.Description, Payload: name})
	}
	return reader.model, nil
}

type modelReader struct {
	defs  map[string]json.RawMessage
	seen  map[string]bool
	model model
}

func (r *modelReader) payload(name string) error {
	if r.seen[name] {
		return nil
	}
	r.seen[name] = true
	definition, ok := r.defs[name]
	if !ok {
		return fmt.Errorf("%w: $ref to %s, which $defs does not have", ErrUnsupported, name)
	}
	var object rawObject
	if err := json.Unmarshal(definition, &object); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if len(object.Required) != len(object.Properties) {
		return fmt.Errorf("%w: %s has a property that is not required, or the reverse", ErrUnsupported, name)
	}

	// Reserve this payload's place before the objects it holds.
	at := len(r.model.Payloads)
	r.model.Payloads = append(r.model.Payloads, payload{Name: name})
	fields := make([]field, 0, len(object.Required))
	for _, key := range object.Required {
		described, ok := object.Properties[key]
		if !ok {
			return fmt.Errorf("%w: %s requires %s and does not describe it", ErrUnsupported, name, key)
		}
		read, err := r.field(key, described)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", name, key, err)
		}
		fields = append(fields, read)
	}
	r.model.Payloads[at].Fields = fields
	return nil
}

func (r *modelReader) field(key string, described json.RawMessage) (field, error) {
	var keywords map[string]json.RawMessage
	if err := json.Unmarshal(described, &keywords); err != nil {
		return field{}, err
	}
	for keyword := range keywords {
		if !slices.Contains([]string{"type", "format", "$ref", "items", "x-known-values"}, keyword) {
			return field{}, fmt.Errorf("%w: keyword %q", ErrUnsupported, keyword)
		}
	}
	var property rawProperty
	if err := json.Unmarshal(described, &property); err != nil {
		return field{}, err
	}

	read := field{Key: key, Known: property.Known}
	if name, isRef := strings.CutPrefix(property.Ref, defsPrefix); isRef {
		if len(keywords) != 1 {
			return field{}, fmt.Errorf("%w: a $ref beside other keywords", ErrUnsupported)
		}
		read.Kind, read.Object = kindObject, name
		return read, r.payload(name)
	}

	typeName, nullable, err := typeOf(property.Type)
	if err != nil {
		return field{}, err
	}
	read.Nullable = nullable
	switch {
	case typeName == "string" && property.Format == "":
		read.Kind = kindText
	case typeName == "string" && property.Format == "uuid":
		read.Kind = kindUUID
	case typeName == "string" && property.Format == "date-time":
		read.Kind = kindTime
	case typeName == "integer" && property.Format == "":
		read.Kind = kindInteger
	case typeName == "array" && !nullable && property.Items != nil && string(property.Items.Type) == `"string"` && property.Items.Format == "":
		read.Kind, read.Known = kindTextList, property.Items.Known
	default:
		return field{}, fmt.Errorf("%w: type %s with format %q", ErrUnsupported, property.Type, property.Format)
	}
	if read.Known != nil && read.Kind != kindText && read.Kind != kindTextList {
		return field{}, fmt.Errorf("%w: known values on a field that is not text", ErrUnsupported)
	}
	return read, nil
}

// typeOf reads "type", which is a name or a name and "null".
func typeOf(raw json.RawMessage) (string, bool, error) {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one, false, nil
	}
	var several []string
	if json.Unmarshal(raw, &several) == nil && len(several) == 2 && several[1] == "null" {
		return several[0], true, nil
	}
	return "", false, fmt.Errorf("%w: type %s", ErrUnsupported, raw)
}
