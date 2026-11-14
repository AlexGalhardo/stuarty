# Synthesis of the research

> Stage 1, research. All sources accessed 2026-11-14.
> This file lists the conclusions that most change the architecture, each with the evidence behind it and what it implies, then the main technical risks and the questions the research left open. Detail and citations are in the thematic files of this folder; raw evidence is in [`notes/`](notes/) and [`projects/`](projects/).
> Decisions referred to as D-xxx are the author's, in [`../brainstorming.md`](../brainstorming.md); ADRs are in [`../adrs/`](../adrs/).

## Contents

1. [The conclusions](#1-the-conclusions)
2. [Main technical risks](#2-main-technical-risks)
3. [Questions the research raised, and where each was settled](#3-questions-the-research-raised-and-where-each-was-settled)
4. [What remains open](#4-what-remains-open)
5. [How strong the evidence is](#5-how-strong-the-evidence-is)

## 1. The conclusions

### 1. An engine tied to its browser UI cannot be given to agents afterwards

- **Evidence.** OpenCut (98 thousand stars) archived its editor in May 2026 and restarted; it has no MCP, CLI or API, its undo was non-serializable classes and its render needs the DOM. Omniclip's new MCP needs a browser tab open and has no headless render. FreeCut's agent API starts Chrome through Playwright and duplicates the UI's operations in a second dispatcher. FilmCraft, built from one command registry dispatched by UI, CLI and MCP, is the counter-example. Sources: [`post-mortems.md`](post-mortems.md) §12, the project profiles.
- **Implication.** The command core and the render must work with no UI and no browser from the first day. This is D-001 and ADR-0002.

### 2. Having an MCP server is no longer a differential; the combination is

- **Evidence.** CapCut has an official integration with Codex over MCP and Canva has an official MCP (found by one research pass, not rechecked). FilmCraft and Diffusion Studio's `editor` are agent-first and open. Source: [`agents-mcp.md`](agents-mcp.md) §2.
- **Implication.** The product is the combination nobody in that set has: a single binary that edits and renders with no browser, a permissive licence, a human UI that is a client of the same serializable commands, and a text-first verification loop.

### 3. Preview and export diverge wherever two things decide what a frame contains

- **Evidence.** Remotion #12603 (two rounding rules, one frame apart), #11315 and #5984 (fallback and per-system fonts); FreeCut's series of divergences from a DOM playback path and a canvas export path; Reframe's DOM preview against a filter graph. Source: [`post-mortems.md`](post-mortems.md) §1.
- **Implication.** One function decides the content of a frame; executors only place pixels; text is rasterized once by our own code with bundled fonts; the paused frame comes from the export executor. ADR-0003, ADR-0012.

### 4. Time must be integer inside the core

- **Evidence.** Editly #49 (the same specification renders different durations at different frame rates), MLT #69, Remotion #12603; OpenCut replaced float seconds because float "accumulated rounding errors"; the author of mediabunny advises an integer coordinate system. Source: [`engine.md`](engine.md) §2.
- **Implication.** Time is an integer number of frames at the project's rational frame rate; seconds and timecode exist only at the edge, as strings with a unit. ADR-0004.

### 5. A server-ordered operation log with last-writer-wins is enough; a CRDT is not needed

- **Evidence.** Figma's published design: an authoritative server orders events, conflicts resolve per property, operational transforms and full CRDTs were judged unnecessary for a document of objects. FreeCut and Omnitool already use a base revision for agent edits; OpenChatCut applies an agent's batch as one undo step. Source: [`engine.md`](engine.md) §4, [`agents-mcp.md`](agents-mcp.md) §7.
- **Implication.** Batches with a base revision, an idempotency key and a dry run; an append-only log with the actor; undo as a compensating batch per actor. ADR-0006.

### 6. Phone footage must be normalized before it is edited

- **Evidence.** Hardware encoders produce variable frame intervals at a nominal 30 fps (OBS #13396); rotation and mirroring live in a display matrix that tools mishandle (Remotion #5509, mediabunny #492); HLG footage from iPhones looks washed out after conversion; long GOPs make seeking slow. Source: [`engine.md`](engine.md) §5.
- **Implication.** Every import produces normalized media and a proxy; preview and export read those, never the original. ADR-0010.

### 7. Agents need a small tool list and a text-first loop

- **Evidence.** Tool selection degrades beyond 30 to 50 tools and OpenAI advises fewer than 20; the Resolve MCP recommends 37 compound tools over 390; a 1000×1000 image costs 1,296 tokens in Claude, so a visual pass over a 60 second video costs about 15,000 tokens; Figma and Playwright both steer agents to text before images. Source: [`agents-mcp.md`](agents-mcp.md) §4 and §6.
- **Implication.** Fifteen MCP tools by intent, edits through one `edit` tool carrying typed operations; every write returns a text diff and lint findings; images only on request, from the export path. ADR-0015.

### 8. For coding agents a CLI with JSON and a skill is the primary surface

- **Evidence.** Playwright recommends its CLI with skills for most coding agents; Shotstack ships both and uses exit codes 0, 1 and 2; a vendor benchmark found 4 to 32 times more tokens through MCP on one service. No benchmark covers video editing. Source: [`agents-mcp.md`](agents-mcp.md) §5.
- **Implication.** The CLI is specified first and both surfaces are generated from one registry. ADR-0016.

### 9. The licence position decides the encoder, and the encoder varies by machine

- **Evidence.** x264 is GPL; only the BtbN `lgpl` build is LGPL among the prebuilt binaries surveyed; publishing an image or installer is distribution; Cisco's patent coverage for OpenH264 applies only to its own downloaded binary; H.264 patents remain active in the United States and Brazil. The spike found that the LGPL build has every filter needed except `eq` and `boxblur`, and that `h264_mf`, `libopenh264` and `h264_nvenc` work on the test machine. Sources: [`licensing.md`](licensing.md), [`spikes.md`](spikes.md).
- **Implication.** Ship only an LGPL FFmpeg as a separate process; encode with the operating system or hardware encoder first; compare composed frames, not encoded bytes, in tests. D-006, ADR-0011.

### 10. vukem already provides most of the server stack, and lacks three things

- **Evidence.** In the core: an Inertia adapter, SQLite, ORM and migrations, a queue with a database driver, server-sent events, tokens with scopes, OpenAPI generation, a command registry, a safe process runner. Missing: progress and cancellation of a job, an MCP server for applications, a documented way to serve a client-rendered page. Source: [`vukem-practices-and-releases.md`](vukem-practices-and-releases.md), [`stack-tradeoffs.md`](stack-tradeoffs.md) §6.
- **Implication.** Most of the stack draft is cut (PostgreSQL, sqlc, goose, river, asynq, tusd, cobra, WebSocket). The three gaps are covered by product code and reported as VF-002, VF-003 and VF-004.

### 11. Local-first removes the hardest parts of a browser editor

- **Evidence.** The memory and export post-mortems (OpenCut #914, FreeCut #316, ffmpeg.wasm #876, mediabunny #144) all come from doing heavy work in a tab. With the process on the user's machine, a command round trip takes milliseconds. Source: [`performance.md`](performance.md) §1.
- **Implication.** The browser is a thin client with no editing logic and no media pipeline; WebCodecs, OPFS and Workers are not needed in the MVP. D-003, ADR-0007, ADR-0013.

### 12. The system webview is a risk only if the browser does media work

- **Evidence.** No evidence of usable WebCodecs or WebGPU was found for WKWebView or WebKitGTK; WebView2 is Chromium; Wails v3 has been stable since 2026-10-26; Tauri with Chromium is in alpha. Source: [`stack-tradeoffs.md`](stack-tradeoffs.md) §7.
- **Implication.** Because the preview needs only `<video>`, a WebGL2 canvas and server-sent events, a system webview is likely enough and Wails v3 is the first candidate, to be confirmed by a test on real machines before BETA. D-004, ADR-0023.

### 13. A silent failure is the worst failure for an agent

- **Evidence.** OpenCut's export stuck at 5% with no error; FreeCut reporting "Export complete" for a file with no audio; Remotion rendering wrong frames with no message. Source: [`post-mortems.md`](post-mortems.md) §9.
- **Implication.** Export is a job with real progress and typed errors, and is reported successful only after the file passed verification. ADR-0018.

### 14. Safety for agents comes from removing capabilities, not from confirmations

- **Evidence.** The "lethal trifecta" analysis: private data, untrusted content and external communication together are what make prompt injection dangerous. In a video editor the untrusted content is the transcript, file names and metadata. Source: [`agents-mcp.md`](agents-mcp.md) §9.
- **Implication.** No outbound capability, no irreversible delete, resource ceilings, untrusted text marked as data. D-011, ADR-0020.

### 15. The release pattern to follow is concrete and already automated

- **Evidence.** vukem's 58 releases share one format: title equal to the tag, body copied from the changelog section, entries with scope and commit link, signed binaries, SBOM and provenance, one release per task group. Source: [`vukem-practices-and-releases.md`](vukem-practices-and-releases.md).
- **Implication.** The pipeline is adapted, not invented, with two changes: the image also goes to Docker Hub and contains a pinned LGPL FFmpeg. ADR-0025.

## 2. Main technical risks

| # | Risk | Why it is a risk | Mitigation |
| --- | --- | --- | --- |
| R1 | The Go compositor is too slow | Composing every frame in Go moves 8.3 MB per frame in and out of pipes | Measured by `spikes/go-compositor`; two fallbacks defined in SDD-004 §12 that change only `engine/export` |
| R2 | The Go text stack falls short | `go-text/typesetting` is version 0.x; colour emoji and complex scripts are unverified | A Prototype task measures them before the text contract is frozen; fallback library named in ADR-0012 |
| R3 | H.264 on a Linux server without a GPU | Only OpenH264 is available in an LGPL build, with the licence question of [`licensing.md`](licensing.md) §8 | Policy decided in D-017: an own FFmpeg build before 1.0 |
| R4 | Preview and export still diverge | Two pixel executors remain | Reference-frame tests in CI with a numeric tolerance; truth on pause |
| R5 | vukem changes under the project | Pre-1.0, with breaking changes in recent releases | Pin the version; budget upgrade work in each phase; log friction |
| R6 | `<video>` seek is not precise enough for scrubbing | No primary source on its precision | Short-GOP proxies; measurement in the Prototype; WebCodecs as the deferred option |
| R7 | The MCP Go SDK lacks parts of revision `2026-07-28` | Its support for the tasks extension is unverified | The design never depends on tasks; a Prototype task checks the SDK |
| R8 | HLG footage looks wrong after normalization | Tone mapping has more than one answer | A test set of real iPhone footage before the ingest profile is frozen |
| R9 | whisper.cpp is slow or its word timings are poor on CPU | Unmeasured before the spike | `spikes/whisper-local`; an API provider is the configured alternative |
| R10 | The ALPHA scope is large | D-009 put 18 features in it | Split into releasable task groups, templates last |

## 3. Questions the research raised, and where each was settled

| Theme | Question | Settled by |
| --- | --- | --- |
| Scope | What happens to the existing Studio application | D-002 |
| Scope | Local and self-host first, or a hosted service | D-003 |
| Scope | Which borderline features enter the MVP | D-009 |
| Web and desktop | Which comes first | D-004 |
| Web and desktop | How much code they share | ADR-0007, ADR-0023: everything but the shell |
| Core and render | Language of the core and what is the truth of the render | D-001, ADR-0002, ADR-0003 |
| Core and render | Tolerance between preview and export | ADR-0003 item 6 |
| Core and render | Export from the normalized media or the original | ADR-0010 |
| Agent tools | Shape of the MCP catalogue; CLI or MCP first | ADR-0015, ADR-0016 |
| Agent tools | Human confirmation or autonomy | D-011, ADR-0020 |
| Stack | Database, queue, real time, upload | D-003, ADR-0009, ADR-0017, ADR-0018 |
| Codecs and licences | H.264 policy | D-006, ADR-0011 |
| Captions | Local or API transcription | D-010, ADR-0021 |
| Security | Login and scopes | D-012, ADR-0019 |
| Hosting and telemetry | What is hosted; what is collected | D-015, D-016 |
| Repository | History, name, language | D-005, D-007, D-008 |
| UI | Ambition of the manual UI in the MVP | D-013 |
| Media | Bundled media | D-014 |

## 4. What remains open

For the author:

1. Docker Hub credentials as repository secrets, needed by the release workflow.
2. The costs of code signing and notarization, at BETA.

Settled after the first version of this file: the OpenH264 point of [`licensing.md`](licensing.md) §8 (D-017), the default caption position (D-018), starter templates (D-019) and when the git history is wired (D-020).

For measurement in the Prototype (each is a ROADMAP task):

3. Go compositor throughput (R1), text stack quality (R2), `<video>` seek precision (R6), MCP SDK coverage (R7), HLG tone mapping (R8), whether vukem's tooling runs with the module root one level above the application (ADR-0001).

## 5. How strong the evidence is

- Strong: repository states, versions, licences and the issues cited, which were read at the source and partly rechecked by the architect ([`notes/architect-verifications.md`](notes/architect-verifications.md)).
- Medium: architecture of the reference projects, read from code without running it.
- Weak: which features creators use (vendor data only); official upload specifications of Instagram, TikTok, X and Kwai (partly blocked, none for Kwai); anything marked `[training]` in the notes.
- Absent: any benchmark of video editing by agents, or of an export pipeline comparable to this one.
