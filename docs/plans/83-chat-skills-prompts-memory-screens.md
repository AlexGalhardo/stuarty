# 83: Chat, skills, prompts and memory screens

## Goal

Talking to an agent and managing what it knows become clear: each permission is readable, and each skill, prompt and memory note says what it is and whether it can be used.

## Context and related decisions

- Author decisions D-134 and D-137, SDD-017, SDD-018, ADR-0040. Frames 08 to 11 and their state and responsive frames in the final redesign file held by the author.
- Code: `web/resources/js/dashboard/chat/`, `web/resources/js/pages/Skills/`, `web/resources/js/pages/Prompts/`, `web/resources/js/pages/Memory/`, `web/resources/js/pages/Library/`.

## User stories

- As a creator, I want each tool call in the conversation to be readable so that I can follow the agent.
- As a creator, I want to see whether a skill was reviewed before an agent may use it.
- As a creator, I want to approve a memory note before it is kept.

## Scope

- Modules: `web/resources/js/dashboard/chat/`, the Skills, Prompts, Memory and Library pages, their language keys.
- Must not touch: `core/`, `service/`, the shell's scene bar.

## Out of scope

New agent capabilities, and any "always allow" option.

## Dependencies

74, 75 and 76.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | Chat panel, Skills, Prompts, Memory |
| HTTP, CLI, MCP | None |

## Core commands and permissions

None new.

## Technical design

- `ChatMessage`, `ToolLine` and `Composer` keep agent text inside `UntrustedText`.
- A permission card shows the same fields as the scene bar. The two reflect one server state.
- In a text-only project, the suggested prompts are about text.
- Skills show their review state in words; prompts show `{{variables}}` as fields; memory shows pending notes first.

## Tasks

- [x] 1. The conversation: messages, tool lines and composer. Verify: `cd web && bun run test -- ChatConversation ChatEventView` passes.
- [x] 2. The permission card with allow and reject once. Verify: `cd web && bun run e2e -- agent-chat` passes.
- [x] 3. Agent setup and its states. Verify: `cd web && bun run test -- ProfileSetup` passes.
- [x] 4. Skills. Verify: `cd web && bun run test -- Library` passes.
- [x] 5. Prompts and memory. Verify: `cd web && bun run typecheck && bun run check:i18n` pass.
- [x] 6. Check 1440, 1280 and 390 px in both themes. Verify: `cd web && bun run e2e -- a11y` passes.

## Test strategy

Existing chat tests and specs, a hostile-text fixture in the conversation, axe.

## Acceptance criteria

1. **Given** a tool call with markup in its title, **when** shown, **then** it renders as text.
2. **Given** an open permission, **when** answered in the chat, **then** the bar and the center show the same result.
3. **Given** an unreviewed skill, **when** listed, **then** it says it is not offered to agents.
4. **Given** a pending memory note, **when** approved by a person, **then** it is kept, and a rejected one is not.
5. `bun run typecheck`, `bun run check:i18n`, `bun run lint` and `task test:web` pass.

## Risks and mitigation

- Restyling can drop an untrusted boundary: a test with hostile text covers it.
- **STOP** if a frame needs an agent capability that does not exist.

## Size

M: restyle of four connected areas. About 6 commits.

## Priority

Must. The chat is the main agent surface.

## Hand-off for review

The commits, each criterion with its command and result, screenshots of a permission in both themes.
