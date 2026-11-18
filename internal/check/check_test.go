package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	root     = "../.."
	fixtures = root + "/fixtures/webhooks"
)

// checked runs the Go half over dir and returns the files it found a problem
// with, and what it printed.
func checked(t *testing.T, mode string, dir string) ([]string, string) {
	t.Helper()

	var out bytes.Buffer
	if _, err := run(root, mode, dir, false, false, false, &out); err != nil {
		t.Fatal(err)
	}
	var wrong []string
	for _, line := range strings.Split(out.String(), "\n") {
		if file, isProblem := strings.CutPrefix(line, "  "); isProblem && strings.Contains(file, ": ") {
			wrong = append(wrong, strings.SplitN(file, ": ", 2)[0])
		}
	}
	sort.Strings(wrong)
	return wrong, out.String()
}

func TestEveryValidFixtureIsReadAsItsType(t *testing.T) {
	wrong, printed := checked(t, modeEvents, fixtures+"/events/valid")
	if len(wrong) != 0 {
		t.Errorf("problems with %v", wrong)
	}
	if !strings.Contains(printed, "64 files, 34 event types, checked with the Go SDK: 0 problems") {
		t.Errorf("printed:\n%s", printed)
	}
}

func TestTheOpenFixturesThatAreNotTypedAreTheProblems(t *testing.T) {
	wrong, printed := checked(t, modeEvents, fixtures+"/events/open")

	// What the fixtures' README says an SDK does not hand over as its type.
	var want []string
	entries, err := os.ReadDir(fixtures + "/events/open")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "known-type-") || entry.Name() == "unknown-type.json" {
			want = append(want, entry.Name())
		}
	}
	if strings.Join(wrong, " ") != strings.Join(want, " ") {
		t.Errorf("problems with %v, want %v", wrong, want)
	}
	if !strings.Contains(printed, "unknown-type.json: its type, billing.invoice.paid, is not in the schema") {
		t.Errorf("printed:\n%s", printed)
	}
}

func TestEveryInvalidFixtureIsAProblem(t *testing.T) {
	wrong, _ := checked(t, modeEvents, fixtures+"/events/invalid")
	entries, err := os.ReadDir(fixtures + "/events/invalid")
	if err != nil {
		t.Fatal(err)
	}
	if len(wrong) != len(entries) {
		t.Errorf("%d problems for %d files", len(wrong), len(entries))
	}
}

