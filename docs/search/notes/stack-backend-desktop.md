# Backend stack Go + desktop + engines: raw research

> **Status of this file.** Research notes delivered by a research agent on 2026-11-14 and reviewed by the architect. The opinions and recommendations in this file predate the author's decisions (`../../brainstorming.md`, D-001 to D-016) and the ADRs (`../../adrs/`). Where they differ, the ADRs prevail. Facts rechecked by the architect are listed in [`architect-verifications.md`](architect-verifications.md).

Access date for everything: 2026-11-14. Researcher: agent (Sonnet 5.5). Labels: FACT (primary source consulted), INFERENCE (explicit reasoning), OPINION, NOT VERIFIED (training knowledge, not confirmed in the source this session; treat as a hypothesis).

Source convention: `gh api repos/<o>/<r>` and `gh release list -R <o>/<r>` (GitHub API, accessed 2026-11-14) for version/date/last commit (`pushed_at` field)/license/stars. "Last commit" below = the repo's `pushed_at` (approximation: it may reflect a push on any branch).

## Index

- Findings that most change the architecture (summary)
- Part A: Go backend (A1 MCP and spec; A2 CLI/HTTP/Inertia; A3 data and queues; A4 upload/S3; A5 contracts (OpenAPI/JSON Schema/Protobuf); A6 real-time and observability; A7 FFmpeg, ASR, release, sandbox)
- Part B: Desktop (B1 frameworks; B2 system webview vs Chromium; B3 FFmpeg sidecar; B4 signing; B5 Rust renderer; B6 IPC)
- Part C: Ready-made editing engines
- Part D: Questions with evidence
- Gaps

## Findings that most change the architecture

1. FACT: the current MCP spec is `2026-07-28` and it is STATELESS: no `initialize`, no `Mcp-Session-Id`, no SSE resumability; tasks became an official extension; elicitation/sampling become "multi round-trip requests"; Roots/Sampling/Logging are deprecated. Source: <https://modelcontextprotocol.io/specification/2026-07-28/changelog> (accessed 2026-11-14). Implication: the editor's MCP server must be stateless, with explicit handles (project/timeline id) in the tool arguments; a long render is the `io.modelcontextprotocol/tasks` extension (polling `tasks/get`), not a session.
2. FACT: Wails v3.0.0 became STABLE on 2026-10-26 (v3.1.0 on 2026-11-01), with "server mode" (the same app as an HTTP server without a window), a built-in updater, and bindings via static analysis. Mobile is experimental. Source: <https://github.com/wailsapp/wails/releases/tag/v3.0.0>. The data in your premise ("v3 alpha") is out of date.
3. FACT: Tauri v3 is in alpha (alpha.6, 2026-11-06) and there is a `tauri-runtime-cef` crate release v3.0.0-alpha.7 (2026-11-06); `tauri-apps/cef-rs` is active (CEF 156). In other words, Tauri with embedded Chromium exists, but it is alpha. Sources: `gh release list -R tauri-apps/tauri`; <https://github.com/tauri-apps/cef-rs>. Tauri v2 stable is 2.12.5 (2026-10-21).
4. FACT: River (transactional queue) has a `riversqlite` driver in the repo, so you can self-host WITHOUT Postgres/Redis using SQLite. Source: `gh api repos/riverqueue/river/contents/riverdriver` (lists `riversqlite`) and the README ("Postgres or SQLite"). Maturity of the SQLite driver: NOT VERIFIED (see Gaps).
5. FACT: MLT (engine of Shotcut/Kdenlive) is active (v7.42.0, 2026-10-03), LGPL-2.1 in the core, has SWIG bindings (python, nodejs, java, csharp, lua, perl, php, ruby, tcl) and does NOT have Go; modules `plusgpl`, `movit`, `qt` etc. exist separately. Usable as a `melt` sidecar binary (INFERENCE).
6. FACT (partial): system webviews for a video editor: WebView2 = Chromium; WebGPU is in Safari 26 (macOS/iOS 26, GA September 2025); WebKitGTK: the 2.50/2.52 notes do not mention WebCodecs/WebGPU (see B2). The client-side rendering decision (WebCodecs) is the number 1 risk for desktop with Wails/Tauri-wry.
7. FACT: gonertia v3.0.0 (2026-04-11) declares "Inertia 3.0 compatibility" in the README; compatible with `@inertiajs/react` 3.x. The repo license appears as NOASSERTION in the API (check the LICENSE file before adopting).

---

## Part A: Go backend

### A1. MCP: SDKs and specification

Candidate table

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source + date |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| modelcontextprotocol/go-sdk | Official MCP SDK in Go | v1.8.0 (2026-09-14) | 2026-11-13 | NOASSERTION in the API (repo README/LICENSE to check; history: MIT→Apache-2.0 transition; NOT VERIFIED) | 5.3k stars, 82 open issues, v1.x (stable semver), negotiates up to `2026-07-28` | `MCPGODEBUG` flags removed in 1.9.0; license to be confirmed | ADOPT | <https://github.com/modelcontextprotocol/go-sdk> and release v1.8.0, 2026-11-14 |
| mark3labs/mcp-go | Community SDK | v1.5.0 (2026-10-30) | 2026-11-13 | MIT | 9.2k stars, 54 issues | not the official one; alignment with spec 2026-07-28 NOT VERIFIED | EVALUATE (only if the official one lacks something) | <https://github.com/mark3labs/mcp-go>, 2026-11-14 |

