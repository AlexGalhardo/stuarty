# 22: X and TikTok drafts

## Goal

A creator who also posts on X and TikTok handles them from the same calendar, within what those two networks really allow: X as a paid post they see the price of, TikTok as a draft sent to their phone to finish there.

## Context and related decisions

- ADR-0035, ADR-0032, ADR-0046.
- Author decision D-050 (X optional because each post is paid; TikTok only as a draft or private until an audit). PRD-004 RF-400, RF-401 (X, TikTok), RF-407, RF-408.
- Research note `web/02-redes-sociais`: on 2026-11-18 the official pages said that X charges per post and more for a post with a link, and that an unaudited TikTok app can only post privately, for a handful of users, and that the audit excludes personal use. Read them again before code.

## Scope

- Modules: `service/social/x/`, `service/social/tiktok/`, their setup guides, `core/publish/` (limits, fields and the cost of an X post), SDD-020.

## Out of scope

- Public posting on TikTok through an unaudited app, and any way around the audit.
- X features beyond a post with text and one video or image (threads, polls, replies).
- Any unofficial API or browser automation.

## Dependencies

20.

## Technical design

- **X adapter**: sign-in with the user's own app; chunked media upload then the post; the per-post price is data in the connector's `Describe()` with its source and date, shown in the approval and counted against ceilings (RF-408). When the text contains a link, the higher price is shown. The user can edit the prices in settings when X changes them.
- **TikTok adapter**: sign-in with the user's own app; the **upload to inbox** flow, which leaves a draft in the user's TikTok app to finish and publish there. The target's final state is `sent_as_draft`, a state added to `core/publish` that the calendar shows distinctly from `published`. The guide explains the limit plainly and what would change after an audit.
- **AI disclosure** (RF-407): set where each API accepts it, as in epic 19.
- **Limits as data**, with source and date, validated against the real file before upload.
- **Contract fixtures** for every endpoint, replayed with no network.

## Tasks

- [x] 1. Read the official pages of both networks again; record flows, limits, prices and the date in SDD-020; STOP if either no longer offers the path described. Verify: `task lint:md` exits 0 and the links are in the hand-off.
- [x] 2. Add the limits, the cost of a post and `sent_as_draft` to `core/publish` with table tests. Verify: `go test ./core/publish/...` passes.
- [x] 3. Implement the X adapter with contract fixtures. Verify: `go test ./service/social/x/...` passes with no network.
- [x] 4. Implement the TikTok adapter with contract fixtures. Verify: `go test ./service/social/tiktok/...` passes with no network.
- [x] 5. Show cost and the draft state in the approval, the calendar and the publication editor; write the two guides; both languages. Verify: `task test:web` and `web/e2e/specs/x-tiktok.spec.ts` pass with fake servers.
- [ ] 6. One real post on X and one real draft on TikTok by the author, or marked as not verified. Verify: the result or the note is in the hand-off.

## Test strategy

- Unit: cost with and without a link; the draft state in the state machine.
- Contract: both adapters, with rate-limit and permission errors.
- End to end: schedule to both with fake servers and see cost and draft state.

## Acceptance criteria

1. An X target shows its price before approval, and the ledger records it after publishing.
2. A TikTok target ends as `sent_as_draft`, and the UI says the post must be finished in the TikTok app.
3. Neither adapter offers an option the official pages do not allow for an unaudited or unpaid app.
4. Both adapters needed only their packages and registration lines.
5. `go test ./core/publish/... ./service/social/...` and `task test:web` pass with no network access.

## Risks and mitigation

- **Prices and access tiers change often.** Mitigation: prices are editable data with a date; task 1 before code.
- **Users expect a public TikTok post.** Mitigation: the state name, the guide and the approval text say "draft".
- **STOP** at task 1 when a path is gone.

## Size

M

## Hand-off for review

The diff of `epic/22-x-and-tiktok-drafts`; the official pages with the date read; fixtures; the outputs of the acceptance commands; what was verified for real.
