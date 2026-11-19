# 14: The user's agent hosted in the chat panel

## Goal

A creator talks to their own coding agent inside Stuarty, in a panel that is always one click away, and watches it work on the project in front of them: with the subscription and the login they already have in that agent, and no key handed to Stuarty.

## Context and related decisions

- ADR-0037 (Stuarty hosts the user's agent over the Agent Client Protocol; it runs no agent loop of its own), ADR-0036, ADR-0032, ADR-0008.
- Author decisions D-059 (the local agent as a way to connect), D-060 (Stuarty hosts the user's agent). PRD-004 RF-313, RF-380 to RF-383, RF-360 (local agent).
- Research note `web/03-integracao-ia` (the protocol, which agents speak it, what each provider allows).

## Scope

- Modules: `service/agenthost/` (new: process supervision, the protocol client, sessions, transcripts), `core/chat/` (new: pure rules of a session and of a permission request), `web/app/` (controllers, SSE, CLI and MCP read access), `web/resources/js/dashboard/chat/` (the panel in the right dock of epic 07), `web/database/migrations/`, `docs/sdds/017-hosted-agent.md` (new), SDD-007 rows.

## Out of scope

- An agent loop, a prompt orchestrator or tools executed by Stuarty on a model's behalf: the hosted agent does that.
- Bundling or installing an agent: the user installs their own.
- Voice input.

## Dependencies

07, 13.

## Technical design

- **Agent profiles**: a profile is a name, an executable path and arguments, written only from a privileged session (it is a command that will run on the machine). Stuarty ships suggestions for known agents; it never downloads one.
- **Process**: `service/agenthost` starts the agent as a child process with the project's working folder and speaks the protocol over its standard streams (JSON-RPC). The protocol client is written against the published specification; a library is a new dependency and a STOP.
- **Stuarty as the agent's tools**: the session hands the agent the address of Stuarty's own MCP server and a token scoped to the current project, so the agent edits through the same command core as any other agent, with the same approval queue.
- **Session rules in `core/chat`**: states, what a permission request is (the agent asks before a tool call the protocol marks as needing consent), and which answers a person may give. Approving a publication or a costly generation still happens in the approval queue, never in the chat.
- **Streaming**: agent messages, thoughts the agent chooses to show, tool calls and their results flow to the panel over SSE; everything the agent writes is rendered as untrusted text (no HTML, links shown in full).
- **Transcripts**: stored per project in the workspace, exportable as Markdown, deletable.
- **Network deployments**: the agent runs where the server runs. The panel is available only when a profile exists on that machine; the UI says so.
- **Service**: `ChatSessionStart`, `ChatSend`, `ChatCancel`, `ChatAnswerPermission`, `ChatSessionList`, `ChatTranscriptGet`, `AgentProfileList`, `AgentProfileSet` (privileged).

## Tasks

- [x] 1. Read the protocol specification again; write SDD-017 (profiles, process, messages used, errors) and SDD-000 names; add SDD-007 rows (a profile as command injection, agent text as markup, a token wider than the project, an agent that never exits). Verify: `task lint:md` exits 0.
- [x] 2. Implement `core/chat` with table tests. Verify: `go test ./core/chat/...` passes.
- [ ] 3. Implement the protocol client against a fake agent binary built in the tests. Verify: `go test ./service/agenthost/... -run TestProtocol` passes.
- [ ] 4. Implement process supervision: start, cancel, time limits, kill on shutdown, one process tree per session. Verify: `go test ./service/agenthost/... -run TestProcess` passes on Windows and in `bash scripts/test-linux-docker.sh ./service/agenthost/...`.
- [ ] 5. Issue the project-scoped token and pass the MCP address to the agent. Verify: `go test ./service/agenthost/... -run TestScopedToken` passes (a call to another project is refused).
- [ ] 6. Store transcripts and expose the service operations and the SSE stream. Verify: `go test ./service/... -run TestChat` and `go test ./web/contract/...` pass.
- [ ] 7. Build the panel: messages, tool calls, permission prompts, cancel, new session, profile picker; both languages; keyboard and screen reader labels. Verify: `task test:web` and `web/e2e/specs/agent-chat.spec.ts` pass with the fake agent.
- [ ] 8. Security tests of the new rows. Verify: `go test -run TestSecurity_ ./...` and `go run ./internal/tools/threatcheck` exit 0.
- [ ] 9. Manual run with one real agent installed on the author's machine, or marked as not verified. Verify: the transcript or the note is in the hand-off.

## Test strategy

- Unit: session and permission rules.
- Integration: a fake agent that speaks the protocol, edits the timeline through MCP, asks a permission, and misbehaves (hangs, floods, exits).
- Security: one `TestSecurity_` test per new row.
- End to end: a conversation that results in an edit visible on the timeline.

## Acceptance criteria

1. From any section of the dashboard the chat panel opens, starts a session with a configured profile and shows the agent's messages as they arrive.
2. An edit the agent makes appears in the project's operation log with the agent as the actor, and can be undone.
3. The agent's token cannot read or change another project.
4. A profile can be created or changed only from a privileged session; an agent credential is refused on every surface.
5. Closing Stuarty leaves no agent process behind (test).
6. Agent text containing markup is shown as text (test).
7. `go test ./core/chat/... ./service/agenthost/...`, `go test -run TestSecurity_ ./...` and `task test:web` pass.

## Risks and mitigation

- **The protocol is young and changes.** Mitigation: the client is one package; the version it speaks is recorded in SDD-017.
- **An agent profile is arbitrary code execution by design.** Mitigation: privileged sessions only; the UI shows the full command; never a profile from a project file or an import.
- **STOP** before adding a protocol library, and if a provider's terms forbid driving its agent this way.

## Size

L

## Hand-off for review

The diff of `epic/14-hosted-agent-chat`; the specification version; the fake agent and how to run it; the outputs of the acceptance commands; whether a real agent was verified.
