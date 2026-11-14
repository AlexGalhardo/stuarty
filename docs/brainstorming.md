# Brainstorming and decisions

> Stage 2: interview with the author. Each decision is recorded here right after the author answers.
> After any context compaction, reread this file and `STATUS.md` (workspace root, D-025) before continuing.
> The interview started on 2026-11-14 at the author's request, before all the thematic files of stage 1 were written. The raw research behind the recommendations is in `docs/search/notes/` and `docs/search/projects/`.

## Open decisions

Order: from the most structural to the most peripheral. Struck-through items have already become decisions (see below).

1. Scope and repository
   - ~~Destination of the existing Studio code in `web/`.~~ D-002
   - ~~Audience and operating mode that comes first.~~ D-003
   - ~~First push to `vukem-video`: history and tags of the Studio.~~ D-005
2. Web and desktop
   - ~~What comes first.~~ D-004. Open: how much code to share between the Go core and the UI.
   - Desktop shell (Wails v3, Electron, Tauri) and the problem of system webviews.
3. Core and render
   - ~~Language of the command core and source of truth for the render.~~ D-001
   - Accepted tolerance between preview and export.
   - Export from the normalized media or from the original.
4. Tools for agents
   - MCP catalogue shape (few meta-tools or one tool per command).
   - Priority between CLI and MCP.
   - ~~Human confirmation for destructive or expensive actions.~~ D-011
5. Stack where there is still doubt (database, queue, real time, upload).
6. Formats, codecs and licences: ~~H.264 and FFmpeg policy~~ D-006. Open: export preset, accepted input formats.
7. ~~Transcription (local or API)~~ D-010, ~~MVP scope, keyframes and templates~~ D-009.
8. ~~Authentication~~ D-012. Open: scope details (technical decision for the architect).
9. ~~Storage, costs, hosting~~ D-003, D-015. ~~Embedded media~~ D-014.
10. ~~Observability and telemetry~~ D-016. ~~UI ambition~~ D-013. CI/CD and releases follow the vukem standard (technical decision of the architect, already fixed by the brief).
11. ~~Document language~~ D-008, ~~product name~~ D-007. Open: open source strategy and community.
12. Risks and delivery order.

All twelve themes were covered by decisions D-001 to D-025. The working state is in `STATUS.md` at the workspace root.

## Decisions taken

### D-001 — Command core in Go; export render by FFmpeg

- Date: 2026-11-14
- Decision: the command core is written in Go and runs without a browser (CLI, MCP, server, desktop). Export is done by FFmpeg, driven by Go, and is the source of truth for the render. The browser preview runs the same frame plan computed by the core.
- Alternatives considered: (a) core and renderer in TypeScript with headless Chrome for export, as Remotion and Revideo do: richer for motion design and no pixel divergence, but CLI and server would then require Chromium, and Go/vukem would become just a shell; (b) native Rust + WASM, as FilmCraft and the new OpenCut do: one codebase for all targets, but a third language and a much higher cost.
- Reason: the differentiator is agents, CLI and API as first-class users. Projects that tied the engine to the browser paid for it: OpenCut archived the editor and started over; Omniclip's MCP requires an open tab; FreeCut's API starts a Chrome through Playwright (`search/post-mortems.md` §12, `search/projects/`).
- Consequence accepted by the author: text and animations are drawn by our own code, with a smaller set of effects than CSS/canvas would offer. Since there will be two pixel executors (browser for preview, FFmpeg for export), the tolerance between them must be defined and tested (`search/post-mortems.md` §1).
- Impact: ADR of the core, ADR of the render, SDD of the command catalogue, SDD of the render pipeline, SDD of tests (reference frames), all phases of the ROADMAP.

### D-002 — The editor replaces the Studio in `web/`

- Date: 2026-11-14
- Decision: the editor starts in `web/` and replaces the Studio app. The useful parts are reused (CI, docs site, login, job limits, FFmpeg call, MP4 check). The Studio code stays preserved in the GitHub history.
- Alternatives considered: (a) evolve the Studio and build the editor inside it: it would inherit decisions from another product (`libx264` encoder from a GPL build, Remotion, job model); (b) coexist, with Studio and editor in separate folders: two products in one repository, more complex CI and releases.
- Reason: the Studio is another product (brief → prompt → FFmpeg → MP4, with no timeline or editing). See `search/notes/existing-studio.md`.
- Note: the "brief → video" flow may come back later as an agent that assembles the timeline through the command core. It stays out of the MVP until a decision to the contrary.
- Impact: ROADMAP (task group for removing and reusing the Studio), ADR of repository structure, decision on history and tags (open).

