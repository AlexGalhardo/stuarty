# 32: Nested compositions

## Goal

A creator builds an intro once, as its own small timeline, and drops it into every video like a single clip. When they change the intro, every video that uses it changes too.

## Context and related decisions

- ADR-0042, ADR-0039 (a project holds several timelines), ADR-0029.
- Author decision D-065. PRD-001 RF-083.
- SDD-001, SDD-002, SDD-013 (the plan), SDD-004.

## Scope

- Modules: `core/timeline/` (the composition clip), `core/command/` (commands and golden cases), `core/plan/` (resolving a nested timeline into the plan), `engine/audio/` (mixing a nested timeline's audio), `engine/compositor/` and `renderer/` (only if the plan needs an intermediate surface), the timeline UI in `web/resources/js/editor/`, SDD-001, SDD-002, SDD-013.
- A composition is another timeline of the same project used as a clip.

## Out of scope

- Compositions shared between projects (a template covers that case).
- Time remapping curves on a composition clip beyond constant speed.
- Collapsing transformations in 3D, adjustment layers.

## Dependencies

06, 31.

## Technical design

- **Model**: a clip kind `composition` that references a `timeline_id` of the same project, with the usual clip fields (in and out points inside the nested timeline, transform, opacity, blend, effects, volume). The nested timeline keeps its own size and frame rate; the plan conforms it like any media (frame rate by nearest frame, size by the clip's framing).
- **Rules in the core**: no cycles (a timeline cannot contain itself at any depth) and a maximum depth of four, both checked when a command is applied and reported with a stable error; trashing a timeline that others use is refused with the list of users; the lint reports an empty or missing composition.
- **Plan**: `core/plan` resolves nesting. When the composition clip has no opacity, blend, effect or mask of its own, its layers are flattened into the parent's plan with composed transforms. Otherwise the plan carries a **group** whose layers are drawn to an intermediate surface that is then placed as one layer; the group is specified in SDD-013 before code, on the CPU path first, then the GPU path, with parity cases.
- **Audio**: the mixer mixes the nested timeline to a bus and treats the bus as the clip's audio, with the clip's volume and fades.
- **Commands**: `clip.add_composition`, `composition.create_from_selection` (moves the selected clips into a new timeline and replaces them with one composition clip, in one atomic batch), `composition.expand` (the inverse, when no group feature is in use).
- **Revisions**: a parent's render depends on the child's revision; caches are keyed by both, and an edit of the child invalidates the parents' previews through an event.
- **UI**: a composition clip shows the nested timeline's name and thumbnail; double click opens it in a tab with a breadcrumb back; "create composition from selection" in the context menu.

## Tasks

- [x] 1. Amend SDD-001, SDD-002 and SDD-013 (the clip kind, the rules, flattening, the group and its pixel rules). Verify: `task lint:md` exits 0 and the tests that read SDD-002 pass.
- [x] 2. Implement the clip kind, cycle and depth checks, and `clip.add_composition` with golden cases. Verify: `go test ./core/command/... ./core/timeline/...` passes.
- [x] 3. Implement flattening in `core/plan` with golden plans. Verify: `go test ./core/plan/... -run TestNested` passes.
- [x] 4. Implement the group on the CPU path of both executors that export uses today. Verify: `go test ./engine/compositor/... ./engine/renderhost/...` and `cargo test` in `renderer/` pass with new golden frames.
- [ ] 5. Implement the group on the GPU path and the browser executor in use. Verify: the parity tests of `renderer-parity` pass.
- [x] 6. Mix nested audio. Verify: `go test ./engine/audio/... -run TestNestedMix` passes.
- [x] 7. Implement `composition.create_from_selection` and `composition.expand` with golden cases. Verify: `go test ./core/command/... -run TestComposition` passes.
- [ ] 8. Key caches by child revision and emit the invalidation event. Verify: `go test ./service/... -run TestNestedInvalidation` passes.
- [ ] 9. Expose on the three surfaces; build the UI in both languages. Verify: `go test ./web/contract/...`, `task generate:check`, `task test:web` and `web/e2e/specs/nested-composition.spec.ts` pass.

## Test strategy

- Golden cases per command; golden plans for flattening; golden frames for groups.
- Property test: flattening a composition with identity properties equals inlining its clips.
- Preview and export parity on a project with two levels of nesting.
- End to end: create from selection, edit the child, see the parent change.

## Acceptance criteria

1. A timeline can be used as a clip in another timeline of the same project and exports correctly, picture and sound.
2. A cycle and a depth above four are refused with stable errors.
3. Editing the nested timeline changes the parent's preview and export without any edit to the parent.
4. A composition with opacity or a mask renders as one unit (golden frames), and preview matches export within tolerance.
5. `composition.create_from_selection` followed by undo restores the original clips exactly.
6. `go test ./core/... ./engine/... ./web/contract/...`, the Rust gate, `task test:web` and `task generate:check` pass.

## Risks and mitigation

- **The group is renderer work larger than this epic's size.** Mitigation: flattening ships first and covers compositions without group properties; **STOP** and report if the group cannot be specified within SDD-013's pixel contract, leaving those properties refused with a clear error.
- **Stale previews.** Mitigation: cache keys include the child's revision.

## Size

M

## Hand-off for review

The diff of `epic/32-nested-compositions`; golden plans and frames; parity results; the outputs of the acceptance commands; whether the group shipped.
