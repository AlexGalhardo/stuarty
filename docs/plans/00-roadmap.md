# Roadmap of Stuarty

The order of the work: which epics exist, what each depends on, what can run side by side, and which epic delivers each requirement. One epic is one file of this folder (`NN-<slug>.md`), in the format of `.agents/rules/plans.md`: one senior takes it from its first task to its hand-off, and the specialist reviews it. The process is ADR-0045; the product is PRD-001 to PRD-004.

- There are no phases and no milestones. The first complete flow to deliver is a short vertical video with captions and a thumbnail, from the briefing to the publication (PRD-004 section 3); section 5 lists the epics that close it.
- Epics are numbered in the order of dispatch. Two tracks run in parallel after the foundation: the platform (three seniors) and the editor (two seniors).
- Every commit lands on the integration branch `release/0.30.0`, in the draft pull request #106 (codename "release 0.30.0"). The pull request is split into releases only after the QA campaign (section 4).
- Written against commit `6fe3eca7`. After epic 02 the binary is `stuarty`; epics 03 onwards use the new names.

## 1. Status

Status values: `TODO`, `IN PROGRESS (<agent>, <date>)`, `BLOCKED (<reason>)`, `DONE (<date>, <commits>)`. Only the orchestrator writes this column, in the same commit that merges the epic.

### 1.1 Foundation (sequential, one at a time, before everything)

| Epic | Title | Size | Depends on | Status |
| --- | --- | --- | --- | --- |
| [01](01-remove-the-gio-window.md) | Remove the Gio window | S | — | DONE (2026-11-18, d5f2c792..1ac59baf) |
| [02](02-rename-to-stuarty.md) | Rename the code to Stuarty, with a workspace migration | L | 01 | DONE (2026-11-19, cdd8ce10..dfb93934) |
| [03](03-drop-the-phase-labels.md) | Drop the phase labels from the registry and the specifications | S | 02 | DONE (2026-11-19, 78261496..f36e63b7) |

### 1.2 Platform track (three seniors)

