# PRD-003: Distribution, self-hosting and documentation

- Status: Accepted (2026-11-14); revised on 2026-11-18 for Stuarty (ADR-0032, ADR-0033, ADR-0034, ADR-0041, ADR-0044, ADR-0045, ADR-0046): the phase column was removed, RF-216 to RF-221 were added and RF-200, RF-206, RF-209, RF-213 and RF-214 were rewritten
- Depends on: author decisions D-003, D-004, D-006, D-008, D-012, D-015, D-016, D-053, D-054, D-055, D-057, D-058, D-068, D-070, D-074, D-078; research notes `licensing` and `web/07-stack-desktop-web-self-hosted`
- Implemented by: SDD-005, SDD-007, SDD-009, SDD-010, SDD-011; the requirements added on 2026-11-18 are specified by the epics that build them (`docs/plans/00-roadmap.md`)

## 1. Problem

A tool that handles people's footage, holds the keys to their social accounts and is driven by agents has to be trivial to install, safe to leave running, clear about what it sends out and to whom, and documented in a form both people and agents can read.

## 2. Users

| User | Need |
| --- | --- |
| A person on Windows, macOS or Linux | Download one file, run it, create |
| A self-hoster with a VPS | A container image, a script and a short configuration |
| A contributor | A repository that explains itself |
| An agent reading the documentation | Plain text pages, a machine-readable index, a reference that matches the binary |

## 3. Functional requirements

| ID | Requirement |
| --- | --- |
| RF-200 | The product is one executable that contains the server, the CLI, the MCP server and the UI assets. It is named `stuarty` (ADR-0044; it was `vukem-video` until release 0.29.0) |
| RF-201 | All data lives in one workspace directory with a documented layout; its location is configurable |
| RF-202 | Started on the local machine, the UI opens without login; the CLI and MCP server authenticate with a local token; other web origins cannot reach the API |
| RF-203 | Exposed on the network, login is mandatory and programmatic access uses API keys with scopes, optionally limited to a project |
| RF-204 | The product finds FFmpeg (bundled, on the path or at a configured location), reports its version and licence flags, detects by test which encoders work and tells the user what to do when none does |
| RF-205 | Local transcription installs its helper and model through an explicit step (command or button) that states the size and downloads from pinned locations with checksums; no job or agent call triggers a download |
| RF-206 | With no connection configured the product opens no outbound network connection. With connections, it reaches only the destinations the user connected, and every one of them is listed in the product (ADR-0032) |
| RF-207 | A diagnostic command reports versions, FFmpeg details, encoders and workspace health, and can write a bundle for a bug report that contains no credential |
| RF-208 | Opening a workspace from an older version migrates it after making a backup; a workspace from a newer version is refused with a clear message. A workspace of the former product name is migrated the same way (ADR-0044) |
| RF-209 | A release has a tag, a title, notes from the changelog, signed binaries for three operating systems and two architectures, an SBOM and provenance. No release is cut while the epics are implemented (ADR-0045) |
| RF-210 | A container image published to GHCR and Docker Hub with a pinned LGPL FFmpeg and its licence notices |
| RF-211 | A documentation site in English and Brazilian Portuguese with a user guide, an agents section, a generated reference and a self-hosting guide |
| RF-212 | Every documentation page has a plain Markdown twin, and the site has `llms.txt` and a sitemap |
| RF-213 | A packaged desktop application with installer and auto-update: the Wails shell around the same binary and the same web UI (ADR-0041). Windows first; macOS and Linux after the webview detection runs on real machines |
| RF-214 | The repository has `README`, `LICENSE` (MIT), `CONTRIBUTING`, `SECURITY`, `CODE_OF_CONDUCT`, `CHANGELOG`, an `AGENTS.md` for contributors and the epics of `docs/plans/` |
| RF-215 | Third-party notices for everything shipped (FFmpeg, fonts, libraries) are included in each artifact |
| RF-216 | Third-party credentials are stored only in an encrypted vault opened by a master key that is kept in the operating system keychain on a desktop and supplied as a secret on a server (ADR-0033) |
| RF-217 | A user can connect a Google account with an OAuth client of their own Google Cloud project; on a server that account can also be the login (ADR-0034) |
| RF-218 | A VPS installation is one script that starts the container image behind Caddy with HTTPS, and a second script that updates it after a backup of the workspace |
| RF-219 | On a desktop, a background service publishes scheduled posts while the window is closed; it is installed with consent and can be turned off (ADR-0046) |
| RF-220 | The Connections section lists every outbound destination, what it may receive, when it was last used, and removes its credentials when the connection is removed |
| RF-221 | The first start under the new product name finds a workspace and the environment variables of the former name, migrates or adopts them after a backup, and says so (ADR-0044) |

## 4. Non-functional requirements

| ID | Requirement |
| --- | --- |
| NFR-040 | From download to the first exported video takes under 10 minutes for a new user following the getting-started page |
| NFR-041 | The release archive for one platform, without FFmpeg, is under 60 MB |
| NFR-042 | CI on a pull request completes in under 20 minutes, once CI is a gate again (ADR-0045) |
| NFR-043 | No artifact we publish bundles or links a GPL or non-redistributable component of ours or of our dependencies; the packages of a container base image are listed with their licences |
| NFR-044 | A release is produced by pushing a tag; the only manual steps are merging the release pull request and pushing the tag |
| NFR-045 | Windows and the web version in Docker are validated for every epic; macOS and Linux desktops are validated by their own epic (ADR-0041) |
| NFR-046 | A copy of the workspace without the master key reveals no credential |

## 5. Success metrics

1. A new user installs the product on a VPS with the two scripts of RF-218 and reaches the login page over HTTPS without editing a file by hand.
2. Every friction met with the vukem framework is recorded when found, and none is fixed silently in the application (ADR-0047).
3. The reference pages of the site are regenerated from the binary's catalogues and never differ from them.

## 6. Out of scope

| Feature | Why |
| --- | --- |
| A hosted service, a public demo instance, an official OAuth client run by the project | Stuarty is self-hosted (author decisions D-003, D-015, D-051) |
| Telemetry, crash reporting services | Author decision D-016 |
| Bundling a GPL FFmpeg or x264 | Author decision D-006 |
| Mobile applications | Not requested |
| Package manager distribution (Homebrew, winget, apt) | Later |
| Versioned documentation snapshots | Later |
| Paid code signing | Author decision D-033, until the author decides otherwise |
