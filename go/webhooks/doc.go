// Package webhooks verifies webhooks sent by Cimento and reads them as typed
// events.
//
// Make a Webhook from an endpoint's signing secret, and either mount Handler
// or call Webhook.Verify with a request's raw body and headers:
//
//	wh, err := webhooks.NewWebhook(os.Getenv("CIMENTO_WEBHOOK_SECRET"))
//	if err != nil {
//		log.Fatal(err)
//	}
//	http.Handle("/webhooks/cimento", webhooks.Handler(wh, handle))
//
// An Event's Data is a pointer to the payload struct of its type, for a type
// switch. The EventType constants name every type this version knows. Cimento
// adds types and fields over time: an event of a type this version does not
// know is still verified and returned, with its data as JSON in RawData.
//
// The event types and payload structs are generated from the schema Cimento
// publishes for its events.
package webhooks
