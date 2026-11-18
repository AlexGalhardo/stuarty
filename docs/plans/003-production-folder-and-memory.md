# Plan 003: Production folder and learnings memory as skills

This plan answers how an agent creates the production folder of a video, reads past learnings before planning, and records new learnings after delivery, using only skills (no product code).

- Owner: senior-d (skills); estagiario-1 (the dry run)
- Status: TODO
- Effort: S-M
- Depends on: plan 002 (plugin, skill lint)
- Written against: `8787714a`

## 1. Why

Reuse is the multiplier of the studio: the tenth meme should cost less than the first because the agent already knows the caption style that worked, the platform limits, and the mistakes to avoid. Memory in plain Markdown keeps it readable by the author, diffable, and engine-independent (author decision D-043).

## 2. Current state (drift check)

```text
go run ./internal/tools/skillcheck studio    # exits 0 (plan 002 done)
ls studio/skills                             # video-studio  vukem-video
```

If plan 002 is not done, stop.

## 3. Conventions to match

- Skill format and body sections: SDD-014 section 3. The router `studio/skills/video-studio/SKILL.md` is the exemplar to copy.
- Layout of the production folder and of memory: SDD-014 sections 4 and 6. Do not invent fields; when one is missing, SDD-014 changes first.
- Skills give shell commands for both POSIX shells and PowerShell when they differ (the author works on Windows).

## 4. Tasks

### Step 1: Write the `studio-new` skill

- File: `studio/skills/studio-new/SKILL.md`, plus `studio/templates/production/brief.md`, `studio/templates/production/learnings.md`, `studio/templates/production/assets/SOURCES.md` (empty tables with their headers).
- Steps the skill gives the agent:
  1. resolve the root (`VUKEM_STUDIO_HOME`, default `~/vukem-studio`), and the folder name `<yyyy-mm-dd>-<slug>` (slug: kebab-case ASCII, at most 40 characters; a numeric suffix `-2`, `-3` when the name exists);
  2. create the folders of SDD-014 section 4 and copy the three templates;
  3. fill `brief.md`: request (verbatim), use case, platform(s), target duration, aspect ratio, engine chosen and why, learnings recalled (filled by `recall`);
  4. print the absolute path.
- Commit: `feat(studio): add the studio-new skill and production templates`.
- Acceptance:
  - [ ] `go run ./internal/tools/skillcheck studio` exits 0.
  - [ ] following the skill by hand in a scratch directory creates exactly the SDD-014 layout (paste `tree` or `Get-ChildItem -Recurse` output).

### Step 2: Write the `recall` and `retro` skills

- Files: `studio/skills/recall/SKILL.md`, `studio/skills/retro/SKILL.md`, `studio/templates/memory/MEMORY.md`, `studio/templates/memory/fact.md`.
- `recall`: read `<root>/memory/MEMORY.md` (create it from the template when absent); select the facts whose `use_cases`, `tags` or platform match the brief; open only those files; write the chosen fact names into `brief.md` under "Learnings recalled". Budget: at most 15 facts read per video.
- `retro`: after delivery, append to the production folder's `learnings.md` (what worked, what failed, numbers: duration, renders needed, cost); then, for each item worth reusing, check `MEMORY.md` for an existing fact and update it instead of duplicating; write a new fact file only when none covers it; add one index line per new fact (`- [Title](file.md): hook`). A fact that turned out wrong is deleted and its index line removed.
- Fact types (SDD-014 section 6): `style`, `platform`, `engine`, `provider`, `mistake`.
- Commit: `feat(studio): add the recall and retro memory skills`.
- Acceptance:
  - [ ] `go run ./internal/tools/skillcheck studio` exits 0.
  - [ ] a hand run of `retro` twice with the same learning leaves one fact file and one index line (paste both files).

### Step 3: Dry run of the three skills by a Haiku-class agent

- Owner: estagiario-1 (runs and reports; writes no skill).
- Setup: a scratch `VUKEM_STUDIO_HOME` under the session scratch directory; the plugin installed from the repository path; no other context.
- Prompt to give the agent: "Plan a 20-second vertical meme video about a car chase. Create its production folder, recall learnings, and after pretending it was delivered, record two learnings."
- Report: the folder tree, `brief.md`, `learnings.md`, `MEMORY.md`, each fact file, and every step where the agent hesitated or deviated from the skill.
- Acceptance:
  - [ ] the tree matches SDD-014 section 4;
  - [ ] `MEMORY.md` has two index lines and two fact files with valid frontmatter;
  - [ ] each deviation becomes a fix to the skill text in a follow-up commit by senior-d, or a recorded reason why not.

## 5. Out of scope

- Any search index (SQLite or other); add one only when `MEMORY.md` passes about 200 lines.
- Any change to the editor.

## 6. STOP conditions

- The agent needs a field SDD-014 does not define: stop; SDD-014 changes first.

## 7. Maintenance

When memory grows, the first pressure point is the 15-fact read budget of `recall`; raise it in the skill only with a measured reason written in `learnings.md`.
