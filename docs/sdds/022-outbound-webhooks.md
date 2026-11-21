# SDD-022: Outbound webhooks

- Status: Draft (2026-11-20). Written by epic 59 before its code.
- Depends on: ADR-0032 (outbound only to connected destinations, no telemetry), ADR-0033 (secrets only in the vault), ADR-0008, SDD-000 (names), SDD-003 (the surfaces), SDD-005 (persistence), SDD-007 (threat model, outbound paths), SDD-014 (the vault and the gate), SDD-015 (approvals), SDD-020 (publications)
- Implements: author decision D-121 (outbound only, no inbound webhooks)
- Owner area: epic 59 (`core/webhook/`, `service/webhooks/`, `service/webhook*.go`, `internal/outbound/webhook.go`, the `webhooks` adapters of `web/app/`, the Webhooks page)

## Contents

1. Overview
2. The webhook record
3. Events and the payload
4. Delivery, signature and retries
5. The address rules
6. Service operations and scopes
7. Surfaces
8. Errors, table, identifiers and limits
9. The page
10. Tests

## 1. Overview

A webhook tells another tool that something happened in Stuarty: a post went out, an export is ready, an approval waits for a person, a job failed. The person registers an address, chooses the events and keeps the signing secret that proves a call came from this workspace. Only outbound webhooks exist; Stuarty receives no webhook (D-121).

Four rules carry the design:

1. **With no webhook registered, nothing leaves the machine.** The dispatcher has nothing to send and opens no socket (ADR-0032). A webhook is a destination the person typed, so it is a connection in the sense of ADR-0032 and is listed as path 9 of SDD-007 section 10.1.
2. **The payload is metadata.** Identifiers, states, counts and the public address of a published post. Never a credential, a token, a file, a text body, a prompt or a path (section 3).
3. **The secret is shown once.** The server generates the signing secret, puts it in the vault (SDD-014) under the id of the webhook, and returns it in the answer of the creation and of the rotation, and nowhere else (ADR-0033).
4. **Delivery never runs in a request.** A bounded worker sends, retries and records; the operations only write rows and wake it.

## 2. The webhook record

| Field | Type | Rule |
| --- | --- | --- |
| `id` | string | `whk_` and 10 lower-case Crockford base32 characters (SDD-000 section 2) |
| `name` | string | 1 to 60 characters, no control character; text a person typed, so a surface that shows it to an agent delimits it as untrusted text |
| `url` | string | `https`, or `http` for a loopback host chosen explicitly (section 5); at most 2048 characters; no userinfo and no fragment. The path and the query are kept: many receivers put their token there, so the URL is shown only to a caller with the `admin` scope and the MCP tool shows the host only |
| `events` | list of strings | one to five of the events of section 3; no duplicate |
| `enabled` | boolean | a disabled webhook receives nothing; its pending deliveries stay and are sent when it is enabled again (up to the retry limit) |
| `secret_set` | boolean | always true; the secret itself is in no result but the creation and the rotation |
| `created_at`, `updated_at` | time | RFC 3339, UTC |
| `last_delivery` | object or null | `state`, `at` and `status` of the newest delivery |

At most 20 webhooks per workspace (`E_LIMIT_EXCEEDED`). The secret has the form `whsec_` and 43 URL-safe base64 characters (32 random bytes); it is kept in the vault under the id of the webhook.

## 3. Events and the payload

| Event | When | `data` members |
| --- | --- | --- |
| `publication.published` | a target of a publication went out (`published`) | `publication_id`, `target_id`, `project_id`, `network`, `connection_id`, `url` (the address of the post, only when the visibility of the target is `public`, `https`, and at most 2048 characters: the link of an unlisted post is what keeps it private, and a target that names no visibility is private), `published_at` |
| `publication.failed` | a target of a publication failed for good (`failed`) | `publication_id`, `target_id`, `project_id`, `network`, `connection_id`, `error_kind` (the kind of SDD-020 section 11, never its text) |
| `export.ready` | an export job succeeded: of a timeline (`export`), or of an image (`image.export`), which has no project and so no `project_id` | `job_id`, `project_id`, `kind`, `format`, `bytes`, `width`, `height` |
| `approval.requested` | a request waits for a person | `approval_id`, `kind`, `project_id`, `job_id`, `connection_id`, `expires_at`; never its summary |
| `job.failed` | a job ended in `failed`, with no `project_id` when it belongs to no project | `job_id`, `project_id`, `kind`, `state`, `error_code` |
| `webhook.test` | the person pressed "send test"; it cannot be chosen as an event | `webhook_id` |

The body of a delivery is one JSON object:

```json
{"event":"export.ready","id":"evt_3f9c0a71b2d4e5a60718","occurred_at":"2026-11-20T12:00:00Z","workspace":"wsp_4c1e0d92ab","data":{"job_id":"job_...","project_id":"prj_...","kind":"export","format":"mp4","bytes":1048576,"width":1080,"height":1920}}
```

