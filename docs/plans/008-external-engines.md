# Plan 008: HyperFrames and Remotion as agent-side engines, and the hand-off into the editor

This plan answers how an agent makes a video with HyperFrames or Remotion alone when the request allows it, and how a render from either engine becomes an asset in the editor when footage must be added or the result adjusted. When it is delivered, the plugin has one skill and one starter scene per engine, a hand-off skill, and a pilot that made a text-only meme card without opening the editor.

- Owner: senior-d (skills and templates), orchestrator (approval gate, pilot)
- Effort: M
- Depends on: plan 003; the author's approval of the pinned engine versions (step 1)
- Origin: author decisions D-041 and D-042
- Status: TODO
- Written against: `8787714a`

## 1. Why

Kinetic text, animated cards and data-driven compositions are faster to write as HTML or React than to build with timeline commands. The author wants the simplest engine to win; the editor stays the place for footage and for fine adjustments. The engines run on the user's machine, outside the product, so ADR-0003 (one plan, truth on pause) is not affected: an external render enters the editor as a media file like any other.

## 2. Facts recorded on 2026-11-18 (research note `engines-2026-11`)

- HyperFrames (HeyGen): Apache-2.0; scenes in HTML; rendered with a headless browser and FFmpeg (`npx hyperframes render`); needs Node 22+ and FFmpeg; flags not fully documented.
- Remotion: source-available licence, not open source: free for individuals, non-profits and companies of up to three people; companies above that need a paid licence; cloud rendering for companies needs a separate licence. CLI `npx remotion render`.
- MLT `melt` (LGPL-2.1) and FilmCraft (Apache-2.0, repository created in 2026-09, headless CLI announced but not documented) were considered and are not part of this plan.

Re-verify these facts in step 1; they were gathered by an agent and are claims until checked.

## 3. Steps

### Step 1: Pin the engine versions and get the author's approval

- Owner: orchestrator. Read each engine's release page; choose the latest stable release (no `rc`, `beta`, `canary`); write the versions, licences and Node requirement into a short table in `studio/references/engines.md` and ask the author to approve them (AGENTS.md rule 9 applies to templates the plugin ships).
- Acceptance:
  - [ ] the author's approval is recorded as an author decision; `task lint:md` clean.

### Step 2: Write the `engine-hyperframes` skill and its starter scene

- Owner: senior-d. Files: `studio/skills/engine-hyperframes/SKILL.md`, `studio/templates/hyperframes/meme-card/` (a 1080x1920, 8-second scene: a caption that pops in word by word over a background image or colour, parameters in one JSON file), with a `package.json` pinning the approved exact version.
- The skill covers: copy the template into `<production>/engine/hyperframes/`, edit the parameters, render to `<production>/out/`, check with `ffprobe`, write `<file>.render.json` (SDD-014 section 5).
- Commit: `feat(studio): add the HyperFrames engine skill and meme card scene`.
- Acceptance:
  - [ ] `go run ./internal/tools/skillcheck studio` exits 0.
  - [ ] rendering the template unchanged produces an MP4 of 1080x1920 and 8 s ± 1 frame (paste `ffprobe`).

### Step 3: Write the `engine-remotion` skill and its starter composition

- Owner: senior-d. Same structure as step 2 under `studio/skills/engine-remotion/` and `studio/templates/remotion/meme-card/`.
- The skill's first step shows the licence summary and asks the user to confirm eligibility once; the answer is saved as a memory fact (`type: engine`) so it is not asked again.
- Commit: `feat(studio): add the Remotion engine skill and meme card composition`.
- Acceptance: as step 2.

### Step 4: Write the `engine-handoff` skill

- Owner: senior-d. File: `studio/skills/engine-handoff/SKILL.md`.
- Content: when a render needs footage, captions from speech, an audio mix or a fine adjustment, import `out/<file>` with `media_import` into the editor project (create one at the render's size and fps from `<file>.render.json`), place it on track 1, continue with the editor skill; record the project id in `engine/vukem/`.
- Commit: `feat(studio): add the engine hand-off skill`.
- Acceptance:
  - [ ] `go run ./internal/tools/skillcheck studio` exits 0.

### Step 5: Pilot: a text-only meme without the editor (the delivery of this plan)

- Owner: orchestrator, with a Haiku-class model and only the plugin.
- Request: "Make an 8-second vertical meme card: 'When the GTA 6 trailer drops at 3 a.m.' with a dramatic zoom, for Stories."
- Acceptance:
  - [ ] the agent chose HyperFrames (or Remotion, when the licence fact says yes) and wrote why in `brief.md`;
  - [ ] the output passes `ffprobe` checks (1080x1920, 8 s ± 1 frame) and has its `render.json`;
  - [ ] a follow-up "add this gameplay clip under the card" goes through `engine-handoff` into the editor and exports (paste the job result).

## 4. Out of scope

- A render-engine interface inside `engine/` or `service/` (author decision D-041; ADR-0003 stands).
- MLT, FilmCraft or other engines; add a plan when one is wanted.

## 5. STOP conditions

- The author does not approve a version, or an engine has no stable release: ship only the other engine's skill and record why.
- A template needs a dependency beyond the engine itself: stop and ask for approval.

## 6. Maintenance

Engine upgrades change only `studio/references/engines.md`, the template `package.json` and the skill, together, after re-rendering the starter scene.
