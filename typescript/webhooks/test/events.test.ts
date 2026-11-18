import { readFileSync } from "node:fs";

import { describe, expect, test } from "vitest";

import { eventTypes, parseEvent } from "../src/index";
import { codeOf, eventFixtures } from "./fixtures";

const decode = (body: Uint8Array) =>
  JSON.parse(new TextDecoder().decode(body)) as {
    id: string;
    type: string;
    occurredAt: string;
    data: Record<string, unknown>;
  };

describe("parseEvent reads every valid fixture as its type and loses nothing", () => {
  const valid = [...eventFixtures("valid")];

  test.each(valid)("%s", (name, body) => {
    const sent = decode(body);
    const event = parseEvent(body);
    expect(event).toEqual({ known: true, ...sent });
    expect(event.type).toBe(name.replace(/\.nulls$/, ""));
    expect(parseEvent(new TextDecoder().decode(body))).toEqual(event);
  });

  test("the fixtures cover every event type", () => {
    const read = new Set(valid.map(([, body]) => decode(body).type));
    expect([...read].sort()).toEqual([...eventTypes].sort());
  });
});

describe("parseEvent accepts what it does not know", () => {
  const open = eventFixtures("open");
  const known = new Set<string>(eventTypes);

  // The fixtures' rule: data is handed over as its type unless the type is
  // unknown or the file is one whose data does not fit.
  test.each([...open])("%s", (name, body) => {
    const sent = decode(body);
    const event = parseEvent(body);
    const typed = known.has(sent.type) && !name.startsWith("known-type-");
    expect(event.known).toBe(typed);
    expect(event).toEqual({
      known: typed,
      id: sent.id,
      type: sent.type,
      occurredAt: sent.occurredAt,
      data: sent.data,
    });
  });

  test("an unknown type keeps its data", () => {
    const event = parseEvent(open.get("unknown-type")!);
    expect(event.known).toBe(false);
    expect(event.type).toBe("billing.invoice.paid");
    expect(event.data).toMatchObject({ amount: 4200 });
  });

  test("unknown fields and values come through on a known event", () => {
    const fields = parseEvent(open.get("unknown-fields")!);
    if (!fields.known || fields.type !== "training.assignment.completed") {
      throw new Error("not read as its type");
    }
    expect(fields.data.completedBy).toBe("learner");
    expect(fields.data).toHaveProperty("score", 92);

    const values = parseEvent(open.get("unknown-values")!);
    if (!values.known || values.type !== "agent_hub.finding.created") {
      throw new Error("not read as its type");
    }
    expect(values.data.kind).toBe("drift");
  });
});

describe("parseEvent refuses what is not an event", () => {
  test.each([...eventFixtures("invalid")])("%s", async (_name, body) => {
    expect(await codeOf(() => parseEvent(body))).toBe("invalid_event");
  });

  test("an empty body", async () => {
    expect(await codeOf(() => parseEvent(""))).toBe("invalid_event");
    expect(await codeOf(() => parseEvent(new Uint8Array()))).toBe(
      "invalid_event",
    );
  });

  test.each([
    "2026-02-30T08:00:00Z",
    "2026-11-16T24:00:00Z",
    "2026-11-16T08:00:60Z",
    "2026-13-01T08:00:00Z",
    "2026-11-16 08:00:00Z",
    "2026-11-16T08:00:00",
    "2026-11-16t08:00:00z",
    "2026-11-16T08:00:00+25:00",
  ])("a time Go would not parse: %s", async (occurredAt) => {
    const body = JSON.stringify({
      id: "evt",
      type: "webhook.test",
      occurredAt,
      data: { endpointId: "e" },
    });
    expect(await codeOf(() => parseEvent(body))).toBe("invalid_event");
  });

  test.each(["2028-02-29T23:59:59.5+05:30", "2026-11-16T08:00:00-00:00"])(
    "a time Go would parse: %s",
    (occurredAt) => {
      const body = JSON.stringify({
        id: "evt",
        type: "webhook.test",
        occurredAt,
        data: { endpointId: "e" },
      });
      expect(parseEvent(body).known).toBe(true);
    },
  );
});

test("data is checked against the schema's own description of it", () => {
  // Every payload the schema defines has a description the package checks.
  const schema = JSON.parse(
    readFileSync(
      new URL("../../../schema/webhooks/events.schema.json", import.meta.url),
      "utf8",
    ),
  ) as { $defs: Record<string, { required: string[] }> };
  const completed = schema.$defs["TrainingAssignmentCompletedPayload"]!;
  const sample = decode(
    eventFixtures("valid").get("training.assignment.completed")!,
  );
  for (const key of completed.required) {
    const data = { ...sample.data };
    delete data[key];
    expect(parseEvent(JSON.stringify({ ...sample, data })).known).toBe(false);
    const withNull = { ...sample.data, [key]: null };
    expect(
      parseEvent(JSON.stringify({ ...sample, data: withNull })).known,
    ).toBe(false);
  }
  const wrong: Record<string, unknown> = {
    completedAt: "yesterday",
    completedBy: 7,
    user: "ada",
    assignmentId: ["id"],
  };
  for (const [key, value] of Object.entries(wrong)) {
    const data = { ...sample.data, [key]: value };
    expect(parseEvent(JSON.stringify({ ...sample, data })).known).toBe(false);
  }
  const counts = decode(
    eventFixtures("valid").get("agent_hub.simulation.completed")!,
  );
  const fractional = {
    ...counts.data,
    counts: { ...(counts.data["counts"] as object), agents: 1.5 },
  };
  expect(
    parseEvent(JSON.stringify({ ...counts, data: fractional })).known,
  ).toBe(false);
});
