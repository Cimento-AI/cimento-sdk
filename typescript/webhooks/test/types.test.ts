import { expectTypeOf, test } from "vitest";

import {
  parseEvent,
  type EventPayloads,
  type EventType,
  type KnownWebhookEvent,
  type TrainingAssignmentCompletedPayload,
  type UnknownWebhookEvent,
  type WebhookEvent,
} from "../src/index";
import { eventFixtures } from "./fixtures";

test("known narrows an event, and then type narrows its data", () => {
  const event = parseEvent(
    eventFixtures("valid").get("training.assignment.completed")!,
  );
  expectTypeOf(event).toEqualTypeOf<WebhookEvent>();

  if (!event.known) {
    expectTypeOf(event).toEqualTypeOf<UnknownWebhookEvent>();
    expectTypeOf(event.type).toEqualTypeOf<string>();
    expectTypeOf(event.data).toEqualTypeOf<Record<string, unknown>>();
    return;
  }
  expectTypeOf(event).toEqualTypeOf<KnownWebhookEvent>();
  expectTypeOf(event.type).toEqualTypeOf<EventType>();

  switch (event.type) {
    case "training.assignment.completed":
      expectTypeOf(
        event.data,
      ).toEqualTypeOf<TrainingAssignmentCompletedPayload>();
      expectTypeOf(event.data.user.email).toEqualTypeOf<string | null>();
      // Known values are suggested, and any other string is still allowed.
      expectTypeOf<"learner">().toExtend<typeof event.data.completedBy>();
      expectTypeOf<"something new">().toExtend<typeof event.data.completedBy>();
      break;
    case "phishing.simulation.clicked":
    case "phishing.simulation.reported":
      expectTypeOf(event.data).toEqualTypeOf<
        EventPayloads["phishing.simulation.clicked"]
      >();
      break;
    case "agent_hub.settings.updated":
      expectTypeOf(event.data.changedKeys).toBeArray();
      break;
  }
});

test("an unknown event's data is not a payload", () => {
  expectTypeOf<
    UnknownWebhookEvent["data"]
  >().not.toExtend<TrainingAssignmentCompletedPayload>();
  // @ts-expect-error a type that is not one of the event types
  const missing: EventPayloads["training.assignment.exploded"] = null;
  void missing;
});
