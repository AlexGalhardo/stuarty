# PRD-004: Agent studio

- Status: Accepted (2026-11-18)
- Depends on: author decisions D-041, D-042, D-043, D-044; ADR-0031
- Implemented by: SDD-014 (see [traceability](../traceability.md))

## 1. Problem

Agents already edit through the same command core as a person. They also need a place to plan a video, keep its files, choose an engine, and remember what worked, without putting provider clients or keys inside the product.

## 2. Users

| User | Need |
| --- | --- |
| An agent with the studio plugin | Produce a short vertical video from a brief and a clip |
| A person reviewing the result | A folder that shows the brief, the sources and the final file |
| A later session | Learnings from earlier videos, without rereading every folder |

## 3. Functional requirements

Phase: `S` studio.

| ID | Requirement | Phase |
| --- | --- | --- |
| RF-300 | An agent with only the plugin installed can produce a vertical video for each of the three use cases: a game meme cut, a product promo and an authorial video | S |
| RF-301 | Every video has a production folder with the SDD-014 layout | S |
| RF-302 | An agent reads relevant learnings before planning and records new ones after delivery | S |
| RF-303 | An agent picks the simplest engine for the request and writes the reason in `brief.md` | S |
| RF-304 | Every asset has its origin and licence recorded | S |
| RF-305 | An agent reverts a project to a revision in one call (plan 007) | S |
| RF-306 | An agent creates and lists named checkpoints and reads the diff between two revisions (plan 009) | S |

## 4. Non-functional requirements

| ID | Requirement |
| --- | --- |
| NFR-060 | Every skill passes the skill lint |
| NFR-061 | A Haiku-class model completes the pilot of plan 004 |

## 5. Success metrics

1. The pilot of plan 004 exports a 9:16 MP4 that passes that plan's checks.
2. A production folder from that pilot matches SDD-014 section 4.
3. `learnings.md` from the pilot has at least one fact promoted into memory.

## 6. Out of scope

| Feature | Why |
| --- | --- |
| A provider client inside `service/` | ADR-0031 item 2 and ADR-0020 |
| Bundling HyperFrames or Remotion | Plan 008; they stay agent-side tools |
| Changing the command core for the studio | The studio adds no operation, table or error code |
