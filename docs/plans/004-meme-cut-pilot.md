# Plan 004: Pilot: a vertical meme cut from game footage, end to end

This plan answers what the first complete studio flow is, and how we prove that a cheap model can run it: from a gameplay clip and a one-line request to a 9:16 MP4 ready to post on Instagram Reels, TikTok, YouTube Shorts, X and Stories, made with the editor.

- Owner: estagiario-2 (platform research), senior-d (reference and skill), orchestrator (the pilot run)
- Status: TODO
- Effort: M
- Depends on: plan 003
- Written against: `8787714a`

## 1. Why

It is the simplest of the three use cases and needs almost nothing new: the editor already cuts, captions from a transcript, removes silences, changes speed, adds effects, ducks music and normalises loudness. What is missing is the recipe that chains those operations for a meme, the platform limits in one place, and the proof that it works without a human.

## 2. Current state (drift check)

```text
go run ./internal/tools/skillcheck studio          # exits 0; studio-new, recall, retro exist (plan 003)
grep -c "caption.from_transcript\|clip.remove_silences\|audio.duck\|clip.set_speed\|clip.set_effects" docs/sdds/002-command-catalogue.md   # > 0
ls web/app/agentdocs/recipes                       # 1-import-cut-caption.json ... 5-remove-silences.json
```

The editor commands named in this plan must exist in `docs/sdds/002-command-catalogue.md`; if one is missing, stop.

## 3. Tasks

### Step 1: Gather the current platform limits (research)

- Owner: estagiario-2. Output is a research note in the maintainers' private notes, not in the repository.
- For Instagram Reels, Instagram Stories, TikTok, YouTube Shorts and X: aspect ratio, recommended resolution, frame rates accepted, maximum duration, maximum file size, recommended codec and bitrate, audio loudness target if published, safe zones for text (the areas covered by the platform's buttons and captions), caption length limits. Each value with its official source URL and the date read; "not published" when there is none.
- Acceptance:
  - [ ] every cell has a value with a URL, or "not published".

### Step 2: Write `studio/references/platforms.md`

- Owner: senior-d. From the research note: one table per platform, the source URL of each value, and the date. A "defaults" section: 1080x1920, 30 fps, H.264 + AAC, -14 LUFS integrated, text inside the common safe zone.
- Commit: `docs(studio): add the platform limits reference`.
- Acceptance:
  - [ ] `task lint:md` clean; every value has a source link.

### Step 3: Write the `meme-cut` skill

- File: `studio/skills/meme-cut/SKILL.md` (links `../../references/platforms.md`).
- Inputs: one or more gameplay clips (local files), the joke or the moment to cut, the platform(s), optional sound effects or music files already in `assets/`.
- Steps, each naming the editor operation (the `vukem-video` skill has the syntax):
  1. `studio-new`, then `recall` (use case `game-meme`).
  2. Create a 1080x1920 project at 30 fps (or from the starter template "vertical cut with captions" when present); import the clips with `media_import`.
  3. Find the moment: transcribe when there is speech; otherwise use `view --mode sheet` contact sheets to locate it; cut to at most the target duration (default 15 s; hook in the first 2 s).
  4. Framing: `clip.set_framing` to fill 9:16 (crop the centre of action, not letterbox).
  5. Text: the meme caption as a text item at the top safe zone; spoken lines with `caption.from_transcript` and `caption.set_style` (large, outlined, two to four words per line).
  6. Comedy beats: `clip.set_speed` for a freeze or slow-motion beat, `clip.set_effects` for shake or zoom on the punchline, sound effects from `assets/` with `audio.add`, `audio.duck` under speech.
  7. `lint`, then `view` at the hook, the punchline and the last frame; fix what is wrong.
  8. Export MP4; verify with the job `checks` and with `ffprobe` (resolution, fps, duration, loudness within 1 LU of -14); write `out/<file>.render.json` (SDD-014 section 5) with `engine` `vukem-video`.
  9. `retro`.
- Rights section (mandatory): never use the game's soundtrack or licensed music; keep clips short and transformed (commentary, captions, edits); record every asset in `assets/SOURCES.md`; warn that platforms may still remove game footage.
- Commit: `feat(studio): add the meme-cut skill`.
- Acceptance:
  - [ ] `go run ./internal/tools/skillcheck studio` exits 0.
  - [ ] every editor operation named in the skill appears in `docs/sdds/002-command-catalogue.md` (a grep loop over the names; paste the result).

### Step 4: Pilot run with a Haiku-class model (the delivery of this plan)

- Owner: orchestrator. Uses the test media the maintainers already keep (any clip with a car chase or a crash; no copyrighted soundtrack).
- Run: a fresh session of a Haiku-class model with only the plugin, the editor running locally (`vukem-video serve` on a scratch workspace), and the request "Make a 15-second vertical meme from this clip for TikTok and Reels: the moment the car flips, with a funny caption and a sound effect."
- Acceptance:
  - [ ] an MP4 at 1080x1920, 30 fps, at most 15.5 s, integrated loudness -14 ± 1 LUFS (`ffprobe` and `ffmpeg -af ebur128` output pasted);
  - [ ] the production folder matches SDD-014 section 4, with `SOURCES.md` filled and `out/<file>.render.json` present;
  - [ ] at least one learning promoted to memory;
  - [ ] every product problem found becomes a `VV-NNN` entry in the feedback queue, every skill problem a fix commit; the report lists both.

## 4. Out of scope

- Generating the sound effect with a provider (plan 005); the pilot uses a local file.
- Any change to the editor's code.

## 5. STOP conditions

- The pilot needs an editor feature that does not exist (for example a text animation preset): record a `VV-NNN` and finish the pilot without it; do not add the feature inside this plan.

## 6. Maintenance

Platform limits change. `platforms.md` carries the date each value was read; a value older than six months is re-checked before the next pilot.
