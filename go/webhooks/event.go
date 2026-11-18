package webhooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

// An Event is one thing that happened in Cimento.
type Event struct {
	// ID is the same on every attempt to deliver the event, so it is what
	// to de-duplicate on.
	ID string
	// Type is one of the EventType constants, or a type added to Cimento
	// since this version of the package.
	Type string
	// OccurredAt is when it happened, which is not when it was sent.
	// Events can arrive out of order; sort by this.
	OccurredAt time.Time
	// Data is a pointer to the payload struct of the event's type, such as
	// *TrainingAssignmentCompletedPayload. Several types share a struct;
	// tell them apart by Type.
	//
	// It is nil for a type this version of the package does not know, and
	// for data that does not fit the type: a field is missing, is null where
	// it may not be, or is of another kind. RawData has it then.
	Data any
	// RawData is the event's data as JSON, exactly as it was sent.
	RawData json.RawMessage
}

// Parse reads body as an event without checking who sent it. It is for tests
// and for bodies already verified; a receiver should call Webhook.Verify.
//
// An event of a type this version of the package does not know is not an
// error, and nor is a field it does not know.
func Parse(body []byte) (*Event, error) {
	var envelope struct {
		ID         *string         `json:"id"`
		Type       *string         `json:"type"`
		OccurredAt *string         `json:"occurredAt"`
		Data       json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidEvent, err)
	}
	switch {
	case envelope.ID == nil || *envelope.ID == "":
		return nil, fmt.Errorf("%w: it has no id", ErrInvalidEvent)
	case envelope.Type == nil || *envelope.Type == "":
		return nil, fmt.Errorf("%w: it has no type", ErrInvalidEvent)
	case envelope.OccurredAt == nil:
		return nil, fmt.Errorf("%w: it has no occurredAt", ErrInvalidEvent)
	case !bytes.HasPrefix(bytes.TrimSpace(envelope.Data), []byte("{")):
		return nil, fmt.Errorf("%w: its data is not an object", ErrInvalidEvent)
	}
	occurredAt, err := time.Parse(time.RFC3339, *envelope.OccurredAt)
	if err != nil {
		return nil, fmt.Errorf("%w: its occurredAt is not a time", ErrInvalidEvent)
	}

	event := &Event{ID: *envelope.ID, Type: *envelope.Type, OccurredAt: occurredAt, RawData: envelope.Data}
	payload, known := newPayload(event.Type)
	if known && fits(envelope.Data, reflect.TypeOf(payload).Elem()) && json.Unmarshal(envelope.Data, payload) == nil {
		event.Data = payload
	}
	return event, nil
}

var timeType = reflect.TypeFor[time.Time]()

// fits reports whether data has every field of a payload struct, with null
// only where the field is a pointer. Decoding alone would not say: it leaves
// a field that is missing or null at its zero value, which a receiver could
// not tell from a value Cimento sent.
func fits(data json.RawMessage, payload reflect.Type) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return false
	}
	for i := range payload.NumField() {
		field := payload.Field(i)
		value, present := fields[field.Tag.Get("json")]
		if !present {
			return false
		}
		isNull := string(bytes.TrimSpace(value)) == "null"
		switch {
		case isNull:
			if field.Type.Kind() != reflect.Pointer {
				return false
			}
		case field.Type.Kind() == reflect.Struct && field.Type != timeType:
			if !fits(value, field.Type) {
				return false
			}
		case field.Type.Kind() == reflect.Slice:
			var items []json.RawMessage
			if json.Unmarshal(value, &items) != nil {
				return false
			}
			for _, item := range items {
				if string(bytes.TrimSpace(item)) == "null" {
					return false
				}
			}
		}
	}
	return true
}
