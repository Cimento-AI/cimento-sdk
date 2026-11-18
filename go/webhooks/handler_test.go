package webhooks

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// deliveryCase is a fixture that is a real delivery: its signature verifies
// and its body is an event.
func deliveryCase(t *testing.T) signatureCase {
	t.Helper()
	return signatureCases(t)[1]
}

// serve sends one request to a Handler for the fixture's secret and clock, and
// reports the response, the event the handler was given and why the request
// was refused, if it was.
func serve(t *testing.T, c signatureCase, method string, body io.Reader, handle func(event *Event) error) (*http.Response, *Event, error) {
	t.Helper()

	verifier, err := verifierFor(c)
	if err != nil {
		t.Fatal(err)
	}
	var handled *Event
	var rejected error
	handler := Handler(verifier, func(_ context.Context, event *Event) error {
		handled = event
		if handle != nil {
			return handle(event)
		}
		return nil
	}, OnReject(func(_ *http.Request, err error) { rejected = err }))

	request := httptest.NewRequest(method, "/webhooks/cimento", body)
	for name, value := range c.Headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder.Result(), handled, rejected
}

func TestHandlerAnswersNoContentAndHandsOverTheEvent(t *testing.T) {
	t.Parallel()

	c := deliveryCase(t)
	response, event, rejected := serve(t, c, http.MethodPost, strings.NewReader(c.Body), nil)
	if response.StatusCode != http.StatusNoContent || rejected != nil {
		t.Fatalf("status %d, rejected %v", response.StatusCode, rejected)
	}
	if event == nil || event.Type != EventTypeAgentHubFindingCreated {
		t.Fatalf("handled %+v", event)
	}
	if _, ok := event.Data.(*AgentHubFindingCreatedPayload); !ok {
		t.Errorf("data is %T", event.Data)
	}
	if body, _ := io.ReadAll(response.Body); len(body) != 0 {
		t.Errorf("the response has a body: %q", body)
	}
}

func TestHandlerAnswersServerErrorWhenTheEventIsNotHandled(t *testing.T) {
	t.Parallel()

	c := deliveryCase(t)
	response, event, rejected := serve(t, c, http.MethodPost, strings.NewReader(c.Body), func(*Event) error {
		return errors.New("the database is down")
	})
	if response.StatusCode != http.StatusInternalServerError || event == nil || rejected != nil {
		t.Errorf("status %d, handled %+v, rejected %v", response.StatusCode, event, rejected)
	}
	if body, _ := io.ReadAll(response.Body); len(body) != 0 {
		t.Errorf("the response repeats the error: %q", body)
	}
}

func TestHandlerRefusesEveryFixtureThatDoesNotVerify(t *testing.T) {
	t.Parallel()

	statuses := map[string]int{
		"missing_header":    http.StatusBadRequest,
		"invalid_timestamp": http.StatusBadRequest,
		"timestamp_too_old": http.StatusUnauthorized,
		"timestamp_too_new": http.StatusUnauthorized,
		"invalid_signature": http.StatusUnauthorized,
	}
	for _, c := range signatureCases(t) {
		want, refused := statuses[c.Expect]
		if !refused {
			continue
		}
		response, event, rejected := serve(t, c, http.MethodPost, strings.NewReader(c.Body), nil)
		if response.StatusCode != want {
			t.Errorf("%s: status %d, want %d", c.Name, response.StatusCode, want)
		}
		if event != nil {
			t.Errorf("%s: the event was handled", c.Name)
		}
		if !errors.Is(rejected, outcomes[c.Expect]) {
			t.Errorf("%s: rejected with %v", c.Name, rejected)
		}
		if body, _ := io.ReadAll(response.Body); len(body) != 0 {
			t.Errorf("%s: the response says why: %q", c.Name, body)
		}
	}
}

func TestHandlerRefusesATrustedBodyThatIsNotAnEvent(t *testing.T) {
	t.Parallel()

	reference := signatureCases(t)[0]
	response, event, rejected := serve(t, reference, http.MethodPost, strings.NewReader(reference.Body), nil)
	if response.StatusCode != http.StatusBadRequest || event != nil || !errors.Is(rejected, ErrInvalidEvent) {
		t.Errorf("status %d, handled %+v, rejected %v", response.StatusCode, event, rejected)
	}
}

func TestHandlerTakesOnlyPost(t *testing.T) {
	t.Parallel()

	c := deliveryCase(t)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodHead} {
		response, event, rejected := serve(t, c, method, strings.NewReader(c.Body), nil)
		if response.StatusCode != http.StatusMethodNotAllowed || response.Header.Get("Allow") != http.MethodPost || event != nil {
			t.Errorf("%s: status %d, Allow %q, handled %+v", method, response.StatusCode, response.Header.Get("Allow"), event)
		}
		if !errors.Is(rejected, ErrMethodNotAllowed) || !strings.Contains(rejected.Error(), method) {
			t.Errorf("%s: rejected with %v", method, rejected)
		}
	}
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func TestHandlerRefusesABodyItCannotReadWhole(t *testing.T) {
	t.Parallel()

	c := deliveryCase(t)
	tooLarge := bytes.Repeat([]byte("a"), maxBodyBytes+1)
	response, event, rejected := serve(t, c, http.MethodPost, bytes.NewReader(tooLarge), nil)
	var maxBytes *http.MaxBytesError
	if response.StatusCode != http.StatusRequestEntityTooLarge || event != nil || !errors.As(rejected, &maxBytes) {
		t.Errorf("too large: status %d, handled %+v, rejected %v", response.StatusCode, event, rejected)
	}

	response, event, rejected = serve(t, c, http.MethodPost, brokenBody{}, nil)
	if response.StatusCode != http.StatusBadRequest || event != nil || rejected == nil {
		t.Errorf("unreadable: status %d, handled %+v, rejected %v", response.StatusCode, event, rejected)
	}

	response, event, rejected = serve(t, c, http.MethodPost, http.NoBody, nil)
	if response.StatusCode != http.StatusUnauthorized || event != nil || !errors.Is(rejected, ErrInvalidSignature) {
		t.Errorf("empty: status %d, handled %+v, rejected %v", response.StatusCode, event, rejected)
	}
}

func TestHandlerWorksWithoutOnReject(t *testing.T) {
	t.Parallel()

	c := deliveryCase(t)
	verifier, err := verifierFor(c)
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(verifier, func(context.Context, *Event) error { return nil })
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("tampered")))
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status %d", recorder.Code)
	}
}

func TestHandlerNeedsAVerifierAndAFunction(t *testing.T) {
	t.Parallel()

	verifier, err := verifierFor(deliveryCase(t))
	if err != nil {
		t.Fatal(err)
	}
	for name, build := range map[string]func(){
		"no verifier": func() { Handler(nil, func(context.Context, *Event) error { return nil }) },
		"no function": func() { Handler(verifier, nil) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			build()
		}()
	}
}
