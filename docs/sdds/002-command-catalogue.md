# SDD-002: Command catalogue of the core

- Amended (2026-11-18) for Stuarty: this is the catalogue of the video domain. Other domains get their own catalogue (ADR-0031, ADR-0038).
- Status: Draft for freeze (2026-11-14). Parts A and B are the contract; Part C details each command. Part C is frozen: all 86 commands have golden cases and pass the registry-wide gates. The binary exposes all 86 commands, 85 of them public (epic 32 added three, C.84 to C.86); `font.add` and `font.remove` have scope `media` like `asset.add` and `asset.remove`.
- Depends on: SDD-001 (document), ADR-0002 (command core), ADR-0004 (time), ADR-0006 (log, concurrency, undo), ADR-0014 (JSON Schema)
- Implements: RF-010 to RF-016, RF-020 to RF-026, RF-030 to RF-036, RF-040, RF-041, RF-050, RF-051, RF-100, RF-105, RF-107, RF-112 to RF-115, RF-119, NFR-001, NFR-025
- Owner area: senior-a (command core and schema)

## Part A: the model

### A.1 Vocabulary

| Term | Meaning |
| --- | --- |
| **Command** | A named kind of edit in the catalogue, for example `item.split`. Defined once in `core/command` |
| **Operation** | One use of a command: `{"op": "<command>", "args": {...}}` |
| **Batch** | An ordered list of operations applied atomically as one revision and one undo step |
| **Service operation** | Something the product does that is not an edit of the document: create a project, import media, export, undo. Defined in SDD-003, not here |

### A.2 The registry

`core/command` exposes a registry. For each command it holds:

| Field | Purpose |
| --- | --- |
| `name` | `namespace.action`, lower case, stable |
| `summary` | one sentence, used as tool and help text |
| `description` | a short paragraph for `command_help` |
| `args_schema` | JSON Schema of `args`, generated from a Go struct; `additionalProperties: false` |
| `example` | a valid `args` value |
| `errors` | the error codes the command can return |
| `scope` | `edit` or `media` (ADR-0019) |
| `internal` | true when only the service may issue it (see A.9) |
| `apply` | the function of A.3 |

`Registry.List()`, `Registry.Get(name)` and `Registry.Schema(name)` are what every surface uses. Nothing else describes a command.

### A.3 Apply

```go
// Apply is pure: no clock, no randomness, no I/O.
func (r *Registry) Apply(doc *timeline.Document, op Operation, env Env) (Result, error)

type Env struct {
    NewIDs    func(prefix string) string // deterministic in tests; random in production
    Timelines timeline.Graph             // the other timelines of the production (epic 32); nil when the caller has none
}

type Result struct {
    Doc       *timeline.Document // new document; the input is not modified
    Inverse   []Primitive        // undo, section A.6
    Diff      []Change           // section A.5
    Effective map[string]any     // values actually applied, section A.4
    Spawned   []*timeline.Document // timelines the command made besides Doc (composition.create_from_selection); the caller stores them
}

// timeline.Graph is what the three composition commands read of the production: pure lookups the caller
// answers from its own store. The documents it returns are read-only.
type Graph interface {
    Doc(id string) (*timeline.Document, bool) // a timeline of the production by its id
    Users(id string) []string                 // the ids of the timelines that hold a composition of id, sorted
}
```

`Env.Timelines` is how a command that places one timeline in another stays pure: it never opens a store. A command that needs it and finds it nil answers `E_NOT_FOUND` for the timeline (the production is unknown). The golden runner builds it from the optional file `timelines.json` of a case (the other documents of the production as an array of canonical documents). `Result.Spawned` holds the new documents of a command that makes a timeline; the service creates their rows in the transaction of the commit, with the rows of the assets their documents name (copied from the timeline they came from, so their files are served and kept under them), and an undo of the batch leaves them in place (it restores the document, and the timeline is the user's to trash).

Rules:

1. `Apply` validates `args` against the schema first and returns `E_VALIDATION` with the path of the first invalid value.
2. It then checks the command's preconditions against `doc` and returns the listed error if one fails. The document is unchanged on any error.
3. On success the returned document satisfies every invariant of SDD-001 §4. `Apply` calls `timeline.Validate` on its result in tests and in debug builds; a violation there is a bug in the command, reported as `E_INTERNAL`.
4. Identifiers of created objects come from `args` when the caller supplied them (the fields named `id` or `new_id`) and otherwise from `env.NewIDs`. The service fills them in before storing the operation, so a stored operation always carries the identifiers and replays identically. Two commands create a number of objects that is known only after they run (`clip.remove_silences`, `caption.from_transcript`): they report the identifiers they used in `effective.new_ids`, and the service stores the operation with `new_ids` equal to that list.
5. An operation that adds, removes, moves or changes an item of a track that is `locked` when it starts is refused with `E_INVALID_STATE` (`details.track_ids` names the tracks) and changes nothing. `track.update` can still change the track, so unlocking works. Undo and redo are not operations and are not refused. (VV-036)
6. A batch is applied by folding `Apply` over its operations. If any fails, the whole batch fails with that error and `op_index` set.

### A.4 Time arguments

Every argument that is a time or a duration is a string parsed by `core/timeval.Parse(s, fps)`:

| Form | Example | Meaning |
| --- | --- | --- |
| seconds | `"12.5s"`, `"0.04s"` | decimal seconds |
| frames | `"375f"` | integer frames at the project rate |
| timecode | `"00:00:12.500"`, `"01:02.5"` | `HH:MM:SS.mmm` or `MM:SS.mmm` |

A JSON number, an empty string or a string with no unit is rejected with `E_INVALID_TIME` and a hint showing the three accepted forms. Seconds and timecode are rounded to the nearest frame, half away from zero. Unless a command says otherwise, a time argument is an absolute timeline position.

Parsing is strict: no spaces, no sign, no exponent, no other unit (`"500ms"` is rejected), minutes and seconds below 60 in a timecode, at most 64 characters, and a result of at most 2^40 frames. The frame rate is a rational between 1 and 1,000 frames per second. `effective.seconds` is the frame's time rounded to the millisecond; it is for reading only and is never parsed back.

For every time argument the result's `effective` map has an entry with the frame applied and its seconds: `"start": {"frame": 374, "seconds": 12.467, "requested": "12.48s"}`.

### A.5 Diff

`Result.Diff` is the list of changes, in a fixed order (settings, assets, tracks, items; each by identifier):

```json
{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"start": [0, 30], "duration": [150, 120]}}
```

`kind` is one of `settings.updated`, `asset.added`, `asset.removed`, `track.added`, `track.updated`, `track.removed`, `item.added`, `item.updated`, `item.removed`, `font.added`, `font.removed`, `project.updated`. For `added` and `removed` the change carries the whole object under `object`. The diff is written in the form of a fetched document: an object and both sides of a changed field carry every default (`scale`, `opacity`, `filter`, `filter_amount`, `speed`, `volume`, `fit`, `contrast`, `saturation`, `fill`, `stroke_color`, `settings.width`, and so on), so that patching the fetched document before the edit with the diff gives the fetched document after it with no derivation (SDD-012 section 5.4; `TestGoldenCases` checks it for every golden case). Only a field with no default (`x`, `y`, `crop`, `blur`, `markers`, `source_in`, ...) reads as absent, that is `null`. Undo primitives (A.6) stay canonical: a field equal to its default is absent in them. For `updated`, `changes` maps each changed top-level field of the object to `[before, after]`; an absent side is `null`.

For `settings.updated` and `project.updated` the `id` is the project identifier. The diff of a batch is the net difference between the document before the batch and the document after it: an object created and removed inside the batch does not appear, and a field changed more than once appears once, with its first before-value and its last after-value.

`core/command.DescribeDiff(doc, diff)` renders the diff as text, one line per change, with times in seconds and frames, for agents and for the history view. Its exact format is in SDD-003.

**Diff of two documents (RF-121, epic 36).** `core/command.Diff(a, b)` returns the changes that turn document `a` into document `b`, in the form above (the form of a fetched document, whole objects for `added` and `removed`, `[before, after]` for a changed field), in the order of a batch diff. It is computed from the two documents and never from the log, so it is the same after any sequence of edits, undos and redos that led to them. `core/command.DiffEntries(a, b)` classifies each change and sorts them most significant first: a track added or removed, then an item added or removed, then `settings.updated` and `project.updated`, then an item **trimmed** (`duration` or `source_in` changed), then an item **moved** (`start` or `track` changed, and neither of those two), then any other change of an item or a track, then assets and fonts; ties by kind and identifier. An entry has the change, its `verb` (`added`, `removed`, `moved`, `trimmed`, `changed`), its `noun` (`track`, the type of an item: `clip`, `audio`, `text`, `caption` or `shape`, then `asset`, `font`, `settings`, `project`) and the `DescribeDiff` line of the change. Equal documents give no entry.

### A.6 Undo

The inverse of an operation is not another catalogue command. It is a list of three **primitives** computed from the before-state of the objects the operation touched:

| Primitive | Effect |
| --- | --- |
| `patch {collection, id, set, unset}` | set the listed top-level fields to their previous values and remove the listed ones |
| `put {collection, object}` | insert an object that was removed |
| `remove {collection, id}` | remove an object that was created |

`collection` is `settings`, `project`, `assets`, `tracks`, `items` or `fonts`. Applying the inverse primitives of a batch in reverse order to the document that the batch produced yields exactly the document before the batch (canonical bytes equal).

Undo and redo are service operations (SDD-003). The rule from ADR-0006 that they implement: when an actor undoes its batch and a field in a `patch` was changed by a later batch of another actor, that field is skipped; when a `put` would overlap an item that another actor placed, or a `remove` targets an object another actor changed, that primitive is skipped. Skipped primitives are listed in the result. The undo is itself a batch in the log, with its own inverse, which is what redo applies.

The primitives of one undo are applied together to a working copy, with no validation between them, and the result is validated once at the end. Intermediate states may therefore break an invariant (two items overlapping while a ripple is being reversed); only the final document must be valid. Their order matters in one way only: an object must exist before it is patched and must not exist before it is put. If the final document is invalid, the primitives that touch an object named in a violation are dropped and the application starts again from the document before the undo; this ends because each round drops at least one primitive (SDD-005 §12.2).

**Revert (RF-120, epic 36).** `history.revert` is a service operation, not a catalogue command, because it needs a document the batch itself cannot carry: the document at an earlier revision. The service rebuilds that document (SDD-005 §12.4) and the core computes the change: `core/command.Revert(current, target)` returns a `BatchResult` whose `Doc` is `target`, whose `Diff` is `Diff(current, target)` and whose `Inverse` is the primitives that turn `target` back into `current`. Reverting is therefore an ordinary batch: it is one revision with an inverse, so it is undone by `Undo` of the actor who asked and redone by `Redo`; it is validated as any batch (the target was a valid document); and it changes the locked tracks like undo does (it is not an edit of an item, it restores a state). The document equals the target in canonical bytes, which `TestRevertProperty` checks over a random sequence of edits for every earlier revision. `core/command.RebuildAt(current, inverses)` is the other half: it applies the stored inverses of the revisions after the target, newest first and each in reverse order, all together on a working copy with no guard, and returns the document at the target, or an error when any primitive is skipped (the log does not fit the document). The service compares its hash with the `doc_sha256` of the target row before it trusts it.

### A.7 Concurrency and idempotency

A batch request carries `base_revision`. The service decides:

1. If `base_revision` equals the current revision: apply.
2. If it is older: collect the diffs of the intervening batches issued by **other** actors. If any of them added, updated or removed an object that this batch refers to by identifier in its `args`, or changed the project (name, markers) or its settings when the batch has a `project.rename`, `marker.*` or `settings.*` operation, reject with `E_REVISION_CONFLICT`. Otherwise apply the batch to the current document; if an operation then fails (for example with `E_OVERLAP` because someone placed an item there), return that error.
3. If it is newer than the current revision: `E_VALIDATION`.

`E_REVISION_CONFLICT` carries the current revision and the text diff from `base_revision` to it, so the caller can re-plan.

A batch whose `base_revision` is more than 1,000 revisions behind is rejected with `E_REVISION_CONFLICT` and the hint to read the project again, without reading the log.

A batch may carry an `idempotency_key` (8 to 64 characters from `[A-Za-z0-9._~-]`). The service stores the result of a committed batch under `(project, actor, key)` for 24 hours. A second request with the same key returns the stored result with `"replayed": true` and changes nothing; if the second request's operations differ from the first's, the answer is `E_IDEMPOTENCY_MISMATCH`.

`dry_run: true` runs steps 1 and 2 and returns the result that would be committed, with `"applied": false`, without writing.

A revert (A.6) carries `base_revision` too, the revision the caller read when it chose the target; absent, it is the current revision. When it is older, a revert conflicts when another actor's change in between added, updated or removed an object that the revert changes (the objects of `Diff(current, target)`, and the project or the settings when those differ). The error is the same `E_REVISION_CONFLICT` with the text diff, so a person reads what the other actor did before going back over it.

### A.8 Errors

Every error has this shape on every surface:

```json
{
  "code": "E_OVERLAP",
  "message": "Item itm_m1n2p3q4r5 would occupy frames 100-149 on track trk_f6g7h8j9k0, where item itm_c3d4e5f6g7 already is (frames 90-179).",
  "hint": "Use start \"180f\" (6.000s), the next free position on that track, or move to another track.",
  "retryable": false,
  "op_index": 1,
  "path": "args.start",
  "details": {"conflicting_item": "itm_c3d4e5f6g7", "next_free": 180}
}
```

Rules for messages: state what was wrong with the actual values; `hint` states one concrete way to succeed; never include a stack trace, a file path of the server or SQL. The caller's own words may be quoted back even when they look like a file name (an unknown argument `ts` gives `args.ts is not an argument of this command.`); `CheckError` is for what the server adds, and the fuzz target `FuzzApplyArgs` removes the caller's strings before it checks. `details` is machine-readable and specific to the code.

Codes of the core (service codes are in SDD-003):

| Code | Meaning | Retryable |
| --- | --- | --- |
| `E_UNKNOWN_COMMAND` | no such command; `details.known` lists the names | no |
| `E_VALIDATION` | `args` does not match the schema; `path` points at the value | no |
| `E_INVALID_TIME` | a time string is malformed or negative; a length is shorter than one frame where that is not allowed; an item would end after frame 108,000; or a time addresses a frame outside the item it refers to | no |
| `E_NOT_FOUND` | an identifier or a word index in `args` does not exist; `details.kind` (`item`, `track`, `asset`, `word`, `keyframe`) and `details.id` | no |
| `E_ID_EXISTS` | an identifier supplied for a new object is already used | no |
| `E_TYPE_MISMATCH` | the item or track is not of a type the command accepts | no |
| `E_OVERLAP` | the result would overlap another item on the track; `details.conflicting_item`, `details.next_free` | no |
| `E_OUT_OF_SOURCE` | the item would read outside its asset; `details.available` gives the valid range | no |
| `E_INVALID_STATE` | a precondition on the current document fails; the message says which | no |
| `E_IN_USE` | the object is referenced by others; `details.used_by` | no |
| `E_NOT_EMPTY` | a track has items and `with_items` was not set | no |
| `E_LIMIT` | a limit of SDD-001 would be exceeded (tracks, items, assets, words in the document, words in one caption, keyframes of one property) | no |
| `E_FONT_UNKNOWN` | the font family or weight is not available; `details.available` | no |
| `E_SLOT` | a slot is missing, already filled, of another kind, or its name is used by another item; `details.reason` | no |
| `E_REVISION_CONFLICT` | section A.7 | yes |
| `E_IDEMPOTENCY_MISMATCH` | section A.7 | no |
| `E_INTERNAL` | a bug; the batch was not applied | no |

### A.9 Commands whose arguments come from analysis

Three commands depend on data the core cannot compute because it needs media: silences, a transcript, an asset's probe. `Apply` stays pure by receiving that data in `args`. The caller writes the short public form; the service fills the `resolved` field from artifacts that ingest and transcription already produced (the audio envelope and the transcript, SDD-011) and stores the operation with it. An edit never starts a media job: when the artifact it needs is missing, the command fails with `E_INVALID_STATE` and a hint that names the job to run first (SDD-007 §5.4). The core itself, given an operation with no `resolved` field (a caller that bypassed the service), refuses it with `E_VALIDATION` and the same hint (Part C §C.27, §C.40, §C.41).

| Public argument | Field filled by the service |
| --- | --- |
| `clip.remove_silences {min_silence, padding, threshold_db}` | `resolved.silences`: ranges in asset frames, taken from the set of the silence artifact measured at `threshold_db` (SDD-011 §9.1) |
| `caption.from_transcript {...}`, `caption.retime_from_clip {...}` | `resolved.words`: words with times in asset frames |
| `audio.duck {speech_item_ids, ...}` | `resolved.speech`: one entry per speech item, `{item_id, ranges}`, with the speech ranges of that item's asset in asset frames, half-open, taken from the speech map of the asset (SDD-011) |
| `audio.mark_beats {...}` | `resolved.beats`: the beats of the music asset as strictly increasing asset frames, taken from the beat grid of the asset (SDD-011); at most 108,000. Not used when `bpm` is given |
| `asset.add` | issued only by the import flow, with the facts measured on the verified normalized media, never copied from the probe (SDD-011 §4.9) (`internal: true`) |

`resolved` is internal: the service refuses a caller that sends it on these commands with `E_VALIDATION` (`path` `operations[i].args.resolved`), so a caller cannot inject silences, words or beats that the analysis did not produce (VV-206). Golden tests that run without media call the core directly with `resolved`.

**OpenTimelineIO import (RF-065).** `core/otio` import adds no command and no operation: it returns a batch of existing operations (`track.add`, `clip.add`, `audio.add`, `item.rename`, `item.set_transition`) that the caller applies like any other batch, so conflicts, limits and undo behave as everywhere else. The file is never opened by the core (SDD-001 section 9).

### A.10 Determinism and limits

1. Maps are iterated in sorted key order wherever the order affects the result.
2. Arithmetic on time is integer. Arithmetic on space uses `float64` with results rounded to 3 decimal places before they are stored, so documents are stable across platforms.
3. A batch has at most 200 operations and its JSON is at most 512 KiB; beyond that the service returns `E_PAYLOAD_TOO_LARGE`.

4. A command accepts all of its arguments and all of the values its schema lists, wherever it is registered.

### A.11 Tests required for every command

For each command, `core/command/testdata/<command>/` holds golden cases, each a directory as SDD-008 §3 defines it: `before.json`, `op.json`, and either `after.json` (the document after, canonical), `inverse.json`, `diff.json` and `effective.json`, or `error.json`. The suite runs every case and, for every success case, also applies the inverse and compares with `before.json` byte for byte. Each command has at least: one success case per optional argument, one case per error code it lists, and the boundary cases named in Part C.

### A.12 How `core/command` implements Part A

These are the decisions of the implementation (task P1-ENG-001) that Part A leaves open.

1. **Errors** live in `core/command` (`Error`, the `Code*` constants, `CheckError`). `core/timeval` and `core/timeline` keep their own error types; a command reads a time through `Context.ParseTime`, which turns the `timeval` error into `E_INVALID_TIME` with the path of the argument. `CheckError` fails on an unknown code, a missing message or hint (`E_INTERNAL` needs no hint), a wrong `retryable` flag, and on a message or hint that holds a file path, a stack trace or SQL; the golden runner applies it to every `error.json`.
2. **The schema of `args` is a Go struct.** A command is built with `Define[A](Spec, apply)`. Tags on `A`: `json:"name"` (required) or `json:"name,omitempty"` (optional), `enum:"a,b"`, `min:"1" max:"200"` (a number, or the length of a string or an array), `desc:"..."`, and `schema:"name"` for a field of type `any` (or a map of `any`) that the apply function checks itself: the named schema in `core/command/schema_named.go` describes exactly what that check accepts, so no property of a public command lacks a JSON type. A schema that also accepts `null` (a merge that removes a key) has `type` `[T, "null"]`. Every argument of the time type has `x-vv-kind: "time"` in its schema, derived from the type and not from the name (SDD-012 �9.2). `Apply` rejects an unknown key, a missing required key, a wrong type, a fractional integer and a value outside its constraints, with `E_VALIDATION` and the path (`args.item_ids[2]`). `Registry.Schema(name)` returns the JSON Schema of the struct (`additionalProperties: false`), and `Registry.Help(name)` the help text. `Registry.Register` refuses a command without a schema (the arguments must be a struct), an example that its own schema rejects, an error list, a description or a scope. `Registry.Lookup` is `Get`.
3. **`Context`.** The function of a command receives a `*Context` whose `Doc` is a private copy it may edit freely; `Context.NewID(prefix, supplied)` is rule 4 of A.3, `ParseTime`, `SetEffective` and `SetEffectiveTime` write `effective`. A panic, or an error that is not an `*Error`, becomes `E_INTERNAL` without its text. `Registry.Debug` makes `Apply` validate its result (rule 3).
4. **Batches.** `Registry.ApplyBatch(doc, ops, env)` folds `Apply`; its `Diff` and `Inverse` are those of the net change from the first document to the last. A batch of no operation is `E_VALIDATION`.
5. **Diff.** `Change{Kind, ID, Changes, Object}` is computed by comparing the canonical forms of the two documents, so a field equal to its default is absent and reads as `null` in `changes`. The order is: `project.updated`, `settings.updated`, then assets, tracks, items and fonts, each by identifier. A font whose files changed is `font.removed` followed by `font.added`.
6. **Primitives.** The JSON form is `{"type": "patch", "collection": "items", "id": "itm_...", "set": {...}, "unset": [...]}`, `{"type": "put", "collection", "id", "object"}` and `{"type": "remove", "collection", "id"}`. For `settings` and `project` the `id` is the project identifier, for `fonts` the family; only `patch` applies to those two. The inverse of an operation or batch is computed from the two canonical documents: `remove` for what was added, `put` for what was removed, `patch` with the old values (and `unset` for a field that was absent) for what changed. Undo applies the list in reverse order. `ApplyPrimitives` returns a `Skipped` entry per primitive (or per group of fields of a patch) it did not apply, with the reasons of SDD-005 §12.2 plus `invalid_primitive` for a primitive that is malformed. A violation that names no object touched by a remaining primitive (for example a frame rate that breaks a source range) drops all the remaining primitives.
7. **Conflicts.** `Conflicts` returns `{collection, id, kind}`; the service adds the revision and the actor from the batch that contained the change. An identifier counts as referenced when it appears as a string anywhere in `args`, which includes the `id` of a new object.
8. **The first commands** (task P1-ENG-002) follow Part C with these differences, each listed here so that Part C and the golden files agree:
   - A JSON number for a time argument is `E_INVALID_TIME` with the three accepted forms in the hint (A.4), not `E_VALIDATION`.
   - `item.split` and `item.trim` advance or move `source_in` only for a clip or audio item whose asset is a video or an audio file; an image clip keeps its `source_in` (R9 exempts images, and `clip.add` refuses a `source_in` on an image).
   - `asset.add` refuses a `sample_count` on an asset with no audio (the rule says it is required exactly when the asset has audio).
   - `settings.set` accepts `background: "$name"` when `template.colors` has the name; `Validate` accepts the reference in `settings.background` (SDD-001 section 3.1).
   - The weights of the bundled font families come from `timeline.BundledWeights`, which a test keeps equal to `renderer/fonts/FONTS.md`; the weight checked is the given one, else the default 700, so Anton needs an explicit `font_weight` 400. `E_FONT_UNKNOWN` carries `details.available`.
   - `item.delete` leaves `effective.freed` out for `ripple: "none"`; `E_OUT_OF_SOURCE` of `item.trim` on the start edge carries `details.available = {source_in, earliest_to}`.
   - `caption.add` with `text` refuses a word of more than 80 characters with `E_VALIDATION`; the error codes of `track.remove` are `E_NOT_FOUND` and `E_NOT_EMPTY` (there is no schema error to report).
   - The `order` strings, the seconds in hints and the messages in Part C are examples; the golden files hold the real ones.

### A.13 Export presets and the fit check (`core/export`)

The rules of exporting for a network are pure and live in `core/export`, not in a handler: the table of presets (SDD-004 section 8.2), `Check` and the codec names. No command is added to the registry and the document is never changed: a preset does not edit the timeline, it says whether the timeline as it is fits a network. `Check(preset, input)` takes the pixels, the frame rate, the duration in milliseconds and, when the file exists, its size in bytes, and returns the findings below in a fixed order (duration, frame rate, resolution, aspect ratio, size), each with `code`, `severity`, `message`, `limit` and `got`. A limit of `0` in a preset is not checked. The aspect ratio is compared as a fraction of integers, with the tolerance the preset states.

| Finding code | Raised when |
| --- | --- |
| `X_DURATION_TOO_LONG`, `X_DURATION_TOO_SHORT` | the duration is over the maximum or under the minimum of the preset |
| `X_FPS_TOO_HIGH`, `X_FPS_TOO_LOW` | the frame rate is over or under the preset's range |
| `X_RESOLUTION_TOO_SMALL`, `X_RESOLUTION_TOO_LARGE` | a side, or the width, is outside the preset's range |
| `X_ASPECT_RATIO` | the width over the height is outside the preset's range |
| `X_SIZE_TOO_LARGE` | the file, or the estimate of it (the targets of the preset times the duration), is over the maximum |

These are findings of a check, not errors of a call: they travel in the result of `ExportCheck` and, when an export is refused for them, in `details.findings` of `E_VALIDATION` with `details.reason = "preset_mismatch"`. They are listed in SDD-000 section 15.

## Part B: the catalogue

Scope is `edit` unless stated. "Time" marks arguments parsed by A.4. Optional arguments are written with `?`.

### B.1 Project and settings

| Command | Arguments | Effect |
| --- | --- | --- |
| `project.rename` | `name` | Sets the project name |
| `settings.set` | `width?`, `height?`, `background?`, `loudness_target?` | Sets the given settings. When the size changes, every visual item's `x` and `y` are scaled by the ratio of new to old size on each axis; `scale` is untouched; items with default position stay centred |
| `settings.reframe` | `aspect?` (`"16:9"`, `"9:16"`, `"1:1"`, `"4:5"`) or `width` and `height` | Re-targets the project to another aspect: sets the size like `settings.set`, and also scales text, shape and fit `none` clips by the smaller axis ratio so they keep their share of the frame; see C.60 |
| `settings.set_fps` | `fps` (`"30"`, `"30000/1001"` or a rational object) | Rescales every time quantity `q` to `round(q × new / old)`, then restores invariants: a duration that became 0 becomes 1; an overlap created by rounding is removed by shortening the earlier item. `effective.adjusted` lists every item changed by a fix-up |

### B.2 Assets

| Command | Scope | Arguments | Effect |
| --- | --- | --- | --- |
| `asset.add` | media, internal | `asset` (an Asset object of SDD-001 §3.2) | Adds the asset. Adding an asset whose `hash` is already present returns the existing identifier in `effective.asset_id` and changes nothing |
| `asset.remove` | media | `asset_id` | Removes the asset from the project. Fails with `E_IN_USE` if an item uses it |

### B.3 Tracks

| Command | Arguments | Effect |
| --- | --- | --- |
| `track.add` | `kind`, `name?`, `id?`, and one of `position?` (`"top"` default, `"bottom"`), `above?` (track id), `below?` (track id) | Adds a track at that place in the order |
| `track.update` | `track_id`, `name?`, `hidden?`, `muted?`, `locked?`, `solo?`, `volume?` | Sets the given fields; `locked` refuses edits of the items of the track (A.3 rule 5), `solo` plays only soloed tracks, `volume` (0 to 4) is a factor on the volume of its items |
| `track.move` | `track_id` and one of `position`, `above`, `below` | Changes the track's place in the order |
| `track.remove` | `track_id`, `with_items?` (default false) | Removes the track. With items on it, fails with `E_NOT_EMPTY` unless `with_items` is true, in which case its items are removed too |
| `track.set_caption_style` | `track_id`, `style` (partial CaptionStyle), `replace?` (default false) | Merges `style` into the track's caption style, or replaces it |

### B.4 Items of any type

| Command | Arguments | Effect |
| --- | --- | --- |
| `item.move` | `item_id`, `start?` (time), `track_id?`, `slide?` (default false) | Moves the item in time, to another track of the same kind, or both. At least one is required. With `slide` the touching neighbours are trimmed to keep touching it (C.11) |
| `item.trim` | `item_id`, `edge` (`"start"` or `"end"`), `to` (time), `ripple?` (`"none"` default, `"track"`), `roll?` (default false) | Moves that edge to `to`. With ripple `"track"` the later items of the track shift by the change in length; with `roll` the touching edge of the neighbour moves to `to` too. Rules in B.10 and C.12 |
| `item.slip` | `item_id`, `source_in` (time) | Shows another part of the source: sets `source_in` of a clip or audio item without moving or resizing it; see C.59 |
| `item.split` | `item_id`, `at` (time), `new_id?` | Splits the item into two at `at`. Rules in B.10 |
| `item.delete` | `item_ids` (1 to 200), `ripple?` (`"none"` default, `"track"`, `"all"`) | Removes the items. With `ripple: "track"` every later item on each affected track moves earlier by the length freed on that track; with `"all"` every later item on every track moves by the length freed, which requires the deleted items to cover one contiguous range |
| `item.duplicate` | `item_id`, `start?` (time), `track_id?`, `new_id?` | Copies the item to the given place. A slot marking is not copied |
| `item.rename` | `item_id`, `name` | Sets the label |
| `item.set_transform` | `item_id`, `x?`, `y?`, `scale?`, `rotation?`, `opacity?` | Sets the given fields on a visual item. A property that has keyframes cannot be set this way: `E_INVALID_STATE` with the hint to use `keyframe.set` or `keyframe.clear` |
| `item.set_transition` | `item_id`, `edge` (`"in"`, `"out"`), `kind` (a transition kind or `null` to remove), `duration?` (time, as a length; default `"0.5s"`) | Sets or removes the transition on a visual item |
| `item.set_audio` | `item_id`, `volume?`, `fade_in?` (time, as a length), `fade_out?`, `audio_muted?`, `denoise?`, `voice?` | Sets audio properties on a `clip` or `audio` item; also the noise reduction strength (TASK-0233) and the voice preset (TASK-0254) |
| `item.crossfade` | `from_item_id`, `to_item_id`, `duration` (time, as a length), `track_id?` | Helper for RF-023. Requires two visual items on different tracks where `from` starts before `to` and the two overlap in time by at least `duration`. Sets `transition_out: fade` on `from` and `transition_in: fade` on `to`, both of that duration, and the matching audio fades. For two neighbours that touch on one track, `track_id` (another visual track) moves `to` there and makes it start `duration` earlier, from the source it has before the cut (C.20, VV-042) |
| `timeline.close_gap` | `track_id`, `at` (time), `ripple?` (`"track"` default, `"all"`) | Finds the empty range on the track that contains frame `at` and moves later items earlier by its length. Fails with `E_INVALID_STATE` when `at` is inside an item or after the last item |

### B.5 Clips and audio

| Command | Arguments | Effect |
| --- | --- | --- |
| `clip.add` | `track_id`, `asset_id`, `start?` (time; default the end of the last item of the track), `source_in?` (time, offset into the asset; default 0), `duration?` (time, as a length; default: the rest of the asset from `source_in`, or `"5s"` for an image), `fit?`, `id?` | Places a video or image asset on a visual track |
| `audio.add` | `track_id`, `asset_id`, `start?` (default the end of the last item of the track), `source_in?`, `duration?`, `id?` | Places an audio asset, or the audio of a video asset, on an audio track |
| `clip.add_composition` | `track_id`, `timeline_id`, `start?`, `source_in?`, `duration?` (default the rest of the nested timeline from `source_in`), `fit?`, `id?` | Places another timeline of the production on a visual track as one clip (RF-083, SDD-001 section 3.5.1). Refuses a cycle and a nesting deeper than 4. See C.84 |
| `composition.create_from_selection` | `item_ids` (1 to 100), `name?`, `timeline_id?`, `id?` | Moves the selected items into a new timeline of the production and puts one composition clip in their place, in one batch. See C.85 |
| `composition.expand` | `item_id` | The inverse: puts the items of the nested timeline back on the timeline in place of a composition that uses no group feature. See C.86 |
| `clip.set_framing` | `item_id`, `fit?`, `crop?` | Sets fit and crop |
| `clip.set_speed` | `item_id`, `speed` (`"1.5x"`, `"0.5x"` or a rational object) or `curve` (2 to 8 `{t, speed}` points), `ripple?` (`"none"` default, `"track"`) | Changes speed keeping the same source range: the new duration is `ceil(duration × old / new)`, reduced by the rounding excess if that would read past the end of the asset. Keyframe times and transition and fade lengths are scaled by the same ratio and rounded. With `ripple: "track"`, later items on the track shift by the change in length; otherwise a resulting overlap is `E_OVERLAP`. Captions linked to the clip are not changed; the lint reports them as out of sync until `caption.retime_from_clip` runs |
| `clip.set_color` | `item_id`, `brightness?`, `contrast?`, `saturation?`, `temperature?`, `tint?`, `hue?` | Sets colour adjustment; also white balance and hue rotation (TASK-0233). See C.26 |
| `clip.set_filter` | `item_id`, `filter?`, `amount?` | Sets the colour filter of a clip and how much of it is mixed in |
| `clip.set_blur` | `item_id`, `radius` | Sets the blur radius of a clip in pixels; 0 removes it |
| `clip.replace_media` | `item_id`, `asset_id` | Swaps the asset of a clip for another of the same kind, keeping its place, trim, transform, keyframes and effects (the re-render of an animation, SDD-011 section 15). See C.78 |
| `item.repeat` | `item_id`, `copies?` or `until?` (exactly one), `ripple?` (`none` default, `track`), `link?`, `new_ids?` | Repeats an item back to back after its end, a number of times or until a time, such as a looping background (VV-123). See C.74 |
| `clip.set_shadow` | `item_id`, `shadow` (`{color?, opacity?, dx?, dy?, blur?}` or null) | Gives a clip a drop shadow that follows its visible shape (VV-122); null removes it. See C.72 |
| `clip.set_outline` | `item_id`, `outline` (`{color?, width?}` or null) | Gives a clip an outline that follows its visible shape (VV-122); null removes it. See C.73 |
| `clip.set_chroma_key` | `item_id`, `color` (`#RRGGBB` or null), `similarity?`, `smoothness?`, `spill?` | Keys out one colour of a clip (a green screen); null removes the key. See C.55 |
| `clip.set_blend` | `item_id`, `mode` (`normal`, `multiply`, `screen`, `overlay`, `add`) | Sets how a visual item is mixed with what is below it. See C.61 |
| `clip.set_mask` | `item_id`, `mask` (`{shape, x, y, width, height, feather?, invert?}` or null) | Limits where a visual item is drawn to a rectangle or an ellipse in the item's own frame; null removes it. See C.62 |
| `clip.set_flip` | `item_id`, `horizontal?`, `vertical?` (booleans) | Mirrors a visual item left to right and or top to bottom; a side not given keeps its value. See C.63 |
| `item.set_enabled` | `item_id`, `enabled` (boolean) | Turns any item off or on without removing it: a disabled item stays on the timeline and is ignored by preview, export and audio. See C.64 |
| `clip.set_effects` | `item_id`, `effects` (up to 6 `{kind, ...}`) | Sets the visual effects of a clip: motion blur, glow, vignette, grain, RGB split and shake, each with its parameters. See C.68 |
| `item.paste_attributes` | `from_item_id`, `item_ids`, `attributes` | Copies attribute groups (color, filter, blur, framing, transform, blend, mask, flip, chroma_key, effects, audio) from one item to 1 to 20 others. See C.69 |
| `item.freeze_frame` | `item_id`, `at`, `duration?`, `freeze_id?`, `new_id?` | Splits a clip at a frame and inserts between the parts a still of the source frame shown there, held for `duration` (default 3s); the later items of the track move later. See C.70 |
| `caption.import_subtitles` | `track_id`, `text`, `format?`, `start?`, `new_ids?` | Creates one caption per cue of an SRT or WebVTT text on a visual track (the file is read by the caller). See C.71 |
| `item.link` | `item_ids` (2 to 20), `id?` | Puts items in one link group: `item.move`, `item.trim` and `item.split` on one of them apply to all of them. See C.65 |
| `item.unlink` | `item_ids` | Takes items out of their link group; a group left with one item is dissolved. See C.66 |
| `item.extract_audio` | `item_id`, `track_id`, `id?` | Creates a linked audio item from the audio of a video clip and mutes the clip. See C.67 |
| `clip.remove_silences` | `item_id`, `min_silence?` (length, default `"0.4s"`), `padding?` (length kept on each side of speech, default `"0.1s"`), `threshold_db?` (-30, -35, -40 or -45, default -35), `ripple?` (`"track"` default, `"none"`), `new_ids?` | Replaces the clip, or an audio item, by one item per non-silent stretch of its source range (A.9). With ripple the pieces are placed back to back from the original start and later items on the track move earlier; without, each piece keeps its original position. The first piece keeps the item's identifier, its `transition_in` and `fade_in`; the last keeps `transition_out` and `fade_out`. `effective.removed` is the total length removed. With no silence found the document is unchanged |
| `audio.duck` | `item_id`, `speech_item_ids` (1 to 20 items), `gain_db?` (-40 to -1, default -12), `attack?` (length, default `"0.2s"`), `release?` (length, default `"0.5s"`) | Lowers a music item while speech sounds, as generated `volume` keyframes; the speech comes through `resolved` (A.9). `keyframe.clear` on `volume` undoes it |
| `audio.processor_add` | `item_id` or `track_id`, `kind`, `params?`, `index?`, `enabled?` | Adds an audio processor (equaliser, compressor, limiter, gate, de-esser, noise reduction) to the chain of an item or a track. See C.79 |
| `audio.processor_set` | `item_id` or `track_id`, `index`, `params?`, `enabled?` | Changes the parameters or the switch of one processor of a chain. See C.80 |
| `audio.processor_move` | `item_id` or `track_id`, `index`, `to` | Moves a processor to another place of its chain. See C.81 |
| `audio.processor_remove` | `item_id` or `track_id`, `index` | Removes a processor from a chain. See C.82 |
| `audio.apply_preset` | `item_id` or `track_id`, `preset` | Replaces a chain with a preset: voice, voice with music, podcast or clean up a noisy room. See C.83 |
| `marker.add` | `item_id?`, `at`, `label?` | Puts a marker at a frame of the timeline, or of an item with `item_id` (TASK-0254); see C.56 |
| `marker.set` | `item_id?`, `at`, `to?`, `label?` | Moves or relabels the marker at a frame, of the timeline or of an item; see C.57 |
| `marker.remove` | `item_id?`, `at` or `all` | Removes the marker at a frame, or every marker, of the timeline or of an item; see C.58 |
| `audio.mark_beats` | `item_id`, `every?` (1 to 64, default 1), `label?`, `replace?` (default true), `bpm?` (20 to 400), `offset?` | Writes markers on a music item at the beats of the visible part of its audio; the beats come through `resolved` (A.9), or from the manual tempo `bpm` and `offset` when the analysis finds no pulse (VV-153) |

### B.6 Text and shapes

| Command | Arguments | Effect |
| --- | --- | --- |
| `text.add` | `track_id?`, `start`, `duration` (length), `text`, `style?` (partial TextStyle), `x?`, `y?`, `id?` | Adds a text item |
| `text.set` | `item_id`, `text?`, `style?` (partial, merged), `reset_style?` (default false) | Changes content or style |
| `text.apply_preset` | `item_id`, `preset` (`"fade_up"`, `"fade_down"`, `"pop"`, `"zoom_in"`, `"slide_left"`, `"slide_right"`), `edge` (`"in"`, `"out"`), `duration?` (length, default `"0.4s"`) | Writes the keyframes or transition that implement the preset on that edge, replacing keyframes of the same properties inside that edge's range. Works on any visual item. The definitions are in Part C |
| `shape.add` | `track_id?`, `start`, `duration`, `shape`, `width`, `height`, `fill?`, `stroke_color?`, `stroke_width?`, `radius?`, `x?`, `y?`, `id?` | Adds a shape item |
| `shape.set` | `item_id`, and any of the shape fields | Changes a shape |

### B.7 Captions

| Command | Arguments | Effect |
| --- | --- | --- |
| `caption.add` | `track_id?`, `start`, and either `words` (array of `{text, start, duration}` with times relative to the caption as lengths) or `text` with `duration` (length); `linked_item?`, `x?`, `y?`, `id?` | Adds a caption item. `x` and `y` (pixels, the centre of the item) are used when the track's caption style position is `custom`. With `text`, it is split on whitespace and the words share the duration in proportion to their character counts, each word getting at least one frame |
| `caption.set_words` | `item_id`, `words` or `text` | Replaces the words; the item's duration becomes the end of the last word unless `duration?` is given |
| `caption.edit_word` | `item_id`, `index`, `text` | Changes one word's text. An empty `text` removes the word; removing the only word of a caption is `E_INVALID_STATE` |
| `caption.split` | `item_id`, `before_word` (index, 1 or more), `new_id?` | Splits a caption into two at a word boundary |
| `caption.merge` | `item_ids` (exactly 2, on the same track, with no other item between them) | Joins two captions; the result keeps the first identifier and spans both |
| `caption.set_breaks` | `item_id`, `breaks` (array of word indexes) | Sets forced line breaks |
| `caption.set_style` | `item_id`, `style` (partial CaptionStyle, or `null` to clear the override) | Sets the per-item style override |
| `caption.from_transcript` | `clip_item_id`, `track_id`, `max_words?` (default 5), `max_chars?` (default 28), `max_gap?` (length, default `"0.6s"`), `replace?` (default true), `new_ids?` | Creates caption items on the track for the speech in the clip's visible source range (A.9). A new caption starts when adding the next word would exceed `max_words` or `max_chars`, or when the gap to the next word exceeds `max_gap`. Each caption gets `linked_item`. With `replace`, existing captions linked to that clip on that track are removed first. Word times are mapped from the asset through the clip's `source_in`, `speed` and `start` |
| `caption.retime_from_clip` | `clip_item_id` | Recomputes every caption linked to the clip from the transcript and the clip's current position, in-point and speed, keeping each caption's edited texts where the word count is unchanged |

### B.8 Keyframes

| Command | Arguments | Effect |
| --- | --- | --- |
| `keyframe.set` | `item_id`, `property`, `at` (time, absolute timeline position inside the item), `value`, `easing?`, `bezier?` | Adds a keyframe at that frame or replaces the one there. If the property had no keyframes and `at` is not the item's first frame, a keyframe with the property's current static value is also added at the item's first frame |
| `keyframe.remove` | `item_id`, `property`, `at` (time) | Removes the keyframe at that frame. Removing the last keyframe of a property writes its value into the static field |
| `keyframe.clear` | `item_id`, `property?` | Removes all keyframes of the property, or of every property. The value at the item's first frame becomes the static value |
| `keyframe.set_easing` | `item_id`, `property`, `at`, one of `easing?`, `bezier?` (four numbers) and `preset?` | Changes the easing of the segment that starts at an existing keyframe: a named easing, a custom cubic Bezier or a named Bezier preset (epic 31, RF-080). See C.75. `keyframe.set` takes `bezier` too |
| `animation.apply_preset` | `item_id`, `preset`, `edge?`, `duration?`, `direction?`, `amount?` | Writes a layer animation (fade, slide, scale, pop, push in, pull out) as ordinary keyframes at the start or the end of a visual item (epic 31, RF-082). See C.76 |
| `animation.list_presets` | none | Describes the animation presets and the named Bezier presets, for agents. Changes nothing. See C.77 |

### B.9 Templates

| Command | Arguments | Effect |
| --- | --- | --- |
| `slot.mark` | `item_id`, `name`, `description?`, `required?`, `duration_policy?` | Marks an item as a slot; the slot kind follows from the item type |
| `slot.unmark` | `item_id` | Removes the marking |
| `slot.fill` | `name`, and one of `asset_id` with `source_in?`, `text`, `words` | Fills a slot. For media with `duration_policy: "fit"` the item keeps its duration and the asset must be at least that long from `source_in` (`E_OUT_OF_SOURCE`); with `"stretch_timeline"` the item takes the asset's remaining duration and later items on every track shift by the change |
| `template.set_info` | `description?`, `colors?` (map of name to colour; a `null` value removes the name) | Sets template information. Removing a colour that a style refers to is `E_IN_USE` |

### B.10 Rules shared by trim and split

These rules are the part of the catalogue most likely to be implemented wrongly; Part C repeats them per command with golden cases.

**Trim, edge `start`, to frame `T`.** Requires `T < start + duration`. Let `d = T - start` (negative when extending earlier).

- `start` becomes `T`; `duration` becomes `duration - d`.
- `clip` and `audio`: `source_in` becomes `source_in + floor(d × speed)` for positive `d` and `source_in - ceil(-d × speed)` for negative `d`; a negative result is `E_OUT_OF_SOURCE`. Images ignore this.
- Keyframes: each `t` becomes `t - d`. If that removes keyframes before frame 0 of a property that still has later keyframes, a keyframe with the interpolated value is written at 0. Keyframes are never left outside the item.
- Captions: each word's `start` becomes `start - d`. Words that end at or before 0 are removed; a word that crosses 0 is clamped to start at 0. No word left is `E_INVALID_STATE`.
- `fade_in`, `transition_in`: unchanged in length. If transitions or fades no longer fit the new duration: `E_INVALID_STATE`, with the hint to shorten them first.
- A resulting overlap is `E_OVERLAP`.

**Trim, edge `end`, to frame `T`.** Requires `T > start`. `duration` becomes `T - start`. The source range check applies when the item grows. Keyframes after the new duration are removed, with an interpolated keyframe written at the new end (`t` equal to the new duration) when the property had keyframes on both sides. Words that start at or after the new duration are removed; a word that crosses it is clamped.

**Split at frame `T`.** Requires `start < T < start + duration`. The original item keeps its identifier and becomes the left part with `duration = T - start`. A new item is the right part with `start = T`, `duration = end - T` and, for `clip` and `audio`, `source_in` advanced by `floor((T - start) × speed)`.

- `transition_in` and `fade_in` stay on the left part; `transition_out` and `fade_out` go to the right part. A transition or fade longer than its part is shortened to the part's duration.
- Keyframes are divided at `T`; for each animated property both parts get an interpolated keyframe at the cut, so the motion is unchanged.
- Caption words go to the part that contains their first frame, clamped to that part. A part left with no word is not created, and the command then fails with `E_INVALID_STATE` (split a caption at a word boundary with `caption.split`).
- A slot marking stays on the left part only.
- Text and shape items are simply divided.
- Markers (M3): a marker before `T` stays on the left part; the others go to the right part with `frame` counted from its start.

**Markers (SDD-001 section 3.4).** Markers follow the picture as keyframes do and never make a command fail:

- M1, trim of the start by `d`: each `frame` becomes `frame - d`; a marker that falls before frame 0 is removed.
- M2, trim of the end, or any cut of the item to a new `duration`: markers at or after the new duration are removed.
- M3, split: as above.
- M4, `clip.set_speed` and `settings.set_fps`: each `frame` is scaled by the same ratio as the keyframe times and rounded; a marker that lands at or after the duration, or on the frame of the marker before it, is removed (the first of two stays). The rescale is not listed in `effective.adjusted`.
- `clip.remove_silences` applies M1 and M2 to each piece, so a marker stays with the piece that holds its frame.

### B.11 Project fonts

Scope is `media` (SDD-007 section 5.4). A font is first stored by content hash in the workspace `fonts/` directory by the font import of the service (SDD-007 section 7.7); these commands only name it in the document.

| Command | Arguments | Effect |
| --- | --- | --- |
| `font.add` | `family`, `files` (1 to 9 of `{weight, hash}`) | Adds a user family to the `fonts` map, or weights to a family that exists (a weight that exists gets the new file). The family is not a bundled one. Diff: `font.added`, or `font.removed` then `font.added` when a family changes |
| `font.remove` | `family` | Removes a user family. A family that a text or caption style or a track caption style uses is `E_IN_USE` with `details.used_by`. The files stay in the workspace |

A style that names a family added earlier in the same batch validates, because operations apply in order (A.12 rule 4); the reverse order is `E_FONT_UNKNOWN`. Where `E_FONT_UNKNOWN` is raised and the project has user fonts, the message lists the bundled families and the project fonts separately.

## Part C: command details

Part C gives, for every command of Part B: the argument table with types and defaults, preconditions, postconditions, the errors with an example message each, the inverse it produces, its idempotency, one example of input and output, and the list of golden cases required by A.11. It is written by the owner area from Parts A and B before implementation starts, and reviewed by the orchestrator; a contradiction between Part C and Parts A or B is resolved in favour of A and B and reported as a document revision.

<!-- PART C IS APPENDED BELOW THIS LINE -->

### Shared rules used by the sections of Part C

Each section refers to these rules by number. They add detail to Parts A and B and do not change them.

- **R1. Order of checks.** Schema validation first (`E_VALIDATION`), then the preconditions in the order listed in the section; the first failure is returned. An identifier argument that names nothing gives `E_NOT_FOUND` with `details.kind` (`item`, `track`, `asset`, `word`, `keyframe`) and `details.id`.
- **R2. No-op.** When the operation would leave the document unchanged it succeeds, with `diff` empty, `inverse` empty and `effective` filled as usual. The batch still counts as one revision. "Same result" in the idempotency paragraphs means this.
- **R3. Stored form in diffs.** A diff and an inverse use the canonical stored form (SDD-001 §2.8). A field equal to its default is absent, so it appears as `null` on that side of a change (for example `"hidden": [null, true]`).
- **R4. Settings and project diffs.** `settings.updated` and `project.updated` carry the project id as `id`. Their primitives use `collection` `settings` or `project` with `id` equal to the project id.
- **R5. Inverse order.** The `Inverse` list is in the order in which the operation mutated the document. Undo applies it in reverse (A.6). Every `patch` carries `set` for fields that existed before and `unset` for fields that were absent before.
- **R6. Identifiers.** A supplied `id` or `new_id` must match `^<prefix>_[0-9a-hjkmnp-tv-z]{10}$` (`E_VALIDATION` otherwise). If it is already used in that collection the result is `E_ID_EXISTS`. A missing one comes from `env.NewIDs`. `effective` always reports the identifier used.
  - A slot name already used by another item is not an identifier clash: it is `E_SLOT` with `details.reason` `"name_in_use"`.
  - `ast_placeholder` is a reserved literal, not an asset identifier. It is admitted only where `slot.fill`, `clip.add` and `audio.add` accept an asset reference, and it never matches a real asset. Every other command that takes an `asset_id` rejects it with `E_VALIDATION`, because it does not match the identifier pattern.
- **R7. Times and lengths.** A time argument is parsed by A.4. A length is a time that is not a position and must be at least 1 frame, unless the argument is marked "may be 0"; otherwise `E_INVALID_TIME`. A resulting item with `start + duration` above 108,000 gives `E_INVALID_TIME` with `path` at the argument that caused it. `effective` reports each time argument as in A.4 and, for lengths, the same triple.
- **R8. Free position.** `next_free(track, from, length, ignore)` is the smallest frame `f` at or after `from` such that `[f, f + length)` overlaps no item of the track except `ignore`; it is `null` when `f + length` would pass 108,000. In an `E_OVERLAP`, `details.conflicting_item` is the overlapping item with the lowest `start` and `details.next_free` is the value above. For `item.trim`, `next_free` is instead the nearest value of `to` that avoids the overlap.
- **R9. Source range.** `A` is the asset duration in project frames (SDD-001 §3.2). For a `clip` or `audio` item with `source_in = si` and speed `num/den`, the range holds when `si + ceil(duration × num / den) <= A`. The longest duration for a given `si` is `maxdur(si) = floor((A - si) × den / num)`. Images are exempt. `E_OUT_OF_SOURCE` carries `details.available = {"asset_duration": A, "source_in": si, "max_duration": maxdur(si)}`.
- **R10. Keyframe value.** `value_at(K, t)` is the interpolated value of a keyframe list at relative frame `t` (SDD-004). Before the first keyframe it is the first value, after the last it is the last value. Examples in this part use linear easing. A value written as a new keyframe or static field is rounded to 3 decimals (A.10). When a property has no keyframes its value is its static field, with these defaults when absent: `scale` 1, `rotation` 0, `opacity` 1, `volume` 1, `x` half the project width, `y` half the project height.
- **R11. KShift(d).** The item loses its first `d` frames (`d` may be negative, meaning it gains `-d`). For each property with keyframes: let `v0 = value_at(K, d)` when `d > 0`; every `t` becomes `t - d`; keyframes with a negative `t` are dropped. If any was dropped and keyframes remain whose first `t` is above 0, a keyframe `{t: 0, value: v0}` is inserted, with the easing of the last dropped keyframe (omitted when linear). If every keyframe was dropped, the property's keyframes are removed and its static field becomes `v0`. An empty `keyframes` object is removed.
- **R12. KCut(L).** The item is cut to `L` frames at the back. For each property: keyframes with `t > L` are dropped. If any was dropped and keyframes remain whose last `t` is below `L`, a keyframe `{t: L, value: value_at(K, L)}` is appended with no easing. If every keyframe was dropped, the keyframes are removed and the static field becomes `value_at(K, L)`.
- **R13. KSplit(Tc).** Cut at relative frame `Tc`, `0 < Tc < duration`. For each property with keyframes `K`, let `B` be the keyframes with `t < Tc`, `E` the keyframe at exactly `Tc` if any, `A` the keyframes with `t > Tc`, and `v = value_at(K, Tc)`.
  - Left part: `B`, then `E` if it exists. If `E` does not exist, and `A` is not empty and `B` is not empty, the cut keyframe `{t: Tc, value: v}` with no easing is appended. If `B` and `E` are both empty the left part has no keyframes for that property and its static field is `v`.
  - Right part: `A` shifted by `-Tc`, preceded by `{t: 0, value: v}` only when `A` is not empty and (`B` is not empty or `E` exists). The easing of that keyframe is the easing of `E`, else of the last keyframe of `B` (omitted when linear). If `A` is empty the right part has no keyframes for that property and its static field is `v`.
  - A part that lost at least one keyframe gains at most one, so the limit of 100 keyframes per property cannot be exceeded.
  - For an eased (non-linear) segment that is cut, the two halves keep the segment's easing, so the curve is only approximately preserved; for linear and hold it is exact.
- **R14. Fit.** The invariants `fade_in + fade_out <= duration` and `transition_in.duration + transition_out.duration <= duration` must hold after the command. A command that would break them returns `E_INVALID_STATE` with a hint that names the amount to shorten and the command to use (`item.set_audio`, `item.set_transition`). The commands `settings.set_fps`, `clip.set_speed` and `clip.remove_silences` instead apply the **fit fix**: reduce `fade_out` by the excess (minimum 0), then `fade_in`; reduce `transition_out.duration` by the excess, removing the transition if that leaves less than 1, then the same for `transition_in`.
- **R15. Style arguments.** A style argument is an object whose keys are fields of TextStyle (CaptionStyle for captions); an unknown key is `E_VALIDATION`. Each value obeys the ranges of SDD-001 §3.6 and §3.7. A colour is a hex colour or `"$name"` with `name` in `template.colors`; anything else is `E_VALIDATION` with `path` at the field. After merging, `font_family` and `font_weight` must exist (bundled set or `fonts`), otherwise `E_FONT_UNKNOWN` with `details.available` listing the families, or the weights of the family when only the weight is wrong. A style object (a text item `style`, a caption item `style` override, a track `caption_style`) stores exactly the keys the caller set, whatever their values, even a value equal to the default. The rule of SDD-001 that omits defaults does not apply inside a style object, because an explicit key on an item overrides a different value on the track. A key leaves the object only when the caller passes `null` for it on a merge. An empty result is stored as absent. `null` values are rejected in commands that create items.
- **R16. Word arguments.** `words` is an array of 1 to 60 objects `{text, start, duration}`; the schema accepts any length of at least 1 so that the count is reported as `E_LIMIT`. `text` has 1 to 80 characters (Unicode code points) and no whitespace. `start` and `duration` are lengths relative to the item (`start` may be 0, `duration` at least 1 frame). Words must be in order and must not overlap. A time that does not parse, or a `duration` below 1 frame like any length that parses but is too short, is `E_INVALID_TIME` with `path` at the value (`args.words[i].duration`). An empty `words` array is `E_VALIDATION`. Any other violation (text length, whitespace in text, order, overlap) is `E_VALIDATION` with `path` `args.words[i]`. More than 60 words in one caption is `E_LIMIT` in every command, and so is the document total of 5,000 words.
- **R17. Split core S(X, Tc).** Used by `item.split` and `caption.split`. The right part is a copy of `X` with the new id. Left: `duration = Tc`. Right: `start = X.start + Tc`, `duration = X.duration - Tc`; for `clip` and `audio`, `source_in = X.source_in + floor(Tc × num / den)`. `transition_out` and `fade_out` move from the left to the right; `transition_in` and `fade_in` stay on the left and are removed from the right; `slot` stays on the left only. Each kept transition or fade longer than its part is shortened to the part's duration. Keyframes follow R13. Words are handled by the command.
- **R18. Resizing at the back.** When a command changes an item's duration to `D'` and keeps its `start`: if `D' < D` keyframes follow R12; fades and transitions are checked by R14; growth is checked for overlap by R8 and for the 108,000 limit by R7; for `clip` and `audio` the source range is checked by R9.
- **R19. Diff order.** Entries are sorted by kind group (settings, assets, tracks, items) and then by identifier, as in A.5.

**Where the shared rules live.** Each rule has one implementation in `core/command`, named here so a command never carries a copy: R8 `nextFree` and R9 `maxDur` in `nextfree.go` (R9's `checkSource` stays with the item helpers in `item.go`); R10 to R13 (`kShift`, `kCut`, `kSplit`) in `keyframes_common.go`, evaluating values through `timeline.ValueAt` and `timeline.Ease` of `core/timeline/interp.go`, which `core/plan` also calls (SDD-004 §3.1); R14 `checkFit` and the fit fix `fitFix` in `fit.go`; R15 style merge and `E_FONT_UNKNOWN` in `style.go`; R16 word arguments and the `E_LIMIT` of 60 words per caption and 5,000 per document in `words.go`; R17 `splitCore` in `split_core.go`. `core/command` does not import `core/plan`; `internal/tools/importcheck` fails the build when it does.

### Decisions of Part C

Part C makes choices that Parts A and B leave open or do not state. Each one is listed here so that it is visible and can be challenged in one place. The sections apply them.

| # | Command | Decision | Why |
| --- | --- | --- | --- |
| 1 | `settings.set_fps` | The fix-ups run in the order a (rescale, a duration that became 0 becomes 1), b (source range), c (overlap, one pass per track over items sorted by start and id), d (keyframes), e (words, a caption keeps at least one word), f (fit fix of R14), each against the result of the previous one. An item whose end passes 108,000 after a or c is `E_INVALID_TIME` and nothing is applied. `effective.adjusted` lists every item changed by a `became_zero` correction or by b to f | A fix that runs before a step that still shortens items can leave an invariant broken |
| 2 | R15, `track.set_caption_style`, `text.add`, `text.set`, `caption.set_style` | A style object (a text item `style`, a caption item `style`, a track `caption_style`) stores exactly the keys the caller set, even when a value equals the default. A key leaves only when the caller passes `null`. The rule of SDD-001 that omits defaults does not apply inside style objects | An explicit item key must override a different value on the track, so it cannot be dropped |
| 3 | `clip.set_speed` | The new duration is `ceil(duration x old / new)`. If that fails R9 only by rounding, it is reduced to `maxdur(source_in)` and reported in `effective.clamped`; `E_OUT_OF_SOURCE` only when no frame fits | A speed change should succeed when it can, and the source range is exact |
| 4 | `clip.set_speed` | The speed string has at most 3 decimals (`"0.333x"` is 333/1000) | A bounded decimal maps exactly to a rational |
| 5 | `item.crossfade` | Requires `from.start < to.start` and `min(from.end, to.end) - to.start >= duration`, else `E_INVALID_STATE` | The two fades must lie inside the real overlap in the intended order |
| 6 | `caption.merge` | The operation removes the second caption first, then extends the first. The inverse list is the put of the second, then the patch of the first | An object must not exist before it is put, and must exist before it is patched |
| 7 | `caption.merge` | Refuses captions that have keyframes (`E_INVALID_STATE`). The first caption keeps `style`, `slot`, `transition_in`, `name`, position and `linked_item`; it takes the `transition_out` of the second; every other field of the second is dropped and listed in `effective.dropped`. The slot of the second is dropped | Keyframes of two captions cannot be joined without guessing; the rest follows the first caption |
| 8 | Undo of commands that shift items (`item.delete`, `timeline.close_gap`, `clip.set_speed`, `clip.remove_silences`, `slot.fill`) | The primitives of one undo are applied to a working copy without validating in between, and the result is validated once at the end (A.6). Inverse lists are in mutation order (R5) | No per-command ordering of shifts is needed |
| 9 | `caption.set_words`, `caption.retime_from_clip`, `item.split`, `caption.split`, `item.trim` | Inverse and diff carry every changed field, including the static fields written by R11, R12 or R13 | Undo must restore the static value that a keyframe cut wrote |
| 10 | `item.split`, `caption.split` (R13) | For each property: the left part gets the keyframes before the cut and the keyframe at the cut if any; a cut keyframe is added only when both sides keep keyframes. The right part gets a keyframe at 0 only when it keeps keyframes and the cut had keyframes before it or on it. A part left with none gets the static value. An eased segment is preserved only approximately | A part that lost keyframes gains at most one, so the limit of 100 cannot be exceeded |
| 11 | R16, `caption.add`, `caption.set_words`, `slot.fill` | A word `duration` below 1 frame is `E_INVALID_TIME` with `path` at the value. An empty `words` array is `E_VALIDATION` | A length that parses but is too short is a time error, like every other length |
| 12 | All commands | An item end past frame 108,000 is `E_INVALID_TIME` in every command, `settings.set_fps` included. A time argument outside the item it addresses (`keyframe.set`, `keyframe.remove`, `item.split`) is `E_INVALID_TIME` with `path` at the argument and the inclusive range in the message | One condition gets one code in every command |
| 12a | All commands | More than 60 words in one caption is `E_LIMIT` (R16, `caption.add`, `caption.set_words`, `caption.merge`, `slot.fill`). A word index that does not exist is `E_NOT_FOUND` with `details.kind` `word` (`caption.edit_word`, `caption.split`) | Same reason as row 12 |
| 12b | `slot.mark` | A slot name used by another item is `E_SLOT` with `details.reason` `"name_in_use"` | A name clash is not an identifier clash |
| 13 | `slot.fill` with `words` | Checks 60 words in the caption and 5,000 in the document (`E_LIMIT`) | The command writes words, so it must enforce the same limits as the others |
| 14 | `clip.remove_silences`, `caption.from_transcript` | The caller may pass `new_ids`. A shorter list is completed from `env.NewIDs`; extra ids are ignored and listed in `effective.unused_ids`. `effective.new_ids` lists the ids used and the service stores the operation with that list. Duplicates in `new_ids` are `E_VALIDATION`. With `replace` true, ids are checked after the replaced captions are removed | A stored operation must replay identically (A.3 rule 4) |
| 15 | `caption.retime_from_clip` | Transcript words go to the linked captions in `start` order by word count. A caption whose word count is unchanged keeps its edited texts; otherwise it takes the transcript texts and is listed in `effective.text_reset`. A caption that receives no word is removed and listed in `effective.removed`; the removal is in the diff and is undone by undo | Keeps the behaviour of Part B and makes the other cases explicit |
| 16 | This table | Every extension of Part C beyond Parts A and B is listed here | Nothing is hidden in a section |
| 17 | R12, `settings.set_fps`, `item.trim`, `caption.set_words` | A keyframe written by R12 is at `t` equal to the new duration. This is allowed because `t` ranges from 0 to `duration` | One rule for every cut at the back |
| 18 | All commands with arguments | An argument belongs to its command: all values of `item.delete` with `ripple` are accepted wherever the command is registered | The registry stays unambiguous |
| 19 | `caption.add` | With `text`, every word gets at least 1 frame and the rest is shared in proportion to character counts by largest remainder, lower index first on ties | A deterministic split with no zero-length word |
| 20 | `item.set_transition` | A `duration` together with `kind: null` is `E_VALIDATION` | Removing a transition has no length |
| 21 | `clip.add` | `source_in` on an image is `E_VALIDATION`. A default duration that would end after 108,000 is clamped to `108000 - start`; only an explicit duration is an error | An image has no source position; a default should not fail |
| 22 | `clip.add`, `audio.add`, `slot.fill`, R6 | `ast_placeholder` is a reserved literal. `clip.add` and `audio.add` accept it, skip the asset checks and then require `duration`. `slot.fill` and every other command treat it as no real asset | It never matches a real asset |
| 23 | `clip.set_framing` | A `crop` merges with the current crop side by side; a side not given keeps its value; the merged sums must stay below 1 on each axis | A partial crop must not be able to hide the whole image |
| 24 | `shape.add`, `shape.set` | `radius` is valid only with `rounded_rect`; changing the shape to another one removes a stored `radius` | A radius on another shape has no meaning |
| 25 | `caption.set_breaks` | `breaks` is strictly increasing and each entry is from 1 to the word count minus 1 | A break at 0 or after the last word does nothing |
| 26 | `caption.from_transcript` | `max_words` is 1 to 60 and `max_chars` is 1 to 500 | Bounds the size of a caption |
| 27 | `slot.unmark` | Refused (`E_INVALID_STATE`) for an unfilled media slot that holds the placeholder | The item would keep a placeholder asset with no slot, which is invalid |
| 28 | `template.set_info` | `description` has 0 to 500 characters; colour names match `[a-z][a-z0-9_]{0,39}`; removing an absent name is ignored | Bounds the template data |
| 29 | `item.delete` | Duplicate ids in `item_ids` are `E_VALIDATION` | A repeated id has no meaning |
| 30 | `item.set_transform`, `shape.add`, `text.add` | `x` and `y` are stored once set, even at the frame centre | An explicit position must stay explicit after a size change |
| 31 | `item.set_audio`, `clip.remove_silences`, `caption.from_transcript` | Lengths that may be 0: `fade_in`, `fade_out`, `padding`, `max_gap`. Every other length is at least 1 frame | A zero fade, padding or gap is meaningful |
| 32 | `clip.remove_silences` | The first piece keeps the id, `transition_in`, `fade_in` and `slot`; the last keeps `transition_out` and `fade_out`; a single piece keeps all | One identity for the original clip |
| 33 | `settings.set` | `scale` and text `size` are untouched when the frame size changes | Part B states it; the rule is repeated for the golden cases |
| 34 | R2 | A no-op succeeds with an empty diff and an empty inverse and still counts as a revision | Retries are safe |
| 35 | R4 | The diff `id` of `settings.updated` and `project.updated` is the project id | These objects have no id of their own |
| 36 | `track.set_caption_style` (A.10) | `CaptionStyle.highlight` accepts `"none"` and `"word"`. `caption.add` takes no style, so the rule has one place | Word timing is data, so `words` are accepted, and the highlight is drawn when the style asks for it (RF-032) |
| 37 | `caption.add` | `x` and `y` are optional and are used when the position of the caption style is `custom`; they are stored on the item. `item.set_transform` is a separate command | A custom position is reachable without that command |

### C.1 `project.rename`

Sets the project name.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `name` | string | yes | none | 1 to 120 characters, stored exactly as given (no trimming) |

**Preconditions.**

1. `name` is a string of 1 to 120 characters (`E_VALIDATION`).

**Postconditions.**

1. `name` of the document is the given string.
2. `diff` has one `project.updated` entry with `changes.name`; empty when the name is unchanged.
3. `effective` is `{"name": "<name>"}`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `name` is empty or longer than 120 characters | `args.name must have 1 to 120 characters, got 121.` | `Send a name of at most 120 characters, for example "Launch teaser".` |

**Inverse.** `patch {collection: "project", id: <project id>, set: {name: <old name>}}`.

**Idempotency.** Same result (R2): the second application changes nothing.

**Example.** Document: project `prj_7k2m9x4q1b` named "Launch teaser".

```json
{"op": "project.rename", "args": {"name": "Launch teaser v2"}}
```

```json
[{"kind": "project.updated", "id": "prj_7k2m9x4q1b", "changes": {"name": ["Launch teaser", "Launch teaser v2"]}}]
```

**Golden cases.**

- `ok_basic`
- `ok_min_length_1`
- `ok_max_length_120`
- `ok_same_name_noop`
- `err_validation_empty_name`
- `err_validation_121_chars`

### C.2 `settings.set`

Sets frame size, background colour or loudness target, rescaling the position of visual items when the size changes.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `width` | integer | no | unchanged | even, 16 to 4096 |
| `height` | integer | no | unchanged | even, 16 to 4096 |
| `background` | colour | no | unchanged | `#RRGGBB` or `#RRGGBBAA` or `"$name"`; stored upper case; or `"transparent"` (VV-172), stored as is: no background colour, see SDD-001 §3.1 |
| `loudness_target` | number | no | unchanged | -30 to -5 |

At least one argument is required.

**Preconditions.**

1. At least one argument is present and each obeys its rule (`E_VALIDATION`).

**Postconditions.**

1. The given fields of `settings` are set; an omitted field is untouched.
2. If `width` changes, for every visual item and every `x` keyframe value: `x' = round3(x × new_width / old_width)`. If `height` changes, the same for `y` with the heights. Only explicitly stored `x` and `y` are scaled; an item with no `x` or `y` stays centred. `scale`, text `size`, shape sizes and `max_width` are not touched (Part B).
3. `diff` has `settings.updated` (field changes) followed by one `item.updated` per item whose `x`, `y` or `keyframes` changed.
4. `effective` has the values applied for every given setting, plus `rescaled_items` (the number of items changed by point 2).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | no argument, odd or out-of-range size, bad colour, bad loudness, unknown `$name` | `args.width must be even and between 16 and 4096, got 1081.` | `Use width 1080 or 1082.` |

**Inverse.** One `patch {collection: "settings", id: <project id>, set: <old values of the fields that existed>, unset: <given fields that were absent, because they held the default>}`, then one `patch {collection: "items", id, set: {x, y, keyframes as they were}, unset: [the fields that were absent before]}` for each rescaled item; a field that was absent (for example `x` on an item at the default position) is in `unset`, never in `set` (R5).

**Idempotency.** Same result: the second application finds the values already set and rescales nothing.

**Example.** A 1080x1920 project with one text item `itm_s6t7v8w9x0` at `x` 810, `y` 480 and an `x` keyframe list `[{t:0,value:810},{t:30,value:270}]`.

```json
{"op": "settings.set", "args": {"width": 720, "height": 1280}}
```

```json
[
  {"kind": "settings.updated", "id": "prj_7k2m9x4q1b", "changes": {"width": [1080, 720], "height": [1920, 1280]}},
  {"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {
    "x": [810, 540], "y": [480, 320],
    "keyframes": [{"x": [{"t": 0, "value": 810}, {"t": 30, "value": 270}]}, {"x": [{"t": 0, "value": 540}, {"t": 30, "value": 180}]}]}}
]
```

Width 1080 and height 1920 are the defaults, so they are absent before the change (R3).

**Golden cases.**

- `ok_width_only`
- `ok_height_only`
- `ok_both_sizes_rescale_items`
- `ok_default_position_items_stay_centred`
- `ok_keyframed_x_rescaled`
- `ok_background`
- `ok_background_with_alpha_uppercased`
- `ok_background_transparent`
- `ok_loudness_boundaries_minus30_minus5`
- `ok_size_boundaries_16_and_4096`
- `ok_same_values_noop`
- `err_validation_no_argument`
- `err_validation_odd_width`
- `err_validation_width_15`
- `err_validation_width_4098`
- `err_validation_loudness_minus31`
- `err_validation_unknown_color_reference`
- `err_validation_background_unknown_keyword`

### C.3 `settings.set_fps`

Changes the project frame rate, rescaling every time quantity and repairing the invariants that rounding can break.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `fps` | string or rational object | yes | none | `"30"` (means 30/1), `"30000/1001"`, or `{"num": int, "den": int}`; `den` is 1 or 1001; `num/den` from 1 to 120 |

**Preconditions.**

1. `fps` is one of the three forms with `den` 1 or 1001 and `num/den` from 1 to 120 (`E_VALIDATION`; a decimal such as `"29.97"` is rejected).
2. After the rescale (step a below) and after the overlap repair (step c), every item ends at or before frame 108,000 (`E_INVALID_TIME`, R7, `path` `args.fps`). Nothing is applied.

**Postconditions.**

1. Let the ratio be `r = (new.num × old.den) / (new.den × old.num)`. The fix-ups run in the order a to f below, each against the result of the previous one. An earlier step never relies on a later one, so no invariant is checked before the last step has run.
   - **a. Rescale.** Every time quantity `q` becomes `round(q × r)`, with halves rounded up, in integer arithmetic. The quantities are: item `start`, `duration`, `source_in`, `fade_in`, `fade_out`, `transition_in.duration`, `transition_out.duration`, keyframe `t` and caption word `start`. A caption word is rescaled by its two ends: its `start` and its end (`start + duration`) become `round(x × r)` and the new `duration` is the difference, so touching words keep touching and a rounding error never adds up from one word to the next. Speeds, asset facts and `breaks` are not changed. An item `duration`, transition duration or word duration that became 0 becomes 1 (reason `became_zero`). Rounding loses information, so a round trip through several rates can move a word by one frame; the words stay contiguous.
   - **b. Source range.** Source range (R9, with `A` computed at the new rate): any `clip` or `audio` item whose range fails R9 gets `duration = maxdur(source_in)`. If that is below 1, `source_in` becomes `max(0, A - ceil(num / den))` and `duration` is recomputed the same way.
   - **c. Overlap.** One pass per track over its items sorted by `(start, id)`. For an item X with previous item P (the item processed just before X, after its own fix): if `P.start + P.duration > X.start` then, when `X.start > P.start`, set `P.duration = X.start - P.start`; otherwise (`X.start <= P.start`, which is the equal-start case, or X lying before a P that was itself moved) set `s = P.start + 1 - X.start`, then `P.duration = 1`, `X.start = X.start + s` and `X.duration = max(1, X.duration - s)`. X then becomes P for the next item. Each fix only shortens P or moves X later, and never touches a pair already processed, so one pass is enough. For two items with equal starts, `s` is 1: X moves by one frame.
   - **d. Keyframes.** Against the final duration: keyframes that now share a `t` are reduced to the last of them (reason `keyframe_collision`); then R12 is applied with `L = duration` when keyframes lie beyond it (reason `keyframe_cut`).
   - **e. Words.** Against the final duration: words are processed in order, `start = max(start, previous end)` (reason `word_overlap`); a word whose end passes the duration is shortened to end there and a word with no frame left is removed (reason `word_clamp`). A caption always keeps at least one word: if none would survive, the first word is kept with `start` 0 and `duration` 1.
   - **f. Fit.** The fit fix of R14 against the final duration (reason `fit`).
2. An item whose end would pass 108,000 after step a, or after step c, is `E_INVALID_TIME` and nothing is applied.
3. `settings.fps` is the new rational. A project already at that rate is unchanged (R2).
4. `diff`: `settings.updated` with `fps`, then `item.updated` for every changed item.
5. `effective` has `fps`, `ratio` as `{num, den}` and `adjusted`: a list of `{"id", "field", "reason", "rescaled", "final"}` for every item changed by a `became_zero` correction or by steps b to f, in order of step and then of id, where `reason` is one of `became_zero`, `source_range`, `overlap`, `keyframe_collision`, `keyframe_cut`, `word_overlap`, `word_clamp`, `fit`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `fps` is malformed or outside the allowed set | `args.fps "29.97" is not a valid frame rate; den must be 1 or 1001.` | `Use "30000/1001" for 29.97 fps.` |
| `E_INVALID_TIME` | an item would end after frame 108,000 | `Item itm_m1n2p3q4r5 would end at frame 144000 at 60 fps; the limit is 108000.` | `Shorten the item so that it ends at or before frame 54000 at 30 fps first, or keep 30 fps.` |

**Inverse.** `patch {collection: "settings", id, set: {fps: <old>}}` (or `unset` when it was the default), then one `patch {collection: "items", id, set: <every time field as before>, unset: [...]}` for each changed item.

**Idempotency.** Same result: the second application finds the rate already set. Applying rate A, then B, then A again is not an identity (the rounding is lossy, ADR-0004).

**Example.** A 30 fps project, one visual track `trk_f6g7h8j9k0` with three text items. The operation changes to 12 fps, so `r = 12/30 = 2/5`.

| Item | Before (start, duration) | Rescaled | After fix-ups |
| --- | --- | --- | --- |
| `itm_a1b2c3d4e5` | 4, 4 | 2 (1.6), 2 (1.6) | start 2, duration 1 (overlaps the next item, which starts at 3) |
| `itm_f6g7h8j9k0` | 8, 5 | 3 (3.2), 2 (2.0) | start 3, duration 2 |
| `itm_p7q8r9s0t1` | 15, 1 | 6 (6.0), 0 (0.4) | start 6, duration 1 (became zero) |

```json
{"op": "settings.set_fps", "args": {"fps": "12"}}
```

```json
[
  {"kind": "settings.updated", "id": "prj_7k2m9x4q1b", "changes": {"fps": [null, {"num": 12, "den": 1}]}},
  {"kind": "item.updated", "id": "itm_a1b2c3d4e5", "changes": {"start": [4, 2], "duration": [4, 1]}},
  {"kind": "item.updated", "id": "itm_f6g7h8j9k0", "changes": {"start": [8, 3], "duration": [5, 2]}},
  {"kind": "item.updated", "id": "itm_p7q8r9s0t1", "changes": {"start": [15, 6]}}
]
```

`effective.adjusted` is `[{"id": "itm_p7q8r9s0t1", "field": "duration", "reason": "became_zero", "rescaled": 0, "final": 1}, {"id": "itm_a1b2c3d4e5", "field": "duration", "reason": "overlap", "rescaled": 2, "final": 1}]` (step order first: `became_zero` belongs to step a).

As built, `field` is the name of the changed field (`start`, `duration`, `source_in`, `fade_in`, `fade_out`, `transition_in.duration`, `transition_out.duration`, `words[i].duration` for a `became_zero` word, `keyframes.<property>` and `words` for steps d and e); for `keyframes.<property>` `rescaled` and `final` are the lists of `t`, for `words` the lists of `[start, duration]`; a transition removed by the fit fix has `final` 0 and a keyframe list that disappears has `final` null. A caption's `breaks` that point past the last word after step e are dropped.

**Golden cases.**

- `ok_30_to_24_string_form`
- `ok_to_ntsc_30000_1001_string_form`
- `ok_object_form`
- `ok_same_rate_noop`
- `ok_duration_became_zero`
- `ok_overlap_shortens_earlier_item`
- `ok_overlap_equal_starts_shifts_later_item`
- `ok_equal_start_after_rescale`
- `ok_overlap_after_source_range_shortening`
- `ok_keyframes_rescaled_and_collision_reduced`
- `ok_caption_words_rescaled_and_repaired`
- `ok_caption_keeps_one_word`
- `ok_fades_and_transitions_fit_fix`
- `ok_source_range_repaired_at_asset_end`
- `ok_boundary_120_fps`
- `ok_boundary_1_fps`
- `err_validation_decimal_string`
- `err_validation_den_not_1_or_1001`
- `err_validation_121_fps`
- `err_invalid_time_item_passes_108000`
- `ok_markers_rescaled`

### C.4 `asset.add`

Adds an asset that the import flow has already ingested, or reports the existing asset with the same hash.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `asset` | Asset object (SDD-001 §3.2) | yes | none | `id` is an `ast_` identifier; `hash` is `sha256:` plus 64 hex characters; `video` requires `frame_rate`, `frame_count`, `width`, `height`; `audio` requires `sample_count`; `image` requires `width`, `height`; `sample_count` is required exactly when the asset has audio (kind `audio`, or kind `video` with `has_audio` true); `lut` (TASK-0254) requires nothing more |

**Preconditions.**

1. `asset` satisfies the shape rules above (`E_VALIDATION`).
2. If an asset with the same `hash` is present, the command succeeds as a no-op (Postcondition 3) and the checks below are skipped.
3. `asset.id` is not used by an asset with a different hash (`E_ID_EXISTS`).
4. The document has fewer than 200 assets (`E_LIMIT`).

**Postconditions.**

1. `assets[asset.id]` is the given object, with `has_audio` omitted when false.
2. `diff` has one `asset.added` with the whole object under `object`.
3. When the hash is already present, nothing changes, `effective.asset_id` is the existing identifier (which may differ from `asset.id`) and `effective.existing` is true.
4. Otherwise `effective` is `{"asset_id": "<id>", "existing": false}`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | a required field of the kind is missing, or a field is malformed | `args.asset.frame_count is required for kind "video".` | `Send the facts measured on the normalized media: frame_rate, frame_count, width and height.` |
| `E_ID_EXISTS` | the id is used by an asset with another hash | `Asset id ast_3c5v8n2w6d already exists with a different hash.` | `Send a new id, or omit it so the service generates one.` |
| `E_LIMIT` | 200 assets already exist | `The document already has 200 assets; the limit is 200.` | `Remove an unused asset with asset.remove first.` |

**Inverse.** `remove {collection: "assets", id: <asset.id>}`; nothing for the no-op.

**Idempotency.** Same result: the second application finds the hash and returns the existing identifier without a change.

**Example.** An empty project.

```json
{"op": "asset.add", "args": {"asset": {"id": "ast_3c5v8n2w6d", "hash": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", "kind": "video", "name": "IMG_0421.MOV", "frame_rate": {"num": 30, "den": 1}, "frame_count": 900, "sample_count": 1440000, "width": 1080, "height": 1920, "has_audio": true}}}
```

```json
[{"kind": "asset.added", "id": "ast_3c5v8n2w6d", "object": {"id": "ast_3c5v8n2w6d", "hash": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", "kind": "video", "name": "IMG_0421.MOV", "frame_rate": {"num": 30, "den": 1}, "frame_count": 900, "sample_count": 1440000, "width": 1080, "height": 1920, "has_audio": true}}]
```

**Golden cases.**

- `ok_video_with_audio`
- `ok_video_without_audio`
- `ok_audio`
- `ok_image`
- `ok_same_hash_returns_existing_id`
- `ok_199_assets_then_add`
- `err_validation_video_missing_frame_count`
- `err_validation_sample_count_missing_with_audio`
- `err_validation_bad_hash`
- `err_id_exists_other_hash`
- `err_limit_200_assets`
- `ok_lut`

### C.5 `asset.remove`

Removes an asset from the project when no item uses it.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `asset_id` | asset id | yes | none | `ast_` identifier |

**Preconditions.**

1. `asset_id` matches the `ast_` identifier pattern (`E_VALIDATION`). The reserved literal `ast_placeholder` does not match it here (R6).
2. `asset_id` exists in `assets` (`E_NOT_FOUND`).
3. No item has `asset` or `lut.asset` equal to `asset_id` (`E_IN_USE`, `details.used_by` lists the item ids sorted).

**Postconditions.**

1. `assets[asset_id]` is removed. Nothing else changes; the file stays in the content store (ADR-0006).
2. `diff` has one `asset.removed` with the object.
3. `effective` is `{"asset_id": "<id>"}`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `asset_id` is not an identifier of an asset, including `ast_placeholder` | `args.asset_id "ast_placeholder" is not an asset identifier.` | `Use the id of an asset listed by project_get.` |
| `E_NOT_FOUND` | no such asset | `Asset ast_3c5v8n2w6d does not exist.` | `List the project's assets with project_get and use one of their ids.` |
| `E_IN_USE` | an item uses it | `Asset ast_3c5v8n2w6d is used by items itm_m1n2p3q4r5, itm_s6t7v8w9x0.` | `Delete those items with item.delete, then remove the asset.` |

**Inverse.** `put {collection: "assets", object: <the asset>}`.

**Idempotency.** Error: the second application returns `E_NOT_FOUND`.

**Example.** A document with asset `ast_3c5v8n2w6d` used by no item.

```json
{"op": "asset.remove", "args": {"asset_id": "ast_3c5v8n2w6d"}}
```

```json
[{"kind": "asset.removed", "id": "ast_3c5v8n2w6d", "object": {"id": "ast_3c5v8n2w6d", "hash": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", "kind": "video", "name": "IMG_0421.MOV", "frame_rate": {"num": 30, "den": 1}, "frame_count": 900, "sample_count": 1440000, "width": 1080, "height": 1920, "has_audio": true}}]
```

**Golden cases.**

- `ok_unused_asset`
- `ok_asset_used_only_by_deleted_item`
- `err_not_found`
- `err_validation_placeholder_id`
- `err_in_use_one_item`
- `err_in_use_several_items_sorted`
- `err_in_use_by_a_lut`

### C.6 `track.add`

Adds an empty track at a place in the track order.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `kind` | `"visual"` or `"audio"` | yes | none | |
| `name` | string | no | `""` | 0 to 60 characters |
| `id` | track id | no | generated | R6 |
| `position` | `"top"` or `"bottom"` | no | none of the three given: `"top"` for a visual track, `"bottom"` for an audio track (below the visual ones, as in an NLE) | at most one of `position`, `above`, `below` (`E_VALIDATION` if two are present) |
| `above` | track id | no | none | the new track is placed directly over this track |
| `below` | track id | no | none | the new track is placed directly under this track |

**Preconditions.**

1. Arguments match the schema, and at most one of `position`, `above`, `below` is present (`E_VALIDATION`).
2. `above` or `below` names an existing track (`E_NOT_FOUND`, `details.kind` `track`).
3. `id`, when given, is not used by a track (`E_ID_EXISTS`).
4. The document has fewer than 50 tracks (`E_LIMIT`).

**Postconditions.**

1. A track `{id, kind, name, order}` is added; `hidden`, `muted` and `caption_style` are absent. The order is one global sequence for both kinds (SDD-001 §3.3.1).
2. `tracks` is a map by id, so a listing of it has no stacking: the stack is the `order` field, ascending, and the smallest is the top layer, whatever the batch or the kind of the tracks added before. `order` is computed from the sorted list of existing orders: `top` is `OrderBetween("", first)`; `bottom` is `OrderBetween(last, "")`; `above X` is `OrderBetween(prev, X.order)` where `prev` is the order of the track just before `X` (or `""`); `below X` is `OrderBetween(X.order, next)` where `next` is the order of the track just after `X` (or `""`). With no tracks, `OrderBetween("", "")`. The `order` strings in this section are illustrative: the exact strings are fixed by the algorithm of SDD-001 section 3.3.1 and by the golden files.
3. No other track is changed.
4. `diff` has one `track.added`. `effective` is `{"track_id": "<id>", "order": "<order>"}`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad `kind`, name over 60 characters, or two placement arguments | `args: position and above cannot be used together.` | `Send only one of position, above and below.` |
| `E_NOT_FOUND` | `above` or `below` names no track | `Track trk_zzzzzzzzzz does not exist (args.above).` | `Use the id of a track listed by project_get.` |
| `E_ID_EXISTS` | `id` is already a track | `Track id trk_f6g7h8j9k0 already exists.` | `Omit id to let the service generate one.` |
| `E_LIMIT` | 50 tracks exist | `The document already has 50 tracks; the limit is 50.` | `Remove an unused track with track.remove first.` |

**Inverse.** `remove {collection: "tracks", id: <new id>}`.

**Idempotency.** With the same `id`: error `E_ID_EXISTS`. Without `id`: another track is added each time.

**Example.** A document with one visual track `trk_f6g7h8j9k0` with `order` `a1`.

```json
{"op": "track.add", "args": {"kind": "visual", "name": "Captions", "id": "trk_a1b2c3d4e5"}}
```

```json
[{"kind": "track.added", "id": "trk_a1b2c3d4e5", "object": {"id": "trk_a1b2c3d4e5", "kind": "visual", "name": "Captions", "order": "a0"}}]
```

**Golden cases.**

- `ok_default_top_on_empty_document`
- `ok_default_top`
- `ok_audio_explicit_top`
- `ok_position_bottom`
- `ok_above`
- `ok_below`
- `ok_audio_kind`
- `ok_name_60_chars`
- `ok_supplied_id`
- `ok_49_tracks_then_add`
- `err_validation_two_placements`
- `err_validation_name_61_chars`
- `err_not_found_above`
- `err_id_exists`
- `err_limit_50_tracks`

### C.7 `track.update`

Sets the name, the hidden, muted, locked, solo or magnet flag, or the volume of a track.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `track_id` | track id | yes | none | |
| `name` | string | no | unchanged | 0 to 60 characters; `""` clears it |
| `hidden` | boolean | no | unchanged | visual tracks only |
| `muted` | boolean | no | unchanged | both kinds |
| `locked` | boolean | no | unchanged | both kinds; `true` refuses every edit of the items of the track (A.3 rule 5) until it is `false` again. VV-036 |
| `solo` | boolean | no | unchanged | both kinds; while any track is solo, only soloed tracks sound, and while any visual track is solo, only soloed visual tracks are drawn (SDD-004 section 7) |
| `magnet` | boolean | no | unchanged | both kinds; `true` packs the items of the track from frame 0 with no gap, now and after every later command (TASK-0254, SDD-001 section 3.3); a locked track is not repacked; `false` leaves the items where they are |
| `volume` | number | no | unchanged | both kinds; 0 to 4, rounded to 3 decimals; a factor on the volume of the items of the track; 1 removes the field |

At least one of `name`, `hidden`, `muted`, `locked`, `solo`, `magnet`, `volume` is required.

**Preconditions.**

1. At least one field is present and valid (`E_VALIDATION`).
2. The track exists (`E_NOT_FOUND`).
3. `hidden` is only given for a visual track (`E_TYPE_MISMATCH`). `volume` is 0 to 4 (`E_VALIDATION`).

**Postconditions.**

1. The given fields are set. A field set to its default (`""`, `false`) is removed from the stored track.
2. `diff` has one `track.updated` with the changed fields (R3); empty when nothing changes. With `magnet` the items that were packed move too: one `item.updated` with `start` for each, in the same operation and the same undo step. The same repack runs after every other command for each unlocked track with `magnet`.
3. `effective` has the stored values of the given fields.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | nothing to set, or a value out of range | `args: give at least one of name, hidden, muted, locked, solo, magnet, volume.` | `Send for example {"track_id": "trk_f6g7h8j9k0", "muted": true}.` |
| `E_NOT_FOUND` | unknown track | `Track trk_zzzzzzzzzz does not exist.` | `Use the id of a track listed by project_get.` |
| `E_TYPE_MISMATCH` | `hidden` on an audio track | `Track trk_m1n2p3q4r5 is an audio track; hidden applies to visual tracks only.` | `Use muted to silence an audio track.` |

**Inverse.** `patch {collection: "tracks", id, set: <old values of the changed fields>, unset: <fields that were absent>}`.

**Idempotency.** Same result.

**Example.** A document with visual track `trk_f6g7h8j9k0` (not hidden).

```json
{"op": "track.update", "args": {"track_id": "trk_f6g7h8j9k0", "hidden": true}}
```

```json
[{"kind": "track.updated", "id": "trk_f6g7h8j9k0", "changes": {"hidden": [null, true]}}]
```

**Golden cases.**

- `ok_rename`
- `ok_clear_name`
- `ok_hide_visual`
- `ok_unhide_removes_field`
- `ok_mute_audio_track`
- `ok_mute_visual_track`
- `ok_lock`
- `ok_unlock_removes_field`
- `ok_solo_audio_track`
- `ok_magnet_on_packs_the_track`
- `ok_magnet_off_keeps_the_items`
- `ok_magnet_on_a_packed_track_only_sets_the_flag`
- `ok_magnet_on_locked_track_keeps_the_items`
- `ok_volume`
- `ok_volume_1_removes_field`
- `ok_edit_a_locked_track_itself`
- `ok_same_values_noop`
- `ok_name_60_chars`
- `err_validation_volume_above_4`
- `err_validation_no_field`
- `err_validation_name_61_chars`
- `err_not_found`
- `err_type_mismatch_hidden_on_audio`

### C.8 `track.move`

Changes the place of a track in the track order.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `track_id` | track id | yes | none | |
| `position` | `"top"` or `"bottom"` | one of the three | none | exactly one of `position`, `above`, `below` |
| `above` | track id | one of the three | none | must differ from `track_id` |
| `below` | track id | one of the three | none | must differ from `track_id` |

**Preconditions.**

1. Exactly one of `position`, `above`, `below` is present (`E_VALIDATION`).
2. `track_id` and the reference track exist (`E_NOT_FOUND`).
3. The reference track is not `track_id` itself (`E_INVALID_STATE`).

**Postconditions.**

1. The new `order` is computed as in C.6 over the sorted orders without the moved track.
2. When the track already has that place (first for `top`, last for `bottom`, directly over or under the reference), nothing changes (R2).
3. `diff` has one `track.updated` with `order`. Items are untouched.
4. `effective` is `{"track_id": "<id>", "order": "<new order>"}`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | none or more than one placement argument | `args: give exactly one of position, above, below.` | `Send {"track_id": "trk_f6g7h8j9k0", "position": "top"}.` |
| `E_NOT_FOUND` | unknown track or reference | `Track trk_zzzzzzzzzz does not exist (args.above).` | `Use the id of a track listed by project_get.` |
| `E_INVALID_STATE` | a track placed relative to itself | `Track trk_f6g7h8j9k0 cannot be placed above itself.` | `Name a different reference track, or use position.` |

**Inverse.** `patch {collection: "tracks", id, set: {order: <old>}}`.

**Idempotency.** Same result: after the move the track is already in place.

**Example.** Tracks `trk_a1b2c3d4e5` (order `a0`), `trk_f6g7h8j9k0` (`a1`), `trk_m1n2p3q4r5` (`a2`).

```json
{"op": "track.move", "args": {"track_id": "trk_m1n2p3q4r5", "below": "trk_a1b2c3d4e5"}}
```

```json
[{"kind": "track.updated", "id": "trk_m1n2p3q4r5", "changes": {"order": ["a2", "a0v"]}}]
```

`OrderBetween("a0", "a1")` is shown as `a0v` in this example. The string is illustrative: the exact strings are fixed by the algorithm of SDD-001 section 3.3.1 and by the golden files.

**Golden cases.**

- `ok_to_top`
- `ok_to_bottom`
- `ok_above`
- `ok_below`
- `ok_already_in_place_noop`
- `err_validation_no_placement`
- `err_validation_two_placements`
- `err_not_found_track`
- `err_not_found_reference`
- `err_invalid_state_relative_to_itself`

### C.9 `track.remove`

Removes a track, and its items when `with_items` is true.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `track_id` | track id | yes | none | |
| `with_items` | boolean | no | `false` | when true, the items of the track are removed too |

**Preconditions.**

1. The track exists (`E_NOT_FOUND`).
2. The track has no items, or `with_items` is true (`E_NOT_EMPTY`, `details.items` lists the item ids sorted).

**Postconditions.**

1. Every item of the track (when `with_items`) is removed, then the track. Captions elsewhere that link to a removed item keep the dangling `linked_item` (allowed by SDD-001 §3.7).
2. `diff`: `track.removed` first, then one `item.removed` per item, sorted by id (R19).
3. `effective` is `{"track_id": "<id>", "removed_items": ["<ids sorted>"]}`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_NOT_FOUND` | unknown track | `Track trk_zzzzzzzzzz does not exist.` | `Use the id of a track listed by project_get.` |
| `E_NOT_EMPTY` | items on the track and `with_items` false | `Track trk_f6g7h8j9k0 has 2 items: itm_m1n2p3q4r5, itm_s6t7v8w9x0.` | `Send "with_items": true to remove the items with the track, or move them first.` |

**Inverse.** `put {collection: "items", object}` for each removed item in the order they were removed, then `put {collection: "tracks", object}`. Undo applies them in reverse, so the track returns before its items.

**Idempotency.** Error: the second application returns `E_NOT_FOUND`.

**Example.** A track `trk_f6g7h8j9k0` holding one text item.

```json
{"op": "track.remove", "args": {"track_id": "trk_f6g7h8j9k0", "with_items": true}}
```

```json
[
  {"kind": "track.removed", "id": "trk_f6g7h8j9k0", "object": {"id": "trk_f6g7h8j9k0", "kind": "visual", "order": "a1"}},
  {"kind": "item.removed", "id": "itm_s6t7v8w9x0", "object": {"id": "itm_s6t7v8w9x0", "type": "text", "track": "trk_f6g7h8j9k0", "start": 0, "duration": 60, "text": "Hello"}}
]
```

**Golden cases.**

- `ok_empty_track`
- `ok_with_items_removes_items`
- `ok_with_items_false_explicit_on_empty_track`
- `ok_dangling_linked_item_left_alone`
- `err_not_found`
- `err_not_empty`

### C.10 `track.set_caption_style`

Merges a partial caption style into a track's caption style, or replaces it.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `track_id` | track id | yes | none | a visual track |
| `style` | partial CaptionStyle | yes | none | R15; stored exactly as given, keys equal to a default included; `null` values remove keys when merging and are ignored when replacing |
| `replace` | boolean | no | `false` | true: `caption_style` becomes exactly `style` |

**Preconditions.**

1. Arguments match the schema and R15 (`E_VALIDATION`).
2. The track exists (`E_NOT_FOUND`).
3. The track is visual (`E_TYPE_MISMATCH`).
4. The resulting `font_family` and `font_weight` exist (`E_FONT_UNKNOWN`).

**Postconditions.**

1. Merge: each key of `style` is set (or removed for `null`) on `caption_style`, with the value given even when it equals the default (R15). Replace: `caption_style` is `style` without its `null` keys. An empty result removes `caption_style` from the track.
2. Items of the track are not changed; their effective style follows SDD-001 §3.7.
3. `diff` has one `track.updated` with `caption_style` as a whole object; empty when nothing changes.
4. `effective` is `{"caption_style": <stored style after the command>}`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | unknown key, value out of range, bad colour, unknown `$name` | `args.style.size must be between 8 and 1000, got 4.` | `Use size 64.` |
| `E_NOT_FOUND` | unknown track | `Track trk_zzzzzzzzzz does not exist.` | `Use the id of a track listed by project_get.` |
| `E_TYPE_MISMATCH` | audio track | `Track trk_m1n2p3q4r5 is an audio track and cannot hold captions.` | `Use a visual track.` |
| `E_FONT_UNKNOWN` | family or weight not available | `Font "Comic" is not available; available families: Inter, Roboto.` | `Use font_family "Inter".` |

**Inverse.** `patch {collection: "tracks", id, set: {caption_style: <old>}}` or `unset: ["caption_style"]` when there was none.

**Idempotency.** Same result.

**Example.** Track `trk_a1b2c3d4e5` has `caption_style {"highlight": "none"}`.

```json
{"op": "track.set_caption_style", "args": {"track_id": "trk_a1b2c3d4e5", "style": {"size": 72, "position": "middle"}}}
```

```json
[{"kind": "track.updated", "id": "trk_a1b2c3d4e5", "changes": {"caption_style": [{"highlight": "none"}, {"highlight": "none", "position": "middle", "size": 72}]}}]
```

**Golden cases.**

- `ok_merge_into_existing`
- `ok_merge_into_none`
- `ok_replace`
- `ok_null_value_removes_key`
- `ok_key_equal_to_default_is_kept`
- `ok_replace_with_empty_style_removes_caption_style`
- `ok_template_color_reference`
- `ok_same_style_noop`
- `err_validation_unknown_key`
- `err_validation_size_below_8`
- `err_validation_unknown_color_reference`
- `err_not_found_track`
- `err_type_mismatch_audio_track`
- `err_font_unknown_family`
- `err_font_unknown_weight`

### C.11 `item.move`

Moves an item in time, to another track of the same kind, or both.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | |
| `start` | time | no | unchanged | absolute position of the item's first frame |
| `track_id` | track id | no | unchanged | a track that accepts the item's type |
| `slide` | boolean | no | `false` | the slide edit (VV-036): needs `start`, not with `track_id`. The item on the same track that ends where the moved item starts gets its end edge moved to the new start, and the item that starts where it ends gets its start edge moved to the new end, as `item.trim` does (source, keyframes and words follow). A side with no touching item is left alone, so a slide without neighbours is a plain move |

At least one of `start` and `track_id` is required.

**Preconditions.**

1. At least one of `start`, `track_id` is present, and `start` parses (`E_VALIDATION`, `E_INVALID_TIME`).
2. The item exists, and `track_id` (when given) exists (`E_NOT_FOUND`).
3. The target track accepts the item type: visual types on a visual track, `audio` on an audio track (`E_TYPE_MISMATCH`).
4. The new end `start + duration` is at most 108,000 (`E_INVALID_TIME`, R7).
5. The item does not overlap another item of the target track, ignoring itself (`E_OVERLAP`, R8).
6. With `slide`: `start` is given and `track_id` is not (`E_VALIDATION`); each trimmed neighbour keeps at least one frame (`E_INVALID_STATE`), its source range holds (`E_OUT_OF_SOURCE`) and its fades and transitions still fit.

**Postconditions.**

1. `start` and `track` of the item are set. Every other field is unchanged: `keyframes`, caption `words`, transitions, fades and `slot` are relative to the item and travel with it. When the item changes track, the effective caption style becomes that of the new track.
2. `diff` has one `item.updated` with `start` and/or `track`; empty for a no-op (R2).
3. `effective` has `start` (the A.4 triple) and `track_id`.

**Linked items (TASK-0277).** When the item has a `link` (SDD-001 section 3.5), every other item of its link group moves by the same number of frames as the item did (`start - old start`), on its own track; a `track_id` change moves the named item only. A linked item that would start before frame 0 or end after 108,000 is `E_INVALID_TIME`, one that would overlap on its track is `E_OVERLAP` (the message names the linked item and the hint says to move by less or unlink it), and a linked item on a locked track refuses the whole command with `E_INVALID_STATE` like any edit of a locked track. `effective.moved_with` lists the linked ids (sorted) when any moved. `slide` neighbours are those of the named item only.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | neither `start` nor `track_id` | `args: give at least one of start and track_id.` | `Send {"item_id": "itm_m1n2p3q4r5", "start": "5s"}.` |
| `E_INVALID_TIME` | bad time string, or the item would end after 108,000 | `args.start "100000s" puts item itm_m1n2p3q4r5 at frames 3000000-3000149, past the limit 108000.` | `Use a start of at most "107850f" for this 150-frame item.` |
| `E_NOT_FOUND` | unknown item or track | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | track does not accept the type | `Item itm_m1n2p3q4r5 is a clip; track trk_m1n2p3q4r5 is an audio track.` | `Move it to a visual track.` |
| `E_INVALID_STATE` | with `slide`, a neighbour would be left with no frame | `Trimming the start of itm_c3d4e5f6g7 to frame 490 leaves no frame: the item ends at 340.` | `Slide by less, or trim the neighbour itm_c3d4e5f6g7 with item.trim first.` |
| `E_OUT_OF_SOURCE` | with `slide`, a neighbour would read outside its asset | `Moving the start of itm_c3d4e5f6g7 from frame 250 to 220 needs source frame -20 at speed 1/1 (source_in 10); the asset begins at 0.` | `Slide by less, or trim the neighbour itm_c3d4e5f6g7 with item.trim first.` |
| `E_OVERLAP` | overlap on the target | `Item itm_m1n2p3q4r5 would occupy frames 100-249 on track trk_f6g7h8j9k0, where item itm_c3d4e5f6g7 already is (frames 90-179).` | `Use start "180f" (6.000s), the next free position on that track, or move to another track.` |

**Inverse.** `patch {collection: "items", id, set: {start: <old>, track: <old>}}` for the fields that changed.

**Idempotency.** Same result: the item is already there.

**Example.** A 30 fps document: clip `itm_m1n2p3q4r5` on `trk_f6g7h8j9k0`, `start` 0, `duration` 150, nothing else on the track.

```json
{"op": "item.move", "args": {"item_id": "itm_m1n2p3q4r5", "start": "5s"}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"start": [0, 150]}}]
```

`effective.start` is `{"frame": 150, "seconds": 5, "requested": "5s"}`.

**Golden cases.**

- `ok_move_in_time`
- `ok_move_to_other_track_same_kind`
- `ok_move_time_and_track`
- `ok_move_next_to_neighbour_touching`
- `ok_move_to_frame_0`
- `ok_move_ending_at_frame_108000`
- `ok_keyframes_and_words_unchanged`
- `ok_same_position_noop`
- `err_validation_no_argument`
- `err_invalid_time_malformed`
- `err_invalid_time_ends_after_108000`
- `err_not_found_item`
- `err_not_found_track`
- `err_type_mismatch_audio_track`
- `err_overlap_in_time`
- `err_overlap_on_other_track`
- `ok_slide_later_trims_the_previous_and_next`
- `ok_slide_earlier_trims_the_previous_and_next`
- `ok_slide_without_neighbours_is_a_move`
- `err_validation_slide_without_start`
- `err_validation_slide_with_track_id`
- `err_invalid_state_slide_leaves_no_frame_in_the_next_item`
- `err_out_of_source_slide_earlier_next_item_reads_before_zero`
- `err_overlap_slide_into_a_non_touching_item`
- `err_overlap_slide_extends_the_previous_over_another_item`
- `ok_linked_items_move_together`
- `ok_linked_items_move_earlier_together`
- `ok_linked_move_to_other_track_moves_only_the_item`
- `ok_linked_peer_out_of_the_way_unaffected`
- `err_invalid_time_linked_peer_before_frame_0`
- `err_overlap_linked_peer`
- `err_invalid_state_linked_peer_on_a_locked_track`
- `ok_magnet_repacks_after_a_move_to_the_front`
- `ok_a_track_without_magnet_is_not_packed`

### C.12 `item.trim`

Moves the start edge or the end edge of an item to a timeline position, keeping the source, keyframes and words consistent.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | any item type |
| `edge` | `"start"` or `"end"` | yes | none | |
| `to` | time | yes | none | absolute timeline position of the edge; `end` is exclusive (the first frame after the item) |
| `ripple` | `"none"` or `"track"` | no | `"none"` | `"track"` shifts every later item of the track (start at or after the old end of the item) by the change in duration, so a gap is closed or opened; with edge `start` the item then keeps its `start` and only its duration and source change. VV-036 |
| `roll` | boolean | no | `false` | the roll edit (VV-036): the edge of the neighbour that touches the trimmed one moves to `to` as well, so the cut moves and no other time changes. For edge `end` the neighbour is the item of the track that starts at the old end; for edge `start` the item that ends at the old start. The neighbour is trimmed like any item (source, keyframes, words), and `effective.rolled_item` names it. Not with `ripple` `"track"` (`E_VALIDATION`); with no touching neighbour `E_INVALID_STATE` |

**Preconditions.**

1. Arguments match the schema and `to` parses (`E_VALIDATION`, `E_INVALID_TIME`).
2. The item exists (`E_NOT_FOUND`).
3. Edge `start`: `to < start + duration`. Edge `end`: `to > start` (`E_INVALID_STATE`). Edge `end` with `to > 108000` is `E_INVALID_TIME`.
4. Edge `start` moving earlier on a `clip` or `audio` item: the new `source_in` is not negative (`E_OUT_OF_SOURCE`). Edge `end` growing a `clip` or `audio` item: the source range holds (R9, `E_OUT_OF_SOURCE`). Images are exempt.
5. A caption keeps at least one word (`E_INVALID_STATE`).
6. Fades and transitions still fit the new duration (R14, `E_INVALID_STATE`).
7. The item does not overlap another item of its track (`E_OVERLAP`, R8; `next_free` is the nearest valid `to`). With `roll` the neighbour is trimmed first, so the cut can move either way; it keeps at least one frame (`E_INVALID_STATE`), its source range holds (`E_OUT_OF_SOURCE`) and its fades and transitions fit. With ripple `"track"` the later items move first, so growing never overlaps them, but none may end after frame 108,000 (`E_INVALID_TIME`).

**Postconditions.**

1. Edge `start`, with `d = to - start` (negative when the item grows to the left): `start = to`, `duration = duration - d`. For `clip` and `audio` (not image clips), with speed `num/den`: for `d > 0`, `source_in = source_in + floor(d × num / den)`; for `d < 0`, `source_in = source_in - ceil(-d × num / den)`.
2. Edge `start`, keyframes: R11 with that `d`. Caption words: each word `start` becomes `start - d`; a word whose new end is at or before 0 is removed; a word that crosses 0 starts at 0 and keeps its end. `breaks` follow the removed words: with `r` words removed, an entry `i` becomes `i - r` and is dropped when below 1 or at or beyond the new word count.
3. Edge `end`: `duration = to - start`; `start` and `source_in` do not change. If the item shrinks, keyframes follow R12 with `L = to - start`. Words with `start >= duration` are removed and a word that crosses the new end gets `duration = new duration - start`; `breaks` drop entries at or beyond the new word count.
4. `fade_in`, `fade_out`, `transition_in`, `transition_out` keep their lengths.
5. `diff` has one `item.updated` with the changed fields (`start`, `duration`, `source_in`, `keyframes`, `words`, `breaks` and the static fields written by R11 or R12). No change when `to` is the current edge (R2).
6. `effective` has `to` (the A.4 triple), `start`, `duration` and, for `clip` and `audio`, `source_in`, all as applied.

**Linked items (TASK-0277).** When the item has a `link`, the same edge of every other item of its link group moves by the same number of frames as the edge of the item (`to` minus the old edge), each with the rules of its own type (source range, fades, transitions, words, R14), and the first linked item that cannot follow refuses the command with the error it would give alone, the hint naming it. `ripple` and `roll` apply to the named item only. A linked item on a locked track refuses the command as for any edit of a locked track. `effective.trimmed_with` lists the linked ids (sorted) when any was trimmed.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad `edge` or missing argument | `args.edge must be "start" or "end", got "left".` | `Use "start" or "end".` |
| `E_INVALID_TIME` | malformed `to`, or `to` above 108,000 for edge `end` | `args.to "2000000f" is past the timeline limit of 108000 frames.` | `Use a value up to "108000f" (3600s).` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_INVALID_STATE` | edge past the other edge; caption left with no word; fades or transitions no longer fit | `Trimming the start of itm_m1n2p3q4r5 to frame 240 leaves no frame: the item ends at 240.` | `Use a start before "240f", or delete the item.` |
| `E_OUT_OF_SOURCE` | the source would start before 0 or end after the asset | `Moving the start of itm_m1n2p3q4r5 from frame 90 to 40 needs source frame -15 at speed 3/2 (source_in 60); the asset begins at 0.` | `Use to "50f" (1.667s), the earliest start this clip allows at speed 3/2.` |
| `E_OVERLAP` | the new range overlaps a neighbour | `Item itm_m1n2p3q4r5 would occupy frames 70-239 on track trk_f6g7h8j9k0, where item itm_c3d4e5f6g7 already is (frames 0-79).` | `Use to "80f" (2.667s), the nearest start that avoids the overlap.` |

**Inverse.** `patch {collection: "items", id, set: <old values of every changed field>, unset: <fields that were absent>}`. One primitive.

**Idempotency.** Same result: the second application finds the edge already at `to` (`d = 0`).

**Example.** 30 fps project; asset `ast_3c5v8n2w6d` has 900 frames at 30 fps. Clip `itm_m1n2p3q4r5` on `trk_f6g7h8j9k0`: `start` 90, `duration` 150, `source_in` 60, `speed` 3/2 (source range 60 + ceil(150 × 3/2) = 285), `fade_in` 10, `fade_out` 12, `transition_in` fade 15 frames, `keyframes.scale` = `[{t:0, value:1}, {t:30, value:1.3, easing:"ease_out"}, {t:100, value:1}]`. Trim the start edge to frame 97, so `d = 7`.

| Field | Before | After | Why |
| --- | --- | --- | --- |
| `start` | 90 | 97 | `to` |
| `duration` | 150 | 143 | 150 - 7 |
| `source_in` | 60 | 70 | 60 + floor(7 × 3/2) = 60 + floor(10.5) |
| keyframe `t` values | 0, 30, 100 | 23, 93, plus a new one at 0 | each `t - 7`; the one at 0 becomes -7 and is dropped |
| new keyframe at 0 | none | value 1.07 | linear value at relative frame 7: 1 + 0.3 × 7/30 |

```json
{"op": "item.trim", "args": {"item_id": "itm_m1n2p3q4r5", "edge": "start", "to": "97f"}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {
  "start": [90, 97], "duration": [150, 143], "source_in": [60, 70],
  "keyframes": [
    {"scale": [{"t": 0, "value": 1}, {"t": 30, "value": 1.3, "easing": "ease_out"}, {"t": 100, "value": 1}]},
    {"scale": [{"t": 0, "value": 1.07}, {"t": 23, "value": 1.3, "easing": "ease_out"}, {"t": 93, "value": 1}]}]}}]
```

`effective.to` is `{"frame": 97, "seconds": 3.233, "requested": "97f"}`.

The same clip trimmed at the end with `{"edge": "end", "to": "150f"}` (and, for this variant only, a linear second keyframe) gets `duration` 60. The keyframe at 100 is dropped, and since the last remaining keyframe is at 30, a keyframe at 60 is appended with value `1.3 + (1 - 1.3) × 30/70 = 1.171`.

**Golden cases.**

- `ok_start_edge_shrink_clip_speed_1`
- `ok_start_edge_shrink_clip_speed_3_2_floor`
- `ok_start_edge_extend_clip_source_decreases`
- `ok_start_edge_extend_to_source_in_exactly_0`
- `ok_start_edge_keyframes_interpolated_at_0`
- `ok_start_edge_all_keyframes_dropped_becomes_static`
- `ok_start_edge_caption_words_removed_and_clamped`
- `ok_start_edge_image_clip_source_in_untouched`
- `ok_start_edge_text_item`
- `ok_start_edge_to_frame_0`
- `ok_end_edge_shrink_keyframes_cut`
- `ok_end_edge_extend_within_source`
- `ok_end_edge_extend_to_asset_end_exactly`
- `ok_end_edge_extend_image_clip`
- `ok_end_edge_caption_word_clamped`
- `ok_end_edge_to_frame_108000`
- `ok_edge_already_at_to_noop`
- `err_validation_bad_edge`
- `err_invalid_time_malformed`
- `err_invalid_time_end_past_108000`
- `err_not_found`
- `err_invalid_state_start_edge_at_item_end`
- `err_invalid_state_end_edge_at_item_start`
- `err_invalid_state_caption_no_word_left`
- `err_invalid_state_fade_no_longer_fits`
- `err_invalid_state_transition_no_longer_fits`
- `err_out_of_source_start_before_zero`
- `err_out_of_source_end_past_asset`
- `err_overlap_start_edge_extends_into_neighbour`
- `err_overlap_end_edge_extends_into_neighbour`
- `ok_start_edge_shrink_drops_and_shifts_markers`
- `ok_start_edge_extend_shifts_markers`
- `ok_end_edge_shrink_drops_markers_past_the_end`
- `ok_ripple_end_edge_shrink_closes_the_gap`
- `ok_ripple_end_edge_extend_pushes_later_items`
- `ok_ripple_start_edge_shrink_keeps_the_start`
- `ok_ripple_start_edge_extend_pushes_later_items`
- `ok_ripple_none_is_the_default`
- `err_overlap_end_edge_extend_without_ripple`
- `err_invalid_time_ripple_pushes_past_108000`
- `ok_roll_end_edge_earlier_grows_the_next_item`
- `ok_roll_end_edge_later_shrinks_the_next_item`
- `ok_roll_start_edge_later_grows_the_previous_item`
- `ok_roll_start_edge_earlier_shrinks_the_previous_item`
- `err_validation_roll_with_ripple`
- `err_invalid_state_roll_without_a_touching_neighbour`
- `err_invalid_state_roll_leaves_no_frame_in_the_neighbour`
- `err_out_of_source_roll_neighbour_reads_before_zero`
- `ok_linked_trim_end_together`
- `ok_linked_trim_start_together`
- `ok_linked_trim_extend_start_together`
- `err_invalid_state_linked_peer_would_have_no_frame`
- `err_invalid_state_linked_peer_on_a_locked_track`
- `ok_magnet_follows_a_trim_of_the_end`
- `ok_freeze_frame_start_keeps_the_held_frame`
- `ok_freeze_frame_end_grows_past_the_clip_length`

### C.13 `item.split`

Splits an item into two items at a timeline frame.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | any item type |
| `at` | time | yes | none | absolute timeline position, strictly inside the item |
| `new_id` | item id | no | generated | R6; the id of the right part |

**Preconditions.**

1. Arguments match the schema and `at` parses (`E_VALIDATION`, `E_INVALID_TIME`).
2. The item exists (`E_NOT_FOUND`).
3. `start < at < start + duration`, that is, `at` lies in the inclusive range `start + 1` to `start + duration - 1` (`E_INVALID_TIME`, `path` `args.at`).
4. `new_id`, when given, is not used by an item (`E_ID_EXISTS`).
5. The document has fewer than 500 items (`E_LIMIT`).
6. For a caption, both parts receive at least one word (`E_INVALID_STATE`).

**Postconditions.** With `Tc = at - start`, R17 applies: the original keeps its id and becomes the left part with `duration = Tc`; the right part is a new item with `start = at`, `duration = old duration - Tc` and, for `clip` and `audio`, `source_in = old source_in + floor(Tc × num / den)`.

1. `transition_in` and `fade_in` stay on the left; `transition_out` and `fade_out` go to the right; each is shortened to its part's duration if longer; `slot` stays on the left only.
2. Keyframes follow R13: for each animated property, the keyframes before the cut stay on the left and those after it go to the right, shifted by `-Tc`; a keyframe at the cut goes to both parts; an interpolated keyframe at the cut is written only where R13 says so (a part that would otherwise lose all its motion gets the static value instead).
3. Caption words: a word goes to the left part when `start + word.start < at`, else to the right. In the left part a word that crosses the cut gets `duration = Tc - word.start`. In the right part `word.start` becomes `word.start - Tc`. `breaks`: the left keeps entries `i` with `1 <= i <= left word count - 1`; the right keeps entries `i > left word count` as `i - left word count`. `linked_item` and `style` are copied to the right part.
4. Every other field (name, position, scale, rotation, opacity, text, style, asset, speed, volume, framing, colour, shape fields) is copied to the right part unchanged.
5. `diff`: `item.updated` for the left part and `item.added` for the right part, sorted by id.
6. `effective` has `at` (the A.4 triple), `new_id`, `left` and `right`, each `{"id", "start", "duration"}`.

**Linked items (TASK-0277).** When the item has a `link`, every other item of its link group that `at` falls strictly inside is split at the same frame by the same rule; the others are left alone. The left parts keep the link group, and the right parts, the new item and the right parts of the linked items, form a new link group whose id is `lnk_` plus the ten characters of `new_id`; if no linked item was cut the right part has no link, since a group of one links nothing. `new_ids` supplies the ids of the right parts of the cut linked items, in the order of the ids of those items (missing ones come from the generator); `effective.new_ids` and `effective.split_with` list them when any was cut. A linked item that cannot be split (a speed curve, a caption with no word on a side) refuses the command, the hint naming it, and a linked item on a locked track refuses it as for any edit of a locked track.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | missing argument or bad `new_id` form | `args.new_id "clip-2" is not an item id: an id is "itm_" followed by 10 characters from 0-9 and a-z without i, l, o and u.` | `Use an id like "itm_s6t7v8w9x0" or omit it.` |
| `E_INVALID_TIME` | malformed `at`, or `at` not strictly inside the item | `args.at "60f" is not inside item itm_m1n2p3q4r5 (frames 60-179); it must be between 61 and 179 inclusive.` | `Use at "61f" or later, up to "179f".` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_INVALID_STATE` | a caption part would have no word | `Splitting caption itm_m1n2p3q4r5 at frame 70 leaves the left part with no word.` | `Split at a word boundary with caption.split.` |
| `E_ID_EXISTS` | `new_id` already used | `Item id itm_s6t7v8w9x0 already exists.` | `Omit new_id or use an unused id.` |
| `E_LIMIT` | 500 items exist | `The document already has 500 items; the limit is 500.` | `Delete unused items before splitting.` |

**Inverse.** `patch {collection: "items", id: <left id>, set: <old values of every changed field, including static fields written by R13>, unset: <fields that were absent>}`, then `remove {collection: "items", id: <new id>}`, in the order of mutation (R5).

**Idempotency.** Error: after the first split, `at` equals the left part's end, so the second application returns `E_INVALID_TIME`.

**Example.** 30 fps project; asset `ast_3c5v8n2w6d` has 900 frames. Clip `itm_m1n2p3q4r5` on `trk_f6g7h8j9k0`: `start` 60, `duration` 120 (frames 60-179), `source_in` 30, `speed` 3/2, `fade_in` 12, `fade_out` 12, `transition_in` fade 40, `transition_out` slide_left 10, `keyframes.opacity` = `[{t:0, value:1}, {t:60, value:0.4}]` (linear). Split at frame 97, so `Tc = 37`.

| Field | Left (`itm_m1n2p3q4r5`) | Right (`itm_s6t7v8w9x0`) |
| --- | --- | --- |
| `start`, `duration` | 60, 37 | 97, 83 |
| `source_in` | 30 | 30 + floor(37 × 3/2) = 30 + floor(55.5) = 85 |
| `fade_in`, `fade_out` | 12, none | none, 12 |
| `transition_in`, `transition_out` | fade 37 (40 shortened to the part), none | none, slide_left 10 |
| `opacity` keyframes | `t0: 1`, `t37: 0.63` | `t0: 0.63`, `t23: 0.4` |

The cut value is the linear value at relative frame 37: `1 + (0.4 - 1) × 37/60 = 0.63`.

```json
{"op": "item.split", "args": {"item_id": "itm_m1n2p3q4r5", "at": "97f", "new_id": "itm_s6t7v8w9x0"}}
```

```json
[
  {"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {
    "duration": [120, 37],
    "fade_out": [12, null],
    "transition_in": [{"kind": "fade", "duration": 40}, {"kind": "fade", "duration": 37}],
    "transition_out": [{"kind": "slide_left", "duration": 10}, null],
    "keyframes": [{"opacity": [{"t": 0, "value": 1}, {"t": 60, "value": 0.4}]}, {"opacity": [{"t": 0, "value": 1}, {"t": 37, "value": 0.63}]}]}},
  {"kind": "item.added", "id": "itm_s6t7v8w9x0", "object": {
    "id": "itm_s6t7v8w9x0", "type": "clip", "track": "trk_f6g7h8j9k0", "start": 97, "duration": 83,
    "asset": "ast_3c5v8n2w6d", "source_in": 85, "speed": {"num": 3, "den": 2},
    "fade_out": 12, "transition_out": {"kind": "slide_left", "duration": 10},
    "keyframes": {"opacity": [{"t": 0, "value": 0.63}, {"t": 23, "value": 0.4}]}}}
]
```

**Golden cases.**

- `ok_clip_speed_1`
- `ok_clip_speed_3_2_floor_source_in`
- `ok_audio_item`
- `ok_image_clip`
- `ok_text_item`
- `ok_shape_item`
- `ok_at_first_frame_plus_1`
- `ok_at_last_frame`
- `ok_transition_longer_than_left_part_shortened`
- `ok_fade_in_stays_left_fade_out_goes_right`
- `ok_slot_stays_left`
- `ok_keyframes_cut_between_two_keyframes`
- `ok_keyframes_cut_exactly_on_a_keyframe`
- `ok_keyframes_all_before_cut`
- `ok_keyframes_all_after_cut`
- `ok_keyframes_100_before_cut_limit_kept`
- `ok_caption_words_split_by_first_frame`
- `ok_caption_word_crossing_cut_clamped_left`
- `ok_caption_breaks_divided`
- `ok_new_id_supplied`
- `err_validation_bad_new_id`
- `err_invalid_time_no_unit`
- `err_not_found`
- `err_invalid_time_at_equals_start`
- `err_invalid_time_at_equals_end`
- `err_invalid_state_caption_right_part_no_word`
- `err_invalid_state_caption_left_part_no_word`
- `err_id_exists`
- `err_limit_500_items`
- `ok_markers_divided_at_the_cut`
- `ok_linked_split_together`
- `ok_linked_split_with_generated_ids`
- `ok_linked_split_cuts_only_the_items_that_span_the_frame`
- `ok_linked_split_where_no_peer_spans_leaves_the_right_part_unlinked`
- `err_invalid_state_linked_peer_speed_curve`
- `err_invalid_state_linked_peer_on_a_locked_track`
- `ok_freeze_frame_parts_hold_the_same_frame`

### C.14 `item.delete`

Removes items, and optionally closes the freed space on the same track or on all tracks.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_ids` | array of item ids | yes | none | 1 to 200 entries, no duplicates |
| `ripple` | `"none"`, `"track"`, `"all"` | no | `"none"` | all three values are accepted |

**Preconditions.**

1. Arguments match the schema (`E_VALIDATION`).
2. Every id exists (`E_NOT_FOUND`; `details.id` is the first missing id in the given order, `details.missing` lists all).
3. `ripple: "all"`: the deleted items together cover one contiguous range of frames, that is, their intervals, sorted by `start`, leave no uncovered frame between the lowest start and the highest end (`E_INVALID_STATE`, `details.gap` is the first uncovered range).
4. After the shifts no two items overlap (`E_OVERLAP`; possible only with `"all"`, when an item that is not shifted straddles the range).

**Postconditions.**

1. All listed items are removed.
2. `ripple: "track"`: for every item that stays, on a track that lost items, `start` decreases by the sum of the durations of the deleted items of that track whose end is at or before this item's `start`. Gaps are kept.
3. `ripple: "all"`: let `[a, b)` be the covered range; every remaining item on every track with `start >= b` moves earlier by `b - a`. The timeline markers (SDD-001 §3.1 `markers`) follow it (VV-201): one inside `[a, b)` is removed and one at or after `b` moves earlier by `b - a`; `effective` has `markers_removed` and `markers_moved` when any marker is touched. `ripple: "track"` leaves them where they are. The markers of an item are the item's and go with it.
4. `diff`: `item.removed` for every deleted item and `item.updated` (`start`) for every shifted one, sorted by id.
5. `effective` has `deleted` (ids sorted), `shifted` (`[{"id", "from", "to"}]` sorted by id) and `freed` (frames per track as an object, for `"track"`; one number for `"all"`).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | empty or over-long list, duplicates, bad `ripple` | `args.item_ids must have 1 to 200 entries, got 0.` | `Send at least one item id.` |
| `E_NOT_FOUND` | an id does not exist | `Item itm_zzzzzzzzzz does not exist.` | `Use the ids of items listed by project_get.` |
| `E_INVALID_STATE` | `"all"` over a non-contiguous range | `Deleted items cover frames 0-59 and 90-119; frames 60-89 are not covered.` | `Add the item covering frames 60-89 to item_ids, or use ripple "track".` |
| `E_OVERLAP` | a shift creates an overlap | `Item itm_p7q8r9s0t1 would occupy frames 40-59 on track trk_m1n2p3q4r5, where item itm_c3d4e5f6g7 already is (frames 20-69).` | `Use ripple "track", or delete the straddling item too.` |

**Inverse.** `put {collection: "items", object}` for each deleted item (sorted by id), then `patch {collection: "items", id, set: {start: <old>}}` for each shifted item, in the order of mutation (R5). Undo applies the primitives to a working copy without validating in between and validates once at the end (A.6).

**Idempotency.** Error: the second application returns `E_NOT_FOUND`.

**Example.** One visual track `trk_f6g7h8j9k0` with text items `itm_a1b2c3d4e5` (start 0, duration 60), `itm_f6g7h8j9k0` (start 60, duration 30) and `itm_p7q8r9s0t1` (start 100, duration 20).

```json
{"op": "item.delete", "args": {"item_ids": ["itm_a1b2c3d4e5"], "ripple": "track"}}
```

```json
[
  {"kind": "item.removed", "id": "itm_a1b2c3d4e5", "object": {"id": "itm_a1b2c3d4e5", "type": "text", "track": "trk_f6g7h8j9k0", "start": 0, "duration": 60, "text": "One"}},
  {"kind": "item.updated", "id": "itm_f6g7h8j9k0", "changes": {"start": [60, 0]}},
  {"kind": "item.updated", "id": "itm_p7q8r9s0t1", "changes": {"start": [100, 40]}}
]
```

**Golden cases.**

- `ok_one_item_ripple_none`
- `ok_many_items_ripple_none`
- `ok_200_items`
- `ok_ripple_track_one_item`
- `ok_ripple_track_two_items_between_shift_differs`
- `ok_ripple_track_two_tracks_independent`
- `ok_ripple_all_one_track_range`
- `ok_ripple_all_moves_and_drops_timeline_markers`
- `ok_ripple_track_leaves_timeline_markers`
- `ok_ripple_all_range_covered_by_items_on_two_tracks`
- `ok_ripple_track_keeps_gaps`
- `err_validation_empty_list`
- `err_validation_201_ids`
- `err_validation_duplicate_ids`
- `err_not_found_first_missing_reported`
- `err_invalid_state_ripple_all_not_contiguous`
- `err_overlap_ripple_all_straddling_item`
- `ok_magnet_closes_the_gap_of_the_deleted_item`

### C.15 `item.duplicate`

Copies an item to another position, optionally on another track.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | the source |
| `start` | time | no | the first free position at or after the end of the source on the target track | absolute start of the copy |
| `track_id` | track id | no | the source's track | must accept the item type |
| `new_id` | item id | no | generated | R6 |

**Preconditions.**

1. Arguments match the schema and `start`, when given, parses (`E_VALIDATION`, `E_INVALID_TIME`).
2. The item and `track_id` exist (`E_NOT_FOUND`).
3. The target track accepts the item type (`E_TYPE_MISMATCH`).
4. `new_id` is not used (`E_ID_EXISTS`).
5. The document has fewer than 500 items (`E_LIMIT`), and a caption copy keeps the word total at or below 5,000 (`E_LIMIT`).
6. The copy ends at or before 108,000 (`E_INVALID_TIME`) and overlaps no item of the target track, the source included (`E_OVERLAP`, R8).

**Postconditions.**

1. A new item is a copy of the source with the new id, `start` and `track`. Every other field is copied, including `keyframes`, `words`, `linked_item` and `name`. `slot` is not copied.
2. The source is not changed.
3. `diff` has one `item.added`. `effective` has `new_id`, `start` (the A.4 triple) and `track_id`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad id form | `args.new_id "x" is not an item id: an id is "itm_" followed by 10 characters from 0-9 and a-z without i, l, o and u.` | `Use an id like "itm_s6t7v8w9x0" or omit it.` |
| `E_INVALID_TIME` | malformed time, or the copy ends after 108,000 | `The copy of itm_m1n2p3q4r5 would end at frame 108030; the limit is 108000.` | `Use a start of at most "107940f".` |
| `E_NOT_FOUND` | unknown item or track | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | track does not accept the type | `Item itm_m1n2p3q4r5 is a text item; track trk_m1n2p3q4r5 is an audio track.` | `Use a visual track.` |
| `E_ID_EXISTS` | `new_id` used | `Item id itm_s6t7v8w9x0 already exists.` | `Omit new_id or use an unused id.` |
| `E_LIMIT` | 500 items | `The document already has 500 items; the limit is 500.` | `Delete unused items first.` |
| `E_OVERLAP` | the copy overlaps | `Item itm_s6t7v8w9x0 would occupy frames 30-89 on track trk_f6g7h8j9k0, where item itm_m1n2p3q4r5 already is (frames 0-59).` | `Use start "60f" (2.000s), the next free position on that track, or another track.` |

**Inverse.** `remove {collection: "items", id: <new id>}`.

**Idempotency.** Not idempotent: a second application with the same `start` and no `new_id` overlaps the first copy (`E_OVERLAP`); without `start` it makes another copy at the next free position; with the same `new_id` it returns `E_ID_EXISTS`.

**Example.** 30 fps project (so `"3s"` is frame 90). Text item `itm_m1n2p3q4r5` on `trk_f6g7h8j9k0`, `start` 0, `duration` 60, `text` "Hello", marked as slot `title`.

```json
{"op": "item.duplicate", "args": {"item_id": "itm_m1n2p3q4r5", "start": "3s", "new_id": "itm_s6t7v8w9x0"}}
```

```json
[{"kind": "item.added", "id": "itm_s6t7v8w9x0", "object": {"id": "itm_s6t7v8w9x0", "type": "text", "track": "trk_f6g7h8j9k0", "start": 90, "duration": 60, "text": "Hello"}}]
```

**Golden cases.**

- `ok_same_track`
- `ok_other_track`
- `ok_slot_not_copied`
- `ok_keyframes_copied`
- `ok_caption_words_copied`
- `ok_copy_touching_source`
- `ok_copy_ending_at_108000`
- `ok_new_id_supplied`
- `err_validation_bad_new_id`
- `err_invalid_time_end_past_108000`
- `err_not_found_item`
- `err_not_found_track`
- `err_type_mismatch_track_kind`
- `err_id_exists`
- `err_limit_500_items`
- `err_overlap_with_source`
- `err_overlap_with_other_item`

### C.16 `item.rename`

Sets the label of an item.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | |
| `name` | string | yes | none | 0 to 120 characters; `""` clears the label |

**Preconditions.**

1. Arguments match the schema (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).

**Postconditions.**

1. `name` is set; `""` removes the field.
2. `diff` has one `item.updated` with `name`; empty when unchanged. `effective` is `{"name": "<name>"}`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | name over 120 characters | `args.name must have at most 120 characters, got 121.` | `Shorten the label.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |

**Inverse.** `patch {collection: "items", id, set: {name: <old>}}` or `unset: ["name"]`.

**Idempotency.** Same result.

**Example.** Clip `itm_m1n2p3q4r5` without a name.

```json
{"op": "item.rename", "args": {"item_id": "itm_m1n2p3q4r5", "name": "Intro shot"}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"name": [null, "Intro shot"]}}]
```

**Golden cases.**

- `ok_set_name`
- `ok_clear_name`
- `ok_name_120_chars`
- `ok_same_name_noop`
- `err_validation_121_chars`
- `err_not_found`

### C.17 `item.set_transform`

Sets position, scale, rotation or opacity of a visual item.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a visual item |
| `x`, `y` | number | no | unchanged | finite; position of the item's centre in pixels |
| `scale` | number | no | unchanged | greater than 0, at most 20 |
| `rotation` | number | no | unchanged | finite, degrees clockwise |
| `opacity` | number | no | unchanged | 0 to 1 |

At least one of `x`, `y`, `scale`, `rotation`, `opacity` is required.

**Preconditions.**

1. At least one property is present and valid (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).
3. The item is `clip`, `text`, `caption` or `shape` (`E_TYPE_MISMATCH`).
4. None of the given properties has keyframes (`E_INVALID_STATE`, `details.properties` lists them).

**Postconditions.**

1. Each given property is set to its value rounded to 3 decimals. `x` and `y` are always stored once set. `scale` 1, `rotation` 0 and `opacity` 1 remove the field (they are the defaults).
2. `diff` has one `item.updated`; empty when nothing changes. `effective` has the stored values.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | nothing to set, or a value out of range | `args.opacity must be between 0 and 1, got 1.5.` | `Use opacity 1.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | audio item | `Item itm_s6t7v8w9x0 is an audio item and has no transform.` | `Use item.set_audio for audio items.` |
| `E_INVALID_STATE` | a property has keyframes | `Property scale of itm_m1n2p3q4r5 has 2 keyframes, which define its value.` | `Use keyframe.set to change a keyframe, or keyframe.clear to remove them first.` |

**Inverse.** `patch {collection: "items", id, set: <old values>, unset: <fields that were absent>}`.

**Idempotency.** Same result.

**Example.** Text item `itm_s6t7v8w9x0` with default transform.

```json
{"op": "item.set_transform", "args": {"item_id": "itm_s6t7v8w9x0", "x": 540, "scale": 1.5}}
```

```json
[{"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"x": [null, 540], "scale": [1, 1.5]}}]
```

**Golden cases.**

- `ok_set_x_y`
- `ok_set_scale`
- `ok_set_rotation_negative`
- `ok_set_opacity_0_and_1`
- `ok_scale_1_removes_field`
- `ok_value_rounded_to_3_decimals`
- `ok_caption_item`
- `ok_shape_item`
- `ok_same_values_noop`
- `err_validation_no_property`
- `err_validation_scale_0`
- `err_validation_scale_20_001`
- `err_validation_opacity_above_1`
- `err_not_found`
- `err_type_mismatch_audio_item`
- `err_invalid_state_property_has_keyframes`

### C.18 `item.set_transition`

Sets or removes the transition on one edge of a visual item.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a visual item |
| `edge` | `"in"` or `"out"` | yes | none | |
| `kind` | `"fade"`, `"slide_left"`, `"slide_right"`, `"slide_up"`, `"slide_down"`, `"zoom"`, `"whip"`, `"dip"`, `"wipe"`, `"spin"`, `"shrink"`, `"grow"` or `null` | yes | none | `null` removes the transition. The last four were added by TASK-0278 (formulas in SDD-004 section 3.2) |
| `color` | `#RRGGBB` | no | `"#000000"` | `dip` only; on another kind `E_VALIDATION`. Black is stored as absent |
| `direction` | `"left"`, `"right"`, `"up"`, `"down"` | no | `"left"` | `whip` and `wipe` only; on another kind `E_VALIDATION`. `left` is stored as absent |
| `duration` | time, as a length | no | `"0.5s"` | at least 1 frame; rejected with `kind: null` (`E_VALIDATION`). The default applies whenever it is omitted, even when a transition already exists |

A transition needs something visible beneath it (VV-154): the previous clip must run under the edge, on another track below, so that the transition crosses it; otherwise the linter reports `L_TRANSITION_EMPTY` (a `transition_in` at frame 0 and a `transition_out` at the end of the picture, even when a bar, title or caption runs on after it, are not reported). The command itself does not check this. Example: clip A on the lower track ends at 4.5 s and clip B on the upper track starts at 4 s; B gets `{"item_id": B, "edge": "in", "kind": "fade", "duration": "0.5s"}`.

**Preconditions.**

1. Arguments match the schema; `duration` is not combined with `kind: null` (`E_VALIDATION`); `duration` parses and is at least 1 frame (`E_INVALID_TIME`).
2. The item exists (`E_NOT_FOUND`).
3. The item is `clip`, `text`, `caption` or `shape` (`E_TYPE_MISMATCH`).
4. `transition_in.duration + transition_out.duration <= duration` after the change (R14, `E_INVALID_STATE`).

**Postconditions.**

1. `transition_in` or `transition_out` is `{kind, duration}` (with `color` for a dip that is not black, with `direction` for a whip or a wipe that is not `left`), or removed when `kind` is `null`. The other edge is untouched.
2. `diff` has one `item.updated`; empty when nothing changes (including removing a transition that does not exist). `effective` has `edge`, `kind` and, when set, `duration` as the A.4 triple.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad `kind` or `edge`, `duration` with `kind: null`, `color` on a kind that is not `dip`, `direction` on a kind that is not `whip` or `wipe`, a malformed `color` or `direction` | `args.kind "spin" is not a transition kind.` | `Use fade, slide_left, slide_right, slide_up, slide_down, zoom, whip, dip, wipe or null.` |
| `E_INVALID_TIME` | malformed or zero `duration` | `args.duration "0s" is 0 frames; a transition lasts at least 1 frame.` | `Use "0.5s" or "15f".` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | audio item | `Item itm_s6t7v8w9x0 is an audio item; transitions apply to visual items.` | `Use item.set_audio fade_in and fade_out instead.` |
| `E_INVALID_STATE` | the two transitions would not fit | `transition_in 60 frames plus transition_out 100 frames exceeds duration 150 of itm_m1n2p3q4r5.` | `Use a duration of at most "90f" (3.000s) or shorten transition_out first.` |

**Inverse.** `patch {collection: "items", id, set: {transition_<edge>: <old>}}` or `unset` when there was none.

**Idempotency.** Same result.

**Example.** 30 fps; clip `itm_m1n2p3q4r5` with `duration` 150 and no transitions.

```json
{"op": "item.set_transition", "args": {"item_id": "itm_m1n2p3q4r5", "edge": "in", "kind": "fade"}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"transition_in": [null, {"kind": "fade", "duration": 15}]}}]
```

`effective.duration` is `{"frame": 15, "seconds": 0.5, "requested": "0.5s"}`.

**Golden cases.**

- `ok_set_in_default_duration`
- `ok_set_out_explicit_duration`
- `ok_replace_existing_kind`
- `ok_remove_existing`
- `ok_remove_absent_noop`
- `ok_each_kind`
- `ok_duration_1_frame`
- `ok_sum_equals_item_duration`
- `ok_text_caption_shape_items`
- `err_validation_unknown_kind`
- `err_validation_duration_with_null_kind`
- `err_invalid_time_zero_duration`
- `err_not_found`
- `err_type_mismatch_audio_item`
- `err_invalid_state_sum_exceeds_duration`
- `ok_zoom`
- `ok_spin`
- `ok_shrink`
- `ok_grow`
- `ok_whip_with_direction`
- `ok_whip_default_direction_left_not_stored`
- `ok_dip_with_colour`
- `ok_dip_black_not_stored`
- `ok_wipe_with_direction`
- `ok_replace_colour_dip_with_fade_drops_parameters`
- `err_validation_color_with_fade`
- `err_validation_color_not_6_digit_hex`
- `err_validation_direction_with_fade`
- `err_validation_direction_unknown`

### C.19 `item.set_audio`

Sets volume, fades or the mute flag of a clip or audio item.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip` or `audio` item |
| `volume` | number | no | unchanged | 0 to 4 |
| `fade_in` | time, as a length (may be 0) | no | unchanged | `"0s"` removes the fade |
| `fade_out` | time, as a length (may be 0) | no | unchanged | |
| `audio_muted` | boolean | no | unchanged | `clip` items only |
| `denoise` | number | no | unchanged | 0 to 1; added by TASK-0233; 0 removes the field |
| `voice` | string | no | unchanged | `none`, `clear`, `warm`, `deep`, `bright` or `radio`; added by TASK-0254; `none` removes the field |

At least one of the six is required.

**Preconditions.**

1. At least one argument is present and valid (`E_VALIDATION`, `E_INVALID_TIME`).
2. The item exists (`E_NOT_FOUND`).
3. The item is a `clip` or `audio` item, not a clip of an image (an image has no audio), and `audio_muted` is only given for a `clip` (`E_TYPE_MISMATCH`).
4. When `volume` is given, `volume` has no keyframes (`E_INVALID_STATE`, hint to use `keyframe.set` or `keyframe.clear`). The check does not apply when only fades or `audio_muted` are given.
5. `fade_in + fade_out <= duration` after the change (R14, `E_INVALID_STATE`).

**Postconditions.**

1. Given fields are set; `volume` 1, fades of 0, `audio_muted` false, `denoise` 0 and `voice` `none` remove the field.
2. `diff` has one `item.updated`; empty when nothing changes. `effective` has the stored `volume`, `audio_muted`, `denoise`, `voice` (`none` when there is no preset) and each fade as an A.4 triple.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | nothing to set, `volume` or `denoise` out of range, or a `voice` that is not a preset | `args.volume must be between 0 and 4, got 5.` | `Use volume 4 for the loudest setting.` |
| `E_INVALID_TIME` | malformed fade | `args.fade_in "2" has no unit.` | `Use "2s", "60f" or "00:00:02.000".` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | text, shape or caption item; a clip of an image; or `audio_muted` on an `audio` item | `Item itm_s6t7v8w9x0 is a text item and has no audio.` | `Use item.set_transform for visual properties.` |
| `E_INVALID_STATE` | volume has keyframes; or the fades do not fit | `fade_in 100 frames plus fade_out 60 frames exceeds duration 150 of itm_m1n2p3q4r5.` | `Use fade_in of at most "90f" (3.000s) or shorten fade_out first.` |

**Inverse.** `patch {collection: "items", id, set: <old values>, unset: <fields that were absent>}`.

**Idempotency.** Same result.

**Example.** 30 fps; clip `itm_m1n2p3q4r5`, `duration` 150, default audio.

```json
{"op": "item.set_audio", "args": {"item_id": "itm_m1n2p3q4r5", "volume": 0.8, "fade_in": "1s", "fade_out": "0.5s"}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"volume": [1, 0.8], "fade_in": [null, 30], "fade_out": [null, 15]}}]
```

**Golden cases.**

- `ok_volume_only`
- `ok_volume_0_and_4`
- `ok_fades`
- `ok_fade_0_removes_field`
- `ok_fades_sum_equals_duration`
- `ok_audio_muted_on_clip`
- `ok_audio_item`
- `ok_same_values_noop`
- `ok_fades_only_when_volume_has_keyframes`
- `ok_denoise`
- `ok_denoise_boundaries_0_and_1`
- `ok_denoise_0_removes_field`
- `ok_denoise_on_audio_item`
- `ok_denoise_rounded_to_3_decimals`
- `ok_denoise_when_volume_has_keyframes`
- `err_validation_no_argument`
- `err_validation_volume_4_001`
- `ok_voice`
- `ok_voice_changes_the_preset`
- `ok_voice_none_removes_field`
- `ok_voice_on_audio_item`
- `ok_voice_with_volume_and_denoise`
- `ok_voice_same_value_noop`
- `err_validation_unknown_voice`
- `err_type_mismatch_voice_on_text_item`
- `err_validation_denoise_above_1`
- `err_validation_denoise_negative`
- `err_invalid_time_no_unit`
- `err_not_found`
- `err_type_mismatch_text_item`
- `err_type_mismatch_audio_muted_on_audio_item`
- `err_type_mismatch_image_clip`
- `err_invalid_state_volume_has_keyframes`
- `err_invalid_state_fades_exceed_duration`

### C.20 `item.crossfade`

Sets a matching fade-out and fade-in on two overlapping visual items of different tracks.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `from_item_id` | item id | yes | none | the item that leaves |
| `to_item_id` | item id | yes | none | the item that arrives |
| `duration` | time, as a length | yes | none | at least 1 frame |
| `track_id` | track id | only for neighbours | none | VV-042. For two items that touch on one track (`to.start == from.end`): another visual track, to which `to` moves. It also starts `duration` frames earlier, as `item.trim` with edge `start` does (its `source_in` drops by `duration` at its speed, so what is seen after the cut does not move), and so overlaps `from` by exactly `duration`; the rest is as for two tracks. This needs no model change: no two items of a track overlap. An item on two different tracks refuses `track_id` |

**Preconditions.**

1. Arguments match the schema and `duration` parses to at least 1 frame (`E_VALIDATION`, `E_INVALID_TIME`).
2. Both items exist (`E_NOT_FOUND`).
3. Both are visual items (`E_TYPE_MISMATCH`).
4. They are on different tracks, or touch on one track and `track_id` names another visual track (`E_INVALID_STATE` when the items do not touch, when `to` would start before frame 0 or when `track_id` is missing; `E_VALIDATION` when `track_id` is given for items of two tracks; `E_TYPE_MISMATCH` for the same or an audio track; `E_NOT_FOUND`). With `track_id`, `to` must have `duration` frames of source before the cut unless it has no source range (`E_OUT_OF_SOURCE`), and the new range must be free on that track (`E_OVERLAP`).
5. `from.start < to.start`, and the overlap `min(from.end, to.end) - to.start` is at least `duration`, where `end` is `start + duration` (`E_INVALID_STATE`, `details.overlap`). An item that starts at or before the start of `from`, or a `to` that ends inside the fade, fails here.
6. After the change both items satisfy R14: for `from`, `duration + transition_in.duration <= from.duration`, and for a `clip` also `fade_in + duration <= from.duration`; for `to`, `duration + transition_out.duration <= to.duration`, and for a `clip` also `duration + fade_out <= to.duration` (`E_INVALID_STATE`).

**Postconditions.**

1. `from.transition_out = {fade, duration}` and `to.transition_in = {fade, duration}`, replacing any existing ones.
2. For a `clip`, `from.fade_out = duration` and `to.fade_in = duration`. Other visual types have no audio fades.
3. Positions, tracks and every other field are unchanged. The fade covers the last `duration` frames of `from` and the first `duration` frames of `to`; the two coincide exactly when the overlap equals `duration`.
4. `diff` has one `item.updated` per item. `effective` has `duration` (the A.4 triple) and `overlap` (frames); with `track_id` also `to_track_id` and `to_start`, where `to` now starts (the `diff` of `to` then has `track`, `start`, `duration` and `source_in` as well).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | missing argument | `args.duration is required.` | `Send for example "duration": "1s".` |
| `E_INVALID_TIME` | malformed or zero `duration` | `args.duration "0f" is 0 frames.` | `Use "1s" or "30f".` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the ids of items listed by project_get.` |
| `E_TYPE_MISMATCH` | an audio item, or `track_id` that is the track of `from` or an audio track | `Item itm_s6t7v8w9x0 is an audio item; crossfade needs visual items.` | `Use item.set_audio fades for audio items.` |
| `E_INVALID_STATE` | same track without `track_id` (or not touching, or no room before frame 0), `to` not starting after `from`, too little overlap, or no room | `Items overlap by 20 frames (120-139); a 30-frame crossfade needs 30.` | `Move itm_s6t7v8w9x0 to start at "110f" or earlier with item.move (from ends at 140), or use duration "20f".` |
| `E_OUT_OF_SOURCE` | with `track_id`, `to` has less source before the cut than `duration` | `Moving the start of itm_s6t7v8w9x0 from frame 150 to 120 needs source frame -20 at speed 1/1 (source_in 10); the asset begins at 0.` | `Use a shorter duration (itm_s6t7v8w9x0 has less source before the cut), or move it to another track with item.move first.` |
| `E_OVERLAP` | with `track_id`, the new range of `to` is not free on that track | `Item itm_s6t7v8w9x0 would occupy frames 120-299 on track trk_a1b2c3d4e5, where item itm_c3d4e5f6g7 already is (frames 100-159).` | `Use start "160f" (5.333s), the next free position on that track, or move to another track.` |

**Inverse.** One `patch {collection: "items", id, set: <old values>, unset: <fields that were absent>}` per item, `from` first.

**Idempotency.** Same result.

**Example.** 30 fps; clip `itm_m1n2p3q4r5` on `trk_f6g7h8j9k0` (start 0, duration 150) and clip `itm_s6t7v8w9x0` on `trk_m1n2p3q4r5` (start 120, duration 150); overlap is 30 frames.

```json
{"op": "item.crossfade", "args": {"from_item_id": "itm_m1n2p3q4r5", "to_item_id": "itm_s6t7v8w9x0", "duration": "1s"}}
```

```json
[
  {"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"transition_out": [null, {"kind": "fade", "duration": 30}], "fade_out": [null, 30]}},
  {"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"transition_in": [null, {"kind": "fade", "duration": 30}], "fade_in": [null, 30]}}
]
```

**Golden cases.**

- `ok_overlap_equals_duration`
- `ok_overlap_larger_than_duration`
- `ok_replaces_existing_transitions_and_fades`
- `ok_text_items_no_audio_fades`
- `ok_to_shorter_than_from_overlap_equals_to_duration`
- `ok_same_result_twice_noop`
- `err_validation_missing_duration`
- `err_invalid_time_zero_duration`
- `err_not_found_from`
- `err_not_found_to`
- `err_type_mismatch_audio_item`
- `err_invalid_state_same_track`
- `err_invalid_state_overlap_too_small`
- `err_invalid_state_from_starts_after_to_starts`
- `err_invalid_state_from_and_to_start_together`
- `err_invalid_state_to_ends_inside_the_fade`
- `err_invalid_state_to_ends_before_from_starts`
- `err_invalid_state_existing_transition_leaves_no_room`
- `ok_neighbours_use_the_source_before_the_cut`
- `ok_neighbours_image_clip_has_no_source_to_check`
- `ok_neighbours_text_items`
- `ok_neighbours_whole_source_handle_used`
- `err_invalid_state_neighbours_do_not_touch`
- `err_invalid_state_neighbours_start_before_frame_0`
- `err_out_of_source_neighbour_has_no_handle`
- `err_overlap_target_track_is_taken`
- `err_type_mismatch_target_is_an_audio_track`
- `err_type_mismatch_target_is_the_same_track`
- `err_not_found_target_track`
- `err_validation_track_id_with_items_on_two_tracks`

### C.21 `timeline.close_gap`

Closes the empty range that contains a given frame on a track.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `track_id` | track id | yes | none | |
| `at` | time | yes | none | absolute frame inside the gap |
| `ripple` | `"track"` or `"all"` | no | `"track"` | `"all"` moves later items of every track |

**Preconditions.**

1. Arguments match the schema and `at` parses (`E_VALIDATION`, `E_INVALID_TIME`).
2. The track exists (`E_NOT_FOUND`).
3. No item of the track contains frame `at`, and at least one item starts after `at` (`E_INVALID_STATE`).
4. After the moves no two items overlap (`E_OVERLAP`; possible only with `"all"`).

**Postconditions.**

1. The gap is `[g0, g1)`: `g0` is the end of the last item that ends at or before `at` (0 when there is none) and `g1` is the start of the first item that starts after `at`. Its length is `L = g1 - g0`.
2. `"track"`: every item of the track with `start >= g1` moves earlier by `L`. `"all"`: every item of every track with `start >= g1` moves earlier by `L`, and so do the timeline markers: one inside `[g0, g1)` is removed and one at or after `g1` moves earlier by `L` (VV-201; `effective.markers_removed` and `markers_moved` when any is touched). `"track"` leaves the markers.
3. `diff`: one `item.updated` (`start`) per moved item, sorted by id. `effective` has `at` (the A.4 triple), `gap` (`{"start", "end", "length"}`) and `moved` (`[{"id", "from", "to"}]`). In the example, `effective.at` is `{"frame": 70, "seconds": 2.333, "requested": "70f"}`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad `ripple` or missing argument | `args.ripple must be "track" or "all", got "none".` | `Use "track" or "all".` |
| `E_INVALID_TIME` | malformed `at` | `args.at "70" has no unit.` | `Use "70f" or "2.333s".` |
| `E_NOT_FOUND` | unknown track | `Track trk_zzzzzzzzzz does not exist.` | `Use the id of a track listed by project_get.` |
| `E_INVALID_STATE` | `at` inside an item, or no item after it | `Frame 40 is inside item itm_m1n2p3q4r5 (frames 0-59); there is no gap there.` | `Use an at between "60f" and "99f", the gap after that item.` |
| `E_OVERLAP` | a shift on another track overlaps | `Item itm_p7q8r9s0t1 would occupy frames 60-89 on track trk_m1n2p3q4r5, where item itm_c3d4e5f6g7 already is (frames 30-79).` | `Use ripple "track" to move only this track.` |

**Inverse.** `patch {collection: "items", id, set: {start: <old>}}` per moved item, in the order of mutation (R5). Undo applies them to a working copy and validates once at the end (A.6).

**Idempotency.** Error in practice: after closing, frame `at` is inside the item that moved there (`E_INVALID_STATE`), unless it now falls in another gap, which is then closed.

**Example.** One track `trk_f6g7h8j9k0`: items `itm_a1b2c3d4e5` (start 0, duration 60), `itm_f6g7h8j9k0` (start 100, duration 30), `itm_p7q8r9s0t1` (start 150, duration 20). The gap is `[60, 100)`, `L = 40`.

```json
{"op": "timeline.close_gap", "args": {"track_id": "trk_f6g7h8j9k0", "at": "70f"}}
```

```json
[
  {"kind": "item.updated", "id": "itm_f6g7h8j9k0", "changes": {"start": [100, 60]}},
  {"kind": "item.updated", "id": "itm_p7q8r9s0t1", "changes": {"start": [150, 110]}}
]
```

**Golden cases.**

- `ok_gap_between_two_items`
- `ok_gap_at_start_of_track`
- `ok_gap_first_frame`
- `ok_gap_last_frame`
- `ok_ripple_all_moves_other_tracks`
- `ok_ripple_all_moves_and_drops_timeline_markers`
- `ok_ripple_track_leaves_timeline_markers`
- `ok_ripple_track_leaves_other_tracks`
- `err_validation_bad_ripple`
- `err_invalid_time_no_unit`
- `err_not_found_track`
- `err_invalid_state_at_inside_item`
- `err_invalid_state_at_after_last_item`
- `err_invalid_state_item_touching_no_gap`
- `err_overlap_ripple_all_other_track`

### C.22 `clip.add`

Places a video or image asset on a visual track. An animated image (RF-028) is an image asset with `frame_delays` (SDD-001 section 3.2): it follows every image rule of this section, so the default duration is `"5s"`, `source_in` must be omitted and the animation loops for as long as the item lasts (SDD-004 section 3.3); there is no hold mode and the command has no argument for it. A **sticker** is not a type: it is an image or animated image placed with this command on a visual track above the picture (a smaller `order`), usually with `item.set_transform` to size and place it; the alpha channel of the image is kept by the compositor.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `track_id` | track id | yes | none | a visual track |
| `asset_id` | asset id | yes | none | an asset of kind `video` or `image`, or the reserved literal `ast_placeholder` (R6): the item is created with the placeholder, the asset checks of preconditions 2, 3 and 5 are skipped, and `duration` is then required (`E_VALIDATION`) |
| `start` | time | no | the end of the last item of the track (`"0s"` on an empty track) | absolute (VV-158). Omitted, the item goes to the first free position at or after the end of the track, so it never overlaps |
| `source_in` | time | no | `"0s"` | offset into the asset; for an image it must be omitted or 0 (`E_VALIDATION`) |
| `duration` | time, as a length | no | rest of the asset from `source_in`; `"5s"` for an image | at least 1 frame |
| `fit` | `"cover"`, `"contain"`, `"none"` | no | `"cover"`; for an image on a track with a visual track below it (a larger `order`): `"none"` when the image fits the frame, `"contain"` when it does not | a logo or sticker enters at its natural size instead of filling the frame (VV-039) |
| `id` | item id | no | generated | R6 |

**Preconditions.**

1. Arguments match the schema and times parse (`E_VALIDATION`, `E_INVALID_TIME`).
2. The track and the asset exist (`E_NOT_FOUND`).
3. The track is visual and the asset is `video` or `image` (`E_TYPE_MISMATCH`).
4. `id` is not used (`E_ID_EXISTS`); the document has fewer than 500 items (`E_LIMIT`).
5. For a video: `source_in < A` and the duration obeys R9 with speed 1 (`E_OUT_OF_SOURCE`).
6. The item ends at or before 108,000 (`E_INVALID_TIME`) and does not overlap an item of the track (`E_OVERLAP`, R8).

**Postconditions.**

1. A `clip` item is added: `start`, `track`, `asset`, `duration`, plus `source_in` when above 0 and `fit` when not `"cover"`. Speed is 1 and every other field is absent (default).
2. The default `duration` for a video is `A - source_in`; for an image it is the frames of `"5s"`. A default that would end after frame 108,000 is clamped to `108000 - start` (only an explicit duration is an error).
3. `diff` has one `item.added`. `effective` has `id`, `start`, `source_in` and `duration` (A.4 triples; `requested` is `null` for a default).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad argument, or `source_in` on an image | `args.source_in must be omitted for image asset ast_9m8n7p6q5r.` | `Remove source_in; images have no source position.` |
| `E_INVALID_TIME` | malformed time, zero duration, item ending after 108,000, or `start` at or after 108,000 | `The clip would end at frame 108100; the limit is 108000.` | `Use a duration of at most "900f" from this start.` |
| `E_NOT_FOUND` | unknown track or asset | `Asset ast_zzzzzzzzzz does not exist.` | `Use the id of an asset listed by project_get.` |
| `E_TYPE_MISMATCH` | audio track, or an audio asset | `Asset ast_4h6j8k2m3n is an audio asset; clip.add places video or images.` | `Use audio.add on an audio track.` |
| `E_ID_EXISTS` | `id` used | `Item id itm_m1n2p3q4r5 already exists.` | `Omit id or use an unused id.` |
| `E_LIMIT` | 500 items | `The document already has 500 items; the limit is 500.` | `Delete unused items first.` |
| `E_OUT_OF_SOURCE` | the range leaves the asset | `Clip of 900 frames from source frame 60 would read to frame 960; asset ast_3c5v8n2w6d has 900 frames.` | `Use duration "840f" (28.000s), the most this asset allows from source_in 60.` |
| `E_OVERLAP` | overlap on the track | `Item itm_m1n2p3q4r5 would occupy frames 100-249 on track trk_f6g7h8j9k0, where item itm_c3d4e5f6g7 already is (frames 90-179).` | `Use start "180f" (6.000s), the next free position, omit start to append at the end of the track, or move to another track.` |

**Inverse.** `remove {collection: "items", id: <new id>}`.

**Idempotency.** With `id`: the second application returns `E_ID_EXISTS`. Without: the copy overlaps the first (`E_OVERLAP`).

**Example.** 30 fps; empty track `trk_f6g7h8j9k0`; asset `ast_3c5v8n2w6d` (video, 900 frames).

```json
{"op": "clip.add", "args": {"track_id": "trk_f6g7h8j9k0", "asset_id": "ast_3c5v8n2w6d", "start": "0s", "source_in": "2s", "duration": "5s", "id": "itm_m1n2p3q4r5"}}
```

```json
[{"kind": "item.added", "id": "itm_m1n2p3q4r5", "object": {"id": "itm_m1n2p3q4r5", "type": "clip", "track": "trk_f6g7h8j9k0", "start": 0, "duration": 150, "asset": "ast_3c5v8n2w6d", "source_in": 60}}]
```

**Golden cases.**

- `ok_video_default_duration_rest_of_asset`
- `ok_video_explicit_duration`
- `ok_video_source_in`
- `ok_video_ends_exactly_at_asset_end`
- `ok_image_default_5s`
- `ok_image_explicit_duration`
- `ok_animated_image_default_5s_loops`
- `ok_animated_image_explicit_duration`
- `ok_sticker_on_an_upper_track`
- `ok_image_on_an_upper_track_keeps_natural_size`
- `ok_image_on_an_upper_track_larger_than_frame_is_contained`
- `ok_image_on_an_upper_track_explicit_fit`
- `err_validation_source_in_on_animated_image`
- `ok_fit_contain`
- `ok_fit_none`
- `ok_start_omitted_goes_to_the_end_of_the_track`
- `ok_start_omitted_on_empty_track_is_0`
- `ok_start_frame_0_on_empty_track`
- `ok_touching_neighbour`
- `ok_ends_exactly_at_108000`
- `ok_default_duration_clamped_to_108000`
- `ok_supplied_id`
- `ok_placeholder_asset_with_duration`
- `err_validation_placeholder_asset_without_duration`
- `err_validation_source_in_on_image`
- `err_validation_bad_fit`
- `err_invalid_time_malformed`
- `err_invalid_time_zero_duration`
- `err_invalid_time_ends_after_108000`
- `err_not_found_track`
- `err_not_found_asset`
- `err_type_mismatch_audio_track`
- `err_type_mismatch_audio_asset`
- `err_id_exists`
- `err_limit_500_items`
- `err_out_of_source_one_frame_too_long`
- `err_out_of_source_source_in_at_asset_end`
- `err_overlap`
- `err_type_mismatch_lut_asset`

### C.23 `audio.add`

Places an audio asset, or the audio of a video asset, on an audio track.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `track_id` | track id | yes | none | an audio track |
| `asset_id` | asset id | yes | none | kind `audio`, or kind `video` with `has_audio` true, or the reserved literal `ast_placeholder` (R6, as in C.22: the asset checks are skipped and `duration` is required) |
| `start` | time | no | the end of the last item of the track (`"0s"` on an empty track) | absolute (VV-158). Omitted, the item goes to the first free position at or after the end of the track, so it never overlaps |
| `source_in` | time | no | `"0s"` | offset into the asset |
| `duration` | time, as a length | no | rest of the asset from `source_in` | at least 1 frame |
| `id` | item id | no | generated | R6 |

**Preconditions.** As C.22, with these differences: step 3 requires an audio track and an asset that is `audio` or a `video` with `has_audio` (`E_TYPE_MISMATCH`); there is no image case, so `source_in < A` always applies; there is no `fit`.

1. Arguments match the schema and times parse (`E_VALIDATION`, `E_INVALID_TIME`).
2. The track and asset exist (`E_NOT_FOUND`).
3. The track is audio and the asset has audio (`E_TYPE_MISMATCH`).
4. `id` is not used (`E_ID_EXISTS`); fewer than 500 items (`E_LIMIT`).
5. `source_in < A` and the duration obeys R9 (`E_OUT_OF_SOURCE`).
6. The item ends at or before 108,000 (`E_INVALID_TIME`) and does not overlap (`E_OVERLAP`).

**Postconditions.**

1. An `audio` item is added: `start`, `track`, `asset`, `duration`, plus `source_in` when above 0. `A` for an audio asset is `floor(sample_count × fps.num / (48000 × fps.den))`. The default duration is `A - source_in`, clamped to `108000 - start`.
2. `diff` has one `item.added`. `effective` as in C.22.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad argument | `args.asset_id "ast_" is not an asset id: an id is "ast_" followed by 10 characters from 0-9 and a-z without i, l, o and u.` | `Use an id like "ast_4h6j8k2m3n".` |
| `E_INVALID_TIME` | as C.22 | `The audio item would end at frame 108400; the limit is 108000.` | `Use a duration of at most "500f".` |
| `E_NOT_FOUND` | unknown track or asset | `Track trk_zzzzzzzzzz does not exist.` | `Use the id of a track listed by project_get.` |
| `E_TYPE_MISMATCH` | visual track, image asset, or video without audio | `Asset ast_3c5v8n2w6d is a video without audio; audio.add needs audio.` | `Use clip.add on a visual track to show it.` |
| `E_ID_EXISTS` | `id` used | `Item id itm_s6t7v8w9x0 already exists.` | `Omit id or use an unused id.` |
| `E_LIMIT` | 500 items | `The document already has 500 items; the limit is 500.` | `Delete unused items first.` |
| `E_OUT_OF_SOURCE` | the range leaves the asset | `Audio of 900 frames from source frame 30 would read to frame 930; asset ast_4h6j8k2m3n has 900 frames.` | `Use duration "870f" (29.000s).` |
| `E_OVERLAP` | overlap on the track | `Item itm_s6t7v8w9x0 would occupy frames 30-929 on track trk_m1n2p3q4r5, where item itm_c3d4e5f6g7 already is (frames 0-99).` | `Use start "100f" (3.333s), the next free position, omit start to append at the end of the track, or move to another track.` |

**Inverse.** `remove {collection: "items", id: <new id>}`.

**Idempotency.** As C.22.

**Example.** 30 fps; empty audio track `trk_m1n2p3q4r5`; asset `ast_4h6j8k2m3n` (audio, 1,440,000 samples, so `A` = 900).

```json
{"op": "audio.add", "args": {"track_id": "trk_m1n2p3q4r5", "asset_id": "ast_4h6j8k2m3n", "start": "1s", "id": "itm_s6t7v8w9x0"}}
```

```json
[{"kind": "item.added", "id": "itm_s6t7v8w9x0", "object": {"id": "itm_s6t7v8w9x0", "type": "audio", "track": "trk_m1n2p3q4r5", "start": 30, "duration": 900, "asset": "ast_4h6j8k2m3n"}}]
```

**Golden cases.**

- `ok_audio_asset_default_duration`
- `ok_start_omitted_goes_to_the_end_of_the_track`
- `ok_audio_asset_explicit_duration_and_source_in`
- `ok_video_with_audio_asset`
- `ok_ends_exactly_at_asset_end`
- `ok_supplied_id`
- `ok_placeholder_asset_with_duration`
- `err_validation_bad_asset_id`
- `err_invalid_time_ends_after_108000`
- `err_not_found_track`
- `err_not_found_asset`
- `err_type_mismatch_visual_track`
- `err_type_mismatch_image_asset`
- `err_type_mismatch_video_without_audio`
- `err_id_exists`
- `err_limit_500_items`
- `err_out_of_source`
- `err_overlap`
- `err_type_mismatch_lut_asset`

### C.24 `clip.set_framing`

Sets how a clip's asset is fitted into the frame and how much of each side is cropped.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip` |
| `fit` | `"cover"`, `"contain"`, `"none"` | no | unchanged | |
| `crop` | object with any of `left`, `top`, `right`, `bottom` | no | unchanged | each 0 to 1; sides not given keep their current value; the merged result needs `left + right < 1` and `top + bottom < 1`; a side that has keyframes (`crop_left`, `crop_top`, `crop_right`, `crop_bottom`, VV-120) cannot be given |

At least one of `fit` and `crop` is required.

**Preconditions.**

1. At least one argument is present and valid; the merged crop obeys the sum rules (`E_VALIDATION`, `path` `args.crop`).
2. The item exists (`E_NOT_FOUND`).
3. The item is a `clip` (`E_TYPE_MISMATCH`).

**Postconditions.**

1. `fit` is set (`"cover"` removes the field). `crop` is the merged object holding only the sides above 0; an all-zero crop removes the field.
2. `diff` has one `item.updated`; empty when unchanged. `effective` has `fit` and `crop` as stored.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | nothing to set, a side out of range, or sides summing to 1 or more | `args.crop: left 0.6 plus right 0.5 is 1.1; it must be below 1.` | `Use left 0.4 and right 0.5.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not a clip | `Item itm_s6t7v8w9x0 is a text item; framing applies to clips.` | `Use item.set_transform to size other items.` |
| `E_INVALID_STATE` | a given crop side has keyframes (VV-120) | `Property crop_left of itm_s6t7v8w9x0 has 2 keyframes, which define its value.` | `Use keyframe.set to change a keyframe, or keyframe.clear to remove them first.` |

**Inverse.** `patch {collection: "items", id, set: {fit, crop: <old>}, unset: <fields that were absent>}`.

**Idempotency.** Same result.

**Example.** Clip `itm_m1n2p3q4r5` with default framing.

```json
{"op": "clip.set_framing", "args": {"item_id": "itm_m1n2p3q4r5", "fit": "contain", "crop": {"left": 0.1, "right": 0.1}}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"fit": ["cover", "contain"], "crop": [null, {"left": 0.1, "right": 0.1}]}}]
```

**Golden cases.**

- `ok_fit_only`
- `ok_crop_only`
- `ok_crop_merges_with_existing_sides`
- `ok_crop_zero_removes_field`
- `ok_fit_cover_removes_field`
- `ok_crop_sides_sum_just_below_1`
- `ok_image_clip`
- `ok_same_values_noop`
- `err_validation_no_argument`
- `err_validation_side_above_1`
- `err_validation_horizontal_sum_1`
- `err_validation_merged_vertical_sum_1`
- `err_not_found`
- `err_type_mismatch_text_item`
- `err_invalid_state_crop_side_has_keyframes`
- `ok_other_crop_side_is_free`

### C.25 `clip.set_speed`

Changes the speed of a clip or audio item, constant or along a curve (TASK-0275), while keeping the same source range.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip` of a video asset, or an `audio` item |
| `speed` | string or rational object | one of `speed`, `curve`, `reverse` | none | `"<decimal>x"` with at most 3 decimals (`"1.5x"` is 3/2, `"0.5x"` is 1/2, `"0.333x"` is 333/1000), or `{"num","den"}`; stored reduced to lowest terms; from 1/4 to 4 inclusive. Clears the curve |
| `reverse` | boolean | one of `speed`, `curve`, `reverse` | none | `true` plays the same source range backwards, `false` plays it forwards (the field is then absent). Alone it changes nothing else: the duration, the speed and the curve stay |
| `curve` | array of `{"t", "speed"}` | one of `speed`, `curve` | none | 2 to 8 points; `t` is the position inside the item as a fraction of its duration, a multiple of 0.001, strictly increasing, the first 0 and the last 1; `speed` is a string `"<decimal>x"` of at most 3 decimals (or a `{"num","den"}` whose `den` divides 1000), from 1/4 to 4 inclusive. The speed ramps linearly between points |
| `ripple` | `"none"` or `"track"` | no | `"none"` | `"track"` shifts later items of the track |

**Preconditions.**

1. Arguments match the schema; at least one of `speed`, `curve` and `reverse` is given and not both `speed` and `curve`; `speed` parses and is within 1/4 to 4; `curve` has 2 to 8 points that meet the rules above (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).
3. The item is a `clip` whose asset is a video, or an `audio` item, and is not a freeze frame (`E_TYPE_MISMATCH`; a freeze frame has no speed, TASK-0254).
4. At least one frame fits in the source range at the new speed: `maxdur(source_in)` at the new speed is at least 1 (`E_OUT_OF_SOURCE`). A new duration that fails R9 only because of rounding is not an error: it is reduced (postcondition 1).
5. Every item ends at or before 108,000 after the change (`E_INVALID_TIME`).
6. Without ripple, a longer item does not overlap its neighbour (`E_OVERLAP`, R8).

**Postconditions.**

1. Let `old` and `new` be the speeds and `d` the duration. For a curve, `new` is its **mean speed** `M = sum over the segments of (t[i+1] - t[i]) x (s[i] + s[i+1]) / 2` (SDD-004 section 3.11), an exact rational in lowest terms, and `old` of an item that has a curve is its stored mean; so the formulas below are the same for a constant speed and for a curve, and the source range an item reads, `d x M`, is what stays unchanged. The new duration is `d' = ceil(d × old.num × new.den / (old.den × new.num))`. If `source_in + ceil(d' × new.num / new.den)` would pass `A`, `d'` becomes `maxdur(source_in)` computed at the new speed, and `effective.clamped` is `{"duration": {"from": <d'>, "to": <maxdur>}}` (it is `null` otherwise); if `maxdur(source_in)` is below 1, no frame fits and the result is `E_OUT_OF_SOURCE`.
2. Let `r = old / new` as a rational. `transition_in.duration`, `transition_out.duration`, `fade_in`, `fade_out` and every keyframe `t` become `round(x × r)` (halves up). Then, in order: transitions below 1 become 1; fades may become 0 and are then removed; keyframe `t` above `d'` becomes `d'`; keyframes sharing a `t` keep the last; the fit fix of R14. Each fix-up is listed in `effective.adjusted` as in C.3.
3. `speed` is the new value (for a curve, the mean `M`), absent when it is 1/1; with only `reverse` given, `speed` and `speed_curve` stay and the duration does not change. `reverse` is the given value, absent when false. `speed_curve` is the given points with `t` rounded to 3 decimals and the speeds in lowest terms when `curve` was given, and is removed when `speed` was given. `source_in` and the source range are unchanged (up to the ceiling). Keyframes, transitions and fades are in frames of the item, not of the source, so a curve does not change what they mean; they only follow the rescale of postcondition 2.
4. `ripple: "track"`: every item of the track with `start >= old end` moves by `d' - d` (earlier when the clip got shorter, later when longer). Otherwise nothing else moves.
5. Captions whose `linked_item` is this item are not changed; `effective.linked_captions` lists their ids so the caller can run `caption.retime_from_clip`.
6. `diff`: `item.updated` for the clip, then for every shifted item, sorted by id. `effective` has `speed` (`{num, den}`), `curve_points` (only when a curve was set), `duration` (`{"before", "after"}`), `result_duration` (the new duration, VV-163), `shift`, the lists above and, when `ripple` is not `"track"` and the item got shorter with a later item on its track, `gap` (`{"start", "frames", "hint"}`: the gap left before that item, with the hint to use `ripple: "track"`). The linter reports such a gap as `L_BLACK_FRAMES` (or `L_GAP` under text).
7. **What a curve forbids.** An item with a `speed_curve` has no constant speed, so a command that turns a frame of the timeline into a frame of the source inside it returns `E_INVALID_STATE` with the hint to remove the curve first: `item.split`, `item.trim` of the start edge (and the neighbour slide of `item.move` that does it), `clip.remove_silences`, `caption.from_transcript`, `caption.retime_from_clip`, `audio.mark_beats` and `audio.duck` with the item among the speech items. A reversed item (postcondition 8) also gets `E_INVALID_STATE` from the last five of these, which read the source forwards; it can be split and trimmed. `item.trim` of the end edge and every other command keep the curve: its points are fractions of the duration, so the curve is stretched over the new duration and the item reads `d x M` frames of the new duration. Commands that only need the source range (R9, `slot.fill`, the fit fix) use `M`, which is exact for the whole range.
8. **Reverse.** A reversed item reads the same source range `[source_in, source_in + C)`, `C = ceil(d x M)`, backwards (SDD-004 section 3.12). So `item.trim` of the start edge keeps `source_in` (the range loses or gains frames at its end, and a start that would read past the asset is `E_OUT_OF_SOURCE`), `item.trim` of the end edge sets `source_in' = source_in + C - C'` with `C'` the range of the new duration (below 0 is `E_OUT_OF_SOURCE`), and `item.split` at `tc` gives the left part `source_in + C - ceil(tc x M)` and the right part the original `source_in`. A freeze frame is `item.freeze_frame` (C.70).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | unparsable speed, or outside 1/4 to 4; both or neither of `speed` and `curve`; a curve that is not valid | `args.speed "5x" is above the maximum 4x.` | `Use a speed from "0.25x" to "4x".` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | text, shape, caption or image clip | `Item itm_s6t7v8w9x0 is a text item and has no speed.` | `Change the duration with item.trim instead.` |
| `E_OUT_OF_SOURCE` | not even one frame fits at the asset end | `At speed 4x the clip needs 4 source frames from frame 898; asset ast_3c5v8n2w6d has 900.` | `Use item.trim to move source_in earlier, or choose a lower speed.` |
| `E_INVALID_TIME` | an item would end after 108,000 | `Item itm_s6t7v8w9x0 would end at frame 108040; the limit is 108000.` | `Use a higher speed, or shorten the clip first.` |
| `E_OVERLAP` | longer item overlaps | `Item itm_m1n2p3q4r5 would occupy frames 30-209 on track trk_f6g7h8j9k0, where item itm_s6t7v8w9x0 already is (frames 120-149).` | `Use ripple "track" to shift later items, or move itm_s6t7v8w9x0 to "210f".` |

**Inverse.** `patch {collection: "items", id, set: <old values of every changed field>, unset: <fields that were absent>}` for the clip, then `patch {... set: {start: <old>}}` per shifted item, in the order of mutation (R5).

**Idempotency.** Same result: the second application finds the speed already set (the new duration equals the current one).

**Example.** 30 fps; asset `ast_3c5v8n2w6d` has 900 frames. Clip `itm_m1n2p3q4r5` on `trk_f6g7h8j9k0`: `start` 30, `duration` 90, `source_in` 0, speed 1, `fade_in` 10, `fade_out` 8, `transition_in` fade 15, `keyframes.scale` = `[{t:0, value:1}, {t:7, value:1.2}, {t:90, value:1}]`. Text item `itm_s6t7v8w9x0` on the same track: `start` 120. New speed `"1.5x"` with `ripple: "track"`: `r = 1/(3/2) = 2/3`.

| Quantity | Before | After | Why |
| --- | --- | --- | --- |
| `duration` | 90 | 60 | ceil(90 × 2/3) = 60 |
| `fade_in` | 10 | 7 | round(6.667) |
| `fade_out` | 8 | 5 | round(5.333) |
| `transition_in.duration` | 15 | 10 | round(10.0) |
| keyframe `t` | 0, 7, 90 | 0, 5, 60 | round(0), round(4.667), round(60.0) |
| text `start` | 120 | 90 | shift by 60 - 90 = -30 |
| source range | 90 frames | ceil(60 × 3/2) = 90 | unchanged |

```json
{"op": "clip.set_speed", "args": {"item_id": "itm_m1n2p3q4r5", "speed": "1.5x", "ripple": "track"}}
```

```json
[
  {"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {
    "duration": [90, 60], "speed": [{"num": 1, "den": 1}, {"num": 3, "den": 2}],
    "fade_in": [10, 7], "fade_out": [8, 5],
    "transition_in": [{"kind": "fade", "duration": 15}, {"kind": "fade", "duration": 10}],
    "keyframes": [{"scale": [{"t": 0, "value": 1}, {"t": 7, "value": 1.2}, {"t": 90, "value": 1}]}, {"scale": [{"t": 0, "value": 1}, {"t": 5, "value": 1.2}, {"t": 60, "value": 1}]}]}},
  {"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"start": [120, 90]}}
]
```

**Golden cases.**

- `ok_speed_up_string_form`
- `ok_slow_down_string_form`
- `ok_object_form_reduced`
- `ok_back_to_speed_1_removes_field`
- `ok_ripple_track_shortening`
- `ok_ripple_track_lengthening`
- `ok_ripple_none_leaves_gap`
- `ok_duration_ceiling`
- `ok_keyframes_rescaled_and_collision_reduced`
- `ok_fade_becomes_zero_removed`
- `ok_transition_min_1_and_fit_fix`
- `ok_clamped_at_asset_end`
- `ok_audio_item`
- `ok_boundary_speed_quarter_and_4`
- `ok_linked_captions_listed_not_changed`
- `ok_same_speed_noop`
- `err_validation_speed_above_4`
- `err_validation_speed_below_quarter`
- `err_validation_speed_without_x`
- `err_not_found`
- `err_type_mismatch_text_item`
- `err_type_mismatch_image_clip`
- `err_out_of_source_no_frame_fits`
- `err_invalid_time_ripple_pushes_item_past_108000`
- `err_overlap_lengthening_without_ripple`
- `ok_markers_rescaled_and_collisions_dropped`
- `ok_curve_ramp_up_and_down`
- `ok_curve_two_points_is_a_ramp`
- `ok_curve_eight_points`
- `ok_curve_ripple_track_with_keyframes_and_transition`
- `ok_curve_over_constant_speed`
- `ok_curve_replaces_curve`
- `ok_speed_clears_curve`
- `ok_curve_clamped_at_asset_end`
- `ok_curve_audio_item`
- `err_validation_curve_one_point`
- `err_validation_curve_nine_points`
- `err_validation_curve_first_t_not_0`
- `err_validation_curve_last_t_not_1`
- `err_validation_curve_t_not_increasing`
- `err_validation_curve_t_4_decimals`
- `err_validation_curve_speed_above_4`
- `err_validation_curve_speed_rational_not_thousandths`
- `err_validation_speed_and_curve`
- `err_validation_no_speed_no_curve`
- `err_type_mismatch_curve_on_image_clip`
- `err_out_of_source_curve_no_frame_fits`
- `ok_reverse_alone`
- `ok_reverse_with_speed`
- `ok_reverse_with_curve`
- `ok_reverse_false_removes_field`
- `ok_speed_keeps_reverse`
- `ok_reverse_keeps_curve`
- `ok_reverse_audio_item`
- `ok_reverse_same_value_noop`
- `err_validation_reverse_not_boolean`
- `err_type_mismatch_reverse_on_image_clip`
- `err_out_of_source_reverse_speed_no_frame_fits`
- `err_type_mismatch_freeze_frame`

### C.26 `clip.set_color`

Sets the colour adjustments of a clip, the lookup table (LUT) that grades it and its advanced colour: HSL bands, wheels and curves (SDD-004 §3.14).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip` |
| `brightness` | number | no | unchanged | -1 to 1 |
| `contrast` | number | no | unchanged | 0 to 2 |
| `saturation` | number | no | unchanged | 0 to 2 |
| `temperature` | number | no | unchanged | -1 to 1; added by TASK-0233; positive is warmer, negative cooler |
| `tint` | number | no | unchanged | -1 to 1; positive is more magenta, negative greener |
| `hue` | number | no | unchanged | -180 to 180 degrees; rotates the colours about the grey axis |
| `lut_asset` | asset id | no | unchanged | added by TASK-0254; a `lut` asset of the project; with no current table the intensity is 1, and giving another asset than the current one resets it to 1 |
| `lut_intensity` | number | no | unchanged (1 for a new table) | 0 to 1, rounded to 3 decimals; 0 removes the table; given alone it needs a clip that already has a table |
| `hsl` | list | no | unchanged | added by TASK-0254 and NEW-09b; up to 8 `{range, hue?, saturation?, lightness?}` with `range` one of `red`, `orange`, `yellow`, `green`, `cyan`, `blue`, `purple`, `magenta` (once each) and the three values -1 to 1 (default 0; `hue` is a shift of up to 30 degrees each way). A given range replaces the stored band of that range; a band with all three at 0 removes it |
| `wheels` | object | no | unchanged | `shadows`, `midtones`, `highlights`, each `{hue?, amount?, luma?}` with `hue` 0 to 360 degrees (0 red, 120 green, 240 blue), `amount` 0 to 1 and `luma` -1 to 1 (default 0). A given wheel replaces the stored one; `amount` 0 and `luma` 0 removes it |
| `curves` | object | no | unchanged | `master`, `red`, `green`, `blue`, each a list of 2 to 8 `{x, y}` (0 to 1, rounded to 3 decimals) with `x` strictly increasing. A given channel replaces the stored curve; `[]` or the line (0,0)-(1,1) removes it |

At least one of the eleven is required.

**Preconditions.**

1. At least one is present and in range; the `hsl` ranges are known and unique and every curve is valid; `lut_asset` has the form of an asset id; `lut_intensity` is not given alone for a clip with no table (`E_VALIDATION`).
2. The item and the `lut_asset` exist (`E_NOT_FOUND`).
3. The item is a `clip`, and `lut_asset` is of kind `lut` (`E_TYPE_MISMATCH`).

**Postconditions.**

1. Given values are set, rounded to 3 decimals; values equal to the default (0, 1, 1, 0, 0, 0) remove the field, and so does a `lut_intensity` of 0 for the table.
2. `diff` has one `item.updated`; empty when unchanged. `effective` has the stored values, and `lut` as `{asset_id, intensity}` or null.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | nothing to set, or out of range | `args.contrast must be between 0 and 2, got 2.5.` | `Use contrast 2 for the strongest setting.` |
| `E_INVALID_STATE` | a given colour property has keyframes (added with NEW-15) | `Property brightness of itm_m1n2p3q4r5 has 2 keyframes, which define its value.` | `Use keyframe.set to change a keyframe, or keyframe.clear to remove them first.` |
| `E_NOT_FOUND` | unknown item or `lut_asset` | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not a clip, or `lut_asset` is not a `lut` asset | `Item itm_s6t7v8w9x0 is an audio item; colour applies to clips.` | `Use a clip item.` |

**Inverse.** `patch {collection: "items", id, set: <old values>, unset: <fields that were absent>}`.

**Idempotency.** Same result.

**Example.** Clip `itm_m1n2p3q4r5` with default colour.

```json
{"op": "clip.set_color", "args": {"item_id": "itm_m1n2p3q4r5", "brightness": 0.1, "saturation": 1.2}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"brightness": [null, 0.1], "saturation": [1, 1.2]}}]
```

**Golden cases.**

- `ok_brightness_only`
- `ok_all_three`
- `ok_boundaries_minus1_1_0_2`
- `ok_default_values_remove_fields`
- `ok_rounded_to_3_decimals`
- `ok_same_values_noop`
- `ok_temperature_tint_hue`
- `ok_white_balance_boundaries`
- `ok_lut_sets_the_table_at_full_intensity`
- `ok_lut_with_an_intensity`
- `ok_lut_intensity_changes_the_current_table`
- `ok_lut_other_table_resets_the_intensity`
- `ok_lut_intensity_0_removes_the_table`
- `ok_lut_with_a_colour_adjustment`
- `ok_lut_same_values_noop`
- `err_validation_lut_intensity_without_a_table`
- `err_validation_lut_intensity_above_1`
- `err_validation_lut_asset_bad_id`
- `err_not_found_lut_asset`
- `err_type_mismatch_lut_asset_is_a_video`
- `ok_hsl_bands_in_hue_order`
- `ok_hsl_replaces_the_band_of_the_range`
- `ok_hsl_neutral_band_removes_it`
- `ok_wheels_set_midtones`
- `ok_wheels_neutral_wheel_removes_it`
- `ok_curves_set_red_and_blue`
- `ok_curves_empty_list_removes_the_channel`
- `ok_curves_identity_removes_the_channel`
- `ok_grade_with_a_colour_adjustment`
- `err_validation_hsl_range_twice`
- `err_validation_hsl_unknown_range`
- `err_validation_hsl_hue_above_1`
- `err_validation_wheel_hue_above_360`
- `err_validation_curve_one_point`
- `err_validation_curve_x_not_increasing`
- `err_validation_curve_nine_points`
- `err_validation_curve_y_above_1`
- `ok_hue_180`
- `ok_new_defaults_remove_fields`
- `ok_new_rounded_to_3_decimals`
- `err_validation_no_argument`
- `err_invalid_state_property_has_keyframes`
- `err_validation_brightness_above_1`
- `err_validation_saturation_negative`
- `err_validation_temperature_above_1`
- `err_validation_tint_below_minus_1`
- `err_validation_hue_above_180`
- `err_not_found`
- `err_type_mismatch_audio_item`

### C.27 `clip.remove_silences`

Replaces a clip by one clip per non-silent stretch of its source range.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip` of a video asset with audio |
| `min_silence` | time, as a length | no | `"0.4s"` | at least 1 frame; shorter silences are ignored |
| `padding` | time, as a length (may be 0) | no | `"0.1s"` | speech side kept around each cut |
| `threshold_db` | integer | no | `-35` | one of `-30`, `-35`, `-40`, `-45` (`E_VALIDATION` otherwise): the noise level below which audio counts as silence. The core only validates it; the service uses it to choose the set of ranges it puts in `resolved.silences` (SDD-011 §9.1) |
| `ripple` | `"track"` or `"none"` | no | `"track"` | |
| `new_ids` | array of item ids | no | generated | ids for the second, third, ... pieces, in order. If the list is shorter than needed the rest comes from `env.NewIDs`; if it is longer the extra ids are ignored and listed in `effective.unused_ids`. Duplicates inside the list are `E_VALIDATION` |
| `resolved` | `{"silences": [{"start", "end"}]}` | filled by the service (A.9) | none | integer frames on the asset's time axis, half-open `[start, end)`, sorted, not overlapping, `start < end`; required by `Apply` (`E_VALIDATION` when missing or malformed, `path` `args.resolved.silences[i]`) |

**Preconditions.**

1. Arguments match the schema, `new_ids` has no duplicate, and times parse (`E_VALIDATION`, `E_INVALID_TIME`).
2. The item exists (`E_NOT_FOUND`).
3. The item is a `clip` whose asset is a video with `has_audio` (`E_TYPE_MISMATCH`).
4. Each used new id is unused (`E_ID_EXISTS`); the ids in `unused_ids` are not checked.
5. At least one piece remains (`E_INVALID_STATE`: the clip is all silence).
6. The document has room for the pieces: at most 500 items (`E_LIMIT`).

**Postconditions.** Let `si = source_in`, `num/den` the speed, `S = [si, si + ceil(duration × num / den))` the visible source range, `p` and `m` the padding and minimum in frames. Lengths here are measured on the asset's time axis, whatever the speed.

1. Each silence is clipped to `S`; clipped silences shorter than `m` are dropped. For each remaining `[s, e)` the removed interval is `[s', e')` with `s' = s + p` unless `s = si` (then `s' = s`) and `e' = e - p` unless `e` is the end of `S` (then `e' = e`); it counts only if `e' - s' >= 1`.
2. The pieces are the parts of `S` outside the removed intervals, each `[a, b)`. A piece becomes an item with `source_in = a` and `duration = floor((b - a) × den / num)`; a piece whose duration would be 0 is dropped. With no removed interval the document is unchanged (R2) and `effective.removed` is 0.
3. Placement. `ripple: "track"`: pieces are back to back from the original `start`; items of the track with `start >=` the original end move earlier by `removed`. `ripple: "none"`: piece start is `start + ceil((a - si) × den / num)`, and nothing else moves; pieces never overlap each other or other items.
4. Fields. The first piece keeps the id, `transition_in`, `fade_in` and `slot`; the last keeps `transition_out` and `fade_out` (a single piece keeps both). Pieces in between have none of them. All other fields are copied. Each kept transition or fade gets the fit fix of R14. Keyframes of each piece are the original ones after R11 with `d = ceil((a - si) × den / num)`, then R12 with the piece's duration.
5. `removed` is `original duration - sum of piece durations`. Each caption linked to the clip (`linked_item`) is linked to the piece that holds its first word (the last piece that starts at or before the word's source frame, the first piece when none does), so that `caption.retime_from_clip` finds the captions on every piece (VV-152); its `item.updated` is in the diff. Nothing else of the caption changes.
6. `diff`: `item.updated` for the first piece, `item.added` for the others and `item.updated` for shifted items, all in one list sorted by id (R19), not grouped by kind. `effective` has `min_silence` and `padding` (A.4 triples; `requested` is `null` for a default), `threshold_db` (the value used), `removed` (`{"frames", "seconds"}`), `pieces` (`[{"id", "start", "duration", "source_in"}]`), `new_ids` (the identifiers used for the added pieces, in order) and `unused_ids` (the extra ids of the argument). The service stores the operation with `new_ids` equal to `effective.new_ids` (A.3, rule 4).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `resolved` missing or malformed, bad `ripple`, duplicate ids in `new_ids` | `args.resolved.silences[1] ends at 150 before it starts at 160.` | `Send silences as sorted, non-overlapping ranges.` |
| `E_INVALID_TIME` | malformed length, or `min_silence` of 0 frames | `args.min_silence "0s" is 0 frames.` | `Use "0.4s" or "12f".` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not a clip, or the asset has no audio | `Asset ast_9m8n7p6q5r has no audio; there is nothing to analyse.` | `Choose a clip whose asset has audio.` |
| `E_ID_EXISTS` | a new id is used | `Item id itm_s6t7v8w9x0 already exists.` | `Omit new_ids to generate ids.` |
| `E_INVALID_STATE` | no speech left | `Clip itm_m1n2p3q4r5 is silent over its whole source range 60-359.` | `Delete the clip with item.delete, or use a larger min_silence.` |
| `E_LIMIT` | too many items | `Splitting itm_m1n2p3q4r5 into 40 pieces would make 520 items; the limit is 500.` | `Use a larger min_silence to get fewer pieces.` |

**Inverse.** `patch {collection: "items", id: <first piece id>, set: <old values of every changed field>, unset: <fields that were absent>}`, `remove` per added piece in order, then `patch {... set: {start: <old>}}` per shifted item.

**Idempotency.** Same result with default values when the analysis is run again on the result: what remains of a silence beside speech is the padding, shorter than `min_silence`. With the same `resolved` and a `padding` of at least `min_silence` more can be removed.

**Example.** 30 fps; asset `ast_3c5v8n2w6d` (900 frames, has audio). Clip `itm_m1n2p3q4r5` on `trk_f6g7h8j9k0`: `start` 30, `duration` 300, `source_in` 60, speed 1, `fade_in` 6, `fade_out` 9; so `S = [60, 360)`. Text item `itm_v2w3x4y5z6` on the same track: `start` 330, `duration` 40. Defaults: `m` = 12 frames, `p` = 3 frames. Resolved silences: `[100,160)`, `[200,215)`, `[250,256)`, `[340,360)`.

| Silence | Length | Decision | Removed interval |
| --- | --- | --- | --- |
| `[100,160)` | 60 | kept, interior | `[103,157)` (54 frames) |
| `[200,215)` | 15 | kept, interior | `[203,212)` (9 frames) |
| `[250,256)` | 6 | below 12, ignored | none |
| `[340,360)` | 20 | touches the end of `S` | `[343,360)` (17 frames) |

Pieces: `[60,103)` 43 frames, `[157,203)` 46 frames, `[212,343)` 131 frames. Removed: 54 + 9 + 17 = 80 = 300 - 220. With ripple the pieces start at 30, 73 and 119 (ending at 250), and the text item moves from 330 to 250. Without ripple they would start at 30, 127 and 182.

```json
{"op": "clip.remove_silences", "args": {"item_id": "itm_m1n2p3q4r5", "new_ids": ["itm_s6t7v8w9x0", "itm_c3d4e5f6g7"],
  "resolved": {"silences": [{"start": 100, "end": 160}, {"start": 200, "end": 215}, {"start": 250, "end": 256}, {"start": 340, "end": 360}]}}}
```

```json
[
  {"kind": "item.added", "id": "itm_c3d4e5f6g7", "object": {"id": "itm_c3d4e5f6g7", "type": "clip", "track": "trk_f6g7h8j9k0", "start": 119, "duration": 131, "asset": "ast_3c5v8n2w6d", "source_in": 212, "fade_out": 9}},
  {"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"duration": [300, 43], "fade_out": [9, null]}},
  {"kind": "item.added", "id": "itm_s6t7v8w9x0", "object": {"id": "itm_s6t7v8w9x0", "type": "clip", "track": "trk_f6g7h8j9k0", "start": 73, "duration": 46, "asset": "ast_3c5v8n2w6d", "source_in": 157}},
  {"kind": "item.updated", "id": "itm_v2w3x4y5z6", "changes": {"start": [330, 250]}}
]
```

`effective.removed` is `{"frames": 80, "seconds": 2.667}`, `effective.min_silence` is `{"frame": 12, "seconds": 0.4, "requested": null}`, `effective.padding` is `{"frame": 3, "seconds": 0.1, "requested": null}`, `effective.new_ids` is `["itm_s6t7v8w9x0", "itm_c3d4e5f6g7"]` and `effective.unused_ids` is `[]`.

**Golden cases.**

- `ok_three_pieces_ripple_track`
- `ok_three_pieces_ripple_none`
- `ok_short_silence_ignored`
- `ok_silence_at_start_edge_no_padding_on_outer_side`
- `ok_silence_at_end_edge_no_padding_on_outer_side`
- `ok_padding_0`
- `ok_padding_larger_than_half_silence_removes_nothing`
- `ok_min_silence_custom`
- `ok_speed_3_2_piece_durations_floor`
- `ok_single_piece_keeps_both_fades_and_transitions`
- `ok_keyframes_divided_among_pieces`
- `ok_slot_stays_on_first_piece`
- `ok_new_ids_partial_rest_generated`
- `ok_new_ids_extra_ignored_and_reported`
- `ok_no_silence_noop`
- `err_validation_resolved_missing`
- `err_validation_silences_overlap`
- `err_validation_new_ids_duplicate`
- `err_invalid_time_min_silence_zero`
- `err_not_found`
- `err_type_mismatch_not_a_clip`
- `err_type_mismatch_asset_without_audio`
- `err_id_exists`
- `err_invalid_state_all_silence`
- `err_limit_over_500_items`

### C.28 `text.add`

Adds a text item to a visual track.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `track_id` | track id | no | the highest visual track free for the range, else a new visual track on top | a visual track. When omitted the core chooses (and says which in `effective.track_id`), so an overlay over a clip never fails with `E_OVERLAP` |
| `start` | time | yes | none | absolute |
| `duration` | time, as a length | yes | none | at least 1 frame |
| `text` | string | yes | none | 1 to 2,000 characters (Unicode code points); line breaks allowed |
| `style` | partial TextStyle | no | none | R15; no `null` values |
| `x`, `y` | number | no | frame centre | finite |
| `id` | item id | no | generated | R6 |

**Preconditions.**

1. Arguments match the schema and R15, times parse (`E_VALIDATION`, `E_INVALID_TIME`).
2. The track exists (`E_NOT_FOUND`).
3. The track is visual (`E_TYPE_MISMATCH`).
4. `id` is not used (`E_ID_EXISTS`); fewer than 500 items (`E_LIMIT`).
5. The font family and weight of `style` exist (`E_FONT_UNKNOWN`).
6. The item ends at or before 108,000 (`E_INVALID_TIME`) and does not overlap (`E_OVERLAP`, R8).

**Postconditions.**

1. A `text` item is added with `start`, `duration`, `track`, `text`, and `style`, `x`, `y` only when given. The stored style holds exactly the keys given, including a key whose value equals the default (R15).
2. `diff` has one `item.added`. `effective` has `id`, `start` and `duration` (A.4 triples) and `style` as stored.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | empty or over-long text, bad style value, bad id | `args.text must have 1 to 2000 characters, got 0.` | `Send the text to show, for example "Hello".` |
| `E_INVALID_TIME` | malformed time, zero duration, item ending after 108,000 | `args.duration "0s" is 0 frames.` | `Use "2s" or "60f".` |
| `E_NOT_FOUND` | unknown track | `Track trk_zzzzzzzzzz does not exist.` | `Use the id of a track listed by project_get.` |
| `E_TYPE_MISMATCH` | audio track | `Track trk_m1n2p3q4r5 is an audio track; text needs a visual track.` | `Use a visual track.` |
| `E_ID_EXISTS` | `id` used | `Item id itm_s6t7v8w9x0 already exists.` | `Omit id or use an unused id.` |
| `E_LIMIT` | 500 items | `The document already has 500 items; the limit is 500.` | `Delete unused items first.` |
| `E_FONT_UNKNOWN` | unknown family or weight | `Font "Comic" is not available; available families: Inter, Roboto.` | `Use font_family "Inter".` |
| `E_OVERLAP` | overlap | `Item itm_s6t7v8w9x0 would occupy frames 30-89 on track trk_f6g7h8j9k0, where item itm_m1n2p3q4r5 already is (frames 0-59).` | `Use start "60f" (2.000s), the next free position on that track, or another track.` |

**Inverse.** `remove {collection: "items", id: <new id>}`.

**Idempotency.** With `id`: the second application returns `E_ID_EXISTS`. Without: it overlaps the first (`E_OVERLAP`).

**Example.** 30 fps; empty visual track `trk_f6g7h8j9k0`.

```json
{"op": "text.add", "args": {"track_id": "trk_f6g7h8j9k0", "start": "0s", "duration": "2s", "text": "Hello", "style": {"size": 96}, "id": "itm_s6t7v8w9x0"}}
```

```json
[{"kind": "item.added", "id": "itm_s6t7v8w9x0", "object": {"id": "itm_s6t7v8w9x0", "type": "text", "track": "trk_f6g7h8j9k0", "start": 0, "duration": 60, "text": "Hello", "style": {"size": 96}}}]
```

**Golden cases.**

- `ok_minimal`
- `ok_with_style`
- `ok_style_value_equal_to_default_is_kept`
- `ok_with_x_y`
- `ok_multiline_text`
- `ok_text_2000_chars`
- `ok_template_color_reference`
- `ok_supplied_id`
- `ok_ends_exactly_at_108000`
- `err_validation_empty_text`
- `err_validation_text_2001_chars`
- `err_validation_style_null_value`
- `err_validation_unknown_style_key`
- `err_invalid_time_zero_duration`
- `err_invalid_time_ends_after_108000`
- `err_not_found_track`
- `err_type_mismatch_audio_track`
- `err_id_exists`
- `err_limit_500_items`
- `err_font_unknown_family`
- `err_font_unknown_weight`
- `err_overlap`
- `ok_style_with_a_project_font`
- `err_font_unknown_family_lists_project_fonts`
- `err_font_unknown_weight_of_a_project_font`

### C.29 `text.set`

Changes the content or the style of a text item.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `text` item |
| `text` | string | no | unchanged | 1 to 2,000 characters |
| `style` | partial TextStyle | no | unchanged | R15; the keys are stored as given, defaults included; `null` removes a key |
| `reset_style` | boolean | no | `false` | true: the stored style is emptied first, then `style` is applied |

At least one of `text`, `style`, `reset_style: true` is required.

**Preconditions.**

1. Arguments match the schema and R15, and something is requested (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).
3. The item is a `text` item (`E_TYPE_MISMATCH`).
4. The resulting font family and weight exist (`E_FONT_UNKNOWN`).

**Postconditions.**

1. `text` is replaced when given. The stored style becomes: (empty if `reset_style`, else the current style) merged with `style` by R15. An empty result removes `style`.
2. Timing, position and every other field are unchanged.
3. `diff` has one `item.updated` (`text`, `style`); empty when unchanged. `effective` has `text` and the stored `style`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | nothing requested, bad text or style | `args: give at least one of text, style, reset_style.` | `Send for example {"item_id": "itm_s6t7v8w9x0", "text": "New text"}.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not a text item | `Item itm_m1n2p3q4r5 is a caption; text.set changes text items.` | `Use caption.set_words or caption.edit_word for captions.` |
| `E_FONT_UNKNOWN` | unknown family or weight | `Font weight 800 does not exist for family "Inter"; available weights: 400, 700.` | `Use font_weight 700.` |

**Inverse.** `patch {collection: "items", id, set: {text, style: <old>}, unset: <fields that were absent>}`.

**Idempotency.** Same result.

**Example.** Text item `itm_s6t7v8w9x0`, `text` "Hello", style `{"size": 96}`.

```json
{"op": "text.set", "args": {"item_id": "itm_s6t7v8w9x0", "text": "Hello world", "style": {"color": "#FFE600"}}}
```

```json
[{"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"text": ["Hello", "Hello world"], "style": [{"size": 96}, {"color": "#FFE600", "size": 96}]}}]
```

**Golden cases.**

- `ok_text_only`
- `ok_style_merge`
- `ok_style_null_removes_key`
- `ok_item_key_equal_to_default_is_kept`
- `ok_reset_style_only`
- `ok_reset_style_then_style`
- `ok_text_and_style`
- `ok_same_values_noop`
- `err_validation_nothing_requested`
- `err_validation_empty_text`
- `err_validation_reset_style_false_alone`
- `err_not_found`
- `err_type_mismatch_caption_item`
- `err_font_unknown_family`
- `err_font_unknown_weight`

### C.30 `text.apply_preset`

Writes the keyframes or the transition of a named animation preset on one edge of a visual item.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip`, `text`, `caption` or `shape` item |
| `preset` | `"fade_up"`, `"fade_down"`, `"pop"`, `"zoom_in"`, `"slide_left"`, `"slide_right"` | yes | none | definitions below |
| `edge` | `"in"` or `"out"` | yes | none | |
| `duration` | time, as a length | no | `"0.4s"` | at least 1 frame, at most the item's duration |

**Preconditions.**

1. Arguments match the schema and `duration` parses to at least 1 frame (`E_VALIDATION`, `E_INVALID_TIME`).
2. The item exists (`E_NOT_FOUND`).
3. The item is visual (`E_TYPE_MISMATCH`).
4. `n <= D`, where `n` is the preset length in frames and `D` the item duration (`E_INVALID_STATE`).
5. For `slide_left` and `slide_right`: the two transitions still fit (R14, `E_INVALID_STATE`).
6. No written property ends up with more than 100 keyframes (`E_LIMIT`).

**Definitions.** `n` is the length in frames, `D` the item duration, `H = round3(0.05 × project height)` pixels. For each property `p` written, the **base** `b_p` is the value the item has where the preset meets the rest of the animation: `b_p = value_at(K_p, n)` for edge `in` and `value_at(K_p, D - n)` for edge `out`, taken before the command changes anything (R10: the static value when `p` has no keyframes). Keyframes are written as `(t, value, easing)`. Values are rounded to 3 decimals; `scale` is capped at 20.

| Preset | Edge | Keyframes written |
| --- | --- | --- |
| `fade_up` | in | `y`: `(0, b_y + H, ease_out)`, `(n, b_y)`. `opacity`: `(0, 0, ease_out)`, `(n, b_o)` |
| `fade_up` | out | `y`: `(D-n, b_y, ease_in)`, `(D, b_y - H)`. `opacity`: `(D-n, b_o, ease_in)`, `(D, 0)` |
| `fade_down` | in | `y`: `(0, b_y - H, ease_out)`, `(n, b_y)`. `opacity`: `(0, 0, ease_out)`, `(n, b_o)` |
| `fade_down` | out | `y`: `(D-n, b_y, ease_in)`, `(D, b_y + H)`. `opacity`: `(D-n, b_o, ease_in)`, `(D, 0)` |
| `zoom_in` | in | `scale`: `(0, 0.8 × b_s, ease_out)`, `(n, b_s)`. `opacity`: `(0, 0, ease_out)`, `(n, b_o)` |
| `zoom_in` | out | `scale`: `(D-n, b_s, ease_in)`, `(D, 1.2 × b_s)`. `opacity`: `(D-n, b_o, ease_in)`, `(D, 0)` |
| `pop` | in | `scale`: `(0, 0.6 × b_s, ease_out)`, `(q, 1.1 × b_s, ease_in_out)`, `(n, b_s)`. `opacity`: `(0, 0, ease_out)`, `(n, b_o)` |
| `pop` | out | `scale`: `(D-n, b_s, ease_in_out)`, `(D-q, 1.1 × b_s, ease_in)`, `(D, 0.6 × b_s)`. `opacity`: `(D-n, b_o, ease_in)`, `(D, 0)` |
| `slide_left` | in or out | no keyframes: `transition_<edge> = {kind: "slide_left", duration: n}` |
| `slide_right` | in or out | no keyframes: `transition_<edge> = {kind: "slide_right", duration: n}` |

`q = round(0.7 × n)`; the middle `pop` keyframe is omitted when `n = 1`. "Up" means the item moves up the frame on both edges (`y` grows downward): `fade_up` in starts lower and rises into place, `fade_up` out rises away; `fade_down` is the opposite. `zoom_in` grows the item on both edges. `pop` out is the time mirror of `pop` in. The last keyframe of an `in` preset, `(n, b)`, carries the easing in force at frame `n` before the command (the easing of the last keyframe at or before `n`, with its `bezier` numbers when it is a curve, linear when there is none); the last keyframe of an `out` preset has no easing. A keyframe whose easing is linear is stored without `easing`.

**Postconditions.**

1. For each property written: keyframes with `t` in `[0, n]` (edge `in`) or `[D-n, D]` (edge `out`) are removed, then the keyframes of the table are inserted. Keyframes of that property outside the range, and the static field, are untouched. Properties not in the table are untouched.
2. For `slide_left` and `slide_right` the transition of that edge is replaced and no keyframe changes.
3. `diff` has one `item.updated` with `keyframes` (or `transition_<edge>`). `effective` has `preset`, `edge`, `duration` (the A.4 triple), `written` (the property names) and `base` (the `b_p` used).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | unknown preset or edge | `args.preset "bounce" is not a preset.` | `Use fade_up, fade_down, pop, zoom_in, slide_left or slide_right.` |
| `E_INVALID_TIME` | malformed or zero `duration` | `args.duration "0f" is 0 frames.` | `Use "0.4s" or "12f".` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | audio item | `Item itm_s6t7v8w9x0 is an audio item; presets animate visual items.` | `Use item.set_audio fades for audio.` |
| `E_INVALID_STATE` | `n` above the item duration; or the slide transitions do not fit | `Preset duration 120 frames is longer than item itm_s6t7v8w9x0 (90 frames).` | `Use duration "90f" or shorter.` |
| `E_LIMIT` | more than 100 keyframes | `Property y of itm_s6t7v8w9x0 would have 101 keyframes; the limit is 100.` | `Remove keyframes with keyframe.clear first.` |

**Inverse.** `patch {collection: "items", id, set: {keyframes: <old>}}` (or `unset: ["keyframes"]`), or for slides the transition field.

**Idempotency.** Same result: the second application computes the same base and rewrites the same keyframes.

**Example.** 1080x1920 project, 30 fps. Text item `itm_s6t7v8w9x0`, `start` 0, `duration` 90, no keyframes, `y` not set (so `b_y` = 960), opacity 1. `n = 12`, `H = round3(0.05 × 1920) = 96`.

```json
{"op": "text.apply_preset", "args": {"item_id": "itm_s6t7v8w9x0", "preset": "fade_up", "edge": "in"}}
```

```json
[{"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"keyframes": [null, {
  "opacity": [{"t": 0, "value": 0, "easing": "ease_out"}, {"t": 12, "value": 1}],
  "y": [{"t": 0, "value": 1056, "easing": "ease_out"}, {"t": 12, "value": 960}]}]}}]
```

`effective.duration` is `{"frame": 12, "seconds": 0.4, "requested": "0.4s"}`.

**Golden cases.**

- `ok_fade_up_in_default_duration`
- `ok_fade_up_out`
- `ok_fade_down_in`
- `ok_fade_down_out`
- `ok_pop_in`
- `ok_pop_out`
- `ok_pop_n_1_no_overshoot_keyframe`
- `ok_pop_n_2`
- `ok_zoom_in_in`
- `ok_zoom_in_out`
- `ok_zoom_in_out_scale_capped_at_20`
- `ok_slide_left_in`
- `ok_slide_left_out`
- `ok_slide_right_in`
- `ok_slide_right_out_replaces_existing_transition`
- `ok_explicit_duration`
- `ok_duration_equals_item_duration`
- `ok_base_from_static_non_default_scale_and_opacity`
- `ok_base_from_existing_keyframes_outside_range`
- `ok_replaces_keyframes_inside_range_keeps_outside`
- `ok_last_keyframe_inherits_easing`
- `ok_in_and_out_presets_combined`
- `ok_clip_caption_shape_items`
- `ok_same_preset_twice_noop`
- `err_validation_unknown_preset`
- `err_invalid_time_zero_duration`
- `err_not_found`
- `err_type_mismatch_audio_item`
- `err_invalid_state_duration_longer_than_item`
- `err_invalid_state_slide_transitions_do_not_fit`
- `err_limit_101_keyframes`

### C.31 `shape.add`

Adds a shape item to a visual track.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `track_id` | track id | no | the highest visual track free for the range, else a new visual track on top | a visual track. When omitted the core chooses (and says which in `effective.track_id`), so an overlay over a clip never fails with `E_OVERLAP` |
| `start` | time | yes | none | absolute |
| `duration` | time, as a length | yes | none | at least 1 frame |
| `shape` | `"rect"`, `"rounded_rect"`, `"ellipse"` | yes | none | |
| `width`, `height` | number | yes | none | greater than 0, pixels before `scale` |
| `fill` | colour | no | `#FFFFFF` | hex or `"$name"` |
| `stroke_color` | colour | no | `#000000` | |
| `stroke_width` | number | no | 0 | 0 or more |
| `radius` | number | no | 0 | 0 or more; only with `"rounded_rect"` (`E_VALIDATION` otherwise) |
| `x`, `y` | number | no | frame centre | finite |
| `id` | item id | no | generated | R6 |

**Preconditions.**

1. Arguments match the schema; `radius` is only given for `"rounded_rect"`; colours are valid (`E_VALIDATION`, `E_INVALID_TIME`).
2. The track exists (`E_NOT_FOUND`).
3. The track is visual (`E_TYPE_MISMATCH`).
4. `id` is not used (`E_ID_EXISTS`); fewer than 500 items (`E_LIMIT`).
5. The item ends at or before 108,000 (`E_INVALID_TIME`) and does not overlap (`E_OVERLAP`, R8).

**Postconditions.**

1. A `shape` item is added with `start`, `duration`, `track`, `shape`, `width`, `height` and the optional fields that differ from their defaults (`x` and `y` whenever given). Colours are stored upper case.
2. `diff` has one `item.added`. `effective` has `id`, `start`, `duration` (A.4 triples).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | width or height not above 0, bad colour, `radius` on another shape | `args.radius is only valid for shape "rounded_rect", got "rect".` | `Remove radius, or use shape "rounded_rect".` |
| `E_INVALID_TIME` | malformed time, zero duration, ends after 108,000 | `args.duration "0s" is 0 frames.` | `Use "2s" or "60f".` |
| `E_NOT_FOUND` | unknown track | `Track trk_zzzzzzzzzz does not exist.` | `Use the id of a track listed by project_get.` |
| `E_TYPE_MISMATCH` | audio track | `Track trk_m1n2p3q4r5 is an audio track; shapes need a visual track.` | `Use a visual track.` |
| `E_ID_EXISTS` | `id` used | `Item id itm_s6t7v8w9x0 already exists.` | `Omit id or use an unused id.` |
| `E_LIMIT` | 500 items | `The document already has 500 items; the limit is 500.` | `Delete unused items first.` |
| `E_OVERLAP` | overlap | `Item itm_s6t7v8w9x0 would occupy frames 30-89 on track trk_f6g7h8j9k0, where item itm_m1n2p3q4r5 already is (frames 0-59).` | `Use start "60f" (2.000s), the next free position on that track, or another track.` |

**Inverse.** `remove {collection: "items", id: <new id>}`.

**Idempotency.** As C.28.

**Example.** Empty visual track `trk_f6g7h8j9k0`; 30 fps.

```json
{"op": "shape.add", "args": {"track_id": "trk_f6g7h8j9k0", "start": "0s", "duration": "3s", "shape": "rect", "width": 400, "height": 200, "fill": "#ff0000", "x": 540, "y": 960, "id": "itm_s6t7v8w9x0"}}
```

```json
[{"kind": "item.added", "id": "itm_s6t7v8w9x0", "object": {"id": "itm_s6t7v8w9x0", "type": "shape", "track": "trk_f6g7h8j9k0", "start": 0, "duration": 90, "shape": "rect", "width": 400, "height": 200, "fill": "#FF0000", "x": 540, "y": 960}}]
```

**Golden cases.**

- `ok_rect_minimal`
- `ok_ellipse`
- `ok_rounded_rect_with_radius`
- `ok_fill_lower_case_stored_upper_case`
- `ok_fill_equal_to_default_not_stored`
- `ok_stroke`
- `ok_template_color_reference`
- `ok_supplied_id`
- `err_validation_width_0`
- `err_validation_radius_on_rect`
- `err_validation_bad_color`
- `err_invalid_time_zero_duration`
- `err_not_found_track`
- `err_type_mismatch_audio_track`
- `err_id_exists`
- `err_limit_500_items`
- `err_overlap`

### C.32 `shape.set`

Changes fields of a shape item.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `shape` item |
| `shape` | `"rect"`, `"rounded_rect"`, `"ellipse"` | no | unchanged | |
| `width`, `height` | number | no | unchanged | greater than 0 |
| `fill`, `stroke_color` | colour | no | unchanged | hex or `"$name"` |
| `stroke_width` | number | no | unchanged | 0 or more |
| `radius` | number | no | unchanged | 0 or more; valid only when the resulting shape is `"rounded_rect"` |

At least one of the shape fields is required. Position and transform are changed with `item.set_transform`.

**Preconditions.**

1. At least one field is present and valid; `radius` is not given when the resulting shape is not `"rounded_rect"` (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).
3. The item is a `shape` (`E_TYPE_MISMATCH`).

**Postconditions.**

1. Given fields are set; values equal to a default remove the field. If `shape` changes to a value other than `"rounded_rect"`, a stored `radius` is removed.
2. `diff` has one `item.updated`; empty when unchanged. `effective` has the stored values of the given fields.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | nothing to set, bad value, `radius` on a non-rounded shape | `args.height must be greater than 0, got 0.` | `Use a height of at least 1.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not a shape | `Item itm_m1n2p3q4r5 is a clip; shape.set changes shapes.` | `Use the command for that item type.` |

**Inverse.** `patch {collection: "items", id, set: <old values>, unset: <fields that were absent>}`.

**Idempotency.** Same result.

**Example.** Shape `itm_s6t7v8w9x0`, `rect` 400x200, fill `#FF0000`.

```json
{"op": "shape.set", "args": {"item_id": "itm_s6t7v8w9x0", "shape": "rounded_rect", "radius": 24, "width": 600}}
```

```json
[{"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"shape": ["rect", "rounded_rect"], "radius": [null, 24], "width": [400, 600]}}]
```

**Golden cases.**

- `ok_width_height`
- `ok_fill_and_stroke`
- `ok_to_rounded_rect_with_radius`
- `ok_change_to_ellipse_removes_radius`
- `ok_radius_on_rounded_rect`
- `ok_default_value_removes_field`
- `ok_same_values_noop`
- `err_validation_no_field`
- `err_validation_height_0`
- `err_validation_radius_on_ellipse`
- `err_not_found`
- `err_type_mismatch_clip_item`

### C.33 `caption.add`

Adds a caption item with timed words.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `track_id` | track id | no | the highest visual track free for the range, else a new visual track on top | a visual track. When omitted the core chooses (and says which in `effective.track_id`), so an overlay over a clip never fails with `E_OVERLAP` |
| `start` | time | yes | none | absolute |
| `words` | array of `{text, start, duration}` | one of `words` or `text` | none | R16; the item `duration` is the end of the last word |
| `text` | string | one of `words` or `text` | none | split on Unicode whitespace; at least 1 word (none is `E_VALIDATION`) and at most 60 (`E_LIMIT`), each 1 to 80 characters |
| `duration` | time, as a length | with `text` only | none | at least 1 frame and at least the number of words; rejected with `words` (`E_VALIDATION`) |
| `linked_item` | item id | no | none | the item need not exist |
| `x`, `y` | number | no | absent | finite; the centre of the item in pixels, used when the track's caption style position is `custom`; stored on the item as SDD-001 §3 says |
| `id` | item id | no | generated | R6 |

**Preconditions.**

1. Exactly one of `words` and `text` is present, `duration` accompanies `text` only, and the arguments obey R16 or the text rules (`E_VALIDATION`, `E_INVALID_TIME`).
2. The track exists (`E_NOT_FOUND`).
3. The track is visual (`E_TYPE_MISMATCH`).
4. `duration` is at least the word count of `text` (`E_INVALID_STATE`, `details.min_duration`).
5. `id` is not used (`E_ID_EXISTS`); fewer than 500 items, at most 60 words in the caption and at most 5,000 words in the document after the command (`E_LIMIT`).
6. The item ends at or before 108,000 (`E_INVALID_TIME`) and does not overlap (`E_OVERLAP`, R8).

**Postconditions.**

1. A `caption` item is added with `start`, `track`, `words` (frames, relative to the item), `duration`, and `linked_item`, `x` and `y` when given. `breaks`, `style`, position and the rest are absent.
2. With `words`: `duration` is `last.start + last.duration`. With `text`: let `D` be the duration, `n` the word count, `c_i` the character count of word `i` and `W = sum of c_i`. Each word gets 1 frame, then the remaining `D - n` frames are shared in proportion to `c_i`: word `i` receives `floor((D - n) × c_i / W)`, and the frames still unassigned go one each to the words with the largest fractional remainder, lower index first on ties. Words follow one another with no gap from relative frame 0.
3. `diff` has one `item.added`. `effective` has `id`, `start`, `duration` (A.4 triples) and `word_count`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | both or neither of `words` and `text`; `duration` with `words`; bad word; `x` or `y` not finite | `args.words[1] overlaps words[0]: starts at 5 but the previous word ends at 10.` | `Start each word at or after the end of the previous one.` |
| `E_INVALID_TIME` | malformed time, zero word duration, item ending after 108,000 | `args.words[0].duration "0f" is 0 frames.` | `Use at least "1f".` |
| `E_NOT_FOUND` | unknown track | `Track trk_zzzzzzzzzz does not exist.` | `Use the id of a track listed by project_get.` |
| `E_TYPE_MISMATCH` | audio track | `Track trk_m1n2p3q4r5 is an audio track; captions need a visual track.` | `Use a visual track.` |
| `E_INVALID_STATE` | `duration` shorter than the word count | `Duration 3 frames is too short for 4 words; each word needs 1 frame.` | `Use duration "4f" or longer.` |
| `E_ID_EXISTS` | `id` used | `Item id itm_s6t7v8w9x0 already exists.` | `Omit id or use an unused id.` |
| `E_LIMIT` | 500 items, 61 words in the caption, or 5,000 words in the document | `The document would have 5010 caption words; the limit is 5000.` | `Remove captions you no longer need.` |
| `E_OVERLAP` | overlap | `Item itm_s6t7v8w9x0 would occupy frames 12-59 on track trk_a1b2c3d4e5, where item itm_c3d4e5f6g7 already is (frames 0-29).` | `Use start "30f" (1.000s), the next free position on that track.` |

**Inverse.** `remove {collection: "items", id: <new id>}`.

**Idempotency.** As C.28.

**Example.** 30 fps; empty visual track `trk_a1b2c3d4e5`. Text "Meet the new editor." has words of 4, 3, 3 and 7 characters (`W` = 17) and `D` = 48, so 44 frames are shared: `44 × 4/17 = 10.35`, `44 × 3/17 = 7.76`, `7.76`, `44 × 7/17 = 18.12`. Floors give 10, 7, 7, 18 (42); the 2 left over go to the largest remainders, "the" and "new" (0.76 each). Durations are 1 + 10, 1 + 8, 1 + 8, 1 + 18 = 11, 9, 9, 19 (total 48); starts 0, 11, 20, 29.

```json
{"op": "caption.add", "args": {"track_id": "trk_a1b2c3d4e5", "start": "12f", "text": "Meet the new editor.", "duration": "48f", "linked_item": "itm_m1n2p3q4r5", "id": "itm_s6t7v8w9x0"}}
```

```json
[{"kind": "item.added", "id": "itm_s6t7v8w9x0", "object": {"id": "itm_s6t7v8w9x0", "type": "caption", "track": "trk_a1b2c3d4e5", "start": 12, "duration": 48, "linked_item": "itm_m1n2p3q4r5",
  "words": [{"text": "Meet", "start": 0, "duration": 11}, {"text": "the", "start": 11, "duration": 9}, {"text": "new", "start": 20, "duration": 9}, {"text": "editor.", "start": 29, "duration": 19}]}}]
```

**Golden cases.**

- `ok_words_form`
- `ok_words_with_gaps_duration_is_last_end`
- `ok_text_form_proportional_largest_remainder`
- `ok_text_form_tie_goes_to_lower_index`
- `ok_text_form_duration_equals_word_count`
- `ok_text_form_single_word`
- `ok_text_form_collapses_runs_of_whitespace`
- `ok_60_words`
- `ok_linked_item_that_does_not_exist`
- `ok_supplied_id`
- `ok_custom_position_x_y`
- `err_validation_both_words_and_text`
- `err_validation_neither_words_nor_text`
- `err_validation_duration_with_words`
- `err_validation_empty_words`
- `err_validation_word_with_whitespace`
- `err_validation_word_81_chars`
- `err_validation_words_overlap`
- `err_invalid_time_zero_word_duration`
- `err_invalid_time_ends_after_108000`
- `err_not_found_track`
- `err_type_mismatch_audio_track`
- `err_invalid_state_duration_below_word_count`
- `err_id_exists`
- `err_limit_61_words`
- `err_limit_5000_words`
- `err_limit_500_items`
- `err_overlap`

### C.34 `caption.set_words`

Replaces the words of a caption.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `caption` |
| `words` | array of `{text, start, duration}` | one of `words`, `text` | none | R16 |
| `text` | string | one of `words`, `text` | none | split on whitespace (each word 1 to 80 characters) and spread over the duration like `caption.add` with `text` |
| `duration` | time, as a length | no | with `words`: end of the last word; with `text`: the current duration | with `words`: at least the end of the last word; with `text`: at least 1 frame per word |

**Preconditions.**

1. Arguments obey R16 (`E_VALIDATION`, `E_INVALID_TIME`).
2. The item exists (`E_NOT_FOUND`) and is a `caption` (`E_TYPE_MISMATCH`).
3. `duration`, when given, is at least the end of the last word (`E_INVALID_STATE`).
4. The caption has at most 60 words and the document keeps at most 5,000 words (`E_LIMIT`).
5. R18 for the new duration: overlap (`E_OVERLAP`), end limit (`E_INVALID_TIME`), fades and transitions fit (`E_INVALID_STATE`).

**Postconditions.**

1. `words` are replaced (converted to frames). `breaks` is cleared, because its indexes referred to the old words. `duration` is the given one or the end of the last word.
2. If the item got shorter, keyframes follow R12, which may also write static fields (`x`, `y`, `scale`, `rotation`, `opacity`). `start`, `linked_item`, `style` and every other field are unchanged.
3. `diff` has one `item.updated` with every changed field, including the static fields written by R12 (`words`, `breaks`, `duration`, `keyframes` and those). `effective` has `duration` (A.4 triple) and `word_count`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad words (R16), including an empty array | `args.words must have at least 1 entry, got 0.` | `Send at least one word.` |
| `E_INVALID_TIME` | malformed time or a word `duration` below 1 frame | `args.words[1].start "5" has no unit.` | `Use "5f" or "0.167s".` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not a caption | `Item itm_m1n2p3q4r5 is a text item; use text.set.` | `Use text.set for text items.` |
| `E_INVALID_STATE` | `duration` before the last word ends, or fades do not fit | `args.duration 20 frames is before the last word ends at 24.` | `Use duration "24f" or longer, or omit it.` |
| `E_LIMIT` | 61 words in the caption, or 5,000 words in the document | `The document would have 5003 caption words; the limit is 5000.` | `Remove captions you no longer need.` |
| `E_OVERLAP` | a longer caption overlaps | `Item itm_s6t7v8w9x0 would occupy frames 12-95 on track trk_a1b2c3d4e5, where item itm_c3d4e5f6g7 already is (frames 60-119).` | `Use words that end by "48f", or move the next caption.` |

**Inverse.** `patch {collection: "items", id, set: <old values of every changed field, including the static fields written by R12>, unset: <fields that were absent>}`.

**Idempotency.** Same result.

**Example.** 30 fps; caption `itm_s6t7v8w9x0`, `start` 12, `duration` 48, four words, no keyframes.

```json
{"op": "caption.set_words", "args": {"item_id": "itm_s6t7v8w9x0", "words": [{"text": "Hello", "start": "0f", "duration": "12f"}, {"text": "world", "start": "12f", "duration": "12f"}]}}
```

```json
[{"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {
  "duration": [48, 24],
  "words": [[{"text": "Meet", "start": 0, "duration": 9}, {"text": "the", "start": 9, "duration": 6}, {"text": "new", "start": 15, "duration": 12}, {"text": "editor.", "start": 27, "duration": 21}],
            [{"text": "Hello", "start": 0, "duration": 12}, {"text": "world", "start": 12, "duration": 12}]]}}]
```

**Golden cases.**

- `ok_replace_shorter`
- `ok_replace_longer_free_space`
- `ok_explicit_duration_longer_than_last_word`
- `ok_breaks_cleared`
- `ok_keyframes_cut_when_shorter`
- `ok_60_words`
- `ok_keyframes_all_dropped_becomes_static_when_shorter`
- `err_validation_empty_words`
- `err_validation_words_overlap`
- `err_validation_word_with_whitespace`
- `err_invalid_time_no_unit`
- `err_invalid_time_zero_word_duration`
- `err_limit_61_words`
- `err_not_found`
- `err_type_mismatch_text_item`
- `err_invalid_state_duration_before_last_word_end`
- `err_invalid_state_fade_no_longer_fits`
- `err_limit_5000_words`
- `err_overlap_longer_caption`

### C.35 `caption.edit_word`

Changes the text of one word, or removes the word.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `caption` |
| `index` | integer | yes | none | 0-based, 0 or more |
| `text` | string | yes | none | 0 to 80 characters, no whitespace; `""` removes the word |

**Preconditions.**

1. Arguments match the schema (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`, `details.kind` `item`).
3. The item is a `caption` (`E_TYPE_MISMATCH`). This is checked before the word index.
4. `index` is below the word count (`E_NOT_FOUND`, `details.kind` `word`, `details.id` the index).
5. Removing the only word of a caption is `E_INVALID_STATE`; removing the last of several words is allowed.

**Postconditions.**

1. Replacing: `words[index].text` is the new text; timing is untouched.
2. Removing: the word is removed and the others keep their own `start` and `duration`, so a gap is left; the item's `duration` does not change. `breaks`: an entry `i` becomes `i` if `i <= index`, else `i - 1`; entries below 1, at or beyond the new word count, or duplicated are dropped.
3. `diff` has one `item.updated` (`words`, and `breaks` when it changed); empty when the text is unchanged. `effective` has `index`, `removed` (boolean) and `word_count`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | negative index, text with whitespace or over 80 characters | `args.text "new editor" contains whitespace; a word has none.` | `Use caption.set_words to replace one word by several.` |
| `E_NOT_FOUND` | unknown item, or no word at `index` (`details.kind` `word`) | `Caption itm_s6t7v8w9x0 has 4 words; index 7 does not exist.` | `Use an index from 0 to 3.` |
| `E_TYPE_MISMATCH` | not a caption | `Item itm_m1n2p3q4r5 is a text item; use text.set.` | `Use text.set for text items.` |
| `E_INVALID_STATE` | removing the only word | `Removing word 0 would leave caption itm_s6t7v8w9x0 with no words.` | `Delete the caption with item.delete instead.` |

**Inverse.** `patch {collection: "items", id, set: {words, breaks: <old>}, unset: <fields that were absent>}`.

**Idempotency.** Replacing: same result. Removing: the second application removes the next word that moved into `index` (not idempotent), or fails with `E_NOT_FOUND` or `E_INVALID_STATE` when none is left.

**Example.** Caption `itm_s6t7v8w9x0` with words "Meet", "the", "new", "editor.".

```json
{"op": "caption.edit_word", "args": {"item_id": "itm_s6t7v8w9x0", "index": 3, "text": "editor"}}
```

```json
[{"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"words": [
  [{"text": "Meet", "start": 0, "duration": 9}, {"text": "the", "start": 9, "duration": 6}, {"text": "new", "start": 15, "duration": 12}, {"text": "editor.", "start": 27, "duration": 21}],
  [{"text": "Meet", "start": 0, "duration": 9}, {"text": "the", "start": 9, "duration": 6}, {"text": "new", "start": 15, "duration": 12}, {"text": "editor", "start": 27, "duration": 21}]]}}]
```

**Golden cases.**

- `ok_replace_text`
- `ok_replace_with_same_text_noop`
- `ok_replace_80_chars`
- `ok_remove_middle_word_leaves_gap`
- `ok_remove_first_word`
- `ok_remove_last_of_several_words`
- `ok_remove_adjusts_breaks`
- `err_validation_negative_index`
- `err_validation_text_with_whitespace`
- `err_validation_text_81_chars`
- `err_not_found_item`
- `err_not_found_index_out_of_range`
- `err_type_mismatch_text_item`
- `err_type_mismatch_checked_before_index`
- `err_invalid_state_remove_only_word`

### C.36 `caption.split`

Splits a caption into two at a word boundary.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `caption` |
| `before_word` | integer | yes | none | 1 or more; the index of the first word of the right part |
| `new_id` | item id | no | generated | R6 |

**Preconditions.**

1. Arguments match the schema (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`) and is a `caption` (`E_TYPE_MISMATCH`).
3. A word exists at index `before_word`, that is, `before_word` is below the word count (`E_NOT_FOUND`, `details.kind` `word`, `details.id` the index, `details.word_count`).
4. `new_id` is not used (`E_ID_EXISTS`); fewer than 500 items (`E_LIMIT`).

**Postconditions.** Let `k = before_word` and `Tc = words[k].start` (relative to the item). The cut falls at the start of word `k`, so the two parts together cover exactly the original range. R17 applies with that `Tc`.

1. Left part (the original id): `duration = Tc`, words `0` to `k - 1` unchanged. Right part: `start = start + Tc`, `duration = old duration - Tc`, words `k` onward with `start` reduced by `Tc`.
2. `breaks`: the left keeps entries `i` with `1 <= i <= k - 1`; the right keeps entries `i > k` as `i - k`. `linked_item` and `style` are copied to the right part. Transitions, keyframes (R13) and `slot` follow R17 (captions have no fades or `source_in`). When a property has all its keyframes before the cut, or all after it, the part with none gets the static value (R13).
3. `diff`: `item.updated` for the left part (every changed field, including static fields written by R13) and `item.added` for the right, sorted by id. `effective` has `new_id`, `left` and `right` (each `{"id", "start", "duration"}`).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `before_word` below 1 or bad id | `args.before_word must be 1 or more, got 0.` | `Use 1 to split after the first word.` |
| `E_NOT_FOUND` | unknown item, or no word at `before_word` (`details.kind` `word`) | `Caption itm_s6t7v8w9x0 has 4 words; word 4 does not exist, so the right part would be empty.` | `Use a before_word from 1 to 3.` |
| `E_TYPE_MISMATCH` | not a caption | `Item itm_m1n2p3q4r5 is a text item; use item.split.` | `Use item.split for other items.` |
| `E_ID_EXISTS` | `new_id` used | `Item id itm_c3d4e5f6g7 already exists.` | `Omit new_id or use an unused id.` |
| `E_LIMIT` | 500 items | `The document already has 500 items; the limit is 500.` | `Delete unused items first.` |

**Inverse.** `patch {collection: "items", id: <left id>, set: <old values of every changed field, including static fields written by R13>, unset: <fields that were absent>}`, then `remove {collection: "items", id: <new id>}`, in the order of mutation (R5).

**Idempotency.** Not idempotent: the second application splits the left part again; with the same `before_word` it fails with `E_NOT_FOUND` (kind `word`) when the left part has too few words, and with `E_ID_EXISTS` for the same `new_id`.

**Example.** 30 fps; caption `itm_s6t7v8w9x0` on `trk_a1b2c3d4e5`: `start` 12, `duration` 48, words Meet (0, 9), the (9, 6), new (15, 12), editor. (27, 21), `linked_item` `itm_m1n2p3q4r5`. `before_word` 2, so `Tc = 15`.

```json
{"op": "caption.split", "args": {"item_id": "itm_s6t7v8w9x0", "before_word": 2, "new_id": "itm_c3d4e5f6g7"}}
```

```json
[
  {"kind": "item.added", "id": "itm_c3d4e5f6g7", "object": {"id": "itm_c3d4e5f6g7", "type": "caption", "track": "trk_a1b2c3d4e5", "start": 27, "duration": 33, "linked_item": "itm_m1n2p3q4r5",
    "words": [{"text": "new", "start": 0, "duration": 12}, {"text": "editor.", "start": 12, "duration": 21}]}},
  {"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"duration": [48, 15],
    "words": [[{"text": "Meet", "start": 0, "duration": 9}, {"text": "the", "start": 9, "duration": 6}, {"text": "new", "start": 15, "duration": 12}, {"text": "editor.", "start": 27, "duration": 21}],
              [{"text": "Meet", "start": 0, "duration": 9}, {"text": "the", "start": 9, "duration": 6}]]}}
]
```

**Golden cases.**

- `ok_split_in_the_middle`
- `ok_before_word_1`
- `ok_before_last_word`
- `ok_gap_before_cut_word_stays_in_left_duration`
- `ok_breaks_divided`
- `ok_style_and_linked_item_copied_to_right`
- `ok_transition_in_left_transition_out_right`
- `ok_keyframes_divided`
- `ok_keyframes_all_before_cut`
- `ok_keyframes_all_after_cut`
- `ok_slot_stays_left`
- `err_validation_before_word_0`
- `err_not_found`
- `err_not_found_before_word_equals_word_count`
- `err_type_mismatch_text_item`
- `err_id_exists`
- `err_limit_500_items`

### C.37 `caption.merge`

Joins two captions of one track into one that spans both.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_ids` | array of item ids | yes | none | exactly 2 distinct ids, in any order; the one with the lower `start` is the "first" |

**Preconditions.**

1. Exactly two distinct ids (`E_VALIDATION`).
2. Both items exist (`E_NOT_FOUND`) and are captions (`E_TYPE_MISMATCH`).
3. They are on the same track, and no other item of the track starts between the end of the first and the start of the second (`E_INVALID_STATE`, `details.between` lists the ids).
4. Neither has keyframes (`E_INVALID_STATE`, hint to use `keyframe.clear`).
5. The merged caption has at most 60 words (`E_LIMIT`).

**Postconditions.** The operation first removes the second caption, then extends the first.

1. The first item gets `duration = second.start + second.duration - first.start` and `words` = its words followed by the second's words with `start` increased by `second.start - first.start`. `breaks`: the first's entries, then the second's entries increased by the first's word count. No break is added at the junction.
2. The first item keeps its `style`, `slot`, `transition_in`, `name`, position and `linked_item` (the second's `linked_item` is used only when the first has none). It takes the second's `transition_out`. The second item is removed, with its other fields; `effective.dropped` lists the fields of the second that were present and not carried over.
3. `diff`: `item.updated` (first) and `item.removed` (second), sorted by id. `effective` has `id` (the first's) and `word_count`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | not exactly two distinct ids | `args.item_ids must have exactly 2 entries, got 3.` | `Merge two captions at a time.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the ids of captions listed by project_get.` |
| `E_TYPE_MISMATCH` | an item is not a caption | `Item itm_m1n2p3q4r5 is a text item; only captions can be merged.` | `Pass two caption items.` |
| `E_INVALID_STATE` | different tracks, an item between, or keyframes | `Item itm_p7q8r9s0t1 (frames 40-59) lies between captions itm_s6t7v8w9x0 and itm_c3d4e5f6g7.` | `Move or delete itm_p7q8r9s0t1 first.` |
| `E_LIMIT` | more than 60 words | `The merged caption would have 64 words; the limit is 60.` | `Remove words first, or keep the captions separate.` |

**Inverse.** The operation first removes the second caption, then extends the first. The inverse list, in mutation order, is `[put {collection: "items", object: <second>}, patch {collection: "items", id: <first id>, set: <old values>, unset: <fields that were absent>}]`. Undo applies it in reverse: it shrinks the first caption, then restores the second, so the second never exists while the first still spans it.

**Idempotency.** Error: the second application returns `E_NOT_FOUND` for the removed item.

**Example.** The result of C.36 undone by hand: captions `itm_s6t7v8w9x0` (`start` 12, `duration` 15, words Meet, the) and `itm_c3d4e5f6g7` (`start` 27, `duration` 33, words new (0, 12), editor. (12, 21)), adjacent on `trk_a1b2c3d4e5`.

```json
{"op": "caption.merge", "args": {"item_ids": ["itm_c3d4e5f6g7", "itm_s6t7v8w9x0"]}}
```

```json
[
  {"kind": "item.removed", "id": "itm_c3d4e5f6g7", "object": {"id": "itm_c3d4e5f6g7", "type": "caption", "track": "trk_a1b2c3d4e5", "start": 27, "duration": 33, "linked_item": "itm_m1n2p3q4r5",
    "words": [{"text": "new", "start": 0, "duration": 12}, {"text": "editor.", "start": 12, "duration": 21}]}},
  {"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"duration": [15, 48],
    "words": [[{"text": "Meet", "start": 0, "duration": 9}, {"text": "the", "start": 9, "duration": 6}],
              [{"text": "Meet", "start": 0, "duration": 9}, {"text": "the", "start": 9, "duration": 6}, {"text": "new", "start": 15, "duration": 12}, {"text": "editor.", "start": 27, "duration": 21}]]}}
]
```

**Golden cases.**

- `ok_adjacent_captions`
- `ok_ids_given_in_reverse_order`
- `ok_gap_between_captions_spans_both`
- `ok_breaks_combined`
- `ok_style_of_second_dropped_and_reported`
- `ok_linked_item_taken_from_second_when_first_has_none`
- `ok_transition_out_taken_from_second`
- `ok_merged_exactly_60_words`
- `err_validation_three_ids`
- `err_validation_same_id_twice`
- `err_not_found`
- `err_type_mismatch_text_item`
- `err_invalid_state_different_tracks`
- `err_invalid_state_item_between`
- `err_invalid_state_keyframes_present`
- `err_limit_61_words`

### C.38 `caption.set_breaks`

Sets the forced line breaks of a caption.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `caption` |
| `breaks` | array of integers | yes | none | strictly increasing, each from 1 to word count minus 1; `[]` clears |

**Preconditions.**

1. Arguments match the schema and the range and order rules (`E_VALIDATION`, `path` `args.breaks[i]`).
2. The item exists (`E_NOT_FOUND`) and is a `caption` (`E_TYPE_MISMATCH`).

**Postconditions.**

1. `breaks` is the given array; `[]` removes the field.
2. `diff` has one `item.updated`; empty when unchanged. `effective` is `{"breaks": [...]}`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | entry out of range, not increasing | `args.breaks[1] is 4 but the caption has 4 words; the largest index is 3.` | `Use indexes from 1 to 3.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not a caption | `Item itm_m1n2p3q4r5 is a text item; text breaks lines in its text.` | `Put a line break in the text with text.set.` |

**Inverse.** `patch {collection: "items", id, set: {breaks: <old>}}` or `unset: ["breaks"]`.

**Idempotency.** Same result.

**Example.** Caption `itm_s6t7v8w9x0` with four words and no breaks.

```json
{"op": "caption.set_breaks", "args": {"item_id": "itm_s6t7v8w9x0", "breaks": [2]}}
```

```json
[{"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"breaks": [null, [2]]}}]
```

**Golden cases.**

- `ok_one_break`
- `ok_several_breaks`
- `ok_break_at_last_allowed_index`
- `ok_clear_with_empty_array`
- `ok_same_breaks_noop`
- `err_validation_index_0`
- `err_validation_index_equals_word_count`
- `err_validation_not_increasing`
- `err_validation_duplicate`
- `err_not_found`
- `err_type_mismatch_text_item`

### C.39 `caption.set_style`

Sets, changes or clears the style override of one caption.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `caption` |
| `style` | partial CaptionStyle or `null` | yes | none | R15. The keys are merged into the existing override (VV-070); a `null` value inside removes that key; with `replace` the object replaces the whole override and is stored exactly as given, keys equal to a default included, and `null` values inside are `E_VALIDATION`; `null` and `{}` clear the override. `highlight` is `"none"` or `"word"` |
| `replace` | boolean | no | false | true replaces the whole override with `style` instead of merging |

**Preconditions.**

1. Arguments match the schema and R15 (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`) and is a `caption` (`E_TYPE_MISMATCH`).
3. The resulting font family and weight exist (`E_FONT_UNKNOWN`).

**Postconditions.**

1. The item's `style` is the existing override with the given keys set (a `null` value removes its key), or exactly the given object with `replace`, or absent for `null`, `{}` or an empty result. Every key kept is kept even when equal to the default (R15). Keeping the key matters: it overrides a different value on the track. The track's `caption_style` is not touched.
2. `diff` has one `item.updated` (`style`); empty when unchanged. `effective` has the stored `style`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | unknown key, value out of range, bad colour | `args.style.highlight must be "none" or "word", got "all".` | `Use "word" to highlight the spoken word.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not a caption | `Item itm_m1n2p3q4r5 is a text item; use text.set.` | `Use text.set for text items.` |
| `E_FONT_UNKNOWN` | unknown family or weight | `Font "Comic" is not available; available families: Inter, Roboto.` | `Use font_family "Inter".` |

**Inverse.** `patch {collection: "items", id, set: {style: <old>}}` or `unset: ["style"]`.

**Idempotency.** Same result.

**Example.** Caption `itm_s6t7v8w9x0` without override.

```json
{"op": "caption.set_style", "args": {"item_id": "itm_s6t7v8w9x0", "style": {"size": 80, "highlight": "word"}}}
```

```json
[{"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"style": [null, {"highlight": "word", "size": 80}]}}]
```

**Golden cases.**

- `ok_set_override`
- `ok_merge_keeps_the_other_keys`
- `ok_null_removes_one_key`
- `ok_replace_existing_override`
- `ok_key_equal_to_default_overrides_track_style`
- `ok_clear_with_null`
- `ok_clear_with_empty_object`
- `ok_clear_when_none_noop`
- `ok_template_color_reference`
- `err_validation_bad_highlight`
- `err_validation_null_value_inside`
- `err_validation_unknown_color_reference`
- `err_not_found`
- `err_type_mismatch_text_item`
- `err_font_unknown_family`

### C.40 `caption.from_transcript`

Creates caption items for the speech in a clip's visible source range, from a transcript resolved by the service.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `clip_item_id` | item id | yes | none | a `clip` of a video asset with audio |
| `track_id` | track id | yes | none | a visual track that receives the captions |
| `max_words` | integer | no | 5 | 1 to 60 |
| `max_chars` | integer | no | 28 | 1 to 500 |
| `max_gap` | time, as a length (may be 0) | no | `"0.6s"` | a pause longer than this starts a new caption |
| `replace` | boolean | no | `true` | remove the existing captions linked to this clip on this track first |
| `new_ids` | array of item ids | no | generated | one per created caption, in order. If the list is shorter than needed the rest comes from `env.NewIDs`; if it is longer the extra ids are ignored and listed in `effective.unused_ids`. Duplicates inside the list are `E_VALIDATION` |
| `resolved` | `{"words": [{"text", "start", "end"}]}` | filled by the service (A.9) | none | `start` and `end` are integer frames on the asset's time axis, half-open, in order, not overlapping; `text` has 1 to 80 characters and no whitespace; required by `Apply` (`E_VALIDATION`, `path` `args.resolved.words[i]`) |

**Preconditions.**

1. Arguments match the schema, `new_ids` has no duplicate, and `resolved` is well formed (`E_VALIDATION`, `E_INVALID_TIME`).
2. The clip and the track exist (`E_NOT_FOUND`).
3. The item is a `clip` whose asset is a video with `has_audio`, and the track is visual (`E_TYPE_MISMATCH`).
4. At least one word falls inside the visible range (`E_INVALID_STATE`; nothing is removed in that case).
5. Each used new id is unused (`E_ID_EXISTS`). With `replace` true the ids are checked after the captions being replaced are removed, so a new id may reuse the id of a caption that is being replaced. The ids in `unused_ids` are not checked.
6. At most 500 items and 5,000 words in the document after the command (`E_LIMIT`).
7. The new captions do not overlap an item of the track that stays (`E_OVERLAP`, R8). Captions removed by `replace` do not count.

**Postconditions.** Let `si = source_in`, speed `num/den`, `S = [si, S_end)` with `S_end = si + ceil(duration × num / den)`, and `t(p) = start + ceil((p - si) × den / num)` the first timeline frame that shows asset frame `p`. The clip's end is `start + duration`.

1. **Word mapping.** A word is kept when `si <= word.start < S_end`. Its timeline range is `[t(word.start), t(min(word.end, S_end)))`. In order, each start becomes `max(start, previous end)`, each end becomes `max(end, start + 1)` and is capped at the clip's end; a word whose start reaches the clip's end is dropped.
2. **Grouping.** Walking the mapped words in order, a word starts a new caption when adding it would give the current caption more than `max_words` words, or when the caption's text (words joined by single spaces) would exceed `max_chars` characters, or when its start minus the previous word's end is greater than `max_gap` frames. A first word longer than `max_chars` still forms its own caption.
3. Each group becomes a `caption` item on `track_id`: `start` is its first word's start, `duration` the last word's end minus that start, `words` are relative to `start`, `linked_item` is `clip_item_id`. No style, breaks or position.
4. With `replace` true, every caption item on `track_id` whose `linked_item` is `clip_item_id` is removed first, wherever it is. Captions on other tracks are not touched.
5. `diff`: `item.removed` and `item.added` entries sorted by id. `effective` has `max_gap` (the A.4 triple; `requested` is `null` for the default), `created` (ids in order), `new_ids` (the identifiers used, in order; the same list as `created`), `unused_ids` (the extra ids of the argument), `removed` (ids), `words_used` and `words_skipped`. The service stores the operation with `new_ids` equal to `effective.new_ids` (A.3, rule 4).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `resolved` missing or malformed, `max_words` out of range, duplicate ids in `new_ids` | `args.resolved.words[2] starts at 90 before the previous word ends at 100.` | `Send words in order without overlap.` |
| `E_INVALID_TIME` | malformed `max_gap` | `args.max_gap "0.6" has no unit.` | `Use "0.6s" or "18f".` |
| `E_NOT_FOUND` | unknown clip or track | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of a clip listed by project_get.` |
| `E_TYPE_MISMATCH` | not a clip, asset without audio, or audio track | `Track trk_m1n2p3q4r5 is an audio track; captions need a visual track.` | `Use a visual track, for example the one from track.add.` |
| `E_INVALID_STATE` | no word in the visible range | `The transcript has no word between source frames 60 and 179 of clip itm_m1n2p3q4r5.` | `Check that the clip contains speech, or widen the clip with item.trim.` |
| `E_ID_EXISTS` | a new id used | `Item id itm_c3d4e5f6g7 already exists.` | `Omit new_ids to generate ids.` |
| `E_LIMIT` | too many items or words | `The document would have 5010 caption words; the limit is 5000.` | `Remove captions you no longer need.` |
| `E_OVERLAP` | overlap with a kept item | `Item itm_c3d4e5f6g7 would occupy frames 92-133 on track trk_a1b2c3d4e5, where item itm_p7q8r9s0t1 already is (frames 92-121).` | `replace true removes only the captions of this clip on this track, not the item in the way; clear that item or use another track.` |

**Inverse.** `put {collection: "items", object}` for each removed caption, then `remove {collection: "items", id}` for each created one, in the order the operation produced them.

**Idempotency.** Same result with `replace` true: the second application removes the captions it created and creates identical ones. Because identifiers are checked after the captions being replaced are removed (precondition 5), repeating the same `new_ids` is accepted and gives the same ids; without `new_ids`, new ids are generated. With `replace` false: `E_OVERLAP`.

**Example.** 30 fps; asset `ast_3c5v8n2w6d` (video with audio). Clip `itm_m1n2p3q4r5` on `trk_f6g7h8j9k0`: `start` 90, `duration` 120, `source_in` 60, speed 1, so `S = [60, 180)` and `t(p) = 30 + p`. Caption track `trk_a1b2c3d4e5` has caption `itm_p7q8r9s0t1` (`start` 92, `duration` 30, one word "Old", `linked_item` the clip). Defaults: `max_words` 5, `max_chars` 28, `max_gap` 18 frames.

| Transcript word (asset frames) | Kept | Timeline range |
| --- | --- | --- |
| Intro `[20,30)` | no, starts before 60 | none |
| Hello `[62,70)` | yes | `[92,100)` |
| everyone, `[72,86)` | yes | `[102,116)` |
| welcome `[90,100)` | yes | `[120,130)` |
| to `[100,104)` | yes | `[130,134)` |
| the `[104,108)` | yes | `[134,138)` |
| channel `[110,128)` | yes | `[140,158)` |
| today `[170,182)` | yes, start inside `S` | `[200,210)` (end capped at the clip's end) |

`effective.max_gap` is `{"frame": 18, "seconds": 0.6, "requested": null}`, `effective.new_ids` is `["itm_c3d4e5f6g7", "itm_s6t7v8w9x0", "itm_h2j3k4m5n6"]` and `effective.unused_ids` is `[]`.

Grouping: "Hello everyone, welcome to" is 26 characters; adding "the" would make 30, above 28, so a new caption starts at "the". "the channel" is followed by a 42-frame pause (158 to 200), above 18, so "today" starts a third caption. The old caption is removed first.

```json
{"op": "caption.from_transcript", "args": {"clip_item_id": "itm_m1n2p3q4r5", "track_id": "trk_a1b2c3d4e5", "new_ids": ["itm_c3d4e5f6g7", "itm_s6t7v8w9x0", "itm_h2j3k4m5n6"],
  "resolved": {"words": [
    {"text": "Intro", "start": 20, "end": 30}, {"text": "Hello", "start": 62, "end": 70}, {"text": "everyone,", "start": 72, "end": 86},
    {"text": "welcome", "start": 90, "end": 100}, {"text": "to", "start": 100, "end": 104}, {"text": "the", "start": 104, "end": 108},
    {"text": "channel", "start": 110, "end": 128}, {"text": "today", "start": 170, "end": 182}]}}}
```

```json
[
  {"kind": "item.added", "id": "itm_c3d4e5f6g7", "object": {"id": "itm_c3d4e5f6g7", "type": "caption", "track": "trk_a1b2c3d4e5", "start": 92, "duration": 42, "linked_item": "itm_m1n2p3q4r5",
    "words": [{"text": "Hello", "start": 0, "duration": 8}, {"text": "everyone,", "start": 10, "duration": 14}, {"text": "welcome", "start": 28, "duration": 10}, {"text": "to", "start": 38, "duration": 4}]}},
  {"kind": "item.added", "id": "itm_h2j3k4m5n6", "object": {"id": "itm_h2j3k4m5n6", "type": "caption", "track": "trk_a1b2c3d4e5", "start": 200, "duration": 10, "linked_item": "itm_m1n2p3q4r5",
    "words": [{"text": "today", "start": 0, "duration": 10}]}},
  {"kind": "item.removed", "id": "itm_p7q8r9s0t1", "object": {"id": "itm_p7q8r9s0t1", "type": "caption", "track": "trk_a1b2c3d4e5", "start": 92, "duration": 30, "linked_item": "itm_m1n2p3q4r5",
    "words": [{"text": "Old", "start": 0, "duration": 30}]}},
  {"kind": "item.added", "id": "itm_s6t7v8w9x0", "object": {"id": "itm_s6t7v8w9x0", "type": "caption", "track": "trk_a1b2c3d4e5", "start": 134, "duration": 24, "linked_item": "itm_m1n2p3q4r5",
    "words": [{"text": "the", "start": 0, "duration": 4}, {"text": "channel", "start": 6, "duration": 18}]}}
]
```

**Golden cases.**

- `ok_worked_example_replace_true`
- `ok_replace_false_with_free_track`
- `ok_replace_removes_only_captions_linked_to_this_clip_on_this_track`
- `ok_max_words_split`
- `ok_max_chars_split`
- `ok_max_gap_split_strict_greater_than`
- `ok_max_gap_0`
- `ok_single_word_longer_than_max_chars`
- `ok_clip_with_speed_3_2_mapping_uses_ceil`
- `ok_word_before_in_point_dropped`
- `ok_word_crossing_out_point_kept_and_capped`
- `ok_word_starting_at_out_point_dropped`
- `ok_clip_not_at_frame_0`
- `ok_new_ids_partial_rest_generated`
- `ok_new_ids_extra_ignored_and_reported`
- `ok_replace_true_new_ids_reuse_replaced_ids`
- `err_validation_resolved_missing`
- `err_validation_words_overlap`
- `err_validation_new_ids_duplicate`
- `err_validation_max_words_61`
- `err_invalid_time_max_gap_no_unit`
- `err_not_found_clip`
- `err_not_found_track`
- `err_type_mismatch_not_a_clip`
- `err_type_mismatch_asset_without_audio`
- `err_type_mismatch_audio_track`
- `err_invalid_state_no_word_in_range`
- `err_id_exists`
- `err_limit_5000_words`
- `err_limit_500_items`
- `err_overlap_replace_false`

### C.41 `caption.retime_from_clip`

Recomputes the timing of every caption linked to a clip from the transcript and the clip's current position, in-point and speed.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `clip_item_id` | item id | yes | none | a `clip` of a video asset with audio |
| `resolved` | `{"words": [...]}` | filled by the service (A.9) | none | as in C.40 |

**Preconditions.**

1. Arguments match the schema and `resolved` is well formed (`E_VALIDATION`).
2. The clip exists (`E_NOT_FOUND`).
3. The item is a `clip` whose asset is a video with `has_audio` (`E_TYPE_MISMATCH`).
4. At least one caption has `linked_item` equal to the clip, and at least one transcript word is visible (`E_INVALID_STATE`).
5. After retiming, fades and transitions of each caption fit its new duration (R14, `E_INVALID_STATE`).
6. No caption overlaps another item of its track (`E_OVERLAP`, R8, checked on the final state of the track).

**Postconditions.**

1. The visible transcript words `W` are mapped to the timeline exactly as in C.40, Postcondition 1.
2. Captions linked to the clip are handled per track, each track independently. On a track, the captions are taken in order of `start` (ties by id). Caption `i` has `n_i` words and takes the next `n_i` words of `W` in order. Captions are not matched by time, because after a trim or speed change the old positions say nothing about the transcript.
3. A caption that receives `n_i` words, its own word count, keeps its own texts (edited ones included) and gets the new timings. A caption that receives fewer words, because `W` ran out, takes the transcript texts of the words it receives and its id is listed in `effective.text_reset`; one that receives none is removed and its id is listed in `effective.removed`. Transcript words left over are not added (`effective.unassigned` counts them); `caption.from_transcript` creates captions for them.
4. For each retimed caption: `start` is the first word's mapped start, `duration` the last word's end minus `start`, word `start` and `duration` come from the mapping. `breaks` entries at or beyond the new word count are dropped. If the caption got shorter, keyframes follow R12, which may also write static fields.
5. `linked_item`, `style`, position, transitions and every other field are unchanged.
6. `diff`: `item.updated` per retimed caption (every changed field, including static fields written by R12) and `item.removed` per removed one, sorted by id. The removal is part of the diff and is undone by undo. `effective` has `retimed` (ids), `removed` (ids), `text_reset` (ids) and `unassigned`.

**Decision.** Transcript words are assigned to the linked captions in `start` order by word count. A caption whose word count is unchanged keeps its edited texts; otherwise it takes the transcript texts and its id is listed in `effective.text_reset`. A caption that receives no word is removed and its id is listed in `effective.removed`. This keeps the behaviour of Part B ("keeping each caption's edited texts where the word count is unchanged") and makes the other cases explicit.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `resolved` missing or malformed | `args.resolved is required.` | `Run this through the service, which fills resolved from the transcript.` |
| `E_NOT_FOUND` | unknown clip | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of a clip listed by project_get.` |
| `E_TYPE_MISMATCH` | not a clip, or no audio | `Asset ast_9m8n7p6q5r has no audio; there is no transcript to follow.` | `Pass the clip that carries the speech.` |
| `E_INVALID_STATE` | no linked caption, no visible word, or fades do not fit | `No caption is linked to clip itm_m1n2p3q4r5.` | `Create captions with caption.from_transcript first.` |
| `E_OVERLAP` | a retimed caption overlaps another item | `Item itm_s6t7v8w9x0 would occupy frames 120-135 on track trk_a1b2c3d4e5, where item itm_p7q8r9s0t1 already is (frames 130-149).` | `Move itm_p7q8r9s0t1 away, or put it on another track.` |

**Inverse.** `patch {collection: "items", id, set: <old values of every changed field: start, duration, words, breaks, keyframes and the static fields written by R12>, unset: <fields that were absent>}` per retimed caption, and `put {collection: "items", object}` per removed one, in the order of mutation (R5).

**Idempotency.** Same result: with the same clip state and transcript the second application computes the same timings.

**Example.** The result of C.40, then the clip was sped up to 1.5x with `clip.set_speed` (`start` 90, `duration` 80, `source_in` 60, speed 3/2; `S = [60, 180)`) and the caption `itm_c3d4e5f6g7` word "welcome" was edited to "Welcome". Mapping with `t(p) = 90 + ceil((p - 60) × 2/3)`:

| Word | Asset frames | Timeline range |
| --- | --- | --- |
| Hello | `[62,70)` | `[92,97)` |
| everyone, | `[72,86)` | `[98,108)` |
| welcome | `[90,100)` | `[110,117)` |
| to | `[100,104)` | `[117,120)` |
| the | `[104,108)` | `[120,122)` |
| channel | `[110,128)` | `[124,136)` |
| today | `[170,182)` capped at 180 | `[164,170)` |

The captions have 4, 2 and 1 words; `W` has 7 words, so each caption takes its own count and keeps its texts. `effective` is `{"retimed": ["itm_c3d4e5f6g7", "itm_h2j3k4m5n6", "itm_s6t7v8w9x0"], "removed": [], "text_reset": [], "unassigned": 0}`.

```json
{"op": "caption.retime_from_clip", "args": {"clip_item_id": "itm_m1n2p3q4r5", "resolved": {"words": [
  {"text": "Hello", "start": 62, "end": 70}, {"text": "everyone,", "start": 72, "end": 86}, {"text": "welcome", "start": 90, "end": 100},
  {"text": "to", "start": 100, "end": 104}, {"text": "the", "start": 104, "end": 108}, {"text": "channel", "start": 110, "end": 128}, {"text": "today", "start": 170, "end": 182}]}}}
```

```json
[
  {"kind": "item.updated", "id": "itm_c3d4e5f6g7", "changes": {"duration": [42, 28], "words": [
    [{"text": "Hello", "start": 0, "duration": 8}, {"text": "everyone,", "start": 10, "duration": 14}, {"text": "Welcome", "start": 28, "duration": 10}, {"text": "to", "start": 38, "duration": 4}],
    [{"text": "Hello", "start": 0, "duration": 5}, {"text": "everyone,", "start": 6, "duration": 10}, {"text": "Welcome", "start": 18, "duration": 7}, {"text": "to", "start": 25, "duration": 3}]]}},
  {"kind": "item.updated", "id": "itm_h2j3k4m5n6", "changes": {"start": [200, 164], "duration": [10, 6], "words": [[{"text": "today", "start": 0, "duration": 10}], [{"text": "today", "start": 0, "duration": 6}]]}},
  {"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"start": [134, 120], "duration": [24, 16], "words": [
    [{"text": "the", "start": 0, "duration": 4}, {"text": "channel", "start": 6, "duration": 18}],
    [{"text": "the", "start": 0, "duration": 2}, {"text": "channel", "start": 4, "duration": 12}]]}}
]
```

**Golden cases.**

- `ok_worked_example_speed_3_2`
- `ok_clip_moved_captions_follow`
- `ok_clip_trimmed_at_start_first_words_dropped_edited_texts_kept_when_counts_match`
- `ok_edited_texts_kept_when_counts_match`
- `ok_caption_receives_fewer_words_takes_transcript_texts_and_is_listed_in_text_reset`
- `ok_caption_receiving_no_word_removed_and_listed_in_removed`
- `ok_removed_caption_restored_by_undo`
- `ok_leftover_transcript_words_counted_not_added`
- `ok_captions_on_two_tracks_each_start_from_first_word`
- `ok_breaks_beyond_new_word_count_dropped`
- `ok_keyframes_cut_when_caption_shorter`
- `ok_unchanged_clip_noop`
- `err_validation_resolved_missing`
- `err_not_found_clip`
- `err_type_mismatch_not_a_clip`
- `err_type_mismatch_asset_without_audio`
- `err_invalid_state_no_linked_caption`
- `err_invalid_state_no_visible_word`
- `err_invalid_state_transition_no_longer_fits`
- `err_overlap_with_other_item`

### C.42 `keyframe.set`

Adds or replaces a keyframe of one property.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | |
| `property` | `"x"`, `"y"`, `"scale"`, `"rotation"`, `"opacity"`, `"volume"`, `"brightness"`, `"contrast"`, `"saturation"`, `"temperature"`, `"tint"`, `"hue"`, `"mask_x"`, `"mask_y"`, `"mask_width"`, `"mask_height"`, `"mask_feather"`, `"crop_left"`, `"crop_top"`, `"crop_right"`, `"crop_bottom"` | yes | none | `volume` for `audio` items and clips of a video (not of an image); `brightness`, `contrast`, `saturation`, `temperature`, `tint` and `hue` (added by TASK-0254 and NEW-15) for clips only; `mask_x`, `mask_y`, `mask_width`, `mask_height` and `mask_feather` (VV-120) for a visual item that has a mask (`clip.set_mask`); `crop_left`, `crop_top`, `crop_right` and `crop_bottom` (VV-120) for clips only; the others for visual items |
| `at` | time | yes | none | absolute; from the item's `start` to `start + duration` inclusive |
| `value` | number | yes | none | valid for the property: `scale` above 0 and at most 20, `opacity` 0 to 1, `volume` 0 to 4, a colour property the range of `clip.set_color` (-1 to 1 for `brightness`, `temperature` and `tint`, 0 to 2 for `contrast` and `saturation`, -180 to 180 for `hue`), a mask or crop property the range of its static field (`mask_x`, `mask_y` 0 to 1, `mask_width`, `mask_height` 0.001 to 1, `mask_feather` 0 to 200, a crop side 0 to 1), `x`, `y`, `rotation` finite |
| `easing` | `"linear"`, `"ease_in"`, `"ease_out"`, `"ease_in_out"`, `"hold"`, `"bezier"` | no | `"linear"` | applies from this keyframe to the next |
| `bezier` | array of 4 numbers | when `easing` is `"bezier"` | none | `[x1, y1, x2, y2]` as in SDD-001 §3.8: `x1`, `x2` from 0 to 1, `y1`, `y2` from -2 to 3. Given with any other easing, or `"bezier"` without it, is `E_VALIDATION` (epic 31) |

**Preconditions.**

1. Arguments match the schema and `at` parses (`E_VALIDATION`, `E_INVALID_TIME`); `bezier` goes with `easing` `"bezier"` and with nothing else.
2. The item exists (`E_NOT_FOUND`).
3. The property applies to the item type (`E_TYPE_MISMATCH`); a `mask_*` property needs the item to have a mask (`E_INVALID_STATE`, hint to use `clip.set_mask`).
4. `at` lies in the inclusive range `start` to `start + duration` (`E_INVALID_TIME`, `path` `args.at`, `details.range`). A keyframe may sit at `start + duration`, because `t` ranges from 0 to `duration`.
5. The property ends with at most 100 keyframes (`E_LIMIT`).

**Postconditions.** Let `t = at - start`.

1. If the property has a keyframe at `t`, it is replaced by `{t, value, easing}`; an omitted `easing` becomes linear (stored without the key). Otherwise `{t, value, easing}` is inserted in order.
2. If the property had no keyframes and `t > 0`, a keyframe `{t: 0, value: <static value by R10>}` is also added first, linear.
3. The static field is not changed. Other properties are untouched.
4. `diff` has one `item.updated` with `keyframes` (whole object). `effective` has `at` (the A.4 triple), `t`, `auto_first_keyframe` and `replaced` (booleans).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad property, easing or value | `args.value must be above 0 and at most 20 for scale, got 0.` | `Use a scale such as 1.2.` |
| `E_INVALID_TIME` | malformed `at`, or `at` outside the item | `args.at "300f" is outside item itm_s6t7v8w9x0; the allowed frames are 30 to 120 inclusive (a keyframe may sit at start + duration).` | `Use an at from "30f" to "120f".` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | property does not apply | `Property opacity does not apply to audio item itm_s6t7v8w9x0.` | `Use property volume for audio items.` |
| `E_LIMIT` | would pass 100 keyframes | `Property scale of itm_s6t7v8w9x0 would have 101 keyframes; the limit is 100.` | `Remove keyframes with keyframe.remove first.` |
| `E_INVALID_STATE` | a `mask_*` property on an item with no mask | `Item itm_s6t7v8w9x0 has no mask to animate.` | `Set one with clip.set_mask first.` |

**Inverse.** `patch {collection: "items", id, set: {keyframes: <old>}}` or `unset: ["keyframes"]`.

**Idempotency.** Same result.

**Example.** 30 fps; text item `itm_s6t7v8w9x0`, `start` 30, `duration` 90, `scale` not set (static 1), no keyframes.

```json
{"op": "keyframe.set", "args": {"item_id": "itm_s6t7v8w9x0", "property": "scale", "at": "2s", "value": 1.2, "easing": "ease_out"}}
```

```json
[{"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"keyframes": [null, {"scale": [{"t": 0, "value": 1}, {"t": 30, "value": 1.2, "easing": "ease_out"}]}]}}]
```

`effective` has `t` 30, `auto_first_keyframe` true, `replaced` false.

**Golden cases.**

- `ok_first_keyframe_at_item_start`
- `ok_first_keyframe_later_adds_static_value_at_0`
- `ok_insert_between_existing`
- `ok_replace_existing_resets_easing_when_omitted`
- `ok_replace_with_easing`
- `ok_at_last_frame_of_item_t_equals_duration`
- `ok_each_property`
- `ok_volume_on_audio_item`
- `ok_x_static_absent_uses_frame_centre`
- `ok_99th_to_100th_keyframe`
- `err_validation_bad_property`
- `err_validation_bad_easing`
- `err_validation_scale_0`
- `err_validation_opacity_1_5`
- `err_invalid_time_no_unit`
- `err_not_found`
- `err_type_mismatch_opacity_on_audio`
- `err_type_mismatch_volume_on_image`
- `ok_colour_property_on_a_clip`
- `ok_colour_property_at_its_limits`
- `err_validation_colour_above_range`
- `err_type_mismatch_colour_on_text`
- `err_type_mismatch_volume_on_text`
- `err_invalid_time_at_before_item`
- `err_invalid_time_at_after_item_end`
- `err_invalid_time_at_one_past_start_plus_duration`
- `err_limit_101_keyframes`
- `ok_mask_x_first_keyframe_keeps_static`
- `ok_mask_width_at_its_limit`
- `ok_mask_feather`
- `err_invalid_state_mask_missing`
- `err_validation_mask_width_zero`
- `err_validation_mask_feather_above_range`
- `ok_crop_left`
- `ok_crop_bottom_on_cropped_clip`
- `err_validation_crop_above_range`
- `err_type_mismatch_crop_on_text`
- `ok_bezier_easing`
- `err_validation_bezier_without_the_easing`
- `err_validation_bezier_easing_without_numbers`
- `err_validation_bezier_x_above_1`

### C.43 `keyframe.remove`

Removes the keyframe of a property at a frame.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | |
| `property` | the properties of `keyframe.set` | yes | none | |
| `at` | time | yes | none | absolute position of the keyframe |

**Preconditions.**

1. Arguments match the schema and `at` parses (`E_VALIDATION`, `E_INVALID_TIME`).
2. The item exists (`E_NOT_FOUND`).
3. The property applies to the item type (`E_TYPE_MISMATCH`).
4. `at` lies in the inclusive range `start` to `start + duration` (`E_INVALID_TIME`, `path` `args.at`).
5. The property has a keyframe at `at - start` (`E_NOT_FOUND`, `details.kind` `keyframe`).

**Postconditions.**

1. The keyframe is removed. If keyframes remain, the static field is unchanged.
2. If it was the last keyframe, the property's keyframe list is removed and the static field becomes the removed keyframe's value (`x` and `y` are stored explicitly; other defaults are omitted). An empty `keyframes` object is removed.
3. `diff` has one `item.updated` with `keyframes` and, in the last-keyframe case, the static field. `effective` has `at` (the A.4 triple) and `remaining` (the number of keyframes left).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad property | `args.property "zoom" is not animatable.` | `Use x, y, scale, rotation, opacity or volume.` |
| `E_INVALID_TIME` | malformed `at`, or `at` outside the item | `args.at "300f" is outside item itm_s6t7v8w9x0; the allowed frames are 30 to 120 inclusive.` | `Use an at from "30f" to "120f".` |
| `E_NOT_FOUND` | unknown item, or no keyframe at that frame | `Property scale of itm_s6t7v8w9x0 has no keyframe at frame 90; keyframes are at frames 30, 60.` | `Use at "60f" (2.000s), or list the keyframes with project_get.` |
| `E_TYPE_MISMATCH` | property does not apply | `Property volume does not apply to text item itm_s6t7v8w9x0.` | `Use opacity, scale, x, y or rotation.` |

**Inverse.** `patch {collection: "items", id, set: {keyframes: <old>, <static field if it changed>: <old>}, unset: <fields that were absent>}`.

**Idempotency.** Error: the second application returns `E_NOT_FOUND` (no keyframe at that frame).

**Example.** 30 fps; text item `itm_s6t7v8w9x0`, `start` 30, `duration` 90, `keyframes.scale` = `[{t:0, value:1}, {t:30, value:1.2, easing:"ease_out"}]`.

```json
{"op": "keyframe.remove", "args": {"item_id": "itm_s6t7v8w9x0", "property": "scale", "at": "2s"}}
```

```json
[{"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"keyframes": [
  {"scale": [{"t": 0, "value": 1}, {"t": 30, "value": 1.2, "easing": "ease_out"}]},
  {"scale": [{"t": 0, "value": 1}]}]}}]
```

**Golden cases.**

- `ok_remove_middle_keyframe`
- `ok_remove_first_keyframe_others_remain`
- `ok_remove_last_remaining_keyframe_writes_static_scale`
- `ok_remove_last_remaining_keyframe_writes_static_x`
- `ok_remove_last_remaining_keyframe_value_equal_to_default_omits_field`
- `ok_other_properties_keep_keyframes`
- `err_validation_bad_property`
- `err_invalid_time_no_unit`
- `err_invalid_time_at_outside_item_message_states_inclusive_range`
- `err_not_found_item`
- `err_not_found_no_keyframe_at_frame`
- `err_not_found_property_has_no_keyframes`
- `err_type_mismatch_volume_on_text`
- `ok_mask_last_keyframe_writes_static`

### C.44 `keyframe.clear`

Removes all keyframes of one property, or of every property, keeping the value at the item's first frame.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | |
| `property` | `"x"`, `"y"`, `"scale"`, `"rotation"`, `"opacity"`, `"volume"` | no | every property | |

**Preconditions.**

1. Arguments match the schema (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).
3. A given `property` applies to the item type (`E_TYPE_MISMATCH`).

**Postconditions.**

1. For each selected property with keyframes `K`: the list is removed and the static field becomes `value_at(K, 0)`, which is the first keyframe's value (R10); defaults are omitted, `x` and `y` are stored explicitly. A property with no keyframes is left alone. An empty `keyframes` object is removed.
2. `diff` has one `item.updated` with `keyframes` and every static field that changed; empty when nothing was cleared (R2). `effective` has `cleared` (the property names).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad property | `args.property "zoom" is not animatable.` | `Use x, y, scale, rotation, opacity or volume.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | property does not apply | `Property volume does not apply to text item itm_s6t7v8w9x0.` | `Use opacity, scale, x, y or rotation.` |

**Inverse.** `patch {collection: "items", id, set: {keyframes: <old>, <static fields>: <old>}, unset: <fields that were absent>}`.

**Idempotency.** Same result: the second application finds nothing to clear.

**Example.** 30 fps; text item `itm_s6t7v8w9x0` with `keyframes.opacity` = `[{t:0, value:0}, {t:12, value:1}]` and no static opacity (default 1).

```json
{"op": "keyframe.clear", "args": {"item_id": "itm_s6t7v8w9x0", "property": "opacity"}}
```

```json
[{"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"keyframes": [{"opacity": [{"t": 0, "value": 0}, {"t": 12, "value": 1}]}, null], "opacity": [1, 0]}}]
```

**Golden cases.**

- `ok_clear_one_property`
- `ok_clear_all_properties`
- `ok_first_keyframe_not_at_frame_0_its_value_still_used`
- `ok_value_equal_to_default_omits_static_field`
- `ok_x_written_explicitly`
- `ok_property_without_keyframes_noop`
- `ok_item_without_keyframes_noop`
- `err_validation_bad_property`
- `err_not_found`
- `err_type_mismatch_volume_on_text`
- `ok_colour_property_returns_to_static`
- `ok_crop_returns_to_static`

### C.45 `slot.mark`

Marks an item as a template slot; the slot kind follows from the item type.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip`, `audio`, `text` or `caption` item |
| `name` | string | yes | none | matches `[a-z][a-z0-9_]{0,39}`; unique among the document's slots |
| `description` | string | no | `""` | up to 200 characters |
| `required` | boolean | no | `true` | |
| `duration_policy` | `"fit"` or `"stretch_timeline"` | no | `"fit"` | media slots only (`E_VALIDATION` otherwise) |

**Preconditions.**

1. Arguments match the schema (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).
3. The item type is `clip`, `audio`, `text` or `caption` (`E_TYPE_MISMATCH`).
4. No other item has a slot with that `name` (`E_SLOT`, `details.reason` `"name_in_use"`, `details.item` the id of the other item).
5. The item has no slot, or already has a slot with the same `name`: re-marking the same item with the same name updates the slot. An item that already has a slot with another name is `E_INVALID_STATE`, and the hint names `slot.unmark`.

**Postconditions.**

1. The item's `slot` is `{name, kind, description, required, duration_policy}` with `kind` `media` (for `clip` and `audio`), `text` or `caption`; defaults are omitted from the stored object (`kind` and `name` always stay). `filled` is false, or unchanged when updating an existing slot of the same name. The item's content, including its `asset`, is not changed.
2. `diff` has one `item.updated` with `slot`; empty when unchanged. `effective` has the stored `slot`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad `name`, over-long description, `duration_policy` on a non-media slot | `args.name "Title" does not match [a-z][a-z0-9_]{0,39}.` | `Use "title".` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | shape item | `Item itm_s6t7v8w9x0 is a shape; slots are for media, text and captions.` | `Mark a clip, audio, text or caption item.` |
| `E_SLOT` | name used by another item's slot (`details.reason` `"name_in_use"`) | `Slot name "title" is already used by item itm_m1n2p3q4r5.` | `Use another name, or unmark the other slot first.` |
| `E_INVALID_STATE` | the item already has a slot with another name | `Item itm_s6t7v8w9x0 is already slot "headline".` | `Use slot.unmark first, or reuse the name "headline" to update it.` |

**Inverse.** `patch {collection: "items", id, set: {slot: <old>}}` or `unset: ["slot"]`.

**Idempotency.** Same result: marking again with the same name and values changes nothing.

**Example.** Text item `itm_s6t7v8w9x0`, not a slot.

```json
{"op": "slot.mark", "args": {"item_id": "itm_s6t7v8w9x0", "name": "title", "description": "Headline"}}
```

```json
[{"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"slot": [null, {"name": "title", "kind": "text", "description": "Headline"}]}}]
```

**Golden cases.**

- `ok_text_slot`
- `ok_caption_slot`
- `ok_clip_slot_kind_media`
- `ok_audio_slot_kind_media`
- `ok_media_slot_stretch_timeline`
- `ok_required_false`
- `ok_remark_same_name_updates_fields_keeps_filled`
- `ok_name_40_chars`
- `err_validation_uppercase_name`
- `err_validation_name_41_chars`
- `err_validation_description_201_chars`
- `err_validation_duration_policy_on_text_slot`
- `err_not_found`
- `err_type_mismatch_shape_item`
- `err_slot_name_used_by_other_item`
- `err_invalid_state_item_already_has_other_slot`

### C.46 `slot.unmark`

Removes the slot marking of an item.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | |

**Preconditions.**

1. The item exists (`E_NOT_FOUND`).
2. The item has a slot (`E_SLOT`, `details.item`).
3. The item is not an unfilled media slot still holding `"ast_placeholder"`, which would leave an invalid asset reference (`E_INVALID_STATE`).

**Postconditions.**

1. `slot` is removed. Nothing else changes.
2. `diff` has one `item.updated` with `slot`. `effective` is `{"removed_slot": "<name>"}`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_SLOT` | no slot on the item | `Item itm_s6t7v8w9x0 is not a slot.` | `Mark it with slot.mark, or pick an item that is a slot.` |
| `E_INVALID_STATE` | unfilled media slot with the placeholder asset | `Slot "hero" on itm_m1n2p3q4r5 is unfilled and still uses ast_placeholder.` | `Fill it with slot.fill, or delete the item.` |

**Inverse.** `patch {collection: "items", id, set: {slot: <old>}}`.

**Idempotency.** Error: the second application returns `E_SLOT`.

**Example.** Text item `itm_s6t7v8w9x0` marked as slot `title`.

```json
{"op": "slot.unmark", "args": {"item_id": "itm_s6t7v8w9x0"}}
```

```json
[{"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"slot": [{"name": "title", "kind": "text"}, null]}}]
```

**Golden cases.**

- `ok_unmark_text_slot`
- `ok_unmark_filled_media_slot`
- `ok_unmark_unfilled_text_slot`
- `err_not_found`
- `err_slot_item_is_not_a_slot`
- `err_invalid_state_unfilled_media_slot_with_placeholder`

### C.47 `slot.fill`

Fills a template slot with media, text or caption words.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `name` | string | yes | none | the slot name |
| `asset_id` | asset id | one of three | none | for a `media` slot; the reserved literal `ast_placeholder` passes the schema (R6) and is then `E_NOT_FOUND`, because it never matches a real asset |
| `source_in` | time | no | `"0s"` | with `asset_id` only (`E_VALIDATION` otherwise) |
| `text` | string | one of three | none | for a `text` slot; 1 to 2,000 characters |
| `words` | array of `{text, start, duration}` | one of three | none | for a `caption` slot; R16 |

**Preconditions.**

1. Exactly one of `asset_id`, `text`, `words`; `source_in` only with `asset_id`; values valid (`E_VALIDATION`, `E_INVALID_TIME`).
2. A slot with that `name` exists (`E_SLOT`, `details.available` lists the names).
3. The argument matches the slot kind (`asset_id` for `media`, `text` for `text`, `words` for `caption`) and the slot is not yet filled (`E_SLOT`).
4. Media: the asset exists (`E_NOT_FOUND`) and suits the item: a `clip` takes a video or image asset, an `audio` item an audio asset or a video with audio (`E_TYPE_MISMATCH`).
5. Media, `duration_policy` `fit`, not an image: the source range holds with the item's current duration (R9, `E_OUT_OF_SOURCE`). Policy `stretch_timeline`, not an image: `maxdur(source_in)` is at least 1 (`E_OUT_OF_SOURCE`).
6. Caption words: the caption has at most 60 words, and the document has at most 5,000 caption words after the fill (`E_LIMIT`).
7. Resizing checks of R18 (`E_OVERLAP`, `E_INVALID_TIME`, `E_INVALID_STATE`), for `stretch_timeline` and for caption words that change the duration.

**Postconditions.**

1. Media: `asset` is `asset_id` and `source_in` is the given value (stored when above 0, removed when 0). Policy `fit`: `duration` is unchanged. Policy `stretch_timeline`: `duration` becomes `maxdur(source_in) = floor((A - source_in) × den / num)` for video and audio, and stays the same for an image. With `delta = new duration - old duration` and `E` the old end, every item of every track with `start >= E` moves by `delta`, and the filled item follows R18 (keyframes cut by R12 when it shrinks).
2. Text: `text` is replaced; nothing else changes.
3. Caption words: `words` replace the item's words, `breaks` is cleared, and `duration` becomes the end of the last word, then R18.
4. In every case `slot.filled` is true. Other slots are untouched.
5. `diff`: `item.updated` for the filled item and for every shifted item, sorted by id. `effective` has `slot` (the name), `kind`, `source_in` (the A.4 triple, media slots only; `requested` is `null` when the default `"0s"` applies), `duration` (`{"before", "after"}`) and `shifted` (`[{"id", "from", "to"}]`).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | none or several of the three; `source_in` without `asset_id` | `args: give exactly one of asset_id, text, words.` | `For slot "hero" (media) send asset_id.` |
| `E_INVALID_TIME` | malformed time; resize past 108,000 | `args.source_in "30" has no unit.` | `Use "30f" or "1s".` |
| `E_SLOT` | unknown name, wrong kind, already filled | `Slot "hero" is a media slot; it needs asset_id, not text.` | `Send asset_id for slot "hero".` |
| `E_NOT_FOUND` | unknown asset, including `ast_placeholder` | `Asset ast_zzzzzzzzzz does not exist.` | `Import the media first, then use its asset id.` |
| `E_LIMIT` | 61 words in the caption, or more than 5,000 caption words in the document | `The document would have 5004 caption words; the limit is 5000.` | `Remove captions you no longer need, or send fewer words.` |
| `E_TYPE_MISMATCH` | asset kind does not suit the item | `Asset ast_9m8n7p6q5r is an image; slot "voice" holds an audio item.` | `Use an audio asset.` |
| `E_OUT_OF_SOURCE` | the asset is too short (`fit`) | `Slot "hero" needs 90 frames from source frame 20; asset ast_4h6j8k2m3n has 100 frames, so 80 are available.` | `Use an asset of at least 110 frames, a smaller source_in, or set duration_policy to stretch_timeline.` |
| `E_OVERLAP` | a shift or growth overlaps | `Item itm_s6t7v8w9x0 would occupy frames 50-109 on track trk_a1b2c3d4e5, where item itm_c3d4e5f6g7 already is (frames 30-149).` | `Use duration_policy "fit", or move itm_c3d4e5f6g7.` |
| `E_INVALID_STATE` | fades or transitions no longer fit | `fade_in 40 frames plus fade_out 40 frames exceeds the new duration 60 of itm_m1n2p3q4r5.` | `Shorten the fades with item.set_audio first.` |

**Inverse.** `patch {collection: "items", id, set: <old values of every changed field, including slot>, unset: <fields that were absent>}` for the filled item, then one `patch {... set: {start: <old>}}` per shifted item, in the order of mutation (R5). Undo applies them to a working copy and validates once at the end (A.6).

**Idempotency.** Error: the second application returns `E_SLOT` because the slot is filled.

**Example.** 30 fps template. Asset `ast_4h6j8k2m3n` is a video of 300 frames at 30 fps. Clip `itm_m1n2p3q4r5` on `trk_f6g7h8j9k0`: `start` 0, `duration` 90, `asset` `ast_placeholder`, speed 1, slot `{name: "hero", kind: "media", description: "Main footage", duration_policy: "stretch_timeline"}`. Text `itm_s6t7v8w9x0` on `trk_a1b2c3d4e5`: `start` 90, `duration` 60. Text `itm_c3d4e5f6g7` on `trk_m1n2p3q4r5`: `start` 30, `duration` 120.

Fill with `source_in` `"1s"` (30 frames): `A = 300`, so the new duration is `300 - 30 = 270`; `delta = 270 - 90 = 180`; the old end `E` is 90. `itm_s6t7v8w9x0` starts at 90, which is at or after `E`, so it moves to 270. `itm_c3d4e5f6g7` starts at 30, before `E`, so it does not move. `effective.source_in` is `{"frame": 30, "seconds": 1, "requested": "1s"}`.

```json
{"op": "slot.fill", "args": {"name": "hero", "asset_id": "ast_4h6j8k2m3n", "source_in": "1s"}}
```

```json
[
  {"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {
    "asset": ["ast_placeholder", "ast_4h6j8k2m3n"], "source_in": [null, 30], "duration": [90, 270],
    "slot": [{"name": "hero", "kind": "media", "description": "Main footage", "duration_policy": "stretch_timeline"},
             {"name": "hero", "kind": "media", "description": "Main footage", "duration_policy": "stretch_timeline", "filled": true}]}},
  {"kind": "item.updated", "id": "itm_s6t7v8w9x0", "changes": {"start": [90, 270]}}
]
```

With `duration_policy` `fit` instead, an asset of 100 frames and the same `source_in` of 20 frames would fail: `20 + ceil(90 × 1) = 110 > 100`, so `E_OUT_OF_SOURCE` with `details.available.max_duration` 80.

**Golden cases.**

- `ok_media_fit_asset_long_enough`
- `ok_media_fit_asset_exactly_long_enough`
- `ok_media_fit_with_source_in`
- `ok_media_fit_image_asset_no_source_check`
- `ok_media_fit_speed_3_2`
- `ok_media_stretch_longer_shifts_later_items_on_all_tracks`
- `ok_media_stretch_shorter_shifts_earlier_and_cuts_keyframes`
- `ok_media_stretch_image_keeps_duration`
- `ok_media_audio_slot_with_video_asset_with_audio`
- `ok_text_slot`
- `ok_caption_slot_duration_follows_last_word`
- `ok_other_slots_untouched`
- `err_validation_none_of_three`
- `err_validation_two_of_three`
- `err_validation_source_in_without_asset_id`
- `err_invalid_time_source_in_no_unit`
- `err_invalid_time_stretch_past_108000`
- `err_slot_unknown_name`
- `err_slot_wrong_kind_text_for_media`
- `err_slot_wrong_kind_asset_for_text`
- `err_slot_already_filled`
- `err_not_found_asset`
- `err_not_found_placeholder_asset`
- `err_type_mismatch_image_for_audio_slot`
- `err_type_mismatch_audio_asset_for_clip_slot`
- `err_out_of_source_fit_asset_too_short`
- `err_out_of_source_stretch_source_in_at_asset_end`
- `err_overlap_stretch_shorter_shift_collides`
- `err_overlap_caption_words_longer_than_gap`
- `err_limit_61_caption_words`
- `err_limit_5000_caption_words`
- `err_invalid_state_fades_no_longer_fit`

### C.48 `template.set_info`

Sets the template description and its named colours.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `description` | string | no | unchanged | 0 to 500 characters |
| `colors` | object: name to colour or `null` | no | unchanged | names match `[a-z][a-z0-9_]{0,39}`; a colour is `#RRGGBB` or `#RRGGBBAA`, stored upper case; `null` removes the name |

At least one of `description` and `colors` is required.

**Preconditions.**

1. At least one argument is present and valid (`E_VALIDATION`).
2. No removed colour is referenced by `"$name"` anywhere in the document: text and caption styles of items, track `caption_style`, shape `fill` and `stroke_color`, `settings.background` (`E_IN_USE`, `details.used_by` lists the item and track ids, and `"settings"`).

**Postconditions.**

1. If the document has no `template`, it gets `{"description": "", "colors": {}}` first. `description` is set when given. For each entry of `colors`: a colour sets or replaces the name; `null` removes it, and removing a name that does not exist is ignored.
2. When the document had no `template` and the command changes nothing (only removals of absent names), nothing is added (R2).
3. `diff` has one `project.updated` with `template` as a whole object (R4). `effective` is `{"template": <stored template>}`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | nothing to set, bad name, bad colour, description over 500 characters | `args.colors "Brand" is not a valid name; it must match [a-z][a-z0-9_]{0,39}.` | `Use "brand".` |
| `E_IN_USE` | a removed colour is referenced | `Colour "brand" is used by itm_s6t7v8w9x0 (style.color) and trk_a1b2c3d4e5 (caption_style.highlight_color).` | `Change those fields to a fixed colour with text.set and track.set_caption_style, then remove "brand".` |

**Inverse.** `patch {collection: "project", id: <project id>, set: {template: <old>}}` or `unset: ["template"]` when there was none.

**Idempotency.** Same result.

**Example.** Template document with `template` `{"description": "", "colors": {"old": "#112233"}}`; no style refers to `old`.

```json
{"op": "template.set_info", "args": {"description": "Product teaser", "colors": {"brand": "#ff3366", "old": null}}}
```

```json
[{"kind": "project.updated", "id": "prj_7k2m9x4q1b", "changes": {"template": [
  {"description": "", "colors": {"old": "#112233"}},
  {"description": "Product teaser", "colors": {"brand": "#FF3366"}}]}}]
```

**Golden cases.**

- `ok_description_only`
- `ok_add_colors_creates_template_object`
- `ok_replace_colour_value`
- `ok_colour_uppercased`
- `ok_remove_unused_colour`
- `ok_remove_absent_name_noop`
- `ok_description_500_chars`
- `ok_instance_flag_kept`
- `ok_same_values_noop`
- `err_validation_nothing_to_set`
- `err_validation_bad_colour_name`
- `err_validation_bad_colour_value`
- `err_validation_description_501_chars`
- `err_in_use_text_style`
- `err_in_use_track_caption_style`
- `err_in_use_settings_background`

### C.49 `clip.set_filter`

Sets the colour filter of a visual item (RF-027, SDD-001 §3.4; text, caption and shape items since VV-025, the filter applies to every visual item). The filters are a fixed list implemented in Go by the compositor (SDD-004 §3.6); there is no way to name another one.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip`, `text`, `caption` or `shape` item |
| `filter` | string | no | unchanged | `none`, `grayscale`, `sepia`, `invert`, `warm` or `cool` |
| `amount` | number | no | unchanged | 0 to 1; how much of the filter is mixed in |

At least one of `filter` and `amount` is required.

**Preconditions.**

1. At least one is present, and each given value is valid (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).
3. The item is not an `audio` item (`E_TYPE_MISMATCH`).

**Postconditions.**

1. Given values are set, `amount` rounded to 3 decimals; values equal to the default (`none`, 1) remove the field. Setting `filter` to `none` keeps `filter_amount` as it was.
2. `diff` has one `item.updated`; empty when unchanged. `effective` has `filter` and `amount` as stored.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | nothing to set, an unknown filter, or `amount` out of range | `args.filter is "sharpen", which is not one of the accepted values.` | `Use one of: none, grayscale, sepia, invert, warm, cool.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | an audio item | `Item itm_s6t7v8w9x0 is an audio item; filter applies to visual items.` | `Use item.set_audio for the sound of an audio item.` |

**Inverse.** `patch {collection: "items", id, set: <old values>, unset: <fields that were absent>}`.

**Idempotency.** Same result.

**Example.** Clip `itm_m1n2p3q4r5` with no filter.

```json
{"op": "clip.set_filter", "args": {"item_id": "itm_m1n2p3q4r5", "filter": "sepia", "amount": 0.6}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"filter": ["none", "sepia"], "filter_amount": [1, 0.6]}}]
```

**Golden cases.**

- `ok_filter_only`
- `ok_amount_only`
- `ok_filter_and_amount`
- `ok_each_filter`
- `ok_boundaries_amount_0_1`
- `ok_default_values_remove_fields`
- `ok_none_keeps_amount`
- `ok_rounded_to_3_decimals`
- `ok_image_clip`
- `ok_text_item`
- `ok_shape_item`
- `ok_same_values_noop`
- `err_validation_no_argument`
- `err_validation_unknown_filter`
- `err_validation_amount_above_1`
- `err_validation_amount_negative`
- `err_not_found`
- `err_type_mismatch_audio_item`

### C.50 `clip.set_blur`

Sets the blur of a visual item (RF-027, SDD-001 §3.4; text, caption and shape items since VV-025).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip`, `text`, `caption` or `shape` item |
| `radius` | number | yes | none | 0 to 100, pixels at project resolution |

**Preconditions.**

1. `radius` is present and in range (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).
3. The item is not an `audio` item (`E_TYPE_MISMATCH`).

**Postconditions.**

1. `blur` is `radius` rounded to 3 decimals; 0 removes the field.
2. `diff` has one `item.updated`; empty when unchanged. `effective` has `blur`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `radius` missing or out of range | `The value of args.radius is 120, outside the accepted range.` | `Use a value between 0 and 100.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | an audio item | `Item itm_s6t7v8w9x0 is an audio item; blur applies to visual items.` | `Use item.set_audio for the sound of an audio item.` |

**Inverse.** `patch {collection: "items", id, set: <old value>, unset: <field if it was absent>}`.

**Idempotency.** Same result.

**Example.** Clip `itm_m1n2p3q4r5` with no blur.

```json
{"op": "clip.set_blur", "args": {"item_id": "itm_m1n2p3q4r5", "radius": 12.5}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"blur": [null, 12.5]}}]
```

**Golden cases.**

- `ok_radius`
- `ok_boundaries_0_100`
- `ok_zero_removes_field`
- `ok_rounded_to_3_decimals`
- `ok_image_clip`
- `ok_text_item`
- `ok_caption_item`
- `ok_same_value_noop`
- `err_validation_no_argument`
- `err_validation_radius_above_100`
- `err_validation_radius_negative`
- `err_not_found`
- `err_type_mismatch_audio_item`

### C.51 `audio.duck`

Lowers a music item while speech sounds (RF-043). The result is ordinary `volume` keyframes (SDD-001 section 3.8), so nothing about the item needs a new field and `keyframe.clear` on `volume` undoes the ducking. The speech comes from the speech map of the speech items' assets, supplied by the service in `resolved` (A.9); the command never starts a job.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | an `audio` item whose asset has audio, with no `volume` keyframes |
| `speech_item_ids` | array of item ids | yes | none | 1 to 20 `clip` or `audio` items whose assets have audio, each once, none of them `item_id` |
| `gain_db` | number | no | -12 | -40 to -1; the change of volume while speech sounds |
| `attack` | time, as a length | no | `"0.2s"` | may be 0; the ramp down that ends where speech starts |
| `release` | time, as a length | no | `"0.5s"` | may be 0; the ramp up that starts where speech ends |
| `resolved` | object | filled by the service | none | `{"speech": [{"item_id", "ranges": [{"start", "end"}]}]}`: one entry for each id of `speech_item_ids`, with sorted, non-overlapping, half-open ranges in asset frames. Missing is `E_VALIDATION` |

**Preconditions.**

1. The arguments are valid and `resolved` holds one entry for each speech item (`E_VALIDATION`); `attack` and `release` are lengths (`E_INVALID_TIME`).
2. The music item and every speech item exist (`E_NOT_FOUND`).
3. The music item is an audio item with audio, and the speech items are clips or audio items with audio (`E_TYPE_MISMATCH`).
4. The music item has no `volume` keyframes (`E_INVALID_STATE`, hint: run `keyframe.clear` on `volume` first).
5. The keyframes fit the limit of 100 per property (`E_LIMIT`; the command never truncates).

**Postconditions.**

1. Each speech range `[s, e)` in asset frames of a speech item `P` becomes timeline frames `[P.start + floor((s - P.source_in) / speed), P.start + ceil((e - P.source_in) / speed))`, clipped to `P` and to the music item, then made relative to the music item's start. The ranges of all speech items are sorted, and ranges whose ramps would touch (the next ramp starts at or before the end of the previous one) are joined.
2. `low = round3(volume x 10^(gain_db / 20))` where `volume` is the static volume of the music item. The `volume` keyframes are: frame 0 at `volume`; for each stretch `[s, e)`: `volume` at `max(s - attack, 0)`, `low` at `max(s, that + 1)` and, when `e` is before the end of the item, `low` at `e` and `volume` at `min(e + max(release, 1), duration)`. Two keyframes on one frame keep the later. Easing is `linear`.
3. With no speech inside the music item the document is unchanged. The static `volume` field is never changed.
4. `diff` has one `item.updated` (`keyframes`). `effective` has `gain_db`, `attack` and `release` (time form), `ducked`, `low_volume` and `keyframes` (their count).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `resolved` missing or with a missing, repeated or unknown entry or bad ranges, `gain_db` out of range, a speech item that is the music or repeated | `args.resolved is missing: the speech of the items is not known.` | `The service fills it from the speech analysis; run the analysis of the speech assets first, then retry.` |
| `E_INVALID_TIME` | `attack` or `release` is not a time | `args.attack "later" is not a time.` | `Use "2s" or "60f".` |
| `E_NOT_FOUND` | the music or a speech item does not exist | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | the music is not an audio item, or an item has no asset with audio | `Item itm_m1n2p3q4r5 is a clip item, not an audio item; only music can be ducked.` | `Choose an audio item.` |
| `E_INVALID_STATE` | the music item already has `volume` keyframes | `Item itm_c3d4e5f6g7 already has 2 volume keyframes.` | `Run keyframe.clear on volume first, then duck again.` |
| `E_LIMIT` | more than 100 keyframes needed | `Ducking itm_c3d4e5f6g7 would need 160 volume keyframes; the limit is 100.` | `Duck a shorter item, split it, or use longer ramps so that nearby speech merges.` |

**Inverse.** `patch {collection: "items", id, unset: ["keyframes"]}` (or the old `keyframes` when the item had keyframes on other properties).

**Idempotency.** Not idempotent: a second call is `E_INVALID_STATE` until `keyframe.clear` on `volume` runs. After it, the same call gives the same result.

**Example.** Music `itm_c3d4e5f6g7` (300 frames), speech from 2 s to 4 s at 30 fps, defaults.

```json
{"op": "audio.duck", "args": {"item_id": "itm_c3d4e5f6g7", "speech_item_ids": ["itm_m1n2p3q4r5"]}}
```

The volume keyframes are `(0, 1)`, `(54, 1)`, `(60, 0.251)`, `(120, 0.251)`, `(135, 1)`: at 3 s (frame 90) the gain is 0.251 and at 0 s and 6 s it is 1.

**Golden cases.**

- `ok_one_stretch_defaults`
- `ok_gain_attack_release_given`
- `ok_zero_attack_and_release`
- `ok_two_stretches_apart`
- `ok_close_stretches_held_low`
- `ok_speech_at_start`
- `ok_speech_to_the_end`
- `ok_music_volume_scaled`
- `ok_no_speech_leaves_document`
- `ok_speech_outside_the_music_ignored`
- `ok_speech_item_trimmed_and_sped_up`
- `ok_two_speech_items`
- `ok_clip_with_audio_as_music_source`
- `err_validation_resolved_missing`
- `err_validation_speech_entry_missing`
- `err_validation_speech_entry_for_other_item`
- `err_validation_ranges_overlap`
- `err_validation_gain_not_below_0`
- `err_validation_speech_is_the_music`
- `err_invalid_time_attack`
- `err_not_found`
- `err_not_found_speech_item`
- `err_type_mismatch_music_is_a_clip`
- `err_type_mismatch_speech_is_a_text`
- `err_type_mismatch_speech_asset_without_audio`
- `err_invalid_state_music_already_animated`
- `err_limit_over_100_keyframes`

Also tested outside the golden directories: `TestDuckThenClearRestoresTheDocument` (ducking and then `keyframe.clear` on `volume` gives the original document byte for byte) and `TestDuckGainAtFrames` (`timeline.ValueAt` at 0 s, 3 s and 6 s).

### C.52 `audio.mark_beats`

Writes markers on a music item at the beats of its audio (RF-044), so that cuts and transitions can be aligned to them. The beats come from the beat grid of the asset, supplied by the service in `resolved` (A.9).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | an `audio` item whose asset has audio |
| `every` | integer | no | 1 | 1 to 64; marks the 1st, the (1 + every)th, ... beat of the visible part |
| `label` | string | no | none | up to 60 characters; the label of every marker |
| `replace` | boolean | no | true | true replaces the markers of the item; false keeps them and adds the beats (at a frame that has a marker, the existing one stays) |
| `bpm` | number | no | none | 20 to 400 beats per minute (VV-153). The manual tempo for music whose pulse the analysis cannot find: the beat grid is then `offset + k × 60 / bpm` seconds, `k = 0, 1, ...`, each rounded to the nearest frame, up to the end of the visible source range; `resolved` is ignored |
| `offset` | time, as a length (may be 0) | no | `"0s"` | only with `bpm` (`E_VALIDATION` without it): the time of the first beat on the asset's own time axis, from its start |
| `resolved` | object | filled by the service | none | `{"beats": [frame, ...]}`: strictly increasing asset frames from 0, at most 108,000. Missing is `E_VALIDATION` unless `bpm` is given |

**Preconditions.**

1. The arguments are valid and, without `bpm`, `resolved` is present with increasing beats (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`) and is an audio item with audio (`E_TYPE_MISMATCH`); these are checked before `resolved`.
3. The beat grid (the detected one, or the manual one of `bpm`) is not empty and at least one beat lies inside the visible part of the item (`E_INVALID_STATE`).
4. The markers fit the limit of 200 per item (`E_LIMIT`; the command never truncates).

**Postconditions.**

1. A beat `b` of the asset at or after `source_in` is at item frame `ceil((b - source_in) / speed)`; it counts when that frame is before `duration`. Beats that fall on a frame already taken keep the first.
2. Every `every`th of those frames, from the first, gets a marker `{frame, label}`.
3. `diff` has one `item.updated` (`markers`). `effective` has `beats` (markers written by this call) and `markers` (the markers the item has now).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `resolved` missing, beats not increasing, `every` or `label` out of range | `The beat grid of the asset is not known.` | `Wait until the asset is analysed (media_inspect shows its state), then retry.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not an audio item, or the asset has no audio | `Item itm_m1n2p3q4r5 is a clip item, not an audio item; beats are marked on music.` | `Choose an audio item.` |
| `E_INVALID_STATE` | the beat grid is empty (the asset is not rhythmic music, a sound effect for instance), or no beat lies in the visible part | `The beat grid has no beat inside the visible part of item itm_c3d4e5f6g7.` | `Check that the item holds music, or widen it with item.trim.` |
| `E_LIMIT` | more than 200 markers | `Item itm_c3d4e5f6g7 would have 220 markers; the limit is 200.` | `Use a larger every, or trim the item first.` |

**Inverse.** `patch {collection: "items", id, set: {markers: <old>}}`, or `unset: ["markers"]` when there were none.

**Idempotency.** Same result with the default `replace`; with `replace: false` a repeated call changes nothing (the frames are taken).

**Example.** Music `itm_c3d4e5f6g7` at frame 30 for 120 frames from asset frame 60, a beat every 15 frames, `every` 2.

```json
{"op": "audio.mark_beats", "args": {"item_id": "itm_c3d4e5f6g7", "every": 2, "resolved": {"beats": [0, 15, 30, 45, 60, 75, 90, 105, 120, 135, 150, 165, 180, 195, 210]}}}
```

Markers at item frames 0, 30, 60 and 90.

**Golden cases.**

- `ok_every_beat_of_the_visible_part`
- `ok_every_second_beat`
- `ok_label_given`
- `ok_replaces_existing_markers`
- `ok_keeps_existing_markers`
- `ok_speed_2_beats_on_the_same_frame_once`
- `ok_beat_on_the_last_visible_frame`
- `ok_video_asset_audio`
- `ok_manual_tempo_with_offset`
- `ok_manual_tempo_when_the_grid_is_empty`
- `err_validation_resolved_missing`
- `err_validation_beats_not_increasing`
- `err_validation_beat_negative`
- `err_validation_offset_without_bpm`
- `err_validation_bpm_10`
- `err_validation_every_0`
- `err_validation_every_65`
- `err_validation_label_over_60_characters`
- `err_not_found`
- `err_type_mismatch_clip`
- `err_type_mismatch_asset_without_audio`
- `err_invalid_state_empty_beat_grid`
- `err_invalid_state_no_beat_in_the_visible_part`
- `err_limit_over_200_markers`

### C.53 `font.add`

Names a font that is already stored by hash in the workspace as a family of the project (RF-053), so that styles can use it (SDD-001 section 3.9). The core never reads or stores a file: it checks the form of the arguments and edits the `fonts` map. The service refuses, before applying, a `hash` that the workspace font store does not hold (`E_NOT_FOUND`, hint to import the font first).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `family` | string | yes | none | 1 to 60 characters from letters, digits, space, dot, hyphen and underscore, starting with a letter or digit; not a bundled family |
| `files` | array of `{weight, hash}` | yes | none | 1 to 9 entries; `weight` is 100 to 900 in steps of 100 and appears once; `hash` is `sha256:` and 64 lower-case hex characters |

**Preconditions.**

1. Every argument is valid (`E_VALIDATION` with the path, for example `args.files[1].weight`).

**Postconditions.**

1. A new family gets the given files, sorted by weight. For a family that exists, each given weight is added, and a weight that exists takes the new hash; the other weights stay.
2. `diff` has `font.added`, or `font.removed` then `font.added` when the family existed and changed; it is empty when nothing changed. `effective` has `family` and `weights` (all of the family's weights, ascending).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad family name, a bundled family, no file, a bad or repeated weight, a bad hash | `args.family "Inter" is a bundled family.` | `Choose another name; the bundled families are: Anton, Bebas Neue, Inter, JetBrains Mono, Montserrat, Playfair Display.` |
| `E_NOT_FOUND` | a `hash` the workspace font store does not hold (raised by the service before Apply, so no golden case; `web/app/servicetest/fonts_test.go` covers it) | `a font file of the operation is not in the workspace.` | `import the font file first, then add the family` |

**Inverse.** `remove fonts <family>` for a new family; `put fonts <family>` with the old object otherwise.

**Idempotency.** Same result.

**Example.**

```json
{"op": "font.add", "args": {"family": "Brand Sans", "files": [{"weight": 400, "hash": "sha256:abababababababababababababababababababababababababababababababab"}, {"weight": 700, "hash": "sha256:cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd"}]}}
```

**Golden cases.**

- `ok_new_family_two_weights`
- `ok_one_file`
- `ok_family_name_with_space_dot_and_hyphen`
- `ok_adds_a_weight_to_a_family`
- `ok_replaces_the_file_of_a_weight`
- `ok_same_file_again_changes_nothing`
- `err_validation_bundled_family`
- `err_validation_family_empty`
- `err_validation_family_bad_characters`
- `err_validation_family_starts_with_space`
- `err_validation_family_over_60_characters`
- `err_validation_no_files`
- `err_validation_weight_not_a_multiple_of_100`
- `err_validation_weight_above_900`
- `err_validation_weight_twice`
- `err_validation_hash_malformed`
- `err_validation_hash_upper_case`

Also tested outside the golden directories: `TestFontAddThenStyleInOneBatch` (a `text.add` whose style names a family added earlier in the same batch validates; the reverse order is `E_FONT_UNKNOWN`).

### C.54 `font.remove`

Removes a user family from the `fonts` map. The files stay in the workspace store (nothing in the core deletes a file).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `family` | string | yes | none | a key of `fonts` |

**Preconditions.**

1. The family is not a bundled one (`E_VALIDATION`) and is a key of `fonts` (`E_NOT_FOUND`, the hint lists the project fonts).
2. No text or caption item style and no track `caption_style` has this `font_family` (`E_IN_USE`, `details.used_by` lists the item and track ids, sorted).

**Postconditions.**

1. The family is removed; an empty `fonts` map is left out of the document.
2. `diff` has one `font.removed`. `effective` has `family`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | the family is bundled | `Font family "Inter" is bundled and cannot be removed.` | `Only the families added with font.add can be removed.` |
| `E_NOT_FOUND` | not a project font | `Font family "Nowhere" is not one of the project fonts.` | `Use one of: Brand Sans, Spare.` |
| `E_IN_USE` | a style uses it | `Font family "Brand Sans" is used by itm_s6t7v8w9x0 (style.font_family).` | `Change those styles to another font_family with text.set, caption.set_style and track.set_caption_style, then remove "Brand Sans".` |

**Inverse.** `put fonts <family>` with the old object.

**Idempotency.** Not idempotent: the second call is `E_NOT_FOUND`.

**Example.**

```json
{"op": "font.remove", "args": {"family": "Brand Sans"}}
```

**Golden cases.**

- `ok_remove_an_unused_family`
- `ok_remove_the_last_family`
- `err_not_found_family`
- `err_validation_bundled_family`
- `err_in_use_text_style`
- `err_in_use_caption_style`
- `err_in_use_track_caption_style`
- `err_in_use_lists_every_user`

### C.55 `clip.set_chroma_key`

Sets the chroma key of a clip (VV-015, SDD-001 §3.4): the colour that becomes transparent, for a green screen or a clip on a plain background. The compositor draws it (SDD-004 §3.7).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip` |
| `color` | colour or null | yes | none | `#RRGGBB`, `#RRGGBBAA` or a `$name` of `template.colors`; null removes the key |
| `similarity` | number | no | current value, else 0.3 | 0 to 1; how far from `color` a pixel is still removed |
| `smoothness` | number | no | current value, else 0.1 | 0 to 1; the soft edge beyond `similarity` |
| `spill` | number | no | current value, else 0 | 0 to 1; how much of `color` is taken out of the pixels that stay |

**Preconditions.**

1. `color` is present and is a colour or null; each given number is in range; with `color` null none of the numbers is given (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).
3. The item is a `clip` (`E_TYPE_MISMATCH`).

**Postconditions.**

1. `chroma_key` is set with the four values, the numbers rounded to 3 decimals; null removes the field.
2. `diff` has one `item.updated`; empty when unchanged. `effective.chroma_key` is the object as stored, or null.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `color` missing or not a colour, a number out of range, or a number given with `color` null | `args.color "green" is not a colour.` | `Send "#RRGGBB" or "#RRGGBBAA".` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not a clip | `Item itm_s6t7v8w9x0 is an audio item; chroma key applies to clips.` | `Use item.set_transform to size other items.` |

**Inverse.** `patch {collection: "items", id, set: <old value>, unset: <field if it was absent>}`.

**Idempotency.** Same result.

**Example.** Clip `itm_m1n2p3q4r5` with no key.

```json
{"op": "clip.set_chroma_key", "args": {"item_id": "itm_m1n2p3q4r5", "color": "#00FF00"}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"chroma_key": [null, {"color": "#00FF00", "similarity": 0.3, "smoothness": 0.1, "spill": 0}]}}]
```

**Golden cases.**

- `ok_defaults`
- `ok_all_values`
- `ok_keeps_current_values`
- `ok_one_value`
- `ok_null_removes_key`
- `ok_null_without_key_noop`
- `ok_boundaries_0_1`
- `ok_rounded_to_3_decimals`
- `ok_image_clip`
- `err_validation_no_color`
- `err_validation_bad_color`
- `err_validation_similarity_above_1`
- `err_validation_spill_negative`
- `err_validation_value_with_null`
- `err_not_found`
- `err_type_mismatch_audio_item`

### C.56 `marker.add`

Puts a marker on the timeline (VV-035, SDD-001 §3.1): a labelled frame the person or the agent keeps, for example to find a place again or to cut on it. Without `item_id` it is a marker of the document, in frames of the timeline. With `item_id` (TASK-0254) it is a marker of that item (SDD-001 §3.4, the kind `audio.mark_beats` writes): `at` is still a position of the timeline and must fall inside the item, the marker is stored relative to the item's first frame and moves, trims and splits with it (M1 to M4), and a marker written this way survives an OTIO round trip.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | no | none | the item to mark instead of the timeline (TASK-0254); `at` must fall inside it |
| `at` | time | yes | none | the position on the timeline, 0 up to the frame limit |
| `label` | string | no | none | up to 60 characters |

**Preconditions.**

1. `at` has a unit and is in range; `label` is at most 60 characters (`E_INVALID_TIME`, `E_VALIDATION`).
2. No marker exists at that frame (`E_INVALID_STATE`; use `marker.set`).
3. The timeline, or the item, has fewer than 200 markers (`E_LIMIT`). With `item_id` the item exists (`E_NOT_FOUND`) and `at` is inside it (`E_VALIDATION`).

**Postconditions.**

1. `markers` of the document (of the item with `item_id`) is the old list plus `{frame, label}`, in frame order.
2. `diff` has one `project.updated` (`markers`), or one `item.updated` with `item_id`. `effective` has `at` as an A.4 triple and `markers`, the count.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_INVALID_TIME` | `at` has no unit | `args.at "30" has no unit.` | `Use "1s", "30f" or "00:00:01.000".` |
| `E_VALIDATION` | `at` out of range, or a label over 60 characters | `args.label has 61 characters; the limit is 60.` | `Shorten the label.` |
| `E_INVALID_STATE` | a marker exists at that frame | `The timeline already has a marker at frame 30.` | `Use marker.set to change it.` |
| `E_LIMIT` | 200 markers already | `The timeline would have 201 markers; the limit is 200.` | `Remove markers with marker.remove first.` |

**Inverse.** `patch {collection: "project", set: {markers: <old>}}`, or `unset: ["markers"]` when there were none; with `item_id`, the same on `collection: "items"` and the item id.

**Idempotency.** Not idempotent: the second call is `E_INVALID_STATE`.

**Example.**

```json
{"op": "marker.add", "args": {"at": "1s", "label": "drop"}}
```

**Golden cases.**

- `ok_first_marker`
- `ok_without_label`
- `ok_between_existing_keeps_order`
- `ok_frame_0`
- `err_invalid_state_frame_taken`
- `err_invalid_time_no_unit`
- `err_validation_label_over_60_characters`
- `err_limit_over_200_markers`
- `ok_item_first_marker`
- `ok_item_marker_at_the_first_frame`
- `ok_item_marker_keeps_order`
- `ok_item_marker_on_the_last_frame`
- `err_validation_item_marker_before_the_item`
- `err_validation_item_marker_after_the_item`
- `err_invalid_state_item_frame_taken`
- `err_not_found_item`
- `err_validation_item_id_form`
- `err_limit_over_200_markers_on_an_item`

### C.57 `marker.set`

Moves or relabels the marker at a frame of the timeline, or of an item with `item_id` (then `at` and `to` are positions of the timeline inside the item, as in C.56).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | no | none | the item that holds the marker (TASK-0254) |
| `at` | time | yes | none | the frame of the marker to change |
| `to` | time | no | unchanged | the new frame |
| `label` | string | no | unchanged | up to 60 characters; `""` removes the label |

At least one of `to` and `label` is required.

**Preconditions.**

1. The times have units and are in range; the label is at most 60 characters (`E_INVALID_TIME`, `E_VALIDATION`); one of `to` and `label` is given (`E_VALIDATION`).
2. A marker exists at `at` (`E_NOT_FOUND`).
3. No other marker is at `to` (`E_INVALID_STATE`).

**Postconditions.**

1. The marker has the new frame and label; the list stays in frame order.
2. `diff` has one `project.updated` (`markers`); empty when nothing changes. `effective` has `at`, the final frame as an A.4 triple, and `markers`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_INVALID_TIME` | a time has no unit | `args.to "45" has no unit.` | `Use "1s", "30f" or "00:00:01.000".` |
| `E_VALIDATION` | nothing to change, out of range, or a label over 60 characters | `marker.set needs to or label.` | `Give the new position, the new label, or both.` |
| `E_NOT_FOUND` | no marker at `at` | `The timeline has no marker at frame 40.` | `Read the markers with project_get detail full.` |
| `E_INVALID_STATE` | another marker is at `to` | `The timeline already has a marker at frame 90.` | `Remove it first, or choose another position.` |

**Inverse.** Same form as C.56.

**Idempotency.** Same result.

**Example.**

```json
{"op": "marker.set", "args": {"at": "1s", "to": "2s"}}
```

**Golden cases.**

- `ok_move`
- `ok_relabel`
- `ok_clear_label`
- `ok_move_past_the_next_keeps_order`
- `ok_same_values_noop`
- `err_validation_nothing_to_change`
- `err_not_found_no_marker_there`
- `err_invalid_state_target_taken`
- `err_invalid_time_no_unit`
- `ok_item_move`
- `ok_item_relabel`
- `err_not_found_item_marker_there`
- `err_invalid_state_item_target_taken`
- `err_validation_item_target_outside`

### C.58 `marker.remove`

Removes the marker at a frame of the timeline, or every marker; with `item_id` (TASK-0254) the markers of that item, the beat markers of `audio.mark_beats` included.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | no | none | the item that holds the marker (TASK-0254) |
| `at` | time | one of | none | the frame of the marker to remove |
| `all` | boolean | one of | none | `true` removes every marker of the timeline, or of the item |

**Preconditions.**

1. Exactly one of `at` and `all` is given, and `at` has a unit (`E_VALIDATION`, `E_INVALID_TIME`).
2. With `at`, a marker exists there (`E_NOT_FOUND`).

**Postconditions.**

1. The marker is gone; `markers` is absent when none is left.
2. `diff` has one `project.updated` (`markers`); empty when there were none and `all` is true. `effective` has `markers`, the count left.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_INVALID_TIME` | `at` has no unit | `args.at "30" has no unit.` | `Use "1s", "30f" or "00:00:01.000".` |
| `E_VALIDATION` | both or neither of `at` and `all` | `marker.remove needs exactly one of at and all.` | `Give the position of the marker, or all true.` |
| `E_NOT_FOUND` | no marker at `at` | `The timeline has no marker at frame 40.` | `Read the markers with project_get detail full.` |

**Inverse.** Same form as C.56.

**Idempotency.** With `at` not idempotent (the second call is `E_NOT_FOUND`); with `all` the same result.

**Example.**

```json
{"op": "marker.remove", "args": {"at": "1s"}}
```

**Golden cases.**

- `ok_remove_one`
- `ok_remove_the_last_one`
- `ok_remove_all`
- `ok_remove_all_none_noop`
- `err_validation_at_and_all`
- `err_validation_neither`
- `err_not_found_no_marker_there`
- `err_invalid_time_no_unit`
- `ok_item_remove_one`
- `ok_item_remove_all`
- `ok_item_remove_all_leaves_the_timeline_markers`
- `err_not_found_item_marker_there`

### C.59 `item.slip`

Shows another part of the source in a clip or audio item without moving or resizing it (the slip edit, VV-036).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip` or `audio` item of a video or audio asset |
| `source_in` | time | yes | none | the new offset into the asset where the item starts reading; 0 or more |

**Preconditions.**

1. Both arguments are present and `source_in` parses (`E_VALIDATION`, `E_INVALID_TIME`).
2. The item exists (`E_NOT_FOUND`).
3. The item is a `clip` or `audio` item of a video or audio asset: not an image clip, a placeholder, a text, a caption or a shape (`E_TYPE_MISMATCH`).
4. The source range of the item, `source_in` plus its duration at its speed, stays inside the asset (R9, `E_OUT_OF_SOURCE`).

**Postconditions.**

1. `source_in` is set. `start`, `duration`, the speed, the keyframes, the fades, the transitions and every other item are unchanged.
2. `diff` has one `item.updated` with `source_in`; empty when `source_in` is already that value. `effective` has `source_in` (the A.4 triple).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `item_id` or `source_in` missing | `The required argument args.source_in is missing.` | `Add "source_in" (a time) to the arguments.` |
| `E_INVALID_TIME` | malformed `source_in` | `args.source_in "soon" is not a time.` | `Use "2s", "60f" or "00:00:02.000".` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | no source range to slip | `Item itm_m1n2p3q4r5 is a text and has no source range to slip.` | `Slip applies to clip and audio items of a video or audio asset, not to images, text, captions or shapes.` |
| `E_OUT_OF_SOURCE` | the range would end after the asset | `The range of 150 frames from source frame 751 would read to frame 901; asset ast_3c5v8n2w6d has 900 frames.` | `Use a duration of at most "149f" (4.967s), the most this asset allows from source_in 751.` |

**Inverse.** `patch {collection: "items", id, set: {source_in: <old>}}`; `unset` when the old value was 0 and absent.

**Idempotency.** Same result.

**Example.** A 150 frame clip `itm_m1n2p3q4r5` with `source_in` 30 on an asset of 900 frames.

```json
{"op": "item.slip", "args": {"item_id": "itm_m1n2p3q4r5", "source_in": "2s"}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"source_in": [30, 60]}}]
```

**Golden cases.**

- `ok_slip_later`
- `ok_slip_to_zero`
- `ok_slip_to_the_end_of_the_asset`
- `ok_slip_with_speed`
- `ok_slip_audio`
- `ok_slip_same_source_in_noop`
- `err_validation_no_source_in`
- `err_invalid_time_malformed`
- `err_not_found_item`
- `err_type_mismatch_image_clip`
- `err_type_mismatch_text_item`
- `err_out_of_source_past_the_asset`

### C.60 `settings.reframe`

Re-targets the project to another aspect ratio, from one preset or a size, and refits the items to the new frame (VV-038).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `aspect` | `"16:9"`, `"9:16"`, `"1:1"`, `"4:5"` | one of | none | the frame size 1920x1080, 1080x1920, 1080x1080 or 1080x1350 |
| `width` | integer | with `height` | none | even, 16 to 4096 |
| `height` | integer | with `width` | none | even, 16 to 4096 |

Give `aspect`, or `width` and `height` together.

**Preconditions.**

1. Exactly one of `aspect` and the pair `width` and `height` is given, and each size obeys its rule (`E_VALIDATION`).
2. No item would end with a scale outside 0.001 to 20, counting its scale keyframes (`E_VALIDATION`).

**Postconditions.**

1. `settings.width` and `settings.height` are set, and the `x` and `y` of the items and their keyframes are scaled axis by axis, as in C.2 postcondition 2.
2. With `f` the smaller of `new_width / old_width` and `new_height / old_height`, every `text` item, every `shape` item and every `clip` with fit `none` gets `scale = round3(scale × f)` and its `scale` keyframes the same, so the overlays keep their share of the frame instead of overflowing it. Clips with fit `cover` or `contain` fill the frame by themselves (adjust their crop with `clip.set_framing`), captions are laid out by the safe area and the style, and audio has no frame: none of them is resized. Text sizes, `max_width` and shape sizes are not touched.
3. `diff` has `settings.updated` followed by one `item.updated` per item changed; empty when the size is the same. `effective` has `width`, `height`, `rescaled_items` (items whose `x`, `y` or their keyframes changed) and `resized_items` (items whose scale changed).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | neither or both forms given, an odd size, or a scale out of range | `Item itm_m1n2p3q4r5 would get the scale 30, outside 0.001 to 20.` | `Set the size with settings.set to keep the items as they are, or give the item a scale nearer to 1 first.` |

**Inverse.** The same two forms as C.2: one `patch` of `settings`, then one `patch` of `items` per changed item.

**Idempotency.** Same result: the second application finds the size already set.

**Example.** A 1080x1920 project with a text item `itm_c3d4e5f6g7` at `y` 1500 with scale 1.5.

```json
{"op": "settings.reframe", "args": {"aspect": "1:1"}}
```

`f` is 0.5625, so the text item gets scale 0.844 and `y` 843.75; the project `height` goes from 1920 to 1080.

**Golden cases.**

- `ok_9_16_to_16_9_scales_overlays_by_the_height_ratio`
- `ok_9_16_to_1_1`
- `ok_9_16_to_4_5`
- `ok_16_9_to_9_16_scales_overlays_by_the_width_ratio`
- `ok_width_and_height`
- `ok_scale_keyframes_follow`
- `ok_cover_and_contain_clips_keep_their_scale`
- `ok_same_size_noop`
- `err_validation_no_argument`
- `err_validation_aspect_and_size`
- `err_validation_width_without_height`
- `err_validation_odd_height`
- `err_validation_unknown_aspect`
- `err_validation_scale_would_pass_20`

### C.61 `clip.set_blend`

Sets the blend mode of a visual item (VV-015, SDD-001 §3.4): how its pixels are mixed with what is below. The compositors draw it (SDD-004 §3.8).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip`, `text`, `caption` or `shape` item |
| `mode` | string | yes | none | `normal`, `multiply`, `screen`, `overlay` or `add`; `normal` removes the stored field |

**Preconditions.**

1. `mode` is present and is one of the five names (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).
3. The item is not an audio item (`E_TYPE_MISMATCH`).

**Postconditions.**

1. `blend` is set to `mode`; `normal` removes the field.
2. `diff` has one `item.updated`; empty when unchanged. `effective.blend` is `mode`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `mode` missing or not one of the five | `args.mode "dodge" is not a blend mode.` | `Use normal, multiply, screen, overlay or add.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | an audio item | `Item itm_s6t7v8w9x0 is an audio item; blend applies to visual items.` | `Use item.set_audio for audio items.` |

**Inverse.** `patch {collection: "items", id, set: <old value>, unset: <field if it was absent>}`.

**Idempotency.** Same result.

**Example.** Clip `itm_m1n2p3q4r5` with no blend.

```json
{"op": "clip.set_blend", "args": {"item_id": "itm_m1n2p3q4r5", "mode": "screen"}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"blend": [null, "screen"]}}]
```

**Golden cases.**

- `ok_multiply`
- `ok_screen`
- `ok_overlay`
- `ok_add`
- `ok_replaces_mode`
- `ok_normal_removes_field`
- `ok_normal_without_field_noop`
- `ok_same_mode_noop`
- `ok_image_clip`
- `err_validation_no_mode`
- `err_validation_unknown_mode`
- `err_not_found`
- `err_type_mismatch_audio_item`

### C.62 `clip.set_mask`

Sets or removes the mask of a visual item (VV-015, SDD-001 §3.4): the rectangle or ellipse, in the item's own frame, that limits where the item is drawn. The coverage formula is in SDD-004 §3.9.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip`, `text`, `caption` or `shape` item |
| `mask` | object or null | yes | none | the whole mask, which replaces the current one and clears its keyframes (`mask_x`, `mask_y`, `mask_width`, `mask_height`, `mask_feather`, VV-120); null removes it |
| `mask.shape` | string | yes | none | `rect` or `ellipse` |
| `mask.x`, `mask.y` | number | yes | none | the top-left corner, a fraction of the item's width and height, 0 to 1 |
| `mask.width`, `mask.height` | number | yes | none | the size, a fraction of the item's width and height, 0.001 to 1 |
| `mask.feather` | integer | no | 0 | the soft edge in pixels at project resolution, 0 to 200 |
| `mask.invert` | boolean | no | false | keep the outside of the shape instead of the inside |

The fractions are rounded to 3 decimals. `x + width` and `y + height` must be at most 1: the mask stays inside the item. The frame is the item's, not the canvas, so the mask moves, scales and rotates with the item.

**Preconditions.**

1. `mask` is present (`E_VALIDATION`); when it is an object, `shape`, `x`, `y`, `width` and `height` are present and in range (`E_VALIDATION`).
2. The mask is inside the item after rounding (`E_VALIDATION`).
3. The item exists (`E_NOT_FOUND`).
4. The item is not an audio item (`E_TYPE_MISMATCH`).

**Postconditions.**

1. `mask` is set to the given object, with `feather` 0 when not given and `invert` stored only when true; null removes the field.
2. `diff` has one `item.updated`; empty when unchanged. `effective.mask` is the stored mask, or null.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `mask` missing, a member missing, out of range or unknown, or the mask goes outside the item | `The mask goes outside the item: x + width or y + height is above 1.` | `Keep x + width and y + height at most 1.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | an audio item | `Item itm_c3d4e5f6g7 is an audio item; a mask applies to visual items.` | `Use item.set_audio for audio items.` |

**Inverse.** `patch {collection: "items", id, set: <old value>, unset: <field if it was absent>}`.

**Idempotency.** Same result.

**Example.** Clip `itm_m1n2p3q4r5` with no mask.

```json
{"op": "clip.set_mask", "args": {"item_id": "itm_m1n2p3q4r5", "mask": {"shape": "ellipse", "x": 0.1, "y": 0.1, "width": 0.8, "height": 0.8}}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"mask": [null, {"shape": "ellipse", "x": 0.1, "y": 0.1, "width": 0.8, "height": 0.8, "feather": 0}]}}]
```

**Golden cases.**

- `ok_ellipse`
- `ok_rect_feather_invert`
- `ok_replaces_mask`
- `ok_null_removes_mask`
- `ok_null_without_mask_noop`
- `ok_same_mask_noop`
- `ok_boundaries`
- `ok_rounded_to_3_decimals`
- `ok_image_clip`
- `err_validation_no_mask`
- `err_validation_unknown_shape`
- `err_validation_outside_item`
- `err_validation_width_zero`
- `err_validation_feather_above_200`
- `err_validation_missing_height`
- `err_not_found`
- `err_type_mismatch_audio_item`
- `ok_replace_clears_mask_keyframes`
- `ok_null_clears_mask_keyframes`

### C.63 `clip.set_flip`

Mirrors a visual item (TASK-0233, SDD-001 §3.4): `horizontal` flips it left to right, `vertical` top to bottom. The geometry is in SDD-004 §3.10.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip`, `text`, `caption` or `shape` item |
| `horizontal` | boolean | no | the current value | true mirrors left to right; false clears it |
| `vertical` | boolean | no | the current value | true mirrors top to bottom; false clears it |

**Preconditions.**

1. At least one of `horizontal` and `vertical` is present (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).
3. The item is not an audio item (`E_TYPE_MISMATCH`).

**Postconditions.**

1. `flip_x` and `flip_y` are set from the given sides; false is stored as an absent field.
2. `diff` has one `item.updated`; empty when unchanged. `effective.horizontal` and `effective.vertical` are the stored values.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | neither side given | `args: give at least one of horizontal, vertical.` | `Send for example {"horizontal": ...}.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | an audio item | `Item itm_s6t7v8w9x0 is an audio item; flip applies to visual items.` | `Use item.set_audio for audio items.` |

**Inverse.** `patch {collection: "items", id, set: <old value>, unset: <field if it was absent>}`.

**Idempotency.** Same result.

**Example.** Clip `itm_m1n2p3q4r5` not mirrored.

```json
{"op": "clip.set_flip", "args": {"item_id": "itm_m1n2p3q4r5", "horizontal": true}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"flip_x": [null, true]}}]
```

**Golden cases.**

- `ok_horizontal`
- `ok_vertical`
- `ok_both`
- `ok_keeps_the_side_not_given`
- `ok_false_clears_the_field`
- `ok_false_without_field_noop`
- `ok_same_value_noop`
- `ok_image_clip`
- `err_validation_no_argument`
- `err_not_found`
- `err_type_mismatch_audio_item`

### C.64 `item.set_enabled`

Turns an item off or on (TASK-0233, SDD-001 §3.4): a disabled item keeps its place, its settings and its markers, and is not planned (SDD-004 §3).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | any item |
| `enabled` | boolean | yes | none | false turns the item off; true turns it on |

**Preconditions.**

1. `enabled` is present (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).

**Postconditions.**

1. `disabled` is true when `enabled` is false; true `enabled` removes the field.
2. `diff` has one `item.updated`; empty when unchanged. `effective.enabled` is the given value.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `enabled` missing | `args.enabled is required.` | `Send true or false.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |

**Inverse.** `patch {collection: "items", id, set: <old value>, unset: <field if it was absent>}`.

**Idempotency.** Same result.

**Example.** Clip `itm_m1n2p3q4r5`, enabled.

```json
{"op": "item.set_enabled", "args": {"item_id": "itm_m1n2p3q4r5", "enabled": false}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"disabled": [null, true]}}]
```

**Golden cases.**

- `ok_disable_clip`
- `ok_enable_clip`
- `ok_disable_audio_item`
- `ok_disable_text_item`
- `ok_enable_without_field_noop`
- `ok_disable_twice_noop`
- `err_validation_no_enabled`
- `err_not_found`

### C.65 `item.link`

Puts items in one link group (TASK-0277, SDD-001 section 3.5): `item.move`, `item.trim` and `item.split` on one of them apply to all of them.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_ids` | list of item ids | yes | none | 2 to 20 distinct items, each a `clip`, `audio`, `text` or `shape` item |
| `id` | link id | no | generated | `lnk_` and 10 characters; a group id that no item uses |

**Preconditions.**

1. `item_ids` has 2 to 20 distinct well-formed ids and `id`, when given, is a link id (`E_VALIDATION`).
2. Every item exists (`E_NOT_FOUND`).
3. No item is a caption (`E_TYPE_MISMATCH`; captions follow a clip with `linked_item`).
4. No item is already linked (`E_INVALID_STATE`, the hint says `item.unlink`).
5. `id`, when given, is not the link of another item (`E_ID_EXISTS`).

**Postconditions.**

1. `link` of every item is the group id: `id`, or the one the generator returns for the prefix `lnk`.
2. `diff` has one `item.updated` per item, sorted by id. `effective.link_id` is the group id and `effective.item_ids` the sorted ids. The service stores a generated group id in the `id` argument of the operation, so a replay gets the same one.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | fewer than 2 or more than 20 items, an id listed twice, a malformed id | `args.item_ids[1] "itm_m1n2p3q4r5" is listed twice.` | `List each item id once.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | a caption | `Item itm_s6t7v8w9x0 is a caption; captions follow a clip with linked_item, not a link group.` | `Link clip, audio, text or shape items.` |
| `E_INVALID_STATE` | an item already linked | `Item itm_c3d4e5f6g7 is already linked (group lnk_f6g7h8j9k0).` | `Unlink it with item.unlink first.` |
| `E_ID_EXISTS` | `id` used by another group | `Link id lnk_a1b2c3d4e5 is already used by item itm_s6t7v8w9x0.` | `Omit id or use an unused id.` |

**Inverse.** `patch {collection: "items", id, unset: ["link"]}` per item, in order of mutation (R5).

**Idempotency.** Not idempotent: a second application finds the items already linked (`E_INVALID_STATE`).

**Example.** A clip `itm_m1n2p3q4r5` and its audio item `itm_c3d4e5f6g7`.

```json
{"op": "item.link", "args": {"item_ids": ["itm_m1n2p3q4r5", "itm_c3d4e5f6g7"], "id": "lnk_a1b2c3d4e5"}}
```

```json
[
  {"kind": "item.updated", "id": "itm_c3d4e5f6g7", "changes": {"link": [null, "lnk_a1b2c3d4e5"]}},
  {"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"link": [null, "lnk_a1b2c3d4e5"]}}
]
```

**Golden cases.**

- `ok_link_clip_and_audio`
- `ok_link_three_items`
- `ok_link_with_given_id`
- `ok_link_with_generated_id`
- `ok_link_text_and_shape`
- `err_validation_one_item`
- `err_validation_listed_twice`
- `err_validation_bad_link_id`
- `err_not_found_item`
- `err_type_mismatch_caption`
- `err_invalid_state_already_linked`
- `err_id_exists_link_in_use`

### C.66 `item.unlink`

Takes items out of their link group (TASK-0277).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_ids` | list of item ids | yes | none | 1 to 50 distinct linked items |

**Preconditions.**

1. `item_ids` has 1 to 50 distinct well-formed ids (`E_VALIDATION`).
2. Every item exists (`E_NOT_FOUND`).
3. Every item is linked (`E_INVALID_STATE`).

**Postconditions.**

1. `link` is removed from every listed item.
2. A group left with exactly one item is dissolved: that item loses its `link` too and is listed in `effective.also_unlinked` (sorted; `[]` when none).
3. `diff` has one `item.updated` per changed item, sorted by id. `effective.item_ids` is the sorted list given.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | an id listed twice, a malformed id, an empty or longer than 50 list | `args.item_ids[1] "itm_m1n2p3q4r5" is listed twice.` | `List each item id once.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_INVALID_STATE` | an item that is not linked | `Item itm_m1n2p3q4r5 is not linked.` | `Only linked items can be unlinked.` |

**Inverse.** `patch {collection: "items", id, set: {"link": <old group id>}}` per changed item, in order of mutation (R5).

**Idempotency.** Not idempotent: a second application finds the items not linked (`E_INVALID_STATE`).

**Example.** Two linked items; unlinking one dissolves the group.

```json
{"op": "item.unlink", "args": {"item_ids": ["itm_m1n2p3q4r5"]}}
```

```json
[
  {"kind": "item.updated", "id": "itm_c3d4e5f6g7", "changes": {"link": ["lnk_a1b2c3d4e5", null]}},
  {"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"link": ["lnk_a1b2c3d4e5", null]}}
]
```

**Golden cases.**

- `ok_unlink_one_of_three`
- `ok_unlink_two_of_two_dissolves`
- `ok_unlink_one_of_two_unlinks_the_other`
- `ok_unlink_whole_group`
- `err_validation_listed_twice`
- `err_not_found_item`
- `err_invalid_state_not_linked`

### C.67 `item.extract_audio`

Creates an audio item from the audio of a video clip, linked to it, and mutes the clip (TASK-0277).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip` of a video asset that has audio |
| `track_id` | track id | yes | none | an audio track |
| `id` | item id | no | generated | the id of the new audio item |

**Preconditions.**

1. The ids are well formed (`E_VALIDATION`).
2. The item and the track exist (`E_NOT_FOUND`).
3. The item is a `clip` (not an audio item), the track is an audio track and the asset of the clip is a video with audio (`E_TYPE_MISMATCH`).
4. The clip is not `audio_muted` (`E_INVALID_STATE`: its audio was already extracted or silenced).
5. The project has fewer than 500 items (`E_LIMIT`) and `id` is unused (`E_ID_EXISTS`).
6. The audio item does not overlap another item of the track (`E_OVERLAP`, R8).

**Postconditions.**

1. A new `audio` item on `track_id` with the `start`, `duration`, `asset`, `source_in`, `speed`, `speed_curve`, `reverse`, `volume`, `fade_in`, `fade_out`, `denoise` and `voice` of the clip, and the `volume` keyframes of the clip. Other keyframes, the transitions, the colour and the transform stay on the clip.
2. The clip gets `audio_muted: true`. Both items get the `link` of the clip when it has one (the audio item joins its group); otherwise the group id is `lnk_` plus the ten characters of the new item id.
3. `diff`: `item.added` for the audio item, then `item.updated` for the clip. `effective` has `new_id`, `link` and `track_id`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | a malformed id | `args.id "audio1" is not an item id.` | `Use itm_ and 10 characters, or omit id.` |
| `E_NOT_FOUND` | unknown item or track | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not a clip, a visual track, an image clip, an asset with no audio | `Track trk_a1b2c3d4e5 is a visual track; the audio item needs an audio track.` | `Use an audio track, for example the one from track.add.` |
| `E_INVALID_STATE` | the clip is already muted | `Clip itm_m1n2p3q4r5 is already muted: its audio was extracted or silenced.` | `Unmute it with item.set_audio muted false first, or use its audio item.` |
| `E_OVERLAP` | the range is taken on the audio track | `Item itm_d4e5f6g7h8 would occupy frames 30-119 on track trk_m1n2p3q4r5, where item itm_c3d4e5f6g7 already is (frames 60-89).` | `Use another audio track.` |
| `E_ID_EXISTS` | `id` is used | `Item id itm_m1n2p3q4r5 already exists.` | `Omit id or use an unused id.` |
| `E_LIMIT` | 500 items | `items would exceed the limit of 500.` | `Delete items first.` |

**Inverse.** `patch {collection: "items", id: <clip>, unset: ["audio_muted", "link"]}` (or set the old link), then `remove {collection: "items", id: <new>}`, in order of mutation (R5).

**Idempotency.** Not idempotent: a second application finds the clip muted (`E_INVALID_STATE`).

**Example.** Clip `itm_m1n2p3q4r5` (start 30, 90 frames, `source_in` 10) and the audio track `trk_m1n2p3q4r5`.

```json
{"op": "item.extract_audio", "args": {"item_id": "itm_m1n2p3q4r5", "track_id": "trk_m1n2p3q4r5", "id": "itm_c3d4e5f6g7"}}
```

**Golden cases.**

- `ok_extract_audio`
- `ok_extract_audio_with_generated_id`
- `ok_copies_audio_settings`
- `ok_copies_curve_and_reverse`
- `ok_joins_the_group_of_the_clip`
- `err_validation_bad_new_id`
- `err_not_found_item`
- `err_not_found_track`
- `err_type_mismatch_not_a_clip`
- `err_type_mismatch_visual_track`
- `err_type_mismatch_image_clip`
- `err_type_mismatch_asset_without_audio`
- `err_invalid_state_already_muted`
- `err_overlap_audio_track_busy`
- `err_id_exists`
- `err_limit_500_items`

### C.68 `clip.set_effects`

Sets the visual effects of a clip (TASK-0278, SDD-001 section 3.4, pixels in SDD-004 section 3.13). One list command instead of an add, a set and a remove per effect: the whole list is the argument, so an agent states the result it wants, the inverse is one patch, and the tool list stays one entry smaller.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip` |
| `effects` | list of effects | yes | none | up to 6; `[]` removes them all. An effect is `{"kind", ...}` with `kind` one of `motion_blur`, `glow`, `vignette`, `grain`, `rgb_split`, `shake`, each kind at most once, and only the parameters of its kind |

The parameters, with range and default: `motion_blur` `amount` 0 to 1 (0.5), `angle` -180 to 180 degrees (0); `glow` `amount` 0 to 1 (0.5), `radius` 1 to 100 frame pixels (12), `threshold` 0 to 1 (0.6); `vignette` `amount` 0 to 1 (0.5), `softness` 0 to 1 (0.5); `grain` `amount` 0 to 1 (0.3), `size` 1 to 8 (1), `seed` 0 to 65535 (0); `rgb_split` `amount` 0 to 1 (0.3), `angle` -180 to 180 (0); `shake` `amount` 0 to 1 (0.3), `speed` 1 to 30 per second (8), `seed` 0 to 65535 (0). `size`, `speed` and `seed` are whole numbers; the other values are rounded to 3 decimals.

**Preconditions.**

1. Arguments match the schema: known kinds, no kind twice, at most 6 effects, a parameter only on a kind that takes it, every value in range (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).
3. The item is a `clip` (`E_TYPE_MISMATCH`).

**Postconditions.**

1. `effects` is the given list in the given order, each effect with every parameter of its kind present (the defaults filled in, values rounded); it is absent when the list is empty. A parameter equal to its default is still stored, so a stored effect says everything.
2. `diff` has one `item.updated`; empty when nothing changes. `effective.effects` is the number of effects.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | unknown kind, a kind twice, a parameter of another kind, a value out of range, more than 6 effects | `args.effects[0].radius does not apply to vignette.` | `vignette takes [amount softness].` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not a clip | `Item itm_s6t7v8w9x0 is a text item; effects applies to clips.` | `Use item.set_transform to size other items.` |

**Inverse.** `patch {collection: "items", id, set: {effects: <old>}}` or `unset: ["effects"]` when there were none.

**Idempotency.** Same result.

**Example.** Clip `itm_m1n2p3q4r5` with no effects.

```json
{"op": "clip.set_effects", "args": {"item_id": "itm_m1n2p3q4r5", "effects": [{"kind": "vignette", "amount": 0.6}]}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"effects": [null, [{"kind": "vignette", "amount": 0.6, "softness": 0.5}]]}}]
```

**Golden cases.**

- `ok_each_kind_with_defaults`
- `ok_motion_blur_parameters`
- `ok_glow_parameters`
- `ok_vignette_parameters`
- `ok_grain_parameters`
- `ok_rgb_split_parameters`
- `ok_shake_parameters`
- `ok_amount_0_is_stored`
- `ok_decimals_rounded_to_3`
- `ok_order_is_kept`
- `ok_replaces_the_whole_list`
- `ok_empty_list_removes_them`
- `ok_empty_list_without_effects_noop`
- `ok_same_list_noop`
- `err_validation_unknown_kind`
- `err_validation_kind_listed_twice`
- `err_validation_parameter_of_another_kind`
- `err_validation_amount_above_1`
- `err_validation_angle_above_180`
- `err_validation_radius_below_1`
- `err_validation_size_above_8`
- `err_validation_speed_0`
- `err_validation_seed_above_65535`
- `err_validation_seven_effects`
- `err_not_found`
- `err_type_mismatch_text_item`

### C.69 `item.paste_attributes`

Copies the look of one item to others (TASK-0254): the "paste attributes" of an editor, so an agent or a person grades one clip and applies the same grade, filter or transform to the rest in one operation and one undo step.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `from_item_id` | item id | yes | none | the item to copy from |
| `item_ids` | list of item ids | yes | none | 1 to 20 other items, each once |
| `attributes` | list of names | yes | none | 1 to 11 of the groups below, each once |

The groups, the item types that have them and what each copies (the source value is copied as it is, so a default value clears a value of the target):

| Group | Item types | Fields |
| --- | --- | --- |
| `color` | clip | `brightness`, `contrast`, `saturation`, `temperature`, `tint`, `hue`, `lut` |
| `filter` | clip, text, caption, shape | `filter`, `filter_amount` |
| `blur` | clip, text, caption, shape | `blur` |
| `framing` | clip | `fit`, `crop` |
| `transform` | clip, text, caption, shape | `x`, `y`, `scale`, `rotation`, `opacity` |
| `blend` | clip, text, caption, shape | `blend` |
| `mask` | clip, text, caption, shape | `mask` |
| `flip` | clip, text, caption, shape | `flip_x`, `flip_y` |
| `chroma_key` | clip, text, caption, shape | `chroma_key` |
| `effects` | clip, text, caption, shape | `effects` |
| `audio` | clip, audio | `volume`, `denoise`, `voice` |

Keyframes, fades, speed, transitions and the position, size and source of the item are never copied.

**Preconditions.**

1. `item_ids` has 1 to 20 ids, each once and none the source; `attributes` has 1 to 11 known names, each once; the ids have the item form (`E_VALIDATION`).
2. The source and every target exist (`E_NOT_FOUND`).
3. The source has every group asked for (`E_TYPE_MISMATCH`, for example `color` from a text item).
4. At least one target has at least one group asked for (`E_TYPE_MISMATCH`). A group the target type does not have is skipped for that target, not an error.
5. No target has keyframes on a property of a copied group: any of `x`, `y`, `scale`, `rotation`, `opacity` for `transform`, `volume` for `audio` (`E_INVALID_STATE`; use `keyframe.clear` first). Keyframes of other properties do not matter.

**Postconditions.**

1. Each target has the source values of the groups it has, and nothing else changes.
2. `diff` has one `item.updated` per target that changes; empty when nothing changes. `effective` has `items` (the number of targets), `attributes` (as given) and, when something was skipped, `skipped`: a list of `{item_id, attribute}`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | an empty or too long list, an unknown or repeated group, the source among the targets, a repeated target | `args.attributes[1] "sparkle" is not an attribute group.` | `Use one of: color, filter, blur, framing, transform, blend, mask, flip, chroma_key, effects, audio.` |
| `E_NOT_FOUND` | the source or a target does not exist | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | the source lacks a group, or no target has any group | `None of the target items has the attributes color.` | `Choose targets of a type that has them, or other groups.` |
| `E_INVALID_STATE` | a target has keyframes on a copied property | `Property x of itm_c3d4e5f6g7 has 2 keyframes, which define its value.` | `Use keyframe.clear on the target first, or leave transform out of attributes.` |

**Inverse.** One `patch {collection: "items", id, set: {...}}` per changed target with its old values (`unset` for a field that was absent), in the order of the targets.

**Idempotency.** Same result.

**Example.** Copy the grade of clip `itm_m1n2p3q4r5` to clip `itm_c3d4e5f6g7`.

```json
{"op": "item.paste_attributes", "args": {"from_item_id": "itm_m1n2p3q4r5", "item_ids": ["itm_c3d4e5f6g7"], "attributes": ["color", "filter"]}}
```

**Golden cases.**

- `ok_color_to_a_clip`
- `ok_filter_and_blur_to_a_clip`
- `ok_framing_to_a_clip`
- `ok_transform_to_a_clip_and_a_text`
- `ok_look_to_a_text`
- `ok_filter_and_blur_to_a_shape`
- `ok_audio_to_an_audio_item`
- `ok_skips_the_groups_the_target_lacks`
- `ok_every_group`
- `ok_defaults_of_the_source_clear_the_target`
- `ok_same_values_noop`
- `ok_keyframes_of_other_groups_do_not_block`
- `err_validation_no_attributes`
- `err_validation_unknown_attribute`
- `err_validation_attribute_listed_twice`
- `err_validation_no_targets`
- `err_validation_target_is_the_source`
- `err_validation_target_listed_twice`
- `err_validation_bad_target_id`
- `err_not_found_source`
- `err_not_found_target`
- `err_type_mismatch_source_lacks_the_group`
- `err_type_mismatch_no_target_has_the_group`
- `err_invalid_state_target_transform_has_keyframes`
- `err_invalid_state_target_volume_has_keyframes`

### C.70 `item.freeze_frame`

Holds one frame of a clip (TASK-0254): the "freeze frame" of an editor. The clip is split at the frame and a still of the source frame shown there is put between the two parts, for as long as asked, in one operation and one undo step.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip` of a video asset, not linked, with no speed curve, not a freeze frame itself |
| `at` | time | yes | none | absolute frame strictly inside the clip (`start < at < end`), like `item.split`; the picture held is the one shown at `at` |
| `duration` | time | no | `3s` | at least 1 frame |
| `freeze_id` | item id | no | generated | id of the freeze frame |
| `new_id` | item id | no | generated | id of the right part of the split |

**Preconditions.**

1. `item_id`, `freeze_id` and `new_id` have the item form and `freeze_id` is not `new_id` (`E_VALIDATION`).
2. `at` and `duration` are times and `duration` is at least 1 frame; `at` is strictly inside the clip (`E_INVALID_TIME`); the last item of the track, moved by `duration`, still ends by frame 108,000 (`E_INVALID_TIME`).
3. The item exists (`E_NOT_FOUND`).
4. The item is a `clip` of a video asset (`E_TYPE_MISMATCH`; an image is already a still).
5. The item is not already a freeze frame, has no `link` and has no speed curve (`E_INVALID_STATE`; the commands that split an item refuse a curve, and a linked sound would go out of step).
6. The ids are not used (`E_ID_EXISTS`) and the document has room for two more items (`E_LIMIT`).

**Postconditions.**

1. The left part keeps the id and the front of the clip (R17, as `item.split`); the right part is `new_id`, from `at + duration`. The source frame held is `source_in + SourceOffset(at - start)` (`effective.source_frame`), which is the first frame of the right part, so a reversed or sped-up clip holds the frame that is on screen.
2. The freeze frame, `freeze_id`, is a copy of the clip with `freeze` true, from `at`, for `duration`, `source_in` the frame above, speed 1, no curve or reverse, `audio_muted` true, no fades, transitions, keyframes, markers or slot. Its transform, colour, filter, mask and effects are the static values of the clip.
3. Every other item of the clip's track that started at or after the end of the clip moves later by `duration`; items of other tracks do not move. A track with `magnet` is then packed as after any command.
4. `diff` has the `item.updated` of the left part and of the moved items, and two `item.added`. `effective` has `at`, `duration`, `freeze_id`, `new_id`, `source_frame`, and `left`, `freeze` and `right` as `{id, start, duration}`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | a bad id, or `freeze_id` equal to `new_id` | `args.freeze_id and args.new_id are the same id.` | `Use two different ids or omit them.` |
| `E_INVALID_TIME` | `at` is a bad time or not inside the clip, `duration` is 0, or the track would pass the end | `args.at "0f" is not inside item itm_m1n2p3q4r5 (frames 0-89); it must be between 1 and 89 inclusive.` | `Use at "1f" or later, up to "89f".` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not a clip of a video asset | `Item itm_e5f6g7h8j9 is an audio item; a freeze frame needs a clip of a video asset.` | `Use a clip of a video asset.` |
| `E_INVALID_STATE` | a freeze frame, a linked item or an item with a speed curve | `Item itm_m1n2p3q4r5 is linked to other items, which a freeze frame would put out of step.` | `Unlink it with item.unlink first.` |
| `E_ID_EXISTS` | an id is used | `Item id itm_c3d4e5f6g7 already exists.` | `Omit id or use an unused id.` |
| `E_LIMIT` | fewer than two free item places | `The document has 499 items; a freeze frame adds two and the limit is 500.` | `Delete unused items first.` |

**Inverse.** `patch` of the changed items with their old values, `delete` of the two new items.

**Idempotency.** Not idempotent: a second call splits again.

**Example.** Hold the frame at 30 for 2 seconds.

```json
{"op": "item.freeze_frame", "args": {"item_id": "itm_m1n2p3q4r5", "at": "30f", "duration": "2s", "freeze_id": "itm_f1g2h3j4k5", "new_id": "itm_m2n3p4q5r6"}}
```

**Golden cases.**

- `ok_holds_the_frame_for_the_given_duration`
- `ok_default_duration_is_3_seconds`
- `ok_later_items_of_the_track_move_other_tracks_do_not`
- `ok_reversed_clip_holds_the_frame_that_is_on_screen`
- `ok_sped_up_clip_holds_the_frame_that_is_on_screen`
- `ok_keyframes_and_fades_stay_with_the_parts`
- `ok_last_frame_of_the_clip`
- `ok_magnet_track_stays_packed`
- `err_validation_bad_freeze_id`
- `err_validation_same_ids`
- `err_invalid_time_zero_duration`
- `err_invalid_time_at_the_start_of_the_clip`
- `err_invalid_time_at_the_end_of_the_clip`
- `err_invalid_time_malformed`
- `err_invalid_time_track_would_pass_108000`
- `err_not_found`
- `err_type_mismatch_audio_item`
- `err_type_mismatch_image_clip`
- `err_invalid_state_linked_item`
- `err_invalid_state_already_a_freeze_frame`
- `err_invalid_state_speed_curve`
- `err_id_exists`
- `err_limit_500_items`

### C.71 `caption.import_subtitles`

Creates one caption item for each cue of an SRT or WebVTT text (TASK-0254, NEW-32a). The command is pure: the caller or the service reads the file and sends its text, and the parser is `core/subtitle`. Writing the captions of a document back as SRT or WebVTT is `subtitle.Format` over the cues a caller builds from the captions (the service does the file); the core has no command for it, because it changes nothing.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `track_id` | track id | yes | none | a visual track |
| `text` | string | yes | none | the SRT or WebVTT text, at most 2,000,000 characters and 5,000 cues |
| `format` | `"srt"`, `"vtt"` | no | `vtt` when the text starts with `WEBVTT`, else `srt` | |
| `start` | length | no | `0` | added to every cue |
| `new_ids` | list of item ids | no | generated | ids of the captions, in cue order |

**Preconditions.**

1. `track_id` and the `new_ids` have the id form, the `new_ids` are not repeated and `start` is a length that may be 0 (`E_VALIDATION`).
2. `text` is valid in the format: a time line is `hh:mm:ss,mmm --> hh:mm:ss,mmm` (VTT: the hours may be left out and the milliseconds follow a dot or a comma), the cue ends after it starts, minutes and seconds are below 60; and at least one cue has text (`E_VALIDATION`, the message names the cue).
3. The track exists (`E_NOT_FOUND`) and is visual (`E_TYPE_MISMATCH`).
4. The ids are not used (`E_ID_EXISTS`) and the document has room for one item per cue (`E_LIMIT`).
5. Each cue has at least as many frames as words (`E_INVALID_STATE`), no word longer than 80 characters (`E_VALIDATION`), and the words stay within the limits of a caption and of the document (`E_LIMIT`).
6. A cue does not overlap an item of the track nor a cue before it, and ends by frame 108,000 (`E_OVERLAP`, `E_INVALID_TIME`).

**Postconditions.**

1. Each cue becomes a caption: its start and end, in milliseconds, are rounded to the nearest frame of the project (`ms x fps / 1000`) and `start` is added; the duration is at least 1 frame. Header, cue numbers, VTT identifiers and settings, `NOTE`, `STYLE` and `REGION` blocks, `<...>` markup and `{\...}` override tags are dropped; the lines of a cue are joined by a space, and its words share the duration in proportion to their characters, as `caption.add` with `text`. A cue with no text is skipped.
2. `diff` has one `item.added` per caption. `effective` has `created` and `new_ids` (the ids used), `unused_ids`, `cue_count` and `format`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | a bad id, a text that is not valid SRT or WebVTT, or no cue with text | `args.text is not valid srt: cue 1: "00:00:01" is not a time (hh:mm:ss,mmm).` | `Send SubRip or WebVTT text with a time line (00:00:01,000 --> 00:00:02,500) for each cue.` |
| `E_INVALID_TIME` | a cue ends past frame 108,000 | `The item would occupy frames 108000-108044, past the limit 108000.` | `Use a start of at most "107955f" for this 45-frame item.` |
| `E_NOT_FOUND` | unknown track | `Track trk_zzzzzzzzzz does not exist.` | `Use the id of a track listed by project_get.` |
| `E_TYPE_MISMATCH` | an audio track | `Track trk_m1n2p3q4r5 is an audio track; captions need a visual track.` | `Use a visual track.` |
| `E_INVALID_STATE` | a cue with fewer frames than words | `Cue 1 lasts 1 frames but has 3 words; each word needs 1 frame.` | `Lengthen the cue in the file or use a higher frame rate.` |
| `E_ID_EXISTS` | an id is used | `Item id itm_m1n2p3q4r5 already exists.` | `Omit id or use an unused id.` |
| `E_OVERLAP` | a cue overlaps an item of the track | `Item itm_d4d4d4d4d4 would occupy frames 30-74 on track trk_f6g7h8j9k0, where item itm_m1n2p3q4r5 already is (frames 0-89).` | `Cue 1 overlaps another item of the track; pick an empty track or another start.` |
| `E_LIMIT` | too many items | `The document would have 501 items; the limit is 500.` | `Delete unused items first.` |

**Inverse.** `delete` of the created items.

**Idempotency.** Not idempotent: a second call adds the captions again, or fails with `E_OVERLAP`.

**Example.** Two cues on an empty caption track.

```json
{"op": "caption.import_subtitles", "args": {"track_id": "trk_a1b2c3d4e5", "text": "1\n00:00:01,000 --> 00:00:02,500\nHello there\n\n2\n00:00:03,000 --> 00:00:04,000\nBye\n", "new_ids": ["itm_d4d4d4d4d4", "itm_e5e5e5e5e5"]}}
```

**Golden cases.**

- `ok_srt_two_cues`
- `ok_vtt_detected_by_its_header`
- `ok_format_given`
- `ok_start_moves_every_cue`
- `err_validation_bad_time_line`
- `err_validation_no_cue_with_text`
- `err_validation_bad_new_id`
- `err_not_found_track`
- `err_type_mismatch_audio_track`
- `err_invalid_time_cue_past_the_end`
- `err_overlap_item_on_the_track`
- `err_id_exists`
- `err_invalid_state_cue_shorter_than_its_words`
- `err_limit_500_items`

### C.72 `clip.set_shadow`

Sets or removes the drop shadow of a clip (VV-122, SDD-001 §3.5): the silhouette of its picture, moved, blurred and tinted, drawn below it. The pixels are in SDD-004 §3.15.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip` (video or still image) |
| `shadow` | object or null | yes | none | the whole shadow, which replaces the current one; null removes it |
| `shadow.color` | colour | no | `#000000` | |
| `shadow.opacity` | number | no | 0.5 | 0 to 1 |
| `shadow.dx`, `shadow.dy` | number | no | 8 | -500 to 500 pixels |
| `shadow.blur` | number | no | 12 | 0 to 100 pixels |

**Preconditions.**

1. `shadow` is present, an object or null, and each member given is in range (`E_VALIDATION`); a colour is `#RRGGBB`, `#RRGGBBAA` or a `$name` of `template.colors`.
2. The item exists (`E_NOT_FOUND`).
3. The item is a `clip` (`E_TYPE_MISMATCH`).

**Postconditions.**

1. `shadow` is set to the stored object, every member filled in (hex colours upper case, numbers rounded to 3 decimals); null removes the field.
2. `diff` has one `item.updated`; empty when unchanged. `effective.shadow` is the stored object, or null.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `shadow` missing, a member out of range or not a colour | `The value of args.shadow.opacity is 1.5, outside the accepted range.` | `Use a value between 0 and 1.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not a clip | `Item itm_s6t7v8w9x0 is a text item; a shadow applies to clips.` | `Use item.set_transform to size other items.` |

**Inverse.** `patch {collection: "items", id, set: <old value>, unset: <field if it was absent>}`.

**Idempotency.** Same result.

**Example.** Clip `itm_m1n2p3q4r5` with no shadow.

```json
{"op": "clip.set_shadow", "args": {"item_id": "itm_m1n2p3q4r5", "shadow": {"color": "#000000", "opacity": 0.6, "dx": 10, "dy": 12, "blur": 16}}}
```

**Golden cases.**

- `ok_defaults`
- `ok_full`
- `ok_replaces_shadow`
- `ok_null_removes_shadow`
- `ok_null_without_shadow_noop`
- `ok_same_shadow_noop`
- `ok_image_clip`
- `err_validation_no_shadow`
- `err_validation_opacity_above_1`
- `err_validation_blur_above_100`
- `err_validation_bad_colour`
- `err_validation_unknown_template_colour`
- `err_not_found`
- `err_type_mismatch_text_item`

### C.73 `clip.set_outline`

Sets or removes the outline of a clip (VV-122, SDD-001 §3.5): a contour around the visible shape of its picture. The pixels are in SDD-004 §3.15.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip` (video or still image) |
| `outline` | object or null | yes | none | the whole outline, which replaces the current one; null removes it |
| `outline.color` | colour | no | `#000000` | |
| `outline.width` | number | no | 4 | 0.5 to 100 pixels |

**Preconditions.**

1. `outline` is present, an object or null, and each member given is in range (`E_VALIDATION`); a colour is `#RRGGBB`, `#RRGGBBAA` or a `$name` of `template.colors`.
2. The item exists (`E_NOT_FOUND`).
3. The item is a `clip` (`E_TYPE_MISMATCH`).

**Postconditions.**

1. `outline` is set to the stored object, every member filled in (hex colours upper case, numbers rounded to 3 decimals); null removes the field.
2. `diff` has one `item.updated`; empty when unchanged. `effective.outline` is the stored object, or null.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | `outline` missing, a member out of range or not a colour | `The value of args.outline.width is 0.1, outside the accepted range.` | `Use a width between 0.5 and 100.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | not a clip | `Item itm_s6t7v8w9x0 is a text item; an outline applies to clips.` | `Use item.set_transform to size other items.` |

**Inverse.** `patch {collection: "items", id, set: <old value>, unset: <field if it was absent>}`.

**Idempotency.** Same result.

**Example.** Clip `itm_m1n2p3q4r5` with no outline.

```json
{"op": "clip.set_outline", "args": {"item_id": "itm_m1n2p3q4r5", "outline": {"color": "#FFFFFF", "width": 6}}}
```

**Golden cases.**

- `ok_defaults`
- `ok_full`
- `ok_replaces_outline`
- `ok_null_removes_outline`
- `ok_null_without_outline_noop`
- `ok_same_outline_noop`
- `ok_image_clip`
- `err_validation_no_outline`
- `err_validation_width_below_half`
- `err_validation_width_above_100`
- `err_validation_bad_colour`
- `err_not_found`
- `err_type_mismatch_text_item`

### C.74 `item.repeat`

Repeats an item back to back after its end (VV-123): a looping background, a sticker that plays again. Each copy is made like `item.duplicate`, at the end of the one before.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | any item |
| `copies` | integer | one of `copies`, `until` | none | 1 to 200 extra copies (2 plays the item three times) |
| `until` | time | one of `copies`, `until` | none | absolute; after the end of the item and at most frame 108,000. Whole copies are added until the sequence reaches it; the last one is trimmed (as `item.trim` of the end edge does) to end exactly there. At most 200 copies |
| `ripple` | `"none"`, `"track"` | no | `none` | `none` fails with `E_OVERLAP` when a copy meets an item of the track; `track` moves every item of the track that starts at or after the end of the item later by the length added |
| `link` | boolean | no | false | links the item and its copies in one group (SDD-001 §3.4, `item.link`); at most 20 items, and the item must not be linked already |
| `new_ids` | list of item ids | no | generated | ids of the copies in order; at most one per copy |

**Preconditions.**

1. Exactly one of `copies` and `until` is given and valid (`E_VALIDATION`, `E_INVALID_TIME`, `E_LIMIT` for more than 200 copies); `new_ids` are well formed, distinct and at most one per copy (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`).
3. With `link`, the item is not linked and the group has at most 20 items (`E_INVALID_STATE`, `E_VALIDATION`).
4. The project stays within 500 items (`E_LIMIT`), every copy ends at or before frame 108,000 (`E_INVALID_TIME`) and the new ids are unused (`E_ID_EXISTS`).
5. With ripple `none`, no copy overlaps an item of the track (`E_OVERLAP`).

**Postconditions.**

1. Copy `k` starts where copy `k - 1` (the item for `k = 1`) ends and is a clone of the item: keyframes, markers, words, speed, effects and the name are copied; the slot mark is not and the link is the new group's or none. A trimmed last copy follows the rules of `item.trim` for its edge (keyframes and words cut, fades and transitions must still fit).
2. With ripple `track`, the later items of the track are moved first.
3. `diff` has one `item.added` per copy, then the `item.updated` of the shifted and of the linked items. `effective` has `new_ids`, `end` (the A.4 triple of the end of the last copy) and, with `link`, `link`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | neither or both of `copies` and `until`, `copies` out of range, too many `new_ids`, a link group over 20 | `args: give exactly one of copies and until.` | `Use {"copies": 3} or {"until": "120s"}.` |
| `E_INVALID_TIME` | `until` not after the end of the item, or past frame 108,000 | `args.until "90f" is frame 90; the item already ends at frame 90.` | `Use an until after "90f" (3.000s).` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_INVALID_STATE` | `link` on an item that is linked already | `Item itm_m1n2p3q4r5 is already linked (lnk_a1b2c3d4e5).` | `Unlink it with item.unlink first, or repeat without link.` |
| `E_ID_EXISTS` | a supplied id is used | `Item id itm_c3d4e5f6g7 already exists.` | `Omit id or use an unused id.` |
| `E_LIMIT` | more than 200 copies, more than 500 items, or the copies of a caption would take the document over 5,000 caption words | `Repeating itm_m1n2p3q4r5 until 1500f needs 299 copies; the limit is 200.` | `Use a later start of the sequence, a longer item, or two item.repeat calls.` |
| `E_OVERLAP` | a copy meets an item of the track (ripple `none`) | `Item itm_n1p2q3r4s5 would occupy frames 180-269 on track trk_f6g7h8j9k0, where item itm_c3d4e5f6g7 already is (frames 120-149).` | `Use ripple track, or move the item first.` |

**Inverse.** The `remove` of every added item, and a `patch` of the moved and linked items.

**Idempotency.** Error when applied again: the copies now occupy the place of the next ones (`E_OVERLAP`, or more copies with ripple `track`).

**Example.** Clip `itm_m1n2p3q4r5`, 90 frames at 30 fps, alone on its track.

```json
{"op": "item.repeat", "args": {"item_id": "itm_m1n2p3q4r5", "until": "9s", "new_ids": ["itm_g9h8j7k6m5", "itm_n1p2q3r4s5"]}}
```

The copies start at frames 90 and 180; the sequence ends at frame 270 (`effective.end`).

**Golden cases.**

- `ok_copies_one`
- `ok_copies_three_with_ids`
- `ok_until_whole_copies`
- `ok_until_partial_last_copy`
- `ok_ripple_track_moves_later_items`
- `ok_link_the_copies`
- `ok_keyframes_are_copied`
- `ok_text_item`
- `err_validation_neither_copies_nor_until`
- `err_validation_both_copies_and_until`
- `err_validation_copies_zero`
- `err_validation_new_ids_more_than_copies`
- `err_validation_link_over_20_items`
- `err_invalid_time_until_not_after_the_item`
- `err_invalid_time_until_past_the_limit`
- `err_not_found`
- `err_invalid_state_already_linked`
- `err_limit_until_needs_over_200_copies`
- `err_limit_caption_words_over_document_limit`
- `err_overlap_next_item`
- `err_id_exists`

### C.75 `keyframe.set_easing`

Changes how the segment that starts at an existing keyframe moves (RF-080): a named easing, a custom cubic Bezier curve or a named Bezier preset. The value and the time of the keyframe do not change, so it is the command of the curve editor. The core is the only place that evaluates a curve (SDD-004 §3.1).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | |
| `property` | the properties of `keyframe.set` | yes | none | |
| `at` | time | yes | none | absolute position of an existing keyframe |
| `easing` | `"linear"`, `"ease_in"`, `"ease_out"`, `"ease_in_out"`, `"hold"` | one of three | none | exactly one of `easing`, `bezier` and `preset` is given |
| `bezier` | array of 4 numbers | one of three | none | `[x1, y1, x2, y2]`; sets `easing` to `"bezier"`. `x1`, `x2` from 0 to 1, `y1`, `y2` from -2 to 3, at most 3 decimals |
| `preset` | `"in"`, `"out"`, `"in_out"`, `"back"`, `"overshoot"` | one of three | none | a named curve, stored as its four numbers: `in` 0.42, 0, 1, 1; `out` 0, 0, 0.58, 1; `in_out` 0.42, 0, 0.58, 1; `back` 0.36, 0, 0.66, -0.56 (pulls back, then goes); `overshoot` 0.34, 1.56, 0.64, 1 (goes past, then settles) |

**Preconditions.**

1. Arguments match the schema and `at` parses; exactly one of `easing`, `bezier` and `preset`; the `bezier` numbers are in range (`E_VALIDATION`, `E_INVALID_TIME`).
2. The item exists (`E_NOT_FOUND`).
3. The property applies to the item type (`E_TYPE_MISMATCH`).
4. `at` lies in the inclusive range of the item (`E_INVALID_TIME`, `path` `args.at`).
5. The property has a keyframe at `at - start` (`E_NOT_FOUND`, `details.kind` `keyframe`).

**Postconditions.**

1. The keyframe at `t = at - start` has the new easing. A `linear` easing is stored without the key and a non-Bezier easing without `bezier`, as everywhere in the document. Value and `t` of the keyframe and all other keyframes are unchanged.
2. `diff` has one `item.updated` with `keyframes`. `effective` has `at` (the A.4 triple), `t`, `easing` and, for a Bezier, `bezier`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | none or several of the three, a Bezier out of range, an unknown preset | `args.bezier[0] must be from 0 to 1, got 1.5.` | `Keep x1 and x2 from 0 to 1; y1 and y2 may go from -2 to 3.` |
| `E_INVALID_TIME` | malformed `at`, or outside the item | as `keyframe.set` | as `keyframe.set` |
| `E_NOT_FOUND` | unknown item, or no keyframe at `at` | `Property scale of itm_s6t7v8w9x0 has no keyframe at frame 60; keyframes are at frames 30, 90.` | `Use at "30f" (1s), or list the keyframes with project_get.` |
| `E_TYPE_MISMATCH` | the property does not apply | as `keyframe.set` | as `keyframe.set` |

**Inverse.** `patch {collection: "items", id, set: {keyframes: <old>}}`.

**Idempotency.** Same result.

**Example.**

```json
{"op": "keyframe.set_easing", "args": {"item_id": "itm_s6t7v8w9x0", "property": "scale", "at": "1s", "preset": "overshoot"}}
```

**Golden cases.**

- `ok_custom_bezier`
- `ok_preset_overshoot`
- `ok_preset_back_on_crop`
- `ok_bezier_on_mask_width`
- `ok_named_easing_clears_the_bezier`
- `ok_linear_is_stored_absent`
- `ok_last_keyframe_of_the_property`
- `ok_bezier_is_rounded_to_3_decimals`
- `err_validation_none_of_the_three`
- `err_validation_easing_and_preset`
- `err_validation_bezier_x_above_1`
- `err_validation_bezier_y_below_range`
- `err_validation_bezier_wrong_length`
- `err_validation_unknown_preset`
- `err_validation_easing_bezier_is_not_a_name`
- `err_not_found`
- `err_not_found_no_keyframe_at`
- `err_invalid_time_outside_the_item`
- `err_type_mismatch_crop_on_text`

### C.76 `animation.apply_preset`

Applies a layer animation to a visual item (RF-082). The preset expands in the core into ordinary keyframes (SDD-001 §3.8, with a Bezier easing where the preset needs a curve), so the result is edited with the keyframe commands and nothing new reaches the plan, the export or the preview. `animation.list_presets` (C.77) describes the presets.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip`, `text`, `caption` or `shape` item |
| `preset` | `"fade"`, `"slide"`, `"scale"`, `"pop"`, `"push_in"`, `"pull_out"` | yes | none | table below |
| `edge` | `"in"`, `"out"` | no | `"in"` | `in` animates the first `duration` frames of the item, `out` the last `duration` frames |
| `duration` | time, as a length | no | `"0.5s"`; for `push_in` and `pull_out` the whole item | 1 frame up to the duration of the item |
| `direction` | `"left"`, `"right"`, `"up"`, `"down"` | no | `"left"` | `slide` only (else `E_VALIDATION`): the side the item comes from (`in`) or leaves to (`out`) |
| `amount` | number | no | per preset | `slide`: the distance in project pixels, 1 to 10,000, default a quarter of the width (`left`, `right`) or of the height (`up`, `down`); `scale`: the factor at the far end, 0.01 to 20, default 0.5; `push_in` and `pull_out`: the zoom factor, above 1 up to 4, default 1.2; `fade` and `pop` take none (else `E_VALIDATION`) |

Let `v` be the value of the property where the animation meets the rest of the item: its static value, or its keyframed value at the inner edge of the window (the end of an `in` window, the start of an `out` window). For `push_in` and `pull_out` it is the value at the start of the window.

| Preset | Property | `edge` `in` | `edge` `out` | Easing of the segment |
| --- | --- | --- | --- | --- |
| `fade` | `opacity` | 0 to `v` | `v` to 0 | linear |
| `slide` | `x` for `left` and `right`, `y` for `up` and `down` | from `v` minus (`left`, `up`) or plus (`right`, `down`) the distance, to `v` | the reverse | `out` for in, `in` for out (the Bezier presets of C.75) |
| `scale` | `scale` | `amount` × `v` to `v` | `v` to `amount` × `v` | `out` for in, `in` for out |
| `pop` | `scale` | 0.001 to `v` | `v` to 0.001 | `overshoot` for in, `back` for out |
| `push_in` | `scale` | `v` to `v` × `amount` | the same | `in_out` |
| `pull_out` | `scale` | `v` to `v` ÷ `amount` | the same | `in_out` |

**Preconditions.**

1. Arguments match the schema; `duration` parses (`E_VALIDATION`, `E_INVALID_TIME`); `direction` and `amount` are given only where the table takes them (`E_VALIDATION`).
2. The item exists (`E_NOT_FOUND`) and is not an `audio` item (`E_TYPE_MISMATCH`).
3. `duration` is at most the duration of the item (`E_INVALID_TIME`, `path` `args.duration`): a preset longer than its item is refused, not cut.
4. A value the preset would write is valid for the property (`scale` above 0 and at most 20: `E_VALIDATION`, `path` `args.amount`).
5. The property ends with at most 100 keyframes (`E_LIMIT`).

**Postconditions.** Let `D` be the duration of the item and `d` the duration of the preset.

1. For `edge` `in`, the window is `t` from 0 to `d`: the keyframes of the property inside it are removed and two are written, `{0, from}` with the easing of the table and `{d, v}` (the easing of the keyframe that governed `d` before, or linear: the segment it cuts keeps its shape). Keyframes after `d` stay. For `edge` `out`, the window is `t` from `D - d` to `D`: the keyframes inside it are removed and `{D - d, start}` with the easing of the table and `{D, end}` (linear) are written; keyframes before `D - d` stay. When the property had no keyframes, `v` is its static value (R10, which is the frame centre for an unset `x` or `y`).
2. A property the preset does not animate, and the static fields, are not changed. Applying `in` and then `out` to one item gives both ends.
3. `diff` has one `item.updated` with `keyframes`. `effective` has `properties` (the animated property names), `edge`, `duration` (the A.4 triple) and `keyframes` (how many were written).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | unknown preset or direction, `direction` or `amount` where it is not taken, an amount out of range | `args.amount 30 makes a scale above 20 for itm_s6t7v8w9x0.` | `Use a smaller amount.` |
| `E_INVALID_TIME` | malformed or zero `duration`, or longer than the item | `args.duration "5s" is 150 frames; item itm_s6t7v8w9x0 lasts 90.` | `Use a duration of at most "90f".` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | an audio item | `Animation presets apply to visual items; item itm_s6t7v8w9x0 is an audio item.` | `Use keyframe.set with volume for audio items.` |
| `E_LIMIT` | would pass 100 keyframes | as `keyframe.set` | as `keyframe.set` |

**Inverse.** `patch {collection: "items", id, set: {keyframes: <old>}}` or `unset: ["keyframes"]`.

**Idempotency.** Same result when applied twice with the same arguments.

**Example.** 30 fps; text item `itm_s6t7v8w9x0`, `start` 30, `duration` 90, no keyframes.

```json
{"op": "animation.apply_preset", "args": {"item_id": "itm_s6t7v8w9x0", "preset": "fade", "duration": "1s"}}
```

It writes the `opacity` keyframes `{"t": 0, "value": 0}` and `{"t": 30, "value": 1}`.

**Golden cases.**

- `ok_fade_in`
- `ok_fade_out`
- `ok_slide_in_from_left`
- `ok_slide_out_to_the_bottom`
- `ok_slide_with_distance`
- `ok_scale_in`
- `ok_pop_in`
- `ok_pop_out`
- `ok_push_in_the_whole_item`
- `ok_pull_out_on_text`
- `ok_in_then_out_gives_both_ends`
- `ok_replaces_the_keyframes_inside_the_window`
- `ok_keeps_the_keyframes_outside_the_window`
- `ok_shape_item`
- `err_validation_unknown_preset`
- `err_validation_direction_on_fade`
- `err_validation_amount_on_pop`
- `err_validation_scale_above_20`
- `err_invalid_time_duration_longer_than_the_item`
- `err_invalid_time_duration_zero`
- `err_not_found`
- `err_type_mismatch_audio_item`
- `err_limit_101_keyframes`

### C.77 `animation.list_presets`

Describes what `animation.apply_preset` and `keyframe.set_easing` accept, for an agent that has not read this document. It changes nothing: `diff` is empty and so is the inverse.

**Arguments.** None.

**Postconditions.** `effective` has `presets`, a list in the order of the table of C.76, each `{"id", "summary", "properties", "edges", "parameters"}` (`parameters` lists `duration` and, where taken, `direction` and `amount` with their range and default), and `bezier_presets`, an object from each preset name of C.75 to its four numbers.

**Errors.** `E_VALIDATION` when an argument is given.

**Golden cases.**

- `ok_lists_the_presets`
- `err_validation_takes_no_arguments`

### C.78 `clip.replace_media`

Swaps the asset of a clip for another asset of the same kind and changes nothing else: the animation that an external engine rendered again takes the place of the one it replaces (epic 33, SDD-011 section 15). Position, duration, trim (`source_in`), speed, transform, keyframes, effects, transitions and the links to captions stay as they are. Nothing is stretched or trimmed to make the new media fit.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `clip` |
| `asset_id` | asset id | yes | none | a video or image asset of the document, of the same kind as the clip's current asset |

**Preconditions.**

1. Both ids have the form of an identifier (`E_VALIDATION`); `ast_placeholder` does not match it.
2. The item exists (`E_NOT_FOUND`) and is a `clip` (`E_TYPE_MISMATCH`).
3. The clip reads a real asset: a template slot holder (`ast_placeholder`) is `E_INVALID_STATE` with the hint to use `slot.fill`.
4. The new asset exists in the document (`E_NOT_FOUND`).
5. The new asset is a video or an image asset of the same kind as the current one (`E_TYPE_MISMATCH`): a video is replaced by a video, an image (a still or an animated image) by an image.
6. Video: the source range of the clip (R9: `source_in` plus the source frames of its duration, speed included) holds inside the new asset (`E_OUT_OF_SOURCE`, with `details.available` of R9: the length of the new asset and the most the clip may keep).

**Postconditions.**

1. `asset` is `asset_id`. Every other field of the item is unchanged, and so is every other item.
2. The old asset stays in the document (`asset.remove` is a separate command).
3. `diff` has one `item.updated` with `changes.asset`; empty when the asset is the same one. `effective` has `asset_id` and `previous_asset_id`.

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | an id is malformed | `The value of args.asset_id is "x", not an asset id.` | `Use the id of an asset listed by project_get.` |
| `E_NOT_FOUND` | unknown item or asset | `Asset ast_zzzzzzzzzz does not exist.` | `Import the file first, then use its asset id.` |
| `E_TYPE_MISMATCH` | the item is not a clip, or the kinds differ | `Asset ast_k8m2n4p6q9 is an image asset; clip itm_m1n2p3q4r5 plays a video asset.` | `Replace a video with a video; to change the kind, add a new clip with clip.add.` |
| `E_INVALID_STATE` | the clip is a template slot holder | `Item itm_m1n2p3q4r5 is a template slot holder.` | `Use slot.fill to put media in a slot.` |
| `E_OUT_OF_SOURCE` | the clip's range does not fit the new video | `The range of 90 frames from source frame 0 would read to frame 90; asset ast_k8m2n4p6q9 has 60 frames.` | `Use a duration of at most "60f"; or render the animation at the length the clip needs.` |

**Inverse.** `patch {collection: "items", id, set: {asset: <old>}}`.

**Idempotency.** Same result.

**Example.** Clip `itm_m1n2p3q4r5` plays `ast_3c5v8n2w6d`; the animation was rendered again as `ast_k8m2n4p6q9`.

```json
{"op": "clip.replace_media", "args": {"item_id": "itm_m1n2p3q4r5", "asset_id": "ast_k8m2n4p6q9"}}
```

```json
[{"kind": "item.updated", "id": "itm_m1n2p3q4r5", "changes": {"asset": ["ast_3c5v8n2w6d", "ast_k8m2n4p6q9"]}}]
```

**Golden cases.**

- `ok_video_for_video`
- `ok_keeps_trim_speed_and_transform`
- `ok_keeps_keyframes_and_effects`
- `ok_image_for_image`
- `ok_animated_image_for_image`
- `ok_same_asset_noop`
- `ok_longer_video_keeps_duration`
- `err_validation_bad_item_id`
- `err_validation_placeholder_asset`
- `err_not_found_item`
- `err_not_found_asset`
- `err_type_mismatch_audio_item`
- `err_type_mismatch_kind_differs`
- `err_type_mismatch_audio_asset`
- `err_invalid_state_slot_holder`
- `err_out_of_source_new_video_too_short`

### C.79 `audio.processor_add`

Adds an audio processor (epic 34, SDD-004 section 7.1) to the `processors` chain of an item or of a track (SDD-001 section 3.5). The five commands C.79 to C.83 share the rules below.

**Shared rules.** The target is exactly one of `item_id` (a `clip` or `audio` item) or `track_id`; both or neither is `E_VALIDATION`. An unknown item or track is `E_NOT_FOUND`; a `text`, `caption` or `shape` item is `E_TYPE_MISMATCH`. `index` and `to` are 0-based positions in the chain; one outside it is `E_VALIDATION` with the length of the chain in the hint. `params` is an object from the names of the kind (SDD-004 section 7.1) to numbers: an unknown name, or a number outside its range, is `E_VALIDATION` naming the parameter, its unit and its range. The stored `params` of a processor hold only the values that differ from the default, so a value equal to its default is removed and an empty object is not written; an emptied chain removes the `processors` field. The commands do not run on a locked track's items (`E_INVALID_STATE`, as every item command) but a track chain can be edited on a locked track. The inverse is the usual `patch` of the item or track.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | one of the two | none | a `clip` or `audio` item |
| `track_id` | track id | one of the two | none | any track |
| `kind` | string | yes | none | `eq`, `compressor`, `limiter`, `gate`, `deesser` or `noise_reduction` |
| `params` | object | no | `{}` | parameters of the kind, SDD-004 section 7.1 |
| `index` | integer | no | the end | where to insert; 0 to the length of the chain |
| `enabled` | boolean | no | true | false adds it switched off |

**Preconditions.** The shared rules; the chain has fewer than 12 processors (`E_LIMIT`); `profile_start_ms` and `profile_end_ms` are both 0 or the first is at least 1,024 samples (22 ms) below the second, and only on an item (`E_VALIDATION`).

**Postconditions.** The processor is in the chain at `index`; the processors from there on move one place later. `diff` has one `item.updated` or `track.updated` (`processors`). `effective` has `index`, `kind` and `params` (the complete parameters with their defaults).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | no or two targets, unknown kind or parameter, a value out of range, `index` out of the chain, a bad noise profile | `args.params.threshold_db 5 is out of range: -60 to 0 dB.` | `Send a threshold between -60 and 0.` |
| `E_NOT_FOUND` | the item or the track does not exist | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | the item is a text, caption or shape item | `Item itm_s6t7v8w9x0 is a text item and has no audio.` | `Use an audio item or a clip.` |
| `E_LIMIT` | the chain already has 12 processors | `The chain of itm_m1n2p3q4r5 already has 12 processors.` | `Remove one first.` |

**Inverse.** `patch` of `processors` (set the old list, or unset when there was none). **Idempotency.** Not idempotent: a second call adds a second processor.

**Example.**

```json
{"op": "audio.processor_add", "args": {"item_id": "itm_m1n2p3q4r5", "kind": "compressor", "params": {"threshold_db": -24, "ratio": 4}}}
```

**Golden cases.**

- `ok_add_compressor_to_clip`
- `ok_add_eq_with_params_to_audio_item`
- `ok_add_to_track`
- `ok_add_at_index`
- `ok_add_disabled`
- `ok_default_values_are_not_stored`
- `ok_add_noise_reduction_marked_profile`
- `err_validation_no_target`
- `err_validation_both_targets`
- `err_validation_unknown_kind`
- `err_validation_unknown_param`
- `err_validation_param_out_of_range`
- `err_validation_eq_high_pass_between_0_and_20`
- `err_validation_profile_on_track`
- `err_validation_profile_range_inverted`
- `err_validation_index_out_of_range`
- `err_not_found_item`
- `err_not_found_track`
- `err_type_mismatch_text_item`
- `err_limit_13th_processor`

### C.80 `audio.processor_set`

Changes one processor of a chain (the shared rules of C.79).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | one of the two | none | a `clip` or `audio` item |
| `track_id` | track id | one of the two | none | any track |
| `index` | integer | yes | none | the processor, 0-based |
| `params` | object | no | none | the values to change; the others stay. A value equal to the default is removed from the stored `params` |
| `enabled` | boolean | no | none | true or false switches it on or off |

**Preconditions.** The shared rules; at least one of `params` and `enabled` is given (`E_VALIDATION`); `index` names a processor.

**Postconditions.** The `params` are merged and checked as a whole (so a marked noise profile stays consistent). `diff` has one `item.updated` or `track.updated`, none when nothing changed. `effective` has `index`, `kind`, `params` (complete) and `enabled`.

**Errors.** As C.79 without `E_LIMIT`.

**Inverse.** `patch` of `processors`. **Idempotency.** Same result.

**Example.**

```json
{"op": "audio.processor_set", "args": {"track_id": "trk_m1n2p3q4r5", "index": 0, "enabled": false}}
```

**Golden cases.**

- `ok_set_param`
- `ok_set_disabled`
- `ok_set_back_to_default_drops_the_param`
- `ok_set_on_track`
- `ok_set_same_value_noop`
- `err_validation_nothing_to_change`
- `err_validation_param_not_of_the_kind`
- `err_validation_param_out_of_range`
- `err_validation_index_out_of_range`
- `err_not_found_item`
- `err_type_mismatch_text_item`

### C.81 `audio.processor_move`

Moves one processor to another place in its chain: the order is the user's and changes the sound (the shared rules of C.79).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | one of the two | none | a `clip` or `audio` item |
| `track_id` | track id | one of the two | none | any track |
| `index` | integer | yes | none | the processor to move |
| `to` | integer | yes | none | its new place, 0 to the length minus 1 |

**Postconditions.** The processor is at `to`; the ones between the two places shift by one. Moving to the same place changes nothing. `effective` has `index`, `to` and `kind`.

**Errors.** `E_VALIDATION` (no or two targets, `index` or `to` out of the chain), `E_NOT_FOUND`, `E_TYPE_MISMATCH`, as C.79.

**Inverse.** `patch` of `processors`. **Idempotency.** Same result.

**Example.**

```json
{"op": "audio.processor_move", "args": {"item_id": "itm_m1n2p3q4r5", "index": 0, "to": 2}}
```

**Golden cases.**

- `ok_move_first_to_last`
- `ok_move_last_to_first`
- `ok_move_to_the_same_place_noop`
- `ok_move_on_track`
- `err_validation_to_out_of_range`
- `err_validation_index_out_of_range`
- `err_not_found_item`
- `err_type_mismatch_text_item`

### C.82 `audio.processor_remove`

Removes one processor from a chain (the shared rules of C.79).

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | one of the two | none | a `clip` or `audio` item |
| `track_id` | track id | one of the two | none | any track |
| `index` | integer | yes | none | the processor to remove |

**Postconditions.** The later processors move one place earlier; an emptied chain removes the `processors` field. `effective` has `index`, `kind` and `remaining`.

**Errors.** `E_VALIDATION` (no or two targets, `index` out of the chain), `E_NOT_FOUND`, `E_TYPE_MISMATCH`, as C.79.

**Inverse.** `patch` of `processors`. **Idempotency.** Not idempotent: the processor that followed takes the index.

**Example.**

```json
{"op": "audio.processor_remove", "args": {"item_id": "itm_m1n2p3q4r5", "index": 1}}
```

**Golden cases.**

- `ok_remove_middle`
- `ok_remove_last_leaves_no_field`
- `ok_remove_on_track`
- `err_validation_index_out_of_range`
- `err_not_found_item`
- `err_type_mismatch_text_item`

### C.83 `audio.apply_preset`

Replaces the chain of an item or a track with a preset (epic 34; the shared rules of C.79). The presets are `voice` ("voice"), `voice_with_music`, `podcast` and `clean_noisy_room` ("clean up a noisy room"); each is a fixed chain of the processors of SDD-004 section 7.1 with the values of the table below, and the user can then change any of them with `audio.processor_set`.

| Preset | Chain, in order |
| --- | --- |
| `voice` | `noise_reduction` strength 0.5; `eq` high-pass 80 Hz, band 2 250 Hz -2 dB, band 4 3000 Hz +2 dB; `deesser` defaults; `compressor` threshold -20 dB, ratio 3, makeup 3 dB; `limiter` ceiling -1 dB |
| `voice_with_music` | `eq` high-pass 100 Hz, band 2 250 Hz -2 dB, band 4 3500 Hz +2.5 dB; `deesser` defaults; `compressor` threshold -22 dB, ratio 2.5, makeup 2 dB; `limiter` ceiling -1 dB |
| `podcast` | `noise_reduction` strength 0.4; `gate` threshold -55 dB, range -20 dB; `eq` high-pass 70 Hz, band 2 200 Hz -2 dB, band 4 4000 Hz +2 dB; `deesser` defaults; `compressor` threshold -22 dB, ratio 3.5, makeup 4 dB; `limiter` ceiling -1 dB |
| `clean_noisy_room` | `noise_reduction` strength 0.8; `gate` threshold -45 dB, range -15 dB; `eq` high-pass 100 Hz, band 2 300 Hz -3 dB, band 4 3000 Hz +2 dB; `deesser` defaults; `compressor` threshold -20 dB, ratio 3, makeup 3 dB; `limiter` ceiling -1 dB |

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | one of the two | none | a `clip` or `audio` item |
| `track_id` | track id | one of the two | none | any track |
| `preset` | string | yes | none | one of the four ids |

**Postconditions.** The chain is exactly the preset (what was there is replaced). `effective` has `preset` and `processors` (their count). Applying the preset that is already the chain changes nothing.

**Errors.** `E_VALIDATION` (no or two targets, unknown preset), `E_NOT_FOUND`, `E_TYPE_MISMATCH`, as C.79.

**Inverse.** `patch` of `processors`. **Idempotency.** Same result.

**Example.**

```json
{"op": "audio.apply_preset", "args": {"item_id": "itm_m1n2p3q4r5", "preset": "voice"}}
```

**Golden cases.**

- `ok_voice_on_clip`
- `ok_voice_with_music_on_track`
- `ok_podcast_on_audio_item`
- `ok_clean_noisy_room`
- `ok_replaces_the_chain`
- `ok_same_preset_noop`
- `err_validation_unknown_preset`
- `err_validation_no_target`
- `err_type_mismatch_text_item`
- `err_not_found_track`

### C.84 `clip.add_composition`

Places another timeline of the production on a visual track as one `composition` item (SDD-001 section 3.5.1, RF-083). Nothing is copied: the item names the timeline.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `track_id` | track id | yes | none | a visual track |
| `timeline_id` | timeline id | yes | none | a timeline of the production, read through `Env.Timelines` (A.3) |
| `start` | time | no | the end of the last item of the track (`"0s"` on an empty track) | absolute; omitted, the item goes to the first free position at or after the end of the track |
| `source_in` | time | no | `"0s"` | the first frame shown of the nested timeline, in frames of this timeline |
| `duration` | time, as a length | no | the rest of the nested timeline from `source_in`, in frames of this timeline (the nested end frame converted with the frame rates, rounded up) | at least 1 frame |
| `fit` | `"cover"`, `"contain"`, `"none"` | no | `"cover"` | SDD-001 section 3.5.1 |
| `id` | item id | no | generated | R6 |

**Preconditions.**

1. Arguments match the schema and times parse (`E_VALIDATION`, `E_INVALID_TIME`).
2. The track exists and is visual (`E_NOT_FOUND`, `E_TYPE_MISMATCH`); the timeline is in the production (`E_NOT_FOUND`; with no `Env.Timelines` no timeline is).
3. The timeline is not this timeline and does not hold, at any depth, a composition of this timeline (`E_INVALID_STATE`, `details.reason` `composition_cycle`, `details.path` the ids from the nested timeline to this one).
4. The nesting stays within 4: the number of timelines above this one (through `Env.Timelines.Users`), plus one, plus the depth of the nested timeline is at most 4 (`E_LIMIT`, `details.reason` `composition_depth`, `details.depth` the depth it would have).
5. The nested timeline is not empty (`E_INVALID_STATE`); `id` is not used (`E_ID_EXISTS`); the document has fewer than 500 items (`E_LIMIT`).
6. `source_in` is below the converted length `L` of the nested timeline and `source_in + duration` is at most `L` (`E_OUT_OF_SOURCE`). The later edits of the nested timeline do not move this check: a composition that is longer than a shortened nested timeline shows nothing after its end and the lint says so.
7. The item ends at or before 108,000 (`E_INVALID_TIME`) and does not overlap an item of the track (`E_OVERLAP`, R8).

**Postconditions.**

1. A `composition` item is added: `start`, `track`, `timeline`, `duration`, plus `source_in` above 0 and `fit` when not `"cover"`. Every other field is absent.
2. `diff` has one `item.added`. `effective` has `id`, `start`, `source_in`, `duration` (A.4 triples; `requested` is `null` for a default).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad argument | `args.fit "stretch" is not one of cover, contain, none.` | `Use cover, contain or none.` |
| `E_INVALID_TIME` | malformed time, zero duration, item ending after 108,000 | `The item would occupy frames 108000-108099, past the limit 108000.` | `Use a start of at most "7800f" for this 300-frame item.` |
| `E_NOT_FOUND` | unknown track, or a timeline that is not in the production | `Timeline prj_zzzzzzzzzz is not in this production.` | `Use the id of a timeline listed by project_get.` |
| `E_TYPE_MISMATCH` | audio track | `Track trk_a1b2c3d4e5 is an audio track; a composition goes on a visual track.` | `Use a visual track.` |
| `E_INVALID_STATE` | a cycle, or an empty nested timeline | `Timeline prj_f6g7h8j9k0 holds this timeline, so placing it here would make a loop.` | `Use a timeline that does not contain this one.` |
| `E_LIMIT` | nesting deeper than 4, or 500 items | `Placing this timeline would nest compositions 5 deep; the limit is 4.` | `Flatten a level with composition.expand.` |
| `E_ID_EXISTS` | `id` used | `Item id itm_m1n2p3q4r5 already exists.` | `Omit id or use an unused id.` |
| `E_OUT_OF_SOURCE` | the range leaves the nested timeline | `The composition would show frames 0 to 449 of timeline prj_a1b2c3d4e5, which has 300 frames.` | `Use a duration of at most "300f" (10.000s), the most this timeline allows from source_in 0.` |
| `E_OVERLAP` | overlap on the track | `Item itm_m1n2p3q4r5 would occupy frames 100-249 on track trk_f6g7h8j9k0, where item itm_c3d4e5f6g7 already is (frames 90-179).` | `Use start "180f" (6.000s), the next free position, or another track.` |

**Inverse.** `remove {collection: "items", id: <new id>}`.

**Idempotency.** With `id`: the second application returns `E_ID_EXISTS`. Without: the copy overlaps the first (`E_OVERLAP`).

**Example.** 30 fps; empty track `trk_f6g7h8j9k0`; timeline `prj_a1b2c3d4e5` of 300 frames.

```json
{"op": "clip.add_composition", "args": {"track_id": "trk_f6g7h8j9k0", "timeline_id": "prj_a1b2c3d4e5", "start": "0s", "id": "itm_m1n2p3q4r5"}}
```

**Golden cases.**

- `ok_default_duration_rest_of_nested`
- `ok_explicit_duration`
- `ok_source_in`
- `ok_other_frame_rate_converts_the_length`
- `ok_start_omitted_goes_to_the_end_of_the_track`
- `ok_fit_contain`
- `ok_supplied_id`
- `ok_nested_two_levels`
- `ok_depth_exactly_four`
- `err_validation_bad_fit`
- `err_invalid_time_malformed`
- `err_invalid_time_zero_duration`
- `err_not_found_track`
- `err_not_found_timeline`
- `err_not_found_no_production`
- `err_type_mismatch_audio_track`
- `err_invalid_state_itself`
- `err_invalid_state_cycle_direct`
- `err_invalid_state_cycle_indirect`
- `err_invalid_state_empty_timeline`
- `err_limit_depth_five`
- `err_limit_depth_through_the_timelines_above`
- `err_limit_500_items`
- `err_id_exists`
- `err_out_of_source_one_frame_too_long`
- `err_out_of_source_source_in_at_the_end`
- `err_overlap`

### C.85 `composition.create_from_selection`

Moves the selected items into a new timeline of the production and puts one composition item in their place, in one batch (RF-083): the way a creator turns a part they built into something they can reuse. The command is pure: it makes the new document and returns it in `Result.Spawned` (A.3); the service stores it in the same transaction as the commit.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_ids` | list of item ids, 1 to 100 | yes | none | items of this timeline, each once |
| `name` | string, 1 to 120 characters | no | `"Composition"` | the name of the new timeline |
| `timeline_id` | timeline id | no | generated (`prj_`) | the id of the new timeline |
| `id` | item id | no | generated | the id of the composition item |

**Preconditions.**

1. Arguments match the schema; the ids are distinct (`E_VALIDATION`); every item exists (`E_NOT_FOUND`); none is on a locked track (`E_INVALID_STATE`, A.3 rule 5).
2. The document is not a template (`E_INVALID_STATE`): the slots and the colours of a template do not move into a timeline.
3. `timeline_id` is not a timeline of the production and `id` is not an item (`E_ID_EXISTS`).
4. The composition goes on the top-most visual track of the selected items (the smallest `order`; for a selection of audio items only, the top-most visual track of the document, and `E_INVALID_STATE` when there is none) and covers `[first start, last end)` of the selection. An item of that track that is not selected and lies in that range is `E_OVERLAP`.
5. The new timeline has the depth of the deepest composition of the selection plus one, and the production's limit of 4 holds with the timelines above this one (`E_LIMIT`, `details.reason` `composition_depth`). The count of items of this document never grows (n items leave, one comes).

**Postconditions.**

1. The new document has this document's `width`, `height`, `fps`, `background` and `loudness_target`, the given or default `name`, and: the tracks that held selected items, with the same ids, kinds, names and orders (their `locked`, `hidden`, `muted`, `solo`, `volume` and `magnet` kept); the selected items with the same ids, moved earlier by the first start (so the new timeline begins at frame 0); the assets they use and the fonts of this document. An item's `slot` is dropped. A `link` whose other members were not all selected is removed from the moved items, and a caption whose `linked_item` was not selected loses it.
2. This document loses the selected items and gains one `composition` item (`id`) on the track of precondition 4: `start` the first start, `duration` the length of the selection, `timeline` the new id. The tracks stay, empty or not.
3. `diff` has one `item.removed` per selected item and one `item.added`. `effective` has `id`, `timeline_id`, `name`, `start`, `duration` and `moved` (the ids moved, sorted). The inverse restores the selected items exactly; the new timeline is left to the user, who may trash it (SDD-001 section 3.5.1 rule 3).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | no item, more than 100, or a repeated item | `args.item_ids repeats itm_m1n2p3q4r5.` | `Name each item once.` |
| `E_NOT_FOUND` | unknown item | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_INVALID_STATE` | a locked track, a template, or no visual track | `Track trk_f6g7h8j9k0 is locked.` | `Unlock the track first.` |
| `E_ID_EXISTS` | `timeline_id` or `id` used | `Timeline id prj_a1b2c3d4e5 already exists.` | `Omit timeline_id or use an unused id.` |
| `E_OVERLAP` | an item that is not selected lies in the range of the composition | `The composition would cover frames 0-149 of track trk_a1b2c3d4e5, where item itm_s6t7v8w9x0 (frames 60-89) is not selected.` | `Select that item too, or move it to another track.` |
| `E_LIMIT` | nesting deeper than 4 | `Placing this timeline would nest compositions 6 deep; the limit is 4.` | `Flatten a level with composition.expand.` |

**Inverse.** As for any command, the exact inverse of the net change of this document (A.6).

**Idempotency.** With `timeline_id` and `id`: the second application returns `E_ID_EXISTS`.

**Example.**

```json
{"op": "composition.create_from_selection", "args": {"item_ids": ["itm_m1n2p3q4r5", "itm_c3d4e5f6g7"], "name": "Intro", "timeline_id": "prj_a1b2c3d4e5", "id": "itm_k1k2k3k4k5"}}
```

**Golden cases.**

- `ok_one_clip`
- `ok_two_clips_on_two_tracks`
- `ok_selection_not_at_zero_starts_the_nested_timeline_at_zero`
- `ok_with_an_audio_item`
- `ok_a_link_that_is_not_whole_is_removed`
- `ok_supplied_name_and_ids`
- `ok_nests_a_composition`
- `err_validation_no_items`
- `err_validation_repeated_item`
- `err_not_found_item`
- `err_invalid_state_track_locked`
- `err_id_exists_timeline`
- `err_id_exists_item`
- `err_overlap_unselected_item_in_range`
- `err_limit_depth_five`

### C.86 `composition.expand`

Puts the items of a nested timeline on this timeline in place of a composition item that uses no group feature: the inverse of `composition.create_from_selection`. The nested timeline stays in the production, unchanged.

**Arguments**

| Name | Type | Required | Default | Rule |
| --- | --- | --- | --- | --- |
| `item_id` | item id | yes | none | a `composition` item |

**Preconditions.**

1. The item exists and is a composition (`E_NOT_FOUND`, `E_TYPE_MISMATCH`); its timeline is in the production (`E_NOT_FOUND`); its track is not locked (`E_INVALID_STATE`, A.3 rule 5).
2. The item only places its timeline (`E_INVALID_STATE`, `details.reason` and `details.feature`): no `opacity`, `scale`, `rotation`, position, keyframe, transition, mask, effect, shadow, outline, blur, filter, blend or flip of its own, and no volume, fade or mute (`composition_group_feature`); `source_in` 0 and a duration that shows the whole nested timeline (`composition_window`); and the nested timeline has this timeline's `width`, `height` and `fps` (`composition_geometry`).
3. No item id of the nested timeline is used here (`E_ID_EXISTS`); an asset of the nested timeline that has the id of one here has the same file (`E_INVALID_STATE`, `details.reason` `composition_asset_clash`); the document stays within 50 tracks, 500 items and 200 assets (`E_LIMIT`).

**Postconditions.**

1. The composition item is removed. Every track of the nested timeline that holds an item is added here with a new id (`effective.tracks`, in the order of the nested track ids): its visual tracks, keeping their relative order, take the places directly above the track of the item (`order` strings between that track and the one above it), and its audio tracks go after the last track. Every item of the nested timeline is added to its new track with the same id, moved later by the `start` of the item; the assets and fonts it uses are added when missing.
2. `diff` has the `item.removed`, `track.added` and `item.added` entries and the added assets. `effective` has `item_id`, `timeline_id`, `tracks` (the new ids) and `items` (the added ids, sorted).

**Errors.**

| Code | When | Example message | Example hint |
| --- | --- | --- | --- |
| `E_VALIDATION` | bad argument | `args.item_id "x" is not an item id.` | `Use an id like itm_m1n2p3q4r5.` |
| `E_NOT_FOUND` | unknown item, or a timeline that is not in the production | `Item itm_zzzzzzzzzz does not exist.` | `Use the id of an item listed by project_get.` |
| `E_TYPE_MISMATCH` | the item is not a composition | `Item itm_m1n2p3q4r5 is a clip item; composition.expand needs a composition.` | `Use it on a composition item.` |
| `E_INVALID_STATE` | a group feature, a window or a geometry that makes the picture differ, a locked track, or a differing asset | `Item itm_m1n2p3q4r5 has its own opacity, so it is drawn as one unit and cannot be expanded.` | `Remove it first, or keep the composition.` |
| `E_ID_EXISTS` | an item id of the nested timeline is used here | `Item id itm_c3d4e5f6g7 of the nested timeline already exists here.` | `Rename the clash first.` |
| `E_LIMIT` | 50 tracks, 500 items or 200 assets | `Expanding would give the document 501 items; the limit is 500.` | `Delete unused items first.` |

**Inverse.** The exact inverse of the net change (A.6).

**Idempotency.** Not idempotent: the second call is `E_NOT_FOUND`.

**Example.**

```json
{"op": "composition.expand", "args": {"item_id": "itm_k1k2k3k4k5"}}
```

**Golden cases.**

- `ok_one_visual_track`
- `ok_two_visual_tracks_keep_their_order`
- `ok_with_an_audio_track`
- `ok_later_start_shifts_the_items`
- `err_validation_bad_item_id`
- `err_not_found_item`
- `err_not_found_timeline`
- `err_type_mismatch_not_a_composition`
- `err_invalid_state_opacity`
- `err_invalid_state_window`
- `err_invalid_state_other_size`
- `err_invalid_state_track_locked`
- `err_id_exists`
- `err_limit_500_items`
