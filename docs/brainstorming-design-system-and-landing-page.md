# Brainstorming: the design system and the landing page

- Status: Working document (2026-11-20). The author chooses one of the three landing versions; this document records how they were conceived.
- Related: [design-system.md](design-system.md) (the system itself, section 11 is the brand refresh this document proposes), [SDD-010](sdds/010-documentation-site.md) (the documentation site), [ADR-0031](adrs/0031-stuarty-product-scope-and-domains.md) (what Stuarty is).

## 1. The brief

The author asked for a modern, remarkable landing page for Stuarty, in three versions to choose from, with:

- the main features and the problem Stuarty solves: one place to create, edit, manage and publish content, instead of five tools;
- the product in action (short clips, GIFs or images; placeholders where the assets do not exist yet);
- real examples of driving the product through an agent, the CLI, the HTTP API and MCP;
- the navbar kept in structure and behaviour (the mark, the name, search on Ctrl+K, the links), restyled to fit;
- three.js for 3D and animation, a background that moves in real time, smooth and elegant transitions where they help.

The three versions: a **viewport** version (no scrollbar; scrolling moves between scenes), a **classic** long-scroll landing down to the footer, and a **creative** version meant to surprise.

Before building: research of references (section 3), a complete design system for the landing, the documentation and the product (web and desktop), minimal, modern and elegant, still on Base UI and Tailwind CSS v4 ([design-system.md](design-system.md) section 11), and this brainstorming.

Decisions taken with the author before starting (interview of 2026-11-20):

| Question | Decision |
| --- | --- |
| When | Research, brainstorming and the design system start at once; the three pages are built when engineers free up, without delaying the work of the day |
| Where the versions live | Three new routes of the documentation site, `/landing/viewport`, `/landing/classic` and `/landing/creative`; the current home stays until the author chooses; the chosen one becomes the home in one commit |
| The visual identity | Redefined from the official icon (the clapperboard with eyes): indigo, lilac and white. Applied to the landing and the documentation, and to the product only through its tokens (colours, radii, shadows, type), with no screen redesigned now |
| A new dependency | three.js, at an exact stable version, in the documentation site only (the author asked for it by name) |

## 2. What Stuarty has to say

**The problem.** An independent creator today writes the script in one app, cuts the video in another, makes the thumbnail in a third, schedules the post in a fourth and pays a fifth to "use AI" on top. Files, tokens and credentials end up in five clouds. Agents cannot drive any of it end to end.

**The answer, in one line.** Stuarty: create, edit, manage and publish your content in one place, on your own machine, with AI agents as first-class users.

**What makes it different**, in the order a visitor should meet it:

1. **All in one:** video editor, image editor, writing and blog, projects and library, a publishing calendar with an approval queue.
2. **Agents are first-class users:** everything a person does in the UI is possible through the CLI, the HTTP API and MCP, with tested parity.
3. **Yours:** open source (MIT), self-hosted, web and desktop; files, tokens and credentials stay on the user's machine; no telemetry; outbound only to what the user connected; publishing waits for a person.
4. **Bring your own access:** your own AI keys, Sign in with ChatGPT, OpenRouter or your local agent; your own social developer apps.

**Real examples for the "four ways to drive it" block** (taken from the product, `web/contract/testdata/surface.txt` and the agent docs):

- Agent (MCP): an agent asked "cut a 30 s vertical teaser from talk.mp4 with captions" calls the editor's tools by intent, and every change appears in the timeline, attributed to the agent.
- CLI: `stuarty project:create`, `stuarty media:import`, `stuarty edit`, `stuarty export --wait`, each with `--json` for scripts.
- HTTP API: the same operations as REST routes with an event stream (`/api/v1/...`), with a token.
- MCP: `tools/list` returns tools grouped by intent with a domain (ADR-0043), within a fixed token budget.

The exact commands are copied from the build when the pages are written, never from memory.

## 3. References and research

Research of 2026-11-20. Only what was read on the page is stated as fact; several sources refused automated reading (npm, Codrops, the Awwwards listings, Radix Colors, Cursor, Runway, Arc, Bruno Simon), so they are listed as sources to visit by hand, not as findings.

### 3.1 References read

