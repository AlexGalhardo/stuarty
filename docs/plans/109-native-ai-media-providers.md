# 109: Native AI image and video generation providers (Higgsfield, fal.ai, Replicate, Runway, Kling, Luma, FLUX, Ideogram, Recraft and ElevenLabs)

## Goal

Creators can connect the most widely used AI image, video, and voice generation APIs—including **Higgsfield AI**, **fal.ai**, **Replicate**, **Runway**, **Kling**, **Luma**, **Black Forest Labs (FLUX)**, **Ideogram**, **Recraft**, and **ElevenLabs**—directly inside Stuarty with polished, branded connection cards (similar to Cursor, Antigravity, and Claude connection panels), and generate visual, video, and voice assets straight into Stuarty's Asset Library with cost estimates, approvals, and full provenance.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  - **Feedback #15 (AI generation providers part)**: Add native connections for **Higgsfield** and other popular image and video generation providers on the internet so creators can generate visual assets, images, and videos directly inside Stuarty using those official APIs, with clean branded connection cards/buttons similar to Cursor, Antigravity, Claude, etc.
- Research verified against official developer documentation (2026-11-27):
  - **Higgsfield AI** (`https://api.higgsfield.ai`, docs `docs.higgsfield.ai`): official pay-as-you-go REST API (`Authorization: Bearer hf_api_...`) with async job submit/poll/cancel (`POST /{model_id}` + status URL) spanning 50+ models including **Higgsfield Soul v2** (image), **GPT Image 2**, **Flux 2**, **Seedance**, **Genjutsu**, **Kling**, **Wan**, **MiniMax Hailuo**, and **Veo**.
  - **fal.ai** (`https://queue.fal.run` / `https://fal.run`): serverless async queue API (`Authorization: Key <key>`) hosting FLUX, Kling, Wan, Veo, Recraft, Ideogram, and audio/video models under one key.
  - **Replicate** (`https://api.replicate.com/v1`): official predictions API (`Authorization: Bearer <token>`) with dynamic model discovery and async polling for image, video, and voice models.
  - **Runway** (`https://api.dev.runwayml.com/v1`): official async task API (`Authorization: Bearer <key>`, `X-Runway-Version`) for Gen-4 / Gen-4.5 image-to-video, text-to-video, and image generation.
  - **Kling AI** (`https://api.klingai.com/v1`): official video & image generation API with multi-shot and image-to-video support.
  - **Luma Labs** (`https://api.lumalabs.ai/dream-machine/v1`): official Ray video & Photon image generation API with start/end keyframe support.
  - **Black Forest Labs (FLUX)** (`https://api.bfl.ai/v1`): official FLUX Pro/Kontext image generation & editing API (`x-key`).
  - **Ideogram** (`https://api.ideogram.ai`): official typography-accurate image generation & editing API (`Api-Key`).
  - **Recraft** (`https://external.api.recraft.ai/v1`): official raster & native SVG vector image generation API (`Authorization: Bearer <key>`).
  - **ElevenLabs** (`https://api.elevenlabs.io/v1`): official voice synthesis (`xi-api-key`) with live `GET /voices` catalogue.
- ADR-0036 (one provider layer `service/providers/<name>/`, one line in owner, official APIs only — **Midjourney remains excluded** per RF-365 because it has no official API), ADR-0032 (every request through `connections.Manager.With` outbound gate; every generation shows an estimate and waits for human approval or an explicit rule), ADR-0033 (keys sealed in vault), SDD-016 (no vendor SDKs; standard library `net/http` only; zero new Go dependencies!), NFR-063, NFR-064.

## Scope

