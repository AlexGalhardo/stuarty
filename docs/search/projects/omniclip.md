# Profile: Omniclip (omni-media)

> **Status of this file.** Project profile delivered by a research agent on 2026-11-14 and reviewed by the architect. Its "lessons" are input to the design, not decisions; the decisions are in `../../brainstorming.md` and `../../adrs/`. Code excerpts and diagrams are quoted from the project as found and are not translated.

All sources accessed: 2026-11-14. Convention: FACT = read in the primary source; INFERENCE = my conclusion with reasoning; OPINION = my judgment.

Local clones used (shallow, outside the vukem-video repo): `scratchpad/omniclip/omniclip` (branch `main`, SHA `cbe581a`), `scratchpad/omniclip/omniclip-dev` (branch `dev`, SHA `12c0b91`), `scratchpad/omniclip/omnitool` (branch `main`, SHA `d8a525b`). npm tarballs of `@omnimedia/omniso@0.1.1-0` and `@e280/strata@0.5.7` were downloaded with `npm pack` (without install) to read code that is not on the public GitHub.

## 0. Main finding: there are three "Omniclips"

1. **Omniclip 1.x** (branch `main`, legacy). Monolithic editor in Lit, Pixi 7, ffmpeg.wasm. Last commit on `main`: 2025-08-07 (`1.1.3`). FACT: <https://github.com/omni-media/omniclip/commit/cbe581a4a788d9982ffd4a10025c678f9e8707d6> (accessed 2026-11-14).
2. **omnitool** (sibling repo, `omni-media/omnitool`). It is the extracted engine: timeline as JSON, driver in a worker, Pixi 8, mediabunny. Active (last commit 2026-11-02). FACT: <https://github.com/omni-media/omnitool> (accessed 2026-11-14).
3. **Omniclip 2.0** (branch `dev`, version `2.0.0-4`). Rewrite of the editor on top of omnitool, **with MCP server, AI assistant and timeline patches with review**. Last commit 2026-11-13. FACT: <https://github.com/omni-media/omniclip/blob/dev/package.json> (accessed 2026-11-14). The `main` README already warns "Omniclip 2.0 is in development" and "Omni Tools ... integrating with AI or scripting workflows": <https://github.com/omni-media/omniclip/blob/main/README.md> (accessed 2026-11-14).

Other repos in the org (FACT, `gh repo list omni-media`, accessed 2026-11-14): `kimura` (Pixi transform matrix editor, 0 stars, push 2026-10-13), `webm-hero` (VP8/VP9 decoder in WASM via libvpx, push 2025-04-30). The dependency `@omnimedia/omniso@0.1.1-0` (MIT on npm) points to `github.com/omni-media/omniso`, but the repo **does not resolve** on GitHub (`gh repo view omni-media/omniso` returned "Could not resolve to a Repository"): probably private. It contains Zod schemas for the patch/MCP; I read it from the npm tarball.

The whole profile below separates **1.x (legacy)**, **omnitool** and **2.0 (dev)**, because the answers change between them.

## 1. Stack (versions read from package.json/lockfile)

| Layer | Omniclip 1.1.3 (`main`) | omnitool 1.1.0-146 | Omniclip 2.0.0-4 (`dev`) |
| --- | --- | --- | --- |
| Render 2D/GL | pixi.js ^7.4.2 | pixi.js ^8.18.1 (lock: 8.22.0), pixi-filters ^6.1.5, gl-transitions ^1.74.0 | pixi.js ^8.18.1 |
| Demux/decode | mp4box ^0.5.2, web-demuxer ^1.0.1, ffprobe-wasm, mediainfo.js | **mediabunny ^1.65.0** (MPL-2.0) | mediabunny ^1.65.0, mediainfo.js, ffprobe-wasm |
| Encode/mux | WebCodecs `VideoEncoder` (H.264) + **ffmpeg.wasm ^0.12.9** for audio and mux | WebCodecs via mediabunny; `@mediabunny/aac-encoder` ^1.65.0 | same as omnitool |
| State | @benev/slate ^0.1.0-x.14 (`AppCore`, ZipAction) | n/a (pure JSON document) | @e280/strata ^0.5.x (`Prism`, `Chrono`, `Vault`) |
| UI | lit ^2.6.1, @benev/construct | lit ^3.3.2 | lit ^3.3, react ^19.2.8 (only in the assistant), webawesome |
| Workers/RPC | own workers (decode/encode) | @e280/comrade ^0.1.0, @e280/renraku ^0.5.3 | same |
| Collab | sparrow-rtc ^0.2.3 (WebRTC) | n/a | sparrow-rtc ^0.2.9 |
| Others | gsap, wavesurfer.js, opfs-tools, zip.js, posthog-js | @huggingface/transformers ^4.3.1 (Whisper and background removal), @noble/hashes, @awasm/noble (BLAKE3) | + ai ^7, fastify ^5, ws, zod ^4, @modelcontextprotocol/sdk ^1.32.1, better-auth, @better-auth/stripe |

