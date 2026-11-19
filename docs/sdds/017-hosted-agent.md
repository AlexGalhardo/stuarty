# SDD-017: The hosted agent chat

- Status: Draft (2026-11-19). Written by epic 14; the code follows in the same epic.
- Depends on: ADR-0037 (Stuarty hosts the user's agent over the Agent Client Protocol), ADR-0036 (the user's own access), ADR-0032 (approval), ADR-0020 (untrusted text), ADR-0033 (no credential leaves the vault), SDD-000 (names), SDD-003 (surfaces and the MCP server), SDD-005 (persistence), SDD-007 (threats T-70 to T-74, scopes, process policy, untrusted text), SDD-015 (the approval queue)
- Implements: PRD-004 RF-313, RF-380, RF-381, RF-382, RF-383 (the context part is the briefing and Memory the agent reads through its tools, section 6.3), RF-360 (the local agent as a way to connect)
- Owner area: `core/chat/`, `service/agenthost/`, the chat operations in `service/`, the controllers, the stream and the read commands in `web/app/`, the panel in `web/resources/js/dashboard/chat/`

## Contents

1. Overview
2. The protocol and the version
3. Agent profiles
4. The process
5. Sessions and their states
6. What the agent receives
7. Permission requests
8. Streaming
9. Transcripts and storage
10. Service operations, scopes and surfaces
11. Errors and limits
12. Network deployments
13. Tests

## 1. Overview

A creator talks to the agent they already use (Claude Code, Codex, OpenCode or another that speaks the Agent Client Protocol) in a panel of the right dock that is always one click away, and watches the agent work on the open project. The agent is a **separate program the user installed**, with the login the user gave it. Stuarty starts it as a child process, speaks to it over its standard streams, and gives it the address of Stuarty's own MCP server with a token for the open project. Four rules carry the design:

1. **Stuarty has no agent loop.** It sends the person's text to the agent and shows what comes back. It chooses no tool, runs no tool on a model's behalf and calls no model (ADR-0037 item 3).
2. **The agent acts through the public surfaces.** Every edit goes through the MCP server, so it is an operation with the agent as its actor, undoable, and bound by the approval queue (ADR-0037 item 2, ADR-0032). The chat has no private channel to the command core.
3. **A person drives the chat.** Starting a session, sending, answering a permission request, cancelling, deleting and writing a profile need a person in a browser session (`Caller.Browser`). The local token, a key and an agent surface are refused, so a hosted agent cannot start another agent, answer its own permission request or write the command of its own profile (section 10).
4. **What the agent writes is untrusted text.** It is stored and sent as strings, shown as text nodes, never as HTML, with links shown in full (SDD-007 section 8).

Stuarty never bundles, downloads, installs, updates, renames or signs in to an agent. It never reads the agent's credentials and never offers an Anthropic subscription login (D-059): the user signs in inside the agent, as always. The names of vendors and agents appear only as plain text.

The rules of a session, of a permission request and of a transcript are in `core/chat/` as pure functions. `service/agenthost/` holds the process, the protocol client and the sessions behind small interfaces (a store, a token issuer, an event sink). The operations are methods of `service.Service`, in the domain `workspace` (SDD-000 section 1.1).

## 2. The protocol and the version

The client speaks **Agent Client Protocol, protocol version 1** (JSON-RPC 2.0, one message per line over the standard streams of the child, UTF-8, no embedded newline). The version is the constant `agenthost.ProtocolVersion`; the client is one package so that a change of the protocol is a change of one place. The client is written against the published specification, with no library.

Messages the client sends:

