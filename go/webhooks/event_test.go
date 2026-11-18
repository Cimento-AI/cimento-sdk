package webhooks

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseReadsEveryValidFixtureAsItsTypeAndLosesNothing(t *testing.T) {
	t.Parallel()

	types := map[string]bool{}
	for name, body := range eventFixtures(t, "valid") {
		event, err := Parse(body)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		var sent struct {
			ID         string          `json:"id"`
			OccurredAt string          `json:"occurredAt"`
			Data       json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &sent); err != nil {
			t.Fatal(err)
		}
		if event.Type != strings.TrimSuffix(name, ".nulls") || event.ID != sent.ID {
			t.Errorf("%s: read as %s %s", name, event.Type, event.ID)
		}
		if !sameJSON(sent.OccurredAt, event.OccurredAt.Format("2006-01-02T15:04:05.999999999Z07:00")) {
			t.Errorf("%s: occurredAt %s read as %s", name, sent.OccurredAt, event.OccurredAt)
		}
		types[event.Type] = true

		want, _ := newPayload(event.Type)
		if event.Data == nil || reflect.TypeOf(event.Data) != reflect.TypeOf(want) {
			t.Errorf("%s: data is %T, want %T", name, event.Data, want)
			continue
		}
		written, err := json.Marshal(event.Data)
		if err != nil {
			t.Fatal(err)
		}
		var before, after, raw any
		for from, into := range map[*any][]byte{&before: sent.Data, &after: written, &raw: event.RawData} {
			if err := json.Unmarshal(into, from); err != nil {
				t.Fatal(err)
			}
		}
		if !sameJSON(before, after) {
			t.Errorf("%s changed on its way through %T:\n got %s\nfrom %s", name, event.Data, written, sent.Data)
		}
		if !reflect.DeepEqual(before, raw) {
			t.Errorf("%s: RawData is not the data that was sent", name)
		}
	}

	var schema struct {
		Events []json.RawMessage `json:"oneOf"`
	}
	raw, err := os.ReadFile("../../schema/webhooks/events.schema.json")
	if err != nil || json.Unmarshal(raw, &schema) != nil {
		t.Fatalf("read schema: %v", err)
	}
	if len(types) != len(schema.Events) {
		t.Errorf("fixtures were read as %d event types, and the schema has %d", len(types), len(schema.Events))
	}
}

func TestParseAcceptsWhatItDoesNotKnow(t *testing.T) {
	t.Parallel()

	open := eventFixtures(t, "open")
	for name, body := range open {
		event, err := Parse(body)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if event.ID == "" || event.Type == "" || event.OccurredAt.IsZero() || len(event.RawData) == 0 {
			t.Errorf("%s: read as %+v", name, event)
		}
		// The fixtures' rule: data is handed over as its type unless the
		// type is unknown or the file is one whose data does not fit.
		_, known := newPayload(event.Type)
		if typed := known && !strings.HasPrefix(name, "known-type-"); typed != (event.Data != nil) {
			t.Errorf("%s: data is %T", name, event.Data)
		}
	}

	parse := func(name string) *Event {
		body, ok := open[name]
		if !ok {
			t.Fatalf("no open/%s.json", name)
		}
		event, err := Parse(body)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return event
	}

	unknown := parse("unknown-type")
	if unknown.Type != "billing.invoice.paid" || unknown.Data != nil || !strings.Contains(string(unknown.RawData), `"amount"`) {
		t.Errorf("an unknown type: %+v", unknown)
	}
	misshapen := parse("known-type-wrong-shape")
	if misshapen.Type != EventTypePhishingRoundStarted || misshapen.Data != nil || !strings.Contains(string(misshapen.RawData), "about two hundred") {
		t.Errorf("a known type that does not fit: %+v", misshapen)
	}
	for name, eventType := range map[string]string{
		"known-type-missing-field":         EventTypeTrainingCampaignLaunched,
		"known-type-null-field":            EventTypeTrainingAssignmentCompleted,
		"known-type-null-item":             EventTypeAgentHubSettingsUpdated,
		"known-type-comma-time":            EventTypeTrainingAssignmentCompleted,
		"known-type-short-hour":            EventTypeTrainingExemptionCreated,
		"known-type-field-in-another-case": EventTypePhishingRoundStarted,
	} {
		if unfit := parse(name); unfit.Type != eventType || unfit.Data != nil || len(unfit.RawData) == 0 {
			t.Errorf("%s: %+v", name, unfit)
		}
	}
	extra, ok := parse("unknown-fields").Data.(*TrainingAssignmentCompletedPayload)
	if !ok || extra.CompletedBy != TrainingAssignmentCompletedPayloadCompletedByLearner || extra.User.Email == nil {
		t.Errorf("unknown fields: %+v", extra)
	}
	values, ok := parse("unknown-values").Data.(*AgentHubFindingCreatedPayload)
	if !ok || values.Kind != "drift" || values.Response != "quarantine" {
		t.Errorf("unknown values: %+v", values)
	}
	// A key in another case is another key, not the field's.
	cased, ok := parse("unknown-field-in-another-case").Data.(*TrainingAssignmentCompletedPayload)
	if !ok || cased.CompletedBy != "learner" || cased.User.Email == nil || *cased.User.Email != "ada.lovelace@example.com" {
		t.Errorf("a field's key in another case: %+v", cased)
	}
	keys, ok := parse("unknown-changed-key").Data.(*AgentHubSettingsUpdatedPayload)
	if !ok || len(keys.ChangedKeys) != 2 || keys.ChangedKeys[1] != "alerts.daily_digest" {
		t.Errorf("an unknown changed key: %+v", keys)
	}
	whole, nanos := parse("occurred-at-whole-seconds"), parse("occurred-at-nanoseconds")
	if whole.OccurredAt.Nanosecond() != 0 || nanos.OccurredAt.Nanosecond() != 123456789 || whole.OccurredAt.Unix() != nanos.OccurredAt.Unix() {
		t.Errorf("times: %s and %s", whole.OccurredAt, nanos.OccurredAt)
	}
}

func TestParseRefusesWhatIsNotAnEvent(t *testing.T) {
	t.Parallel()

	for name, body := range eventFixtures(t, "invalid") {
		if event, err := Parse(body); event != nil || !errors.Is(err, ErrInvalidEvent) {
			t.Errorf("%s: got %+v, %v", name, event, err)
		}
	}
	if event, err := Parse(nil); event != nil || !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("no body: got %+v, %v", event, err)
	}
}

func TestEventsThatShareAPayloadAreToldApartByType(t *testing.T) {
	t.Parallel()

	valid := eventFixtures(t, "valid")
	clicked, err := Parse(valid[EventTypePhishingSimulationClicked])
	if err != nil {
		t.Fatal(err)
	}
	reported, err := Parse(valid[EventTypePhishingSimulationReported])
	if err != nil {
		t.Fatal(err)
	}
	if reflect.TypeOf(clicked.Data) != reflect.TypeOf(reported.Data) || clicked.Type == reported.Type {
		t.Errorf("%s is %T and %s is %T", clicked.Type, clicked.Data, reported.Type, reported.Data)
	}
}
