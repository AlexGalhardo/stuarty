# ADR-0007: The UI is a thin client; no editing logic in the browser

- Status: Accepted (2026-11-14)
- Deciders: architect; author (D-013)
- Links: [stack trade-offs §5](../search/stack-tradeoffs.md), ADR-0002, ADR-0013, SDD-006, SDD-012

## Context

Restriction 1 forbids duplicated editing logic. A browser editor normally keeps the project state and applies edits locally for responsiveness, which is exactly a second implementation. The briefing asks how much code web and desktop should share and whether the core should be shared with the UI. With D-003 the Go process runs on the user's machine or a self-hosted server, so a command round trip takes milliseconds.

## Decision

1. The browser holds a **read model**: the last project document received from the core, at a known revision, plus UI-only state (selection, zoom, scroll, open panels, playhead).
2. Every edit is a command batch sent to the core. The browser replaces its read model with the document returned, or with events from the server-sent event stream (ADR-0017).
3. During a pointer drag the browser may move or resize the dragged element visually. That is an **ephemeral preview**: it is never written to the read model, it uses only arithmetic on screen coordinates and snapping to values the core already supplied, and on release the browser sends one command and discards the preview in favour of the core's answer. If the command is rejected, the element returns to where it was and the error is shown.
4. The browser never computes a resulting timeline, never interpolates a keyframe, never decides which source frame is shown and never lays out text.
5. Every command of the catalogue has a UI entry point (D-013). A contract test lists the catalogue and fails for a command with none.
6. The desktop shell, when it exists, loads this same UI from the same Go process (ADR-0023). Web and desktop therefore share all code except the shell.

## Alternatives

- **Go core compiled to WebAssembly in the browser.** One implementation on both sides and instant response, at the cost of a multi-megabyte binary and awkward JavaScript interop, to solve latency that local-first does not have.
- **A TypeScript reducer in the browser kept in parity with Go by conformance tests.** Two implementations by design; parity tests catch only what they cover.
- **Optimistic updates applied to the read model.** The browser would need the effect of each command, which is the logic again.

## Consequences

Positive: the browser cannot diverge from the core, because it has nothing to diverge with. UI tests need no timeline logic.

Positive: the same UI works for a remote self-hosted server, with latency as the only difference.

Negative: on a slow network the UI waits for the server on every release. Local-first makes this rare; a remote server with high latency will feel it.

Negative: snapping during drag needs data from the core (snap points for the current revision), one more read endpoint.
