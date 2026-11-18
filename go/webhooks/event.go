package webhooks

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	// for data that does not fit the type's struct. RawData has it then.
	Data any
	// RawData is the event's data as JSON, exactly as it was sent.
	RawData json.RawMessage
}

// Parse reads body as an event without checking who sent it. It is for tests
// and for bodies already verified; a receiver should call Verifier.Verify.
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
	if payload, known := newPayload(event.Type); known && json.Unmarshal(envelope.Data, payload) == nil {
		event.Data = payload
	}
	return event, nil
}
