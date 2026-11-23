# Screen recording: execution order

The order of epics 61 to 73 by dependency: the critical path, what runs side by side and what forms the recording MVP. ADR-0045 has no phases or milestones, so the groups below only express dependencies.

## 1. Before anything

- The paused work is integrated, epic 57 lands and the author chooses a redesign direction.
- ADR-0049, ADR-0050 and ADR-0051 are accepted (author decision D-133).
- **Exception:** the capture spike, task 1 of epic 61, has no UI and can run now.

## 2. Dependency graph

```text
57 + redesign choice + ADR-0049, ADR-0051
        │
        ▼
      **61** ───► **62** ───┬───► **64**
        │                   ├───► **65**
        ▼                   ├───► 66
        63                  ├───► 67  (needs 43 and the signing decision)
                            └───► 68  (needs 43)

transcription today ───► 69 ───► 70 (needs ADR-0050) ───► 72 (BEN2 needs a licence decision)
existing commands   ───► 71
33 (done)           ───► 73
```

**Critical path of the recording MVP: 57, 61, 62, 64.** Epic 65 follows it as the first visible finishing. A delay in the Windows spike moves everything after it.

## 3. Groups by dependency

| Group | Epics | Needs |
| --- | --- | --- |
| A. Recording MVP on Windows | **61, 62, 64** | 57, redesign choice, ADR-0049, ADR-0051 |
| B. Finishing and reach | 65, 66, 63 | 62 (63 needs only 61) |
| C. Editing, independent of capture | 69, 71 | Can start with group A if modules do not intersect |
| D. Local AI | 70, then 72 | 69, ADR-0050; BEN2 needs the author's licence decision |
| E. Other systems | 67, 68 | 62 and epic 43, blocked today for lack of hardware |
| F. External motion | 73 | Epic 33 and an engine installed by the user; independent of every group above |

## 4. What can run side by side

At most five seniors at once, and never two epics on the same modules.

| Together | Why it is safe |
| --- | --- |
| 61 with 71 | 71 adds presets over existing commands |
| 62 with 69 | Capture modules against transcript modules |
| 65 with 63 | Renderer and core against browser upload, if `service/` recording is not touched by both at once |
| 66 with 70 | Layout presets against the AI helper |
| 67 with 68 | Separate backends in `capture/`, when hardware exists |
| 73 with any epic | It changes one skill and documents, unless another epic edits that skill |

Shared files (`go.mod`, `web/routes/`, `web/resources/lang/*.json`, `docs/sdds/000-shared-definitions.md`, `docs/plans/00-roadmap.md`) are merged one epic at a time.

## 5. The recording MVP

Epics **61, 62 and 64** deliver the MVP the author chose: on Windows, screen, microphone, system audio, webcam on its own track, pause, cursor and click events saved, direct placement on the timeline, with agents preparing and a person starting the capture.

## 6. Decisions that block the order

| Decision | Blocks |
| --- | --- |
| Rust in `capture/` and the crates (ADR-0049): **decided, D-133** | Nothing |
| Agent consent rules (ADR-0051): **decided, D-133** | Nothing |
| MCP budget, if an operation cannot fit existing tools | 61, 64 |
| Apple signing and notarization | 67 |
| Real macOS and Linux machines (epic 43) | 67, 68 |
| ONNX Runtime and model policy (ADR-0050): **decided, D-133** | Nothing |
| BEN2 weights licence | The BEN2 half of 72 |
