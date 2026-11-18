# PRD-002: Agents as first-class users

- Status: Accepted (2026-11-14); revised on 2026-11-18 for Stuarty (ADR-0031, ADR-0032, ADR-0037, ADR-0043): the phase column was removed, RF-120, RF-121 and RF-135 to RF-137 were added, RF-130 was rewritten and section 7 was updated
- Depends on: author decisions D-001, D-011, D-012, D-044, D-052, D-060, D-069; research note `agents-mcp`
- Implemented by: SDD-002, SDD-003, SDD-005, SDD-006, SDD-007, SDD-011; the requirements added on 2026-11-18 are specified by the epics that build them (`docs/plans/00-roadmap.md`)

## 1. Problem

An agent that produces content has three difficulties a person does not. It cannot watch the result, so it needs another way to know what it did. It works from a limited context, so definitions and results must be small. And it reads untrusted text, from the footage itself and from the network, which can try to give it orders. Existing tools treat agents as an add-on: a second API beside the real one, a browser that must stay open, or dozens of tools that mirror FFmpeg flags.

## 2. Users

| User | Surface | Typical session |
| --- | --- | --- |
| A coding agent with a shell (Claude Code, Codex, OpenCode) | CLI plus the shipped skill | Runs commands, reads JSON, loops until lint is clean, exports |
| The same agent hosted in the chat panel of Stuarty (ADR-0037) | MCP | Works on the project the person has open |
| An agent in a chat client without a shell | MCP over HTTP or stdio | Calls a dozen tools |
| A script or another program | HTTP API | Same operations, typed client generated from OpenAPI |

## 3. Use cases

1. "Cut the pauses, add captions and export for TikTok" given one input file, unattended.
2. "Make a 20 second commercial from this template with these three clips and this text."
3. A person asks an agent to fix one thing in a project they are editing in the UI, and watches it happen.
4. An agent resumes work on a project it has never seen, by reading its text description.
5. "Make the thumbnail, write the caption of the post and schedule it for tomorrow": the agent prepares everything and the publication waits for the person's approval.

## 4. Functional requirements

Every requirement below applies to each domain of Stuarty (ADR-0031), not only to the video timeline.

| ID | Requirement |
| --- | --- |
| RF-100 | One command catalogue per domain. Each command has a name, a JSON Schema for its arguments, a description, an example and its error codes, all retrievable at run time |
| RF-101 | An MCP server over stdio and over HTTP, with a small fixed set of tools named by intent, each taking the domain it applies to (ADR-0043) |
| RF-102 | A CLI where every command can print one JSON document and uses exit code 0 for success, 1 for a permanent error and 2 for a transient error that is safe to retry |
| RF-103 | An HTTP API covering the same operations, described by an OpenAPI document |
| RF-104 | Every operation available on one surface is available on all of them (MCP, CLI, HTTP API) and in the UI, with the same arguments and results. The only exceptions are the administrative and stream operations listed, each with its reason, in SDD-003 §7.1 (for example emptying the trash, which no agent surface offers: RF-131) |
| RF-105 | Edits are sent as a batch of operations applied atomically as one undo step. A batch names the revision it was based on, may carry an idempotency key and may be a dry run that validates and returns the effect without committing |
| RF-106 | A text view of a document: one line per item, concise by default, full on request, with the order stated |
| RF-107 | Every write returns the new revision, a text description of what changed and any new lint finding |
| RF-108 | A lint operation that reports problems as text: gaps and black frames, items of different tracks that cover each other by mistake, missing media and other disagreements between the document and the media store, text outside the safe area, unfilled template slots, audio that clips, loudness off target. Overlaps inside one track and clips longer than their source are not lint findings: no command can create them |
| RF-109 | A view operation that returns one composed frame or a contact sheet with the time printed in each cell, produced by the export path |
| RF-110 | A media inspection operation that returns, as text, a file's properties and, when available, its transcript with times, its silences and its loudness |
| RF-111 | Long work (ingest, transcription, export, generation, publication) returns a job identifier; a status operation can wait a short time; a job can be cancelled |
| RF-112 | Errors carry a stable code, a message that states what was wrong and what would be accepted, a hint for fixing it and whether a retry can succeed |
| RF-113 | Time values are strings with an explicit unit (seconds, frames or timecode). A bare number is rejected. Every answer states the frame applied |
| RF-114 | Every object has a stable identifier with a type prefix; nothing is addressed by position |
| RF-115 | The history of a document can be read as text, with the actor of each step; an actor can undo and redo its own steps |
| RF-116 | No result exceeds a fixed token budget; longer content is paged |
| RF-117 | The product ships a skill file per domain, an `AGENTS.md` snippet and the MCP server instructions, generated from the catalogues and versioned with the binary |
| RF-118 | A human with the UI open sees an agent's changes as they are made, attributed to the agent |
| RF-119 | When two actors change the same object from the same revision, the later one receives a conflict with the current state instead of overwriting silently |
| RF-120 | An actor can revert a document to an earlier revision as one compensating batch of its own batches, which is itself undoable |
| RF-121 | A revision can be given a name (a checkpoint), and the difference between two revisions can be read as text |

