package webhooks_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

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