| Method | When | Fields used |
| --- | --- | --- |
| `initialize` | right after the start | `protocolVersion`, `clientCapabilities` (**empty**: no `fs`, no `terminal`; the agent uses its own tools and Stuarty's MCP tools), `clientInfo` (`name` `stuarty`, `version`) |
| `session/new` | after `initialize` | `cwd` (section 4), `mcpServers` (section 6) |
| `session/prompt` | one per turn | `sessionId`, `prompt` (one `text` block) |
| `session/cancel` | the person cancels, or a limit ends the turn | `sessionId`; the client answers every pending permission request with the outcome `cancelled` |

Messages the client receives and uses:

| Method | Handling |
| --- | --- |
| `session/update` | `agent_message_chunk`, `agent_thought_chunk`, `tool_call`, `tool_call_update` and `plan` are turned into transcript events (section 8). Any other `sessionUpdate` is ignored, not an error: the protocol grows |
| `session/request_permission` | becomes a permission request (section 7); the reply is sent when a person answers, or `cancelled` |
| any other request from the agent (`fs/*`, `terminal/*`) | answered at once with the JSON-RPC error `-32601` (method not found): Stuarty offered no such capability |

The response of `initialize` is checked: `protocolVersion` must be 1 (otherwise the start fails with `E_AGENT_UNAVAILABLE`, `details.reason` `protocol_version`), and `agentCapabilities.mcpCapabilities.http` must be true, because the MCP server is reached over HTTP with a header (section 6; otherwise `details.reason` `no_http_mcp`). An agent that answers `session/new` with an authentication error (`-32000`, or a non-empty `authMethods` it insists on) makes the start fail with `details.reason` `sign_in_required`, and the panel says to sign in inside the agent first. A line longer than 1 MiB, a message that is not JSON-RPC, or a response to a request that was never made ends the session (reason `protocol`).

## 3. Agent profiles

A **profile** is what makes a program a chat agent: a name, an executable and its arguments. It is arbitrary code execution by design (ADR-0037 item 6, SDD-007 T-70), so:

- It is written only by a person in a browser session with the `admin` scope: `AgentProfileSet` and `AgentProfileRemove` answer `E_PRIVILEGED_SESSION` to everyone else (the local token, every key, MCP, the CLI). A profile is never read from a project file, a bundle, a template, a skill or an import.
- `name` is `[a-z0-9][a-z0-9-]{0,31}`. `command` is one executable: an absolute path, or a bare name that is resolved **when the profile is written** with the operating system's search (`exec.LookPath`), and the absolute path found is what is stored and shown. A relative path with a separator is refused. The file must exist and be a regular file. At most 1024 characters, no control character. `args` is at most 32 strings of at most 512 characters, no control character, no NUL. There is no shell: the program and its arguments are passed as an array, never joined (SDD-007 section 6.1).
- A profile has no environment and no working directory of its own: the environment is section 4's, the working directory is the session's.
- The UI shows the **full command** (path and arguments) of a profile before it is saved and wherever it is listed.
- A workspace holds at most 16 profiles.

Stuarty ships **suggestions**, not profiles: a fixed list in `core/chat` of known agents with the executable names a person would look for and the arguments that put the agent in ACP mode, plus a sentence on how the user installs it. `AgentProfileList` marks which suggestion's executable was found on this machine's search path, so the panel can offer to create the profile with the full command shown. A suggestion is text; nothing is downloaded or run to build it.

```text
agent_profiles(name PRIMARY KEY, command, args_json, created_by, created_ms, updated_ms)
```

## 4. The process

`service/agenthost` starts the profile's program with `os/exec` as one child per session, under the process policy of SDD-007 section 6.1 (no shell, an argument array, no user text in it):

- **Working directory**: `<workspace>/chat/<project id>/`, created with mode 0700. It is inside the workspace and holds nothing of the user's; the agent's own file permissions are its own (ADR-0037, negative consequences).
- **Environment**: built from an allowlist of what a program needs to find its own configuration and sign-in: `PATH`, `HOME`, `USERPROFILE`, `APPDATA`, `LOCALAPPDATA`, `SYSTEMROOT`, `COMSPEC`, `PATHEXT`, `TEMP`, `TMP`, `TMPDIR`, `LANG`, `LC_*`, `XDG_*`, `USER`, `USERNAME`, `LOGNAME`, `SHELL`, `TERM`, `HOMEDRIVE`, `HOMEPATH`, `PROGRAMDATA`, `PROGRAMFILES`, `PROGRAMFILES(X86)`, `ALLUSERSPROFILE`. Nothing else is passed: no `STUARTY_*` variable, no `APP_KEY`, no master key, no local token, no cloud or provider key of the server's environment. The MCP token goes to the agent in the protocol message of section 6, never in the environment. An agent that needs an API key in its environment is outside this version (the user signs in inside the agent).
- **Streams**: standard input and output are the protocol; standard error is drained into a ring of 8 KiB that is used only for the diagnostic of a failed start (never shown as chat text, never logged above debug).
- **One process tree per session**: on Unix the child leads its own process group and the whole group is killed; on Windows the child is placed in a job object that kills every member when closed. Ending a session closes the child's standard input, waits 3 seconds, then kills the tree.
- **Time limits**: the start (the child launched, `initialize` and `session/new` answered) 30 seconds; a turn 15 minutes (the client sends `session/cancel`, waits 5 seconds for the turn to end, then kills the tree and ends the session, reason `timeout`); a permission request that no person answers for 10 minutes is answered `cancelled`; a session with no turn for 30 minutes ends (reason `idle`); a session lives at most 8 hours.
- **Flood limits**: a line of at most 1 MiB; at most 5000 transcript events per session and at most 50000 characters per event (the rest is cut and marked); more than 200 events per second for 5 seconds ends the session (reason `flood`).
- **Shutdown**: closing the server ends every session, kills every tree, and returns only when no child of Stuarty is left (test: `TestProcessShutdownLeavesNoChild`). A crashed server does not leave the child behind on Windows, where the job object ends it with the last handle; on Unix the child reads the end of its input and exits by itself (a program that ignores it stays, which is a residual risk of SDD-007 section 14), and the next start marks the session `ended` with reason `shutdown`.
- **Concurrency**: at most 4 live sessions per workspace and one per project. Starting a session in a project that has one ends the old one first.

## 5. Sessions and their states

A session is one conversation of one project with one profile; its id is `cht_` and 10 characters (SDD-000 section 2). States (`core/chat`):

| State | Meaning | Leaves by |
| --- | --- | --- |
| `starting` | the child is launched and the protocol is being opened | the start succeeds (`idle`), or fails (`ended`) |
| `idle` | the agent waits for a message | a message (`running`), an end (`ended`) |
| `running` | a turn is in progress | the turn ends (`idle`), a permission request arrives (`awaiting_permission`), a cancel (`idle` after the agent confirms, or `ended` when it does not), an end |
| `awaiting_permission` | the agent asked before a tool call and waits | an answer (`running`), a cancel, an end |
| `ended` | the process is gone; the transcript stays; `end_reason` is one of `user`, `exited`, `protocol`, `timeout`, `idle`, `flood`, `shutdown`, `start_failed` | nothing |

A message sent while the state is not `idle` is `E_INVALID_STATE` with `details.reason` `busy` (or `ended`). The transitions are one pure function, `chat.Next`, with a table test.

## 6. What the agent receives

### 6.1 The MCP server

`session/new` carries one MCP server entry of type `http`: the `url` of Stuarty's own MCP endpoint (`<origin>/mcp`, SDD-003 section 4.5) and one header `Authorization: Bearer <token>`. The token is the **session token**:

- Issued when the session starts by an in-memory registry (`auth.AgentTokens`); it is never stored, never logged and never returned by any operation. The secret has the form `stc_` and 43 characters of base64url (256 bits), compared by its SHA-256.
- It is **bound to the project** of the session: the caller it yields has `ProjectID` set, so every operation that resolves a project other than that one answers `E_FORBIDDEN`, and the project list shows only that project (SDD-007 section 5.2, T-06 and T-72).
- Its scopes are `read`, `edit`, `media` and `render`: never `admin`. It cannot create a key, a connection, a rule or a profile, and cannot empty the trash.
- Its actor is `local:chat-<10 characters of the session id>` in local mode and `key:<session id>` in network mode, so an edit shows in the operation log with the agent as the actor (RF-381), and undo and redo are scoped to it.
- Its owner is the person who started the session (network mode: `OwnerUserID`), so the visibility rules of users hold.
- It dies when the session ends, when the server stops, or after 8 hours.
- It is accepted by `POST /mcp` and by the other HTTP routes for the project it is bound to, behind the same guard (peer, Host, Origin) as any credential. It is **refused for the operations of a person** (section 10) whatever its scopes.

The approval queue applies as to any agent: a publication or a costly generation the agent starts waits for a person in the Approvals page, never in the chat (section 7). The session token is not a way around ADR-0032.

### 6.2 The first message

The first `session/prompt` of a session carries two text blocks: a fixed sentence, then the person's message. The sentence names the project (its id, and its name as a quoted string) and tells the agent to read the project's briefing and the relevant Memory with its tools (`project_get`, `library`). Nothing is sent before the person writes: opening the panel starts no turn and costs the user no usage. Stuarty does not paste the briefing or the notes into the prompt: the agent reads them as data through the MCP server, which delimits them as untrusted text (SDD-007 section 8). That is how RF-383 is met without the user pasting anything.

## 7. Permission requests

The agent asks before a tool call that the protocol marks as needing consent (`session/request_permission`). It is the agent's own consent model, shown to the person; it is **not** Stuarty's approval queue:

- The request becomes the transcript event `permission_request` with a request id (`cpr_` and 10 characters), the tool call's title and kind as text, and the options the agent offered (`optionId`, `name`, `kind`).
- A person may answer `allow` or `reject`, or cancel. `allow` selects the option of kind `allow_once`; `reject` the one of kind `reject_once`. **A standing choice (`allow_always`, `reject_always`) is never selected for the person**: a permission that outlives the call is a rule, and rules are written in the Approvals page (SDD-015). If the agent offered no `allow_once`, `allow` is refused (`E_VALIDATION`, `details.reason` `no_one_time_option`); if it offered no `reject_once`, `reject` is sent as the outcome `cancelled`. The function is `chat.PickOption`.
- A request is answered once: a second answer is `E_INVALID_STATE`. A request of a session that ended is dead.
- **Approving a publication or a costly generation never happens here.** Every permission prompt of the panel carries the fixed sentence "Publications and costly generations are decided in Approvals, not here" with a link to the page; the chat answers only the agent's consent to run the call, and a call that needs approval then waits in the queue. The chat has no operation that decides an approval request.

## 8. Streaming

Everything the agent does reaches the panel as **events** over Server-Sent Events, `GET /chat/sessions/{id}/events` (read scope, the same guard and the same revalidation of authorization as the project stream, SDD-006). An event is a JSON object:

| Field | Meaning |
| --- | --- |
| `seq` | the number of the event in the session, from 1, increasing |
| `kind` | `user_message`, `agent_message`, `agent_thought`, `tool_call`, `permission_request`, `permission_answer`, `plan`, `notice`, `state` |
| `at` | the time, RFC 3339 UTC |
| `text` | the text, at most 50000 characters; never markup (the client shows text nodes) |
| `append` | true when `text` continues the event with the same `seq` (a chunk of a message the stream is still writing) |
| `tool` | for `tool_call`: `tool_call_id`, `title`, `tool_kind`, `status` (`pending`, `in_progress`, `completed`, `failed`) and `output` (the text blocks of its content, joined and clipped) |
| `permission` | for `permission_request` and `permission_answer`: `request_id`, `options`, `answer` |
| `state` | for `state`: the session state and `end_reason` |

The SSE frame's `id` is `seq`, so a client that reconnects with `Last-Event-ID` is sent what it missed: the stream first replays the stored events after that number, then follows the live ones, subscribing **before** it reads the store so no event falls between. Chunks of one message are one event whose text grows: the live stream sends each chunk with `append` true, the store keeps the whole text. A heartbeat keeps the connection alive; an unauthorized caller is ended at the next revalidation. At most 8 chat streams per credential.

Agent text is never interpreted by Stuarty: no HTML, no Markdown rendering, no link made clickable from it. The panel shows a URL as text in full, with the host visible (T-71).

## 9. Transcripts and storage

Transcripts are stored **per project** in the database file of the workspace:

```text
chat_sessions(id PRIMARY KEY, project_id REFERENCES projects ON DELETE CASCADE, profile, actor_user,
              state, end_reason, title, created_ms, updated_ms, ended_ms, event_count)
chat_events(session_id REFERENCES chat_sessions ON DELETE CASCADE, seq, kind, at_ms, body_json,
            PRIMARY KEY (session_id, seq))
```

`body_json` is the event of section 8 without `seq`, `kind` and `at`. No credential is stored: the session token is never written, and no table holds an environment, a header or a command line other than the profile's command. A session left `starting`, `idle`, `running` or `awaiting_permission` by a server that died is marked `ended` with reason `shutdown` at the next start.

- `ChatTranscriptGet` returns the events (paged by `after`, at most 500) as JSON, or **the whole transcript as Markdown** (`format` `markdown`): a heading per session, then one block per event with the speaker and the time, every agent text inside a fenced code block whose fence is longer than any run of backticks in the text, so exported text cannot become headings, links or HTML when someone opens the file. The Markdown is built by `chat.RenderMarkdown`.
- `ChatSessionDelete` ends the session if it is live and deletes its rows. Trashing and emptying a project follows the project: the rows go with it (`ON DELETE CASCADE`).
- At most 200 sessions per project are kept; the oldest ended one is deleted when a new one starts.

## 10. Service operations, scopes and surfaces

All are in the domain `workspace`. A "person" is `Caller.Browser` (a browser session; in network mode a logged-in user). Names are in SDD-000 section 1.

| Operation | HTTP | Scope and caller |
| --- | --- | --- |
| `AgentProfileList` | `GET /agents/profiles` | `read`. The profiles with their full command, and the suggestions with whether each was found |
| `AgentProfileSet` | `PUT /agents/profiles/{name}` | `admin` and a person |
| `AgentProfileRemove` | `DELETE /agents/profiles/{name}` | `admin` and a person |
| `ChatSessionStart` | `POST /projects/{id}/chat/sessions` | `edit` and a person; the project check of SDD-007 section 5.2 |
| `ChatSend` | `POST /chat/sessions/{id}/messages` | `edit` and a person |
| `ChatCancel` | `POST /chat/sessions/{id}/cancel` | `edit` and a person; `end` true also ends the session |
| `ChatAnswerPermission` | `POST /chat/sessions/{id}/permissions/{request_id}` | `edit` and a person |
| `ChatSessionList` | `GET /projects/{id}/chat/sessions` | `read`; project check |
| `ChatTranscriptGet` | `GET /chat/sessions/{id}/transcript` | `read`; the project check of the session's project |
| `ChatSessionDelete` | `DELETE /chat/sessions/{id}` | `edit` and a person |
| the event stream | `GET /chat/sessions/{id}/events` | `read`; the project check |

- A caller that is not a person gets `E_PRIVILEGED_SESSION` before the scope is checked, so an agent learns one thing (as in SDD-015 section 4). The session token of section 6 is such a caller: a hosted agent cannot start, steer, cancel, answer or delete a chat, nor write a profile.
- A project-limited caller sees only the sessions of its project; a session of another project is `E_CHAT_NOT_FOUND`, the same answer as for an id that does not exist. In network mode a user sees only the sessions of projects they may see.
- **CLI**: the reads only, `agent:profiles`, `chat:sessions` and `chat:transcript` (with `--format markdown`). **MCP**: no tool. A hosted agent has no use for the chat of another agent, and a tool that reads a transcript would hand one agent's text to another as instructions (SDD-007 T-44's reasoning); the person reads transcripts in the panel and through the CLI. The writes have no CLI command: they are for a person.
- With the audit: starting, ending and deleting a session and writing or removing a profile are recorded in `audit` with the actor, the profile name and, for a profile, the full command. A transcript text is never copied into the audit.

