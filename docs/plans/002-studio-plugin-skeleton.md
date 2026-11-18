# Plan 002: The `studio/` plugin: layout, manifest, skill lint, editor skill, router skill

This plan answers how the studio kit becomes an installable Claude Code plugin in this repository, how its skills are checked on every lint, and how the generated editor skill reaches it without drifting.

- Owner: senior-d (root files, `internal/tools/`, `web/cmd/gen-agent-docs/`, `.github/`)
- Status: IN PROGRESS (orchestrator, 2026-11-18)
- Effort: M
- Depends on: plan 001 (SDD-014 is the contract)
- Written against: `8787714a`

## 1. Why

The studio is the product surface for agents that do more than edit. Without a lint, skills rot silently; without the generated editor skill inside the plugin, an agent gets two different descriptions of the editor's commands.

## 2. Current state (drift check)

```text
ls web/app/agentdocs/out                     # AGENTS.snippet.md  SKILL.md  mcp-instructions.txt
sed -n 1,5p web/app/agentdocs/out/SKILL.md   # frontmatter: name: vukem-video, metadata.version
grep -n "dir := fs.String" web/cmd/gen-agent-docs/main.go   # default web/app/agentdocs/out
grep -n "^  lint:md:" -A3 Taskfile.yml       # bunx markdownlint-cli2@... "**/*.md"
ls internal/tools/labelsync                  # labels.go labels_test.go main.go (the pattern for a small tool)
ls studio 2>/dev/null                        # nothing: the folder does not exist yet
```

If `studio/` already exists, stop and report.

## 3. Conventions to match

- A small Go tool is a `main.go` plus a testable package-level function and a `_test.go`, like `internal/tools/labelsync/`. Standard library only: no YAML library. Parse the frontmatter by hand (it is a flat block between two `---` lines, with one nested `metadata:` map).
- Generated files are written only by their generator (task-cards rule section 4); the `--check` flag of `web/cmd/gen-agent-docs/main.go` is the pattern for drift checks.
- Commit scopes must be in `.github/workflows/pr-title.yml` (two places: the `scopes: |` list near line 40 and the `scopes=` regex near line 119), `.github/labels.txt`, and SDD-009 section 6.2; `internal/tools/labelsync` tests that they agree.

## 4. Tasks

### Step 1: Add the commit scope `studio`

- Files: `.github/workflows/pr-title.yml`, `.github/labels.txt`, `docs/sdds/009-releases-and-ci-cd.md` (section 6.2).
- Commit: `ci(repo): allow the studio commit scope`.
- Acceptance:
  - [ ] `go test ./internal/tools/labelsync/...` passes.
  - [ ] `grep -c "studio" .github/workflows/pr-title.yml` prints 2.

### Step 2: Create the plugin skeleton and the marketplace manifest

- Files: `studio/.claude-plugin/plugin.json`, `studio/README.md`, `.claude-plugin/marketplace.json`, `AGENTS.md` (sections 2 and 4: `studio/` belongs to senior-d), and the release tool under `internal/tools/release/` so that `prepare vX.Y.Z` writes the version into `plugin.json` in the same stamping step that already writes it into the agent docs (follow the code that produced commit `0aeb7748`, "stamp the agent docs with version 0.29.0").
- Before writing the manifests, read the current Claude Code plugin and marketplace documentation (ask the `claude-code-guide` agent or read the official docs) and copy the exact required fields. If the format differs from SDD-014 section 2, stop and report: SDD-014 changes first.
- `plugin.json`: `name` `vukem-studio`, `version` equal to the current product version (0.29.0 at the time of writing), `description`, `license` `MIT`, `repository`.
- `marketplace.json`: one plugin entry, `source` `./studio`.
- `studio/README.md`: what the plugin is, how to install it (`/plugin marketplace add AlexGalhardo/vukem-video`, then `/plugin install vukem-studio`), the list of skills (filled by later cards), the licence note for Remotion (plan 007).
- Commit: `feat(studio): add the plugin skeleton and marketplace manifest`.
- Acceptance:
  - [ ] both JSON files parse (`python -c "import json; json.load(open('studio/.claude-plugin/plugin.json')); json.load(open('.claude-plugin/marketplace.json'))"`).
  - [ ] in Claude Code, `/plugin marketplace add <absolute repository path>` lists `vukem-studio` (paste the output in the report).
  - [ ] `task lint:md` clean.
  - [ ] `go test ./internal/tools/release/...` passes, with a test that `prepare` writes the new version into a copy of `plugin.json`.