| Epic | Title | Size | Depends on | Status |
| --- | --- | --- | --- | --- |
| [04](04-secret-vault-and-connections.md) | Secret vault and connections | L | 03 | DONE (2026-11-19, 8e43a1a2..7410dcd3 replayed onto the integration branch) |
| [05](05-approval-queue-and-cost-ceilings.md) | Approval queue, rules and cost ceilings | M | 04 | DONE (2026-11-19, fe07849a..174a7b89; the MCP approval tool waits for epic 13's budget) |
| [06](06-project-as-production.md) | The project as the container of a production | L | 03 | DONE (2026-11-19, 08a630f5..77cf52b7; closes VV-225, VV-237, VV-241) |
| [07](07-dashboard-shell.md) | Dashboard: sidebar, navigation, profile, theme | M | 03 | DONE (2026-11-19, 70387363..171fb786) |
| [08](08-asset-library.md) | Asset library with origin, rights and reuse | M | 06, 07 | DONE (2026-11-19, `ba30f86f`..`fdcdc23a`, merge `86c682f9`) |
| [09](09-ai-provider-layer.md) | AI provider layer with API keys, text generation and cost | L | 04, 05 | DONE (2026-11-19, `96540531`..`7b5f1caa`, merge `5a97458e`; task 2 provider choice and task 8 real call wait for the author) |
| [10](10-image-and-voice-generation.md) | Image and voice generation | M | 08, 09 | DONE (2026-11-20, `18d2e82b4`..`a94e2d963`, merge `3402308e1`; a real call with the author's OpenAI key not verified) |
| [11](11-video-generation.md) | Video generation | M | 10 | IN PROGRESS (provider-independent part merged 2026-11-20, `4f306b78f`..`0d81c8d7c`, merge `837c37f05`; tasks 1 and 4, the provider adapter, wait for the author's choice of a video provider) |
| [12](12-ai-oauth-connections.md) | Sign in with ChatGPT and OpenRouter | M | 09 | DONE (2026-11-20, `6cdb3a8e6`..`b39b72b65` replayed, merge `42a953ae9`; a real sign-in with the author's accounts not verified) |
| [13](13-mcp-domains-and-agent-docs.md) | MCP tools by domain and generated agent documentation | M | 06 | DONE (2026-11-19, `773674ed`..`e9e671d4`, merge `c3be54a0`) |
| [14](14-hosted-agent-chat.md) | The user's agent hosted in the chat panel | L | 07, 13 | DONE (2026-11-20, `23d31581`..`26a80add0` replayed, merge `365dcd1bd`; run with a real installed agent not verified) |
| [15](15-memory-skills-and-prompts.md) | Memory, skills and prompts | L | 07 | DONE (2026-11-19, 65d33593..d2e8660f) |
| [16](16-curated-skills-and-briefing.md) | Curated skills and the guided briefing | M | 06, 15 | DONE (2026-11-19, `6a134407`..`6771dee1`, merge `7aff38e0`) |
| [17](17-writing.md) | Writing: scripts, copy and captions | M | 06, 09 | DONE (2026-11-20, `ef219641c`..`5fb9bbf52` replayed, merge `6989137a2`; a real draft with a provider key not verified) |
| [18](18-social-connectors-and-youtube.md) | Social connectors and YouTube | L | 04, 05 | DONE (2026-11-20, `4cfc130a`..`375f681d`, merge `f4c35c228`; task 10 real upload waits for the author) |
| [19](19-instagram-and-facebook.md) | Instagram and Facebook | L | 18 | DONE (2026-11-20, `c86c2f10b`..`8428bc2d3`, merge `ada3c608f`; task 9, a real post with the author's Meta app, not verified) |
| [20](20-publishing-calendar-and-scheduler.md) | Publishing: calendar, approval and scheduler | L | 06, 07, 18 | DONE (2026-11-20, `0e4e0dbe2`..`77d7bc3b7`, merge `e0453f55f`; the scheduler runs while the app runs until epic 21; a real post not verified) |
| [21](21-desktop-background-service.md) | Desktop background service for scheduled posts | M | 20 | DONE (2026-11-20, `d7d8dc810`..`38dc447d9` replayed, merge `7eb3945ee`; the tray, the real sign-in registration and the installer are a manual check of the author, D-096) |
| [22](22-x-and-tiktok-drafts.md) | X and TikTok drafts | M | 20 | DONE (2026-11-20, 167d36b57..afe427f7a, merge 68cec8110; threats T-68, T-69, T-79; task 6, a real post and draft, waits for the author) |
| [23](23-blog-and-its-destinations.md) | Blog posts and their four destinations | L | 17, 20 | DONE (2026-11-21, b41f480ee..71dfe873d, merge 099a668f1; threats T-86 to T-88, T-95, T-99; task 10, a real WordPress, waits for the author) |
| [24](24-google-account-and-server-login.md) | Optional Google account and login on a server | M | 04, 18 | DONE (2026-11-20, `46c44b677`..`cde96fa60`, merge `105a2cf8a`; a real Google login and `scripts/no-egress.sh`, which needs Linux, not verified) |
| [25](25-vps-package.md) | VPS package: Caddy, install, update and backup | M | 24 | DONE (2026-11-20, `7da749ed7`..`6293c2d78`, merge `2722becab`; a real VPS, the built image and `caddy validate` not verified) |

### 1.3 Editor track (two seniors)

| Epic | Title | Size | Depends on | Status |
| --- | --- | --- | --- | --- |
| [26](26-browser-preview-on-the-rust-renderer.md) | The browser preview on the Rust renderer | L | 03 | DONE (2026-11-19, e21f512f..9c750319; task 8, the idle-machine playback measurement, waits for an idle window) |
| [27](27-image-document-core.md) | Image documents: core, catalogue, templates and export | L | 03 | DONE (2026-11-19, 97d5c560..7758cdda replayed and merged; WebP export waits for an ffargs builder) |
| [28](28-image-editor-ui.md) | Image editor in the UI | L | 07, 27 | DONE (2026-11-19, `95ed640f`..`28a3e533`, merge `fac66975`) |
| [29](29-image-adjustments-filters-and-masks.md) | Image adjustments, filters, masks and blend modes | L | 27 | DONE (2026-11-19, `5d7680de`..`525c359b`, merge `1ee6a61d`; image LUT waits for the author) |
| [30](30-image-operations-with-a-model.md) | Image operations with a model | S | 10, 28 | DONE (2026-11-20, 7563309a0..0f833f39f, merge eaf08b5db; threats T-96 to T-98) |
| [31](31-motion-easing-crop-and-mask.md) | Motion: easing curves, animated crop and mask, presets | L | 03 | DONE (2026-11-19, `e914c99b`..`a88f8e63`, merge `4c2c5c85`) |
| [32](32-nested-compositions.md) | Nested compositions | M | 06, 31 | DONE (2026-11-20, `07fe073da`..`8264cd564`, merge `320ce36d2`; the live group in the browser moved to epic 52 and constant speed to epic 53 by D-094) |
| [33](33-external-motion-engine-hand-off.md) | Hand-off from an external motion engine | M | 08 | DONE (2026-11-20, `4f4022cc`..`bb19aa733` replayed, merge `3d693ba1d`; real HyperFrames round and no-egress on Linux verified by epic 73) |
| [34](34-audio-processors-and-noise-reduction.md) | Audio processors and noise reduction | L | 03 | DONE (2026-11-20, merge of `epic/34` at `2e6efc574`; neural model and timeline playback of processed audio wait for the author) |
| [35](35-export-presets-batch-and-codecs.md) | Export presets, batch queue, H.265 and AV1 | M | 03 | DONE (2026-11-19, `c1bb77fa`..`c718fdb2`, merge `1063ec42`) |
| [36](36-revert-checkpoints-and-diff.md) | Revert to a revision, checkpoints and a diff | M | 03 | DONE (2026-11-20, `c42f2f5ad`..`90c6df86c`, merge `8d12df6d4`; image documents left for a later epic) |
| [37](37-editor-ui-and-shortcuts.md) | Final editor UI, shortcuts and open fixes | L | 07 | DONE (2026-11-19, `7921c340`..`2715dcc0`, merge `46691bb3`; criterion 4 not met on this Windows machine: WebKit/Firefox limits and VV-238, VV-253, VV-260 to VV-264) |
| [48](48-image-import-and-image-events.md) | Image import and the image event stream | M | 27, 28 | DONE (2026-11-20, `eaabd4cde`..`7975f0872`, merge `80a23e8dd`; an `image` project item is not in the schema, SDD-021 §10.2) |
| [49](49-image-lut.md) | Image lookup tables (.cube) | S | 29 | IN PROGRESS (tasks 1 to 6 merged 2026-11-20, da804c05a..819435062, merge 23d185e93; threats T-92 to T-94; task 7, the pinned renderer digest, waits for VV-276) |
| [50](50-image-revert-checkpoints-and-diff.md) | Revert to a revision, checkpoints and a diff for image documents | M | 36, 29 | DONE (2026-11-21, 916861fda..f0345a119, merge 443892948) |
| [51](51-processed-audio-preview-on-the-timeline.md) | Processed audio preview on the timeline | M | 34 | DONE (2026-11-20, `e1729a0bf`..`8778074ef`, merge `74a429def`; the latency budget on an idle machine not measured) |
| [52](52-live-composition-groups-in-the-browser.md) | Live composition groups in the browser preview | M | 32, 26 | DONE (2026-11-20, `43b11f50e`..`5332706c1`, merge `7cb81c176`; Firefox and WebKit not run) |
| [53](53-constant-speed-for-compositions.md) | Constant speed for compositions | M | 32 | DONE (2026-11-20, 38369a110..6dc199452, merge 425c9bf14; the audio keeps the pitch through atempo, no nested bus) |
| [54](54-image-pipeline-performance.md) | Image pipeline performance (NFR-061) | L | 27, 29, 36, 49, 50 | DONE (2026-11-20, `bb6d11435`..`9df22287f`; the rest of NFR-061 moved to epic 55 by D-118) |
| [55](55-image-renderer-performance.md) | Image renderer performance (NFR-061, the renderer half) | L | 54 | IN PROGRESS (tasks 1 to 4 reviewed and merged in v0.46.0 on 2026-11-21; task 5 measured on 2026-11-21: `compose_ms` 51 passes, `s_export` 2.50 s misses the 2 s limit) |
| [56](56-projects-page-and-project-context.md) | The projects page and the project context | L | 06, 16, 39 | DONE (2026-11-21, in v0.46.0, the last two e2e commits in v0.47.0; a bundle of a project with no timeline and the context of template and bundle projects are open issues) |
| [57](57-local-account-and-encrypted-workspace.md) | A local account and an encrypted workspace | XL | 04, 05, 07 | DONE (2026-11-24, `9d7275203`..`e29ef6e9b`) |
| [58](58-sound-assets.md) | Sound assets | M | 08, 21 | DONE (2026-11-21, in v0.46.0; peaks of a sound over 1 GiB of WAV wait for a peaks job, VV-335) |
| [59](59-outbound-webhooks.md) | Outbound webhooks | L | 04, 05, 30 | DONE (2026-11-21, in v0.47.0; a real public HTTPS receiver not verified) |
| [60](60-video-editor-polish.md) | Video editor polish: dark menus, a wider sidebar, rename, the history sidebar | M | 08, 21 | DONE (2026-11-23, `0987b275e`..`96891b65d`) |

### 1.4 Closing (after the tracks)

| Epic | Title | Size | Depends on | Status |
| --- | --- | --- | --- | --- |
| [38](38-first-complete-flow-pilot.md) | Pilot of the first complete flow | M | 16, 19, 20, 28, 35 | IN PROGRESS (tasks 1 to 3 and the skill fixes merged 2026-11-20, 919a32d8b..e045447db, merge 59722df91; tasks 4 and 6, the paid agent runs, wait for the author's budget) |
| [39](39-recipes-for-commercials-and-long-videos.md) | Recipes for commercials and long videos | M | 33, 38 | DONE (2026-11-21, c9e92558c..34f52a00c, merge 96f77dc1e; the Portuguese guides are in docs/guides/pt-BR) |
| [40](40-documentation-site.md) | Documentation site for Stuarty | L | 02, and each section it documents | DONE (2026-11-21, 3228d086c..59aadcdd9 (replayed from c5f344a07 to reword one subject), merge b97638650; English site with Portuguese guides in docs/guides/pt-BR (D-103); the site domain and an idle-machine timing wait for the author) |
| [41](41-accessibility.md) | Accessibility of every section | M | 07, 28, 37 | DONE (2026-11-20, released in v0.42.0; criterion 7, the pass with a real screen reader, is the author's and stays not verified) |
| [42](42-quality-campaigns.md) | Quality campaigns: budgets, fuzz, crash, soak, security | L | every epic above | IN PROGRESS (tasks 1, 2, 5, 6, 7 merged in v0.43.0; idle budgets measured 2026-11-20, `docs/perf/2026-11-20-windows.md`: every NFR passes but NFR-061, which epics 54 and 55 own; soak of 20 minutes passed, `docs/quality/2026-11-20-soak.md`, the 4-hour run is still owed; no-egress on Linux and the security review session wait) |

### 1.5 Waiting for something outside the team

Written so that they can start the day their condition is met. They do not hold the QA campaign.

| Epic | Title | Size | Waits for | Status |
| --- | --- | --- | --- | --- |
| [43](43-desktop-on-macos-and-linux.md) | The desktop on macOS and Linux | M | real macOS and Linux machines | BLOCKED (no hardware) |
| [44](44-reddit-connector.md) | Reddit | S | Reddit approving the author's application | BLOCKED (Reddit approval) |
| [45](45-post-metrics.md) | Metrics of published posts | M | 19, 20, and the author's go | TODO (after publishing is solid) |
| [46](46-own-ffmpeg-build.md) | The own FFmpeg build, signed and adopted | L | a release to carry the assets (the H.264 question is settled: author decision D-087) | IN PROGRESS (merged 2026-11-25, `004c26801`..; reproducible LGPL build for Linux and Windows amd64, VV-231; the signed publication and the `.config/ffmpeg.env` switch wait for the author and CI, epic 98) |
| [47](47-one-renderer-everywhere.md) | One renderer everywhere: remove the Go compositor and PixiJS | M | 26, 43 | BLOCKED (three-system proof) |

### 1.6 Screen recording and local AI

The order, the critical path and the groups are in `docs/planning/screen-recording/roadmap.md`. These epics reserve SDD-025 (recording) and SDD-024 (local AI). ADR-0049, ADR-0050 and ADR-0051 are proposed.

| Epic | Title | Size | Depends on | Status |
| --- | --- | --- | --- | --- |
| [61](61-recording-foundation-windows-capture.md) | Recording foundation and Windows screen capture | XL | 57, the redesign choice (its spike needs neither) | IN PROGRESS (merged 2026-11-24, `ddb10a3f6`..`2af563b39`; AC6, the NFR-067 budget, waits for a run on an idle machine) |
| [62](62-windows-recording-audio-webcam-events.md) | Windows recording: audio, webcam, pause, cursor and click events | L | 61 | DONE (2026-11-24, `1deedb84c`..`4385b96bf`; the 10-minute drift fixture ran 100 s and real devices are untested, D-144) |
| [63](63-basic-web-recording.md) | Basic web recording | M | 61 | DONE (2026-11-25, `7bf6d9c67`..`51668e588`; Firefox and WebKit runs reported by the hand-off) |
| [64](64-agent-operation-of-recording.md) | Agents in the recording flow | L | 61, 62 | DONE (2026-11-25, `a77a827b6`..`91d977ddc`; threats T-165 to T-167; the tray indicator check by eye and the MCP actions wait for the author, QUESTIONS) |
| [65](65-auto-zoom-smooth-cursor-click-highlight.md) | Automatic zoom, smooth cursor and click highlight | L | 62 | DONE (2026-11-25, `58d315aa9`..`ad5c8c0cf`) |
| [66](66-recording-layouts-backgrounds-teleprompter.md) | Recording layouts, backgrounds and teleprompter | M | 62 | DONE (2026-11-25, `b449de842`..`8daaa8d7b`; threat T-170; no gradient or radius, which need the renderer, and the Wails window capture check wait for the author) |
| [67](67-macos-capture.md) | Recording on macOS | L | 62, 43, the signing decision | TODO |
| [68](68-linux-capture.md) | Recording on Linux | L | 62, 43 | TODO |
| [69](69-edit-by-text-and-filler-words.md) | Edit by text, filler words and repetitions | L | the transcription of today | DONE (2026-11-24, `55e5e49fd`..`a003450b9`) |
| [70](70-local-ai-helper-and-vertical-cuts.md) | Local AI helper and face-aware vertical cuts | XL | ADR-0050, 69 | DONE (2026-11-24, `6197306a2`..`3d6c77676`; a real install waits for the author to publish the helper asset) |
| [71](71-caption-styles-blur-studio-voice.md) | Caption styles, sensitive-data blur and studio voice | M | — | DONE (2026-11-24, `c7e019a79`..`5af33ac99`; blur at the frame edge is VV-392) |
| [72](72-optional-local-models.md) | Optional local models: background removal and depth | L | 70, the BEN2 licence decision | TODO |
| [73](73-hyperframes-end-to-end-round.md) | A real end-to-end round with HyperFrames | S | 33 | DONE (2026-11-23, `e280de049`..`3a9a43dad`) |

### 1.7 Implementation of the final redesign

Author decisions D-134 and D-137. Epic 74 can start now, and so can epic 87, because their modules do not intersect epics 57 and 69. The screens of recording, transcript, caption styles, blur, voice and local models (frames 24 to 31) are built by epics 61 to 72 on the components of this section. Frame 32 is built by epic 85.

| Epic | Title | Size | Depends on | Status |
| --- | --- | --- | --- | --- |
| [74](74-ui-foundation-tokens-and-components.md) | UI foundation: tokens, type and base components | L | — | DONE (2026-11-24, `8d92f0128`..`553c21580`) |
| [75](75-app-shell-sidebar-topbar-scene-bar.md) | App shell: sidebar, top bar, scene bar and notification center | L | 57, 74 | DONE (2026-11-24, `cd4d39254`..`fbd5d82de`) |
| [76](76-agent-activity-read-model.md) | Agent activity: skill, skill state, tool and plain description | L | 57, 75 | DONE (2026-11-24, `6b54fa388`..`c0d8bc9ee`) |
| [77](77-unified-requests-read-model.md) | One read of everything that waits for a person | L | 57, 75, 76 | DONE (2026-11-24, `94b13b716`..`b9e39139f`) |
| [78](78-project-kind-and-workflow.md) | Project kind and the workflow per kind | L | 57 | DONE (2026-11-24, `133faef99`..`254727332`) |
| [79](79-home-projects-and-briefing-screens.md) | Home, projects, briefing and the workflow screens | L | 74, 75, 78 | DONE (2026-11-24, `e079a542f`..`7c8207951`) |
| [80](80-writing-screens.md) | Writing screens and the text flow | M | 74, 75, 78 | DONE (2026-11-24, `924fc7f69`..`e419f0610`) |
| [81](81-asset-library-screens.md) | Asset library screens | M | 74, 75 | DONE (2026-11-24, `411906afe`..`6cca1e337`, v0.50.0) |
| [82](82-publishing-and-approvals-screens.md) | Publishing and approvals screens | L | 74, 75, 77, 78 | DONE (2026-11-24, 642ef4944..f720fe476) |
| [83](83-chat-skills-prompts-memory-screens.md) | Chat, skills, prompts and memory screens | M | 74, 75, 76 | DONE (2026-11-24, `6f7242b77`..`681b2a2ee`) |
| [84](84-connections-settings-account-screens.md) | Connections, settings, account and error screens | L | 57, 74, 75 | DONE (2026-11-24, `ab16d2ce6`..`8a373f993`) |
| [85](85-video-editor-screens.md) | Video editor redesign | XL | 69, 74, 75, 76 | DONE (2026-11-24, `3c77c564e`..`b50da822a`) |
| [86](86-image-editor-screens.md) | Image editor redesign | L | 74, 75, 76 | DONE (2026-11-24, `6d6cc2aef`..`a0afe65ce`) |
| [87](87-site-navbar-and-docs.md) | Site navbar and documentation design | M | — | DONE (2026-11-23, `dffbeea6b`..`56ec38048`) |
| [88](88-landing-page-redesign.md) | Landing page redesign | M | 75, 76, 78, 79, 87 | DONE (2026-11-24, `fb63feaa7`..`e57babc77`) |

At five seniors: first 74 and 87, beside 57 and 69. After 57 and 74: 75 and 78, which do not share modules. After 75: 76, 81 and 84. After 76 and 78: 77, 79, 80, 83 and 86, at most five at a time. Then 82 and 85, and 88 last. Epics 79 and 80 share no page; 85 and 86 share no editor module.

### 1.8 Follow-ups of author decision D-135

Work that earlier epics left open and the author decided to do in epics of their own. None of them is in the dependency diagram of section 2, which stops at epic 42. Epics 91 and 93 both change the embedded renderer module, so 93 takes the final digest after 91; 89 and 85 both touch the transcript panel, and 92 and 84 both touch the Settings account section and `web/resources/lang/*.json`, so the orchestrator orders those merges.

| Epic | Title | Size | Depends on | Status |
| --- | --- | --- | --- | --- |
| [89](89-cut-edges-snap-to-silences.md) | Cut edges snap to silences | S | 69 | DONE (2026-11-24, `7e5eba1e9`..`6e3eb8f03`) |
| [90](90-real-crash-recovery-campaign.md) | Real crash recovery: kill a real process in a publication, an upload and an export | M | 42, 18, 20, 35 | DONE (2026-11-24, `efa8f76b1`..`b2118645a`; the Linux run of criterion 1 needs a container pull) |
| [91](91-renderer-layer-band-and-frame-cache.md) | Image renderer: prepare only the visible band, keep the prepared layer, cache the editor frame | L | 54, 55 | IN PROGRESS (merged 2026-11-25, `e4abcb90e`..`76698ab58`: bands of equal work, the prepared layer kept when turned, the ImageFrame cache; the idle NFR-061 measurement, task 7, and the full contract run, task 8, are owed) |
| [92](92-encrypted-library-and-remaining-sealed-fields.md) | The encrypted library and the rest of what epic 57 left in clear text | XL | 57, 56, 08, 15 | DONE (2026-11-24, `ec1b7c3d0`..`89d537725`) |
| [93](93-renderer-wasm-ci-artifact-and-linux-digest.md) | One renderer module digest on Linux and Windows | M | 55, 91 | IN PROGRESS (merged 2026-11-25, `3d8ccf9e4`..`594117d19` in v0.56.0 and `66faaee69`..`f5507684c`; one digest `5732776…c1b44` built on Windows equals the Linux artifact; task 6 and the Linux runs of criteria 1 to 3 wait for CI, ADR-0045) |
| [94](94-asset-storage-size-reveal-and-rename.md) | Asset storage size, show in folder and rename, with their edge cases | M | 57, 81 | DONE (2026-11-24, `74889c205`..`68a6a822a`) |
| [95](95-landing-page-matches-the-final-design.md) | Landing page that matches the final design, with themed captures and image zoom | M | 88 | DONE (2026-11-25, `d74b0636a`..`08ea98d2a`; v0.55.0 and v0.56.0) |
| [96](96-product-screens-match-the-final-design.md) | Product screens that match the final design, and the landing captured again | M | 95 | DONE (2026-11-25, product screens and the landing captured again on v0.56.0) |
| [97](97-publishing-calendar-day-view.md) | Publishing calendar: a day of its own, notes, quiet days, recurring slots, duplicate and .ics | L | 20, 82, VV-282 | DONE (2026-11-25, `ef6a3b420`..`ae43d4d16`; threats T-180 to T-183; Firefox and WebKit runs of its specs and a real calendar import not run) |
| [98](98-ship-and-adopt-the-own-ffmpeg.md) | Ship the own FFmpeg build in the installers and adopt it | M | 46 | IN PROGRESS (tasks 1 to 4 merged 2026-11-25, `8b7fb80aa`; the setup compiled with the own FFmpeg, a real install and tasks 5 and 6 wait for CI and the author) |
| [99](99-finish-the-final-design.md) | Finish the final design where the screens are still partial | L | 96, 98 | DONE (2026-11-25, `03f100d72`..`a7f70d4ec`; groups A, B, C and D merged; landing and README recaptured) |

### 1.9 Product feedback of 2026-11-27 (author decision D-160)

Epics created from the author's real-world testing of v0.60.0 (`stuarty-desktop-0-60-0.exe` and `stuarty-0-60-0.exe`) and the web feature-gap research against Canva, Photoshop, CapCut, Premiere Pro and DaVinci Resolve.

At five seniors (respecting shared files in `.agents/rules/plans.md` section 4, especially `web/resources/lang/*.json`, `docs/sdds/000-shared-definitions.md` and `renderer/`):

- **Wave A (start immediately, disjoint modules):** `116` (preserve FINAL FigCraft design in `docs/design/final/` — small and fast so all subsequent UI epics edit FigCraft cleanly), `100` (desktop/engine Windows processes & export), `102` (critical UI bugs), `108` (docs/site Docker `:latest` & admin defaults), `109` (AI image/video providers).
- **Wave B (after Wave A):** `101` (installer naming & desktop updater, after `100` and `102`), `103` (login & recovery polish, coordinate `SignIn.tsx` with `101`), `104` (back button, double-click & workflow dots, after `102`), `105` (standalone video & image editors, after `116`), `106` (project-independent asset imports & library copy modal, after `116`).
- **Wave C:** `107` (OAuth-first agent connections; task 1 stops for author approval of ADR-0054), `110` (Google Drive, Figma & open-source creative MCPs, after `106`), `111` (global & per-project Activity History, after `116`), `117` (dedicated `/record` screen recording studio page & sidebar entry, after `105` and `116`), `114` (image editor Canva/Photoshop parity — owns `renderer/`).
- **Wave D (closing & sequential `renderer/` / `lang/*.json`):** `115` (video editor CapCut/Premiere/Resolve parity — after `114` so only one epic touches `renderer/` at a time), `118` (professional Sound Editor `/edit-sound` & SDD-026, after `105`, `106` and `116`), `112` (universal input placeholders, tooltips & help cards — after new UI forms land), `113` (100% Portuguese error localization — coordinate `lang/*.json` merge sequentially with `112`).

Every UI epic in this section updates `docs/design/final/stuarty-redesign-final-trilha.figcraft` (preserved by epic 116) for any new screen or layout change.

| Epic | Title | Size | Depends on | Status |
| --- | --- | --- | --- | --- |
| [100](100-windows-silent-processes-and-export.md) | Windows silent child processes, FFmpeg shim resolution and reliable video export | M | 46, 98 | DONE (2026-11-27, `2f46b534a`..`430667880`; Scoop shim verified live + `TestExportWithTheDetectedEncoder` and desktop `-H windowsgui` build pass) |
| [101](101-installer-naming-and-desktop-updater.md) | Explicit web-localhost installer naming and desktop in-app auto-updater | M | 100 | DONE (2026-11-27, `117de27ca`..`d8928a1aa`; `stuarty-web-localhost-X-Y-Z.exe`/`.AppImage`, update bridge, Login + Sidebar controls, Go/Vitest/Playwright verified) |
| [102](102-critical-ui-bug-fixes.md) | Critical UI bug fixes: workflow navigation, collapsed sidebar version and empty-state layout | S | 75, 79, 99 | DONE (2026-11-27, `3d8d10821`..`bc1cb73bd` + Playwright E2E) |
| [103](103-login-and-recovery-polish.md) | Login, password recovery and account switching polish | S | 57, 84 | DONE (2026-11-27, `092916646`..`5dbf40e8c` + Playwright `account` E2E) |
| [104](104-navigation-and-shortcuts.md) | Global back navigation, double-click shortcuts and clickable workflow steps | M | 102 | DONE (2026-11-27, `34753f3c0`..`ae83cd931` + Playwright E2E) |
| [105](105-standalone-video-and-image-editors.md) | Standalone video and image editors without a pre-created project | L | 06, 28, 85, 86, 116 | TODO |
| [106](106-project-independent-asset-imports.md) | Project-independent drag-and-drop asset imports, library copy prompt and project allocation details | L | 08, 81, 94, 116 | TODO |
| [107](107-oauth-first-agent-connections.md) | OAuth-first agent connections and secondary API key mode | L | 12, 14, 83, 84, 116 | TODO |
| [108](108-docs-docker-latest-and-admin-defaults.md) | Documentation and website Docker `:latest` image and simplified default admin credentials | S | 25, 40, 87 | DONE (2026-11-27, `15cb1a5cc`..`35402cc38`) |
| [109](109-native-ai-media-providers.md) | Native AI image and video generation providers (Higgsfield, fal.ai, Replicate, Runway, Kling, Luma, FLUX, Ideogram, Recraft and ElevenLabs) | L | 10, 11, 84, 116 | TODO |
| [110](110-drive-figma-and-creative-mcps.md) | Native Google Drive, Figma and open-source creative MCP connectors (ArtCraft, OpenPencil and custom MCPs) | L | 04, 84, 106, 116 | TODO |
| [111](111-global-and-project-history.md) | Global and per-project Activity History | M | 06, 75, 79, 116 | DONE (2026-11-27, `8be3ef38e`..`669f44880`; global `/history` + per-project feed, HTTP/CLI `activity`/MCP `history action=activity`, FigCraft 34a/34b, Go/security/contract/surfacebudget/Vitest/Playwright 21/21 verified) |
| [112](112-universal-input-help.md) | Universal input placeholders, contextual tooltips and explanatory help cards | M | 74, 99, 116 | DONE (2026-11-27, `e36f5b851`..`0d1a59096` + Task 5; `FieldHelp.tsx`, 100% EN/pt-BR placeholders + tooltips + help cards across 367 files, `check:input-help` 0 findings, Vitest 1831/1831, Playwright a11y 36/36 verified) |
| [113](113-portuguese-error-localization.md) | Complete Portuguese localization of all error messages, hints and diagnostics | M | 74, 99 | DONE (2026-11-27; `localizeError`, 60 `E_*` + 45 `L_*` + 8 `X_*` + job phases + `check:error-i18n` + Playwright `pt-BR` E2E verified) |
| [114](114-image-editor-canva-photoshop-parity.md) | Image editor parity with Canva and Photoshop: Magic Resize, gradients, curved text, smart guides, vector frames and AI retouching | L | 27, 28, 29, 30, 86, 91, 116 | TODO |
| [115](115-video-editor-capcut-premiere-resolve-parity.md) | Video editor parity with CapCut, Premiere Pro and DaVinci Resolve: speed ramps, stabilization, motion tracking, creator transitions, color wheels and beat-sync montage | L | 26, 31, 34, 53, 69, 71, 85, 114, 116 | TODO |
| [116](116-figcraft-design-source-in-docs.md) | Preserve the FINAL FigCraft design system and frames in `docs/design/final/` | S | 99 | DONE (2026-11-27, `a5e2e2086`) |
| [117](117-screen-recording-ui-and-record-page.md) | Screen recording UI: dedicated `/record` studio page, sidebar entry and standalone polish flow | L | 61, 62, 63, 65, 66, 75, 105, 116 | TODO |
| [118](118-professional-sound-editor.md) | Professional Sound Editor: multi-track audio core, waveform and spectrogram UI, mastering FX rack, live recording and multi-format export | L | 34, 51, 58, 71, 105, 106, 116 | TODO |

## 2. Dependency graph

```text
01 ─► 02 ─► 03 ─┬─► 04 ─► 05 ─┬─► 09 ─┬─► 10 ─► 11
                │             │       ├─► 12
                │             │       └─► 17 ─► 23
                │             └─► 18 ─┬─► 19
                │                     ├─► 20 ─┬─► 21
                │                     │       ├─► 22
                │                     │       └─► 23
                │                     └─► 24 ─► 25
                ├─► 06 ─┬─► 08 ─► 10, 33
                │       ├─► 13 ─► 14
                │       ├─► 16, 17, 20, 32
                ├─► 07 ─┬─► 08, 14, 15 ─► 16
                │       └─► 20, 28, 37
                ├─► 26                          (editor track)
                ├─► 27 ─► 28 ─► 30
                │    └─► 29
                ├─► 31 ─► 32
                └─► 34, 35, 36

16, 19, 20, 28, 35 ─► 38 ─► 39        40, 41 ─► 42 ─► QA campaign ─► releases
```

## 3. What runs side by side

At most five seniors at once. Two epics that touch the same modules are never in progress together; each epic names its modules in "Scope". The waves below are the plan; the orchestrator dispatches the next unblocked epic to whoever is free.

| Wave | Platform (seniors A, B, C) | Editor (seniors D, E) |
| --- | --- | --- |
| 0 | 01, then 02, then 03: one senior, the others wait | — |
| 1 | 04, 06, 07 | 26, 27 |
| 2 | 05, 13, 15 | 31, 35 |
| 3 | 08, 09, 18 | 28, 34 |
| 4 | 10, 16, 19 | 29, 36 |
| 5 | 12, 17, 20 | 32, 37 |
| 6 | 11, 14, 24 | 30, 33 |
| 7 | 21, 22, 23 | 38 (when 16, 19, 20, 28 and 35 are done) |
| 8 | 25, 40 | 39, 41 |
| 9 | 42, by the orchestrator with every agent stopped where it measures | — |

Shared files (`go.mod`, `web/routes/`, `web/resources/lang/*.json`, `docs/sdds/000-shared-definitions.md`, this file) are merged one epic at a time (`.agents/rules/plans.md` section 4). Migrations in `web/database/migrations/` take the timestamp of the moment they are merged.

New specifications have their numbers reserved here, so that epics running side by side never pick the same one:

| Specification | Written by |
| --- | --- |
| SDD-014 connections and vault | 04 |
| SDD-015 approval and cost | 05 |
| SDD-016 AI providers | 09 |
| SDD-017 hosted agent | 14 |
| SDD-018 memory, skills and prompts | 15 |
| SDD-019 writing | 17 |
| SDD-020 publishing | 18 (connectors), 20 (publications and the scheduler) |
| SDD-021 image documents | 27 |
| SDD-026 audio documents and Sound Editor | 118 |

## 4. After the epics

1. **The QA campaign** of `.agents/rules/plans.md` section 6 runs in rounds, with the five seniors in parallel, until a full round finds no problem.
2. Then the single pull request is split into releases, starting at v0.30.0, and CI and the branch protection are restored (ADR-0045). That work is planned with the author when it starts.

## 5. What closes the first complete flow

01, 02, 03, 04, 05, 06, 07, 08, 09, 10, 13, 14, 15, 16, 17, 18, 19, 20, 27, 28, 35 and 38. Everything else widens the product.

## 6. Requirement to epic

"Built" means the requirement was delivered by the video editor up to release 0.29.0 and is kept working by every epic.

### 6.1 The author's requirements of 2026-11-18

| Requirement | Epics |
| --- | --- |
| A unified social media manager to schedule and publish (Facebook, Instagram, X, TikTok, Reddit) | 18, 19, 20, 21, 22, 44, 45 |
| Integration with LLMs and AI platforms, with the user's own authentication | 09, 10, 11, 12, 14, 30 |
| 100% self-hosted: desktop on three systems and a VPS with Docker and scripts; Google login; local data | 04, 21, 24, 25, 43 |
| Writing assistance: video scripts, commercial copy, blog posts | 16, 17, 23 |
| Dashboard with a left sidebar and its sections | 07, 08, 15, 20, 28 |
| Finish the video editor, with the most used fifth of After Effects, Audition and Media Encoder | 26, 31, 32, 33, 34, 35, 36, 37 |
| An image editing engine with the most used fifth of GIMP and Photoshop, usable by agents | 27, 28, 29, 30 |
| Everything a person does in the UI possible through the API, the CLI and MCP | 13, and the surfaces task of every epic |

### 6.2 PRD-001: the video editor

| Requirement | Epics |
| --- | --- |
| RF-001 to RF-006, RF-010 to RF-016, RF-020 to RF-029, RF-030 to RF-036, RF-040 to RF-044, RF-050 to RF-053, RF-060 to RF-065, RF-070 to RF-073 | built; RF-001 and RF-004 change meaning with 06 and 08 |
| RF-074 (shortcuts, onboarding, polish) | 37 |
| RF-075 (keyboard and WCAG 2.2 AA) | 37, 41 |
| RF-080 (editable easing curves), RF-082 (animation presets) | 31 |
| RF-081 (animated crop and mask) | built in release 0.28.0; 31 adds the crop control in the inspector, the parity cases and the new easing on top |
| RF-083 (nested compositions) | 32 |
| RF-084 (clip from an external motion engine) | 33 |
| RF-085 (audio processors), RF-086 (noise reduction) | 34 |
| RF-087 (export presets), RF-088 (batch queue), RF-089 (H.265 and AV1) | 35 |
| NFR-001 to NFR-013 | built; measured again by 42; NFR-011 by 26 |

### 6.3 PRD-002: agents

| Requirement | Epics |
| --- | --- |
| RF-100 to RF-119 | built for the video domain; extended to every domain by 13 and by the surfaces task of each epic |
| RF-120 (revert), RF-121 (checkpoints and diff) | 36 |
| RF-130 (data leaves only to a connected destination), RF-137 (no credential returned) | 04 |
| RF-131, RF-133, RF-134 | built; RF-133 extended to network text by 04 and 09 |
| RF-132 (ceilings, now with cost) | 05 |
| RF-135 (approval queue), RF-136 (rules only from a privileged session) | 05 |
| NFR-020 (token budget, revised once) | 13 |
| NFR-021 to NFR-025 | built; checked again by 13 and 42 |

### 6.4 PRD-003: distribution

| Requirement | Epics |
| --- | --- |
| RF-200 (one executable, renamed), RF-208 and RF-221 (migration of the former name) | 02 |
| RF-201 to RF-205, RF-207, RF-210, RF-212, RF-214, RF-215 | built |
| RF-206 (no outbound without a connection), RF-216 (vault), RF-220 (Connections lists every destination) | 04 |
| RF-209 (releases) | after the epics (section 4); the own FFmpeg assets by 46 |
| RF-211 (documentation site) | 40 |
| RF-213 (desktop application) | built on Windows (Wails shell); 01, 21; macOS and Linux by 43 |
| RF-217 (Google account) | 24 |
| RF-218 (VPS installation) | 25 |
| RF-219 (desktop background service) | 21 |
| NFR-040, NFR-041 | 40, 42 |
| NFR-042, NFR-044 | after the epics (section 4) |
| NFR-043 (no GPL) | every epic that adds a dependency; 34, 46 |
| NFR-045 (Windows and Docker for every epic; macOS and Linux apart) | every epic; 43 |
| NFR-046 (no credential without the master key) | 04 |

### 6.5 PRD-004: the creator platform

| Requirement | Epics |
| --- | --- |
| RF-300, RF-303, RF-304 (project as a production) | 06 |
| RF-301, RF-302 (guided briefing) | 16 |
| RF-310 to RF-312 (dashboard and sidebar) | 07 |
| RF-313 (chat panel available everywhere) | 14 |
| RF-320 to RF-323 (asset library) | 08 |
| RF-330 to RF-332, RF-336 to RF-339 (image document, text, templates, export, lint) | 27 |
| RF-333 to RF-335 (adjustments, filters, masks) | 29 |
| RF-340 (operations with a model) | 30 |
| RF-341, RF-342 (image editor UI, brand kit) | 28 |
| RF-350 to RF-352, RF-354 (texts, scripts, captions per network) | 17 |
| RF-353 (blog post) | 23 |
| RF-360 (four ways to connect) | 09 (API key), 12 (the two OAuth flows), 14 (local agent) |
| RF-361 (generation of text, images, voice, video) | 09, 10, 11 |
| RF-362 to RF-365 (cost, ceilings, catalogue, what is never offered) | 05, 09 |
| RF-380 to RF-383 (agent chat) | 14 |
| RF-385, RF-386, RF-390, RF-391 (memory, prompts, search) | 15 |
| RF-387, RF-388 (skills with origin; third-party skills) | 15 |
| RF-389 (curated skills) | 16, 38, 39 |
| RF-400, RF-401 (connections and networks) | 18, 19, 22, 44 |
| RF-402 to RF-406, RF-408 (publication, calendar, approval, result, missed time, cost) | 20; RF-406 also 21 |
| RF-407 (AI disclosure) | 19, 22 |
| RF-420 to RF-423 (blog destinations) | 23 |
| NFR-060 (everything local) | 04, 25 |
| NFR-061, NFR-062 (image performance and determinism) | 27, 29, 42, 54, 55 |
| NFR-063, NFR-064 (one adapter per network or provider; contract tests) | 09, 18 |
| NFR-065 (accessibility of every section) | 41 |
| NFR-066 (a small-model agent completes the flow) | 38 |

### 6.6 Product feedback of 2026-11-27 (author decision D-160)

| Feedback item | Epics |
| --- | --- |
| Explicit web-localhost installer name (`stuarty-web-localhost-X-Y-Z.exe`) | 101 |
| 1. Hide Windows terminal when launching desktop localhost server | 100 |
| 2. Global Back button on every screen | 104 |
| 3. Login lock copy removal, Recover back-to-login button, switch account button | 103 |
| 4. Standalone single-use video and image editors without pre-created projects | 105 |
| 5. Project-independent drag-and-drop/file-picker asset imports, library save, project allocation details, copy-to-library confirmation modal | 106 |
| 6. OAuth/web-first agent connections with API key secondary (no binary path required) | 107 |
| 7. Desktop update-to-latest button on Login and sidebar footer + online auto-update toggle | 101 |
| 8. Docs and site always use `ghcr.io/alexgalhardo/stuarty:latest` and matching `admin@stuarty.com` credentials | 108 |
| 9. Fix blank white screen when continuing workflow from Home (`/projects/...` -> `/editor/...`) | 102 |
| 10. Fix Windows Terminal / `0x800700e8` Scoop FFmpeg shim pipe failure | 100 |
| 11. Hide product version string in collapsed sidebar | 102 |
| 12. Fix video export `E_EXPORT_FAILED` with encoder fallback and actionable diagnostics | 100 |
| 13. Fix Prompts empty-state step text alignment and border overflow | 102 |
| 14. Remove `"No seu computador. Do seu jeito."` from password recovery footer | 103 |
| 15. Native Higgsfield + popular AI image/video providers, Google Drive, Figma, and open-source creative MCPs (ArtCraft, OpenPencil) | 109, 110 |
| 16. Missing high-demand Canva, Photoshop and video editor (CapCut/Premiere/Resolve) features | 114, 115 |
| 17. Example placeholders and tooltips/help cards on every input across the app | 112 |
| 18. Double-click shortcuts (calendar day, video/image assets) and clickable Home workflow step dots | 104 |
| 19 & 19a. Global History in main sidebar and per-project History inside every project | 111 |
| 20. Translate all remaining English error notices, messages and hints to Portuguese | 113 |
| 21. Preserve FINAL FigCraft design system & frames in `docs/design/final/` and use FigCraft for new UIs | 116 |
| 22. Dedicated Screen Recording UI (`/record` page, sidebar button, Home quick action, polish take flow) | 117 |
| 23. Professional Sound Editor (`/edit-sound`, multi-track core, waveform/spectrogram, mastering FX rack, LUFS) | 118 |

## 7. What happened to the earlier plans

The plans 001 to 034 were removed on 2026-11-18 (author decision D-073). Each was read again before this roadmap was written.

| Earlier plan | Now |
| --- | --- |
| 001 studio architecture records | replaced by ADR-0031 to ADR-0047 and PRD-004 |
| 002 `studio/` plugin, skill lint, generated editor skill, router skill | 15 (skill lint, generated skills, plugin packaging) and 16 (the router and the curated set) |
| 003 production folder and learnings memory | 06 (the production is the project, the briefing is its brief) and 15 (memory, with the recall and retro skills in 16) |
| 004 meme cut pilot with a small model | 38 (the pilot), 35 (the platform limits as export presets) |
| 005 agent-side provider skills, rights and cost ledger | 08 (origin and rights of an asset), 05 and 09 (cost), 10 and 11 (generation) |
| 006 clips from links and an evergreen clip library | 08 (reusable assets with source, rights note, tags and uses) and 16 (the agent-side `fetch-clip` skill; the downloader is never a product dependency) |
| 007 revert to a revision; 009 checkpoints and a diff | 36 |
| 008 HyperFrames and Remotion as engines, and the hand-off | 33 for HyperFrames and for any external render. Remotion is not carried over: its licence is commercial (ADR-0027, ADR-0042) |
| 010 recipes for product promos and authorial videos | 39 |
| 011 finish the Rust renderer | 26 (the preview on the renderer), 42 (budgets), 47 (remove the duplicates after the three-system proof) |
| 012 home dashboard with an agent chat | 07 (dashboard), 08 and 15 (its pages), 14 (the chat). The "inbox" conversation model is not carried over: the chat hosts the user's agent (ADR-0037) |
| 020 UI performance on the reference machine; 021 goldens and export performance; 025 core fuzz, crash and soak | 42, on Windows; the other two systems with 43 |
| 022 security acceptance, fuzz and the independent review | 04 (the threat model for outbound), 42 (the acceptance run) |
| 023 agent surface budgets and the harness; 034 model ladder pilot | 13 (budgets) and 38 (the runs with a small model) |
| 024 first export in ten minutes and the site audit | 40 |
| 026 own FFmpeg build pipeline; 027 adopt it | 46 |
| 028 screen reader pass and WCAG | 41 |
| 029 desktop on real machines and the update feed | 43 |
| 030 CI gates; 031 release 1.0 | not epics: CI and releases return after the QA campaign (section 4, ADR-0045) |
| 032 keyboard shortcuts and final UI | 37 |
| 033 animated crop and mask | already built in release 0.28.0 (the plan was stale); 31 proves it and finishes the inspector and the parity cases (author decision D-065) |