FACT (release notes v1.8.0): supports `2026-07-28` as the newest negotiated version; added `ServerOptions.SetCacheable` (ttlMs/cacheScope in `server/discover`, list and `resources/read`), limits against exhaustion (JSON nesting 1000, `StdioTransport.MaxLineLength`), validated OAuth discovery, `ServerSession.NotifyElicitationComplete`. Source: <https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0> (2026-11-14). The release notes do NOT describe "tasks"; task support in the Go SDK: NOT VERIFIED.

Support for transports (stdio, Streamable HTTP), resources, images in tool results, outputSchema/structuredContent, elicitation, and progress in go-sdk: NOT VERIFIED in the source this session (the spec requires them; the official SDK historically implements them). Check on pkg.go.dev before closing. See Gaps.

MCP spec: current `2026-07-28`; previous `2025-11-25`; there is a draft with minor changes (OAuth Client ID Metadata Document -03; `_meta` restored). Sources: <https://modelcontextprotocol.io/specification/versioning> , <https://modelcontextprotocol.io/specification/2026-07-28/changelog> , <https://modelcontextprotocol.io/specification/draft/changelog> (2026-11-14).

What changed on 2026-07-28 (FACT, same source):

- No protocol sessions or `Mcp-Session-Id`; no `initialize` handshake; each request carries `protocolVersion` and `clientCapabilities` in `_meta`; new RPC `server/discover`.
- `subscriptions/listen` replaces the GET endpoint and `resources/subscribe`; no SSE resumability (`Last-Event-ID` removed): a broken stream loses the in-flight request.
- Tasks left the core for the `io.modelcontextprotocol/tasks` extension (polling `tasks/get`, `tasks/update`; no `tasks/list`).
- Multi Round-Trip Requests (`InputRequiredResult`) replace server→client requests (elicitation/sampling/roots).
- `ttlMs`/`cacheScope` required in listings and `resources/read`; `inputSchema`/`outputSchema` accept any JSON Schema 2020-12; `structuredContent` accepts any JSON; `traceparent` propagation via `_meta`.
- Auth: `iss` (RFC 9207) validated; Dynamic Client Registration deprecated in favor of Client ID Metadata Documents; Roots/Sampling/Logging deprecated; HTTP+SSE deprecated.
- Tool annotations, resource links and structuredContent came in earlier revisions (2025-06/2025-11, NOT VERIFIED this session) and remain in the spec.

INFERENCE for the editor: model the MCP tools as a thin adapter over the command core; render/export/transcription = tasks extension (or a tool that returns a job id plus a polling tool, compatible with clients that do not yet speak tasks); frame previews = images in tool results or resource links. Older MCP clients speak `2025-11-25` (handshake): the SDK negotiates both (FACT: "Clients and servers MAY support multiple versions").

### A2. CLI, HTTP, Inertia

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source + date |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| spf13/cobra | CLI | v1.10.2 (2025-12-04) | 2026-07-11 | Apache-2.0 | 44.8k stars | slow release cadence, but stable | ADOPT | <https://github.com/spf13/cobra>, 2026-11-14 |
| urfave/cli | Alternative CLI | not checked this session | 2026-11-10 | MIT | 24.3k stars | v3 vs v2 (NOT VERIFIED) | EVALUATE | <https://github.com/urfave/cli>, 2026-11-14 |
| romsar/gonertia | Inertia adapter for Go | v3.0.0 (2026-04-11) | 2026-04-11 | NOASSERTION in the API (check LICENSE) | 268 stars, 4 issues, zero deps | single-maintainer project; most recent commit 7 months ago | EVALUATE (writing our own adapter is small: `X-Inertia` header, page JSON, version) | <https://github.com/romsar/gonertia>, 2026-11-14 |

FACT: the gonertia README lists "Inertia 3.0 compatibility" and the import `github.com/romsar/gonertia/v3`. INFERENCE: compatible with the existing app's `@inertiajs/react` 3.x; validate with an integration test (partial reloads, deferred props, SSR if used). The vukem framework itself (the author's) may already have an Inertia layer: check first.

