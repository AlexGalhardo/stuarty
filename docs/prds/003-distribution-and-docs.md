# PRD-003: Distribution, self-hosting and documentation

- Status: Accepted (2026-11-14)
- Depends on: [brainstorming](../brainstorming.md) D-003, D-004, D-005, D-006, D-007, D-008, D-012, D-015, D-016; [licensing](../search/licensing.md); [vukem practices and releases](../search/vukem-practices-and-releases.md)
- Implemented by: SDD-005, SDD-007, SDD-009, SDD-010, SDD-011 (see [traceability](../traceability.md))

## 1. Problem

A tool that handles people's footage and is driven by agents has to be trivial to install, safe to leave running, clear about what it sends out (nothing) and documented in a form both people and agents can read. The project is also the field test of the vukem framework and must follow its release practice.

## 2. Users

| User | Need |
| --- | --- |
| A person on Windows, macOS or Linux | Download one file, run it, edit |
| A self-hoster | A container image and a short configuration |
| A contributor | A repository that explains itself and a release process that is automatic |
| An agent reading the documentation | Plain text pages, a machine-readable index, a reference that matches the binary |

## 3. Functional requirements

Phase: `P` Prototype, `A` ALPHA, `B` BETA.

| ID | Requirement | Phase |
| --- | --- | --- |
| RF-200 | The product is one executable, `vukem-video`, that contains the server, the CLI, the MCP server and the UI assets | P |
| RF-201 | All data lives in one workspace directory with a documented layout; its location is configurable | P |
| RF-202 | Started on the local machine, the UI opens without login; the CLI and MCP server authenticate with a local token; other web origins cannot reach the API | P |
| RF-203 | Exposed on the network, login is mandatory and programmatic access uses API keys with scopes, optionally limited to a project | A |
| RF-204 | The product finds FFmpeg (bundled, on the path or at a configured location), reports its version and licence flags, detects by test which encoders work and tells the user what to do when none does | P |
| RF-205 | Local transcription installs its helper and model through an explicit step (command or button) that states the size and downloads from pinned locations with checksums; no job or agent call triggers a download | A |
| RF-206 | With default settings the product opens no outbound network connection | P |
| RF-207 | A diagnostic command reports versions, FFmpeg details, encoders and workspace health, and can write a bundle for a bug report | A |
| RF-208 | Opening a workspace from an older version migrates it after making a backup; a workspace from a newer version is refused with a clear message | A |
| RF-209 | Releases follow the vukem pattern: tag, title, notes from the changelog, signed binaries for three operating systems and two architectures, SBOM, provenance | P (first release), A (full) |
| RF-210 | A container image published to GHCR and Docker Hub with a pinned LGPL FFmpeg and its licence notices | A |
| RF-211 | A documentation site in English and Brazilian Portuguese with a user guide, an agents section, a generated reference and a self-hosting guide | P (skeleton and reference), A (guide), B (complete) |
| RF-212 | Every documentation page has a plain Markdown twin, and the site has `llms.txt` and a sitemap | P |
| RF-213 | A packaged desktop application with installer, signing and auto-update | B |
| RF-214 | The repository has `README`, `LICENSE` (MIT), `CONTRIBUTING`, `SECURITY`, `CODE_OF_CONDUCT`, `CHANGELOG` and an `AGENTS.md` for contributors. The `ROADMAP` that is the work contract of the agent team lives in the workspace, outside the repository (D-025) | P |
| RF-215 | Third-party notices for everything shipped (FFmpeg, fonts, libraries) are included in each artifact | A |

## 4. Non-functional requirements

| ID | Requirement |
| --- | --- |
| NFR-040 | From download to the first exported video takes under 10 minutes for a new user following the getting-started page |
| NFR-041 | The release archive for one platform, without FFmpeg, is under 60 MB |
| NFR-042 | CI on a pull request completes in under 20 minutes |
| NFR-043 | No artifact we publish bundles or links a GPL or non-redistributable component of ours or of our dependencies; the packages of a container base image are listed with their licences |
| NFR-044 | A release is produced by pushing a tag; the only manual steps are merging the release pull request and pushing the tag |
| NFR-045 | Windows, macOS and Linux are tested in CI on every pull request; Windows 11 itself is tested by the orchestrator before each release |

## 5. Success metrics

1. A release is cut for every completed ROADMAP task group, with notes in the vukem format.
2. `vukem-feedback.md` gains an entry for every friction met with vukem, and none is fixed silently in the application.
3. The reference pages of the site are regenerated in CI and never differ from the binary's catalogue.

## 6. Out of scope

| Feature | Why |
| --- | --- |
| A hosted service, a public demo instance | Decisions D-003 and D-015 |
| Telemetry, crash reporting services | Decision D-016 |
| Bundling a GPL FFmpeg or x264 | Decision D-006 |
| Mobile applications | Not requested |
| Package manager distribution (Homebrew, winget, apt) | After 1.0 |
| Versioned documentation snapshots | After 1.0; vukem keeps one snapshot and the same mechanism can be added |