## 11. Errors and limits

| Code | When | HTTP |
| --- | --- | --- |
| `E_AGENT_NOT_FOUND` | the profile does not exist (`ChatSessionStart`, `AgentProfileRemove`) | 404 |
| `E_CHAT_NOT_FOUND` | the session does not exist, or the caller may not see its project | 404 |
| `E_AGENT_UNAVAILABLE` | the agent could not be started or opened; `details.reason`: `not_found` (the executable is gone), `start_failed`, `exited`, `timeout`, `protocol`, `protocol_version`, `no_http_mcp`, `sign_in_required`, `limit` (the live-session limit) | 503 |
| `E_VALIDATION` | a name, a command, an argument or a text outside the rules (`details.reason` `profile_name`, `command`, `args`, `text_empty`, `text_too_long`, `answer`, `no_one_time_option`, `profile_limit`) | 400 |
| `E_INVALID_STATE` | a message while a turn runs or the session ended; a second answer; `details.reason` `busy`, `ended`, `answered` | 409 |
| `E_PRIVILEGED_SESSION`, `E_FORBIDDEN`, `E_UNAUTHORIZED` | SDD-000 section 8 | 403, 403, 401 |

The limits, all constants of the code (none is a variable): a message of the person at most 20000 characters; 16 profiles; 4 live sessions; one per project; 200 sessions kept per project; 5000 events per session; the times and floods of section 4.