### A3. Data, migrations, queues

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source + date |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| jackc/pgx | Postgres driver | v5.11.0 (2026-09-07) | 2026-11-11 | MIT | 14.4k stars | Postgres only | ADOPT (if Postgres) | gh, 2026-11-14 |
| sqlc-dev/sqlc | SQL→typed Go | v1.31.1 (2026-04-22) | 2026-11-14 | MIT | 18.5k stars, 729 issues | low release cadence | ADOPT | gh, 2026-11-14 |
| pressly/goose | migrations | v3.30.1 (2026-11-12) | 2026-11-12 | NOASSERTION in the API (MIT history; check) | 11.7k stars | n/a | ADOPT | gh, 2026-11-14 |
| ariga/atlas | declarative migrations | not checked | 2026-11-05 | Apache-2.0 (core; cloud/paid features in part, NOT VERIFIED) | 8.8k stars | features tied to an account | EVALUATE | gh, 2026-11-14 |
| golang-migrate/migrate | migrations | not checked | 2026-11-13 | NOASSERTION in the API | 19k stars | n/a | EVALUATE | gh, 2026-11-14 |
| modernc.org/sqlite | Pure Go SQLite (no cgo) | not checked | 2026-11-13 (GitHub mirror; the real source is gitlab.com/cznic/sqlite) | BSD-3-Clause | used by many (NOT VERIFIED count) | slower than cgo in some cases (NOT VERIFIED); trivial cross-compile | ADOPT for self-host/desktop | gh mirror, 2026-11-14 |
| mattn/go-sqlite3 | SQLite via cgo | not checked | 2026-11-11 | MIT | 9.3k stars | cgo complicates cross-compile and the CI matrix | AVOID (unless a benchmark requires it) | gh, 2026-11-14 |
| riverqueue/river | transactional queue | v0.51.2 (2026-11-09) | 2026-11-14 | MPL-2.0 | 5.9k stars; pgxv5, databasesql and **sqlite** drivers | pre-1.0; MPL-2.0 (per-file copyleft, OK for use as a library); SQLite driver without its own validation | ADOPT if Postgres; EVALUATE with SQLite | gh contents riverdriver + README, 2026-11-14 |
| hibiken/asynq | queue on Redis | v0.26.0 (2026-02-03) | 2026-11-11 | MIT | 13.8k stars | requires Redis (violates simple self-host) | AVOID | gh, 2026-11-14 |
| maragudk/goqite | queue on SQLite (and Postgres since v0.4.0) | v0.4.0 | 2026-08-31 | MIT | 542 stars | small, 1 maintainer | EVALUATE (minimal option) | gh, 2026-11-14 |
| mikestefanello/backlite | SQLite queue | not checked | 2026-10-06 | MIT | 157 stars | very small | AVOID (prefer our own jobs table) | gh, 2026-11-14 |

