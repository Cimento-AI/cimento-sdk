package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// eventTypes reads the types the schema has.
func eventTypes(schemaPath string) (map[string]bool, error) {
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInput, err)
	}
	var schema struct {
		Events []struct {
			Title string `json:"title"`
		} `json:"oneOf"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil || len(schema.Events) == 0 {
		return nil, fmt.Errorf("%w: %s has no events: %v", ErrInput, schemaPath, err)
	}
	types := map[string]bool{}
	for _, event := range schema.Events {
		types[event.Title] = true
	}
	return types, nil
}

// A problem is one thing wrong with one file.
type problem struct {
	File string
	What string
}

// problemsIn says what is wrong with each file: the SDK refused it, it has a
// type the schema does not, it has a type the schema does and did not come
// back as that type, or the two SDKs made different things of it. other is
// nil when only one SDK was run.
func problemsIn(checked []result, other []result, known map[string]bool) []problem {
	var problems []problem
	for i, one := range checked {
		switch {
		case one.Error != "":
			problems = append(problems, problem{one.File, "refused: " + one.Error})
		case !known[one.Type]:
			problems = append(problems, problem{one.File, fmt.Sprintf("its type, %s, is not in the schema: the SDK is behind", one.Type)})
		case !one.Typed:
			problems = append(problems, problem{one.File, fmt.Sprintf("its data does not fit %s, so it was not read as its type", one.Type)})
		}
		if other == nil {
			continue
		}
		if i >= len(other) || other[i] != one {
			theirs := "nothing"
			if i < len(other) {
				described, _ := json.Marshal(other[i])
				theirs = string(described)
			}
			ours, _ := json.Marshal(one)
			problems = append(problems, problem{one.File, fmt.Sprintf("the SDKs disagree: Go %s, TypeScript %s", ours, theirs)})
		}
	}
	if other != nil && len(other) > len(checked) {
		problems = append(problems, problem{other[len(checked)].File, "TypeScript checked a file Go did not"})
	}
	return problems
}

// report is one line per file, a count by event type, and the problems.
func report(sdks string, checked []result, problems []problem) string {
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
		fmt.Fprintf(out, "%s  %-36s %s\n", mark, eventType, one.File)
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
	fmt.Fprintf(out, "\n%-36s %5s %5s\n", "event type", "files", "typed")
	for _, eventType := range types {
		fmt.Fprintf(out, "%-36s %5d %5d\n", eventType, byType[eventType].files, byType[eventType].typed)
	}

	fmt.Fprintf(out, "\n%d files, %d event types, checked with %s: %d problems\n", len(checked), len(types), sdks, len(problems))
	for _, found := range problems {
		fmt.Fprintf(out, "  %s: %s\n", found.File, found.What)
	}
	return out.String()
}
