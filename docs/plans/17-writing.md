# 17: Writing: scripts, copy and captions

## Goal

A creator writes the script, the hook and the caption of each network inside the project, by hand or with a model, and the text stays tied to the production it belongs to: the script next to the timeline it will become, the caption next to the post it will go out with.

## Context and related decisions

- ADR-0031 (writing is a domain of the same core), ADR-0039, ADR-0036.
- Author decision D-047. PRD-004 RF-350, RF-351, RF-352, RF-354.
- Research note `web/05-rotina-criadores`.

## Scope

- Modules: `core/writing/` (new: pure rules of text documents, their kinds, revisions, per-network limits), `service/` (text operations), `web/app/` (controllers, CLI, MCP), `web/resources/js/pages/Write/`, `web/database/migrations/`, `docs/sdds/019-writing.md` (new), SDD-000.
- Text kinds: `script` (with scenes and estimated duration), `copy` (free text), `caption` (one text with variants per network).

## Out of scope

- Blog posts and their destinations (epic 23).
- Turning a script into a timeline automatically.
- The subtitle captions of a video: they remain in the video domain; `caption` here means the text that accompanies a post.

## Dependencies

06, 09.

## Technical design

- **A text is a project item** of kind `text` (epic 06) with a `text_kind`, a Markdown body, and revisions: every save is a revision with its actor, so a person can see what an agent changed and go back.
- **Rules in `core/writing`**:
  - script: scenes as headings, narration and on-screen text as marked blocks, `EstimateDuration(script, wordsPerMinute)`;
  - caption: variants per network with the limits of each (length, hashtag count, link behaviour) kept as data in one table with the date and the source page of each limit; `ValidateCaption(variant, network)` returns precise problems;
  - `Diff(revA, revB)` at paragraph level for the text view.
- **Generation**: `TextDraft` asks the provider layer (epic 09) for a draft from the brief and an instruction, optionally using a prompt of the library when epic 15 is merged; the result is a new revision marked as generated, never a silent overwrite. Costs and ceilings as in epic 09.
- **Service**: `TextList`, `TextGet`, `TextWrite`, `TextRevisions`, `TextRevert`, `TextDraft`, `CaptionValidate`, `ScriptEstimate`; registered in the `writing` domain.
- **UI**: the Write section lists texts across projects; the editor is a plain Markdown editor with a preview, the revision list, the per-network counters for captions and a "draft with a model" action. No rich-text dependency.
- **Untrusted text**: bodies are returned to agents delimited, like any text a person or a model wrote.

## Tasks

- [x] 1. Write SDD-019 and SDD-000 names, including the per-network limits table with sources and dates. Verify: `task lint:md` exits 0.
- [x] 2. Implement `core/writing` with table tests. Verify: `go test ./core/writing/...` passes.
- [x] 3. Add storage, revisions and the service operations. Verify: `go test ./service/... -run "TestText|TestCaption|TestScript"` passes.
- [x] 4. Implement `TextDraft` on the provider layer with a fake provider. Verify: `go test ./service/... -run TestTextDraft` passes (a draft is a new revision; cost is recorded).
- [x] 5. Expose on HTTP, CLI and MCP with parity. Verify: `go test ./web/contract/...` passes.
- [x] 6. Build the Write section and the editor in both languages. Verify: `task test:web` and `web/e2e/specs/writing.spec.ts` pass.
- [x] 7. Finish the writing steps of the built-in skills when epic 16 is merged; otherwise leave the note for it. Verify: `go test ./web/resources/skills/...` passes, or the note is in the hand-off.

## Test strategy

- Unit: duration estimates; every caption limit at its boundary; diff.
- Integration: revisions and revert; a generated draft never overwrites.
- Contract: parity on the three surfaces.
- End to end: write a script, draft a caption, see the counters, revert.

## Acceptance criteria

1. A script, a copy text and a caption can be created, edited and reverted in a project on HTTP, CLI, MCP and in the UI.
2. `CaptionValidate` reports the exact limit broken for each network in the table.
3. `TextDraft` creates a revision marked as generated, with provider, model and cost, and leaves earlier revisions intact.
4. `ScriptEstimate` returns a duration that changes with the words-per-minute setting.
5. Texts appear in the project overview and in `ProjectDescribe`.
6. `go test ./core/writing/... ./service/... ./web/contract/...`, `bun run check:thin-client` and `task test:web` pass.

## Risks and mitigation

- **Network limits change.** Mitigation: limits are data with a source and a date; the connectors of epics 18, 19 and 22 validate against the same table.
- **The counters computed in the browser.** Mitigation: the thin-client check; the core validates and the UI displays.

## Size

M

## Hand-off for review

The diff of `epic/17-writing`; the limits table with its sources; the outputs of the acceptance commands.
