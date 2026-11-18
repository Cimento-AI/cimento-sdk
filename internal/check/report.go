package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// schema is what the check needs of the event schema: each event type's
// payload, and each payload's fields.
type schema struct {
	payloads map[string]string
	fields   map[string]map[string]string
}

// readSchema reads the event types the schema has and the fields of each.
func readSchema(schemaPath string) (schema, error) {
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		return schema{}, fmt.Errorf("%w: %v", ErrInput, err)
	}
	var document struct {
		Events []struct {
			Title      string `json:"title"`
			Properties struct {
				Data struct {
					Ref string `json:"$ref"`
				} `json:"data"`
			} `json:"properties"`
		} `json:"oneOf"`
		Defs map[string]struct {
			Properties map[string]struct {
				Ref string `json:"$ref"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &document); err != nil || len(document.Events) == 0 {
		return schema{}, fmt.Errorf("%w: %s has no events: %v", ErrInput, schemaPath, err)
	}
	read := schema{payloads: map[string]string{}, fields: map[string]map[string]string{}}
	for _, event := range document.Events {
		read.payloads[event.Title] = strings.TrimPrefix(event.Properties.Data.Ref, "#/$defs/")
	}
	for name, payload := range document.Defs {
		read.fields[name] = map[string]string{}
		for key, field := range payload.Properties {
			read.fields[name][key] = strings.TrimPrefix(field.Ref, "#/$defs/")
		}
	}
	return read, nil
}

// known cuts data down to the fields the schema gives a payload. The Go SDK
// hands over a struct, which has no others; the TypeScript SDK hands over the
// object as it was sent, which may.
func (s schema) known(data any, payload string) any {
	object, isObject := data.(map[string]any)
	fields, described := s.fields[payload]
	if !isObject || !described {
		return data
	}
	cut := map[string]any{}
	for key, nested := range fields {
		value, present := object[key]
		if !present {
			continue
		}
		if nested != "" {
			value = s.known(value, nested)
		}
		cut[key] = value
	}
	return cut
}

// same reports whether two decoded JSON values are equal, taking two times
// that are the same instant as equal however they are written: Go hands over
// a time and TypeScript the text that was sent.
func same(a any, b any) bool {
	switch a := a.(type) {
	case map[string]any:
		b, ok := b.(map[string]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for key, value := range a {
			other, present := b[key]
			if !present || !same(value, other) {
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
			if !same(a[i], b[i]) {
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
	return a == b
}

// difference says where the two SDKs' results for one file differ, or "".
func (s schema) difference(ours result, theirs result) string {
	switch {
	case ours.File != theirs.File:
		return "they checked different files"
	case ours.Error != theirs.Error:
		return fmt.Sprintf("Go says %q and TypeScript says %q", ours.Error, theirs.Error)
	case ours.ID != theirs.ID || ours.Type != theirs.Type:
		return fmt.Sprintf("Go read %s %s and TypeScript read %s %s", ours.Type, ours.ID, theirs.Type, theirs.ID)
	case ours.Typed != theirs.Typed:
		return fmt.Sprintf("read as its type by Go: %t, by TypeScript: %t", ours.Typed, theirs.Typed)
	case !same(ours.OccurredAt, theirs.OccurredAt):
		return fmt.Sprintf("occurredAt is %s to Go and %s to TypeScript", ours.OccurredAt, theirs.OccurredAt)
	case ours.Error != "":
		return ""
	}
	var goData, tsData any
	if json.Unmarshal(ours.Data, &goData) != nil || json.Unmarshal(theirs.Data, &tsData) != nil {
		return "one SDK's data is not JSON"
	}
	if ours.Typed {
		tsData = s.known(tsData, s.payloads[ours.Type])
	}
	if !same(goData, tsData) {
		wanted, _ := json.Marshal(tsData)
		return fmt.Sprintf("data is %s to Go and %s to TypeScript", ours.Data, wanted)
	}
	return ""
}

// A problem is one thing wrong with one file.
type problem struct {
	File string
	What string
}

// problemsIn says what is wrong with each file: the SDK refused it, it has a
// type the schema does not, it has a type the schema does and did not come
// back as that type, or the two SDKs made different things of it. other is
// nil when only one SDK was run. With agreeOnly, only the last is a problem:
// that is for files an SDK is right to refuse or to leave untyped.
func problemsIn(checked []result, other []result, known schema, agreeOnly bool) []problem {
	var problems []problem
	for i, one := range checked {
		_, inSchema := known.payloads[one.Type]
		switch {
		case agreeOnly:
		case one.Error != "":
			problems = append(problems, problem{one.File, "refused: " + one.Error})
		case !inSchema:
			problems = append(problems, problem{one.File, fmt.Sprintf("its type, %s, is not in the schema: the SDK is behind", one.Type)})
		case !one.Typed:
			problems = append(problems, problem{one.File, fmt.Sprintf("its data does not fit %s, so it was not read as its type", one.Type)})
		}
		if other == nil {
			continue
		}
		if i >= len(other) {
			problems = append(problems, problem{one.File, "the SDKs disagree: TypeScript did not check it"})
		} else if difference := known.difference(one, other[i]); difference != "" {
			problems = append(problems, problem{one.File, "the SDKs disagree: " + difference})
		}
	}
	if other != nil && len(other) > len(checked) {
		problems = append(problems, problem{other[len(checked)].File, "the SDKs disagree: Go did not check it"})
	}
	return problems
}

// report is one line per file and a count by event type, unless quiet, and
// then the total and the problems.
func report(sdks string, checked []result, problems []problem, quiet bool) string {
	out := &strings.Builder{}
	wrong := map[string]bool{}
	for _, found := range problems {
		wrong[found.File] = true
	}
	type count struct{ files, typed int }
	byType := map[string]*count{}
	for _, one := range checked {
		mark, eventType := "ok  ", one.Type
		if wrong[one.File] {
			mark = "FAIL"
		}
		if one.Error != "" {
			eventType = "(" + one.Error + ")"
		}
		if !quiet {
			fmt.Fprintf(out, "%s  %-36s %s\n", mark, eventType, one.File)
		}
		if one.Error == "" {
			if byType[one.Type] == nil {
				byType[one.Type] = &count{}
			}
			byType[one.Type].files++
			if one.Typed {
				byType[one.Type].typed++
			}
		}
	}

	types := make([]string, 0, len(byType))
	for eventType := range byType {
		types = append(types, eventType)
	}
	sort.Strings(types)
	if !quiet {
		fmt.Fprintf(out, "\n%-36s %5s %5s\n", "event type", "files", "typed")
		for _, eventType := range types {
			fmt.Fprintf(out, "%-36s %5d %5d\n", eventType, byType[eventType].files, byType[eventType].typed)
		}
		fmt.Fprintln(out)
	}

	fmt.Fprintf(out, "%d files, %d event types, checked with %s: %d problems\n", len(checked), len(types), sdks, len(problems))
	for _, found := range problems {
		fmt.Fprintf(out, "  %s: %s\n", found.File, found.What)
	}
	return out.String()
}
