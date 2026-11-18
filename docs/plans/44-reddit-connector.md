# 44: Reddit

## Goal

A creator shares a finished piece in the communities they take part in, from the same calendar, respecting each community's rules.

## Context and related decisions

- ADR-0035, ADR-0032.
- Author decision D-050. PRD-004 RF-400, RF-401 (Reddit).
- Research note `web/02-redes-sociais`: on 2026-11-18 Reddit's official pages said that API access needs the platform's approval of the developer's application. Read them again before code.

## Scope

- Modules: `service/social/reddit/`, its setup guide, `core/publish/` (the fields and limits of a Reddit post), SDD-020.
- Link posts and text posts to a chosen community; a video as a link to where it is already published.

## Out of scope

- Native video upload, comments, voting, moderation, direct messages.
- Posting to many communities at once (it is how accounts get banned).

## Dependencies

**Blocked until Reddit approves the author's application for API access** (the author applies; an agent cannot). Technically after 20.

## Technical design

- **Adapter** on the connector interface of epic 18: sign-in with the user's own approved application; `Describe()` with the limits and their source date; `Validate` checks title length, the community name, and required flair when the community demands one (read from the API at validation time); `Publish` submits and returns the permalink.
- **Rate limits**: the API's headers drive backoff; one post per target, never a loop over communities.
- **Community rules**: the approval summary shows the community's rules text (as untrusted text) next to the post, so the person approving sees them.
- **Contract fixtures** recorded from the real API once access exists.

## Tasks

- [ ] 1. Confirm that the application was approved and read the official pages again; record flow, limits and date in SDD-020. Verify: the approval and the links are in the hand-off; `task lint:md` exits 0.
- [ ] 2. Add fields and limits to `core/publish` with table tests. Verify: `go test ./core/publish/...` passes.
- [ ] 3. Implement the adapter with contract fixtures. Verify: `go test ./service/social/reddit/...` passes with no network.
- [ ] 4. Write the setup guide and show community rules in the approval, both languages. Verify: `task test:web` passes.
- [ ] 5. One real post by the author, or marked as not verified. Verify: the result or the note is in the hand-off.

## Test strategy

- Unit: limits and required flair.
- Contract: submit, rate limit, community not found, flair required.
- End to end with a fake server through the calendar.

## Acceptance criteria

1. A link post scheduled to a community is published after approval and its permalink is recorded.
2. A post missing required flair is refused by validation with the list of valid choices.
3. The approval shows the community's rules.
4. The adapter needed only its package and a registration line.
5. `go test ./core/publish/... ./service/social/reddit/...` and `task test:web` pass with no network access.

## Risks and mitigation

- **Access is not granted, or the terms change.** Mitigation: the epic stays blocked; nothing is built on an unofficial path. **STOP** at task 1.

## Size

S

## Hand-off for review

The diff of `epic/44-reddit-connector`; the approval evidence; fixtures; the outputs of the acceptance commands.
