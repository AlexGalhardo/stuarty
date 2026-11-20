# ADR-0025: Releases and CI/CD follow the vukem pattern, starting at v0.5.0

- Status: Accepted (2026-11-14); releases and CI as a gate are deferred by [ADR-0045](0045-epics-one-pull-request-ci-and-releases-deferred.md) (2026-11-18) until the epics are done
- Deciders: author (D-005, restriction 2 of the briefing), architect
- Links: vukem practices and releases, author decision D-005, ADR-0011, SDD-009

## Context

Restriction 2: follow vukem's release pattern strictly. The pattern was read from real releases and workflows and is documented with a verbatim release note. The repository already has releases `v0.1.0` to `v0.4.0` from the Studio, and D-005 keeps that history. The author's standing rule is that images go to GHCR and Docker Hub with tags `X.Y.Z`, `X.Y` and `latest`; vukem publishes only to GHCR.

## Decision

1. **Versioning.** Semantic Versioning, annotated tag `vX.Y.Z`, never moved. The first editor release is `v0.5.0`, with a `BREAKING` entry stating that the Studio was replaced. Before 1.0 a minor may break and a patch never does.
2. **Cadence.** One release per ROADMAP task group. Phase mapping: Prototype ends at `v0.5.x`; ALPHA releases are `v0.6.0` upward, one minor per group; BETA continues the `v0.x` line; 1.0 is `v1.0.0` with no release candidates, as vukem decided for itself.
3. **Changelog.** `CHANGELOG.md` in Keep a Changelog 1.1.0 with a permanent `[Unreleased]` section and comparison links. Version sections are generated from Conventional Commits by git-cliff with vukem's parser rules and are never edited by hand. `[Unreleased]` is not written by hand either: the rule that vukem states and does not fully follow (VF-007) is followed here.
4. **Release notes.** Title equal to the tag. Body is the changelog section without its heading: sections Added, Changed, Deprecated, Removed, Fixed, Security; entries `- **<scope>:** Message ([hash](commit URL))`, `**BREAKING:**` first when it applies. An empty body fails the release.
5. **Artifacts.** GoReleaser builds static binaries (`CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w"`) named `vukem-video_X.Y.Z_<os>_<arch>` for linux, darwin and windows on amd64 and arm64, each archive with `LICENSE.md`, `README.md`, `CHANGELOG.md`. `checksums.txt` signed with cosign keyless; an SBOM per archive; a build provenance attestation.
6. **Image.** Multi-architecture image published to `ghcr.io/alexgalhardo/vukem-video` **and** `alexgalhardo/vukem-video` on Docker Hub, tags `X.Y.Z`, `X.Y`, `latest`, signed by digest. It contains the binary, the pinned LGPL FFmpeg with its notices (ADR-0011) and nothing GPL. Base images have fixed version tags.
7. **CI gates**, as in vukem: Conventional Commit check on pull request titles; Go format, lint, vet, tests with `-race` on Linux, macOS and Windows; front-end types and lint; Playwright end to end; markdown lint; secret scan; govulncheck; a coverage gate; the generated-schema check (ADR-0014). Every action pinned by commit SHA.
8. **Branching.** `main` protected: pull requests only, green checks, linear history. Branches `feat/<name>`, `fix/<name>`, `docs/<name>`. One logical change per commit; the ROADMAP task identifier in the commit body (`Refs: P1-ENG-003`).
9. **Repository files** move from `web/` to the root in the first task group (ADR-0001).

## Alternatives

- **Restart at v0.1.0 with a clean history.** Rejected by the author (D-005).
- **Release on a calendar.** vukem releases per task group; so does this project.
- **GHCR only, as vukem did at `v0.43.0`** (its main branch has since added Docker Hub). Contradicts the author's standing rule.

## Consequences

Positive: a contributor who knows vukem's release flow knows this one.

Negative: the pipeline is heavy for a young product (signing, SBOM, provenance, two registries). It is copied from working files, so the cost is adaptation, not invention.

Negative: Docker Hub needs credentials the author must add as repository secrets; until then the Docker Hub push step is the one manual prerequisite, recorded in the ROADMAP.
