# ADR-0010: Media store and ingest: every import produces normalized media

- Status: Accepted (2026-11-14)
- Deciders: architect; author (D-003)
- Links: [engine §5, §7](../search/engine.md), [post-mortems §4](../search/post-mortems.md), [performance §3, §6](../search/performance.md), ADR-0003, SDD-004, SDD-005

## Context

The inputs of both use cases are phone footage and screen recordings, which carry the problems that broke other editors: variable frame rate (OBS #13396), rotation and mirroring in the display matrix (Remotion #5509, mediabunny #492), HLG footage from iPhones that looks washed out after conversion (Shotcut forum), long GOPs that make seeking slow. D-003 stores files on disk. The stack draft listed tusd, S3 and OPFS.

## Decision

1. **Import is by reference to a file the owner process can read.** In local mode that is a path; the file is copied into the store. Upload over HTTP and object storage are deferred (D-003). Import by URL is out of the MVP (D-011).
2. The **store** is content-addressed: an asset's identity is the SHA-256 of its bytes. Importing the same file twice yields the same asset.
3. Import starts an **ingest job** that produces derived artifacts, each addressed by `asset hash + kind + profile version`:
   - `probe`: ffprobe data as JSON;
   - `normalized`: constant frame rate equal to the source's nominal rate, display matrix applied, 8-bit 4:2:0 BT.709 limited range (tone-mapped when the source is HLG or PQ), short closed GOP, audio as 48 kHz PCM (never AAC here: its priming and padding break the exact `sample_count` of SDD-001; AAC is used only in the proxy);
   - `proxy`: the same at reduced resolution for the browser;
   - `thumbnails`: a strip of frames for the timeline;
   - `peaks`: audio peaks at several zoom levels.
4. Preview reads the proxy; export reads the normalized media. The original is kept and never modified.
5. An artifact is never changed in place. A new ingest profile is a new profile version and therefore a new artifact.
6. Whether a file can be used is established by actually decoding it during ingest, not by asking the container or a capability API. A file that fails ingest is reported with the FFmpeg error translated into an actionable message.
7. Images and audio-only files follow the same model with the artifacts that apply.

## Alternatives

- **Edit the original directly.** No ingest wait and no extra disk, but every later step must handle variable frame rate, rotation, colour and long GOPs, and the two executors would each handle them differently.
- **Export from the original and preview from the proxy.** Slightly higher quality, twice the code paths to keep in agreement. Rejected for the MVP; revisit if a quality complaint justifies it.
- **Browser-side storage (OPFS) and resumable upload (tus).** For a cloud product, not for local-first.

## Consequences

Positive: a whole class of bugs cannot occur after ingest; the tolerance of ADR-0003 is meaningful because both executors read media with the same frame rate, orientation and colour.

Negative: import is not instant; a clip is usable in the timeline only when its normalized media and proxy exist. The UI and the tools report ingest progress.

Negative: disk use is roughly the original plus the normalized copy plus the proxy.

Negative: export quality has one extra generation of compression. The normalized media is encoded at a high quality setting to keep this small.

The codecs of the normalized media and of the proxy are fixed by SDD-011 §4.2 (MPEG-4 Part 2 in MOV with PCM; VP9 in MP4 with AAC); their patent status is an open point for the author before the first public release (`search/licensing.md` §8 item 6).

Open, to be settled by measurement in the Prototype: the exact tone-mapping filter chain for HLG, tested against real iPhone footage.