- `id` is `evt_` and 20 hexadecimal characters of the SHA-256 of the event name and the subject (for example `export.ready` and the job id), so the same event is the same id on every delivery and on a redelivery, and a receiver can drop a duplicate. The subject of `publication.failed` is the target and its approval request: a retry makes a new request, so a target that fails again is a new event. `webhook.test` takes a random id.
- `workspace` is `wsp_` and 10 hexadecimal characters of the SHA-256 of the workspace path: stable, opaque, and it does not reveal the path.
- `data` holds only the members of the table, built from an allow-list in `core/webhook`. A text the product holds (a title, a caption, a summary, a prompt, an error message) is not a member of any event. The body is at most 8 KiB; larger is a defect.
- An event is built once, when it happens, and stored with its delivery, so a redelivery sends the same body with a new signature.

## 4. Delivery, signature and retries

A delivery is a row: the webhook, the event, the stored body, a state (`pending`, `succeeded`, `failed`), the number of attempts, the time of the next attempt and the result of the last. An event creates one delivery per enabled webhook that chose it. The row is written in the same step that sees the event, before the worker is woken, so a restart loses none; the same event for the same webhook is written once (a unique index on the webhook and the event id).

**The request.** `POST` to the URL with:

- `Content-Type: application/json`
- `Stuarty-Signature: t=<unix seconds>,v1=<hex>`, where `v1` is the HMAC-SHA256, with the signing secret as the key, of the text `<t>.<body>` (the digits of `t`, a dot, the exact bytes of the body). A receiver recomputes it and refuses a `t` far from its clock.
- `Stuarty-Event: <event>` and `Stuarty-Delivery: <delivery id>`
- `User-Agent: Stuarty-Webhooks/1`

The client has a timeout of 10 seconds for the whole request, uses no proxy, verifies TLS and follows no redirect: a 3xx is a failure. It reads at most 4 KiB of the answer, only to count it.

**The result.** A `2xx` is a success. A network error, a timeout, a `5xx`, a `408` and a `429` are retried; any other answer (a `4xx`, a `3xx`) is a failure that is not retried, because the receiver said no. After the first attempt there are at most 5 retries, waiting 10 s, 30 s, 90 s, 270 s and 810 s (10 s times 3 to the power of the retry number minus one); after the last the delivery is `failed`. A "send test" makes one attempt and no retry.

**The record.** Each attempt keeps its number, its time, the HTTP status (0 when there was none), its duration, a short error code (`timeout`, `connect`, `refused_address`, `status`, `vault_locked`, `invalid_url`) and the size in bytes of the answer. Never the body of the answer: it is an untrusted text and may hold what the receiver chose to say. The last 50 attempts of a webhook are kept; older rows of a finished delivery are deleted with it after 30 days.

**A locked vault.** The secret is read at the moment of the attempt. When the vault cannot be opened (SDD-014 section 4), the attempt is not made and not counted: the delivery stays `pending` and is tried again after 30 seconds, and it is sent after the unlock.

**The worker.** One dispatcher per server, started by the owner next to the publication scheduler, sleeps until the next delivery is due (and at most 30 s), claims due deliveries and sends at most 4 at once. Another webhook that is slow never delays a due one beyond that bound. A delivery interrupted by a shutdown is `pending` again at the next start.

**Redelivery.** `WebhookRedeliver` copies the stored body of a finished delivery into a new `pending` delivery of the same webhook, with a fresh signature time; it is allowed for any state and counts as a new delivery.

## 5. The address rules

1. The scheme is `https`. `http` is accepted only when the host is `localhost` or an IP address of the loopback block, which the person typed on purpose (a receiver on the same machine, a test).
2. No userinfo, no fragment, a host, at most 2048 characters, no control character.
3. A host that is an IP literal in a private, link-local, shared (RFC 6598), unspecified, multicast or metadata range, or an IPv6 form that carries one (SDD-014 section 6), is refused when the webhook is saved. So is a host whose last label is a number (decimal, octal or `0x` hexadecimal, as in `2130706433`, `0x7f.1` or `127.1`): no public name ends so, and a resolver reads it as an IPv4 address. The loopback block is refused unless rule 1 allows it. A name is not resolved when the webhook is saved (a lookup is a packet).
4. **At connect time** the client of the purpose `webhook` (`internal/outbound`) checks the address it is about to dial, after the name was resolved: a webhook whose host is not loopback refuses every address that is loopback, private, link-local, shared, unspecified, multicast or a named metadata address; a webhook whose host is loopback accepts loopback addresses only. A name that resolved to a public address when it was saved and to an internal one when it is dialled (DNS rebinding) reaches nothing. The host of the request must be the host of the webhook, at every request.
5. No redirect is followed, so a public address cannot send the request to an internal one.

