# SDD-014: The studio kit

- Status: Accepted (2026-11-18)
- Depends on: ADR-0031, ADR-0020
- Implements: PRD-004 RF-300 to RF-306, NFR-060, NFR-061
- Owner area: senior-d (`studio/`)

## Contents

1. Scope and non-goals
2. Plugin layout
3. Skill format
4. Production folder
5. Hand-off manifest
6. Memory
7. Versioning and discovery
8. Acceptance

## 1. Scope and non-goals

The studio kit is a plugin of skills, templates and references. It does not add code to `service/` and it does not add a provider client anywhere in the Go module. The product stays a toolkit with no outbound capability (ADR-0020, ADR-0031 item 1).

## 2. Plugin layout

```text
studio/.claude-plugin/plugin.json
studio/skills/<name>/SKILL.md
studio/templates/<engine>/<name>/
studio/references/<name>.md
studio/README.md
.claude-plugin/marketplace.json
```

The marketplace manifest at the repository root points at `./studio`.

## 3. Skill format

YAML frontmatter has `name` (kebab-case, equal to the folder name) and `description` (one sentence that starts with "Use when", at most 300 characters). Hand-written skills carry no version: the plugin's `version` is the version of all of them. Only the generated editor skill keeps `metadata.version`, which must equal the plugin's.

Body sections: When to use, Inputs, Steps, Checks, Learnings to record. A `SKILL.md` is at most 400 lines. Longer material goes to `references/`.

## 4. Production folder

The root defaults to `~/vukem-studio/productions` and is overridden by `VUKEM_STUDIO_HOME`.

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

| Entry | Who writes it |
| --- | --- |
| `brief.md` | the agent, before any generation or edit |
| `assets/` and `assets/SOURCES.md` | the agent, one row per file it places |
| `engine/vukem/` | the agent, from the editor's project id and batches |
| `engine/hyperframes/` | the agent, when HyperFrames is the engine |
| `engine/remotion/` | the agent, when Remotion is the engine |
| `out/` and each `<file>.render.json` | the agent, after a render |
| `learnings.md` | the agent, after delivery |

The batches file in `engine/vukem/batches.json`:

```json
{
  "project_id": "prj_example",
  "revision": 4,
  "batches": ["edit clip.trim", "edit caption.add"]
}
```

## 5. Hand-off manifest

`<file>.render.json` is the only contract between an external engine and the editor. Example:

```json
{
  "engine": "hyperframes",
  "engine_version": "0.0.0",
  "command": "hyperframes render scene.html",
  "inputs": [
    {"path": "assets/clip.mp4", "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}
  ],
  "width": 1080,
  "height": 1920,
  "fps": 30,
  "duration_ms": 8000,
  "created_at": "2026-11-18T00:00:00Z"
}
```

The editor receives the file in `out/` only through `media_import` (ADR-0031 item 7).

## 6. Memory

`<VUKEM_STUDIO_HOME>/memory/MEMORY.md` is the index, one line per fact. Each fact is its own file with this frontmatter:

```yaml
name: short-hook
description: Open a meme cut on the action, not on a title card.
type: style
tags: [meme, vertical]
use_cases: [game-meme]
source: 2026-11-18-clip
```

`type` is one of `style`, `platform`, `engine`, `provider`, `mistake`. The `recall` skill reads the index first. The `retro` skill writes facts.

## 7. Versioning and discovery

The plugin version equals the product version. The router skill `video-studio` lists every other skill and when to choose it.

## 8. Acceptance

Plan 002 automates these checks:

- `go run ./internal/tools/skillcheck studio` exits 0.
- Every `SKILL.md` has `name` equal to its folder and a `description` that starts with "Use when" and is at most 300 characters.
- The generated editor skill's `metadata.version` equals the plugin version.
- No skill is longer than 400 lines.
- The marketplace manifest points at `./studio`.
