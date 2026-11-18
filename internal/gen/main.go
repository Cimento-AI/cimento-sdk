// Command gen writes the SDKs' event types from the event schema.
//
//	go run ./gen -root ..
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	schemaFile   = "schema/webhooks/events.schema.json"
	fixturesGlob = "fixtures/webhooks/events/valid/*.json"
	goFile       = "go/webhooks/events_gen.go"
	tsFile       = "typescript/webhooks/src/events.gen.ts"
	tsShapesFile = "typescript/webhooks/src/shapes.gen.ts"
	tsTestFile   = "typescript/webhooks/test/fixtures.gen.test.ts"
)

func main() {
	root := flag.String("root", ".", "the root of the repository")
	flag.Parse()
	if err := generate(*root); err != nil {
		log.Fatal(err)
	}
}

func generate(root string) error {
	raw, err := os.ReadFile(filepath.Join(root, schemaFile))
	if err != nil {
		return err
	}
	schema, err := readModel(raw)
	if err != nil {
		return err
	}
	paths, err := filepath.Glob(filepath.Join(root, fixturesGlob))
	if err != nil {
		return err
	}
	fixtures := make([]string, 0, len(paths))
	for _, path := range paths {
		fixtures = append(fixtures, strings.TrimSuffix(filepath.Base(path), ".json"))
	}
	sort.Strings(fixtures)

	goTypes, err := goSource(schema)
	if err != nil {
		return err
	}
	tsTypes, err := tsSource(schema)
	if err != nil {
		return err
	}
	tsTest, err := tsFixtureTest(schema, fixtures)
	if err != nil {
		return err
	}
	for file, content := range map[string][]byte{goFile: goTypes, tsFile: tsTypes, tsShapesFile: tsShapes(schema), tsTestFile: tsTest} {
		path := filepath.Join(root, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", file, err)
		}
	}
	return nil
}
