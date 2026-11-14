# Sources of the research

> Every URL cited in docs/search/, with the files that cite it and what it supports. All were accessed on 2026-11-14. Generated from the documents by script and reviewed; a URL listed here is not a claim that the page was verified beyond what the citing file states.

Total: 571 distinct URLs in 24 files.

- [vukem and the existing application](#vukem-and-the-existing-application) (17)
- [Reference editors](#reference-editors) (327)
- [Engine, formats and post-mortems](#engine-formats-and-post-mortems) (102)
- [Browser platform and libraries](#browser-platform-and-libraries) (19)
- [Backend, desktop and tooling](#backend-desktop-and-tooling) (23)
- [MCP and agent tool design](#mcp-and-agent-tool-design) (39)
- [Licensing and patents](#licensing-and-patents) (30)
- [Platform specifications](#platform-specifications) (9)
- [Other](#other) (5)
- [Sources that did not open](#sources-that-did-not-open) (5)

## vukem and the existing application

| URL | Supports | Cited in |
| --- | --- | --- |
| <https://github.com/AlexGalhardo/vukem> | vukem repository, cloned with depth 50 at HEAD f8fefef for the vukem notes. | `notes/vukem.md` |
| <https://github.com/AlexGalhardo/vukem/blob/main/docs/tooling.md> | vukem docs/tooling.md, the source for the versioning rules. | `notes/vukem.md` |
| <https://github.com/AlexGalhardo/vukem/commit/> | Commit URL prefix used as a link-format example in the vukem notes, not a specific commit. | `notes/vukem.md` |
| <https://github.com/AlexGalhardo/vukem/commit/032c2d83c4da88fa1826b6db26fd8fef18ba81f2> | vukem commit for the toolchain image base digest and Bun checksums in starterdeps. | `notes/vukem.md`, `vukem-practices-and-releases.md` |
| <https://github.com/AlexGalhardo/vukem/commit/06d52d948e8e7826b413558dafec586363b21fc7> | vukem commit scaffolding the personal finance SaaS example. | `notes/vukem.md`, `vukem-practices-and-releases.md` |
| <https://github.com/AlexGalhardo/vukem/commit/1586a42c3c0a4d101c8159e9f0b065b491cb916a> | vukem commit adding an idempotent, region-aware CreateBucket to the s3 module. | `notes/vukem.md` |
| <https://github.com/AlexGalhardo/vukem/commit/1d5909feb3c3064cc3819828768b117e99f4b91b> | vukem commit resolving both paths when vukem upgrade checks a go install binary. | `notes/vukem.md`, `vukem-practices-and-releases.md` |
| <https://github.com/AlexGalhardo/vukem/commit/6439ee896c88f31a81bf4249401b9b8c09e2f3fd> | vukem commit scaffolding the to-do example app in vukem new. | `notes/vukem.md`, `vukem-practices-and-releases.md` |
| <https://github.com/AlexGalhardo/vukem/commit/65fdd8d2fe95da03edefc19324a816beeb10eb71> | vukem commit: the CLI never downgrades to an older latest release. | `notes/vukem.md`, `vukem-practices-and-releases.md` |
| <https://github.com/AlexGalhardo/vukem/commit/830fccca76e44f467ffd4c2ebd26b7380db43bbf> | vukem commit adding vukem upgrade to update the CLI itself. | `notes/vukem.md`, `vukem-practices-and-releases.md` |
| <https://github.com/AlexGalhardo/vukem/commit/8b4ac793c77405f097e6e45c71c01e8740fea3ee> | vukem commit moving project upgrade to vukem update and updating Go and JS dependencies. | `notes/vukem.md`, `vukem-practices-and-releases.md` |
| <https://github.com/AlexGalhardo/vukem/commit/be34f8c27d21bffd3463cdeb30bc3fbecc952685> | vukem commit registering upgrade as the CLI self-update. | `notes/vukem.md`, `vukem-practices-and-releases.md` |
| <https://github.com/AlexGalhardo/vukem/commit/d419e35107a188e7673ab894b1a36bc2688a7395> | vukem commit keeping scaffolded dependencies on the latest stable release (starterdeps). | `notes/vukem.md`, `vukem-practices-and-releases.md` |
| <https://github.com/AlexGalhardo/vukem/commit/d930d79a09cf3faaa5bb84f6c159ae743a4fe752> | vukem commit adding row-level multi-tenancy middleware, context and ORM scope. | `notes/vukem.md`, `vukem-practices-and-releases.md` |
| <https://github.com/AlexGalhardo/vukem/commit/ff4774e02532152507c657510255f665ab848c24> | vukem commit covering images, actions, package managers and docs in starterdeps. | `notes/vukem.md`, `vukem-practices-and-releases.md` |
| <https://github.com/AlexGalhardo/vukem/releases/tag/v0.43.0> | vukem v0.43.0 GitHub release, cited as the release reference. | `notes/vukem.md` |
| <https://vukem.dev/docs/introduction> | vukem.dev docs introduction, cited among the sources the vukem notes read. | `notes/vukem.md` |

## Reference editors

| URL | Supports | Cited in |
| --- | --- | --- |
| <http://freecut.net> | Homepage field of the freecut GitHub repo, used to confirm the project identity. | `projects/freecut.md` |
| <http://freecut.net/> | README of the freecut repo links to its site at freecut.net. | `projects/freecut.md` |
| <https://api.github.com/repos/omni-media/omniclip> | GitHub API record for the omniclip repo, fetched with gh api. | `projects/omniclip.md` |
| <https://bestmediainfo.com/mediainfo/mediainfo-digital/canva-launches-creative-operating-system-with-design-ai-video-20-and-all-new-affinity-10608968> | Press coverage of Canva's creative OS launch, not an official source. | `projects/canva.md` |
| <https://canva.dev/blog/developers/canva-dev-mcp-server> | Canva developer blog post on the Dev MCP Server. | `projects/canva.md` |
| <https://canva.dev/docs/connect/mcp-server> | Canva Connect and Dev MCP Server docs, cited for Canva's API surface. | `projects/canva.md` |
| <https://clipchamp.com/en/blog/free-1080p-exports/> | Clipchamp blog on free exports, cited with the pricing page for premium stock content (via search). | `projects/clipchamp.md` |
| <https://clipchamp.com/en/pricing/> | Clipchamp pricing page; premium stock content in a free project must be removed or paid for. | `projects/clipchamp.md` |
| <https://clipchamp.com/en/whats-new> | Clipchamp what's-new page listing features such as grouping and the speed controller (no dates). | `projects/clipchamp.md` |
| <https://clipchamp.com/pt-br/> | Clipchamp Portuguese homepage, cited for Windows and Mac recording and Store availability. | `projects/clipchamp.md` |
| <https://discord.gg/Nr8t9s5wSM> | Discord invite named as omniclip's main channel; the note says it was not accessible. | `projects/omniclip.md` |
| <https://fluxnote.io/guides/canva-video-editor-review> | Fluxnote review of the Canva editor: basic effects and no chroma key on any plan (one review, to verify). | `projects/canva.md` |
| <https://fluxnote.io/guides/canva-video-pricing-guide-2026> | Fluxnote 2026 pricing guide, a secondary source for a Canva plan price; the note says not to use it unverified. | `projects/canva.md` |
| <https://freecut.net/docs> | freecut user guide, named in the README as the docs link. | `projects/freecut.md` |
| <https://github.com/Augani/openreel-video> | Augani openreel-video repo, rated as a reference closest to the stack (via gh). | `notes/stack-browser.md` |
| <https://github.com/Hommy-master/capcut-mate> | Third-party repo that generates CapCut/Jianying drafts, cited in the CapCut notes. | `projects/capcut.md` |
| <https://github.com/OpenCut-app/OpenCut> | OpenCut repo; the lowercase URL in the notes redirects to this canonical one. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/blob/main/LICENSE> | OpenCut MIT license file, cited for the license terms. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/blob/main/README.md> | OpenCut README, cited for opencut-classic being the one to use today. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/blob/main/apps/desktop/README.md> | OpenCut desktop README, cited for the early-stage status of the desktop app. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/blob/main/changelog/0.3.0.md> | OpenCut 0.3.0 changelog, cited for the compositor, effects and masks added in that release. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/blob/main/crates/media/setup/ffmpeg.json> | OpenCut ffmpeg.json setup file, cited for the LGPL-2.1-or-later lgpl-shared build. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/485> | OpenCut issue #485 on custom fonts. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/594> | OpenCut issue #594: excessive RAM and CPU use on a large file (50 min, 2K video). | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/628> | OpenCut issue #628: out of memory, with a Chrome crash. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/637> | OpenCut issue #637: a plain mp4 from Clipchamp cannot be imported (WebCodecs codec issue). | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/719> | OpenCut issue #719: project JSON for programmatic workflows. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/778> | OpenCut issue #778: headless render support. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/811> | OpenCut issue #811, cited for the roadmap saying transitions and filters are coming. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/812> | OpenCut issue #812: exporting large videos kills the process (3 GB video). | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/817> | OpenCut issue #817 on CJK text. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/822> | OpenCut issue #822 on text, which does not change the source or content. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/826> | OpenCut issue #826 on RTL text. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/827> | OpenCut issue #827: request for an API, CLI or headless mode for agents. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/867> | OpenCut issue #867: state held in Zustand stores, tangled with React. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/868> | OpenCut issue #868: RFC for a local-first agent bridge (third party). | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/873> | OpenCut issue #873: rename and delete races can lose data. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/881> | OpenCut issue #881: web versus desktop focus, with browser limits. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/898> | OpenCut issue #898: captions and karaoke data model. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/912> | OpenCut issue #912: clip audio disappears on export. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/914> | OpenCut issue #914, cited as the source for the export holding everything in memory. | `engine.md`, `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/OpenCut-app/OpenCut/issues/915> | OpenCut issue #915: sound export issue. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic> | OpenCut classic repo, the old code that the README points to as the one to use today. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/> | Pinned link base of the classic repo at commit cf5e79e, used as the abbreviation base. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/AGENTS.md> | OpenCut classic AGENTS.md at the pinned commit, cited as a FACT from the repo. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/Cargo.toml> | OpenCut classic Cargo.toml at the pinned commit, cited as a manifest. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/package.json> | OpenCut classic web package.json at the pinned commit, cited as a manifest. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/animation/types.ts> | OpenCut classic keyframe types, cited for the keyframes model. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/commands/base-command.ts> | OpenCut classic base command class, cited for the command pattern. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/commands/batch-command.ts> | OpenCut classic batch command, cited for ripple being applied through a batch. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/core/managers/audio-manager.ts> | OpenCut classic audio manager, cited for the timeline audio mixing path. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/core/managers/commands.ts> | OpenCut classic command manager; BatchCommand groups commands and selection is saved per entry. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/media/audio.ts> | OpenCut classic audio media code, cited for offline export mixing in createTimelineAudioBuffer. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/media/waveform-summary.ts> | OpenCut classic waveform summary, cited for RMS buckets rendering only the visible part. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/project/types.ts> | OpenCut classic project types, cited for the Project model. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/services/renderer/canvas-renderer.ts> | OpenCut classic canvas renderer, cited for the compositor drawing to an output canvas. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/services/renderer/compositor/wasm-compositor.ts> | OpenCut classic WASM compositor file, cited in the renderer description. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/services/renderer/scene-exporter.ts> | OpenCut classic scene exporter, cited for BufferTarget holding everything in memory. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/services/storage/migrations/AGENTS.md> | OpenCut classic migrations AGENTS.md, cited for the policy of not replacing persisted data so down-migrations work. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/services/storage/migrations/index.ts> | OpenCut classic migrations index, cited for the v0-to-v1 through v30-to-v31 migrations with tests and fixtures. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/services/storage/migrations/runner.ts> | OpenCut classic migration runner, cited with the migration policy. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/services/storage/service.ts> | OpenCut classic storage service, cited for the storage layout and the persistent storage request. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/services/storage/types.ts> | OpenCut classic storage types file, cited in the storage notes. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/services/transcription/worker.ts> | OpenCut classic transcription worker, cited for the Web Worker used only for Whisper. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/services/video-cache/service.ts> | OpenCut classic video cache service, cited for seek generations that discard obsolete seeks (no proxies). | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/timeline/types.ts> | OpenCut classic timeline types, cited as the real timeline types. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/transcription/caption.ts> | OpenCut classic caption module, cited for captions built from CaptionChunk arrays with word-based chunking. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/docs/actions.md> | OpenCut classic actions doc, cited for the UI calling invokeAction. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/notes/primitives-vs-domains.md> | OpenCut classic design note on primitives versus domains, cited as an opinion. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/package.json> | OpenCut classic root package.json, cited as the monorepo manifest. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/rust/crates/gpu/Cargo.toml> | OpenCut classic gpu crate manifest, cited for the webgl feature and wasm-bindgen version. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/rust/crates/time/src/frame_rate.rs> | OpenCut classic frame_rate.rs, cited for its frame-rate handling. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/rust/crates/time/src/media_time.rs> | OpenCut classic media_time.rs, cited for tick values per frame rate (23.976 = 5005, 29.97 = 4004, 30 = 4000). | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/rust/wasm/Cargo.toml> | OpenCut classic wasm crate manifest, cited with the gpu crate manifest. | `projects/opencut.md` |
| <https://github.com/OpenCut-app/opencut-classic/tree/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/core/managers> | OpenCut classic managers directory, cited for managers instantiated in EditorCore. | `projects/opencut.md` |
| <https://github.com/ashreo/CapCutAPI> | CapCutAPI repo, a FastAPI tool (Apache-2.0) that generates CapCut/Jianying drafts. | `projects/capcut.md` |
| <https://github.com/omni-media/omniclip> | omniclip repo, cited as the project source (repo page and GitHub API). | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/dev/TODO.md> | omniclip dev TODO, cited for multithreading listed as a future goal in the 2.0 compositing engine. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/dev/package.json> | omniclip dev package.json, cited for the MCP, AI assistant and timeline-patch features. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/context/controllers/assistant.ts#L34-L64> | omniclip assistant controller, cited for the revision-mismatch check that returns a fresh context. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/context/controllers/cargo.ts> | omniclip cargo controller, cited with strata.ts for the editor architecture. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/context/controllers/cargo.ts#L7-L20> | omniclip cargo controller lines 7-20, cited for the media handling behind an inference. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/context/parts/strata.ts> | omniclip strata.ts, cited for the central OmniSession and its state layer. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/context/parts/strata.ts#L72> | omniclip strata.ts line 72, cited for mutate and undo/redo snapshots. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/context/parts/strata.ts#L9-L61> | omniclip strata.ts lines 9-61, cited for localStorage and content-addressable media storage. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/context/parts/strata.ts#L90-L96> | omniclip strata.ts lines 90-96, cited for Vault version 1 in project 2.0 (no migrators seen). | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/ui/logic/session.ts#L127-L136> | omniclip session.ts, cited for commitPatch recording a patch as one undo mutation. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/ui/logic/session.ts#L148> | omniclip session.ts line 148, cited for an admitted TODO that undo does not undo rotation correctly. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/dev/s/server/mcp/README.md> | omniclip MCP server README, cited for the stdio server with a WebSocket bridge to the browser tab. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/main/LICENSE> | omniclip main LICENSE, cited for MIT licensing of omniclip, omnitool, webm-hero and kimura. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/main/README.md> | omniclip README, cited for the AI and scripting workflow description. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/main/package.json> | omniclip main package.json, cited as a FACT source (package manifest). | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/main/s/context/context.ts#L117-L125> | omniclip context.ts, cited for AppCore history_limit 64 and the historical actions. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/main/s/context/context.ts#L120-L175> | omniclip context.ts, cited for the state tree and the list of controllers. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/main/s/context/controllers/collaboration/controller.ts> | omniclip collaboration controller, cited for WebRTC peer sync via sparrow-rtc and OPFS file transfer. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/main/s/context/controllers/compositor/controller.ts#L65-L100> | omniclip compositor controller, cited for playback inside Pixi and seek via currentTime. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/main/s/context/controllers/store/store.ts> | omniclip store.ts, cited for localStorage persistence with the omniclip_ prefix. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/main/s/context/controllers/video-export/helpers/FFmpegHelper/helper.ts#L36-L88> | omniclip FFmpegHelper, cited for ffmpeg.exec audio extraction and mixing with amix. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/main/s/context/controllers/video-export/parts/encode_worker.ts#L22-L30> | omniclip encode worker, cited for the WebCodecs H.264 encode and the ffmpeg.wasm AAC re-encode. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/blob/main/s/context/types.ts#L26-L100> | cited without a stated claim. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/commit/cbe581a4a788d9982ffd4a10025c678f9e8707d6> | omniclip commit cbe581a, cited as the last commit on main (2025-08-07, version 1.1.3). | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/commits/dev> | omniclip dev commit history, cited for recent commits from 2026-10-20 to 2026-11-13. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/issues/11> | omniclip issue #11, cited with the helper comment about using ffmpeg only. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/issues/12> | omniclip issue #12: CORS and SharedArrayBuffer/COI failures with ffmpeg.wasm. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/issues/13> | omniclip issue #13, cited with #12 and #28 for SharedArrayBuffer/COI and network failures. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/issues/18> | omniclip issue #18, cited for usability and missing crop and transforms in 1.x. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/issues/20> | omniclip issue #20: animated GIF does not play in the preview or the export. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/issues/21> | omniclip issue #21: ghost media after cancelling an import and reimporting. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/issues/26> | omniclip issue #26: the build does not work on Windows (scripts use cp and rm). | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/issues/28> | omniclip issue #28: the Save Video button does not work after export in Brave. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/issues/3> | omniclip issue #3: suggests OpenTimelineIO instead of a custom JSON format. | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/issues/30> | omniclip issue #30 (closed), cited for the author's dependencies (@benev/turtle and others). | `projects/omniclip.md` |
| <https://github.com/omni-media/omniclip/issues/5> | omniclip issue #5: request for 9:16 vertical resolution. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool> | omnitool repo, cited as active (last commit 2026-11-02) with a worker, Pixi 8 and mediabunny. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/README.md> | omnitool README, cited for the CLI being listed as coming soon. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/package-lock.json#L3490> | omnitool package-lock entry for mediabunny 1.65.0 (MPL-2.0). | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/package-lock.json#L3494> | omnitool package-lock lines 3494 and 1224, cited for the MPL-2.0 terms (weak copyleft). | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/package.json> | omnitool main package.json, cited as a FACT source for omnitool's dependencies. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/driver/driver.ts#L16-L26> | omnitool driver, cited for the worker thread and the Pixi compositor on the main thread. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/driver/fns/schematic.ts#L12-L37> | omnitool schematic, cited for decode and encode in the worker with transferred streams. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/driver/parts/compositor.ts#L31-L80> | omnitool compositor, cited for returning a VideoFrame from the canvas (WebGL memory-leak comment). | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/parts/basics.ts#L11-L18> | cited without a stated claim. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/parts/item.ts#L13-L28> | cited without a stated claim. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/export/parts/audio-mix.ts#L19-L100> | omnitool audio-mix, cited for summing active samples with optional clamping and constant sample rate and channels. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/export/parts/produce-video.ts#L19-L58> | omnitool produce-video, cited for the video export path with links to produce.ts. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/export/parts/resamplers.ts#L1-L65> | omnitool resamplers, cited for the audio click fix at resampling boundaries (commit of 2026-11-01). | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/export/produce.ts#L17-L22> | omnitool produce.ts, cited for export resolution coming from the compositor canvas (no resolution parameter in ExportConfig). | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/export/produce.ts#L24-L52> | omnitool produce.ts lines 24-52, cited for the default webm/vp9 and opus settings and the container list. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/parts/samplers/visual/parts/sample.ts#L9-L60> | omnitool visual sample.ts, cited for samplers computing the Layer list for an instant. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/parts/samplers/visual/parts/video-sink.ts> | omnitool video-sink.ts, cited for mediabunny's VideoSampleSink per hash and the decoded-frame cache. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/parts/schedulers.ts#L77-L96> | omnitool schedulers.ts, cited for export timestamps computed as float milliseconds with fixedStep. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/player/parts/playback.ts#L32-L139> | omnitool playback.ts, cited for currentTime derived from the audio playback start and rate. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/player/player.ts#L36-L45> | omnitool player.ts, cited for flushSeeks and random-access visual samplers. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/sugar/omni.ts#L30-L36> | omnitool sugar/omni.ts, cited for the version field (value 1 in Omni.timeline). | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/types.ts#L3-L38> | omnitool timeline types, cited as a FACT source for the keyframes and time types. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/timeline/utils/datafile.ts> | omnitool datafile.ts, cited for media referenced by BLAKE3 hash instead of path. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/s/units/ms.ts> | omnitool units/ms.ts, cited for millisecond numbers with branded Ms, Fps and Seconds types. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/skills/timeline/SKILL.md> | omnitool timeline SKILL.md, cited for the claim that it lists no core tracks or per-item speed fields. | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/blob/main/skills/timeline/SKILL.md#L8> | omnitool timeline SKILL.md line 8, cited as a version inconsistency (the skill says 0, the code writes 1). | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/tree/main/s/timeline/parts/waveform> | omnitool waveform directory, cited for the Waveform parts (collect, cache, render). | `projects/omniclip.md` |
| <https://github.com/omni-media/omnitool/tree/main/skills/timeline> | omnitool skills/timeline directory, cited as documenting the JSON format, filters and transitions. | `projects/omniclip.md` |
| <https://github.com/opencut-app/opencut> | Lowercase OpenCut repo URL, cited as redirecting to the canonical OpenCut repo. | `projects/opencut.md` |
| <https://github.com/reframe-oss/reframe/blob/375a9f3/> | reframe repo at commit 375a9f3, cited as the base of the links to its files. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/blob/375a9f3/CLAUDE.md> | reframe CLAUDE.md, cited for a bot that only reviews and is unrelated to editor automation. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/blob/375a9f3/README.md> | reframe README, cited for its definition as a tool to resize, trim and convert videos in the browser. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/blob/375a9f3/docs/ARCHITECTURE.md> | reframe ARCHITECTURE.md, cited for the multi-thread core used when the page is cross-origin isolated. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/blob/375a9f3/docs/EXPORT_PROFILES.md> | reframe EXPORT_PROFILES.md, cited for the social presets (11, including 9:16) with safe-area guides. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/blob/375a9f3/docs/PROJECTS.md> | reframe PROJECTS.md, cited for the project docs and the state management section. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/blob/375a9f3/package.json> | reframe package.json, cited as a FACT source for next 16.4.1. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/blob/375a9f3/src/lib/captions.ts> | reframe captions.ts, cited for converting SRT and WebVTT into TextOverlay items (maximum 500). | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/blob/375a9f3/src/lib/editHistory.ts> | reframe editHistory.ts, cited for undo grouping, where a reset counts as one step. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/blob/375a9f3/src/lib/engineUrls.ts> | reframe engineUrls.ts, cited for the FFmpeg core CDN URLs with SRI verification. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/blob/375a9f3/src/lib/ffmpeg.ts> | reframe ffmpeg.ts lines 353-356, cited as evidence that the architecture doc is outdated. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/blob/375a9f3/src/lib/ffmpegArgs.ts> | reframe ffmpegArgs.ts, cited as the source for the libx264 ultrafast encoder setting. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/blob/375a9f3/src/lib/project.ts> | reframe project.ts, cited for matching a reopened file by name, size and duration. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/blob/375a9f3/src/lib/types.ts> | reframe types.ts, cited for the EditRecipe type. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1074> | reframe issue #1074, cited with #1256 for rendering tracks in a worker. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1167> | reframe issue #1167, cited for AI automatic captions. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1169> | reframe issue #1169, cited for silence and jump-cut evidence. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1203> | reframe issue #1203: request for accelerated WebGL preview. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1256> | reframe issue #1256, cited with #1074 for multi-track rendering in a worker. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1395> | reframe issue #1395, cited for multi-track render in a worker. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1427> | reframe issue #1427: blob URL leak. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1449> | reframe issue #1449: text overlay export fails, and custom fonts need a file in the virtual FS. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1450> | reframe issue #1450: OOM or freeze when generating output with looped music. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1468> | reframe issue #1468, cited with the ?settings= link API point. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1469> | reframe issue #1469, cited with the ?settings= link API point. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1472> | reframe issue #1472: the size estimate ignores the preset. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1474> | reframe issue #1474: breaks with non-ASCII and emoji input. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1490> | reframe issue #1490: blob URLs are never revoked. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1493> | reframe issue #1493: export can hang indefinitely. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1498> | reframe issue #1498, cited for the preview versus export coordination problem. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1500> | reframe issue #1500: leak in the thumbnail generator. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1501> | reframe issue #1501, cited with #1493 for export hanging without audio. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1510> | reframe issue #1510, a request to add real-time preview. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1512> | reframe issue #1512, a timeline request for multi-track editing. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1539> | reframe issue #1539: the test suite breaks (see the note). | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1575> | reframe issue #1575, cited for a claim about SRI verification. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1578> | reframe issue #1578: image overlays dropped in GIF export. | `projects/reframe.md` |
| <https://github.com/reframe-oss/reframe/issues/1579> | reframe issue #1579: mutating the video drops the background music. | `projects/reframe.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/> | filmcraft repo at commit e4ab3c6, cited as the base of the file links. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/Cargo.toml> | filmcraft Cargo.toml, cited for edition 2024, rust-version 1.95 and the MIT OR Apache-2.0 license. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/README.md> | filmcraft README, cited for its admission that color grading, keys and export rendering run on the CPU. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/ROADMAP.md> | filmcraft ROADMAP, cited for the crate, line and PR counts (measured by the authors). | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/crates/automation/Cargo.toml> | filmcraft automation crate manifest, cited for rmcp 3.5 with the server, macros and transport-io features. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/crates/automation/src/server.rs> | filmcraft automation server.rs, cited for a read-only default max_side of 960 that does not move the playhead. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/crates/engine/src/commands.rs> | filmcraft engine commands.rs, cited for the command predicate, run() and the journal flag. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/crates/export/src/pipeline.rs> | filmcraft export pipeline.rs, cited for export calling filmcraft_render::render_sequence. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/crates/format/src/lib.rs> | filmcraft format lib.rs, cited for the project file format (lines 1-70). | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/crates/project/src/keyframe.rs> | filmcraft keyframe.rs, cited for keys and keyframes (lines 12-175). | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/crates/project/src/track.rs> | filmcraft track.rs, cited for the clip model (lines 64-130). | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/docs/agents.md> | filmcraft docs/agents.md, cited for the claim that the project was largely built by agents. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/docs/architecture.md> | filmcraft architecture doc, cited for the crate table (harfrust shaping, Whisper and Kokoro in candle). | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/docs/control-protocol.md> | filmcraft control protocol doc, cited for blocking command smuggling through an HTTP fetch from a web page. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/docs/graphics.md> | filmcraft graphics doc, cited for graphics layers stored as hidden effect instances. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/docs/project-files.md> | filmcraft project-files doc, cited for SCHEMA_VERSION currently at 17. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6/docs/web.md> | filmcraft web doc, cited for the single-threaded web build (no atomics, nightly build-std). | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/blob/e4ab3c6ab99c355290ad536d012421e06ed33e11b/> | filmcraft repo at pinned commit e4ab3c6, cited as the base of the file links. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/1038> | filmcraft issue #1038: the Windows window never opens. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/1390> | filmcraft issue #1390: the app does not start on Windows. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/1616> | filmcraft issue #1616, quoted for GPU export silently rendering (quote cut off in the note). | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/30> | filmcraft issue #30: hardware acceleration and speed on big footage, the biggest gap. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/3719> | filmcraft issue #3719: the app freezes at startup on Windows. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/4635> | filmcraft issue #4635, quoted for the web build and H.264 export errors. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/4730> | filmcraft issue #4730: request to make the render faster. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/4755> | filmcraft issue #4755: unstable timing and performance (with #4768). | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/4758> | filmcraft issue #4758: blank video after export. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/4768> | filmcraft issue #4768, cited with #4755 for unstable timing and performance. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/4771> | filmcraft issue #4771: space or play has a ~0.5 s delay with Bluetooth headphones. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/4772> | filmcraft issue #4772: no audio when moving the playhead. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/4775> | filmcraft issue #4775: the headless CLI does not initialize. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/4781> | filmcraft issue #4781: a GH5S MOV does not play. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/4783> | filmcraft issue #4783: AVCHD or MTS fails to import without an error. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/4791> | filmcraft issue #4791: artifacts at the edges of the frame. | `projects/filmcraft.md` |
| <https://github.com/storytold/filmcraft/issues/940> | filmcraft issue #940: request for project versioning. | `projects/filmcraft.md` |
| <https://github.com/timii/voidcut/blob/815d596/> | voidcut repo at commit 815d596, cited as the base of the file links. | `projects/voidcut.md` |
| <https://github.com/timii/voidcut/blob/815d596/LICENSE> | voidcut LICENSE, cited for the MIT license. | `projects/voidcut.md` |
| <https://github.com/timii/voidcut/blob/815d596/README.md> | voidcut README, cited for its description as a work in progress. | `projects/voidcut.md` |
| <https://github.com/timii/voidcut/blob/815d596/package.json> | voidcut package.json, cited as a FACT source for the SvelteKit dependency version. | `projects/voidcut.md` |
| <https://github.com/timii/voidcut/blob/815d596/src/lib/interfaces/Media.ts> | voidcut Media.ts, cited for media kept in state as data URLs. | `projects/voidcut.md` |
| <https://github.com/timii/voidcut/blob/815d596/src/lib/interfaces/Player.ts> | voidcut Player.ts, cited for the aspect-ratio presets (9:16 mapped to 1080x1920). | `projects/voidcut.md` |
| <https://github.com/timii/voidcut/blob/815d596/src/lib/interfaces/Timeline.ts> | voidcut Timeline.ts lines 5-62, cited for the real timeline types. | `projects/voidcut.md` |
| <https://github.com/timii/voidcut/blob/815d596/src/lib/utils/export.utils.ts> | voidcut export.utils.ts, cited with Player.ts for the export presets. | `projects/voidcut.md` |
| <https://github.com/timii/voidcut/blob/815d596/src/lib/utils/ffmpeg.utils.ts> | voidcut ffmpeg.utils.ts lines 85-90, cited for the single-thread core 0.12.6 (no core-mt). | `projects/voidcut.md` |
| <https://github.com/timii/voidcut/blob/815d596/src/lib/utils/history.utils.ts> | voidcut history.utils.ts, cited for undo, redo and reset with a limit of 100. | `projects/voidcut.md` |
| <https://github.com/timii/voidcut/blob/815d596/src/lib/utils/persistence/persistence.utils.ts> | voidcut persistence.utils.ts, cited for the RestoreStateDialog on reopen; the note says there is no exportable project file. | `projects/voidcut.md` |
| <https://github.com/timii/voidcut/blob/815d596/src/lib/utils/playback.utils.ts> | voidcut playback.utils.ts, cited for the playback timer that increments currentPlaybackTime. | `projects/voidcut.md` |
| <https://github.com/timii/voidcut/blob/815d596/src/lib/utils/timeline-settings.utils.ts> | voidcut timeline-settings.utils.ts, cited for the clamp and normalize helpers (an inference is drawn from them). | `projects/voidcut.md` |
| <https://github.com/timii/voidcut/blob/815d596/src/lib/utils/waveform.utils.ts> | voidcut waveform.utils.ts, cited for waveform generation with an ffmpeg filter_complex. | `projects/voidcut.md` |
| <https://github.com/timii/voidcut/blob/815d596/src/stores/store.ts> | voidcut store.ts, cited for the global Svelte stores in the SvelteKit SPA. | `projects/voidcut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/> | freecut repo at commit 5ceeeb3, cited as the base of the file links. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/CHANGELOG.md> | freecut CHANGELOG, cited for weekly CalVer instead of SemVer. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/CLAUDE.md> | freecut CLAUDE.md, cited for the rule that production dependencies are pinned exactly. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/LICENSE> | freecut LICENSE, cited for the MIT license. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/README.md> | freecut README, cited for its link to the freecut.net site and the user guide. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/docs/render-frame-decomposition-plan.md> | freecut render frame decomposition plan, cited as a plan that needs manual pixel verification. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/headless/Dockerfile> | freecut headless Dockerfile, cited for the software adapter that may produce blank frames. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/headless/frame-oracle.mjs> | freecut frame-oracle.mjs, cited as a check that cannot be pinned in CI because it depends on the GPU. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/headless/lib/contract.mjs> | freecut headless contract.mjs, cited for an integer fps between 1 and 240. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/headless/lib/http-security.mjs> | freecut headless http-security.mjs, cited with the operation queue and idempotency files. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/headless/lib/idempotency.mjs> | freecut headless idempotency.mjs, cited with contract.mjs and the operation queue. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/headless/lib/operation-queue.mjs> | freecut headless operation-queue.mjs, cited with the idempotency and HTTP security files. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/headless/lib/render-core.mjs> | freecut render-core.mjs lines 265-269, cited with the headless Dockerfile for the software adapter. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/headless/serve.mjs> | freecut headless serve.mjs, cited for the GET /health, POST /render and other routes plus the /v1 API. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/package.json> | freecut package.json scripts, cited for the check:layer-boundaries, check:deps-contracts and check:edge-budgets scripts. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/features/docs/docs-content.ts> | freecut docs-content.ts, cited for docs pages loaded with import.meta.glob and sorted by order. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/features/docs/pages/22-troubleshooting.ts> | freecut 22-troubleshooting.ts, cited for the symptom and solution table. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/features/preview/hooks/use-preview-render-pump-controller.ts> | freecut preview render pump controller, cited with the preview renderer controller. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/features/preview/hooks/use-preview-renderer-controller.ts> | freecut preview renderer controller, cited for preseek, proxy fallback on scrub and the scrub overlay. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/features/timeline/stores/commands/types.ts> | freecut commands types, cited for timeline state captured by snapshot for undo. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/features/timeline/stores/timeline-command-store.ts> | freecut timeline-command-store.ts, cited for history kept per context. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/headless/edit-ops.ts> | freecut headless edit-ops.ts, cited for the public list of 19 edit operations. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/i18n/languages.ts> | freecut languages.ts, cited for the 9 UI locales via i18next. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/infrastructure/audio/THIRD_PARTY_LICENSE> | freecut THIRD_PARTY_LICENSE, cited for the LGPL-2.1 code copied into time-stretch.ts. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/infrastructure/storage/workspace-fs/paths.ts> | freecut workspace-fs paths.ts, cited for the workspace folder chosen by the user. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/runtime/composition-runtime/worklets/soundtouch-preview-processor.worklet.ts> | freecut SoundTouch preview worklet, cited for SoundTouch in an AudioWorklet. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/runtime/player/clock/Clock.ts> | freecut Clock.ts, cited as the time source independent of React's render cycle. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/runtime/renderer/client-render-engine.ts> | freecut client-render-engine.ts, cited for the MP4, WebM and MOV containers. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/runtime/renderer/client-renderer.ts> | freecut client-renderer.ts, cited for mediabunny encoding through WebCodecs and muxing. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/runtime/renderer/render-mode-resources.ts> | freecut render-mode-resources.ts, cited for caches per renderMode (export or preview). | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/runtime/renderer/workers/export-render.worker.ts> | freecut renderer workers directory, cited for export-render.worker.ts and the decoder prewarm worker. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/shared/projects/migrations/index.ts> | freecut migrations index.ts, cited for the normalize step applied on every load. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/shared/projects/migrations/migrations.ts> | freecut migrations.ts, cited for the rule never to modify existing migrations. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/shared/projects/migrations/types.ts> | freecut migrations types.ts, cited for CURRENT_SCHEMA_VERSION = 15. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/types/keyframe.ts> | freecut keyframe.ts, cited for keyframe frames relative to the start of the item. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/types/project.ts> | freecut project.ts, cited for the flat items array with trackId. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/types/timeline.ts> | freecut timeline.ts, cited for frame-based fields with sourceFps for conversion. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/blob/5ceeeb388eef5423640b7db157706274bf1b8165/src/types/transition.ts> | freecut transition.ts, cited for the transition type shape. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/issues/108> | freecut issue #108 (closed): keyframes stopped working. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/issues/116> | freecut issue #116 (closed): a "Cant Export" report. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/issues/137> | freecut issue #137 (closed): audio plays in the editor and disappears on export. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/issues/143> | freecut issue #143 (open): request for Docker support. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/issues/203> | freecut issue #203 (closed) on generated captions. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/issues/250> | freecut issue #250 (closed) on some transitions. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/issues/316> | freecut issue #316 (open): export of a long timeline. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/issues/317> | freecut issue #317 (closed) on Chrome Linux. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/issues/349> | freecut issue #349: a silent audio bug found through the headless work (closed). | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/issues/350> | freecut issue #350, from an external contributor building 80-minute assembly agents on the headless tool. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/issues/351> | freecut issue #351, cited with #350 for the 80-minute headless agent work. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/issues/36> | freecut issue #36 (closed): GIF import and export. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/issues/376> | freecut issue #376 (open): removeChild crash. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/issues/92> | freecut issue #92 (closed): hidden adjustment. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/tree/5ceeeb388eef5423640b7db157706274bf1b8165/src> | freecut src directory, cited for the layer layout (features/ holds the UI). | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/tree/5ceeeb388eef5423640b7db157706274bf1b8165/src/features/docs/pages> | freecut docs pages directory, cited for the numbered doc pages (for example Shapes and Masks). | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/tree/5ceeeb388eef5423640b7db157706274bf1b8165/src/runtime/composition-runtime/components> | freecut composition-runtime components, cited for SoundTouch in an AudioWorklet for pitch. | `projects/freecut.md` |
| <https://github.com/walterlow/freecut/tree/5ceeeb388eef5423640b7db157706274bf1b8165/src/runtime/renderer/audio> | freecut renderer audio directory, cited for the planning, decode, DSP and ducking stages. | `projects/freecut.md` |
| <https://hn.svelte.dev/item/45980760> | Hacker News item for a closed agentic video editing product, cited from a search result (not read). | `notes/agents-mcp.md` |
| <https://omniclip.app/> | cited without a stated claim. | `projects/omniclip.md` |
| <https://petapixel.com/2025/10/30/canvas-new-creative-os-promises-to-be-its-biggest-evolution-yet/> | PetaPixel coverage of Canva's creative OS launch (press coverage). | `projects/canva.md` |
| <https://raw.githubusercontent.com/diffusionstudio/core/main/README.md> | diffusionstudio core README, cited for the README recommending Remotion for SSR (avoid as a base). | `notes/stack-browser.md` |
| <https://sparxno.com/blog/canva-video-editor> | Sparxno Canva editor review (secondary), cited for trim, split and transitions on the free plan. | `projects/canva.md` |
| <https://support.microsoft.com> | Microsoft support page, cited via search for Clipchamp's brand kit being premium in the personal version. | `projects/clipchamp.md` |
| <https://support.microsoft.com/en-US/Clipchamp/which-web-browsers-does-clipchamp-support> | Microsoft support page, cited for the recommendation to use a recent Edge or Chrome. | `projects/clipchamp.md` |
| <https://support.microsoft.com/en-us/clipchamp/how-to-choose-the-right-export-resolution> | Microsoft Clipchamp export help, cited for 480p, 720p, 1080p and 4K exports plus GIF and MP3. | `projects/clipchamp.md` |
| <https://techcommunity.microsoft.com/discussions/microsoft-clipchamp-discussions/clipchamp-app-not-functioning-properly-%E2%80%94-transitions-panel-blank--frequent-crash/4488457> | Microsoft Tech Community thread (Jan 2026) on Clipchamp crashes and a blank transitions panel. | `projects/clipchamp.md` |
| <https://timii.github.io/voidcut/> | voidcut GitHub Pages demo, cited as the demo link. | `projects/voidcut.md` |
| <https://tlinky.com/capcut-review/> | Tlinky CapCut review, a secondary source reporting two paid tiers in 2026 (not confirmed). | `projects/capcut.md` |
| <https://tryuncle.com/learn/davinci-resolve/davinci-resolve-vs-canva-video-editor-for-beginners> | Tryuncle comparison, a secondary source for Canva audio limits (no EQ or mixer). | `projects/canva.md` |
| <https://uk.trustpilot.com/review/www.capcut.com?page=9> | Trustpilot CapCut reviews, cited for complaints that Pro features block export. | `projects/capcut.md` |
| <https://unstar.app/blog/capcut-everything-is-pro-now-cant-export-reviews-2026> | Unstar analysis (third party) of CapCut 1 to 3 star reviews, attributing about 26% of negatives to Pro-locked features. | `projects/capcut.md` |
| <https://web.dev/case-studies/capcut> | web.dev CapCut case study, cited for the Emscripten and WebAssembly build that cut the package by 15%. | `projects/capcut.md` |
| <https://web.dev/case-studies/capcut?hl=zh-tw> | web.dev CapCut case study (zh-tw), cited for WebCodecs hardware encode and decode and simultaneous 4K streams. | `projects/capcut.md` |
| <https://web.dev/case-studies/clipchamp> | web.dev Clipchamp case study, cited for local processing and WebAssembly SIMD accelerating 4K. | `projects/clipchamp.md` |
| <https://www.canva.com/en_in/help/magic-video/> | Canva help page on Magic Video, cited for the 60-second auto-generated video feature. | `projects/canva.md` |
| <https://www.canva.com/help/cant-download-design-or-video/> | Canva help page on downloads, cited with the download-as-video page for export limits. | `projects/canva.md` |
| <https://www.canva.com/help/download-as-video/> | Canva help page on downloading as video, cited via search result for 4K download being desktop only. | `projects/canva.md` |
| <https://www.canva.com/help/sharing-social-media-limitations/> | Canva help page on social sharing limits (not read). | `projects/canva.md` |
| <https://www.canva.com/help/upload-formats-requirements/> | Canva help page on upload formats (via search), cited for ProRes 4444 and HDV failures and 4K reduced to 1080p. | `projects/canva.md` |
| <https://www.canva.com/video-editor/> | Canva video editor page, which returned HTTP 403 to WebFetch (not read). | `projects/canva.md` |
| <https://www.canva.dev/blog/developers/canva-and-coding-agents-platforms/> | Canva developer blog on coding agent platforms, cited via search for specific support. | `projects/canva.md` |
| <https://www.capcut.com/help/capcut-pro-features-unavailable> | CapCut help page, cited for watermarked exports and the subscription prompt on export. | `projects/capcut.md` |
| <https://www.capcut.com/help/export-videos-in-capcut> | CapCut export help, cited for browser support and desktop OS requirements (Windows 10 64-bit or later, macOS 12 or later). | `projects/capcut.md` |
| <https://www.capcut.com/newsroom/revealing-the-creative-trends-on-capcut-in-2025> | CapCut newsroom 2025 trends post, cited as vendor data on most-used features and auto captions. | `features-20-80.md`, `notes/features.md`, `projects/capcut.md` |
| <https://www.capcut.com/pt-br/> | CapCut Portuguese homepage, cited for web, desktop and mobile availability and CapCut Pad. | `projects/capcut.md` |
| <https://www.capcut.com/resource/canva-video-template> | CapCut resource page on Canva templates, cited as a biased source on the lack of per-property keyframes. | `projects/canva.md` |
| <https://www.capcut.com/resource/capcut-standard-vs-pro> | CapCut Standard vs Pro resource page, cited with the pt-br homepage as a general source. | `projects/capcut.md` |
| <https://www.capcut.com/resource/how-to-use-capcut-on-pc> | CapCut PC guide, cited as an official guide (not read in full) for the editor layout. | `projects/capcut.md` |
| <https://www.capcut.com/tools/capcut-x-codex> | CapCut tool page on a Codex integration, cited for an MCP connection that edits projects in the CapCut timeline (no date or public API). | `projects/capcut.md` |
| <https://www.capcut.com/tools/edit-pilot> | CapCut EditPilot tool page, cited for text-based editing on the desktop timeline. | `projects/capcut.md` |
| <https://www.flexclip.com/learn/canva-video-2-alternative.html> | FlexClip competitor page, citing a TechRadar note on Canva slowing in long, heavy browser projects. | `projects/canva.md` |
| <https://www.freecut.net/> | freecut.net site, cited as a FACT source for the project's site. | `projects/freecut.md` |
| <https://www.freecut.net/docs> | freecut docs, listed among the sources in the vukem notes. | `notes/vukem.md`, `projects/freecut.md` |
| <https://www.g2.com/products/clipchamp/reviews> | G2 Clipchamp reviews, cited for a review noting slowness with several 1080p clips on under 16 GB RAM and no auto-save. | `projects/clipchamp.md` |
| <https://www.inkl.com/news/clipchamp-2026-review> | inkl Clipchamp 2026 review (secondary), cited for Clipchamp Premium being included in Microsoft 365 (not confirmed by Microsoft). | `projects/clipchamp.md` |
| <https://www.sikayetvar.com/en/capcut-us/capcut-export-issues-and-overuse-of-pro-features-limit-usability> | Sikayetvar CapCut complaints page, cited with Trustpilot for Pro features blocking export. | `projects/capcut.md` |
| <https://www.sikayetvar.com/en/capcut-us/capcuts-high-pricing-leaves-long-time-users-frustrated> | Sikayetvar complaints page on high prices for long-time users. | `projects/capcut.md` |
| <https://www.w3.org/2021/03/media-production-workshop/talks/soeren-balko-clipchamp-webcodecs.html> | W3C 2021 workshop talk by Clipchamp on WebCodecs, cited for the no-upload, low-cost and privacy benefits and their limits. | `projects/clipchamp.md` |

## Engine, formats and post-mortems

| URL | Supports | Cited in |
| --- | --- | --- |
| <https://bugs.webkit.org/show_bug.cgi?id=284448> | WebKit bug report: Chrome limits concurrent audio decode to 1 (secondary and old source). | `notes/stack-browser.md` |
| <https://bugs.webkit.org/show_bug.cgi?id=300538> | WebKit bug open since Oct 2025 said a WASM feature was the only one missing. | `notes/stack-browser.md` |
| <https://bugs.webkit.org/show_bug.cgi?id=327697> | WebKit bug: channel-count support returns false for 3 to 16 channels, inconsistent with MediaCapabilities. | `notes/stack-browser.md` |
| <https://bugs.webkit.org/show_bug.cgi?id=328491> | WebKit bug: the AAC AudioEncoder treats the first 1024 samples as silence. | `notes/engine-postmortems.md` |
| <https://chromium.googlesource.com/chromium/src/+/HEAD/docs/gpu/vaapi.md> | Chromium GPU doc: NVIDIA hardware encoding needs a third-party driver for VA-API. | `notes/stack-browser.md` |
| <https://chromium.googlesource.com/chromium/src/+/c74bb01a2889afdfdd1e06f0144fb7b02e2e00a2> | Chromium commit with the ChromeOS VA-API encoder limit (16, or 10 on Stoney Ridge). | `notes/stack-browser.md` |
| <https://community.adobe.com/bug-reports-728/iphone-rec-2100-hlg-footage-becomes-washed-out-in-premiere-pro-on-windows-1635208> | Adobe community report of iPhone HLG footage looking washed out in Premiere on Windows (search summary only). | `notes/engine-postmortems.md` |
| <https://community.adobe.com/questions-729/getting-duration-out-of-pproticksin-pproticksout-1368450> | Adobe community thread on converting Premiere ticks to durations. | `notes/engine-postmortems.md` |
| <https://community.render.com/t/ffmpeg-libass-subtitles-filter-not-using-fontsdir-parameter-on-render-deployment/39185> | Render community thread: the ffmpeg subtitles filter ignores fontsdir on Render. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://docs.yjs.dev/api/undo-manager> | Yjs UndoManager docs; the note says the page does not describe per-user undo. | `notes/engine-postmortems.md` |
| <https://emby.media/learn/articles/subtitles/rendering/fonts-typesetting/Subtitle-Font-Fallback.html> | Emby doc: a fallback font can change width, baseline and line breaks (via search). | `notes/engine-postmortems.md` |
| <https://emby.media/learn/articles/subtitles/rendering/fonts-typesetting/Subtitle-Karaoke.html> | Emby doc on ASS karaoke tags such as \k, \K and \kf (via search, not opened in full). | `notes/engine-postmortems.md` |
| <https://ffmpegwasm.netlify.app/docs/overview> | ffmpeg.wasm overview, cited for building a custom core without x264. | `notes/licensing.md`, `notes/stack-browser.md` |
| <https://fftrac-bg.ffmpeg.org/ticket/9002> | FFmpeg ticket #9002: the subtitles filter does not display all ASS captions (via search). | `notes/engine-postmortems.md` |
| <https://forum.shotcut.org/t/iphone-hdr-washed-out/47286> | Shotcut forum thread on iPhone HDR washed out, quoted for HDR to SDR being lossy. | `engine.md`, `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/OpenShot/openshot-qt/issues/5747> | OpenShot issue #5747: clip distorted in the preview but correct in the export. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/OpenShot/openshot-qt/issues/5869> | OpenShot issue #5869: opening a new project overwrites existing project edits (titles read via search). | `notes/engine-postmortems.md` |
| <https://github.com/Vanilagy/mediabunny> | Mediabunny repo, cited as a candidate library with one maintainer (evaluate later). | `notes/licensing.md`, `notes/stack-browser.md`, `stack-tradeoffs.md` |
| <https://github.com/Vanilagy/mediabunny/issues/144> | Mediabunny issue #144: a fix in the library and a report to Chromium. | `engine.md`, `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/Vanilagy/mediabunny/issues/320> | Mediabunny issue #320 (open) asks for a variable frame rate field in PacketStats. | `notes/engine-postmortems.md` |
| <https://github.com/Vanilagy/mediabunny/issues/365> | Mediabunny issue #365: Chromium marked the first chunk as delta and closed the report as won't fix. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/Vanilagy/mediabunny/issues/447> | Mediabunny issue #447: in MP4 the fix is the edit list (Chrome on Android, 2048). | `engine.md`, `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/Vanilagy/mediabunny/issues/492> | Mediabunny issue #492: the mvhd and tkhd matrices must be read, not only rotation. | `engine.md`, `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/Vanilagy/mediabunny/issues/592> | Mediabunny issue #592 (open): HEVC asks for a third-party decoder extension doc. | `notes/engine-postmortems.md` |
| <https://github.com/Vanilagy/mediabunny/issues/644> | Mediabunny issue #644: VideoDecoder cannot omit outputs, by spec, per the author. | `notes/engine-postmortems.md` |
| <https://github.com/Vanilagy/mediabunny/issues/69> | Mediabunny issue #69: use an integer coordinate system or fuzzy epsilon compares. | `engine.md`, `notes/engine-postmortems.md` |
| <https://github.com/Vanilagy/mp4-muxer> | mp4-muxer README, cited as unmaintained and superseded by Mediabunny (AVOID). | `notes/stack-browser.md` |
| <https://github.com/diffusionstudio/core/issues/63> | diffusionstudio core issue #63: a timeline with 200 subclips freezes at a point that depends on file size. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/facebookarchive/flicks> | flicks repo, cited as a library that does not divide the NTSC rates (29.97, 23.976, 59.94). | `engine.md`, `notes/engine-postmortems.md` |
| <https://github.com/ffmpegwasm/ffmpeg.wasm/issues/451> | ffmpeg.wasm issue #451: reducing initial memory in 0.11 caused OOM compared with 0.10. | `notes/engine-postmortems.md` |
| <https://github.com/ffmpegwasm/ffmpeg.wasm/issues/876> | ffmpeg.wasm issue #876: a request open since 2025, with maintainers saying it is bandwidth limited. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/gpac/mp4box.js> | mp4box.js repo, an option to evaluate only if Mediabunny fails on real MP4 files. | `notes/stack-browser.md` |
| <https://github.com/mifi/editly> | Editly repo, cited as one of the editing tools in the engine notes. | `notes/engine-postmortems.md` |
| <https://github.com/mifi/editly/issues/117> | Editly issue #117, listed by search and not opened. | `notes/engine-postmortems.md` |
| <https://github.com/mifi/editly/issues/161> | Editly issue #161: Urdu text not centered in a text component (open, title only). | `notes/engine-postmortems.md` |
| <https://github.com/mifi/editly/issues/327> | Editly issue #327: clips rendered without filling silence up to the exact duration. | `engine.md`, `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/mifi/editly/issues/49> | Editly issue #49: the same 90 s JSON renders 88.70 s at 10 fps and 88.92 s at 30 fps. | `engine.md`, `notes/engine-postmortems.md` |
| <https://github.com/mifi/lossless-cut/issues/126> | LosslessCut issue #126: smart cut, open for a long time. | `notes/engine-postmortems.md` |
| <https://github.com/mifi/lossless-cut/issues/2921> | LosslessCut issue #2921: timestamp jumps when joining files. | `engine.md`, `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/mifi/lossless-cut/issues/3210> | LosslessCut issue #3210: a keyframe-mode segment starts at the next keyframe (MPEG-TS). | `notes/engine-postmortems.md` |
| <https://github.com/mifi/lossless-cut/issues/455> | LosslessCut issue #455: different timebases freeze the second clip on merge. | `notes/engine-postmortems.md` |
| <https://github.com/mltframework/mlt/issues/69> | MLT issue #69: audio is one frame out of sync when the clip has an in-point. | `engine.md`, `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/mltframework/shotcut/issues/1517> | Shotcut issue #1517: a text overlay shifts levels slightly. | `notes/engine-postmortems.md` |
| <https://github.com/mltframework/shotcut/issues/1774> | Shotcut issue #1774: mp4 export freezes at 36:25:440 (closed). | `notes/engine-postmortems.md` |
| <https://github.com/mltframework/shotcut/issues/1791> | Shotcut issue #1791, cited for the VFR conversion prompt in the repro steps. | `notes/engine-postmortems.md` |
| <https://github.com/motion-canvas/motion-canvas/issues/957> | Motion Canvas issue #957: MP3 misaligned after seek, while WAV resolves it. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/obsproject/obs-studio/issues/13396> | OBS Studio issue #13396: clips labelled 30 fps have real intervals between 29.4 and 30.3 fps. | `engine.md`, `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/olive-editor/olive/issues/1700> | Olive issue #1700, cited as a case where redoing a project is easier than migrating it. | `engine.md`, `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/olive-editor/olive/issues/1705> | Olive issue #1705: the old sound keeps playing after saving and reopening. | `engine.md`, `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/olive-editor/olive/issues/2132> | Olive issue #2132 (titles read only): undo and transitions are a fragile area. | `notes/engine-postmortems.md` |
| <https://github.com/olive-editor/olive/issues/2229> | Olive issue #2229, cited next to #1700 as a source for the advice to redo rather than migrate. | `engine.md`, `notes/engine-postmortems.md` |
| <https://github.com/remotion-dev/remotion/issues/10274> | Remotion issue #10274: silently incorrect output that disappears at concurrency 1. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/remotion-dev/remotion/issues/11315> | Remotion issue #11315: an SVG loaded as an image is an isolated document without the page's fonts. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/remotion-dev/remotion/issues/11578> | Remotion issue #11578: 75 frames identical and stale, closed as non-bug. | `notes/engine-postmortems.md` |
| <https://github.com/remotion-dev/remotion/issues/11586> | Remotion issue #11586 (title read only): odd-dimension compositions do not export to H.264 or H.265. | `notes/engine-postmortems.md` |
| <https://github.com/remotion-dev/remotion/issues/11732> | Remotion issue #11732, cited for Series with trimBefore on Audio. | `notes/engine-postmortems.md` |
| <https://github.com/remotion-dev/remotion/issues/12603> | Remotion issue #12603: preview and render pick frames with different rounding rules. | `engine.md`, `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/remotion-dev/remotion/issues/12604> | Remotion issue #12604: wrong text shadow under opacity, because the export renderer partly reimplements CSS. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/remotion-dev/remotion/issues/12703> | Remotion issue #12703: inflated duration and 46.4 ms HLS drift from trailing AAC padding. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/remotion-dev/remotion/issues/12927> | Remotion issue #12927 (title read only): caption retiming across cuts and speed changes. | `notes/engine-postmortems.md` |
| <https://github.com/remotion-dev/remotion/issues/13403> | Remotion issue #13403, cited with #9653 for packages with inconsistent license declarations (open as of 2026-11-14). | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/remotion-dev/remotion/issues/5509> | Remotion issue #5509: an iPhone video has the wrong rotation after conversion. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/remotion-dev/remotion/issues/5984> | Remotion issue #5984: system fonts vary between the Docker Linux render and the Mac Studio. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/remotion-dev/remotion/issues/7099> | Remotion issue #7099: AAC padding increases the measured duration. | `notes/engine-postmortems.md` |
| <https://github.com/remotion-dev/remotion/issues/7300> | Remotion issue #7300: seek uses a fixed 0.15 s tolerance and was closed for lack of a repro. | `notes/engine-postmortems.md` |
| <https://github.com/remotion-dev/remotion/issues/9653> | Remotion issue #9653, cited with #13403 for inconsistent license declarations. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://github.com/w3c/webcodecs/issues/519> | w3c/webcodecs issue #519: missing three frames when decoding H.264 video. | `notes/engine-postmortems.md` |
| <https://github.com/w3c/webcodecs/issues/626> | w3c/webcodecs issue #626 (open since 2023): the spec does not expose priming and padding. | `engine.md`, `notes/engine-postmortems.md` |
| <https://github.com/w3c/webcodecs/issues/940> | w3c/webcodecs issue #940: VideoToolbox's fully specified colorSpace is assumed to take precedence. | `notes/engine-postmortems.md` |
| <https://github.com/wzhudev/reverse-linear-sync-engine> | cited as a search result (not read in full) on sync and server-side effects. | `notes/engine-postmortems.md` |
| <https://img.ly/docs/cesdk/js/create-video/limitations-6a740d/> | IMG.LY CE.SDK limitations page, cited for open-source Chromium on Linux possibly lacking an H.264 encoder. | `notes/stack-browser.md` |
| <https://img.ly/products/video-for-web/> | IMG.LY product page, cited as a commercial, not open-source SDK (search result) to AVOID. | `notes/stack-browser.md` |
| <https://issues.chromium.org/issues/414491381> | Chromium issue 414491381 (open): a 144 Hz monitor slows VideoDecoder 5 to 10 times. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://issues.chromium.org/issues/504680933> | Chromium issue 504680933: HDR HEVC VideoFrame draws fully transparent (regression, fixed in 148). | `notes/engine-postmortems.md` |
| <https://issues.chromium.org/issues/514397892> | Chromium issue 514397892: a "first packet must be a key packet" failure, marked won't fix. | `notes/engine-postmortems.md` |
| <https://issues.chromium.org/issues/532860177> | Chromium issue 532860177: Chrome ignores the H.264 colorSpace in macOS VideoToolbox and returns BT.709. | `engine.md`, `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://issues.chromium.org/issues/552063578> | Chromium issue 552063578 (open): a 144 Hz monitor throttles VideoDecoder 5 to 10 times. | `notes/engine-postmortems.md` |
| <https://issues.chromium.org/issues/554940580> | Chromium issue 554940580: a reader the browser never collected, worked around by cancelling it. | `notes/engine-postmortems.md` |
| <https://linear.app/now/scaling-the-linear-sync-engine> | Linear post on scaling its sync engine, cited from a search result (not read). | `notes/engine-postmortems.md` |
| <https://mediabunny.dev> | Mediabunny site, cited with its GitHub repo and README for the ADOPT rating. | `notes/stack-browser.md` |
| <https://mediabunny.dev/guide/media-sinks> | Mediabunny media sinks guide, cited for getSample returning the last frame with timestamp at or before t (VFR). | `notes/stack-browser.md` |
| <https://mediabunny.dev/guide/supported-formats-and-codecs> | Mediabunny supported formats guide, cited for WebCodecs gaps (AC-3, DTS, TrueHD, ProRes) and polyfills for MP3 and FLAC. | `notes/stack-browser.md` |
| <https://opentimelineio.readthedocs.io/en/latest/tutorials/otio-timeline-structure.html> | OpenTimelineIO timeline structure tutorial, cited for the sequence and Gap model. | `engine.md`, `notes/engine-postmortems.md` |
| <https://raw.githubusercontent.com/AcademySoftwareFoundation/OpenTimelineIO/main/src/opentime/rationalTime.h> | OpenTimelineIO RationalTime header, cited for value plus rate, rescaled_to and rescaling comparisons. | `engine.md`, `notes/engine-postmortems.md` |
| <https://raw.githubusercontent.com/redotvideo/revideo/main/README.md> | Revideo README, cited for its statement that it renders anywhere Node and a headless browser run. | `notes/stack-browser.md` |
| <https://raw.githubusercontent.com/remotion-dev/remotion/main/LICENSE.md> | Remotion LICENSE.md, cited for a license judged incompatible with the project's MIT (reference use only). | `notes/stack-browser.md` |
| <https://tldraw.dev/docs/sync> | tldraw sync docs, cited for migrations and schema validation across client versions. | `engine.md`, `notes/engine-postmortems.md` |
| <https://w3c.github.io/webcodecs/> | W3C WebCodecs spec, cited for the integer microsecond timestamp limit (1/30000 s and 1/48000 s need rounding). | `engine.md`, `notes/engine-postmortems.md` |
| <https://webkit.org/blog/> | WebKit blog index, named as the primary source for Safari 26 WebGPU that was not opened. | `notes/stack-backend-desktop.md` |
| <https://webkit.org/blog/17333/webkit-features-in-safari-26-0/> | WebKit post on Safari 26.0 features, confirming AudioEncoder and AudioDecoder in Safari 26.0. | `notes/stack-browser.md` |
| <https://www.chromium.org/audio-video/> | Chromium audio and video page, cited for AAC and H.264 or HEVC being limited to Google Chrome. | `notes/licensing.md` |
| <https://www.figma.com/blog/how-figmas-multiplayer-technology-works/> | Figma blog on multiplayer, cited for inserting items between two neighbours without renumbering the others. | `engine.md`, `notes/engine-postmortems.md` |
| <https://www.mltframework.org/docs/mltxml/> | MLT XML docs, cited for timing in whole frames and the frame-to-second mapping from the project profile. | `engine.md`, `notes/engine-postmortems.md` |
| <https://www.remotion.dev/docs/ai/> | Remotion AI docs, cited for docs served as markdown via the .md URL or the Accept header. | `notes/agents-mcp.md` |
| <https://www.remotion.dev/docs/client-side-rendering/how-it-works> | Remotion client-side rendering docs, cited for mixing captured audio and encoding via Mediabunny. | `notes/stack-browser.md` |
| <https://www.remotion.dev/docs/client-side-rendering/index.md> | cited without a stated claim. | `notes/stack-browser.md` |
| <https://www.remotion.dev/docs/client-side-rendering/limitations> | Remotion client-side limitations, cited for rAF throttling and slow aborts in a background tab. | `notes/stack-browser.md` |
| <https://www.remotion.dev/docs/hdr> | Remotion HDR docs, cited for Chrome not supporting HDR and converting HDR to SDR through FFmpeg and zscale. | `notes/stack-browser.md` |
| <https://www.remotion.dev/docs/license/faq> | cited without a stated claim. | `notes/licensing.md` |
| <https://www.remotion.dev/docs/license/pricing> | cited without a stated claim. | `notes/licensing.md` |
| <https://www.remotion.dev/docs/mediabunny/webcodecs-bugs> | Remotion and Mediabunny WebCodecs bugs page, cited for negative-timestamp audio being ignored and failures with interlaced video. | `notes/engine-postmortems.md`, `post-mortems.md` |
| <https://www.w3.org/TR/webcodecs/> | W3C WebCodecs spec, cited for codec saturation defined through queue-based backpressure (original text not opened). | `notes/stack-browser.md` |

## Browser platform and libraries

| URL | Supports | Cited in |
| --- | --- | --- |
| <https://bugzilla.mozilla.org/show_bug.cgi?id=1887554> | Firefox bug: a warning is logged for a VideoFrame collected without close(). | `notes/engine-postmortems.md` |
| <https://bugzilla.mozilla.org/show_bug.cgi?id=2082867> | Firefox bug: the AVC VideoEncoder produces invalid AVCC and shifted timestamps. | `notes/engine-postmortems.md` |
| <https://caniuse.com/mdn-api_filesystemsyncaccesshandle> | Browser support table for FileSystemSyncAccessHandle. | `notes/stack-browser.md` |
| <https://caniuse.com/mdn-api_htmlvideoelement_requestvideoframecallback> | Browser support table for requestVideoFrameCallback. | `notes/stack-browser.md` |
| <https://caniuse.com/mdn-api_videoencoder> | Browser support table for the VideoEncoder API, including Chrome and Firefox Android status. | `notes/stack-browser.md` |
| <https://caniuse.com/native-filesystem-api> | Browser support table for the native File System API; Chrome Android is listed as unsupported. | `notes/stack-browser.md` |
| <https://caniuse.com/webcodecs> | Browser support table for WebCodecs, including Safari and Chrome Android status. | `notes/stack-browser.md` |
| <https://caniuse.com/webgpu> | Browser support table for WebGPU, including the Safari version note. | `notes/stack-browser.md` |
| <https://cdn.jsdelivr.net/npm/@ffmpeg/core@0.12.10/dist/umd> | CDN path from which the reframe project loads FFmpeg core at runtime. | `projects/reframe.md` |
| <https://developer.mozilla.org/en-US/docs/Web/API/GPUDevice/importExternalTexture> | MDN on GPUDevice.importExternalTexture: expires on first use and has limited availability. | `notes/stack-browser.md` |
| <https://developer.mozilla.org/en-US/docs/Web/API/Storage_API/Storage_quotas_and_eviction_criteria> | MDN storage quotas: QuotaExceededError and per-origin LRU eviction. | `notes/stack-browser.md` |
| <https://developer.mozilla.org/en-US/docs/Web/API/WebCodecs_API> | MDN WebCodecs API overview, including availability in dedicated workers. | `notes/stack-browser.md` |
| <https://developer.mozilla.org/en-US/docs/Web/API/WebCodecs_API/Using_the_WebCodecs_API> | MDN WebCodecs usage guide, cited via a search summary on frame-memory limits (not opened in full). | `notes/engine-postmortems.md` |
| <https://developer.mozilla.org/en-US/docs/Web/API/Window/crossOriginIsolated> | MDN page on Window.crossOriginIsolated, cited for cross-origin isolation (CORS and CORP needs). | `notes/stack-browser.md` |
| <https://github.com/gpuweb/gpuweb/wiki/Implementation-Status> | gpuweb implementation status wiki, cited for WebGPU shipping since 147 and flag-gated platforms. | `notes/stack-browser.md` |
| <https://spidermonkey.dev/blog/2025/01/15/is-memory64-actually-worth-using.html> | SpiderMonkey blog on Memory64, cited for Chrome 133 and Firefox 134 shipping it. | `notes/stack-browser.md` |
| <https://unpkg.com/@ffmpeg/core@0.12.5> | unpkg URL from which omniclip loads the FFmpeg core at runtime. | `projects/omniclip.md` |
| <https://unpkg.com/@ffmpeg/core@0.12.6/dist/esm> | unpkg CDN path from which voidcut loads the single-thread FFmpeg core at runtime. | `projects/voidcut.md` |
| <https://web.dev/articles/requestvideoframecallback-rvfc> | web.dev requestVideoFrameCallback article, cited (via summary) for presented-frame timing and the Chromium audio clock. | `notes/stack-browser.md` |

## Backend, desktop and tooling

| URL | Supports | Cited in |
| --- | --- | --- |
| <https://access.redhat.com/security/cve/CVE-2026-107697> | Cited as a source for the CVE-2026-107697 entry in the backend and desktop notes. | `notes/stack-backend-desktop.md` |
| <https://blogs.igalia.com/webkit/blog/2026/wip-58/> | Igalia WebKit blog post, cited as a limited-fact source for WebKit feature status. | `notes/stack-browser.md` |
| <https://console.groq.com/docs/speech-to-text> | Groq speech-to-text docs: file size limits and timestamp granularities. | `notes/stack-backend-desktop.md` |
| <https://deepgram.com/pricing> | Deepgram pricing page with per-minute rates for Nova-3 and Whisper Large. | `notes/stack-backend-desktop.md` |
| <https://developer.microsoft.com/en-us/microsoft-edge/webview2/> | WebView2 download page, cited for Fixed Runtime version 157.0.4341.60. | `notes/stack-browser.md` |
| <https://developers.openai.com/api/docs/pricing> | OpenAI API pricing page; per-model transcription details could not be extracted from it. | `notes/stack-backend-desktop.md` |
| <https://github.com/blackboardsh/electrobun> | Electrobun repo, a desktop option to evaluate; its README cites Go, Rust and Odin as native-side languages. | `notes/stack-backend-desktop.md` |
| <https://github.com/mark3labs/mcp-go> | mcp-go repo, an option to evaluate only if the official Go SDK lacks something (not verified). | `notes/stack-backend-desktop.md` |
| <https://github.com/modelcontextprotocol/go-sdk> | MCP Go SDK repo, rated ADOPT, with MCPGODEBUG flags removed in 1.9.0 (license to confirm). | `notes/stack-backend-desktop.md` |
| <https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0> | MCP Go SDK v1.8.0 release, cited for OAuth discovery and NotifyElicitationComplete. | `notes/stack-backend-desktop.md` |
| <https://github.com/romsar/gonertia> | gonertia repo, an option to evaluate (writing a small adapter is the alternative). | `notes/stack-backend-desktop.md` |
| <https://github.com/spf13/cobra> | cobra repo, rated ADOPT (Apache-2.0, 44.8k stars, stable but with a slow release cadence). | `notes/stack-backend-desktop.md` |
| <https://github.com/tauri-apps/cef-rs> | cef-rs repo, cited for embedded Chromium in Tauri, which is still alpha. | `notes/stack-backend-desktop.md` |
| <https://github.com/urfave/cli> | urfave/cli repo, rated EVALUATE (v3 versus v2 not verified). | `notes/stack-backend-desktop.md` |
| <https://github.com/wailsapp/wails/releases/tag/v3.0.0> | Wails v3.0.0 release, cited for the window support, built-in updater and bindings (mobile experimental). | `notes/stack-backend-desktop.md` |
| <https://token.actions.githubusercontent.com> | GitHub Actions OIDC issuer, used as the certificate OIDC issuer in the vukem release verification command. | `notes/vukem.md` |
| <https://v2.tauri.app/develop/debug/linux-graphics/> | Tauri debug docs on Linux graphics: the DMABUF renderer conflicts with NVIDIA, with a fallback without WebGL. | `notes/stack-browser.md` |
| <https://versionhistory.googleapis.com/v1/chrome/platforms/win/channels/stable/versions?pageSize=1> | Chrome version history API, cited for Chrome stable on Windows (158.0.8127.37). | `notes/stack-browser.md` |
| <https://wails.io/docs/guides/linux> | Wails Linux guide, cited with the Tauri page for DMABUF and NVIDIA issues. | `notes/stack-browser.md` |
| <https://webkitgtk.org/2026/03/18/webkitgtk-2.52-highlights.html> | WebKitGTK 2.52 release notes, cited as a negative fact: no mention of WebCodecs, WebGPU or OPFS. | `notes/stack-backend-desktop.md`, `notes/stack-browser.md` |
| <https://www.assemblyai.com/pricing> | AssemblyAI pricing page, cited for Universal-3.5 Pro, Universal-2 and diarization rates. | `notes/stack-backend-desktop.md` |
| <https://www.devclass.com/security/2026/01/14/code-signing-windows-apps-may-be-easier-and-more-secure-with-new-azure-artifact-service/4079554> | DevClass report (Jan 2026) on Azure artifact signing, cited as a secondary source for the Azure signing values. | `notes/stack-backend-desktop.md` |
| <https://www.wiz.io/vulnerability-database/cve/cve-2026-107697> | Wiz vulnerability entry for CVE-2026-107697, cited with the Red Hat page for the suggested FFmpeg protocol whitelist mitigation. | `notes/stack-backend-desktop.md` |

## MCP and agent tool design

| URL | Supports | Cited in |
| --- | --- | --- |
| <https://aistudio.google.com/docs/video-understanding> | Google AI Studio video understanding docs, cited for the timestamp format and context-length figures. | `notes/agents-mcp.md` |
| <https://code.claude.com/docs/en/mcp> | Claude Code MCP docs, cited for the result-size and background-call behaviour. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://cursor.com/docs/context/mcp> | Cursor MCP docs, cited for how MCP image results are attached to chat. | `notes/agents-mcp.md` |
| <https://developers.figma.com/docs/figma-mcp-server/tools-and-prompts/> | Figma MCP server tools docs, cited for large responses and screenshot token cost. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://developers.openai.com/api/docs/guides/function-calling> | OpenAI function-calling guide, cited for the advice on fewer than 20 functions per turn. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://developers.openai.com/api/docs/guides/images-vision> | OpenAI vision guide, cited for the 32x32 patch token calculation. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://developers.openai.com/apps-sdk/guides/mcp-server> | Apps SDK MCP server guide; the note says this URL returned HTTP 404. | `notes/agents-mcp.md` |
| <https://github.com/ChromeDevTools/chrome-devtools-mcp> | Chrome DevTools MCP repo, cited for its CLI and --slim option. | `notes/agents-mcp.md` |
| <https://github.com/ZiAn-Su/OCC-ForAgent> | OCC-ForAgent repo, cited for one undo step and a draft session with human review. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://github.com/diffusionstudio/editor> | diffusionstudio editor repo, cited for its tool list (waveform, transcribe, listen, capture, check). | `agents-mcp.md` |
| <https://github.com/hetpatel-11/Adobe_Premiere_Pro_MCP> | Adobe Premiere Pro MCP repo, cited with davinci-resolve-mcp as an NLE MCP example. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://github.com/microsoft/playwright-mcp> | Playwright MCP repo, cited for the CLI being more token efficient than MCP. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://github.com/misbahsy/video-audio-mcp> | video-audio-mcp repo, cited for missing timeline and undo and raw FFmpeg stderr errors. | `agents-mcp.md` |
| <https://github.com/samuelgursky/davinci-resolve-mcp> | davinci-resolve-mcp repo, cited for its default tool exposure (search_tools, get_tool_schema, invoke_tool). | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://learn.chatgpt.com/docs/extend/mcp?surface=cli> | ChatGPT MCP docs, cited for the page not listing resources, prompts, elicitation or images as supported. | `notes/agents-mcp.md` |
| <https://modelcontextprotocol.io/docs/2026-07-28/tutorials/security/security_best_practices.md> | MCP security best practices (2026-07-28), cited for minimal scopes and tokens for local servers. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://modelcontextprotocol.io/extensions/apps/overview.md> | MCP apps overview, cited for the list of clients that support MCP apps. | `notes/agents-mcp.md` |
| <https://modelcontextprotocol.io/extensions/client-matrix.md> | MCP official extensions matrix, cited for client support. | `notes/agents-mcp.md` |
| <https://modelcontextprotocol.io/extensions/tasks/overview.md> | MCP tasks overview, cited for durable task IDs and cooperative cancellation. | `notes/agents-mcp.md` |
| <https://modelcontextprotocol.io/specification/2026-07-28/changelog> | MCP specification changelog for 2026-07-28, cited for multi-round-trip requests and the deprecated Roots, Sampling and Logging. | `notes/stack-backend-desktop.md` |
| <https://modelcontextprotocol.io/specification/2026-07-28/changelog.md> | MCP changelog (md), cited for the major changes in 2026-07-28 relative to 2025-11-25. | `notes/agents-mcp.md` |
| <https://modelcontextprotocol.io/specification/2026-07-28/server/tools.md> | MCP 2026-07-28 tools spec, cited for tool annotations being hints that clients must treat as untrusted. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://modelcontextprotocol.io/specification/draft/changelog> | MCP draft changelog, cited for later minor changes (CIMD draft-03, _meta restored). | `notes/agents-mcp.md`, `notes/stack-backend-desktop.md` |
| <https://modelcontextprotocol.io/specification/latest> | MCP specification 'latest' page, cited as referencing the 2026-07-28 schema. | `notes/agents-mcp.md` |
| <https://modelcontextprotocol.io/specification/versioning> | MCP versioning page, cited with the draft changes (OAuth Client ID Metadata Document -03, _meta restored). | `notes/stack-backend-desktop.md` |
| <https://opencode.ai/docs/mcp-servers/> | OpenCode MCP docs, cited for server instructions and elicitation support (images not mentioned). | `notes/agents-mcp.md` |
| <https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-search-tool> | Claude platform docs on the tool search tool, cited for the token cost of multi-server tool definitions. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://platform.claude.com/docs/en/build-with-claude/vision> | Claude platform vision docs, cited for image token costs (a 1000x1000 image costs 1,296 tokens). | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://scalekit.com/blog/mcp-vs-cli-use> | Scalekit blog on MCP versus CLI, cited with its caveat that the sample is one service, one model and n=5. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://shotstack.io/docs/guide/agents/cli.md> | Shotstack agents CLI guide, cited for the convention guide served by an MCP tool and by SKILL.md. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://shotstack.io/docs/guide/agents/mcp-server.md> | Shotstack MCP server guide, cited for OAuth in interactive clients and x-api-key for headless or CI. | `notes/agents-mcp.md` |
| <https://shotstack.io/docs/guide/llms-full.txt> | Shotstack llms-full.txt, cited for the clip start and length model (partially read). | `engine.md`, `notes/engine-postmortems.md` |
| <https://simonwillison.net/2025/Jun/16/the-lethal-trifecta/> | Simon Willison's post on the lethal trifecta, cited for mitigating prompt injection by avoiding the combination. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://www.anthropic.com/engineering/advanced-tool-use> | Anthropic engineering post "Advanced tool use", cited as a FACT source. | `notes/agents-mcp.md` |
| <https://www.anthropic.com/engineering/building-effective-agents> | Anthropic post on building effective agents, cited alongside the writing-tools post for tool design. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://www.anthropic.com/engineering/code-execution-with-mcp> | Anthropic engineering post "Code execution with MCP", cited as a FACT source. | `notes/agents-mcp.md` |
| <https://www.anthropic.com/engineering/writing-tools-for-agents> | Anthropic post on writing tools for agents, cited for the poka-yoke advice (change arguments to prevent mistakes). | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://www.descript.com/blog/article/dont-ship-your-api-as-an-mcp> | Descript blog post on MCP, cited for MCP traffic after listing in the Claude and ChatGPT marketplaces. | `agents-mcp.md`, `notes/agents-mcp.md` |
| <https://www.descript.com/blog/article/the-new-descript-how-we-multiplied-the-apps-speed-and-performance> | Descript blog post on app performance, cited for a decoded 4K frame being 33 MB (about 1 GB per second at 30 fps). | `notes/stack-browser.md`, `performance.md` |

## Licensing and patents

| URL | Supports | Cited in |
| --- | --- | --- |
| <https://accessadvance.com/licensing-programs/hevc-advance/> | HEVC Advance licensing page, cited for FRAND terms and no per-unit fees. | `notes/licensing.md` |
| <https://aomedia.org/license/patent-license/> | AOM patent license page; the note says AV1 fee claims were read only in secondary sources. | `licensing.md`, `notes/licensing.md` |
| <https://evermeet.cx/ffmpeg/> | Third-party FFmpeg build site listed in the licensing notes. | `notes/licensing.md` |
| <https://ffmpeg.org/ffmpeg-codecs.html> | FFmpeg codecs doc: fdk-aac is incompatible with GPL, so the resulting binary is not redistributable. | `notes/licensing.md` |
| <https://ffmpeg.org/legal.html> | FFmpeg legal page: nonfree options (fdk-aac, OpenSSL) make the binary non-redistributable. | `licensing.md`, `notes/engine-postmortems.md`, `notes/licensing.md` |
| <https://fonts.google.com/faq> | Google Fonts FAQ for font licensing; the note says the page returned no text (not verified). | `notes/licensing.md` |
| <https://freetype.org/license.html> | Cited without a stated claim. | `notes/licensing.md` |
| <https://git.alpinelinux.org/aports/plain/community/ffmpeg/APKBUILD> | Cited without a stated claim. | `notes/licensing.md` |
| <https://github.com/BtbN/FFmpeg-Builds> | BtbN FFmpeg-Builds repo, cited for the static build sizes. | `licensing.md`, `notes/licensing.md`, `stack-tradeoffs.md` |
| <https://github.com/BtbN/FFmpeg-Builds/tree/master/scripts.d> | BtbN build scripts that list the codecs compiled in (openh264, libvpx, svtav1, aom and others). | `notes/licensing.md` |
| <https://gsap.com/standard-license/> | GSAP standard license page, cited for the restriction on competing with Webflow-type visual builders. | `notes/licensing.md`, `notes/stack-browser.md` |
| <https://ipfray.com/breaking-access-advance-acquires-via-licensing-alliances-hevc-vvc-patent-pools/> | IPFray article on Access Advance and Via LA consolidation into VCL Advance (via search). | `notes/licensing.md` |
| <https://johnvansickle.com/ffmpeg/> | John Van Sickle static FFmpeg builds page, cited as GPL v3 licensed (GPL only, static Linux). | `notes/licensing.md` |
| <https://meta.wikimedia.org/wiki/Have_the_patents_for_H.264_MPEG-4_AVC_expired_yet%3F> | Wikimedia page on H.264 patent expiry, cited for the patent terms in Brazil (2030-11-10) and the US (2030-11-26). | `licensing.md`, `notes/licensing.md` |
| <https://pixabay.com/service/license-summary/> | Pixabay license summary, cited for the note that it says nothing about repositories. | `notes/licensing.md` |
| <https://raw.githubusercontent.com/FFmpeg/FFmpeg/master/LICENSE.md> | FFmpeg LICENSE.md, cited as a second source for non-redistributable builds with nonfree options. | `licensing.md`, `notes/licensing.md` |
| <https://raw.githubusercontent.com/eugeneware/ffmpeg-static/master/README.md> | cited without a stated claim. | `notes/licensing.md` |
| <https://raw.githubusercontent.com/ffmpegwasm/ffmpeg.wasm/main/Dockerfile> | ffmpeg.wasm build Dockerfile, cited for the --enable-gpl, libx264 and libx265 flags. | `notes/licensing.md` |
| <https://raw.githubusercontent.com/ffmpegwasm/ffmpeg.wasm/main/packages/core/package.json> | ffmpeg.wasm core package.json, cited for the @ffmpeg/core GPL-2.0-or-later license declaration. | `notes/licensing.md` |
| <https://raw.githubusercontent.com/harfbuzz/harfbuzz/main/COPYING> | cited without a stated claim. | `notes/licensing.md` |
| <https://raw.githubusercontent.com/jellyfin/jellyfin-ffmpeg/jellyfin/README.md> | cited without a stated claim. | `notes/licensing.md` |
| <https://raw.githubusercontent.com/jrottenberg/ffmpeg/main/README.md> | cited without a stated claim. | `notes/licensing.md` |
| <https://raw.githubusercontent.com/linuxserver/docker-ffmpeg/master/README.md> | cited without a stated claim. | `notes/licensing.md` |
| <https://raw.githubusercontent.com/openai/whisper/main/README.md> | cited without a stated claim. | `notes/licensing.md` |
| <https://raw.githubusercontent.com/wader/static-ffmpeg/master/README.md> | cited without a stated claim. | `notes/licensing.md` |
| <https://www.gnu.org/licenses/gpl-faq.html> | GNU GPL FAQ, cited for running a modified GPL program on a server without releasing it being private use. | `licensing.md`, `notes/licensing.md` |
| <https://www.gyan.dev/ffmpeg/builds/> | gyan.dev FFmpeg builds page, cited for its 64-bit static GPLv3 builds. | `notes/licensing.md` |
| <https://www.openh264.org/BINARY_LICENSE.txt> | OpenH264 binary license, cited for the Cisco-provided binary and the note that source compilers are not covered. | `licensing.md`, `notes/licensing.md` |
| <https://www.via-la.com/licensing-programs/aac/> | Via LA AAC program page, cited for the US$ 5,000 initial fee (US$ 1,000 for small entities) and free AAC bitstream distribution. | `licensing.md`, `notes/licensing.md` |
| <https://www.via-la.com/licensing-programs/avc-h-264/> | Via LA AVC/H.264 program page, cited for the US$ 0.20 per unit rate and no stated expiry date. | `licensing.md`, `notes/licensing.md` |

## Platform specifications

| URL | Supports | Cited in |
| --- | --- | --- |
| <https://ads.tiktok.com/help/article/video-ads-specifications> | TikTok video ad specifications; the note says the page gives no public caption percentages. | `notes/features.md`, `notes/licensing.md` |
| <https://argil.ai/blog/instagram-reel-size-e350f> | Blog post cited for Instagram Reel size and duration limits, where sources diverge. | `notes/features.md` |
| <https://developers.tiktok.com/docs/en/content-posting-api-media-transfer-guide> | TikTok Content Posting API media guide: 4 GB file limit and 3, 5 or 10 minute durations (via search). | `notes/features.md` |
| <https://docs.x.com/x-api/media/quickstart/best-practices> | X API media best practices; the note says the page does not specify the container. | `notes/features.md` |
| <https://support.google.com/youtube/answer/10059070> | YouTube Help (official): up to 3 minutes, maximum 1080p, vertical. | `notes/features.md` |
| <https://support.google.com/youtube/answer/1722171> | YouTube Help, cited for the note that official acceptance of AV1 or VP9 uploads was not confirmed. | `licensing.md`, `notes/licensing.md` |
| <https://vidquickly.blogspot.com/2024/10/kwai-video-specifications-for-ads.html> | Third-party 2024 blog saying 9:16 for Kwai ads; no official source was found. | `notes/features.md` |
| <https://www.facebook.com/business/ads-guide/update/image/instagram-reels> | Meta Ads guide for Instagram Reels image specs, cited for the ~35% bottom and ~6% side safe areas for ads. | `notes/features.md` |
| <https://www.facebook.com/business/help/980593475366490/> | Meta Help page on Stories and Reels safe zones (content not retrieved). | `notes/features.md` |

## Other

| URL | Supports | Cited in |
| --- | --- | --- |
| <https://dev.to/nareshipme/why-we-switched-to-streaming-frame-extraction-for-mobile-video-editing-3cea> | Dev.to article on streaming frame extraction, cited next to a HackerNoon article on browser export pipelines. | `notes/stack-browser.md` |
| <https://hackernoon.com/how-to-optimize-video-export-pipelines-in-browser-based-editors> | HackerNoon article on video export pipelines in browser editors, cited as a secondary source. | `notes/stack-browser.md` |
| <https://keepachangelog.com/en/1.1.0/> | Keep a Changelog 1.1.0, the changelog format the vukem notes follow. | `notes/vukem.md` |
| <https://semver.org/spec/v2.0.0.html> | SemVer 2.0.0 spec, cited for the versioning standard the vukem project adheres to. | `notes/vukem.md` |
| <https://simplearyan.github.io/kinetix/blog/browser-export-performance> | Kinetix blog on browser export performance (secondary source), cited for Firefox exports being slower than Chrome. | `notes/stack-browser.md` |

## Sources that did not open

URLs that the citing file itself says returned 404, 403, no text, or could not be reached or retrieved.

- <https://developers.openai.com/apps-sdk/guides/mcp-server>: HTTP 404. Cited in `notes/agents-mcp.md`.
- <https://discord.gg/Nr8t9s5wSM>: not accessible. Cited in `projects/omniclip.md`.
- <https://fonts.google.com/faq>: returned no text. Cited in `notes/licensing.md`.
- <https://www.canva.com/video-editor/>: HTTP 403 to WebFetch. Cited in `projects/canva.md`.
- <https://www.facebook.com/business/help/980593475366490/>: content not retrieved. Cited in `notes/features.md`.
