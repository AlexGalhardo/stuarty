# PRD-002: Agents as first-class users

- Status: Accepted (2026-11-14)
- Depends on: [brainstorming](../brainstorming.md) D-001, D-011, D-012; [agents and MCP](../search/agents-mcp.md)
- Implemented by: SDD-002, SDD-003, SDD-005, SDD-006, SDD-007, SDD-011 (see [traceability](../traceability.md))

## 1. Problem

An agent that edits video has three difficulties a person does not. It cannot watch the result, so it needs another way to know what it did. It works from a limited context, so definitions and results must be small. And it reads untrusted text from the footage itself, which can try to give it orders. Existing editors treat agents as an add-on: a second API beside the real one, a browser that must stay open, or dozens of tools that mirror FFmpeg flags.

## 2. Users

| User | Surface | Typical session |
| --- | --- | --- |
| A coding agent with a shell (Claude Code, Codex, OpenCode) | CLI plus the shipped skill | Runs commands, reads JSON, loops until lint is clean, exports |
| An agent in a chat client without a shell | MCP over HTTP or stdio | Calls a dozen tools |
| A script or another program | HTTP API | Same operations, typed client generated from OpenAPI |

## 3. Use cases

1. "Cut the pauses, add captions and export for TikTok" given one input file, unattended.
2. "Make a 20 second commercial from this template with these three clips and this text."
3. A person asks an agent to fix one thing in a project they are editing in the UI, and watches it happen.
4. An agent resumes work on a project it has never seen, by reading its text description.

## 4. Functional requirements

Phase: `P` Prototype, `A` ALPHA, `B` BETA.

| ID | Requirement | Phase |
| --- | --- | --- |
| RF-100 | One command catalogue. Each command has a name, a JSON Schema for its arguments, a description, an example and its error codes, all retrievable at run time | P |
| RF-101 | An MCP server over stdio and over HTTP, with a small fixed set of tools named by intent | P |
| RF-102 | A CLI where every command can print one JSON document and uses exit code 0 for success, 1 for a permanent error and 2 for a transient error that is safe to retry | P |
| RF-103 | An HTTP API covering the same operations, described by an OpenAPI document | P |
| RF-104 | Every operation available on one surface is available on all of them (MCP, CLI, HTTP API) and in the UI, with the same arguments and results. The only exceptions are the administrative and stream operations listed, each with its reason, in SDD-003 §7.1 (for example emptying the trash, which no agent surface offers: RF-131) | A |
| RF-105 | Edits are sent as a batch of operations applied atomically as one undo step. A batch names the revision it was based on, may carry an idempotency key and may be a dry run that validates and returns the effect without committing | P |
| RF-106 | A text view of the timeline: one line per item, concise by default, full on request, with the track order stated | P |
| RF-107 | Every write returns the new revision, a text description of what changed and any new lint finding | P |
| RF-108 | A lint operation that reports problems as text: gaps and black frames, items of different tracks that cover each other by mistake, missing media and other disagreements between the document and the media store, text outside the safe area, unfilled template slots, audio that clips, loudness off target. Overlaps inside one track and clips longer than their source are not lint findings: no command can create them | P (first rules), A (all) |
| RF-109 | A view operation that returns one composed frame or a contact sheet with the time printed in each cell, produced by the export path | P |
| RF-110 | A media inspection operation that returns, as text, a file's properties and, when available, its transcript with times, its silences and its loudness | P (properties), A (rest) |
| RF-111 | Long work (ingest, transcription, export) returns a job identifier; a status operation can wait a short time; a job can be cancelled | P (job identifier, polling, waiting), A (cancellation) |
| RF-112 | Errors carry a stable code, a message that states what was wrong and what would be accepted, a hint for fixing it and whether a retry can succeed | P |
| RF-113 | Time values are strings with an explicit unit (seconds, frames or timecode). A bare number is rejected. Every answer states the frame applied | P |
| RF-114 | Every object has a stable identifier with a type prefix; nothing is addressed by position | P |
| RF-115 | The history of a project can be read as text, with the actor of each step; an actor can undo and redo its own steps | P |
| RF-116 | No result exceeds a fixed token budget; longer content is paged | P |
| RF-117 | The product ships a skill file, an `AGENTS.md` snippet and the MCP server instructions, generated from the catalogue and versioned with the binary | A |
| RF-118 | A human with the UI open sees an agent's changes as they are made, attributed to the agent | P |
| RF-119 | When two actors change the same object from the same revision, the later one receives a conflict with the current state instead of overwriting silently | A |

### Safety

| ID | Requirement | Phase |
| --- | --- | --- |
| RF-130 | No operation sends project data to an address chosen by the caller | P |
| RF-131 | No operation available to an agent destroys data irreversibly; deletes go to a trash | A |
| RF-132 | Exports, transcription and imports have configurable ceilings (concurrency, duration, resolution, file size); reaching one returns an actionable, retryable error | A |
| RF-133 | Text that came from media (transcripts, file names, metadata) is returned in delimited fields, truncated to a budget, and described as data | A |
| RF-134 | Every operation records its actor; the record cannot be edited | P |

## 5. Non-functional requirements

| ID | Requirement |
| --- | --- |
| NFR-020 | The MCP tool definitions total at most 6,000 tokens |
| NFR-021 | The concise timeline view is returned in pages of at most 4,000 tokens; a project of 100 items fits in one page and a project of 200 items in two |
| NFR-022 | A single frame returned to an agent costs at most 800 image tokens by default; a contact sheet at most 1,600 |
| NFR-023 | A frame is returned within 1 s and a contact sheet within 2 s (median) |
| NFR-024 | The CLI starts and answers a read command within 300 ms (median) when a server owns the workspace |
| NFR-025 | The same scenario produces the same project document whether driven through MCP, CLI or HTTP |

## 6. Success metrics

1. Contract tests run one scenario through all three surfaces and compare the resulting documents byte for byte.
2. An agent completes the Prototype scenario using only text feedback (no image) in at least 9 of 10 runs.
3. No tool in the catalogue has an outbound side effect; a test enumerates them.

## 7. Out of scope

| Feature | Why |
| --- | --- |
| Draft sessions that a human approves | Decision D-011 |
| Human confirmation prompts | Decision D-011 |
| A general "run code" escape hatch | Typed commands with undo are the product |
| Sampling, roots, prompts or resources as required MCP features | Uneven client support; sampling and roots are deprecated |
| An in-chat UI (MCP Apps) | The editor already has a UI on the same project |
| Agent-initiated publishing, webhooks, import by URL | Decision D-011 |
| A built-in agent or chat inside the editor | The product is operated by the user's own agent |