type signatureCase struct {
	Name    string            `json:"name"`
	Secrets []string          `json:"secrets"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Now     int64             `json:"now"`
	Expect  string            `json:"expect"`
}

// record writes a signature fixture as a recorded request named name.
func record(t *testing.T, dir string, name string, c signatureCase) {
	t.Helper()

	headers := map[string][]string{}
	for header, value := range c.Headers {
		headers[header] = []string{value}
	}
	described, err := json.Marshal(map[string]any{"headers": headers, "receivedAt": time.Unix(c.Now, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	for file, content := range map[string][]byte{name + ".body": []byte(c.Body), name + ".json": described} {
		if err := os.WriteFile(filepath.Join(dir, file), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func deliveries(t *testing.T) []signatureCase {
	t.Helper()

	raw, err := os.ReadFile(fixtures + "/signatures.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []signatureCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	var real []signatureCase
	for _, c := range file.Cases {
		if c.Expect == "ok" && strings.Contains(c.Name, "delivery") {
			real = append(real, c)
		}
	}
	if len(real) < 2 {
		t.Fatalf("only %d fixtures hold a delivery", len(real))
	}
	return real
}

func TestARecordedDeliveryIsVerifiedAsItWasWhenItArrived(t *testing.T) {
	real := deliveries(t)
	dir := t.TempDir()
	record(t, dir, "0001-as-sent", real[0])

	changed := real[0]
	changed.Body = strings.Replace(changed.Body, "block", "nudge", 1)
	record(t, dir, "0002-body-changed", changed)

	late := real[0]
	late.Now += 301
	record(t, dir, "0003-arrived-late", late)

	// A file that is not a recorded request is not checked as one.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a delivery"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv(secretsEnv, " "+real[0].Secrets[0]+" ")
	wrong, printed := checked(t, modeDeliveries, dir)
	if strings.Join(wrong, " ") != "0002-body-changed.body 0003-arrived-late.body" {
		t.Errorf("problems with %v\n%s", wrong, printed)
	}
	for _, want := range []string{
		"ok    agent_hub.finding.created", "0002-body-changed.body: refused: invalid_signature",
		"0003-arrived-late.body: refused: timestamp_too_old", "3 files, 1 event types",
	} {
		if !strings.Contains(printed, want) {
			t.Errorf("did not print %q:\n%s", want, printed)
		}
	}
	if strings.Contains(printed, real[0].Secrets[0]) {
		t.Error("the report has the secret in it")
	}

	t.Setenv(secretsEnv, "whsec_"+strings.Repeat("QUJD", 8)+","+real[0].Secrets[0])
	if wrong, _ := checked(t, modeDeliveries, dir); len(wrong) != 2 {
		t.Errorf("with the right secret second: problems with %v", wrong)
	}
	t.Setenv(secretsEnv, "whsec_"+strings.Repeat("QUJD", 8))
	if wrong, _ := checked(t, modeDeliveries, dir); len(wrong) != 3 {
		t.Errorf("with the wrong secret: problems with %v", wrong)
	}
}

func TestWhatCannotBeCheckedIsAnErrorAndNotAPass(t *testing.T) {
	real := deliveries(t)
	var out bytes.Buffer

	empty := t.TempDir()
	if _, err := run(root, modeEvents, empty, false, false, false, &out); !errors.Is(err, ErrInput) {
		t.Errorf("an empty directory: %v", err)
	}
	if _, err := run(root, modeEvents, filepath.Join(empty, "missing"), false, false, false, &out); !errors.Is(err, ErrInput) {
		t.Errorf("a directory that is not there: %v", err)
	}

	recordedDir := t.TempDir()
	record(t, recordedDir, "0001-as-sent", real[0])
	t.Setenv(secretsEnv, "")
	if _, err := run(root, modeDeliveries, recordedDir, false, false, false, &out); !errors.Is(err, ErrInput) {
		t.Errorf("no secrets: %v", err)
	}
	t.Setenv(secretsEnv, real[0].Secrets[0])
	if err := os.Remove(filepath.Join(recordedDir, "0001-as-sent.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := run(root, modeDeliveries, recordedDir, false, false, false, &out); !errors.Is(err, ErrInput) {
		t.Errorf("a body with no .json beside it: %v", err)
	}
	if _, err := run(filepath.Join(root, "internal"), modeEvents, fixtures+"/events/valid", false, false, false, &out); !errors.Is(err, ErrInput) {
		t.Errorf("a root with no schema: %v", err)
	}
}

func TestTheSDKsDisagreeingIsAProblemOnTheFileTheyDisagreeOn(t *testing.T) {
	known, err := readSchema(root + "/" + schemaFile)
	if err != nil {
		t.Fatal(err)
	}
	completed := func(data string) result {
		return result{
			File: "b.json", ID: "2", Type: "training.assignment.completed", Typed: true,
			OccurredAt: "2026-11-12T15:02:11.902Z", Data: json.RawMessage(data),
		}
	}
	// What Go hands over: the struct's fields, and times as it writes them.
	fromGo := `{"assignmentId":"a","campaignId":"c","user":{"id":"u","email":null},"completedAt":"2026-11-12T15:02:11.5Z","completedBy":"learner"}`
	// What TypeScript hands over: the object as sent, unknown fields and all.
	fromTypeScript := `{"assignmentId":"a","campaignId":"c","user":{"id":"u","email":null,"department":"x"},"completedAt":"2026-11-12T15:02:11.500Z","completedBy":"learner","score":92}`
	ours := []result{{File: "a.json", ID: "1", Type: "webhook.test", Typed: true, OccurredAt: "2026-11-12T16:50:00Z", Data: json.RawMessage(`{"endpointId":"e"}`)}, completed(fromGo)}

	theirs := []result{ours[0], completed(fromTypeScript)}
	theirs[1].OccurredAt = "2026-11-12T15:02:11.902000Z"
	if problems := problemsIn(ours, theirs, known, false); len(problems) != 0 {
		t.Errorf("the same event, written each SDK's way: %v", problems)
	}

	for name, differ := range map[string]func(theirs *result){
		"a field read differently": func(theirs *result) {
			theirs.Data = json.RawMessage(strings.Replace(fromTypeScript, `"learner"`, `"admin"`, 1))
		},
		"a nested field read differently": func(theirs *result) {
			theirs.Data = json.RawMessage(strings.Replace(fromTypeScript, `"email":null`, `"email":"a@example.com"`, 1))
		},
		"a time read differently": func(theirs *result) {
			theirs.Data = json.RawMessage(strings.Replace(fromTypeScript, "11.500Z", "12.500Z", 1))
		},
		"a field TypeScript lacks": func(theirs *result) {
			theirs.Data = json.RawMessage(strings.Replace(fromTypeScript, `"campaignId":"c",`, ``, 1))
		},
		"another occurredAt":             func(theirs *result) { theirs.OccurredAt = "2026-11-12T15:02:12.902Z" },
		"typed by one and not the other": func(theirs *result) { theirs.Typed = false },
		"another id":                     func(theirs *result) { theirs.ID = "3" },
		"refused by one":                 func(theirs *result) { *theirs = result{File: "b.json", Error: "invalid_event"} },
	} {
		theirs := []result{ours[0], completed(fromTypeScript)}
		differ(&theirs[1])
		problems := problemsIn(ours, theirs, known, false)
		if len(problems) != 1 || problems[0].File != "b.json" || !strings.Contains(problems[0].What, "the SDKs disagree") {
			t.Errorf("%s: %v", name, problems)
		}
	}

	if problems := problemsIn(ours, theirs[:1], known, false); len(problems) != 1 || problems[0].File != "b.json" {
		t.Errorf("TypeScript checked fewer files: %v", problems)
	}
	if problems := problemsIn(ours[:1], theirs, known, false); len(problems) != 1 || problems[0].File != "b.json" {
		t.Errorf("TypeScript checked more files: %v", problems)
	}
}

func TestAgreeOnlyLeavesOutWhatBothSDKsAreRightToRefuse(t *testing.T) {
	known, err := readSchema(root + "/" + schemaFile)
	if err != nil {
		t.Fatal(err)
	}
	refused := []result{{File: "a.json", Error: "invalid_event"}, {File: "b.json", ID: "1", Type: "billing.invoice.paid", OccurredAt: "2026-11-12T16:50:00Z", Data: json.RawMessage(`{}`)}}
	if problems := problemsIn(refused, refused, known, false); len(problems) != 2 {
		t.Errorf("without agree-only: %v", problems)
	}
	if problems := problemsIn(refused, refused, known, true); len(problems) != 0 {
		t.Errorf("with agree-only: %v", problems)
	}
	other := []result{refused[0], refused[1]}
	other[0].Error = "missing_header"
	if problems := problemsIn(refused, other, known, true); len(problems) != 1 || problems[0].File != "a.json" {
		t.Errorf("refused for different reasons: %v", problems)
	}
}