### D-003 — Local and self-hosted first; SaaS later

- Date: 2026-11-14
- Decision: the first operating mode is local and self-hosted: a single binary, SQLite, files on disk, one user or a few. Multi-user SaaS comes later, on top of the same core.
- Alternatives considered: (a) multi-user SaaS first (Postgres, S3, resumable upload, quotas, billing): more reach, but much more surface before the first exported video; (b) both from the prototype: doubles the test matrix in the first phase.
- Reason: no uploading of gigabytes, no cloud cost, and no patent risk from whoever charges for encoding H.264 (`search/notes/licensing.md`). It fits vukem, which already ships a single binary with SQLite in pure Go (`search/notes/vukem.md` §3.3).
- Impact: persistence ADR (SQLite first; Postgres out of the MVP), storage ADR (disk first; S3 out of the MVP), upload ADR (import by local path first), queue ADR (vukem's `database` driver), PRD (SaaS requirements go to "later").

### D-004 — Web first; packaged desktop app in BETA

- Date: 2026-11-14
- Decision: web comes first. The initial "desktop" is the same Go binary serving the UI on localhost, opened in Chrome. The packaged desktop app enters in BETA, after a test on real machines.
- Alternatives considered: (a) web and packaged desktop in parallel from the prototype: would require solving webview, signing and auto-update before the editor exists; (b) desktop first: delays self-hosting on a server and vukem's field test as a web app.
- Reason: Wails and Tauri use the system webview, and no evidence was found of usable WebCodecs/WebGPU in WKWebView (macOS) or WebKitGTK (Linux); only WebView2 (Windows) is Chromium (`search/notes/stack-browser.md`, `search/notes/stack-backend-desktop.md`).
- Open: the desktop shell (Wails v3, stable since 2026-10-26; Electron; Tauri v3 with embedded Chromium, still in alpha) is decided before BETA, based on a feature-detection test on real machines.
- Impact: ROADMAP (desktop track starts in BETA; `desktop/` stays empty in the Prototype and ALPHA, except for the webview test), desktop ADR.

### D-005 — Continue the existing git history; the editor starts at v0.5.0

- Date: 2026-11-14
- Decision: the product keeps the history of `AlexGalhardo/vukem-video`. The local repository in `video/` fetches the remote `main`, and the restructuring (move the Studio tree under `web/`, then remove the Studio) lands as ordinary commits through pull requests. No force-push, no deleted tags or releases. The first editor release is `v0.5.0`, with a `BREAKING` note.
- Alternatives considered: (a) start clean: force-push a new history, delete the `v0.1.0`–`v0.4.0` tags and releases, restart at `v0.1.0`; needs the `main` protection switched off and erases the Studio from the repository; (b) a new repository for the editor: clean history without destroying anything, but changes the remote the author named.
- Reason: the remote `main` is protected (pull request only, linear history) and already has releases `v0.1.0` to `v0.4.0` (`search/notes/vukem.md` §1.7). Continuing avoids a tag collision and keeps the Studio code recoverable, which D-002 relies on.
- Caution for whoever wires the git: the Studio tree has its own `docs/` (including a `docs/brainstorming.md`). Checking out `origin/main` into `video/` would overwrite this planning folder. The wiring must not write the remote tree into the working directory; the first ROADMAP task spells out the safe procedure.
- Impact: ROADMAP (first task group: git wiring and monorepo layout), releases SDD (version line starts at `v0.5.0`), CHANGELOG.

### D-006 — H.264 policy: ship only an LGPL FFmpeg, encode H.264 with the system encoder

- Date: 2026-11-14
- Decision: everything we distribute (Docker image, desktop installer) contains only an LGPL build of FFmpeg. H.264 is produced by the operating system or hardware encoder (Media Foundation, VideoToolbox, NVENC, QSV, VAAPI) or by OpenH264 downloaded from Cisco on first run, the way Firefox does it. A user who wants x264 points the product at their own FFmpeg binary.
- Alternatives considered: (a) bundle a GPL FFmpeg with x264: best quality and identical output on every machine, but the image and installer would carry a GPL binary and a software H.264 encoder, with the patent exposure on us; (b) ship no FFmpeg at all and only detect the user's: no licence or patent obligation, worse first run and more variation to test.
- Reason: x264 is GPL; of the prebuilt binaries surveyed only the BtbN `lgpl` variant is actually LGPL; publishing an image or installer counts as distribution; the Cisco patent coverage for OpenH264 applies only to the binary the user downloads from Cisco (`search/notes/licensing.md`). The author is not a lawyer and neither is the architect; the points that need legal advice are listed in that file.
- Accepted cost: encode quality and speed vary by machine, and the encoded file is not byte-identical across machines. Reference tests therefore compare composed frames, not encoded bytes (`search/post-mortems.md` §10).
- Consequence for the existing code: the Studio's `-c:v libx264` and `apk add ffmpeg` (a GPL build) do not carry over.
- Impact: codec ADR, FFmpeg distribution ADR, render pipeline SDD (encoder selection and fallback order), security SDD (binary provenance), Dockerfile, ROADMAP risk list (a Linux server without GPU has only OpenH264 as H.264 encoder).

### D-007 — Product name: Vukem Video

- Date: 2026-11-14
- Decision: the product is called "Vukem Video". The binary and CLI are `vukem-video`. The provisional name `cutroom` is dropped.
- Alternatives considered: (a) `cutroom`: short own brand, but needs a name-conflict check and leaves the repository with a different name from the product; (b) decide later: renaming the Go module and the CLI after people use them is a breaking change.
- Reason: matches the repository and the vukem family (vukem.dev, studio.vukem.dev) and makes the product the framework's showcase.
- Not verified: trademark, domain and package-name availability were not checked for any name.
- Open: the Go module path is still `github.com/AlexGalhardo/vukem-studio` in the Studio's `go.mod`; the new path follows the repository (`github.com/AlexGalhardo/vukem-video/...`) and is fixed in the repository-layout ADR.
- Impact: every SDD that names commands, the CLI contract, the MCP server name, the docs site, the README.

### D-008 — The whole repository is in English

- Date: 2026-11-14
- Decision: everything in the repository is written in English: planning documents (`docs/`), ROADMAP, AGENTS.md, README, code, commits, releases. The documentation site is English first, with pt-BR as the second locale. This overrides the briefing's "docs in pt-BR".
- Alternatives considered: (a) planning documents in pt-BR and public files in English (the briefing's rule); (b) everything in pt-BR.
- Reason: the briefing contradicted itself: it asked for pt-BR documents and for the vukem practices, and both vukem and the Studio keep the repository in English ("Keep the repository in English", `web/README.md`). The author chose the vukem practice.
- Consequence: the documents already written in pt-BR (`docs/STATUS.md`, `docs/brainstorming.md` up to D-004, `docs/search/**`) are translated, and their folders and file names move to English names. The conversation with the author stays in pt-BR.
- Impact: every document from here on; a translation task for the interns; names of the research files.

### D-009 — MVP scope: the core plus keyframes, transcription, silence removal and templates

- Date: 2026-11-14
- Decision: the MVP contains the core set (import; cut, split and trim on several tracks; text; captions with per-word highlight; music with volume and fade; basic transitions; 9:16 format; MP4 1080p export; undo/redo) plus all four borderline items: simple keyframes (position, scale, rotation, opacity), automatic transcription, silence removal and reusable templates.
- Alternatives considered: the architect recommended only keyframes and transcription in the MVP, leaving silence removal and templates for the next phase. The author chose all four.
- Reason: captions are mandatory in practice for short video and the timed transcript is also the cheapest way for an agent to "see" content; keyframes are the base of motion design for commercial video; silence removal is high value for talking-head cuts; templates serve commercial video and agents (`search/notes/features.md`, `search/notes/agents-mcp.md`).
- Sequencing consequence (architect's call, not a scope cut): templates depend on a stable project format, so they are the last MVP item to be built; silence removal depends on transcription or on audio analysis and comes after it.
- Out of the MVP: AI video generation, upscale, multicam, advanced colour grading, audio mixer, plugins, motion tracking, real-time multi-user collaboration, direct publishing to social networks, chroma key and background removal, screen and camera recording. Each gets a one-line justification in the PRD.
- Impact: PRD (functional requirements), command catalogue SDD, timeline schema SDD (keyframes, caption items, template slots), ROADMAP phase contents.

### D-010 — Transcription: local by default, external API optional

- Date: 2026-11-14
- Decision: transcription runs locally by default with whisper.cpp (offline, free, audio never leaves the machine). A user with an API key can switch to an external provider.
- Alternatives considered: (a) local only: simpler and private, slow on weak machines and no way out when quality is not enough; (b) API only: less code and better quality, but needs a key and internet from first use and sends the user's audio to a third party.
- Reason: consistent with local-first (D-003). Prices checked in November 2026: Deepgram US$ 0.0043 per minute, Groq Whisper turbo US$ 0.04 per hour (`search/notes/stack-backend-desktop.md`).
- Not measured: whisper.cpp speed on CPU and the quality of its word-level timestamps. This is a spike candidate before the transcription SDD is frozen.
- Impact: transcription ADR (provider interface), security SDD (the API key is a secret; sending audio out is an explicit user choice), distribution (whisper.cpp binary and model download, model licence).

### D-011 — Agents act autonomously, bounded by undo, a trash and resource limits

- Date: 2026-11-14
- Decision: an agent edits and exports without asking for human confirmation. Every edit is reversible by undo and recorded in the log per actor. Deleting a project or media moves it to a trash instead of destroying it. Exports have a concurrency limit. The agent has no tool that sends data out of the system.
- Alternatives considered: (a) human confirmation in the UI for expensive or destructive actions: safer, but breaks unattended use (an agent running alone from the CLI); (b) draft mode, where the agent edits a separate proposal that a human approves: maximum control, one more concept in the model and in the UI.
- Reason: in local mode the agent belongs to the user, and unattended operation is the product's differential. The security research treats transcripts, file names and metadata as untrusted text that may try to give the agent orders; the mitigation chosen is to remove the dangerous capabilities (no exfiltration channel, no irreversible delete) rather than to add confirmations (`search/notes/agents-mcp.md`, part E).
- Impact: security SDD (threat model for prompt injection, trash, limits), command catalogue SDD (every command declares its undo; delete commands are soft), MCP/CLI/API contracts (no outbound tools; import by URL needs SSRF rules or stays out), real-time protocol SDD (actor on every operation).

### D-012 — No login on localhost; login and scoped API keys when exposed on the network

- Date: 2026-11-14
- Decision: when the server listens on the local machine (127.0.0.1) the UI opens without login, and the CLI and the MCP server use a local token generated at install time. When the server is exposed on the network, login and API keys with scopes become mandatory.
- Alternatives considered: (a) login always: one model for local and server, but friction on first local use and for the agent; (b) no authentication at all until BETA: fastest, leaves server self-hosting insecure until then.
- Reason: local-first (D-003) and agent-first; vukem already provides sessions and opaque tokens with abilities (`search/notes/vukem.md` §3.3).
- Security note to carry into the SDD: "no login on localhost" still needs the local token on every API call plus Origin and Host checks, otherwise any web page the user opens could drive the editor through the browser (DNS rebinding and cross-site requests against localhost).
- Impact: security SDD, API contract (authentication section), CLI contract (where the token is stored), ROADMAP security track.

### D-013 — Manual UI in the MVP: complete coverage, polish later

- Date: 2026-11-14
- Decision: in the MVP every command can be performed from the UI (timeline, preview, inspector), without fine polish: no interface animations, advanced shortcuts or onboarding. Polish comes in BETA.
- Alternatives considered: (a) a companion UI that shows the timeline and preview and allows only basic adjustments, the rest by agent or CLI: ships earlier, leaves out people who do not use an agent; (b) CapCut-level polish in the MVP: best first impression, the most expensive track, and it competes for time with the agent differential.
- Reason: the product promise is that a human and an agent can do the same things on the same project. Coverage proves the single command core; polish does not.
- Testable consequence: a contract test lists the command catalogue and fails if a command has no UI entry point.
- Impact: PRD (UI requirements), ROADMAP UI track, test strategy SDD (UI coverage of the catalogue).

### D-014 — Bundled media: open fonts only

- Date: 2026-11-14
- Decision: the product ships a small set of OFL fonts and nothing else. Music, sound effects, images and video are the user's own files.
- Alternatives considered: (a) fonts plus a small CC0 pack of music and sound effects so templates work out of the box: needs curation and a provenance record per file; (b) integration with stock services (Pexels, Pixabay) using the user's key: huge library, depends on third parties, internet and terms that change.
- Reason: OFL fonts can be redistributed; the Pixabay licence does not allow putting the files in an MIT repository; CC0 does (`search/notes/licensing.md`). Bundled fonts are also what makes text identical in preview and export, since the render must never use system fonts (`search/post-mortems.md` §1 and §6).
- Consequence for templates (D-009): a template references fonts from the bundled set and leaves media as slots the user fills.
- Impact: licensing notes, font ADR (which families, how they travel with a project), timeline schema SDD (font references), template SDD.

### D-015 — Public hosting: the documentation site only

- Date: 2026-11-14
- Decision: the only thing hosted publicly is the static documentation site, on GitHub Pages, as vukem does. The product is distributed as a binary and a Docker image. There is no public instance of the editor.
- Alternatives considered: (a) docs plus a public demo with strict limits: good showcase, but server and render cost and abuse protection from day one; (b) docs served from a container on Railway, as the Studio did: works, costs money, no advantage over a static site.
- Reason: local-first (D-003); no service to keep running or to protect.
- Consequence: the Studio's Railway files (`railway.toml`, `railway.app.toml`, `.railway/`, `.railwayignore`) and the site's standalone server do not carry over.
- Impact: docs site SDD (Astro static build, GitHub Pages deploy), CI/CD SDD, ROADMAP docs track, removal list for the Studio.

### D-016 — No telemetry by default

- Date: 2026-11-14
- Decision: the product sends nothing out. Structured logs stay local. Whoever hosts it can turn on OpenTelemetry pointing at their own collector. No Sentry, no usage analytics.
- Alternatives considered: (a) optional error reporting with Sentry, off by default: helps find real-media bugs, needs an account, a privacy policy and care not to leak file names; (b) anonymous usage telemetry to guide the roadmap: real data on which features are used, tends to be rejected in open source projects.
- Reason: consistent with local-first and open source, and there is no service to maintain. vukem already has an OpenTelemetry driver as a separate module (`search/notes/vukem.md` §3.3).
- Consequence: Sentry leaves the stack draft. Bug reports rely on a local diagnostic bundle the user chooses to attach to an issue.
- Impact: observability ADR, security SDD (no outbound traffic by default is a stated property that can be tested), ROADMAP.

### D-017 — OpenH264 inside the pinned FFmpeg build: accept now, own build before 1.0

- Date: 2026-11-14
- Decision: through the Prototype and ALPHA the product distributes the BtbN `lgpl` FFmpeg build as it is, although it has OpenH264 compiled in. Before the 1.0 release the project produces its own LGPL FFmpeg build without OpenH264, which loads the Cisco binary at run time, the model Fedora uses.
- Alternatives considered: (a) keep the BtbN build at 1.0 as well: no extra work, the patent exposure of a software encoder stays with the distributor; (b) an own build from the start: cleaner, but it puts an FFmpeg build pipeline for three operating systems before the editor exists.
- Reason: the Cisco patent coverage applies only to the binary a user downloads from Cisco, not to a build from source (`search/licensing.md` §4 and §8). D-006 had assumed the download model; this decision states how to get there. Neither the author nor the architect is a lawyer.
- Impact: ADR-0011 (the open point is closed), ROADMAP (a BETA task group for the FFmpeg build pipeline; an exit criterion of BETA), licensing notes.

### D-018 — Default caption position: inside the safe area published by Meta

- Date: 2026-11-14
- Decision: on vertical video the default caption sits with its bottom edge at 65% of the frame height, above the bottom 35% that platform interface covers. The lint warns when text leaves the safe area (top 14%, bottom 35%, sides 6%).
- Alternatives considered: (a) lower, at about 78% of the height, where many creators place captions: more familiar, with the risk of the post description covering part of it; (b) vertically centred: never covered, but it hides the subject.
- Reason: the only safe-area margins found in an official source are the ones Meta publishes for Reels ads (`search/features-20-80.md` §5). No official margin was found for TikTok, Kwai, X or Shorts.
- Consequence: a user who wants the lower position sets the caption style position to `custom`; the lint then reports it as a warning, not an error.
- Impact: SDD-004 §4.1 (already written this way), the lint rule for text outside the safe area (SDD-003), the safe-area guide of the UI.

### D-019 — Three starter templates ship in ALPHA

- Date: 2026-11-14
- Decision: the product ships three templates, as JSON documents without media: a vertical cut with captions, a product teaser with animated text, and a quote card.
- Alternatives considered: (a) no template, only the mechanism: less to maintain, the feature is invisible on first use; (b) a gallery of ten or more: a better showcase, continuous design work that competes with the editor.
- Reason: they prove the template feature, give agents worked examples of slots, and cost little because they contain no media (D-014).
- Impact: ADR-0022, ROADMAP (a task in the templates group; the templates are also fixtures of the template tests), documentation site (a page per template).

### D-020 — The git wiring and first push are the first ROADMAP task

- Date: 2026-11-14
- Decision: this planning session does not touch the git state beyond the empty local repository in `video/`. Connecting it to the remote history (D-005), moving the Studio tree under `web/` and opening the first pull request are the first task of the next session, following a written safe procedure, and the author confirms before any push.
- Alternatives considered: (a) wire the remote and open a pull request at the end of this session; (b) commit locally only, which would later need those commits reapplied on top of the remote history.
- Reason: the remote tree has its own `docs/` folder, so checking it out into the working directory would overwrite the planning documents. The operation deserves its own reviewed step.
- Risk accepted: until that task runs, the planning documents exist only on disk.
- Impact: ROADMAP task group P0 (repository foundation), AGENTS.md (the rule that only the orchestrator pushes, after confirmation by the author).

### D-021 — Everything of the Studio leaves the tree

- Date: 2026-11-14
- Decision: the repository keeps only the editor. Besides the Studio application code, its non-code content is removed too: the skill kits under `web/.agents/skills` (3,177 files, in Portuguese), the launch film under `web/films`, the Studio research under `web/docs`, and its `prompt.md` and `ROADMAP.md`. All of it stays recoverable in the git history.
- Alternatives considered: (a) keep the skill kits for a future brief-to-video agent: three thousand files in Portuguese in a repository that is entirely in English (D-008); (b) copy the content to a separate repository first: nothing leaves an active tree, one more manual step.
- Reason: D-002 (the editor replaces the Studio), D-008 (everything in English), and a small repository.
- What is reused is copied into its new place before the removal: the CI workflows, git hooks, `cliff.toml`, the lint configurations, the documentation site shell, and the ideas (not the code as is) of the FFmpeg runner and the MP4 checks (`search/notes/existing-studio.md`).
- Impact: ROADMAP task group P0 (the removal is its own commit, after the reusable files moved), ADR-0001.

### D-022 — Bundled fonts: a curated set of six families

- Date: 2026-11-14
- Decision: the binary embeds six open font families: Inter (general text, the default), Montserrat (titles), Anton and Bebas Neue (impact captions in the social-media style), Playfair Display (serif) and JetBrains Mono (monospace).
- Alternatives considered: (a) only Inter and Anton: smaller binary, fewer choices; (b) twelve families: more variety, a few megabytes more and more licences to check.
- Reason: covers captions, commercials and technical videos; fonts are the only bundled media (D-014) and what makes text identical in preview and export.
- Not verified yet: the licence file of each family and which weights are shipped. A ROADMAP task checks each licence at its source, records it next to the font files and fixes the list of weights; a family whose licence is not OFL or equivalent is replaced and the replacement recorded here.
- Impact: ADR-0012, SDD-001 §3.6 (the default family `Inter` and the list in `engine/raster/fonts/FONTS.md`), SDD-004 §5.2, third-party notices.

### D-023 — Prototype UI: live view plus the four operations by hand

- Date: 2026-11-14
- Decision: in the Prototype the UI shows the timeline and the preview updating live while an agent edits, plays the video, and lets a person do the same four things the agent does in the reference scenario: import a file, cut or trim, add a caption, export.
- Alternatives considered: (a) view only, with manual editing starting in ALPHA: reaches the end of the Prototype sooner, postpones the proof that the UI uses the same commands; (b) a complete basic editing UI with drag, mouse trimming, inspector and undo: a better demonstration, a longer first phase.
- Reason: it proves both doors on the same command core in the first phase, which is the point of the vertical slice.
- Consequence: drag interactions, the inspector and full coverage of the catalogue are ALPHA work (D-013).
- Impact: PRD-001 (phase of RF-070 and RF-072 stays Prototype; RF-071 stays ALPHA), SDD-012, ROADMAP UI track, the Prototype exit criterion.

### D-024 — Interface theme: dark by default, light available

- Date: 2026-11-14
- Decision: the interface is dark by default, with a light theme available through a toggle.
- Alternatives considered: (a) follow the operating system preference: neutral, but a light system means editing video on a white background; (b) dark only: fewer states to design and test.
- Reason: video editors use a dark surround so the interface does not bias the perception of the picture. The Studio already had a theme toggle to reuse.
- Impact: SDD-012 (design tokens for two themes), UI tests in both themes for contrast (RF-075).

### D-025 — Working files live at the workspace root, outside the product repository

- Date: 2026-11-14
- Decision: `ROADMAP.md`, `STATUS.md`, `AGENTS.md` (the team protocol) and `vukem-feedback.md` live in the workspace root `C:/dev/vukem-video/`, which has no git repository. The product repository `video/` holds the product, its `docs/` and its public files.
- Alternatives considered: the root of the product repository (versioned and published with the product), which is where the briefing first placed `ROADMAP.md`.
- Reason: the author moved `vukem-feedback.md`, `STATUS.md` and `ROADMAP.md` there during the session and, asked on 2026-11-14 which root `ROADMAP.md` belongs to, chose the workspace root. Working files of the agent team are not part of what is published.
- Impact: the ROADMAP is not versioned or published with the product; RF-214 no longer lists it among the repository files. Paths inside ROADMAP tasks are relative to `video/`. This file refers to `STATUS.md` in the workspace root.

### D-026 — One legacy release, then one minor release about every 90 commits; pull requests are rebase-merged

- Date: 2026-11-14
- Decision: (1) the five Studio releases `v0.1.0` to `v0.4.0` are folded into one release `v0.1.0`, whose annotated tag points to the last Studio release commit (`6a420b6`, formerly `v0.4.0`) and whose notes carry the notes of the five; the tags and releases `v0.2.0`, `v0.3.0`, `v0.3.1`, `v0.4.0` are deleted. (2) The editor releases continue the line with sequential minors: `v0.2.0`, `v0.3.0`, ... A new release is cut when `main` has gathered about 90 commits since the previous tag (an average, not a hard number), at a point where the release pipeline is green. (3) Task pull requests are rebase-merged, so their microcommits (Conventional Commits) reach `main` and are what is counted and what the changelog reads.
- Alternatives considered: keep the five Studio releases and start the editor at `v0.5.0` (D-005, ADR-0025); one release per task group; squash-merge with one commit per task.
- Reason: asked by the author on 2026-11-14, after the layout pull requests were merged.
- Impact: supersedes the version line of D-005 and items 1 and 2 of ADR-0025 (ADR-0028); SDD-009 sections 4 and 6.3; ROADMAP (completion protocol items 5 and 6, P1-REL-008, the Prototype exit criterion 8, the group audits that name a version); `CONTRIBUTING.md`. Done on the remote the same day: the tag `v0.1.0` was moved once (the only forced tag push) and the four other tags were deleted.

### D-027 — Release notes have five sections only: Added, Changed, Removed, Fixed, Security

- Date: 2026-11-14
- Decision: a release note (and each `CHANGELOG.md` section) lists only **Added** (something new, `feat`), **Changed** (a breaking change of a public interface, a commit marked `!` or `BREAKING CHANGE:`), **Removed** (something taken out, `revert` or a `Removed:` footer), **Fixed** (a correction, `fix`) and **Security** (a `Security:` footer or the scope `security`). Nothing else: no introduction, no Deprecated section, no "formerly" lines; `perf`, non-breaking `refactor`, `docs`, `test`, `ci`, `build`, `chore` and `style` are left out. The legacy release `v0.1.0` was rewritten in this form the same day.
- Alternatives considered: the vukem mapping (`perf` and `refactor` under Changed, a Deprecated section).
- Reason: asked by the author on 2026-11-14.
- Impact: SDD-009 section 5.1 item 5 and the `commit_parsers` of section 5.2; `CONTRIBUTING.md`; the release step of the team workflow (`AGENTS.md`).