Sources (FACT, accessed 2026-11-14): <https://github.com/omni-media/omniclip/blob/main/package.json> , <https://github.com/omni-media/omnitool/blob/main/package.json> , <https://github.com/omni-media/omniclip/blob/dev/package.json> . Real lock versions: mediabunny 1.65.0 (MPL-2.0) at <https://github.com/omni-media/omnitool/blob/main/package-lock.json#L3490> .

FACT: `main` does not use `ffmpeg.wasm` only for mux, it also uses it to extract/mix audio (`ffmpeg.exec(["-ss", ...,"-vn", ...mp3])` and `amix`): <https://github.com/omni-media/omniclip/blob/main/s/context/controllers/video-export/helpers/FFmpegHelper/helper.ts#L36-L88> . The ffmpeg core is downloaded from `https://unpkg.com/@ffmpeg/core@0.12.5` at runtime (line 25 of the same file).

## 2. Architecture

### omnitool (the one that matters)

```mermaid
flowchart LR
  JSON[TimelineFile JSON] --> S[Samplers puros: sampleVisual / sampleAudio]
  S -->|Layer[]| C[Compositor Pixi no thread principal]
  S -->|AudioData| M[AudioMix + resample]
  D[Driver] -->|Comrade| W[Worker: decodeVideo / decodeAudio / encode]
  C --> P[Preview: canvas]
  C --> E[Export: VideoFrame]
  M --> E
  E --> W
  W -->|mediabunny mux| OUT[mp4/webm/mov/mkv]
```

- FACT: `Driver.setup` creates a worker thread via `@e280/comrade` and a `Compositor` (Pixi) **on the main thread**: <https://github.com/omni-media/omnitool/blob/main/s/driver/driver.ts#L16-L26> . Decode and encode run in the worker (`DriverSchematic.work.{decodeVideo,decodeAudio,encode}`, passing `WritableStream<VideoFrame>`/`MessagePort` by transfer): <https://github.com/omni-media/omnitool/blob/main/s/driver/fns/schematic.ts#L12-L37> .
- FACT: `Compositor.composite(layers)` draws the layer tree on a Pixi 1920x1080 `renderer` (preference `webgl`, with the comment "webgl and webgl2 causes memory leaks on chrome") and returns a `VideoFrame` from the canvas: <https://github.com/omni-media/omnitool/blob/main/s/driver/parts/compositor.ts#L31-L80> .
- FACT: the timeline never stores absolute position; samplers compute the `Layer[]` for an instant: <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/parts/samplers/visual/parts/sample.ts#L9-L60> .
- INFERENCE: Pixi on the main thread (not OffscreenCanvas in a worker) is a bottleneck for simultaneous UI + render; the current code does not use OffscreenCanvas (grep in `s/` showed nothing), and the 2.0 TODO lists "multithreading" as a future goal: <https://github.com/omni-media/omniclip/blob/dev/TODO.md> (section "compositing engine").

### Omniclip 1.x

FACT: single context `OmniContext` with historical state (`AppCore` from slate, `history_limit: 64`) and non-historical state (`watch.stateTree`), controllers (`compositor`, `timeline`, `video_export`, `media`, `collaboration`) and Lit components: <https://github.com/omni-media/omniclip/blob/main/s/context/context.ts#L120-L175> . The `main` README describes "State, Actions, Controllers, Components/Views".

### Omniclip 2.0

FACT: state in `Strata` (Prism + `Chrono` for undo + `Vault` for persistence), `AssistantController` (patches/AI), `McpController`, `CargoController` (media via `@e280/quay`), central `OmniSession`: <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/context/parts/strata.ts> , <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/context/controllers/cargo.ts> . MCP server in `s/server/mcp/` (stdio) with a WebSocket bridge to the browser tab: <https://github.com/omni-media/omniclip/blob/dev/s/server/mcp/README.md> .

## 3. Timeline data model

### omnitool (current format, versioned JSON, integer/float ms)

FACT, <https://github.com/omni-media/omnitool/blob/main/s/timeline/parts/basics.ts#L11-L18> :

```ts
export type TimelineFile = {
	info: "https://omniclip.app/"
	format: "timeline"
	version: number
	rootId: Id          // 128-bit hex opaco
	items: Item.Any[]   // flat array; the order is NOT the playback order
	audio?: AudioSettings
}
```

FACT, <https://github.com/omni-media/omnitool/blob/main/s/timeline/parts/item.ts#L13-L28> and L88-L163:

