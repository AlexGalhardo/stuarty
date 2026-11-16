# ADR-0006: Ordered operation log, optimistic concurrency, undo by compensation

- Status: Accepted (2026-11-14)
- Deciders: architect
- Links: engine §4, agents and MCP §7, SDD-002, SDD-006

## Context

A human and one or more agents edit the same project. Figma's published design uses an authoritative server that orders events and last-writer-wins per object property, and rejected both operational transforms and full CRDTs for its problem. FreeCut's agent API uses an expected revision and an idempotency key; Omnitool uses a base revision per patch; OpenChatCut applies an agent's batch as one undo step. Olive shows undo that does not restore derived state.

## Decision

1. The process that owns a workspace is authoritative. It applies operations one at a time per project and assigns each committed batch the next integer **revision**.
2. A write is a **batch** of one or more operations with: the actor, the `base_revision` it was planned against, and an optional client-chosen idempotency key. A batch is atomic.
3. If `base_revision` is the current revision the batch applies. If the project moved on, the batch applies only when no object it refers to by identifier was added, changed or removed by the intervening batches of other actors; otherwise it is rejected with `E_REVISION_CONFLICT`, the current revision and the diff since `base_revision`. The unit of conflict is the object, not the property: stricter than strictly needed, but simple to decide from the arguments and to explain to a caller (SDD-002 §A.7).
4. A repeated idempotency key returns the stored result of the first attempt.
5. Each committed batch is stored in an append-only log with its inverse operations, actor, timestamp and diff.
6. **Undo** is a new batch, issued by an actor, that applies the inverses of that actor's most recent batch not yet undone. If another actor changed one of the affected properties afterwards, that property is skipped and reported. Redo is symmetric.
7. Side effects (import, proxy generation, transcription, exported files) are not in the log. They produce immutable artifacts addressed by content hash; undoing an import removes the reference, not the file.
8. Anything shown in the preview derives from the document at a revision. No preview cache is maintained by hand.

## Alternatives

- **CRDT (Yjs, Automerge).** Solves offline and peer-to-peer merging, which this product does not have; the document becomes harder to read and audit.
- **Snapshots only.** Undo would be global with no notion of author, and auditing loses intent.
- **Locking the project per actor.** Safe and simple, but a human could not adjust while an agent works.

## Consequences

Positive: the log is the audit trail and the basis for reproducing any state.

Positive: an agent that planned against an old revision gets a precise conflict instead of silently overwriting a human edit.

Negative: object-level conflict detection must be implemented in the core and tested once (each command's referenced ids come from its arguments; SDD-002 §A.7).

Negative: the log grows without bound; compaction into snapshots is a later task.
