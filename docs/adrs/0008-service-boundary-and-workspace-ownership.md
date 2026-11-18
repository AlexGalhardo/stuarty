# ADR-0008: One application service; a workspace has one owner process

- Status: Accepted (2026-11-14); workspace ownership while the desktop window is closed is amended by [ADR-0046](0046-publishing-scheduler-and-blog-destinations.md) (2026-11-18)
- Deciders: architect
- Links: ADR-0002, ADR-0006, ADR-0015, ADR-0016, ADR-0017, SDD-003, SDD-005

## Context

Four surfaces reach the core: HTTP for the UI and the REST API, the MCP server, and the CLI. An agent in a terminal expects `vukem-video ...` to work with nothing else running; a person with the UI open expects an agent's edit to appear live. If the CLI wrote to the database while a server process also had it open, the server would not know about the change and the UI would go stale, and ordering of operations (ADR-0006) would have two authorities.

## Decision

1. `service/` defines one Go interface, `Service`, with the use cases: projects, edit batches, history, media, inspection, lint, view, jobs. Its implementation calls `core/` and `engine/` and depends on interfaces for persistence, the file store and the job runner.
2. Every surface is a thin adapter over `Service`: an HTTP handler, an MCP tool and a CLI command each parse input, call one `Service` method and format the result. No adapter contains editing logic or validation beyond shape.
3. A **workspace** is a directory (default `~/.vukem-video`, overridable by `VUKEM_VIDEO_HOME`) with the database, the content store, caches and exports.
4. A workspace has **one owner process** at a time. `vukem-video serve` takes an exclusive lock on the workspace and writes `runtime.json` with its process id and address; the local token is in a separate file (ADR-0019). The lock is held by the operating system and released when the process dies, so no heartbeat is needed (SDD-005 §6.2).
5. The CLI and the stdio MCP server obtain a `Service` in one of two ways, chosen automatically: if a server owns the workspace, an HTTP client of that server; otherwise, for a one-shot CLI command, the in-process implementation, taking the lock for the duration of the command; and for the stdio MCP server, which lives as long as an agent session, the process becomes the owner itself: it starts what `serve` starts and holds the lock for its lifetime, so that jobs, waiting and the UI all work during the session. Both ways satisfy the same interface, so commands behave the same.
6. In the in-process mode, jobs started by a command run inside that process; a command that starts a job waits for it by default.

## Alternatives

- **CLI always in-process, server polls the database.** Two writers, ordering by luck, stale UI between polls.
- **CLI always needs a running server.** Simple and consistent, but hostile to scripts and CI, where an agent wants one command to do the work.
- **A daemon started on demand.** Hides a long-lived process from the user and complicates shutdown and upgrades.

## Consequences

Positive: one place defines what the product can do; contract tests run the same scenarios against the in-process and the HTTP implementation of `Service`.

Positive: a human with the UI open sees an agent's CLI edits live, because they went through the server.

Negative: two ways to obtain a `Service` must both be tested, and the lock file must be robust to crashes (the lock is an operating-system file lock, released by the kernel when the owner dies; SDD-005 §6.2).

Negative: the HTTP API must cover every `Service` method from the start, since the CLI depends on it in server mode.
