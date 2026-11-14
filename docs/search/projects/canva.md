# Canva Video Editor

> **Status of this file.** Project profile delivered by a research agent on 2026-11-14 and reviewed by the architect. Its "lessons" are input to the design, not decisions; the decisions are in `../../brainstorming.md` and `../../adrs/`. Code excerpts and diagrams are quoted from the project as found and are not translated.

Research on 2026-11-14 (accessed 2026-11-14). The page <https://www.canva.com/video-editor/> returned HTTP 403 to WebFetch (it blocks bots); so did the newsroom. The data comes from the Help Center (searches) and secondary sources. Labels: FACT / INFERENCE / OPINION.

## 1. Platforms and technology

- INFERENCE: web (main editor), desktop and mobile apps. According to the help, 4K on download is "desktop only" (<https://www.canva.com/help/download-as-video/>, via search result; page not opened directly).
- Technology: no public engine document (WebCodecs/WASM/cloud) was found. Gap. Weak INFERENCE: the download requires rendering ("Preparing") and there are daily per-export limits, which suggests server-side rendering; not verified.

## 2. Flow and layout

- FACT (secondary): the old editor was based on "pages" with layered elements; "Video 2.0" (launched with the Creative Operating System, Oct/2025) moved to a continuous layered timeline, with a simplified timeline and AI for trim, sync and effects. Sources: <https://petapixel.com/2025/10/30/canvas-new-creative-os-promises-to-be-its-biggest-evolution-yet/> , <https://bestmediainfo.com/mediainfo/mediainfo-digital/canva-launches-creative-operating-system-with-design-ai-video-20-and-all-new-affinity-10608968> (press coverage; the official page did not open).
- Layout: central canvas + side panel (templates, elements, uploads, text, audio) + bottom timeline. INFERENCE, not verified in an official source.

## 3. Features

- Import: MOV, GIF, MP4, MPEG, MKV, WEBM, up to 1 GB; ProRes 4444, HDV 720p60, G2M3/G2M4 fail; a 4K video uploaded is reduced to 1080p (FACT, <https://www.canva.com/help/upload-formats-requirements/>, via search).
- Cutting: trim, split, transitions on the free plan (secondary: <https://sparxno.com/blog/canva-video-editor>).
- Text/animation: text, in/out animations and "motion paths" (secondary). Lack of per-property keyframes is a competitor's complaint (<https://www.capcut.com/resource/canva-video-template>, a biased source).
- Captions: customizable captions in Video 2.0 (press); languages: not verified.
- Audio: library music, AI voice, volume balancing and multi-track in Video 2.0 (press). Audio cleanup requires Pro (secondary). No EQ/mixer (secondary, <https://tryuncle.com/learn/davinci-resolve/davinci-resolve-vs-canva-video-editor-for-beginners>).
- Effects/filters: basic; no chroma key on any plan according to one review (<https://fluxnote.io/guides/canva-video-editor-review>; to be verified, as that vendor is secondary).
- Templates: strong; it is the core of the product (Brand Kit, Magic Video: a video of up to 60 s generated automatically, <https://www.canva.com/en_in/help/magic-video/>).
- Library/stock: large (photos, videos, music, elements); part of it is Pro-exclusive.
- AI: Magic Video; AI clips (max. 8 s according to a secondary review).
- Collaboration: joint editing, comments, links (product standard; not verified in an official source in this research).
- Export: MP4 and GIF only.

## 4. Limits and paid plans

- FACT (help via search): Free: MP4 up to 30 min and 1920x1080; GIF up to 1 min 1280x720. Pro/Teams/Edu/Nonprofit: MP4 up to 2 h at 3840x2160; GIF up to 2 min 1920x1080. <https://www.canva.com/help/download-as-video/> , <https://www.canva.com/help/cant-download-design-or-video/>
- CONFLICT: one help page says free users see an upgrade prompt when changing quality (480p/720p/4K) and another says the default download is 1080p; secondary sources say 720p is free. Treat free 1080p as likely, but not confirmed.
- FACT: batch export limits: 25 MP4/GIF per download; 500 designs per day or 75 every 5 min. Same help pages.
- Pro price cited in a secondary source: US$14.99/month (~US$6.50/month annually), <https://fluxnote.io/guides/canva-video-pricing-guide-2026>. Do not use without revalidating.
- Direct sharing to networks has its own limitations: <https://www.canva.com/help/sharing-social-media-limitations/> (not read).

## 5. Formats

- Import: see section 3. Export: MP4 (and GIF). Codec/bitrate: no official data; format presets by network: designs start from sizes such as "Instagram/TikTok Video" (INFERENCE).

## 6. What they do well

- OPINION (synthesis of the sources): templates + brand kit + stock in one place; almost no learning curve; collaboration.

## 7. Complaints

- Slowness in long/heavy projects in the browser (TechRadar via <https://www.flexclip.com/learn/canva-video-2-alternative.html>; FlexClip is a competitor).
- Timeline less precise than an NLE; shallow audio; no per-property keyframes, chroma key, etc. (competitor sources, with bias: <https://www.capcut.com/resource/canva-video-template>, <https://fluxnote.io/guides/canva-video-editor-review>).
- A 4K video uploaded is downgraded to 1080p (official help).

## 8. API and agents

- FACT: Canva offers an official remote MCP server (AI Connector, `mcp.canva.com/mcp`, OAuth) for Claude/ChatGPT/Cursor to create and edit designs, and the Connect API (REST). There is also a Dev MCP Server for those building apps. Sources: <https://canva.dev/docs/connect/mcp-server> , <https://canva.dev/blog/developers/canva-dev-mcp-server> , <https://www.canva.dev/blog/developers/canva-and-coding-agents-platforms/> (via search). Specific support for video editing via MCP/API: not verified.

## 9. Lessons

- INFERENCE: Canva's success in short video comes from template + brand kit; for use case 2 (commercial), a "brand kit" (logo, colors, font) applicable to any template is more valuable than a large set of effects.
- OPINION: a simplified timeline that hides complexity serves the casual audience, but it draws the criticism "imprecise". Our scope should have a simple timeline that is precise in frames, without pages.
- INFERENCE: having an official MCP server indicates that the market expects "command-driven" editing; a declarative project format makes this easier.
