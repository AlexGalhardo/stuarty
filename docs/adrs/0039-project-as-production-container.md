# ADR-0039: A project is the container of a whole production

- Status: Accepted (2026-11-18)
- Deciders: author
- Amends: ADR-0005 and ADR-0009 (what a project is), SDD-005
- Links: author decision D-064; ADR-0006, ADR-0010, ADR-0031, ADR-0038, ADR-0046; PRD-004

## Context

Today a project is one timeline document with its history and its assets. A creator's work crosses a briefing, a script, assets, one or more edits, a thumbnail, the files for each network and the publications. Kept in separate libraries, those pieces must be stitched together by hand, which is the fragmentation Stuarty exists to remove (research note `web/05-rotina-criadores`).

## Decision

1. **A project holds a production**: its briefing, its scripts and texts, its assets, one or more timelines, its image documents, its output pieces (an exported file for a destination) and its publications.
2. **Today's project becomes a timeline inside a project.** The timeline document, its operation log and its commands do not change (ADR-0005, ADR-0006). Existing workspaces are migrated: each old project becomes a project with one timeline.
3. **Each item keeps its own history.** A timeline and an image document each have their own revisions and undo; the project records which items it holds and their order in the production.
4. **Assets belong to the workspace and are used by projects.** The asset library is shared; a project references what it uses, so a logo or a music track is stored once (ADR-0010).
5. **Every surface speaks of projects in this sense.** The identifiers and the names of the agent tools are settled in the specification that the epic writes, before code.

## Alternatives

- **Keep the project as the timeline** and link separate libraries by reference. Rejected: no migration, but the single place stays stitched.

## Consequences

Positive: an agent and a person see the same production from the first question to the published post.

Negative: a migration of the data model and of the agent surfaces, and a change of the workspace layout that must respect the rule that a workspace saved by a released version opens in every later one.
