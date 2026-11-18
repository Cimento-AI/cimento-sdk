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
	// Into a map, and not a struct: encoding/json matches a struct's keys
	// without regard to case, so "ID" would be read as the id. The contract's
	// keys are spelt one way, and the TypeScript SDK reads only that spelling.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidEvent, err)
	}
	text := func(key string) (string, bool) {
		var value *string
		if json.Unmarshal(envelope[key], &value) != nil || value == nil {
			return "", false
		}
		return *value, true
	}
	id, hasID := text("id")
	eventType, hasType := text("type")
	sent, hasTime := text("occurredAt")
	data := envelope["data"]
	switch {
	case !hasID || id == "":
		return nil, fmt.Errorf("%w: it has no id", ErrInvalidEvent)
	case !hasType || eventType == "":
		return nil, fmt.Errorf("%w: it has no type", ErrInvalidEvent)
	case !hasTime:
		return nil, fmt.Errorf("%w: it has no occurredAt", ErrInvalidEvent)
	case !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")):
		return nil, fmt.Errorf("%w: its data is not an object", ErrInvalidEvent)
	}
	occurredAt, isTime := parseTime(sent)
	if !isTime {
		return nil, fmt.Errorf("%w: its occurredAt is not a time", ErrInvalidEvent)
	}

	event := &Event{ID: id, Type: eventType, OccurredAt: occurredAt, RawData: data}
	if payload, known := newPayload(event.Type); known {
		if fitted, ok := fit(data, reflect.TypeOf(payload).Elem()); ok && json.Unmarshal(fitted, payload) == nil {
			event.Data = payload
		}
	}
	return event, nil
}

var timeType = reflect.TypeFor[time.Time]()

// parseTime reads an RFC 3339 time as the specification writes one, and as
// the TypeScript SDK reads one. The standard library is looser, and not the
// same from one Go release to the next: it has taken a comma before the
// fraction and an hour of one digit.
func parseTime(text string) (time.Time, bool) {
	digits := func(from int, to int) bool {
		for i := from; i < to; i++ {
			if text[i] < '0' || text[i] > '9' {
				return false
			}
		}
		return true
	}
	// 2006-01-02T15:04:05, then an optional fraction, then Z or +07:00.
	const date = len("2006-01-02T15:04:05")
	if len(text) < date+1 || !digits(0, 4) || text[4] != '-' || !digits(5, 7) || text[7] != '-' || !digits(8, 10) ||
		text[10] != 'T' || !digits(11, 13) || text[13] != ':' || !digits(14, 16) || text[16] != ':' || !digits(17, 19) {
		return time.Time{}, false
	}
	zone := date
	if text[zone] == '.' {
		zone++
		for zone < len(text) && text[zone] >= '0' && text[zone] <= '9' {
			zone++
		}
		if zone == date+1 {
			return time.Time{}, false
		}
	}
	offset := text[zone:]
	numeric := len(offset) == len("+07:00") && (offset[0] == '+' || offset[0] == '-') &&
		digits(zone+1, zone+3) && offset[3] == ':' && digits(zone+4, zone+6)
	if offset != "Z" && !numeric {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	return parsed, err == nil
}

// fit returns data cut down to the fields of a payload struct, under exactly
// the keys the struct names, and whether data has every one of them with null
// only where the field is a pointer and with times written as parseTime reads
// them.
//
// Decoding data itself would say neither. It leaves a field that is missing
// or null at its zero value, which a receiver could not tell from a value
// Cimento sent, and it would take a key spelt in another case for the field.
func fit(data json.RawMessage, payload reflect.Type) (json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return nil, false
	}
	fitted := make(map[string]json.RawMessage, payload.NumField())
	for i := range payload.NumField() {
		field := payload.Field(i)
		key := field.Tag.Get("json")
		value, present := fields[key]
		if !present {
			return nil, false
		}
		isNull := string(bytes.TrimSpace(value)) == "null"
		switch {
		case isNull:
			if field.Type.Kind() != reflect.Pointer {
				return nil, false
			}
		case field.Type == timeType || field.Type == reflect.PointerTo(timeType):
			var text string
			if json.Unmarshal(value, &text) != nil {
				return nil, false
			}
			if _, isTime := parseTime(text); !isTime {
				return nil, false
			}
		case field.Type.Kind() == reflect.Struct:
			nested, ok := fit(value, field.Type)
			if !ok {
				return nil, false
			}
			value = nested
		case field.Type.Kind() == reflect.Slice:
			var items []json.RawMessage
			if json.Unmarshal(value, &items) != nil {
				return nil, false
			}
			for _, item := range items {
				if string(bytes.TrimSpace(item)) == "null" {
					return nil, false
				}
			}
		}
		fitted[key] = value
	}
	encoded, err := json.Marshal(fitted)
	return encoded, err == nil
}