- Modules owned:
  - `docs/sdds/000-shared-definitions.md` and `docs/sdds/016-ai-providers.md` (document each provider's ID, default endpoint, allowed media download hosts, capabilities, limits, and auth header).
  - `core/provider/` (extend provider ID validation, SVG/vector output format support for Recraft where safe, and multi-aspect ratio / keyframe input metadata).
  - `service/providers/higgsfield/`, `service/providers/fal/`, `service/providers/replicate/`, `service/providers/runway/`, `service/providers/kling/`, `service/providers/luma/`, `service/providers/bfl/`, `service/providers/ideogram/`, `service/providers/recraft/`, `service/providers/elevenlabs/` (standard-library HTTP adapters implementing `Provider` + `ImageGenerator`, `VideoGenerator`, `VoiceGenerator`, and/or `ImageEditor` with recorded contract fixtures).
  - `web/app/owner/` (register each new adapter with one line per SDD-016 §2).
  - `web/resources/js/pages/Connections/ProviderCards.tsx` (branded provider card grid with crisp iconography, category filters [All / Text / Image / Video / Voice], status pills, one-click Connect/Test/Models buttons matching the modern Cursor/Antigravity/Claude card aesthetic).
  - `web/resources/js/assetlibrary/GenerateDialog.tsx` (smooth provider + model picker showing capabilities, aspect ratio/duration limits per selected provider, and cost estimate).
  - `web/resources/lang/en.json`, `web/resources/lang/pt-BR.json`.
- Must not touch: `renderer/`, `engine/`, `desktop/`, `core/timeline/`, `service/agenthost/`.

## Out of scope

- Any service without an official public API (Midjourney, unofficial web wrappers — forbidden by ADR-0036 item 3).
- External file storage or design MCP connectors like Google Drive, Figma, ArtCraft, OpenPencil (owned by epic 110).
- Adding vendor Go SDKs to `go.mod` (all adapters use Go's `net/http` standard library only).

## Dependencies

- Epics 10, 11, 84, and 116 (image/voice/video generation pipelines, Connections page, and versioned FigCraft design in `docs/design/final/stuarty-redesign-final-trilha.figcraft` — update FigCraft frames for the branded provider cards).

## Technical design

1. **Zero-dependency adapter pattern (`service/providers/<name>/`)**:
   - Every adapter follows `SDD-016`:
     - Uses `connections.Credential` and its gated `*http.Client` (never opens its own socket, never reads raw env vars).
     - Fetches generated media URLs only from documented CDN/storage hosts declared in the connection's host allowlist (never blind redirect following to arbitrary hosts, preserving SDD-007 SSRF protections).
     - Implements `Submit`, `Poll`, `Fetch`, `Cancel` for async video/image queues so `video.generate` and `image.generate` jobs survive server restarts without double-charging.
     - Ships a documented `Models()` implementation (live catalogue endpoint where the provider offers one—Replicate, ElevenLabs, fal.ai—or documented curated model list with runtime custom model ID override where the provider has no `/models` endpoint—Higgsfield, Runway, Luma, Kling, BFL, Ideogram, Recraft).
     - Never hardcodes prices as facts: reads price from the provider catalogue when published, otherwise prompts the user in the price table (SDD-016 §4.2).

2. **Higgsfield AI adapter (`service/providers/higgsfield/`) — flagship multi-model adapter**:
   - Capabilities: `image`, `video`.
   - Default endpoint: `https://api.higgsfield.ai` (`Authorization: Bearer <key>`).
   - Supports `Idempotency-Key` header on `Submit` so retries never double-bill.
   - Supports `higgsfield-ai/soul/v2`, `higgsfield-ai/soul/standard`, plus video models (`seedance`, `kling`, `wan`, `veo`, `genjutsu`) exposed dynamically or by user-selected model ID.

3. **Branded Provider Connection Cards UI (`ProviderCards.tsx`)**:
   - Group providers cleanly with filter pills (**All**, **AI Chat / Text**, **Image Generation**, **Video Generation**, **Voice**) and sleek branded cards (icon/monogram badge, provider name, capability badges, connected/not-connected pill, one-click **"Connect"** button opening a focused modal with direct docs link to obtain the API key, **"Test"**, **"Models & Prices"**, and spend summary).
   - Keep `GenerateDialog.tsx` fast and clear: selecting "Generate Image" or "Generate Video" automatically filters to connected providers with that capability, or offers a direct "Connect a provider" shortcut when none is connected yet.

## Tasks

- [ ] 1. Amend SDD-000 and SDD-016 with the 10 new provider descriptors (IDs, endpoints, allowed media download hosts, capabilities, auth headers, and terms/provenance notes). Verify: `task lint:md` and `go run ./internal/tools/threatcheck` exit 0.
- [ ] 2. Implement `service/providers/higgsfield/`, `service/providers/fal/`, and `service/providers/replicate/` (multi-model image & video adapters) with recorded contract tests (submit, poll pending/done/failed/refused, fetch, cancel, SSRF host rejection). Verify: `go test ./service/providers/higgsfield/... ./service/providers/fal/... ./service/providers/replicate/...` passes with no network.
- [ ] 3. Implement `service/providers/runway/`, `service/providers/kling/`, and `service/providers/luma/` (dedicated video/image adapters) with recorded contract tests. Verify: `go test ./service/providers/runway/... ./service/providers/kling/... ./service/providers/luma/...` passes with no network.
- [ ] 4. Implement `service/providers/bfl/`, `service/providers/ideogram/`, `service/providers/recraft/`, and `service/providers/elevenlabs/` (dedicated image, vector/typography, and voice adapters) with recorded contract tests. Verify: `go test ./service/providers/...` passes with no network.
- [ ] 5. Register all 10 adapters in `web/app/owner/` and verify `TestAddingAProviderNeedsOnePackageAndOneLine`, parity, and `GenerateJob` restart-safety tests. Verify: `go test ./service/... ./web/app/... ./web/contract/...` passes.
- [ ] 6. Upgrade `ProviderCards.tsx` with branded cards, capability filter tabs, and one-click connect dialogs, update `GenerateDialog.tsx`, add `en.json` + `pt-BR.json` strings, and add Playwright E2E tests against fake provider endpoints. Verify: `task lint`, `task test`, `task test:web`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n`, and the Playwright providers/generate specs pass.

## Test strategy

- Contract tests (zero network): every adapter has recorded JSON fixtures for `Models`, `GenerateImages` / `Submit` / `Poll` / `Fetch` / `Cancel`, HTTP 401/403/429/500/refusal handling, and download-host allowlist enforcement.
- Integration tests: job restart during `Poll` submits only once; approval queue blocks unapproved generation for every new provider; cost ledger records spend; generated asset carries full `origin` (`provider`, `model`, `prompt`, `date`).
- Web & E2E: Vitest tests for `ProviderCards` filtering and connection flow; Playwright spec connecting a fake Higgsfield provider, setting a price, approving generation, and verifying the generated image/video lands in the Asset Library.

## Acceptance criteria

1. All 10 providers (`higgsfield`, `fal`, `replicate`, `runway`, `kling`, `luma`, `bfl`, `ideogram`, `recraft`, `elevenlabs`) are registered in `providers.Registry`, appear as branded connection cards in `/connections`, and require zero new Go dependencies.
2. Every adapter routes 100% of API and media-download traffic through `connections.Manager.With`, enforces `STUARTY_MAX_MEDIA_BYTES`, sanitizes untrusted text, and requires human approval (or an explicit rule) before spending.
3. A creator can generate an image or video via Higgsfield (or any of the other 9 providers) from `GenerateDialog` and find the resulting asset with full provenance in the Asset Library.
4. `go test ./core/provider/... ./service/providers/... ./service/...`, `go test -run TestSecurity_ ./...`, `task lint`, `task test:web`, and `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n` pass with no network access.

## Risks and mitigation

- **Provider API endpoints evolving between docs and production.** Mitigation: isolate each provider in its own package with recorded contract fixtures and configurable connection `endpoint`; **STOP** and ask the author for a real API smoke check only after all contract tests pass (manual verification never blocks local DoD).
- **SSRF via provider-returned media URLs.** Mitigation: every adapter validates download URLs against the connection's explicit CDN host allowlist before fetching through the gated client, backed by a security test per adapter.
- **SVG script injection (Recraft vector outputs).** Mitigation: rasterize or strictly strip scripts/external refs when ingesting SVG outputs (or request PNG/WebP by default and sanitize SVG in `core/` before storage); **STOP** if SVG sanitization requires a new library.

## Size

L

## Hand-off for review

The commits of `epic/109-native-ai-media-providers`, updated SDD-016/SDD-000, Go/security/Vitest/Playwright outputs, and screenshots of the branded Provider Cards UI and Generate dialog.
