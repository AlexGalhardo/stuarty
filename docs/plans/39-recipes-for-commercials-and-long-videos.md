# 39: Recipes for commercials and long videos

## Goal

After the short video works end to end, a creator's agent also knows how to make a product commercial and how to edit a long talking video: two more kinds of production, delivered as skills and templates rather than as new machinery.

## Context and related decisions

- ADR-0040, ADR-0031, ADR-0042.
- Author decision D-071. PRD-004 RF-389.
- Research note `web/05-rotina-criadores`.
- Absorbs the earlier plan on recipes for product promos and authorial videos.

## Scope

- Modules: `web/resources/skills/` (new built-in skills), `web/resources/templates/` (timeline and image templates, embedded like the skills), `core/project/` (briefing question sets per production kind), `licenses/`, SDD-018, the user documentation source for the two recipes.
- Skills: `product-commercial` (hook, problem, product shots, offer, call to action; variants of 15 and 30 seconds; versions per aspect ratio), `long-talking-video` (remove silences and retakes, chapters, captions, cut-away images, a short extracted from the long one), and the copy skills they need adapted from the adopted marketing set.
- Templates: two commercial timelines with slots, one long-video opening, matching thumbnail templates.

## Out of scope

- New commands or features: a recipe uses what exists. A gap becomes a feedback entry.
- Other production kinds (podcast, course, live cut-downs).

## Dependencies

33, 38.

## Technical design

- **A recipe is a skill plus templates plus briefing questions**, nothing else. The skill names real operations (the registry check of epic 16 applies) and, where it needs motion beyond the editor, hands off through `motion-with-an-external-engine` (epic 33).
- **Briefing per production kind**: `core/project` gains a `kind` for a production (`short`, `commercial`, `long`) that selects the question set; the questions are data.
- **Templates** are embedded, installed as built-in, parametrised with slots and the brand kit, and each has a golden export so a change in the core that breaks a template is caught.
- **Each recipe is proven like the pilot**: a scripted run with fake provider and connector that ends in an exported file per aspect ratio, added to the full verification.
- **Adapted material** carries its source and licence, with the notice in `licenses/`.

## Tasks

- [x] 1. Amend SDD-018 (the two recipes, the production kind) and write the briefing question sets. Verify: `task lint:md` exits 0 and `go test ./core/project/... -run TestBriefingKinds` passes.
- [x] 2. Build the commercial templates and their thumbnail templates with golden exports. Verify: `go test ./web/resources/templates/...` passes.
- [x] 3. Write `product-commercial` and its copy skills. Verify: `go test ./web/resources/skills/...` passes (lint, registry check, attribution).
- [x] 4. Write the scripted commercial run. Verify: `bash scripts/recipe-commercial.sh` (new) exits 0 with three exported aspect ratios.
- [x] 5. Build the long-video opening template; write `long-talking-video`. Verify: the same two test commands pass.
- [ ] 6. Write the scripted long-video run on a five-minute fixture, including the extracted short. Verify: `bash scripts/recipe-long-video.sh` (new) exits 0.
- [ ] 7. Write the two user guides in English and in Portuguese. Verify: `task lint:md` exits 0.
- [ ] 8. Add both scripts to `verify.sh --full`. Verify: the full verification runs them.

## Test strategy

- Static: skill lint and registry check.
- Golden exports for each template.
- Scripted runs of both recipes with no network.

## Acceptance criteria

1. A fresh workspace lists the two recipes as built-in skills and their templates.
2. `bash scripts/recipe-commercial.sh` produces a commercial in 9:16, 1:1 and 16:9 from one brief.
3. `bash scripts/recipe-long-video.sh` produces an edited long video with chapters and captions, and a short extracted from it.
4. Every operation the skills name exists in the registry.
5. Adapted skills carry source and licence, and `task notices:check` exits 0.

## Risks and mitigation

- **A recipe needs a feature that does not exist.** Mitigation: file it; the recipe documents the manual step instead of inventing a command. **STOP** if the gap makes the recipe pointless.

## Size

M

## Hand-off for review

The diff of `epic/39-recipes-for-commercials-and-long-videos`; the exported files' verification outputs; the gaps filed.