Answer to the question "which queue works in self-host WITHOUT Postgres/Redis?": FACT: River (`riversqlite` driver) and goqite work on SQLite. OPINION: for the editor, render jobs are few and long; a `jobs` table in SQLite with claim via `UPDATE ... RETURNING` and a worker in a goroutine covers the case (INFERENCE: River's transactional rigor only pays off if Postgres is already there). Choosing a single storage abstraction (SQLite for self-host/desktop; Postgres optional for SaaS) via sqlc with two dialects doubles the queries. OPINION: start with SQLite only, if the hosted mode also fits.

### A4. Upload and S3

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source + date |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| tus/tusd | resumable tus server (Go) | v2.10.1 (2026-09-16) | 2026-11-05 | MIT | 3.9k stars | extra concept (tus protocol) | EVALUATE (only if browser resumable upload is a requirement; client tus-js-client) | gh, 2026-11-14 |
| aws-sdk-go-v2 | S3 client | not checked | 2026-11-13 | Apache-2.0 | 3.7k stars; official | large | ADOPT if AWS S3 | gh, 2026-11-14 |
| minio/minio-go | S3 client | not checked | 2026-11-12 | Apache-2.0 | 3k stars | not checked | EVALUATE (smaller; general S3-compatible) | gh, 2026-11-14 |

INFERENCE/OPINION: hosted web: multipart direct to S3 via pre-signed URLs (the server never touches the bytes) is the simplest; self-host without S3: chunked PUT with an offset to local disk (own contract of ~100 lines) or an embedded tusd. Not verified: the status of MinIO as a project (the organization changed the license/maintenance policy of the community edition in 2025; NOT VERIFIED, check before depending on the MinIO server).

### A5. Contracts: OpenAPI, JSON Schema, Protobuf

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source + date |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| danielgtaylor/huma | OpenAPI 3.1 + JSON Schema from Go structs | v2.40.0 (2026-10-27) | 2026-11-12 | MIT | 4.5k stars | opinionated framework (own router/adapters) | EVALUATE (good if you want the spec generated from the code) | gh, 2026-11-14 |
| oapi-codegen | generates Go from OpenAPI | not checked | 2026-11-11 | Apache-2.0 | 8.7k stars | spec-first | EVALUATE | gh, 2026-11-14 |
| ogen-go/ogen | generates Go client/server from OpenAPI | not checked | 2026-11-13 | Apache-2.0 | 2.2k stars | not checked | EVALUATE | gh, 2026-11-14 |
| swaggo/swag | OpenAPI 2 from comments | not checked | 2026-09-30 | MIT | 13k stars | limited OpenAPI 3 (NOT VERIFIED) | AVOID | gh, 2026-11-14 |
| santhosh-tekuri/jsonschema | JSON Schema validator | v6.0.3 (2026-08-06) | 2026-11-12 | Apache-2.0 | 1.3k stars | validation only | ADOPT (validate command args) | gh, 2026-11-14 |
| google/jsonschema-go | JSON Schema (used by the MCP go-sdk) | v0.4.3 (2026-04-27) | 2026-05-22 | MIT | 491 stars; pre-1.0 | pre-1.0 | ADOPT via transitivity (MCP SDK) | gh, 2026-11-14 |
| invopop/jsonschema | generates JSON Schema from structs | not checked | 2026-04-23 | MIT | 964 stars | not checked | EVALUATE | gh, 2026-11-14 |
| bufbuild/buf + connectrpc/connect-go | Protobuf toolchain / RPC | not checked | 2026-11-13 | Apache-2.0 | 11.5k / 4.2k stars | toolchain weight; MCP and agents speak JSON | AVOID in the MVP | gh, 2026-11-14 |

OPINION, well-founded: the command core has a source of truth in JSON Schema (one schema per command), which feeds (a) the `inputSchema` of the MCP tools (FACT: spec 2026-07-28 accepts any JSON Schema 2020-12), (b) CLI/API validation, (c) generated TS types. Protobuf is only justified by a requirement for binary streaming/gRPC between our own processes (a separate Rust renderer); with a single renderer, JSON is enough. For the HTTP docs, huma or OpenAPI derived from the same schemas.

### A6. Real-time and observability

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source + date |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| coder/websocket | WebSocket | v1.8.15 (2026-06-15) | 2026-11-14 | ISC | 5.6k stars, active | n/a | ADOPT (if WS) | gh, 2026-11-14 |
| gorilla/websocket | WebSocket | not checked | 2025-03-19 | BSD-2-Clause | 24.9k stars; no push for 20 months | weak maintenance | AVOID | gh, 2026-11-14 |
| SSE | job progress | (stdlib `net/http`) | n/a | n/a | n/a | one-way | ADOPT for progress (simpler than WS) | OPINION |
| opentelemetry-go | tracing/metrics | v1.47.0 (2026-10-02) | 2026-11-14 | Apache-2.0 | 6.6k stars | n/a | ADOPT optional | gh, 2026-11-14 |
| getsentry/sentry-go | errors | v0.50.0 (2026-10-08); v1.0.0-alpha.1 (2026-10-19, pre-release) | 2026-11-14 | MIT | 1.1k stars | v1 in alpha (do not use) | ADOPT v0.50.x | gh, 2026-11-14 |

### A7. FFmpeg, ASR, release, sandbox

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source + date |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| FFmpeg (upstream) | media engine | n8.1.3 (tag); n8.2-dev | not checked | LGPL-2.1+ (GPL with libx264 etc.) | reference | parsing CVEs (below) | ADOPT via os/exec | `gh api repos/FFmpeg/FFmpeg/git/matching-refs/tags/n8`, 2026-11-14 |
| os/exec stdlib | invoke ffmpeg/ffprobe | stdlib | n/a | BSD | n/a | build the args manually | ADOPT | OPINION |
| u2takey/ffmpeg-go | fluent wrapper | not checked | 2024-05-24 | Apache-2.0 | 2.4k stars; no push for 18 months | abandoned | AVOID | gh, 2026-11-14 |
| asticode/go-astiav | cgo binding for libav* | v0.43.0 | 2026-10-29 | MIT | 750 stars, active | cgo; libav ABI | AVOID in the MVP (EVALUATE for thumbnails/in-process decoding) | gh, 2026-11-14 |
| ggml-org/whisper.cpp | local ASR | v1.9.5 (2026-10-06) | 2026-11-13 | MIT | 55k stars; `bindings/go` directory exists | large models are slow on CPU | ADOPT as the local default | gh + contents bindings, 2026-11-14 |
| SYSTRAN/faster-whisper | Python ASR | not checked | 2026-11-12 | MIT | 26.7k stars | Python runtime | EVALUATE (GPU server mode) | gh, 2026-11-14 |
| m-bain/whisperX | ASR + word alignment + diarization | not checked | 2026-11-12 | BSD-2-Clause | 25k stars | Python; heavy deps | EVALUATE | gh, 2026-11-14 |
| goreleaser | releases | v2.20.2 (2026-11-12) | 2026-11-14 | MIT | 16k stars | n/a | ADOPT | gh, 2026-11-14 |

Transcription prices (official pages, 2026-11-14):

- FACT Deepgram pay-as-you-go pre-recorded: Nova-3 mono $0.0043/min; multilingual $0.0052/min; Whisper Large $0.0048/min. <https://deepgram.com/pricing>
- FACT AssemblyAI: Universal-3.5 Pro $0.21/h; Universal-2 $0.15/h; diarization +$0.02/h. <https://www.assemblyai.com/pricing>
- FACT Groq: whisper-large-v3-turbo $0.04/h; whisper-large-v3 $0.111/h; limits 25 MB (free) / 100 MB (dev); `timestamp_granularities` word/segment; minimum billing 10 s. <https://console.groq.com/docs/speech-to-text>
- FACT OpenAI: the page lists transcription "from $0.0045/min (gpt-transcribe)" up to $0.034/min; details per model (whisper-1, gpt-4o-transcribe) could NOT be extracted. <https://developers.openai.com/api/docs/pricing> (note: the page summarized by the fetch cites "gpt-6-*" models; confirm on the raw page).

whisper.cpp: word-level timestamps and CPU performance by model size: NOT VERIFIED in this session (knowledge: it supports `--max-len 1 --split-on-word`/token timestamps; the quality of word timestamps is lower than WhisperX with wav2vec2 alignment). Current benchmarks require measurement.

Rational time/timecode in Go: no library evaluated; OPINION: use an own `int64` numerator/denominator (type `RationalTime{Value, Rate}` as in OTIO), with `math/big` only in rare cases; avoid float for clip positions.

ffprobe JSON: `ffprobe -v quiet -print_format json -show_format -show_streams` (NOT VERIFIED in this session; stable for years).

FFmpeg sandbox and risks:

- FACT (via aggregators, not an official advisory; the format "CVE-2026-107697" is unusual, verify in the NVD): FFmpeg before 8.1.3 would have a bypass in the HLS demuxer (`parse_playlist()` calling `io_open` directly, bypassing the protocol whitelist and the extension check), allowing reading of non-media local files or use of restricted protocols. Suggested mitigation: `-protocol_whitelist` and `-allowed_extensions`. Sources: <https://www.wiz.io/vulnerability-database/cve/cve-2026-107697> , <https://access.redhat.com/security/cve/CVE-2026-107697> (accessed 2026-11-14). Historical class: CVE-2016-1897/1898 (HLS reads a local file and exfiltrates it via HTTP), CVE-2023-6605 (SSRF in the DASH demuxer). Source: web search 2026-11-14.
- OPINION/INFERENCE (defense pattern): run FFmpeg in a separate process with `-protocol_whitelist file,pipe` (or `file` only), never accept user URLs/playlists/concat, generate the file list yourself from internal storage paths, run `ffprobe` first with limits, a timeout and CPU/memory limits (cgroups v2 / `ulimit` / Job Objects on Windows), a user with no network (empty netns), and a read-only FS. Tools: bubblewrap, nsjail, gVisor, seccomp: versions/state NOT VERIFIED in this session. Windows/macOS desktop: weak sandbox; for the user's local mode the risk is the user's own (their files), but projects imported from third parties (OTIO/XML with paths) must be sanitized.

---

## Part B: Desktop

### B1. Frameworks

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source + date |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Wails v3 | Go desktop shell + system webview | v3.0.0 stable (2026-10-26); v3.1.0 (2026-11-01); nightlies v3.2.0 | 2026-11-13 | MIT | 37k stars (repo shared v2/v3); ~4 years of alpha/beta; Go >= 1.26 | experimental mobile; advisory GHSA-2p5f-2gx8-mh8m "partially fixed"; system webview; stable for 3 weeks | EVALUATE strongly (if the system webview is enough) | <https://github.com/wailsapp/wails/releases/tag/v3.0.0> , 2026-11-14 |
| Wails v2 | same, old API | v2.17.1 (2026-11-01) | same | MIT | receives critical fixes | legacy architecture | AVOID for a new project | gh, 2026-11-14 |
| Tauri v2 | Rust shell + system webview (wry/tao) | 2.12.5 (2026-10-21) | 2026-11-13 | Apache-2.0 (+MIT, dual license NOT VERIFIED) | 112.9k stars; sidecar, updater, mobile (details NOT VERIFIED) | requires Rust; system webview | EVALUATE | gh, 2026-11-14 |
| Tauri v3 + CEF | Tauri with embedded Chromium | v3.0.0-alpha.6 (2026-11-06); `tauri-runtime-cef` v3.0.0-alpha.7 (2026-11-06) | 2026-11-13 | Apache-2.0 | alpha; cef-rs (492 stars) on CEF 156 | alpha; larger bundle; branch `feat/cef` | EVALUATE (follow it; do not depend on it today) | `gh release list -R tauri-apps/tauri`, <https://github.com/tauri-apps/cef-rs> , 2026-11-14 |
| Electron | Node shell + embedded Chromium | v45.3.2 (2026-11-13); lines 43.x/44.x maintained | 2026-11-14 | MIT | 124k stars | binary ~100+ MB (current size NOT VERIFIED); Node/JS on the desktop | EVALUATE strongly (guaranteed Chromium) | gh, 2026-11-14 |
| Electrobun | TS/Bun shell, CEF option | v2.0.7-beta.21 (2026-11-14) | 2026-11-14 | MIT | 13k stars | still beta; Bun/Zig ecosystem; README cites Go/Rust/Odin as native-side languages | EVALUATE (watch) | <https://github.com/blackboardsh/electrobun> , 2026-11-14 |
| Neutralino | lightweight shell, system webview | not checked | 2026-11-05 | NOASSERTION | 8.7k stars | same webview limitation | AVOID | gh, 2026-11-14 |
| CEF | Chromium Embedded Framework | not checked | 2026-11-12 | NOASSERTION (BSD-style) | 4.9k stars; base of cef-rs | heavy manual integration in Go (cgo) | AVOID directly | gh, 2026-11-14 |

FACT Wails v3: Go services with methods callable from the front end, bindings generated by static analysis, typed events, system tray/menus/dialogs, built-in updater, signing/installers, Taskfile, an MCP service for AI tools, and HTTP server mode (the same app without a window). Platforms: Windows amd64/arm64, macOS Intel/Apple Silicon, Linux amd64/arm64. Linux uses GTK4 + WebKitGTK 6.0 by default (GTK3 up to v3.1). Source: release v3.0.0 (accessed 2026-11-14). Go 1.26+ required; vukem uses Go 1.27.4 in Docker (repo commit), compatible.

go-app and the WebView2 wrapper (`wailsapp/go-webview2`, 53 stars): discarded for scope (INFERENCE).

### B2. The decisive problem: system webview

- FACT: Windows: WebView2 is Chromium (Edge), so WebCodecs/WebGPU behave as in Chrome (INFERENCE from "Chromium"; the evergreen version depends on the installed runtime).
- FACT: macOS: WebGPU is enabled by default in Safari 26 (macOS/iOS/iPadOS/visionOS), GA in September 2025. Source: search "Safari 26 WebGPU" (aggregators; the primary source would be <https://webkit.org/blog/>, not opened). WebCodecs: Safari has supported `VideoDecoder/VideoEncoder` since 16.4 (NOT VERIFIED in this session), and the Safari 26.1 notes cite a WebCodecs fix and the WebGPU renderer. Whether WKWebView (embedded) exposes the same features as the Safari app: NOT VERIFIED.
- FACT (negative): the WebKitGTK 2.52 notes (2026-03-18) do not mention WebCodecs/WebGPU/OPFS: <https://webkitgtk.org/2026/03/18/webkitgtk-2.52-highlights.html> . The Igalia blog (Aug 2026) lists WebCodecs among GStreamer work, without confirming `VideoEncoder`. INFERENCE: Linux/WebKitGTK is the weakest link for an editor that depends on WebCodecs/WebGPU on the client.
- INFERENCE: the common floor across WebView2, WKWebView and WebKitGTK is weak for client-side encoding. Two coherent options, which are "simpler": (1) client-side preview with `<video>` + canvas 2D/WebGL, with render/export ALWAYS via FFmpeg (a Go process), without requiring WebCodecs; (2) Electron (or Tauri v3 CEF once it stabilizes) for guaranteed Chromium.
- Why editors use Electron/Chromium: INFERENCE (consistency of WebCodecs/WebGPU/OPFS across OSes). Test target: run a feature-detection matrix (WebCodecs, WebGPU, OPFS, SharedArrayBuffer, `requestVideoFrameCallback`) in WebView2/WKWebView/WebKitGTK 6.0 before deciding. See Gaps.

### B3. FFmpeg as a sidecar

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source + date |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| BtbN/FFmpeg-Builds | Win/Linux builds (GPL and LGPL, shared/static) | daily auto-build (2026-11-14) | 2026-11-14 | MIT (scripts; binaries per variant) | 12k stars | "latest" binaries are master snapshots; use release tags/branch | ADOPT (Windows/Linux) | `gh release list -R BtbN/FFmpeg-Builds`, 2026-11-14 |
| jellyfin/jellyfin-ffmpeg | FFmpeg with patches (HW accel) | not checked | 2026-11-09 | NOASSERTION | 894 stars | own patches | EVALUATE | gh, 2026-11-14 |
| gyan.dev, evermeet.cx, ffbinaries | Win / macOS / multi builds | not checked | not checked | not checked | not checked | NOT VERIFIED (not opened this session) | EVALUATE | no source |

OPINION: license: distributing an LGPL build (without libx264/x265, using platform encoders: VideoToolbox, NVENC/QSV/AMF, and libsvtav1/libvpx, which are BSD) avoids GPL obligations; a GPL build with libx264 requires distributing the ffmpeg source and attention to the copyleft of the whole. The MIT app as a separate process (a sidecar) is the common practice, but it is a legal interpretation: this is NOT legal advice. Typical size 60-150 MB per platform (NOT VERIFIED).

### B4. Signing, notarization, updater

- FACT: Azure Artifact Signing (formerly Trusted Signing): Basic $9.99/month up to 5,000 signatures; Premium $99.99/month up to 100k; +$0.005 per extra signature; GA in the US, Canada and Europe (January 2026 report). Source: Azure pages (the values came from an en-IN page and the report https://www.devclass.com/security/2026/01/14/code-signing-windows-apps-may-be-easier-and-more-secure-with-new-azure-artifact-service/4079554 , accessed 2026-11-14; official pages for other locales showed "$-"). Eligibility of an individual developer outside the US/Canada/Europe (e.g., Brazil): NOT VERIFIED, so a possible block; alternative: an OV/EV certificate from a CA (cost NOT VERIFIED).
- macOS: Developer ID + notarization require the Apple Developer Program, US$99/year (NOT VERIFIED in this session, historical value).
- Updater: Wails v3 has a built-in updater (FACT); Tauri has an updater plugin (NOT VERIFIED); Electron: `electron-updater`/Squirrel (NOT VERIFIED). In MIT OSS, without signing, users see SmartScreen/Gatekeeper warnings: a real cost of desktop.
- CI matrix: goreleaser v2.20.2 (FACT) covers Go; Wails v3 uses Taskfile and has packaging/signing; macOS requires a macOS runner (cgo/webview), and Linux requires the GTK4/WebKitGTK 6.0 libs (FACT).

### B5. Native renderer in Rust

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source + date |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| gfx-rs/wgpu | GPU | not checked | 2026-11-14 | Apache-2.0 | 18.5k stars | API still evolving | EVALUATE | gh, 2026-11-14 |
| linebender/vello | GPU 2D renderer | not checked | 2026-11-13 | Apache-2.0 | 4.5k stars | pre-1.0 (NOT VERIFIED) | EVALUATE | gh, 2026-11-14 |
| pop-os/cosmic-text | text layout | not checked | 2026-10-27 | Apache-2.0 | 2.2k stars | not checked | EVALUATE | gh, 2026-11-14 |
| grovesNL/glyphon | text in wgpu | not checked | 2026-07-09 | no license in the API (check) | 776 stars | license not declared by the API | AVOID until checked | gh, 2026-11-14 |
| zmwangx/rust-ffmpeg-sys | FFmpeg binding | not checked | 2026-11-11 | no license declared | 216 stars | not checked | AVOID | gh, 2026-11-14 |

OPINION (cost of a SECOND renderer): if the UI preview uses Canvas/WebGL/HTML and the export uses FFmpeg, there are already two semantics (text rasterization, blend, easing, fonts, color management); a third path in Rust/wgpu triples the risk of "preview ≠ export" divergence. Rule: a single composition model (timeline JSON) and ONE reference renderer; the others are declared approximations. Rust is only justified by a measured requirement (e.g., a real-time 4K preview impossible via `<video>`).

### B6. IPC

OPINION: local HTTP (loopback) + JSON for the same contract as the web server is the lowest common denominator (the Go core already speaks HTTP; MCP Streamable HTTP too). stdio JSON-RPC serves the CLI/MCP stdio. gRPC only with a separate renderer and binary streaming; framework bindings (Wails v3 Go services) are ergonomic but couple the desktop UI to the framework (the web UI does not have them), so they duplicate the contract: prefer HTTP for what the web UI also uses.

---

## Part C: Ready-made engines

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source + date |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| MLT Framework | multimedia engine (melt, XML) | v7.42.0 (2026-10-03) | 2026-11-14 | LGPL-2.1 core; the repo contains GPL/GPLv3 (modules `plusgpl`, `movit`, `qt`, `xml` etc., separate) | 1.9k stars; base of Shotcut/Kdenlive (NOT VERIFIED this session) | no Go binding (SWIG: python, nodejs, java, csharp, lua, perl, php, ruby, tcl); native dependencies (Qt for titles); GPL in modules | EVALUATE as a `melt` sidecar renderer (compare with the generated FFmpeg) | gh, README, `src/swig`, `src/modules`, 2026-11-14 |
| GStreamer Editing Services (GES) | editing on GStreamer | not checked | GitHub mirror `GStreamer/gst-editing-services` stalled since 2018 (the real source is GitLab freedesktop; NOT VERIFIED) | LGPL (NOT VERIFIED) | not checked | complex GStreamer plugin distribution | AVOID | gh, 2026-11-14 |
| OpenTimelineIO | timeline interchange | v0.18.1 (2025-11-09, pre-release in the API) | 2026-11-10 | Apache-2.0 | 2.1k stars (ASWF); `RationalTime` model | no Go binding (NOT VERIFIED; C++/Python/C ABI); no stable release after 0.18 in the API | ADOPT as an EXPORT/IMPORT format (adapters), not as native | gh, 2026-11-14 |
| libopenshot | OpenShot engine | not checked | 2026-11-14 | LGPL-3.0 | 1.6k stars | C++ + Qt | AVOID | gh, 2026-11-14 |
| Olive | native editor | not checked | 2024-12-05 | GPL-3.0 | 9.2k stars; no push for 11 months | GPL; stalled | AVOID | gh, 2026-11-14 |
| AviSynth / VapourSynth | video scripts | not checked | not checked | not checked | not checked | NOT VERIFIED | AVOID | no source |
| Generated FFmpeg `filter_complex` | timeline render | n8.1.3 | not checked | LGPL/GPL | de facto standard | limits below | ADOPT as the export renderer | see A7 |
| Headless Chrome (Remotion) | render by frame capture | Remotion v4.0.543 (2026-11-14) | 2026-11-14 | NOASSERTION (own "source-available" license; check LICENSE; companies above a certain size pay) | 73k stars | non-OSI license; CPU cost; determinism depends on per-frame seek | EVALUATE only for templates/motion | gh, 2026-11-14 |
| Revideo | programmatic video (Motion Canvas fork) | not checked | 2026-07-15 | MIT | 4.2k stars, last push 4 months ago | little activity | AVOID | gh, 2026-11-14 |

`filter_complex` limits (INFERENCE/NOT VERIFIED, training knowledge): graphs with hundreds of inputs are slow to build and consume memory; `xfade` covers 2-input transitions; keyframes via expressions (`enable`, `x='...'`, `overlay` with `t`) and `sendcmd` limit complex animation; captions via `ass`/libass (good) and `drawtext` (limited fonts/shaping); mitigation: render in segments and concatenate, or pre-render complex layers (animated text) as PNG/WebM with alpha via another path. This forces accepting that animated text/rich effects are not pure FFmpeg: a decision point for a second path (MLT or headless Chrome for layers).

Render via headless Chrome as the single path: INFERENCE: deterministic if you control the clock (per-frame seek), but slow (per-frame capture, tens to a few hundred fps at best, NOT VERIFIED) and heavy on server CPU/RAM; good fidelity with the web preview (same engine). Advantage: removes the preview≠export duplication. Disadvantage: operational cost and the Remotion license; Puppeteer/Playwright with `beginFrame`/virtual time is the base mechanism (NOT VERIFIED).

---

## Part D: Questions

### D1. Command core shared between Go (server) and web UI

Approaches (summary by INFERENCE/OPINION; sizes and "who does it" were NOT verified in a source this session):

1. Core only on the server; optimistic UI. The server is the only implementation; the UI applies a predictable local "patch" (e.g., a simple reducer for move/trim) and reconciles with the response. Pros: zero duplication of domain logic; fits MCP/CLI/API (all call the same command). Cons: latency for drag operations (mitigated by optimistic UI), and two small implementations of each command's "effect" (authoritative server, optimistic client) that need a parity test.
2. Go → WASM (standard compiler): binary typically several MB (≥2 MB runtime only; NOT VERIFIED), GC and goroutines in single-thread wasm, JS interop via `syscall/js` (poor ergonomics); TinyGo produces much smaller binaries but with limited stdlib/reflection (encoding/json and reflect partial). Pros: one code for validating/applying commands on client and server. Cons: weight and restrictions, debugging friction.
3. Core in TypeScript (Bun/Node embedded): the same logic in the browser and on the server/desktop, but the server stops being Go for the domain; Go remains for HTTP, jobs, FFmpeg, storage. This duplicates the language boundary and goes against vukem being in Go.
4. Core in Rust → WASM + native: a single shared fast code, but it is "complexity that needs a concrete requirement" (a violation of the principle; Rust toolchain cost in CI).

OPINION: approach 1 with a single contract in JSON Schema (one schema per command) + a conformance test suite (golden: initial state + command → final state) run against the Go server and against the client's TS reducer. Parity is verified by a test, not by sharing code. Revisit WASM only if the reducer duplication grows. Examples of "who does it" (Figma, tldraw, Excalidraw, CapCut web) NOT VERIFIED: I do not cite them.

### D2. Desktop: "same Go binary as local server + webview on localhost"

FACT (indirect): Wails v3 offers "server mode": the same app and services run as an HTTP server without a native window (release v3.0.0), which shows the server+UI-in-one-binary pattern is supported by the framework. Source: <https://github.com/wailsapp/wails/releases/tag/v3.0.0> (2026-11-14).
INFERENCE/NOT VERIFIED (training knowledge) about broad use of the pattern (e.g., Jupyter, Syncthing, tools with a local UI, Electron with an internal server) and its risks:

- Port: use an ephemeral port (`127.0.0.1:0`), chosen by the process and passed to the webview; never `0.0.0.0`.
- Other local processes and other users on the machine can reach loopback: require a random token per session (cookie/header) generated at startup and passed to the webview through a channel outside the network (argument/framework IPC), not through the URL in history.
- DNS rebinding/CSRF: validate `Host` (only `127.0.0.1:<port>`/`localhost`), validate `Origin` on mutable requests and WebSocket, no open CORS (`*`).
- The MCP spec also requires equivalent protection on local transports: the Go SDK v1.8.0 removed the options to disable `disablelocalhostprotection`/`enableoriginverification` (FACT: v1.8.0 release notes), which is evidence that the pattern is treated as a real risk.
- Firewall/antivirus may alert on listeners; Windows may ask for permission if the bind is not loopback.

---

## Gaps (what I did NOT verify in the source and needs follow-up)

1. go-sdk: exact list of features (resources, images in tool results, outputSchema, elicitation/progress, support for the tasks extension) and the real license in the LICENSE file (the API returned NOASSERTION; likewise goose, golang-migrate, gonertia).
2. WebCodecs/WebGPU/OPFS feature matrix in WKWebView (embedded, not the Safari app) and WebKitGTK 6.0: only indications; needs empirical testing or an MDN/caniuse page plus WebKit bugs.
3. Maturity and performance of `riversqlite` and goqite under concurrent writes (SQLite WAL, busy timeout).
4. Eligibility of Azure Artifact Signing for a developer in Brazil; current cost of the Apple Developer Program and OV/EV certificates.
5. Current size of Electron, Tauri and LGPL FFmpeg builds; exact license and versions of gyan.dev, evermeet, ffbinaries.
6. OpenAI price per transcription model (the page summarized by the fetch brought model names that I need to reconfirm).
7. Performance and word-level timestamps of whisper.cpp vs WhisperX; current models.
8. Sandbox (bubblewrap/nsjail/gVisor) versions and viability on Windows/macOS; confirm the HLS CVE in the NVD/FFmpeg security pages (the identifier sounded atypical).
9. Tauri `tauri-runtime-cef`: README/production status, bundle size, updater/sidecar support; Tauri v2 sidecar/updater/mobile.
10. Status of MinIO (server) and of unopened wrappers; OTIO Go bindings; GES; exact Remotion license.
11. Real Go→WASM sizes (standard vs TinyGo) and filter_complex limits at scale: measure with a prototype.
