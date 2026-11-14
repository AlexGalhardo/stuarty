# Raw feature inventory (CapCut, Canva, Clipchamp) and platform specs

> **Status of this file.** Research notes delivered by a research agent on 2026-11-14 and reviewed by the architect. The opinions and recommendations in this file predate the author's decisions (`../../brainstorming.md`, D-001 to D-016) and the ADRs (`../../adrs/`). Where they differ, the ADRs prevail. Facts rechecked by the architect are listed in [`architect-verifications.md`](architect-verifications.md).

Generated 2026-11-14. Detailed sources in the profiles `docs/search/projects/{capcut,canva,clipchamp}.md`.

Legend for the product columns: **S** = confirmed in an official source or press cited in the profile; **s?** = only a secondary source or an inference (INFERENCE); **?** = not verified; **-** = not observed/absent according to review (may be a gap); **ext** = standard in DaVinci/Premiere/Descript, not observed in the 3 products, marked "probably out of scope". Complexity column: P/M/G, OPINION. Value: high/medium/low with a reason.

Frequent-use evidence: there is only weak data: CapCut says Text, Effects, Extract Audio and Overlay are the most used, and that auto captions are among the most used AI features (https://www.capcut.com/newsroom/revealing-the-creative-trends-on-capcut-in-2025, accessed 2026-11-14; the vendor's own data). A statistic of "88% of creators use AI captions" (voxbooster) has no accessible methodology: do NOT use it. There is no verified independent research (gap).

| Feature | Category | CapCut | Canva | Clipchamp | Frequent-use evidence | Value case 1 (short 9:16) | Value case 2 (commercial) | Complexity (OPINION) |
|---|---|---|---|---|---|---|---|---|
| Import video/audio/image (MP4, MOV, etc.) | Import | S | S (MOV, MP4, MPEG, MKV, WEBM, 1 GB) | S | no data | high: base of everything | high | M (varied decoding) |
| Record camera/screen/voice | Import | s? | S (video suite) | S | no data | medium: reaction/narration | medium | M |
| Drag to multi-track timeline | Timeline | S | S (Video 2.0) | S | no data | high | high | G |
| Cut/split/trim | Timeline | S | S | S | no data | high: cuts are case 1 | high | M |
| Merge/ripple delete/remove gaps | Timeline | s? | ? | S (gap removal) | no data | high: cut rhythm | medium | M |
| Speed (ramps, slow/fast) | Timeline | s? | s? | S (speed controller) | no data | high: memes/cuts | medium | M |
| Zoom/crop/reframe to 9:16 (reframe) | Timeline | S (resizer) | S (resizer) | S (cropper, aspect presets) | no data | high: convert 16:9 to 9:16 | high: variants per network | M |
| Layers/overlay (PiP, overlays) | Timeline | S | S | S | CapCut: Overlay among the most used | high | high | M |
| Static text | Text | S | S | S | CapCut: Text among the most used | high | high | M |
| Animated text (in/out) | Text | S | S | s? | no data | high: engagement | high: motion design | M |
| Text styles/presets, fonts, brand fonts | Text | S | S | S (20+ fonts) | no data | medium | high: brand | M |
| Automatic captions (speech-to-text) | Captions | S (Pro, see profile) | S (Video 2.0, s?) | S (80+ languages) | CapCut: among the most used AI; "captions almost mandatory" is INFERENCE | high: mandatory in practice | medium | G (ASR in browser/local) |
| Edit captions by text/timestamps | Captions | S | s? | S | no data | high | medium | M |
| Word-by-word stylized captions (karaoke) | Captions | s? | ? | ? | no data | high: format standard | medium | M |
| Caption translation | Captions | s? | ? | s? | no data | medium | medium | G |
| Library music | Audio | S | S | S (200+) | no data | high (licensing is the pain) | high | P (library) / M (license) |
| Extract audio | Audio | S | ? | S (MP3) | CapCut: Extract Audio among the most used | medium | low | P |
| Volume, fade, ducking | Audio | S | S | S | no data | high | high | M |
| Noise reduction/voice enhance | Audio | S (Pro) | s? (Pro) | S | CapCut: noise removal listed as AI | medium | medium | G |
| Silence removal | Audio | s? | ? | S | no data | high: talking-head cuts | low | M |
| Text-to-speech / AI voice | Audio | S (40+ languages) | S | S | CapCut: TTS among the most used AI | medium | medium | G |
| Beat sync/beat markers | Audio | s? | ? | ? | no data | medium: cuts on the rhythm | medium | M |
| Equalizer/multi-bus mixer | Audio | ext | - | - | no data | low | low | G (probably out of scope) |
| Transitions (fade, slide, zoom) | Transitions | S | S | S | no data | medium | high | M |
| Color filters/LUT | Effects | S | s? | S | no data | medium | medium | M |
| Video effects (blur, glitch, shake) | Effects | S | s? | s? | CapCut: Effects among the most used | medium: memes | medium | M/G |
| Color adjustments (brightness, contrast, saturation) | Effects | S | S | S | no data | medium | high | M |
| Background removal/chroma key | Effects | S | - (no chroma key per review) | S | no data | medium | high: product on a clean background | G |
| Keyframes per property | Animation | s? | - (only motion paths, competitor review) | ? | no data | medium | high: motion design | M/G |
| Motion paths/ready animations | Animation | S | S | s? | no data | medium | high | M |
| Masks and blend modes | Animation | s? | ? | s? | no data | low | medium | G |
| Motion tracking | Animation | S (Pro) | ? | ? | no data | low | medium | G |
| Video templates | Templates | S (>1 million, vendor number) | S | S (100+) | CapCut: trending templates (qualitative) | high | high | M (format) / G (catalog) |
| Brand kit (logo, colors, fonts) | Templates | s? | S | S (premium) | no data | low | high | M |
| Stock (video, image, sticker, GIF, SFX) | Library | S | S | S | no data | medium | high | G (licensing) |
| Stickers/emojis/GIFs | Library | S | S | S | no data | medium | low | P |
| AI video generation | AI | S | S (8 s, review) | - | no data | low | medium | G (out of scope) |
| Auto compose / video from prompt | AI | S (EditPilot) | S (Magic Video 60 s) | S (auto compose) | no data | medium | medium | G |
| Editing by command/agent (MCP/skills) | AI | S (EditPilot, CapCut x Codex) | S (Canva MCP, designs) | ? | no data | medium | medium | G (depends on the project model) |
| Video upscale/enhance | AI | S | ? | S (Windows) | no data | low | low | G |
| Real-time collaboration/comments | Collaboration | S (Team) | S | ? | no data | low | medium | G |
| Cloud save/auto-save | Project | S (Pro 1 TB) | S | s? | complaints about lost work in Clipchamp | high | high | M |
| Undo/redo, shortcuts | Project | S | S | S | no data | high | high | M |
| Aspect presets (9:16, 1:1, 16:9, 4:5) | Export | S | S | S | no data | high | high | P |
| 1080p MP4 export | Export | S | S | S | no data | high | high | M |
| 4K export | Export | S (Pro/limits) | S (Pro) | S (premium) | no data | medium | high | M |
| 60 fps export | Export | S (desktop) | ? | ? | no data | medium | medium | P |
| GIF export | Export | s? | S | S | no data | medium: memes | low | P |
| Audio-only export (MP3) | Export | s? | ? | S | no data | low | low | P |
| Publish directly to social networks | Export | s? | S (with limits) | s? | no data | medium | medium | M (third-party APIs, tokens) |
| Watermark on the free plan | Business model | S (Pro templates) | ? | S (premium content only) | recurring complaint about CapCut | n/a | n/a | n/a |
| Advanced color grading (curves, scopes, nodes) | Color | ext | - | - | no data | low | low | G (probably out of scope) |
| Multicam, proxy, transcript-based editing (Descript) | Workflow | ext | - | - | no data | low | low | G (probably out of scope) |
| Compound clips/nesting, OFX plugins | Workflow | ext | - | - | no data | low | low | G (probably out of scope) |
| Fusion/nodes, 3D, Fairlight, HDR | Post | ext | - | - | no data | low | low | G (probably out of scope) |

Method note: where the table says "s?" and "?", the source was not opened (many pages block bots or are SPAs), and the cell should not be treated as fact.

## Specifications of the target platforms

Only what is marked OFFICIAL was verified in an official source; the rest is secondary. Accessed 2026-11-14. Much official information about organic Instagram/TikTok was not found.

### Instagram Reels
- OFFICIAL (Meta, Ads Guide, via search): design resolution 1440x2560, 9:16; recommended safe zone: ~14% top, ~35% bottom, ~6% on each side free of text/logo (this is for ads, not necessarily organic). https://www.facebook.com/business/ads-guide/update/image/instagram-reels
- Secondary: 1080x1920 9:16; MP4/MOV, H.264 + AAC; 30 fps is safe; 4 GB (ads value); duration: sources diverge (90 s to 20 min); avoid HEVC. E.g.: https://argil.ai/blog/instagram-reel-size-e350f . Current maximum organic duration: NOT CONFIRMED.
- Stories/Reels safe zone page: https://www.facebook.com/business/help/980593475366490/ (content not retrieved).

### TikTok
- OFFICIAL (non-Spark ads): 9:16 minimum 540x960 (1080x1920 recommended by third parties), 16:9 and 1:1 also; .mp4/.mov/.mpeg/.3gp/.avi; up to 10 min; up to 500 MB; bitrate >= 516 kbps; the safe zone depends on the caption and is provided as a template file (no public percentages on the page). https://ads.tiktok.com/help/article/video-ads-specifications
- OFFICIAL (Content Posting API): file up to 4 GB; all creators can post 3 min, some 5 or 10 min. https://developers.tiktok.com/docs/en/content-posting-api-media-transfer-guide (via search). Share Video API: <= 50 MB, 3-60 s, MP4.
- Organic in the app/web (current size/duration limit): not confirmed in an official source; guides cite 287 MB iOS/72 MB Android (probably outdated).

### Kwai
- NO official source FOUND. There is only a third-party blog (2024) saying 9:16 for ads. https://vidquickly.blogspot.com/2024/10/kwai-video-specifications-for-ads.html . Duration, bitrate, size, safe zone: UNKNOWN. Suggested action: open the Kwai for Business panel or the creators app. Provisionally treat it as TikTok (INFERENCE, no basis).

### X
- OFFICIAL (docs.x.com): post: 0.5 s to 20 min and up to 8 GB (standard); Premium/verified up to 125 min and 16 GB; H.264 High Profile, 30 or 60 fps; recommended 720x1280 vertical (Premium: 1080p with 1080p playback, others 720p); video bitrate >= 5000 kbps, audio 128 kbps; AAC-LC; required: <= 60 fps, dimensions 32x32 to 1280x1024, aspect ratio 1:3 to 3:1, square pixels, YUV 4:2:0, no open GOP, progressive. The container is not specified on this page. https://docs.x.com/x-api/media/quickstart/best-practices
- Conflict: another version of the same documentation says 512 MB and 140 s; the version above is the one that opened. X's UI safe zone: not found.

### YouTube Shorts
- OFFICIAL: up to 3 minutes; maximum resolution 1080p; vertical. https://support.google.com/youtube/answer/10059070
- Secondary: classification as a Short requires a square or vertical aspect ratio and <= 3 min; 1080x1920 recommended; size: sources diverge. Safe zone: not found in an official source. Music limits (60/90 s) not verified.

### Cautious synthesis (OPINION, for preset decisions)
- Common preset: MP4, H.264 + AAC, 9:16, 1080x1920, 30 fps (60 optional), and conservative text margins based on the Meta template (top ~14%, bottom ~35%, sides ~6%) until the TikTok/Kwai/Shorts/X templates are obtained.
