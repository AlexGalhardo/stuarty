# CapCut (ByteDance)

> **Status of this file.** Project profile delivered by a research agent on 2026-11-14 and reviewed by the architect. Its "lessons" are input to the design, not decisions; the decisions are in `../../brainstorming.md` and `../../adrs/`. Code excerpts and diagrams are quoted from the project as found and are not translated.

Research on 2026-11-14 (accessed 2026-11-14 for all URLs). Labels: FACT (source), INFERENCE, OPINION.
Limitation: most of CapCut's pricing/help pages are SPAs, and several secondary sources diverge. Where they diverge, this is stated.

## 1. Platforms and technology

- FACT: web ("Try online"), desktop (Windows/macOS) and mobile; "CapCut Pad" is also listed. https://www.capcut.com/pt-br/
- FACT: the web version requires a recent Chrome/Edge/Safari, a hardware-accelerated encoder and a stable connection (Chrome on Windows/macOS is the most reliable). Desktop: Windows 10 64-bit+ or macOS 12+. https://www.capcut.com/help/export-videos-in-capcut
- FACT: the editing engine is mostly C++, ported to the web with Emscripten/WebAssembly (native WASM exceptions reduced the package by 15%). https://web.dev/case-studies/capcut
- FACT (zh-tw version of the same page, less reliable because it is a translation and uses ByteDance's own numbers): WebCodecs with hardware encode/decode, several simultaneous 4K streams, H264/HEVC/VP8/VP9/AV1. https://web.dev/case-studies/capcut?hl=zh-tw
- INFERENCE: the "shared native C++ engine + WASM" model explains the web/desktop parity, but it is an investment that a small MIT project does not replicate; the analogous route is a single core (Go/Rust/C++) compiled for both targets, or WebCodecs on the web and FFmpeg on the desktop.

## 2. Flow and UI layout

- Classic NLE layout: media/resources panel (media, audio, text, stickers, effects, transitions, captions, filters, templates) at the top left, player in the center, inspector/properties on the right, multi-track timeline at the bottom. Source: official guides https://www.capcut.com/resource/how-to-use-capcut-on-pc (not read in full; description based on the feature titles and the search summary). Marked as INFERENCE of layout.
- FACT: the home page lists "timeline editor", "smart editor", cutter, converter, resizer, image editor, meme generator and "design studio with templates". https://www.capcut.com/pt-br/

## 3. Features (grouped)

General source: https://www.capcut.com/pt-br/ and https://www.capcut.com/resource/capcut-standard-vs-pro

- Import/recording: media upload; no official list of import formats was found (gap).
- Timeline/cutting: cut, trim, merge, split (FACT, Standard vs Pro page). Motion tracking is Pro (FACT, same page).
- Text: text, stickers (FACT). Premium fonts are limited in Standard.
- Captions: auto captions and speech-to-text (FACT, home). See the plan conflict below.
- Audio: noise removal, voice enhancer, text-to-speech (40+ languages, FACT home), custom voice, extract audio. Extract Audio, Text, Effects and Overlay are the "most used" features according to CapCut itself (https://www.capcut.com/newsroom/revealing-the-creative-trends-on-capcut-in-2025).
- Transitions, effects, filters: free and premium (FACT).
- Keyframes/animation: no official page read; presence in an NLE of this size is INFERENCE (not verified).
- Templates: ">1 million video/graphics templates" migrated to the web (FACT, web.dev zh-cn/zh-tw; ByteDance's number).
- Library/stock: music, fonts, premium effects (FACT).
- AI: AI video generator, upscaler, background removal, inpainting, text/people removal, "EditPilot" (agent: text-based editing on the desktop timeline, https://www.capcut.com/tools/edit-pilot), third-party models listed (Seedance, Sora 2, Veo 3.1, GPT Image 2, Seedream; FACT home).
- Collaboration: Team plan/collaboration (FACT, Standard vs Pro).
- Export: see section 5.

## 4. Limits and paid plans

- FACT: Standard tiers (free), Pro (US$19.99/month or US$179.99/year) and Team (from ~US$24.99/month); "prices vary by region/platform". https://www.capcut.com/resource/capcut-standard-vs-pro
- FACT: Standard has no advanced AI (motion tracking, auto captions); templates/effects/fonts/premium music are only limited; the free 1 GB cloud storage was discontinued in Aug/2024; Pro ~1024 GB. Same page.
- CONFLICT: third parties say auto captions have a limited free tier and others say they are Pro; the official page says auto captions are Pro. Recorded as uncertain. There are also reports of "two paid tiers in 2026" (https://tlinky.com/capcut-review/, secondary, not confirmed).
- FACT: using a Pro template/asset on the free plan exports with a watermark; when exporting, the app warns and asks for a trial/subscription. https://www.capcut.com/help/capcut-pro-features-unavailable
- FACT: on the web, free accounts may get a watermark or a bitrate limit at 4K. https://www.capcut.com/help/export-videos-in-capcut
- Do not cite the prices above without rechecking: secondary sources list different values (US$7.99, 9.99, 89.99).

## 5. Formats and export

- FACT: web 1080p/2K/4K; desktop 2K and 4K (smaller options via project settings), up to 60 fps depending on the source, bitrate adjustable on desktop; mobile 720p to 4K. https://www.capcut.com/help/export-videos-in-capcut
- CONFLICT: the Standard vs Pro page says Standard and Pro export at "8K"; the help page does not list 8K. Treat as not verified.
- Containers/codecs: the help does not specify them; secondary guides cite MP4 and MOV, H.264 and HEVC (not verified).
- Per-network presets: reports of aspect ratios for TikTok/Instagram etc. (not confirmed in an official source in this research).

## 6. What they do well

- OPINION (based on the sources): speed in getting to a captioned vertical video; trending templates; web/desktop/mobile parity; stylized auto captions as the #1 use (CapCut newsroom, auto captions among the most used AI features; the vendor's own source, with platform bias).

## 7. Recurring complaints

- Features that were previously free moved to Pro; an analysis of 1 to 3 star reviews attributes ~26% of substantive negatives to "feature locked behind Pro" (https://unstar.app/blog/capcut-everything-is-pro-now-cant-export-reviews-2026; third-party analysis site, own methodology).
- Watermark on template exports even with Pro active, and "Pro features applied without warning, blocking export" (Trustpilot: https://uk.trustpilot.com/review/www.capcut.com?page=9; https://www.sikayetvar.com/en/capcut-us/capcut-export-issues-and-overuse-of-pro-features-limit-usability). The official answer is "Pro is not active on the account / update the app" (help above).
- High price for long-time users (https://www.sikayetvar.com/en/capcut-us/capcuts-high-pricing-leaves-long-time-users-frustrated).
- No Reddit thread was accessible (gap).

## 8. Public API, automation and agents

- FACT: I did not find an official public API for CapCut/Dreamina/CapCut Commerce Pro.
- FACT: there is an official "CapCut x Codex/ChatGPT" integration "powered by CapCut Skills" with an MCP connection that creates/edits projects that remain editable in the CapCut timeline (https://www.capcut.com/tools/capcut-x-codex; no date or public API described).
- FACT: EditPilot on desktop (PC): natural-language requests applied on the timeline, "not a one-click automatic generator" (https://www.capcut.com/tools/edit-pilot).
- FACT: unofficial community projects: CapCutAPI (HTTP + MCP) and CapCut Mate (FastAPI, Apache-2.0) that generate "drafts" for CapCut/Jianying (https://github.com/ashreo/CapCutAPI, https://github.com/Hommy-master/capcut-mate). Compatibility and maintenance not verified.

## 9. Lessons for our project

- INFERENCE: the real selling point is "stylized auto captions + template + vertical export", not the NLE. This confirms the 20% cut.
- INFERENCE: the project data model (draft JSON) that communities have already reverse-engineered shows that a declarative project format enables agent automation; it is worth designing our JSON as a public contract from the start.
- OPINION: avoid the anti-pattern that generates the most complaints: "paid" templates/effects applied without warning. If there is a premium layer, flag it before applying it, not only at export time.
