# Cimento webhooks for Go

Verify [webhooks sent by Cimento](https://docs.cimento.ai/webhooks/overview) and read them as typed events.

> [!NOTE]
> This module is not released yet. Its API may still change.

```sh
go get github.com/cimento-ai/cimento-sdk/go/webhooks
```

It needs Go 1.26 or newer and has no dependencies.

## Receive webhooks

Make a `Verifier` from your endpoint's signing secret, the `whsec_...` value Cimento shows once when you create the endpoint, and mount `Handler`:

```go
verifier, err := webhooks.NewVerifier(os.Getenv("CIMENTO_WEBHOOK_SECRET"))
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

http.Handle("/webhooks/cimento", webhooks.Handler(verifier, handle,
	webhooks.OnReject(func(r *http.Request, err error) {
		log.Printf("refused a webhook from %s: %v", r.RemoteAddr, err)
	}),
))
```

`Handler` answers Cimento for you:

| Answer   | When                                                                                      |
| -------- | ----------------------------------------------------------------------------------------- |
| 204      | Your function returned nil.                                                               |
| 500      | Your function returned an error. Cimento sends the event again later.                     |
| 401      | The signature does not match, or the timestamp is more than five minutes from your clock. |
| 400      | A webhook header is missing, or the body is not an event.                                 |
| 405, 413 | The request is not a POST, or its body is over 1 MiB.                                     |

The response never says why a request was refused. `OnReject` tells you.

Cimento waits 10 seconds for the answer and may deliver an event more than once. Hand long work to something else, and de-duplicate on `event.ID`. Events can arrive out of order; `event.OccurredAt` is when each one happened.

## Events

`event.Data` is a pointer to the payload struct of the event's type, and the `EventType...` constants name every type. Fields that can be null are pointers.

Cimento adds event types and fields over time. An event of a type this version does not know is still verified and handed to you: `event.Data` is nil and `event.RawData` has its data as JSON. A field this version does not know is ignored. Text fields with a set of values today, such as `CompletedBy`, are plain strings with constants for the known values, because more may be added.

## Verify a request yourself

If `Handler` does not fit your server, call `Verify` with the body exactly as it arrived and the request's headers:

```go
body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
if err != nil {
	http.Error(w, "", http.StatusBadRequest)
	return
}
event, err := verifier.Verify(body, r.Header)
```

The signature is over the bytes Cimento sent. A body that was parsed and written out again will not verify.

`Verify` returns an error that wraps one of these, for `errors.Is`:

| Error                                      | Meaning                                                                   |
| ------------------------------------------ | ------------------------------------------------------------------------- |
| `ErrMissingHeader`                         | `webhook-id`, `webhook-timestamp` or `webhook-signature` is missing       |
| `ErrInvalidTimestamp`                      | `webhook-timestamp` is not a number of seconds                            |
| `ErrTimestampTooOld`, `ErrTimestampTooNew` | the timestamp is outside the tolerance; a replayed request fails this way |
| `ErrInvalidSignature`                      | no signature matches a secret you hold                                    |
| `ErrInvalidEvent`                          | the body is not an event                                                  |

`NewVerifier` returns `ErrMalformedSecret` for a secret that is not `whsec_` followed by base64.

## Rotating a secret

After you rotate an endpoint's secret, Cimento signs with the old and the new one for 24 hours. Give the verifier both while your deployment moves over:

```go
verifier, err := webhooks.NewVerifier(newSecret, webhooks.WithAdditionalSecrets(oldSecret))
```

## Testing your handler

`webhooks.Parse` reads a body as an event without verifying it, for tests. `WithClock` and `WithTolerance` set the clock and the tolerance a `Verifier` uses.
