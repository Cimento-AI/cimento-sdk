// #region readme
import { createServer } from "node:http";

import { Webhook, WebhookVerificationError } from "@cimento/webhooks";

const webhook = new Webhook(process.env.CIMENTO_WEBHOOK_SECRET!);

export const server = createServer(async (request, response) => {
  // The signature is over the body as it was sent, so collect the bytes
  // and verify them before anything parses them.
  const chunks: Buffer[] = [];
  for await (const chunk of request) {
    chunks.push(chunk as Buffer);
  }
  try {
    const event = await webhook.verify(Buffer.concat(chunks), request.headers);
    console.log(event.id, event.type, event.occurredAt);
    response.writeHead(204).end();
  } catch (error) {
    if (!(error instanceof WebhookVerificationError)) {
      throw error;
    }
    const unauthorized = ["invalid_signature", "timestamp_too_old"];
    response.writeHead(unauthorized.includes(error.code) ? 401 : 400).end();
  }
});
// #endregion readme
