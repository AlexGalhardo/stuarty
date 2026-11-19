# 09: AI provider layer with API keys, text generation and cost

## Goal

A creator connects the AI services they already pay for, once, and every part of Stuarty can use them: with the price shown before anything runs, and the model list always current. This epic delivers the layer and its first capability, text.

## Context and related decisions

- ADR-0036 (one provider layer, four ways to connect), ADR-0032, ADR-0033, ADR-0021.
- Author decisions D-059, D-061. PRD-004 RF-360 (API key), RF-361 (text), RF-362 to RF-365, NFR-063, NFR-064.
- Research note `web/03-integracao-ia`.

## Scope

- Modules: `service/providers/` (new: registry, interfaces, cost, catalogue), `service/providers/<name>/` (one adapter per provider), `core/provider/` (new: pure rules of capability, request validation, cost estimation), `web/app/` (controllers, CLI, MCP), provider cards on the Connections page, `docs/sdds/016-ai-providers.md` (new), SDD-000, SDD-007 rows.
- Credential type `api_key` only. Capability `text` only.
- The first adapters: two text providers chosen in task 2 and approved by the author.

## Out of scope

- Image, voice and video (epics 10, 11); OAuth sign-ins (12); the local agent (14).
- A chat or an agent loop: one request, one answer (ADR-0037).
- Moving transcription onto this layer: it keeps working as it is; only its provider settings are read through the registry if that is cheap, otherwise leave a note.

## Dependencies

04, 05.

## Technical design

- **Interfaces** in `service/providers`: `Provider` (id, capabilities, credential kinds, `Models(ctx)`), `TextGenerator` (`Estimate(req)`, `Generate(ctx, req)` with streaming optional). Capability interfaces for image, voice and video are declared here, empty of adapters, so later epics only add adapters.
- **Registry**: built at start from the adapters; a provider is usable when a connection (epic 04) of its kind exists. All HTTP goes through `internal/outbound` with that connection.
- **Rules in `core/provider`**: request validation (length, parameters within the model's limits), estimation from a price table **read from the provider's catalogue when it publishes prices**, otherwise from a table the user can edit in settings; never a price fixed in code as a fact.
- **Jobs**: `text.generate` is a job kind; it records estimate and real cost in the ledger of epic 05 and waits for approval only above the ceiling the user set.
- **Service**: `ProviderList`, `ProviderModels`, `TextGenerate` (returns a job id), with the result stored as text of the project item that asked for it or returned to the caller.
- **Safety**: prompts sent to a provider may contain untrusted text; the result is returned as untrusted text; a provider's error body is never shown raw.
- **Never** (RF-365): an adapter that needs a subscription login a provider forbids, or a service with no official API.

## Tasks

- [x] 1. Write SDD-016 and the SDD-000 names; add SDD-007 rows (prompt with untrusted text, provider output as instructions, cost run-away). Verify: `task lint:md` exits 0.
- [ ] 2. Propose the first two text providers with their API, pricing source and terms; STOP for approval. Verify: the approval is in the hand-off.
- [x] 3. Implement `core/provider` with table tests. Verify: `go test ./core/provider/...` passes.
- [x] 4. Implement the registry, the interfaces and the model catalogue with caching. Verify: `go test ./service/providers/...` passes against recorded responses.
- [ ] 5. Implement the two adapters with contract tests from recorded responses. Verify: `go test ./service/providers/...` passes with no network (`bash scripts/no-egress.sh` still passes).
- [ ] 6. Implement the `text.generate` job with estimate, ledger and ceiling. Verify: `go test ./service/... -run TestTextGenerate` passes.
- [ ] 7. Expose on HTTP, CLI and MCP; add provider cards (connect with a key, test, models, spend) to the Connections page. Verify: `go test ./web/contract/...`, `task test:web` and `web/e2e/specs/providers.spec.ts` pass.
- [ ] 8. Manual check with one real key of the author, if the author provides it; otherwise mark as not verified. Verify: the transcript or the "not verified" note is in the hand-off.

## Test strategy

- Unit: validation and estimation tables.
- Contract: each adapter against recorded request and response pairs, including errors and rate limits.
- Security: the new `TestSecurity_` rows; a key never appears in a log, an error or a result.
- End to end: connect with a fake provider server, generate a text, see the cost.

## Acceptance criteria

1. With a provider connected by API key, `TextGenerate` returns a job that completes and stores the text, on HTTP, CLI and MCP.
2. The estimate is shown before the job runs and the real cost is recorded after; a request above the ceiling is refused or waits for approval as configured.
3. `ProviderModels` returns the provider's current list, not a list fixed in code.
4. Adding a provider needs only a new package under `service/providers/` and one registration line (shown by the second adapter's diff).
5. No surface returns a key; `go test -run TestSecurity_ ./...` passes.
6. `go test ./core/provider/... ./service/providers/...` passes with no network access.

## Risks and mitigation

- **Prices and model names change monthly.** Mitigation: catalogue at run time; editable price table.
- **A paid call in tests.** Mitigation: recorded responses only; the real check is manual and optional.
- **STOP** at task 2, and for any dependency (an SDK): prefer plain HTTP.

## Size

L

## Hand-off for review

The diff of `epic/09-ai-provider-layer`; the approved providers; recorded fixtures and how they were captured; the outputs of the acceptance commands.
