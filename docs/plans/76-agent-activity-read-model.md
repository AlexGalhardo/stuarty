# 76: Agent activity: skill, skill state, tool and plain description

## Goal

The scene bar tells the truth about what an agent is doing: which skill it loaded, whether that skill is loaded for this task, which tool it is using, and in plain words what that tool does.

## Context and related decisions

- The author's standing requirement of explicit agent state, author decision D-137, SDD-017 (hosted agent), SDD-018 (skills), SDD-003 (agent interfaces), ADR-0032, ADR-0040.
- Today `core/chat/event.go` records tool titles and permissions, and no event says that a skill was loaded. `service/agenthost/tokens.go` binds each session token to a `Grant` with its `SessionID`. Skills reach an agent through `SkillGet` in the MCP tool `library` (`web/app/mcp/library.go`).

## User stories

- As a creator, I want to see the skill my agent loaded so that I know which procedure it follows.
- As a creator, I want to know when no skill is loaded so that I do not assume one.
- As a creator, I want the tool described in plain words beside its technical name so that I can judge it.
- As an agent developer, I want the same activity over HTTP and the CLI so that I can inspect a session.

## Scope

- Modules: `core/chat/` (the activity rules), `service/agenthost/`, new `service/chat_activity.go`, the session context in `web/app/mcp/library.go`, a new HTTP endpoint and a CLI command, SDD-000, SDD-003, SDD-017.
- The last task wires the scene bar of epic 75: `web/resources/js/shell/`.
- Must not touch: the approval flow, the store schema of other domains.

## Out of scope

Observing skills that live inside the user's own agent (they are invisible to Stuarty), the notification read model (epic 77), and the workflow (epic 78).

## Dependencies

57 (it changes `service/chat_ops.go`) and 75 for the last task.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | The scene bar shows the real skill and tool |
| HTTP | `GET /api/v1/chat/sessions/{id}/activity` |
| CLI | `chat:activity <session>` |
| MCP | None. The chat has no MCP surface today, and `tools/list` holds 23,340 of 23,400 bytes |

## Core commands and permissions

Names are proposals until task 1 writes them to SDD-000.

| Operation | Does | Permission |
| --- | --- | --- |
| `ChatActivityGet` | Returns the activity of one session | `read`, within the session's project and owner |

## Technical design

- **Observed, never presumed.** When a request authenticated by a session token reads a skill through `SkillGet`, the host records a transcript event of a new kind, `skill_loaded`, with the skill's name and version hash. A skill read by a person, or by a key with no session, records nothing.
- **Skill state**, derived in `core/chat`: `loaded` (read in this session), `available` (a reviewed skill exists in the library, none loaded), `none`. The result lists loaded skills in order and names the last as current. The wording is "loaded in this session": Stuarty cannot prove that the agent still follows it.
- **Tool.** The last `tool_call` gives title, kind and status. For a tool of Stuarty's own MCP server, a table in `core/chat` maps the tool and its `action` to a message key with a plain description. A tool outside the table shows its title, marked as text from the agent.
- **Result.** `{session_id, state, skill:{name,state}, skills_loaded[], tool:{title,technical_name,description_key,status}, updated_at}`. Agent text is delimited as untrusted.
- The event stream carries `skill_loaded`, so the bar updates live with no polling.

## Tasks

- [x] 1. Amend SDD-000, SDD-003 and SDD-017. Verify: `task lint:md` exits 0.
- [ ] 2. The kind `skill_loaded` and the pure activity reducer. Verify: `go test ./core/chat/...` passes with a table of event sequences.
- [ ] 3. Record a skill read with the session of the token, and only then. Verify: `go test ./service/agenthost/... ./web/app/mcp/...` passes, and a person's read records nothing.
- [ ] 4. The plain-description table for Stuarty's tools. Verify: a Go test fails when a tool of `web/app/mcp/definitions/` has no entry, and `bun run check:i18n` exits 0.
- [ ] 5. `ChatActivityGet` with its visibility checks. Verify: `go test ./service/... -run ChatActivity` and `go test -run TestSecurity_ ./...` pass.
- [ ] 6. The HTTP endpoint and the CLI command with parity. Verify: `go test ./web/contract/...` and `go run ./internal/tools/surfacebudget` pass.
- [ ] 7. Wire the scene bar. Verify: `cd web && bun run e2e -- scene-bar` passes with the fake agent loading a skill.

## Test strategy

Reducer tables, a fake agent that loads a skill over MCP, isolation tests with two members, a stream test.

## Acceptance criteria

1. **Given** a session whose agent read a skill through `library`, **when** the activity is read, **then** `skill.state` is `loaded` with that name.
2. **Given** a session that read no skill, **when** the activity is read, **then** the state is `available` or `none`, never `loaded`.
3. **Given** a person reading the same skill in the browser, **when** the activity is read, **then** nothing changed.
4. **Given** a tool of Stuarty, **when** it runs, **then** the result carries its description key and technical name.
5. **Given** a member and another member's session, **when** the activity is requested, **then** the answer is `E_NOT_FOUND` (`go test -run TestSecurity_ ./...`).
6. `go run ./internal/tools/surfacebudget` passes with `tools/list` unchanged, and `task lint`, `task test` and `task test:web` pass.

## Risks and mitigation

- "Loaded" can be read as "followed": the wording says what was observed.
- An agent can read a skill to look busy: the event only reports a read, and grants no permission.
- **STOP** if recording needs a schema change to `chat_events`, if the session cannot be identified without weakening the token model, or if the MCP budget would change.

## Size

L: a new event, a read model and three surfaces. About 7 commits.

## Priority

Must. Without it, the scene bar cannot state the skill.

## Hand-off for review

The commits, each criterion with its command and result, a transcript of the fake agent loading a skill.
