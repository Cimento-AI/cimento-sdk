package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	schemaFile = "schema/webhooks/events.schema.json"
	tsScript   = "typescript/webhooks/scripts/check.mjs"
)

// checkWithTypeScript runs the same files through the TypeScript SDK, as it
// is built, and reads back what it made of each.
func checkWithTypeScript(root string, mode string, dir string) ([]result, error) {
	command := exec.Command("node", filepath.Join(root, tsScript), mode, dir)
	command.Stderr = os.Stderr
	printed, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: the TypeScript check did not run (is the package built?): %v", ErrInput, err)
	}
	var results []result
	if err := json.Unmarshal(printed, &results); err != nil {
		return nil, fmt.Errorf("%w: the TypeScript check printed something else than its results: %v", ErrInput, err)
	}
	return results, nil
}

// run checks dir and reports to out. It returns how many problems it found.
func run(root string, mode string, dir string, both bool, agreeOnly bool, quiet bool, out io.Writer) (int, error) {
	known, err := readSchema(filepath.Join(root, schemaFile))
	if err != nil {
		return 0, err
	}
	names, err := files(mode, dir)
	if err != nil {
		return 0, err
	}
	checked, err := checkWithGo(mode, dir, names)
	if err != nil {
		return 0, err
	}
	sdks := "the Go SDK"
	var other []result
	if both {
		sdks = "both SDKs"
		if other, err = checkWithTypeScript(root, mode, dir); err != nil {
			return 0, err
		}
	}
	problems := problemsIn(checked, other, known, agreeOnly)
	if _, err := io.WriteString(out, report(sdks, checked, problems, quiet)); err != nil {
		return 0, err
	}
	return len(problems), nil
}

func main() {
	root := flag.String("root", ".", "the root of the repository")
	events := flag.String("events", "", "a directory in which every file is an event body")
	deliveries := flag.String("deliveries", "", "a directory of recorded requests")
	only := flag.Bool("go-only", false, "check with the Go SDK alone")
	agree := flag.Bool("agree-only", false, "report only where the SDKs disagree, for files they are right to refuse or leave untyped")
	quiet := flag.Bool("quiet", false, "print the count and the problems, and not a line for each file")
	flag.Parse()

	mode, dir := modeEvents, *events
	if *deliveries != "" {
		mode, dir = modeDeliveries, *deliveries
	}
	if dir == "" || (*events != "" && *deliveries != "") {
		fmt.Fprintln(os.Stderr, "check: give one of -events and -deliveries")
		os.Exit(2)
	}

	problems, err := run(*root, mode, dir, !*only, *agree, *quiet, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
		if errors.Is(err, ErrInput) {
			os.Exit(2)
		}
		os.Exit(1)
	}
	if problems > 0 {
		os.Exit(1)
	}
}
