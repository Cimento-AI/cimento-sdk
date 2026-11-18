import { fetchHandler, Webhook } from "@cimento/webhooks";

// #region readme
export async function deliver(
  receive: (request: Request) => Promise<Response>,
  webhook: Webhook,
  event: { id: string; type: string; occurredAt: string; data: object },
): Promise<Response> {
  const body = JSON.stringify(event);
  const now = new Date();
  return receive(
    new Request("https://receiver.example.com/webhooks/cimento", {
      method: "POST",
      headers: {
        "webhook-id": event.id,
        "webhook-timestamp": String(Math.floor(now.getTime() / 1000)),
        "webhook-signature": await webhook.sign(event.id, now, body),
      },
      body,
    }),
  );
}
// #endregion readme

export { fetchHandler };
