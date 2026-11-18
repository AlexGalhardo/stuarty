# Plan 001: Record the studio direction (ADR-0030, SDD-014, PRD-004)

This plan answers how the agent studio direction (author decisions D-041 to D-044) enters the public planning documents before any code or skill is written, so that every later plan cites a document instead of a conversation.

- Owner: orchestrator (documents only)
- Status: TODO
- Effort: S
- Depends on: nothing
- Written against: `8787714a`

## 1. Why

`docs/README.md` rule 3 says a decision of the author enters the documents through an ADR, and AGENTS.md rule 4 says a specification is the contract. Today no document says that generation, provider integrations and engine choice live on the agent side, that a production folder exists, or how memory works. Without that, an executor of plan 002 has to guess the layout, and a reviewer cannot reject a change that puts a provider client in `service/`.

## 2. Current state (drift check)

Run these and compare. If any differs, stop and report.

```text
git grep -n "No outbound capability" docs/adrs/0020-agent-safety-model.md   # one hit, Decision item 1
ls docs/adrs | tail -1                                                     # 0029-one-renderer-webgpu-webcodecs-rust.md
ls docs/sdds | tail -1                                                     # 013-renderer.md
ls docs/prds | tail -1                                                     # 003-distribution-and-docs.md
```

## 3. Tasks

### Step 1: Write ADR-0030 "The agent studio lives outside the editor"

- File: `docs/adrs/0030-agent-studio-outside-the-editor.md`; one row in the ADR table of `docs/README.md`.
- Structure: copy the section layout of `docs/adrs/0021-transcription-providers.md` (status line, Context, Decision as numbered items, Alternatives, Consequences).
- Decision items to write, one each:
  1. The editor stays a toolkit with no outbound capability; ADR-0020 is unchanged and is cited, not amended.
  2. Generation (image, video, voice, music, sound effects) and provider credentials live on the agent side, through each provider's MCP server or CLI. The product never stores a provider key.
  3. The engine is chosen per request by the agent: HyperFrames, Remotion, or the editor. The editor is used when footage must be cut, captioned, mixed or adjusted.
  4. A production folder holds every video, whatever the engine (layout in SDD-014).
  5. The studio kit is a Claude Code plugin in `studio/`, in the Agent Skills format, versioned with the product's release tags.
  6. Learnings are Markdown: per video and in a global memory folder (format in SDD-014).
  7. Outputs of an external engine enter the editor only as files through `media_import`.
- Alternatives to record: native provider plugins in `service/` (rejected: reopens ADR-0020 and adds keys to the product); a project in the editor for every video (rejected: forces the editor into flows that do not need it); skills in a separate repository (rejected: two versions to keep in step with the generated editor skill).
- Cite decisions as "author decision D-041" (never a path into the private folder).
- Acceptance:
  - [ ] `task lint` reports nothing for the new file.
  - [ ] `docs/README.md` lists ADR-0030 with a one-line summary.
  - [ ] the ADR contains no provider key and no path into the private maintainers' folder (`.agents/rules/development-folder.md` rule 1).

### Step 2: Write SDD-014 "The studio kit"