```ts
export enum Kind { Sequence, Stack, Video, Audio, Text, Gap, Spatial, Animation, Transition, TextStyle, Filter, Caption, Image, Clip }

type Video = { id; kind: Kind.Video; mediaHash: Hash; start: number; duration: number
               spatialId?; animationIds?: Id[]; filterIds?: Id[] } & {label?; enabled?}
type Text  = { id; kind: Kind.Text; content: string; duration: number
               spatialId?; animationIds?; styleId?; filterIds? }
type Caption = { id; kind: Kind.Caption; transcript: Transcription; itemId?; start; duration
               maxChars?; maxDuration?; maxSilence?; spatialId?; animationIds?; styleId?; filterIds? }
type Transition = { id; kind: Kind.Transition; name: TransitionName; duration: number }
type Sequence = { id; kind: Kind.Sequence; childrenIds: Id[]; spatialId?; filterIds? }
type Stack    = { id; kind: Kind.Stack;    childrenIds: Id[]; spatialId?; filterIds? }
type Spatial  = { id; kind: Kind.Spatial; transform: Transform; crop?: Crop }
type Animation= { id; kind: Kind.Animation; anims: VisualAnimations }
type Filter   = { id; kind: Kind.Filter; type; params? }
type TextStyle= { id; kind: Kind.TextStyle; style: TextStyleOptions /* Pixi */ }
```

FACT, keyframes and time types: <https://github.com/omni-media/omnitool/blob/main/s/timeline/types.ts#L3-L38> :

```ts
export type Interpolation = "linear" | "ease" | "easeIn" | "easeOut" | "bounce" | "catmullRom"
export type Keyframe<Value = number> = [time: number, value: Value]
export type Transform = [position: Vec2, scale: Vec2, rotation: number]
export type TrackTransform = { position: {x: Keyframes; y: Keyframes}; scale: {x; y}; rotation: Keyframes }
export type VisualAnimations = { opacity?: Anim<Keyframes>; transform?: Anim<TrackTransform> }
```

Key points:

- **No tracks.** The composition is a tree: `Sequence` runs children in series, `Stack` runs in parallel (the first child is on top). FACT via skill: "there are no core `tracks`, timeline-position, `end`, or per-item speed fields": <https://github.com/omni-media/omnitool/blob/main/skills/timeline/SKILL.md> (section "Timing and sound").
- **Time:** millisecond `number` with a marked type (`Ms`, `Fps`, `Seconds` via brand type): <https://github.com/omni-media/omnitool/blob/main/s/units/ms.ts> . Transcription timestamps in seconds (documented exception). It is neither rational nor frame-based. The export uses `t = i * (1000/fps)` as float (`fixedStep`): <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/parts/schedulers.ts#L77-L96> . INFERENCE: float ms + fps 29.97/23.976 produces non-integer timestamps, and `Math.round(i*dt*1e6)` in `produce-video.ts` mitigates it, but does not eliminate accumulated error in transitions/cuts between frames; for our project, rational time or an integer frame index is worthwhile.
- **Transition consumes its own time:** "Transitions add time; they do not automatically overlap or shorten neighboring clips" (SKILL.md). That is, the transition is an item in the sequence, not an overlap of clips (a different model from the classic NLE).
- **No automatic ripple:** removing a child from the sequence shifts the following ones; to preserve time you must replace it with a `Gap` (SKILL.md, section "Editing").
- **Versioning:** a `version: number` field exists (value 1 in `Omni.timeline`: <https://github.com/omni-media/omnitool/blob/main/s/timeline/sugar/omni.ts#L30-L36>), **but the skill says "Describes Omnitool timeline version `0`"** and the code writes `1`. FACT of inconsistency: <https://github.com/omni-media/omnitool/blob/main/skills/timeline/SKILL.md#L8> vs `omni.ts#L33`. I did not find timeline migration code between versions in `s/` (grep for `migrat`/`version` returns only the creation lines). For project 2.0, the strata `Vault` receives `version: 1` (<https://github.com/omni-media/omniclip/blob/dev/s/client/editor/context/parts/strata.ts#L90-L96>), but I did not see migrators in the editor code.
- **Media by hash:** `mediaHash` (BLAKE3 by default) instead of a path; the JSON does not contain files: <https://github.com/omni-media/omnitool/blob/main/s/timeline/utils/datafile.ts> .
- **Shared attachments:** `spatialId/animationIds/filterIds/styleId` point to items in the same array (ECS-like normalization). Changing a shared attachment affects all owners, with the "clone before editing" rule described in the skill.

### Omniclip 1.x (legacy, classic NLE model)

FACT, <https://github.com/omni-media/omniclip/blob/main/s/context/types.ts#L26-L100> :

```ts
export interface HistoricalState { projectName; projectId; effects: AnyEffect[]; tracks: XTrack[]; filters; animations; transitions }
export interface Effect { id; start_at_position: number; duration: number; start: number; end: number; track: number }
export interface VideoEffect extends Effect { kind: "video"; thumbnail: string; raw_duration; frames; rect: EffectRect; file_hash; name }
export interface XTrack { id; locked; visible; muted }
```

Time in ms, float, separate "timebase" 10-120 fps (`timebase: 25` in the non-historical state). Persistence: `localStorage` with the `omniclip_` prefix (<https://github.com/omni-media/omniclip/blob/main/s/context/controllers/store/store.ts>), and project export/import as ZIP via zip.js (`Project` controller). Collaboration uses OPFS (`opfs-manager.ts`) only for exchanged files.

### Omniclip 2.0 persistence

FACT: the whole project (settings + `Chronicle<EditState>` with `past/present/future`) in `localStorage` via `@e280/kv` + `Vault`, media in content-addressable storage of `@e280/quay` (`cellar` workers): <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/context/parts/strata.ts#L9-L61> and <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/context/controllers/cargo.ts#L7-L20> . INFERENCE: the undo history goes along to `localStorage` (limit ~5 MB), which limits large projects; I did not verify whether `quay` uses OPFS underneath (`quay` code not read).

## 4. Preview and export pipeline

### omnitool

- **Preview** (`VideoPlayer`/`Playback`): `requestAnimationFrame` loop in `realtime()` generates ticks; on each tick `CursorVisualSampler.next(time)` produces `Layer[]` and `driver.composite()`; **the master clock is `AudioContext.currentTime`** (`currentTime = playbackStart + (ctx.currentTime - audioStartSec)*rate`). FACT: <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/player/parts/playback.ts#L32-L139> .
- **Scrubbing:** `seek()` coalesces pending requests (`#pendingSeek`, `#flushSeeks`) and uses `createVisualSampler` (random access) instead of the sequential cursor: <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/player/player.ts#L36-L45> and L73-L83 .
- **Decode:** mediabunny's `VideoSampleSink` per hash with an `Input`/sink cache (`VideoSink`): <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/parts/samplers/visual/parts/video-sink.ts> . Cache of decoded frames: **not found** (the only `cache` in `renderers/` is for static images, `image-sink.ts`). Proxies/low-resolution media: **do not exist** (grep with no results).
- **Reverse playback:** `ReverseCursorVisualSampler` and negative `playbackRate` supported; audio only plays at `playbackRate === 1` (`#startAudio` returns if `!== 1`).
- **Export** (`produce`): same `CursorVisualSampler` + `driver.composite()` per frame, `fixedStep` per fps, `VideoFrame` with `timestamp = round(i*dt*1e6)`; audio mixed by `AudioMix`; both as `ReadableStream` for `driver.encode` in the worker (mediabunny does encode/mux). Defaults `webm/vp9 1 Mbps + opus 128k`, containers `mp4|webm|mov|mkv`: <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/export/produce.ts#L24-L52> and <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/export/parts/produce-video.ts#L19-L58> .
- **Do they share the visual render path?** FACT: yes. `Playback` and `produceVideo` import the same `CursorVisualSampler` and the same `driver.composite` (`playback.ts#L11`, `produce-video.ts#L3`). This is the main architectural advantage over 1.x. Caveat (FACT): **audio does not share**. Preview plays `AudioBufferSourceNode`s via WebAudio; export goes through its own `AudioMix` + `resampleToPlanar` (`produce-audio.ts`). INFERENCE: gain/clamp/resample differences between preview and export are possible only in audio.
- **Limits:** the compositor is fixed at 1920x1080 at creation (`autoDetectRenderer({width:1920,height:1080})`), with `resize()` afterward; export has `framerate` and `bitrate` but the output resolution comes from the compositor canvas (I found no resolution parameter in `ExportConfig`). FACT: <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/export/produce.ts#L17-L22> .

### Omniclip 1.x (different!)

FACT: preview uses `HTMLVideoElement`/`HTMLAudioElement` with `currentTime` and `play()` inside Pixi (`videoManager.play_videos`, `audioManager.play_audios`, `seek()` adjusts `element.currentTime`): <https://github.com/omni-media/omniclip/blob/main/s/context/controllers/compositor/controller.ts#L65-L100> and L203-L225 . Export uses a **separate** pipeline: WebCodecs `Decoder` + `compose_effects(..., exporting=true)` + `VideoEncoder` H.264 `avc1.640034` annexb, and audio re-encoded afterward via ffmpeg.wasm (`aac 192k`): <https://github.com/omni-media/omniclip/blob/main/s/context/controllers/video-export/parts/encode_worker.ts#L22-L30> and the `FFmpegHelper` above. FACT: the author himself says "i decided to not use AudioDecoder etc, instead im just using ffmpeg" (comment in `helper.ts#L38`) and in issue #11: <https://github.com/omni-media/omniclip/issues/11> . INFERENCE: preview via a `<video>` element and export via WebCodecs are distinct paths, so the preview ≠ export risk exists in 1.x; the rewrite in omnitool fixes this for video.

## 5. Audio

FACT (omnitool):

- **Mixing in export:** `AudioMix` mixes in chunks of 1024 frames, sums active samples and applies optional clamping; requires constant `sampleRate` and channels across inputs: <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/export/parts/audio-mix.ts#L19-L100> .
- **Resampling:** own JS implementation: linear when upsampling, **windowed sinc (Hann) with radius 32** when downsampling; compensates chunk boundaries to avoid clicks (commit "fix: prevent audio clicks at resampling boundaries" from 2026-11-01): <https://github.com/omni-media/omnitool/blob/main/s/timeline/renderers/export/parts/resamplers.ts#L1-L65> .
- **Channels:** mono/stereo/5.1 conversion (`audio-channels.ts`), linear gain per item and global (`gain`, `enabled`), no gain keyframe ("There is no core keyframed gain or container gain", SKILL.md).
- **Waveform:** `Waveform` with `parts/{collect,cache,render}`: <https://github.com/omni-media/omnitool/tree/main/s/timeline/parts/waveform> . Real-time level meter: `audio-levels.ts` (AnalyserNode).
- **A/V sync:** the master clock is the `AudioContext`; video follows the audio clock. `AudioData` timestamps in export = `startFrame/sampleRate*1e6`. INFERENCE: this is the correct approach to avoid preview drift; in export, sync is deterministic (frames at a fixed index), with no explicit VFR handling (I found no VFR handling in the code).
- In 1.x: audio was extracted/mixed by ffmpeg.wasm (intermediate mp3 + `amix`), and in preview it played in an `HTMLAudioElement`.

## 6. Undo/redo, action layer, MCP/CLI/API, collaboration

### Undo/redo

- **omnitool:** does not implement it (the document is immutable by convention; `grep -i "undo|history|redo" s/` returns zero lines of logic). Responsibility lies with the host.
- **Omniclip 1.x:** history through slate actions (`AppCore` with `history_limit: 64` and `ZipAction`, actions marked `historical`/`non_historical`): <https://github.com/omni-media/omniclip/blob/main/s/context/context.ts#L117-L125> .
- **Omniclip 2.0:** **full snapshots** of `EditState` (timeline + outliner) via `Chrono(64, ...)`: each `mutate` does `deepClone(present)` and pushes into `past`; `undo/redo` move snapshots. FACT: <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/context/parts/strata.ts#L72> and `@e280/strata@0.5.7/x/prism/chrono/chrono.js` (npm tarball). The code has an admitted TODO: "undo does not undo rotation correctly": <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/ui/logic/session.ts#L148> .
- OPINION: snapshot by deepClone is simple and robust, but scales poorly on large timelines and does not produce semantic diffs for AI/collab.

### Serializable command layer reusable by agent

FACT: **yes, in 2.0, as a declarative patch**, not as named commands. The `TimelinePatch` has `baseRevision` (integer) and `operations` of 5 types (`add`, `replace`, `remove`, `set_root`, `set_audio`) over whole items: types and Zod schema in `@omnimedia/omniso@0.1.1-0` (`x/timeline.d.ts`, `x/assistant/schema.js`, read from the npm tarball; source repo unavailable). Application: `applyTimelinePatch` in `session.commitPatch`, which also validates organization rules (`rules`) and records it as **one** undo mutation: <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/ui/logic/session.ts#L127-L136> .

Optimistic concurrency control: `if (patch.baseRevision !== this.revision) throw ... "Timeline revision mismatch"`; the failed patch returns a fresh context: <https://github.com/omni-media/omniclip/blob/dev/s/client/editor/context/controllers/assistant.ts#L34-L64> . Grouping of patches into an undoable "assistant edit" by id (`canUndo`, `undo(id)`): same file, L66-L90.

### MCP / CLI / API / AI

FACT, <https://github.com/omni-media/omniclip/blob/dev/s/server/mcp/README.md> (accessed 2026-11-14):

- **Local stdio MCP server** (`omniclipmcp`, `bin` in the package.json of `dev`) with a WebSocket bridge on **127.0.0.1** to the editor tab (port 3789, exact origin `OMNICLIP_ORIGIN`, "no account authentication on this local connection", access off by default and enabled by a toggle in Settings).
- Tools: `read_project_context`, `update_settings`, `create_item` (opaque ID), `patch_timeline`, `provide_media` (copy of the source to a temporary file), `video_info/video_analyze/video_watch/video_detail` (proxy to the third-party plugin `claude-video-vision 1.4.0`), `audio_analyze` (**placeholder, "not implemented yet"**).
- Agent skill in `s/server/mcp/skills/omniclip/SKILL.md` and, in omnitool, `npx skills add omni-media/omnitool` with skill `timeline` (documents the JSON, filters and transitions, with catalogs): <https://github.com/omni-media/omnitool/tree/main/skills/timeline> .
- AI chat embedded in the editor (assistant-ui, Vercel `ai`), Google login, Stripe billing, media search by description with local AI: recent commits on `dev` (2026-10-20 to 2026-11-13). FACT: <https://github.com/omni-media/omniclip/commits/dev> (accessed 2026-11-14). INFERENCE: the project is migrating toward a model with **paid cloud** (better-auth + stripe + "premium mode" in the TODO), but the code remains MIT.
- **CLI:** omnitool README says "cli soon" (<https://github.com/omni-media/omnitool/blob/main/README.md>). There is no headless render CLI. **HTTP API:** only an auxiliary Fastify for auth/billing; I did not find a public render API.
- **Critical limitation (FACT):** the MCP only works with an **open browser tab** connected (one project per bridge). There is no headless render. The README admits: "This is an initial integration to evaluate, not a promise of precise automatic cuts."
- The README itself points out that `video-vision` **does not preserve fractions of a second** in the frame labels, "so dense extraction is not proof of frame-accurate timing".

### Real-time collaboration

FACT: peer-to-peer WebRTC via `sparrow-rtc` with host/clients, state compressor, file transfer via OPFS; present since 1.x: <https://github.com/omni-media/omniclip/blob/main/s/context/controllers/collaboration/controller.ts> . The model is "host sends state" (`set_incoming_non_historical_state_webrtc` replaces the state), not CRDT. INFERENCE: without semantic merge, it is mirroring with one owner.

## 7. What they do well (OPINION based on the FACTS above)

1. **A single visual render path** for preview and export (pure samplers + compositor), with the audio clock as master.
2. **Timeline as plain, flat JSON with opaque IDs**, without object references: great for AI to read/write, diff and validate. The README describes "creating timelines from code, automating rendering, and integrating with AI".
3. **Patch with optimistic review** (`baseRevision`) + `create_item` for IDs: simple protocol against concurrent human/agent editing.
4. **Agent skill with embedded catalogs** (JSON filters, transitions) and editing rules like "split" and "ripple delete" written as recipes.
5. **WebCodecs streams** (`ReadableStream<VideoFrame/AudioData>`) between production and encode, with transfer to the worker.
6. Transitions from `gl-transitions` (many), filters from `pixi-filters` (40 catalogued), whisper in the browser via Transformers.js.
7. Permissive licenses (MIT) across the core, and mediabunny instead of ffmpeg.wasm in omnitool.

## 8. What to avoid (OPINION)

1. **`<video>` preview** (1.x) with export through another pipeline: produces preview ≠ export.
2. **ffmpeg.wasm for audio/mux** downloaded from unpkg at runtime: CORS/network failures, SharedArrayBuffer/COI (issues #12, #13, #28). FACT: <https://github.com/omni-media/omniclip/issues/12> , <https://github.com/omni-media/omniclip/issues/13> .
3. **State and undo by snapshot** with history inside `localStorage`.
4. **Time in float ms without a rational unit/frames**: ambiguity at 29.97/23.976 and in cuts.
5. **No tracks and no transition "overlap":** the Sequence/Stack model is elegant for AI, but departs from what human editors expect (a transition that overlaps), requiring `Gap`/nesting for offsets. The skill spends a lot of text explaining this, a sign of cognitive cost.
6. **MCP tied to a browser tab** and no headless render.
7. **"Semi-abandoned" dependencies of the author** (`@benev/*`): issue #30 asks for replacement. Mitigated in 2.0 (migration to `@e280/*`), but it is another personal ecosystem.
8. **Commercial direction change** (auth, Stripe, cloud) on the repository.
9. Inconsistent format versions (skill says `0`, code writes `1`) and no visible migrators.
10. Pixi on the main thread with WebGL and a known memory leak in Chrome (comment in the compositor).

## 9. Features present

**omnitool 1.1.0-146** (FACT: README + `skills/timeline/SKILL.md`): video/audio/image/text clips, `Sequence`/`Stack`/`Gap`, trim via `start/duration`, transitions (gl-transitions; dozens of names in `skills/timeline/assets/transitions.json`), filters (40 from pixi-filters), spatial transforms (position, scale, rotation, crop), keyframes for `opacity` and `transform` with 6 interpolations, animation presets (`slideIn`, `slideOut`, `spinIn`...), text with Pixi `TextStyle`, captions (`Caption`) with `maxChars/maxDuration/maxSilence` and `default` preset, Whisper transcription in the browser (`whisper-tiny_timestamped`), background removal (`Xenova/modnet`), waveform, filmstrip, player with play/pause/seek/rate (incl. reverse), export mp4/webm/mov/mkv, audio with gain/mix/resample, items `enabled/label`.

**Omniclip 1.1.3** (`main` README): trim, split, text/audio/video/image, direct editing on the preview (rotate, resize, text style), undo/redo, export up to 4K, project manager, transitions, filters, timebase 10-120 fps, WebRTC collaboration. "To be added": audio editing (volume), speech to text, **keyframes**. FACT: <https://github.com/omni-media/omniclip/blob/main/README.md> .

**Omniclip 2.0 (`dev`)**: everything above via omnitool + AI assistant, MCP, AI media search, tags/organization rules, proposals, configurable shortcuts, login/billing. The `dev` `TODO.md` still lists Phase 2/3 (selection, slip edit, snapping, etc.) as pending, that is, the 2.0 editor is **pre-alpha** (`2.0.0-4`).

**Not present** (FACT via grep/README): proxies, frame cache, color grading beyond the filters, masks, speed ramp ("no per-item speed fields"), native word-by-word highlight captions (only the `default` preset found), CLI, headless render, cloud export.

## 10. Maintenance status (accessed 2026-11-14)

| Item | omniclip (`main`/`dev`) | omnitool |
| --- | --- | --- |
| Stars | 1474 | 44 |
| Last commit | `main`: 2025-08-07; `dev`: 2026-11-13 (30+ commits since 2026-10, ~399 ahead of `main`) | 2026-11-02 |
| GitHub releases | **none** (`gh release list` empty); tags `v1.1.1` to `v1.1.3` | none |
| Version | 1.1.3 (npm `@omnimedia/omniclip`) / 2.0.0-4 (`dev`) | 1.1.0-146 (npm), changelog says "this will be the first real release" |
| Open issues | 25 (repo counter) | 0 (issues enabled) |
| Contributors | 1 listed by the API (`zenkyuv`, 460 commits); `chase-moskal` appears as a collaborator | 2 (`zenkyuv` 517, `chase-moskal` 62) |
| Branches | 26 (includes `pixijs-v8`, `scute`, `horizon`, `proposal-system`) | n/d |
| License | MIT | MIT |

Sources: <https://github.com/omni-media/omniclip> , <https://api.github.com/repos/omni-media/omniclip> (via `gh api`), <https://github.com/omni-media/omnitool> .

Direction changes (FACT): 2023-11 start of Lit web components; 2025-03 branch `pixijs-v8` ("almost make gl-transition work in pixi v8"); 2025-07 swap of `turtle` for `scute`; 2025-08 last release 1.1.3 and extraction into omnitool; 2026-10/11 2.0 rewrite with AI/MCP/cloud/billing. INFERENCE: a project with one main maintainer (one author holds ~90% of commits) with an irregular rhythm: 15 months without commits on `main`, then an intense burst on `dev`.

## 11. License

- Repos: MIT (omniclip, omnitool, webm-hero, kimura): <https://github.com/omni-media/omniclip/blob/main/LICENSE> . Note: the `main` `package.json` of omniclip declares `"license": "ISC"` while the repo/LICENSE is MIT (minor inconsistency, FACT), and `dev` fixes it to MIT.
- **mediabunny 1.65.0 and @mediabunny/aac-encoder: MPL-2.0** (file-level copyleft, weak; allows use in an MIT project as long as modifications to the MPL files are published). FACT: <https://github.com/omni-media/omnitool/blob/main/package-lock.json#L3494> and L1224 .
- pixi.js 8.x MIT, pixi-filters 6.1.5 MIT, gl-transitions 1.74.0 MIT (omnitool lockfile), `@huggingface/transformers` Apache-2.0 (L649 of the lockfile). Models (`whisper-tiny`, `modnet`) have their own licenses that I **did not verify**.
- **ffmpeg.wasm (@ffmpeg/ffmpeg 0.12.x):** the wrapper is MIT; the **core** (`@ffmpeg/core`) is LGPL/GPL depending on the build (the standard core of the `ffmpegwasm` project is usually GPL because it includes libx264). I did not verify the license of `@ffmpeg/core@0.12.5` in the primary source in this session (**gap**). Relevant because 1.x and `dev` still depend on `@ffmpeg/ffmpeg ^0.12.15` and the core comes from unpkg. omnitool itself **does not use** ffmpeg.wasm.
- Browser codecs (H.264/AAC) and patent licenses are outside the scope of the software license.
- `claude-video-vision 1.4.0` (dependency of `dev` for the MCP): license **not verified**.

## 12. Real pain points (issues/discussions)

Note: the repo has few issues (30 in total, 25 open) and almost all of them are about installation. Preview/audio quality problems show up more in the author's comments and in the code than in issues.

1. <https://github.com/omni-media/omniclip/issues/12> : broken demo ("Is the demo broken"); failure to download ffmpeg.wasm from unpkg (CORS/network), worked only in Edge for the reporter.
2. <https://github.com/omni-media/omniclip/issues/13> (closed): `SharedArrayBuffer is not defined` when used in Vite/Nuxt (requires COOP/COEP headers, `coi-serviceworker`).
3. <https://github.com/omni-media/omniclip/issues/28> : "Save Video" button does not work after export in Brave (File System Access API).
4. <https://github.com/omni-media/omniclip/issues/20> : animated GIF does not play in the preview or in the export (animated image format limit).
5. <https://github.com/omni-media/omniclip/issues/21> : "ghost" media after cancelling an import and reimporting (inconsistent media state).
6. <https://github.com/omni-media/omniclip/issues/11> : WebKit/Safari support; AudioDecoder was not supported in Safari at the time and the author refuses a fallback; error in frame extraction in Safari.
7. <https://github.com/omni-media/omniclip/issues/5> : request for 9:16 vertical resolution ("at least 50% of video editing these days is vertical"). Resolved in 1.x with `AspectRatio` `9/16` in the state and in 2.0 at `settings.aspectRatio` (code read; the issue remains open).
8. <https://github.com/omni-media/omniclip/issues/30> (closed): the author's dependencies (`@benev/turtle`, `@benev/construct`, `@benev/slate`) "semi-abandoned".
9. <https://github.com/omni-media/omniclip/issues/26> : build does not work on Windows (scripts use `cp`, `rm`); answer: use WSL. Lesson for us (dev on Windows).
10. <https://github.com/omni-media/omniclip/issues/3> : suggestion to use OpenTimelineIO instead of inventing a custom JSON format. <https://github.com/omni-media/omniclip/issues/18> : usability and missing crop/transforms in 1.x.
11. Pain points admitted in the code (FACT): "webgl and webgl2 causes memory leaks on chrome" (`compositor.ts#L35`), "undo does not undo rotation correctly" (`session.ts#L148`), and in the MCP README "Video-vision 1.4.0 does not preserve fractional seconds in frame labels". No open issue explicitly addresses audio drift, VFR, text/fonts or memory; I **did not find** discussions (GitHub Discussions not verified) on these topics.

## 13. Lessons for our project (agent-first)

**Copy**

1. **Timeline as a flat JSON document with opaque IDs and attachments by reference** (spatial/animation/filter/style by id). It is the most "LLM-friendly" format I saw in this set, and the skill shows that an agent can be taught to edit it without code.
2. **Patch with `baseRevision` + `create_item`** for human/agent concurrency, returning fresh context also on failure. Ideal for the command core: each command = one atomic patch = one undo step.
3. **A single composition path** (pure samplers that produce `Layer[]` per instant, single compositor) for preview and export; in our case, the same render graph also serves the headless CLI render.
4. **Master clock on the audio** for preview and fixed frames by index in export.
5. **Skill/AGENTS published alongside the lib** with catalogs generated by script (`scripts/catalog.js`) and editing recipes (split, ripple delete).
6. mediabunny for demux/mux/encode instead of ffmpeg.wasm (MPL-2.0 is acceptable for MIT, as long as modifications to the MPL files are returned).

**Avoid / do differently**

1. **Do not tie the agent to a browser tab.** Omniclip's MCP requires the editor open and a local WebSocket bridge. Our differentiator should be a headless command core (Node/desktop) with the UI as one more client.
2. **Time:** use whole frames or rational (num/den) in the document and convert to ms only at the edge, instead of float ms. It defines determinism for 9:16 captions and sync with 29.97/23.976.
3. **Undo by commands/diffs** over the patch log (inverse or checkpoints), not full snapshots in `localStorage`.
4. **Audio on the same preview/export path** (omnitool still diverges: WebAudio in preview, `AudioMix` in export). Measure the difference in an automated parity test.
5. **Format versioning and migration from day 1** (`version` field + tested migrators); the `0` vs `1` inconsistency in omnitool is the example of what to avoid.
6. **Optional tracks or, at least, derived "lanes".** The Sequence/Stack tree is good for AI, but vertical captions and product motion design usually need explicit overlap and offsets; evaluate a hybrid model (tree + `offset`/transition overlap).
7. **Semantic validation on the server for the patch** (existing IDs, types, acyclicity, resulting duration) with readable errors; omnitool leaves this to the "host app's validation" (SKILL.md) and the agent has to check manually.
8. **Frame/proxy cache, `VideoFrame` memory control and render off the main thread** (OffscreenCanvas + worker): absent here; relevant for 9:16 at 1080x1920 and commercials with heavy motion.
9. **Captions:** the `Caption` model (transcription in seconds + breaks by `maxChars/maxDuration/maxSilence`) is a good starting point, but there is only one preset; for vertical shorts we will need per-word styling and per-word animation (not present here).
10. **Product:** avoid coupling the open project to login/billing in the same repository if the goal is MIT and community-driven; separate the cloud layer.

## Gaps and unverified items

- Source code of `omniso` (private/unavailable) and of `@e280/quay` (not read: I did not confirm whether media uses OPFS or IndexedDB in 2.0).
- License of `@ffmpeg/core@0.12.5` and of `claude-video-vision`.
- I did not run anything (no build/run), so performance, real drift and export quality were not measured.
- I did not consult GitHub Discussions nor Discord (main communication channel, <https://discord.gg/Nr8t9s5wSM>, not accessible).
- Exact list of transitions in `transitions.json` (I counted only partially; the file lists dozens of gl-transitions names) and of animation presets (I saw `slideIn`, `slideOut`, `spinIn`).
