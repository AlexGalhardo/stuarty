# 77: One read of everything that waits for a person

## Goal

A creator sees in one list every request that waits for them, whether it came from a chat permission or from the approval queue, each with what will happen, where, at what cost and whether it can be undone.

## Context and related decisions

- Author decision D-137: chat permissions and Approvals stay separate flows in the backend. The notification center is a unified **read** of both, and each decision follows its own flow.
- ADR-0032, ADR-0033, SDD-015 section 4 (visibility per member), SDD-017. Code: `core/chat/permission.go`, `service/approval_ops.go`, `service/chat_ops.go`.

## User stories

- As a creator, I want one list of what waits for me so that I do not check two places.
- As a creator, I want each request to say what happens, to what, at what cost and whether it can be undone so that I can decide.
- As a member of a shared server, I want to see only my requests so that my work stays private.

## Scope

- Modules: new `core/inbox/`, new `service/inbox.go`, the capture of allowlisted permission fields in `service/agenthost/`, a new HTTP endpoint and stream, a CLI command, SDD-000, SDD-015, SDD-017; the notification center and bell in `web/resources/js/shell/`.
- Must not touch: `ApprovalDecide`, `ChatAnswerPermission`, the approval rules and ceilings.

## Out of scope

Merging the two queues, deciding from the new read, any "always allow", and the Approvals page body (epic 82).

## Dependencies

57, 75 and 76.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | Bell, center and scene bar read one list |
| HTTP | `GET /api/v1/inbox` and `GET /api/v1/inbox/events` |
| CLI | `inbox:list` |
| MCP | None: this is the decision surface of a person, and an agent must not read the requests of other sessions |

## Core commands and permissions

Names are proposals until task 1 writes them to SDD-000.

| Operation | Does | Permission |
| --- | --- | --- |
| `InboxList` | Lists open and recent requests from both sources | `read`, filtered by the caller's visibility; refused to a session token |

## Technical design

- **A row** is `{id, source: chat|approval, source_id, state, agent, skill, tool, action_key, item, destination, cost, undo, created_at}`. `id` encodes the source; the row never carries a credential.
- **Decisions.** The client calls the endpoint of the row's source. The read model has no write path, so the privileged checks stay where they are.
- **Permission detail.** `chat.Permission` holds only a title and a kind today. For tools of Stuarty's own MCP server, the host keeps an allowlist of input fields from the protocol's raw input (the project, the item, the operation names). Any other tool keeps its title, and its unknown fields read "Not reported". Text from the agent stays untrusted.
- **Undo** is one of `can`, `cannot` or `not_verified` (author decision D-140). A row says `can` only when Stuarty can prove it, never on the agent's word: a chat permission is asked before its call runs, by the title the agent gives it, so every chat row is `not_verified`. An approval is `cannot`.
- **Cost** comes from the approval's estimate; a chat permission has none.
- **Skill** comes from the activity of epic 76 at the moment of the request.

## Tasks

- [x] 1. Amend SDD-000, SDD-015 and SDD-017. Verify: `task lint:md` exits 0.
- [x] 2. The pure merge, order and row rules in `core/inbox`. Verify: `go test ./core/inbox/...` passes.
- [x] 3. Capture the allowlisted fields of a permission. Verify: `go test ./service/agenthost/...` passes, including a hostile input that must not appear.
- [x] 4. `InboxList` with member isolation, refused to session tokens. Verify: `go test ./service/... -run Inbox` and `go test -run TestSecurity_ ./...` pass.
- [x] 5. HTTP, the stream and the CLI. Verify: `go test ./web/contract/...` and `go run ./internal/tools/surfacebudget` pass.
- [x] 6. Bell, center and bar read the new list. Verify: `cd web && bun run e2e -- notification-center` and `bun run e2e -- approval` pass.

## Test strategy

Merge tables, isolation tests with two members, a hostile tool input fixture, the real fake-agent permission in the browser.

## Acceptance criteria

1. **Given** an open chat permission and a pending approval, **when** `InboxList` runs, **then** both appear with their source.
2. **Given** a row, **when** a person decides it, **then** the request goes through `chat.sessions.answer` or `ApprovalDecide`, and no decision endpoint was added (`git diff` of the endpoints shows only reads).
3. **Given** a session token, **when** it calls `InboxList`, **then** it is refused.
4. **Given** two members, **when** one lists, **then** no row of the other appears (`go test -run TestSecurity_ ./...`).
5. **Given** a tool outside Stuarty, **when** it asks for permission, **then** item and destination are "Not reported" and undo is "Not verified" (D-140).
6. `task lint`, `task test` and `task test:web` pass, and `tools/list` is unchanged.

## Risks and mitigation

- Raw tool input can carry secrets: an allowlist, the scrubber of the recorder, and a test.
- A wrong "can be undone" would mislead a decision: Stuarty says yes only when it can prove it (D-140).
- **STOP** if a row needs a decision path, or if the protocol does not provide the raw input for Stuarty's tools.

## Size

L: a read model over two domains with security tests. About 6 commits.

## Priority

Must. It delivers the clear approval notifications the author asked for.

## Hand-off for review

The commits, each criterion with its command and result, the isolation test output.