- File: `docs/sdds/014-studio-kit.md`; one row in the SDD table of `docs/README.md` (owner senior-d).
- Sections, numbered:
  1. Scope and non-goals (no code in `service/`, no provider client anywhere in the Go module).
  2. **Plugin layout**: `studio/.claude-plugin/plugin.json`, `studio/skills/<name>/SKILL.md`, `studio/templates/<engine>/<name>/`, `studio/references/<name>.md`, `studio/README.md`, and the marketplace manifest `.claude-plugin/marketplace.json` at the repository root that points at `./studio`.
  3. **Skill format**: YAML frontmatter with `name` (kebab-case, equal to the folder name) and `description` (one sentence that starts with "Use when", at most 300 characters). Hand-written skills carry no version: the plugin's `version` is the version of all of them. Only the generated editor skill keeps its `metadata.version`, which must equal the plugin's. Body sections: When to use, Inputs, Steps, Checks, Learnings to record. At most 400 lines per `SKILL.md`; longer material goes to `references/`.
  4. **Production folder**: the layout below, with one line per entry saying who writes it.

     ```text
     <root>/<yyyy-mm-dd>-<slug>/
     ├── brief.md          the request, the platform, the duration, the engine chosen and why
     ├── assets/           every input file; SOURCES.md lists origin, licence, cost of each
     ├── engine/
     │   ├── vukem/        project id, revision exported, the batches used (JSON)
     │   ├── hyperframes/  the scene sources
     │   └── remotion/     the composition sources
     ├── out/              final files, each with <file>.render.json (the hand-off manifest)
     └── learnings.md      what worked, what failed, what to repeat
     ```

     The root defaults to `~/vukem-studio/productions` and is overridden by `VUKEM_STUDIO_HOME`.
  5. **Hand-off manifest** `<file>.render.json`: `engine`, `engine_version`, `command`, `inputs` (path and SHA-256), `width`, `height`, `fps`, `duration_ms`, `created_at`. It is the only contract between an external engine and the editor.
  6. **Memory**: `<VUKEM_STUDIO_HOME>/memory/MEMORY.md` (index, one line per fact) and one file per fact with frontmatter `name`, `description`, `type` (`style`, `platform`, `engine`, `provider`, `mistake`), `tags`, `use_cases`, `source` (the production folder it came from). The `recall` skill reads the index first; the `retro` skill writes facts.
  7. **Versioning and discovery**: the plugin version equals the product version; the router skill `video-studio` lists every other skill and when to choose it.
  8. Acceptance (the checks plan 002 automates).
- Acceptance:
  - [ ] `task lint` clean for the file.
  - [ ] the example layout and both JSON examples parse (`python -c "import json,sys; json.load(open(sys.argv[1]))"` on each example extracted to a scratch file).
  - [ ] SDD-000 is not changed by this card (the studio adds no operation, table or error code).

### Step 3: Write PRD-004 "Agent studio" and its traceability rows

- File: `docs/prds/004-agent-studio.md`, same layout as `docs/prds/003-distribution-and-docs.md`.
- Requirements, numbered from `RF-300` (phase column `S` for studio):
  - RF-300 an agent with only the plugin installed can produce a vertical video for each of the three use cases (game meme cut, product promo, authorial video);
  - RF-301 every video has a production folder with the SDD-014 layout;
  - RF-302 an agent reads relevant learnings before planning and records new ones after delivery;
  - RF-303 an agent picks the simplest engine for the request and writes the reason in `brief.md`;
  - RF-304 every asset has its origin and licence recorded;
  - RF-305 an agent reverts a project to a revision in one call (plan 006);
  - RF-306 an agent creates and lists named checkpoints and reads the diff between two revisions (plan 008);
  - NFR-060 every skill passes the skill lint; NFR-061 a Haiku-class model completes the pilot of plan 004.
- Update `docs/README.md` (PRD table) and `docs/traceability.md` by hand for the new rows (task-cards rule section 3 item 8).
- Acceptance:
  - [ ] the maintainers' traceability script (`.agents/rules/task-cards.md` section 3 item 8) lists RF-300..RF-306 with an ADR (0030) and an SDD (014 or 005).
  - [ ] `task lint` clean.

## 4. Out of scope

- Any code, any skill, any change to ADR-0020 or SDD-007.
- Rewriting ADR-0029 (Phase 5 continues).

## 5. STOP conditions

- An existing ADR contradicts item 2 or 3 of ADR-0030 in a way not listed above: stop and ask the orchestrator.
- The traceability script requires a field the PRD format does not have: stop and report.

## 6. Maintenance

SDD-014 becomes the contract of every skill. A change to the production folder layout or the manifest changes SDD-014 first, in the same commit as the skills that follow it.