| Reference | What it does well | Technique worth borrowing |
| --- | --- | --- |
| [Linear](https://linear.app/) | "A new species of product tool": the hero is the product itself (cards, a board, issues, a code review), with customer quotes | simulated product screens instead of screenshots, sharp and themable |
| [Raycast](https://raycast.com/) | a keyboard-first pitch with a keyboard graphic, a simulated AI agent chat that shows each tool step, category tabs with preview cards | the closest model for "four ways to drive it": a mock agent conversation and tabs |
| [Lusion](https://www.lusion.co/) | 3D visual storytelling, with an explicit "scroll to explore" cue and reel controls | scroll-led scenes with a clear cue; the stack is not stated on the page |
| [Vercel Geist](https://vercel.com/geist/introduction) | one family for sans and mono, a design system published as a site | Geist is already the product's font (design-system.md section 2.7) |
| [Awwwards, Sites of the Day](https://www.awwwards.com/websites/%23E4FF3F/) | current winners include EverSwap (by Lusion), Radian, Vaulk and Novu | the listing only; each site to be visited by hand |

### 3.2 Facts that constrain the build

- three.js: the current stable release is r187, `0.187.0` in its `package.json` ([releases](https://github.com/mrdoob/three.js/releases), [package.json](https://github.com/mrdoob/three.js/blob/dev/package.json)); r187 compiles pipelines in parallel in `compileAsync()`, tree-shakes better and removes what r177 deprecated. The exact version is confirmed with the package manager before it is pinned.
- Reduced motion: `@media (prefers-reduced-motion: reduce)`; scaling and panning are the main vestibular triggers ([MDN](https://developer.mozilla.org/en-US/docs/Web/CSS/@media/prefers-reduced-motion)), so the camera moves of the viewport and creative versions are the first thing to turn off.
- Base UI documents version 1.9.0 ([quick start](https://base-ui.com/react/overview/quick-start)); the product already uses it.
- Tailwind CSS v4 keeps tokens in CSS through `@theme`, each one a custom property, with an OKLCH palette, spacing from one `--spacing` variable and gradients interpolated in OKLab ([Tailwind CSS v4](https://tailwindcss.com/blog/tailwindcss-v4)): the brand gradient of design-system.md section 11 interpolates without the grey middle of sRGB.

### 3.3 Practice applied (recommendations, not sourced)

- A full-viewport site without a scrollbar: a fixed stage, wheel and touch mapped to a progress value that snaps to scenes, arrows and Page Up/Down, Tab staying on real focusable elements, focus moved to the heading of the active scene, a crossfade (or the classic scroll) under reduced motion, and the plain long page when WebGL fails.
- A classic landing for a tool, in the order Linear and Raycast follow: hero with the real call to action and a product mock, the problem, the features, a demo, agents/CLI/API/MCP in code tabs, open source and self-hosting, a last call to action, the footer.
- three.js on a landing: pixel ratio capped at 1.5 to 2, the loop stopped when the canvas is off screen (IntersectionObserver) or the tab is hidden (visibilitychange), rendering on demand where nothing moves, three loaded only in a client island with a static HTML fallback.

### 3.4 Sources to visit by hand

[Awwwards](https://www.awwwards.com/), [Behance](https://www.behance.net/), [Pinterest](https://br.pinterest.com/), [Codrops hub](https://tympanus.net/codrops/hub/), [Lusion](https://lusion.co/), [Active Theory](https://activetheory.net/), [Bruno Simon](https://bruno-simon.com/), [Spline](https://spline.design/), [SiteInspire](https://www.siteinspire.com/), [CSS Design Awards](https://www.cssdesignawards.com/).

## 4. The shared foundation of the three versions

- **The mascot is the mark.** The icon is a clapperboard with two white eyes. In 3D it is a rounded indigo box with a lilac clapper bar on top and two capsule eyes: built from three.js primitives (no model file), so it loads instantly and stays sharp at any size. The clapper bar can snap shut on a scene change ("action!"); the eyes can blink and follow the pointer. It gives every version the same character.
- **The living background.** A full-screen fragment shader: a slow flow of indigo and lilac light on near-black (or on white in the light theme), driven by a noise field, with the scroll velocity and the pointer adding a gentle disturbance. One draw call, a capped pixel ratio, paused when the tab is hidden.
- **Restraint.** The product's principle P3 (minimal) stays: one idea per screen, generous space, at most two hues plus neutral, no glass or glow effects for their own sake. The motion is the decoration.
- **Respect for the visitor.** `prefers-reduced-motion` replaces every animation with a crossfade and stops the shader on its first frame; everything works with the keyboard; a WebGL failure falls back to a static gradient and a still image of the mascot; the text is real HTML, readable by search engines and screen readers.
- **The navbar.** Same structure and behaviour as the documentation (mark, name, search on Ctrl+K, links, theme toggle), on a translucent bar over the scene, restyled with the new tokens.

## 5. The three concepts

### 5.1 Viewport: "The studio in six takes"

A full-screen experience without a scrollbar. Each wheel step, swipe, arrow or Page Down moves to the next **take**, and the clapper bar of the mascot snaps shut on every cut, like on set. The camera dollies through one continuous 3D space; the text of each take fades and rises in.

| Take | Message | Scene |
| --- | --- | --- |
| 1 | Create, edit, manage and publish in one place | the mascot in the centre, the living background |
| 2 | Edit video | floating timeline strips (instanced rounded boxes) slide under a preview plane showing a clip placeholder |
| 3 | Images, writing, the library | the planes rearrange into an image editor layer stack and a document page |
| 4 | Publish with a person in the loop | posts fly to a calendar grid; one waits at an "Approve" gate |
| 5 | Agents drive it too | a terminal plane types a real CLI session; MCP calls appear as light trails into the timeline |
| 6 | Open source, on your machine | the scene pulls back to the mascot; download and "Read the docs" |

A progress rail on the right (six dots, clickable, with names), keyboard support (arrows, Page Up/Down, Home/End), the URL hash per take, and a "skip to the docs" link first in the tab order. With reduced motion, takes crossfade.

### 5.2 Classic: "Calm long scroll"

A conventional long page, built to explain and to convert, with the 3D kept where it earns its place:

1. **Hero:** the headline and two actions over the living background; on the right, the product window (a screenshot placeholder on a three.js plane) tilted in perspective, following the pointer slightly.
2. **The problem:** five scattered tool icons drift and merge into the mascot as the visitor scrolls (scroll-linked).
3. **Features:** four cards (create, edit, manage, publish), each with a short looping clip placeholder.
4. **In action:** a large video placeholder with chapters (the first complete flow: briefing to published short).
5. **Four ways to drive it:** tabs Agent, CLI, HTTP API, MCP with real snippets, a copy button and a typed terminal.
6. **Trust:** open source, self-hosted, no telemetry, publishing waits for a person, credentials in a local vault.
7. **Call to action** and the **footer** of the documentation.

### 5.3 Creative: "The page is a timeline"

The surprise: the landing **is a video editor**. The page scrolls horizontally like a timeline; the vertical wheel moves a **playhead** across tracks of clips rendered in 3D perspective (instanced boxes in the brand colours, receding into the distance). Above them a "preview" panel shows what the playhead is over: each clip is a feature, and passing over it "renders" its story in the preview. An **agent cursor** (the mascot in miniature) edits the page live: it trims a clip, adds a caption, and its MCP calls scroll in a small log, attributed like in the product. At the end the playhead reaches "Export", and the whole timeline collapses into the mascot. A command line at the bottom accepts a few real commands (`help`, `features`, `install`) as an easter egg.

It tells the product's story in the product's own language, and it shows agent attribution, the core of Stuarty, without a word of explanation.

## 6. The design system direction

The full system is [design-system.md](design-system.md); section 11 there is the brand refresh this brainstorming led to. In short:

- **Brand colours from the icon:** indigo `#3534CA` (`oklch(0.442 0.222 272.9)`), lilac `#8078FB` (`oklch(0.645 0.189 282.5)`), white. Indigo carries white text at 8.42:1 on light surfaces; on dark surfaces the brand switches to lilac, which carries near-black text at 5.65:1 (indigo on near-black is only 2.35:1).
- **Two registers.** The product keeps its quiet register (principle P2: content first, neutral chrome, colour for state); the brand appears in the mark, links, the active navigation item and the primary action of the dashboard pages, never inside the editor's canvas or timeline. The landing and the documentation home use the **expressive register**: brand gradients, 3D, larger type, more motion, under the same tokens.
- **Type:** Geist (already bundled) for everything; display sizes for the landing only (`text-5xl` to `text-8xl`, tight tracking, weight 600).
- **Motion:** the product keeps its two kinds of motion; the landing adds scene transitions (500 to 900 ms, `cubic-bezier(0.22, 1, 0.36, 1)`), all disabled by reduced motion.

## 7. Open points for the author

- Which version becomes the home (or which parts of each to merge).
- Real product clips and screenshots to replace the placeholders.
- The logo lockup (icon and name), being generated by the author, which replaces the mark and the name in the navbar when it arrives.