### Step 3: Write the skill lint `internal/tools/skillcheck` and run it in `task lint`

- Files: `internal/tools/skillcheck/main.go`, `internal/tools/skillcheck/skillcheck.go`, `internal/tools/skillcheck/skillcheck_test.go`, `Taskfile.yml` (a `lint:studio` task called from `lint`).
- Rules (SDD-014 section 3), each a test case with a good and a bad fixture under `internal/tools/skillcheck/testdata/`:
  1. every `studio/skills/<dir>/SKILL.md` has frontmatter with `name` and `description`;
  2. `name` equals `<dir>` and is kebab-case ASCII;
  3. `description` starts with `Use when` and has at most 300 characters;
  4. when a skill has `metadata.version` (only the generated `vukem-video` skill does), it equals `version` in `studio/.claude-plugin/plugin.json`;
  5. the file has at most 400 lines;
  6. every relative link in a skill resolves to an existing file.
- Output: one line per finding, `path:line: rule: message`; exit 1 when there is a finding, 0 otherwise.
- Commit: `feat(studio): lint the skills of the studio plugin`.
- Acceptance:
  - [ ] `go test ./internal/tools/skillcheck/...` passes, with one failing fixture per rule.
  - [ ] `go run ./internal/tools/skillcheck studio` exits 0 on the skeleton.
  - [ ] `task lint` runs it (paste the `lint:studio` line of the output).

### Step 4: Make the generator write the editor skill into the plugin

- Files: `web/cmd/gen-agent-docs/main.go`, `web/cmd/gen-agent-docs/main_test.go`, `studio/skills/vukem-video/SKILL.md` (generated).
- Change: besides `--dir`, the generator also writes `SKILL.md` to `studio/skills/vukem-video/SKILL.md`, and `--check` fails when that copy differs. Keep `web/app/agentdocs/out/` unchanged: the release workflow compares it (`.github/workflows/release.yml` around line 218).
- Commit: `feat(studio): ship the generated editor skill in the plugin`.
- Acceptance:
  - [ ] `go run ./web/cmd/gen-agent-docs --check` exits 0 after `go run ./web/cmd/gen-agent-docs`.
  - [ ] a test edits the copy in a temporary directory and `--check` reports it (fails before the change, passes after).
  - [ ] `go run ./internal/tools/skillcheck studio` exits 0.

### Step 5: Write the router skill `video-studio`

- File: `studio/skills/video-studio/SKILL.md`.
- Content (SDD-014 section 3 body sections):
  - When to use: any request to create or change a video.
  - Steps: (1) run `recall` (plan 003) for the use case and platform; (2) create the production folder with `studio-new` (plan 003); (3) choose the engine with the table below and write the reason in `brief.md`; (4) gather assets with the provider skills (plan 005); (5) render; (6) check; (7) run `retro`.
  - Engine table: motion graphics, kinetic text, animated cards with no real footage → HyperFrames; data-driven or templated compositions written in React, when the user's Remotion licence allows → Remotion; cutting, captioning, mixing or adjusting real footage, or fine adjustments of any render → the editor (`vukem-video` skill). A mix is allowed: render a scene with an external engine, then import it into the editor.
  - Skills that do not exist yet are listed as "planned" with their plan number, so the lint's link rule passes.
- Commit: `feat(studio): add the video-studio router skill`.
- Acceptance:
  - [ ] `go run ./internal/tools/skillcheck studio` exits 0.
  - [ ] in a fresh Claude Code session with the plugin installed, asking "make a 15-second vertical meme from this clip" triggers `video-studio` (paste the first lines of the transcript).

## 5. Out of scope

- MCP prompts or resources in `web/app/mcp/` (not chosen; the plugin is the discovery path).
- Any Go code outside `internal/tools/skillcheck/` and `web/cmd/gen-agent-docs/`.

## 6. STOP conditions

- The official plugin format has no marketplace manifest at the repository root, or requires fields SDD-014 does not list: stop; SDD-014 changes first.
- Changing the generator would change any byte of `web/app/agentdocs/out/`: stop and report.

## 7. Maintenance

The release (`go run ./internal/tools/release prepare`) bumps `studio/.claude-plugin/plugin.json` (step 2) and regenerates the editor skill; if either step is skipped, rule 4 of the skill lint fails on the release commit, which is the intended alarm. A new skill needs no version bump of its own.
