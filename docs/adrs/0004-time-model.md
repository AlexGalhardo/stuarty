# ADR-0004: Time is an integer number of frames at the project's rational frame rate

- Status: Accepted (2026-11-14)
- Deciders: architect
- Links: [engine §2](../search/engine.md), [agents and MCP §8](../search/agents-mcp.md), SDD-001, SDD-002

## Context

Floating-point seconds caused real bugs in the editors studied: Remotion #12603 (preview one frame behind the render because two rounding rules chose the frame), Editly #49 (the same 90 s specification renders 88.70 s at 10 fps and 88.92 s at 30 fps), MLT #69 (audio one frame off). OpenCut replaced float seconds with integer ticks because float "accumulated rounding errors". Agents and people think in seconds and timecode, and a tool that accepts "HH:MM:SS or seconds" forces a model to guess.

## Decision

1. The project has a frame rate stored as a rational `{num, den}` (for example `30/1`, `30000/1001`).
2. Every time quantity in the timeline document (position, duration, source in-point, keyframe time, word time) is a non-negative integer number of frames at that rate. There is no float time anywhere in `core/`.
3. Speed is a rational `{num, den}`.
4. Which source frame is shown at timeline frame `t` is one pure function of integers in `core/plan`.
5. Audio is cut and mixed in samples at 48 kHz; the conversion from frames to samples happens once, in `core/plan` (the audio plan, from integers); the mixer uses the sample positions it is given.
6. On every external surface a time is a string with an explicit unit: `"12.5s"`, `"375f"` or `"00:00:12.500"`. A bare number is rejected. One function in `core/timeval` parses it and rounds to the nearest frame; every response reports the frame applied and its time in seconds.
7. Changing a project's frame rate is an explicit command that rescales every quantity and reports what was rounded.

## Alternatives

- **Integer ticks on a fixed base** (OpenCut uses 120,000 per second). More general, but it allows states off the frame grid, which every command would then have to validate. Also 120,000 does not divide 48,000 Hz.
- **Rational per value, as in OpenTimelineIO.** Two numbers per time and comparison with rescaling; generality a 5 minute editor does not use.
- **Float seconds.** The cause of the bugs above.

## Consequences

Positive: an off-grid state cannot be represented; "which frame is time t" is trivial; numbers in the JSON are small and readable.

Negative: audio edits are limited to one frame of precision, the same as the editors studied.

Negative: changing the frame rate of an existing project is lossy; it is rare and explicit.
