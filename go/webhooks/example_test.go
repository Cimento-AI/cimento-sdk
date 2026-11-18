package webhooks_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"time"

	"github.com/cimento-ai/cimento-sdk/go/webhooks"
)

func ExampleHandler() {
	wh, err := webhooks.NewWebhook(os.Getenv("CIMENTO_WEBHOOK_SECRET"))
	if err != nil {
		log.Fatal(err)
	}

	handle := func(ctx context.Context, event *webhooks.Event) error {
		switch data := event.Data.(type) {
		case *webhooks.TrainingAssignmentCompletedPayload:
			log.Printf("%s finished training, recorded by the %s", data.User.ID, data.CompletedBy)
		case *webhooks.PhishingSimulationPayload:
			// Five event types share this payload.
			if event.Type == webhooks.EventTypePhishingSimulationClicked {
				log.Printf("%s clicked a simulation", data.User.ID)
			}
		default:
			// A type added to Cimento since this version, or one this
			// receiver has no use for. Answering 2xx is right for both.
		}
		return nil
	}

	http.Handle("/webhooks/cimento", webhooks.Handler(wh, handle,
		webhooks.OnReject(func(r *http.Request, err error) {
			log.Printf("refused a webhook from %s: %v", r.RemoteAddr, err)
		}),
	))
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func ExampleWebhook_Verify() {
	wh, err := webhooks.NewWebhook(os.Getenv("CIMENTO_WEBHOOK_SECRET"))
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/webhooks/cimento", func(w http.ResponseWriter, r *http.Request) {
		// The signature is over the body as it was sent, so read it whole
		// and verify it before anything parses it.
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "", http.StatusBadRequest)
			return
		}
		event, err := wh.Verify(body, r.Header)
		switch {
		case errors.Is(err, webhooks.ErrInvalidSignature), errors.Is(err, webhooks.ErrTimestampTooOld):
			http.Error(w, "", http.StatusUnauthorized)
			return
		case err != nil:
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		fmt.Println(event.ID, event.Type, event.OccurredAt)
		w.WriteHeader(http.StatusNoContent)
	})
}

// While an endpoint's secret is being rotated, Cimento signs with the old and
// the new secret for 24 hours. A Webhook that holds both accepts either.
func ExampleWithAdditionalSecrets() {
	wh, err := webhooks.NewWebhook(
		os.Getenv("CIMENTO_WEBHOOK_SECRET"),
		webhooks.WithAdditionalSecrets(os.Getenv("CIMENTO_WEBHOOK_PREVIOUS_SECRET")),
	)
	if err != nil {
		log.Fatal(err)
	}
	_ = wh
}

// To test a receiver, sign a body with a secret of your own and send it to
// the handler, as Cimento would.
func ExampleWebhook_Sign() {
	wh, err := webhooks.NewWebhook("whsec_dGVzdGluZy1vbmx5LW5vdC1hLXJlYWwtc2VjcmV0")
	if err != nil {
		log.Fatal(err)
	}
	handler := webhooks.Handler(wh, func(ctx context.Context, event *webhooks.Event) error {
		fmt.Println("handled", event.Type)
		return nil
	})

	id, now := "c7a1e2b3-4d5f-4a6b-8c9d-0e1f2a3b4c5d", time.Now()
	body := []byte(`{"id":"` + id + `","type":"webhook.test","occurredAt":"2026-11-12T16:50:00.124Z",` +
		`"data":{"endpointId":"3b9d5c2e-7f1a-4e8b-a6c4-9d0e1f2a3b4c"}}`)
	signature, err := wh.Sign(id, now, body)
	if err != nil {
		log.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/webhooks/cimento", bytes.NewReader(body))
	request.Header.Set("webhook-id", id)
	request.Header.Set("webhook-timestamp", strconv.FormatInt(now.Unix(), 10))
	request.Header.Set("webhook-signature", signature)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	fmt.Println(response.Code)
	// Output:
	// handled webhook.test
	// 204
}

func ExampleParse() {
	body := []byte(`{
		"id": "c7a1e2b3-4d5f-4a6b-8c9d-0e1f2a3b4c5d",
		"type": "webhook.test",
		"occurredAt": "2026-11-12T16:50:00.124Z",
		"data": {"endpointId": "3b9d5c2e-7f1a-4e8b-a6c4-9d0e1f2a3b4c"}
	}`)

	event, err := webhooks.Parse(body)
	if err != nil {
		log.Fatal(err)
	}
	test := event.Data.(*webhooks.TestPayload)
	fmt.Println(event.Type, test.EndpointID)
	// Output: webhook.test 3b9d5c2e-7f1a-4e8b-a6c4-9d0e1f2a3b4c
}