### Safety

| ID | Requirement |
| --- | --- |
| RF-130 | No operation sends data to an address chosen by its caller. Data leaves the machine only towards a destination the user connected (ADR-0032) |
| RF-131 | No operation available to an agent destroys data irreversibly; deletes go to a trash |
| RF-132 | Exports, transcription, imports, generation and publication have configurable ceilings (concurrency, duration, resolution, file size, cost); reaching one returns an actionable, retryable error |
| RF-133 | Text that came from media or from the network (transcripts, file names, metadata, provider output, fetched pages) is returned in delimited fields, truncated to a budget, and described as data |
| RF-134 | Every operation records its actor; the record cannot be edited |
| RF-135 | An agent can prepare a publication or a costly generation but cannot run it: the request becomes an item of an approval queue that a person approves or rejects, unless a rule created by that person covers it (ADR-0032) |
| RF-136 | An approval rule can be created, changed or removed only from a privileged browser session, never by an agent; every use of a rule is audited |
| RF-137 | No tool, command or endpoint returns a credential, and an agent cannot read the vault (ADR-0033) |

## 5. Non-functional requirements

| ID | Requirement |
| --- | --- |
| NFR-020 | The MCP tool definitions total at most 6,500 tokens. The limit is revised once, by the epic that adds the first new domain (ADR-0043), and stays checked by a test |
| NFR-021 | The concise timeline view is returned in pages of at most 4,000 tokens; a project of 100 items fits in one page and a project of 200 items in two |
| NFR-022 | A single frame returned to an agent costs at most 800 image tokens by default; a contact sheet at most 1,600 |
| NFR-023 | A frame is returned within 1 s and a contact sheet within 2 s (median) |
| NFR-024 | The CLI starts and answers a read command within 300 ms (median) when a server owns the workspace |
| NFR-025 | The same scenario produces the same document whether driven through MCP, CLI or HTTP |

## 6. Success metrics

1. Contract tests run one scenario through all three surfaces and compare the resulting documents byte for byte.
2. An agent completes the reference scenario of PRD-001 using only text feedback (no image) in at least 9 of 10 runs.
3. A test enumerates every tool and command with an outbound effect and proves that each one goes through a connection and, where ADR-0032 says so, through the approval queue.

## 7. Out of scope

| Feature | Why |
| --- | --- |
| A general "run code" escape hatch | Typed commands with undo are the product |
| Sampling, roots, prompts or resources as required MCP features | Uneven client support; sampling and roots are deprecated |
| An agent built into Stuarty | The chat hosts the user's own agent (ADR-0037) |
| A private channel for the hosted agent | It uses the same public surfaces as any other agent (ADR-0037) |
| Import by URL chosen by the caller | It would let an operation reach an address its caller picked (RF-130) |
| Confirmation prompts for edits inside the product | Edits are undoable; only publishing and costly generation wait for a person (ADR-0032) |
