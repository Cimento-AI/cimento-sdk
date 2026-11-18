# Webhook fixtures

Both SDKs are tested against these files, so they cannot come to disagree. A change in how an SDK verifies or reads a webhook starts here.

`internal/fixtures` checks the fixtures against the event schema and against themselves. It shares no code with the SDKs.

## `signatures.json`

A list of verification cases.

| Field              | Meaning                                                          |
| ------------------ | ---------------------------------------------------------------- |
| `name`             | What the case shows. Unique.                                     |
| `secrets`          | The signing secrets the receiver holds.                          |
| `headers`          | The request's headers. Names are matched without regard to case. |
| `body`             | The request body, exactly as sent.                               |
| `now`              | The receiver's clock, in Unix seconds.                           |
| `toleranceSeconds` | How far `webhook-timestamp` may be from `now`. 300 when absent.  |
| `expect`           | `ok`, or why verification fails.                                 |

An SDK has one error for each of these, and no others:

| `expect`            | Meaning                                                                     |
| ------------------- | --------------------------------------------------------------------------- |
| `missing_header`    | `webhook-id`, `webhook-timestamp` or `webhook-signature` is absent or empty |
| `invalid_timestamp` | `webhook-timestamp` is not a whole number of seconds                        |
| `timestamp_too_old` | the timestamp is more than the tolerance before `now`                       |
| `timestamp_too_new` | the timestamp is more than the tolerance after `now`                        |
| `invalid_signature` | no `v1` signature in the header matches any secret                          |
| `malformed_secret`  | a secret is not `whsec_` followed by the base64 of a key, or there are none |

Each failing case has one thing wrong with it, so the cases do not say which error wins when two apply.

A passing case says only that the signature verifies. Whether the body is an event is the business of `events/`.

## `events/`

Each file is a request body, byte for byte. The directory says what an SDK must make of it.

| Directory  | What an SDK must do                                                                                |
| ---------- | -------------------------------------------------------------------------------------------------- |
| `valid/`   | Read it as its type, losing nothing: every field of the event comes back out.                      |
| `open/`    | Accept it. It has a type, a field or a value the SDK does not know, or a time written another way. |
| `invalid/` | Refuse it as not an event.                                                                         |

`valid/<type>.json` is one event of each type in the schema. `valid/<type>.nulls.json` is the same event with every field that may be null set to null; it is made mechanically and need not be an event Cimento would send.

The fixtures do not say what an SDK does with an event of a known type whose `data` lacks a field.

## Where the values come from

- The events in `valid/` are the examples in [Cimento's webhook documentation](https://docs.cimento.ai/webhooks/overview). Ids are made up and addresses are at `example.com`.
- The first case in `signatures.json` is the Standard Webhooks specification's own example.
- The cases named for Cimento's delivery code hold a body and a signature header that Cimento's delivery code produced, from payloads stored and read back the way Cimento stores them. That is why their `data` keys are in another order than the documentation's and their text is escaped.
- Every other case was made from one of those by changing one thing.
- The secrets were generated for these fixtures and sign nothing else.