## 6. Service operations and scopes

Defined in SDD-000 section 1. Every operation is for the whole workspace: a key limited to one project is refused (`E_FORBIDDEN`).

| Operation | Scope | Does |
| --- | --- | --- |
| `WebhookEventList` | read | the events, each with a sentence that says when it fires and what it carries |
| `WebhookList`, `WebhookGet` | admin | the webhooks, with the address |
| `WebhookCreate` | admin | validates, generates the secret, puts it in the vault, records, audits; answers with the secret once |
| `WebhookUpdate` | admin | changes the name, the address, the events or `enabled`; never the secret |
| `WebhookRemove` | admin | deletes the secret from the vault, then the row and its deliveries |
| `WebhookRotateSecret` | admin | generates a new secret, replaces it in the vault, answers with it once; deliveries that are pending are signed with the new one |
| `WebhookTest` | admin | one `webhook.test` delivery, one attempt, answered with the delivery |
| `WebhookDeliveryList` | admin | the deliveries of a webhook, newest first, with their attempts, 50 per page |
| `WebhookRedeliver` | admin | section 4 |

Creating a webhook is a person's action: the MCP surface has no write operation for it, and an agent cannot register an address. Every write is audited (`webhook.create`, `webhook.update`, `webhook.remove`, `webhook.rotate_secret`, `webhook.test`, `webhook.redeliver`), with no address and no secret.

## 7. Surfaces

| Operation | CLI | HTTP (under `/api/v1`) |
| --- | --- | --- |
| `WebhookEventList` | `webhook:events` | `GET /webhook-events` |
| `WebhookList` | `webhook:list` | `GET /webhooks` |
| `WebhookGet` | `webhook:show` | `GET /webhooks/{id}` |
| `WebhookCreate` | `webhook:add` | `POST /webhooks` |
| `WebhookUpdate` | `webhook:update` | `PATCH /webhooks/{id}` |
| `WebhookRemove` | `webhook:remove` | `DELETE /webhooks/{id}` |
| `WebhookRotateSecret` | `webhook:rotate-secret` | `POST /webhooks/{id}/rotate-secret` |
| `WebhookTest` | `webhook:test` | `POST /webhooks/{id}/test` |
| `WebhookDeliveryList` | `webhook:deliveries` | `GET /webhooks/{id}/deliveries` |
| `WebhookRedeliver` | `webhook:redeliver` | `POST /webhooks/{id}/deliveries/{delivery_id}/redeliver` |

The read of an agent goes through the `connection` tool: with `webhooks` true it lists the webhooks (name, host, events, enabled, last delivery; never the path, the query or a secret). One webhook is read with `webhook:show` or `GET /webhooks/{id}`. The tool count and the size of `tools/list` are held by the budget of SDD-003 section 4.8, so no tool is added. The CLI shows the secret of `webhook:add` and `webhook:rotate-secret` once, on standard output, and a flag `--json` carries it in the member `secret`.

The CLI flag that names the address is `--endpoint`, as in `connection:add`: it registers a destination, it does not name where a command sends data at run time.

## 8. Errors, table, identifiers and limits

- `E_WEBHOOK_NOT_FOUND` (404): the webhook or the delivery does not exist (SDD-000 section 8). An address the rules of section 5 refuse, a name, an event that does not exist, too many events are `E_VALIDATION` with `path`. A vault that cannot open is `E_VAULT_LOCKED`.
- Identifiers: `whk_` (webhook), `wdl_` (delivery), SDD-000 section 2.
- Tables `webhooks`, `webhook_deliveries`, `webhook_attempts` (SDD-000 section 7). None holds a secret; a body is metadata.
- Limits: 20 webhooks; 5 events per webhook; a name of 60 characters; an address of 2048; a body of 8 KiB; 10 s per request; 5 retries; 4 sends at once.

## 9. The page

The Webhooks page (`/webhooks`, an entry in the sidebar) lists the webhooks with their state and last delivery, creates one with a name, an address and a checklist of the events, each with the sentence the server gives, shows the secret once with a copy button and a warning that it is not shown again, sends a test, rotates the secret (and shows the new one once), disables and removes. A webhook opens to its deliveries, each with its event, state, attempts and status, and a "send again" button. The page draws what the service answers and computes no result.

## 10. Tests

`core/webhook` (the address rules, the allow-list of payloads, the signature, the retry schedule); `service/webhooks` against a local receiver (success, retry on 5xx, timeout, no redirect, the signature verifies with the shown secret, a locked vault waits, a test send); the security tests of SDD-007 (T-110 onward); the parity rows; the Playwright spec of the page. The check of the payloads runs over every event and fails when a member outside the allow-list, a credential, a path or a text body is present.
