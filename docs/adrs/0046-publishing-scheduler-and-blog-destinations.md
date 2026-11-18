# ADR-0046: Publishing is a scheduled, approved job; blog posts have four destinations

- Status: Accepted (2026-11-18)
- Deciders: author
- Amends: ADR-0008 (who may own a workspace while the window is closed), ADR-0018 (jobs gain a scheduled start)
- Links: author decisions D-077, D-078, D-079; ADR-0032, ADR-0035, ADR-0039; PRD-004

## Context

A creator schedules posts. Publishing at a set time needs a running process: a server is always on, a desktop application is not. ADR-0008 allows one owner process per workspace. Blog posts are written in Stuarty too, and the author wants them to reach a blog without a manual copy.

## Decision

1. **A publication is a job** (ADR-0018) with a destination, an output piece, a text and an optional time. It passes through the approval queue of ADR-0032 before it runs, and it records the result returned by the destination.
2. **The Publish section shows a calendar and the approval queue.** One production can produce several publications, each with the format and the text of its destination.
3. **On a server the scheduler runs in the server process.**
4. **On a desktop a background service publishes at the set time with the window closed.** It is installed with the application, with the user's consent, and can be turned off. It is the workspace owner while the window is closed and hands ownership over, or serves the window, when the application opens; the epic settles that with ADR-0008 before code. A publication whose time passed while nothing ran is never sent late silently: it returns to the queue.
5. **Blog posts have four destinations from the start:** export as files (Markdown and HTML with their images); WordPress through its REST API; an outbound webhook that notifies a URL the user configured when a post is published; and Stuarty's own HTTP API, from which any system can read the posts.
6. **Metrics of published posts are a later epic.**

## Alternatives

- **Publish from a desktop only while the application is open.** Recommended by the orchestrator and rejected by the author: a schedule that silently depends on an open window is not a schedule.
- **Export blog posts only, with WordPress later.** Rejected by the author: the cycle would stay open for blogs.

## Consequences

Positive: a schedule holds on both targets.

Negative: a resident service per operating system (a Windows service or scheduled task, a launch agent, a systemd user unit) to install, update and support.

Negative: webhooks and WordPress are outbound destinations and follow every rule of ADR-0032, including signed webhook payloads and a visible connection.
