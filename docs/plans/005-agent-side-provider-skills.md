# Plan 005: Skills for agent-side generation: audio, images and video

This plan answers how an agent generates the assets a video needs (voice, music, sound effects, images, short video clips) with the providers' own tools, records their origin, licence and cost, and hands them to any engine as files, while the editor stays without any outbound capability.

- Owner: estagiario-1 (research), senior-d (skills)
- Status: TODO
- Effort: M
- Depends on: plan 003
- Written against: `8787714a`

## 1. Why

Author decision D-041: the agents integrate with the providers natively; the editor only edits files that are already there. The value of this plan is the know-how: which tool to call for which asset, how to keep cost bounded, and how to keep the rights traceable. No code is added to the product.

## 2. Current state (drift check)

```text
git grep -n "PurposeHelperDownload\|PurposeTranscribeAPI\|PurposeDesktopUpdate" internal/outbound/purposes.go   # three purposes, unchanged by this plan
ls studio/templates/production/assets/SOURCES.md     # exists (plan 003)
```

If this plan would need a new purpose in `internal/outbound/`, it is wrong: stop.

## 3. Tasks

### Step 1: Research the providers' agent tooling (research)

- Owner: estagiario-1. Output is a research note in the maintainers' private notes.
- For ElevenLabs (voice, sound effects, music), Epidemic Sound, OpenAI image generation, Higgsfield, and one open alternative per category: whether an official MCP server or CLI exists (name, install command, exact stable version), authentication method, the operations relevant to short videos, output formats, pricing unit, the licence of generated or downloaded assets (commercial use, attribution, platform restrictions such as Content ID claims). Each fact with its source URL and date; "not found" otherwise.
- Acceptance:
  - [ ] one table per provider, every cell sourced or "not found".

### Step 2: Write the `gen-audio` skill

- File: `studio/skills/gen-audio/SKILL.md`.
- Content: when to use voice-over, music or a sound effect; which provider tool to call for each (from the research note, with the exact tool names); the parameters that matter for short video (duration, loudness, stems); where the file goes (`assets/`, kebab-case name); the line to add to `assets/SOURCES.md`; how to bring it into the editor (`media_import`, then `audio.add`, `audio.duck`) or into an external engine (a file path in the scene).
- Cost rule: before any paid call, write the estimated cost in `brief.md`; stop and ask the user when the video's total would pass the budget written in the brief (default 2 USD).
- Commit: `feat(studio): add the gen-audio skill`.
- Acceptance:
  - [ ] `go run ./internal/tools/skillcheck studio` exits 0.
  - [ ] every tool name in the skill is in the research note with a source.

### Step 3: Write the `gen-visuals` skill

- File: `studio/skills/gen-visuals/SKILL.md`.
- Same structure as `gen-audio` (step 2), for still images (backgrounds, product shots, thumbnails) and short generated video clips (B-roll), with the target sizes from `studio/references/platforms.md` (1080x1920 for vertical) and a rule to generate at the final aspect ratio rather than crop later.
- Commit: `feat(studio): add the gen-visuals skill`.
- Acceptance: as step 2.

### Step 4: Rights and cost ledger in the production folder

- Files: `studio/templates/production/assets/SOURCES.md` (columns: file, origin (provider and tool, or URL, or "own"), licence, attribution required, cost, date), `studio/skills/studio-new/SKILL.md` and `studio/skills/retro/SKILL.md` (sum the costs into `learnings.md`; promote a `provider` fact when a provider failed or was expensive).
- Commit: `feat(studio): track asset rights and cost per production`.
- Acceptance:
  - [ ] `go run ./internal/tools/skillcheck studio` exits 0.
  - [ ] a hand run of `retro` on a folder with two priced assets writes their sum to `learnings.md`.

## 4. Out of scope

- Storing or reading provider keys anywhere in the repository or the product; keys stay in the agent's own configuration of each MCP server.
- Any change to ADR-0020, SDD-007 or `internal/outbound/`.

## 5. STOP conditions

- A provider has no MCP server or CLI and only an HTTP API: the skill may show a `curl` call that the agent runs, never product code. If even that needs a secret in a file of the repository, stop.
- A provider's licence forbids use in social video: record it in the skill as "do not use for" and continue.

## 6. Maintenance

Provider tools change fast. Each skill names the tool versions and the date they were checked; a pilot that fails because of a tool change updates the skill in the same session and writes a `provider` fact.
