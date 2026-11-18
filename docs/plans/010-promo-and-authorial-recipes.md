# Plan 010: Recipes for product promos and authorial videos

This plan answers how the second and third use cases (promotional videos for commercial and digital products, and authorial videos on the author's topics) get the same end-to-end recipe the meme cut has. When it is delivered, two skills exist, each with a pilot report, and their learnings are in memory.

- Owner: senior-d (skills), orchestrator (pilots)
- Effort: M
- Depends on: plans 004, 005, 006 and 008
- Origin: author decision D-041; the use cases of the author's brief of 2026-11-18
- Status: TODO
- Written against: `8787714a`

## 1. Why

Promos and authorial videos reuse everything the earlier plans build (production folder, memory, clips, generated voice and images, external engines, the editor) but in a different order: a script comes first, then voice, then visuals timed to the voice. Writing that order down once is what makes the next video cheap.

## 2. Current state (drift check)

```text
ls studio/skills   # meme-cut, gen-audio, gen-visuals, fetch-clip, clip-library, engine-hyperframes, engine-remotion, engine-handoff present
```

If any is missing, stop: its plan is not delivered.

## 3. Steps

### Step 1: Write the `product-promo` skill

- Owner: senior-d. File: `studio/skills/product-promo/SKILL.md`.
- Flow: brief (product, audience, offer, call to action, platform, 15/30/60 s) → script with a hook in the first 2 s, one benefit per beat, the call to action at the end → voice-over (`gen-audio`) → visuals per beat: product shots or generated images (`gen-visuals`), screen recordings, kinetic text in HyperFrames for price and offer cards → assemble in the editor timed to the voice (captions from the voice-over transcript, music ducked under speech, the brand kit for colours and logo) → export per platform → `retro`.
- Includes a claims rule: no claim the brief does not support; prices and offers copied verbatim from the brief.
- Commit: `feat(studio): add the product-promo recipe`.
- Acceptance:
  - [ ] `go run ./internal/tools/skillcheck studio` exits 0.

### Step 2: Write the `authorial-video` skill

- Owner: senior-d. File: `studio/skills/authorial-video/SKILL.md`.
- Flow: topic and angle → outline and script in the author's voice (style facts from memory) → narration (the author's own recording when given, otherwise `gen-audio`) → B-roll from clips (`fetch-clip`, `clip-library`) and generated visuals → editor assembly with captions → export → `retro`.
- Commit: `feat(studio): add the authorial-video recipe`.
- Acceptance:
  - [ ] `go run ./internal/tools/skillcheck studio` exits 0.

### Step 3: Pilot both recipes (the delivery of this plan)

- Owner: orchestrator, with a Sonnet-class model first, then a Haiku-class model on the same briefs.
- Briefs: a 30-second vertical promo for a digital product the author names; a 60-second authorial video on a topic the author names.
- Acceptance:
  - [ ] each run produces an export that passes the `ffprobe` checks of `studio/references/platforms.md`, a complete production folder and at least two promoted learnings;
  - [ ] the report compares the two models (steps completed, retries, cost from `SOURCES.md`) and lists every skill fix made.

## 4. Out of scope

- Publishing to the platforms (the editor has no outbound capability; agents may publish with their own tools, outside this plan).

## 5. STOP conditions

- A pilot needs a paid provider call above the brief's budget: stop and ask the author.

## 6. Maintenance

After every tenth delivered video of a use case, re-read its recipe against the memory facts tagged with that use case and fold the stable ones into the skill.
