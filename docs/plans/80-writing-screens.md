# 80: Writing screens and the text flow

## Goal

A creator writes a post, copy or script from start to end in a calm editor, reviews it, and knows what the agent may do there, with no trace of a video workflow.

## Context and related decisions

- Author decisions D-134 and D-137 (`copy` ends in copy or export, and goes to a blog when the person chooses). Frames 03 and 33d, 33e of the final redesign file held by the author, with their state frames.
- SDD-019. Code: `web/resources/js/pages/Write/`.

## User stories

- As a creator, I want a clean Markdown editor with revisions and reading time.
- As a creator, I want to compare revisions before keeping one.
- As a creator, I want to copy or export a copy text with no publishing step.

## Scope

- Modules: `web/resources/js/pages/Write/` and its language keys.
- Must not touch: `core/`, `service/`, the shell, the Publish pages.

## Out of scope

A new blog skill, a destination for `copy`, and collaborative editing.

## Dependencies

74, 75 and 78.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | Write, revisions, review |
| HTTP, CLI, MCP | None |

## Core commands and permissions

None new.

## Technical design

- The page has one primary action per state: Save, then Review, then the next workflow step.
- Reading time and words come from the server.
- The review compares two stored revisions, side by side at desktop width and stacked at phone width, and runs the existing `PostCheck` for a post.
- In a text-only project, the agent panel offers only text actions, and states that cut, video caption and export do not apply.
- A `copy` ends in "Copy" and "Export". Sending it to a blog is an explicit choice.

## Tasks

- [x] 1. The page frame, list and editor on the new components. Verify: `cd web && bun run typecheck && bun run test -- Write` pass.
- [x] 2. Script, caption, draft and post panels. Verify: `cd web && bun run test -- Preview` passes.
- [x] 3. Revisions and the compare view. Verify: new spec `write-review` passes.
- [x] 4. The end of a text by type, including copy. Verify: new spec `write-text-flow` passes.
- [ ] 5. Empty, loading, error and agent states. Verify: `bun run check:i18n` exits 0 and the states have unit tests.
- [ ] 6. Check three widths in both themes. Verify: `cd web && bun run e2e -- a11y` passes.

## Test strategy

Unit tests of each panel, browser specs for review and for a copy text, axe.

## Acceptance criteria

1. **Given** a post, **when** it is edited and saved, **then** words and reading time come from the server's answer.
2. **Given** two revisions, **when** compared and one restored, **then** the restored text is stored as a new revision.
3. **Given** a `copy`, **when** it is finished, **then** "Copy" and "Export" are offered with no required destination.
4. **Given** a text-only project, **when** the agent panel opens, **then** no video action is offered.
5. `bun run typecheck`, `bun run check:i18n`, `bun run lint` and `task test:web` pass.

## Risks and mitigation

- Editor state can be lost in a restyle: the existing tests of the panels stay.
- **STOP** if the compare view needs a new endpoint.

## Size

M: one area, existing data. About 6 commits.

## Priority

Must. Text projects are first-class (D-134).

## Hand-off for review

The commits, each criterion with its command and result, screenshots in both themes.
