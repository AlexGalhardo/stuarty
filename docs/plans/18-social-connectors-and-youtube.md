# 18: Social connectors and YouTube

## Goal

A creator connects their YouTube channel once, with their own Google developer project, and from then on a finished video can go from Stuarty to the channel: after they approve it. This epic also builds the frame every other network plugs into.

## Context and related decisions

- ADR-0035 (social connections with the user's own developer apps), ADR-0032 (publishing waits for a person), ADR-0033.
- Author decisions D-050, D-051. PRD-004 RF-400, RF-401 (YouTube), NFR-063, NFR-064.
- Research note `web/02-redes-sociais` (what each official page said on 2026-11-18: quota per upload, the rule for unverified projects, the scopes).

## Scope

- Modules: `core/publish/` (new: the pure rules of a publication target and of validation against a network's limits), `service/social/` (new: the connector interface and its registry), `service/social/youtube/`, the setup guide component in `web/resources/js/pages/Connections/`, `web/app/` (controllers, CLI, MCP), `docs/sdds/020-publishing.md` (new: the connector part), SDD-000, SDD-007 rows.
- The job kind `publish.upload` with one target, used directly ("publish this output now, after approval"). The calendar and scheduling are epic 20.

## Out of scope

- Instagram and Facebook (19), X and TikTok (22), Reddit (44), scheduling and the calendar (20), metrics (45).
- A shared Stuarty developer app: each user registers their own (ADR-0035).

## Dependencies

04, 05. Reuses the sign-in flow of epic 12 when it is merged; otherwise this epic adds the same generic flow to `service/connections/oauth` and epic 12 reuses it.

## Technical design

- **Connector interface** in `service/social`: `Describe()` (network, what it can publish, limits with source and date), `Validate(post)`, `Publish(ctx, post) -> (remote id, URL, state)`, `Status(ctx, remote id)`, and `AuthConfig()` for the sign-in. One package per network; nothing outside the package knows the network's API (NFR-063).
- **Rules in `core/publish`**: the post model (output asset, title, description, tags, visibility, made-for-kids, thumbnail, AI disclosure flag), validation against the connector's declared limits, and the state machine of an upload (`prepared`, `waiting_approval`, `uploading`, `processing`, `published`, `failed`).
- **The user's own app**: the connection stores the client id and secret the user created in their Google project, in the vault; the guide walks through creating the project, enabling the API, the consent screen and the redirect address, with the exact values to paste. The guide's text states what the research found about unverified projects and links to the official page.
- **YouTube adapter**: resumable upload from the output file through `internal/outbound`; visibility defaults to private; thumbnail upload; quota errors and "channel not eligible" mapped to stable, actionable errors; a restart resumes the upload session instead of starting again.
- **Approval**: every `publish.upload` is an approvable action (epic 05) whose summary shows network, channel, title, visibility and the file.
- **Contract tests** (NFR-064): recorded request and response pairs per endpoint, replayed with no network.
- **Service**: `SocialNetworkList`, `SocialConnectBegin`, `SocialAccountList`, `PublishValidate`, `PublishNow`, `PublishStatus`; registered in the `publish` domain.

## Tasks

- [ ] 1. Read the official YouTube pages again; write the connector part of SDD-020 with limits, scopes, quota and the date; add SDD-007 rows (a post to the wrong account, a token used by another connector, description text as untrusted input). Verify: `task lint:md` exits 0.
- [ ] 2. Implement `core/publish` with table tests. Verify: `go test ./core/publish/...` passes.
- [ ] 3. Implement the connector interface, the registry and a fake connector for tests. Verify: `go test ./service/social/...` passes.
- [ ] 4. Implement the sign-in with the user's own client (or reuse epic 12's flow) and the account list. Verify: `go test ./service/social/youtube/... -run TestAuth` passes against a fake authorization server.
- [ ] 5. Implement the resumable upload, the thumbnail and status, with contract fixtures. Verify: `go test ./service/social/youtube/...` passes with no network, including resume after an interrupted upload.
- [ ] 6. Implement `publish.upload` with approval and the state machine. Verify: `go test ./service/... -run TestPublishNow` passes (no outbound call before approval).
- [ ] 7. Expose on HTTP, CLI and MCP with parity. Verify: `go test ./web/contract/...` passes.
- [ ] 8. Build the setup guide and the "publish now" dialog in both languages. Verify: `task test:web` and `web/e2e/specs/youtube-connect.spec.ts` pass with the fake connector.
- [ ] 9. Security tests of the new rows. Verify: `go test -run TestSecurity_ ./...` and `go run ./internal/tools/threatcheck` exit 0.
- [ ] 10. One real private upload by the author with their own project, or marked as not verified. Verify: the result or the note is in the hand-off.

## Test strategy

- Unit: validation and the state machine.
- Contract: every YouTube endpoint used, from recorded pairs, with quota and eligibility errors.
- Integration: approval gate; resume after restart.
- End to end: connect and publish with the fake connector.

## Acceptance criteria

1. With a connection made from the user's own client, `PublishNow` of an exported file ends `published` with the remote id and URL, after a person approved it.
2. Before approval no request leaves the machine (test with a recording destination).
3. An interrupted upload resumes; it does not start again from zero.
4. A quota or eligibility error is shown as an actionable message, and the job can be retried later.
5. Adding the fake connector needed only its package and one registration line.
6. `go test ./core/publish/... ./service/social/... ./web/contract/...`, `go test -run TestSecurity_ ./...` and `task test:web` pass with no network access.

## Risks and mitigation

- **Google's rules for unverified projects change.** Mitigation: task 1 reads them again; the guide links to the official page and shows the date read.
- **A real upload cannot run in tests.** Mitigation: contract fixtures; the manual check is recorded honestly.
- **STOP** if the user's own project cannot upload without a verification the user cannot do alone.

## Size

L

## Hand-off for review

The diff of `epic/18-social-connectors-and-youtube`; the official pages with the date read; fixtures; the outputs of the acceptance commands; whether a real upload was verified.