## 12. Network deployments

The agent runs **where the server runs**: it is a child of the server process, with the server machine's executables and the server user's sign-in. Consequences:

- The panel is available only when a profile exists on that machine. With none, the panel says so, lists the suggestions with what was found on the server's search path, and, for an administrator in a browser session, offers to create a profile with the full command shown. A member who is not an administrator sees "ask the administrator to connect an agent".
- Writing a profile needs the `admin` scope and a logged-in browser session. In network mode that is an administrator; a member cannot write a command that will run on the server (T-70).
- The session token carries the starter's `OwnerUserID` and is bound to the project; a member's agent cannot reach another member's project (T-72).
- The MCP address given to the agent is the server's own address (`APP_URL`, section 6.1) so that the host and origin rules of network mode (SDD-007 section 4.3) accept it. When the agent runs on the same machine this is a loopback request through the proxy or the listener.
- Local mode: the address is `http://127.0.0.1:<port>/mcp`, the port the server listens on.

## 13. Tests

- `core/chat`: table tests of the state machine, the profile rules, the option picker, the clipping of text, the Markdown renderer (a text full of backticks, headings and HTML stays inside its fence).
- `service/agenthost`: the protocol client against a **fake agent** (`service/agenthost/fakeagent`, a program the tests build): `TestProtocol_*`; the process supervisor: `TestProcess_*` (start, cancel, time limits, a hanging agent, a flooding agent, an exiting agent, shutdown that leaves no child); the token: `TestScopedToken_*` (a call for another project is refused; the token dies with the session).
- `service`: `TestChat_*` over a real store: the operations, the person rule, the project rule, the transcript, the Markdown export, the operation log with the agent as the actor and its undo.
- Security, one test per row of SDD-007 that this SDD adds: `TestSecurity_T70_*` to `TestSecurity_T74_*`.
- End to end: `web/e2e/specs/agent-chat.spec.ts` with the fake agent as the profile: open the panel from any section, start a session, see the messages arrive, answer a permission request, see the edit on the timeline, cancel, export the transcript.
- A run with a real agent installed is a manual check recorded in the hand-off, never a CI test, and never a paid call made by a test.
