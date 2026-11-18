// #region readme
import { fetchHandler, Webhook } from "@cimento/webhooks";

const webhook = new Webhook(process.env.CIMENTO_WEBHOOK_SECRET!);

export const receive = fetchHandler(
  webhook,
  async (event) => {
    if (!event.known) {
      // A type added to Cimento since this version. Answering 2xx is right.
      return;
    }
    switch (event.type) {
      case "training.assignment.completed":
        console.log(
          `${event.data.user.id} finished training, recorded by the ${event.data.completedBy}`,
        );
        break;
      case "phishing.simulation.clicked":
        console.log(`${event.data.user.id} clicked a simulation`);
        break;
    }
  },
  {
    onReject: (request, error) =>
      console.warn(`refused a webhook: ${error.code}`),
  },
);
// #endregion readme
