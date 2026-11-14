# Clipchamp (Microsoft)

> **Status of this file.** Project profile delivered by a research agent on 2026-11-14 and reviewed by the architect. Its "lessons" are input to the design, not decisions; the decisions are in `../../brainstorming.md` and `../../adrs/`. Code excerpts and diagrams are quoted from the project as found and are not translated.

Research on 2026-11-14 (accessed 2026-11-14). The Microsoft support page comparing plans returned 404; I used clipchamp.com and search results. Labels: FACT / INFERENCE / OPINION.

## 1. Platforms and technology

- FACT: web app (no installation) and Windows desktop app (built into Windows 11, Microsoft Store on Windows 10); screen/camera/voice recording on Windows and Mac. <https://clipchamp.com/pt-br/>
- FACT: Microsoft recommends the recent version of Edge or Chrome. <https://support.microsoft.com/en-US/Clipchamp/which-web-browsers-does-clipchamp-support>
- FACT: installable PWA (web.dev case: "all the heavy processing is local, on the user's hardware"; WebAssembly SIMD accelerates 4K decode/encode). <https://web.dev/case-studies/clipchamp>
- FACT (W3C talk from 2021, may be outdated): FFmpeg compiled to WASM (demux, I/O, fallback codecs, filters) + WebCodecs for encode, with decode/encode workers and an effects compositor; WebGL (WebGPU emerging). The pipeline runs in the browser: no upload, low server cost, privacy. Limits: memory, storage, hardware. <https://www.w3.org/2021/03/media-production-workshop/talks/soeren-balko-clipchamp-webcodecs.html>
- INFERENCE: it is the closest example of what an open source web editor can do: 100% local rendering. Note: "downloading" the app for Windows 10/11 is essentially the same web editor.

## 2. Flow and layout

- INFERENCE (not verified in an official page): left sidebar (media, content library, text, transitions, filters, brand kit), preview in the center, properties panel on the right, multi-track timeline at the bottom. The "What's new" page mentions a "floating toolbar and properties panel", "grouping", "multi-item editing", "gap removal" and "speed controller" (<https://clipchamp.com/en/whats-new>; no dates).

## 3. Features

Source: <https://clipchamp.com/pt-br/> and <https://clipchamp.com/en/whats-new>

- Import: upload, camera/screen/voice recording, import of Xbox clips. No list of import formats found (gap).
- Timeline/cutting: cut, trim, rotate, free crop, gap removal, multi-item editing, grouping, speed.
- Text: text overlay, 20+ new fonts, vector shapes.
- Captions: automatic caption generator for 80+ languages, editable style/font/color (home).
- Audio: AI noise removal, silence removal and removal of filler pauses/words, TTS in several languages, recorder, audio visualizer, extraction, MP3 export.
- Transitions, filters: video transitions, color/gradient backgrounds.
- Keyframes: no official source confirms them. Gap (a third-party tutorial mentions keyframes + mask; not reliable).
- Templates: 100+ video templates, 200+ free songs, content library. Aspect ratio presets.
- Stock: videos, images, music, stickers, GIFs, sound effects (home).
- AI: auto compose, upscaler (Windows), image/video background removal and chroma key, noise suppression, silence remover.
- Collaboration: not a highlight; Brand kit (premium in the personal version, <https://support.microsoft.com> via search).
- Export: see section 5.

## 4. Limits and paid plans

- FACT: the free plan exports up to 1080p without a watermark; premium up to 4K. The lock is premium content (stock) in a free project: either remove it or subscribe. <https://clipchamp.com/en/pricing/> and <https://clipchamp.com/en/blog/free-1080p-exports/> (via search)
- FACT: work/school accounts (Standard) include 4K and brand kit. Help via search.
- INFERENCE/secondary: in 2026 premium is included in Microsoft 365 Personal/Family and the standalone plan is no longer sold (<https://www.inkl.com/news/clipchamp-2026-review>; not confirmed in a Microsoft source). The "What's new" page mentions "4K for Essentials subscribers", which contradicts or updates this: treat the plan as uncertain. Prices: do not cite.
- An isolated report of an improper upgrade prompt when exporting 1080p (WPS) has low confidence.

## 5. Formats

- FACT (Microsoft help): export at 480p, 720p, 1080p and 4K; also GIF and audio only (MP3). <https://support.microsoft.com/en-us/clipchamp/how-to-choose-the-right-export-resolution>
- Export containers/codecs: not documented in the sources read (usually MP4 H.264; INFERENCE).
- Aspect ratio presets exist (What's new); social network presets: not verified.

## 6. What they do well

- OPINION: local rendering with no queue and no upload; free 1080p export without a watermark (a differentiator against competitors); accessible automatic captions and silence removal; integration with Windows/Microsoft 365.

## 7. Complaints

- Crashes and projects that fail to load, blank transitions panel (Microsoft Tech Community, Jan/2026: <https://techcommunity.microsoft.com/discussions/microsoft-clipchamp-discussions/clipchamp-app-not-functioning-properly-%E2%80%94-transitions-panel-blank--frequent-crash/4488457>).
- Slowness in projects with several 1080p clips and less than 16 GB of RAM; no auto-save in a G2 review (<https://www.g2.com/products/clipchamp/reviews>).
- Missing advanced effects and audio; slow export (G2/Capterra via search). Note: G2 4.1/5 (52 reviews) according to the search.

## 8. API and agents

- I did not find a public API, plugin or agent/MCP integration for Clipchamp. Gap; negative result, not conclusive.

## 9. Lessons

- INFERENCE: Clipchamp shows that the browser + WebCodecs + FFmpeg-WASM in a PWA path works up to 4K; the talk cites memory/storage limits, which explain the crashes in heavy projects: plan for proxies and auto-save.
- OPINION: the decision to keep 1080p without a watermark sets the expectation an open source editor should match or beat (no watermark even at 4K, since the cost is local).
- OPINION: project persistence (auto-save, recovery) is a requirement, given the reports of lost work.
