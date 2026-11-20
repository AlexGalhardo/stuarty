# 19: Instagram and Facebook

## Goal

A creator publishes the same short video as a Reel on Instagram and on their Facebook Page from Stuarty, with their own Meta developer app, after approving each post.

## Context and related decisions

- ADR-0035, ADR-0032, ADR-0046.
- Author decisions D-050, D-051. PRD-004 RF-400, RF-401 (Instagram, Facebook), RF-407 (AI disclosure), NFR-063, NFR-064.
- Research note `web/02-redes-sociais`: on 2026-11-18 the official pages said that an app used only by its own business needs no App Review at Standard Access, that Instagram requires a professional account, that media is taken from a public URL unless the resumable upload is used, and that the resumable upload for video exists only with Facebook Login. Read them again before code.

## Scope

- Modules: `service/social/instagram/`, `service/social/facebook/`, a shared `service/social/meta/` for the sign-in and the Graph client, the setup guide for a Meta app, `core/publish/` (the limits and the fields of these two networks), SDD-020, SDD-007 rows.
- Reels on Instagram; video and Reels on a Facebook Page. Image posts when the image editor exists are a small follow-up inside this epic only if epic 27 is merged; otherwise noted for later.

## Out of scope

- Stories, carousels, comments, direct messages, ads.
- Personal Facebook profiles (the API does not publish to them).
- Hosting the user's media on a public URL run by the project: there is no central Stuarty service.

## Dependencies

18.

## Technical design

- **Sign-in**: Facebook Login with the user's own app id and secret, asking only for the permissions publishing needs; the Page and the linked Instagram professional account are listed after sign-in and the user picks which ones to connect. Page tokens live in the vault.
- **Upload path**: the resumable upload, so the file goes from the machine to Meta and no public URL is needed. If task 1 finds that this path is not available for a case, that case is not offered and the guide says why; a public URL is never invented.
- **Publishing is two steps at Meta** (create a container, wait for processing, publish): the adapter models them inside `Publish` and `Status`, with polling and backoff, and maps the documented error codes (rate limit, unsupported format, account not professional, permission missing) to stable, actionable errors.
- **Limits as data**: duration, size, aspect ratio, caption length and the daily post limit, each with its source and date; `Validate` checks the output file's real properties (from the probe) before any upload.
- **AI disclosure** (RF-407): when an asset used by the output has origin `generated`, the post carries the disclosure flag where the API accepts one, and the approval summary says whether it is set.
- **Connection destinations**: the Graph hosts and the upload hosts are listed in the connection (epic 04).

## Tasks

- [x] 1. Read the official pages again and record, in SDD-020, the flow, the permissions, the limits, the upload path per case and the date; STOP if own-app use now needs a review the user cannot pass alone. Verify: `task lint:md` exits 0 and the links are in the hand-off.
- [x] 2. Implement the shared sign-in and Graph client with a fake server. Verify: `go test ./service/social/meta/...` passes.
- [x] 3. Add the limits and fields to `core/publish` with table tests. Verify: `go test ./core/publish/...` passes.
- [x] 4. Implement the Instagram adapter with contract fixtures (container, processing, publish, errors). Verify: `go test ./service/social/instagram/...` passes with no network.
- [x] 5. Implement the Facebook Page adapter the same way. Verify: `go test ./service/social/facebook/...` passes with no network.
- [x] 6. Implement the disclosure flag from asset origins. Verify: `go test ./service/... -run TestDisclosure` passes.
- [x] 7. Write the setup guide for a Meta app and the account picker, in both languages. Verify: `task test:web` and `web/e2e/specs/meta-connect.spec.ts` pass with the fake server.
- [x] 8. Security tests of the new rows (posting to a Page the user did not pick; a long-lived token in a log). Verify: `go test -run TestSecurity_ ./...` exits 0.
- [ ] 9. One real post per network by the author, or marked as not verified. Verify: the result or the note is in the hand-off.

## Test strategy

- Unit: limits at their boundaries; the two-step state machine.
- Contract: every Graph endpoint used, with its documented errors.
- Integration: approval gate; a processing failure is reported and retryable.
- End to end: connect, pick accounts, publish with the fake server.

## Acceptance criteria

1. A vertical video inside the limits is published as a Reel to a connected Instagram account and to a connected Page, each after approval.
2. A file outside a limit is refused by `PublishValidate` with the exact limit, before any upload.
3. No public URL is created or required by the product.
4. A post that uses a generated asset carries the disclosure flag where the API accepts it.
5. The two adapters needed only their packages and registration lines (NFR-063).
6. `go test ./core/publish/... ./service/social/...`, `go test -run TestSecurity_ ./...` and `task test:web` pass with no network access.

## Risks and mitigation

- **Meta changes permissions or review rules.** Mitigation: task 1; the guide shows the date read and links to the source.
- **Processing takes long or fails silently.** Mitigation: polling with a ceiling and a clear "still processing" state.
- **STOP** at task 1 when own-app use is no longer possible, and when the only path left needs a public URL.

## Size

L

## Hand-off for review

The diff of `epic/19-instagram-and-facebook`; the official pages with the date read; fixtures; the outputs of the acceptance commands; whether real posts were verified.
