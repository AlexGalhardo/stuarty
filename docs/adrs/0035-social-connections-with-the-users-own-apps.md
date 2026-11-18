# ADR-0035: Social networks are reached through the user's own developer apps

- Status: Accepted (2026-11-18)
- Deciders: author
- Links: author decisions D-050, D-051, D-079; ADR-0032, ADR-0033, ADR-0046; PRD-004

## Context

Stuarty is self-hosted: nothing depends on a service run by the project. Publishing through official APIs needs a registered developer app on each network. The research read the official pages on 2026-11-18 (research note `web/02-redes-sociais`):

- **Meta (Instagram, Facebook):** an app used only for a business its owner owns or manages runs with Standard Access and needs no App Review. Instagram publishes only to professional accounts and, by default, fetches media from a public URL; a resumable upload of a local file exists for video only, with Facebook Login.
- **YouTube:** since 2026-11-02, uploads from unverified API projects are no longer forced to private.
- **X:** pay per use, with no free tier for new apps; a post with a URL costs much more than one without.
- **TikTok:** an unaudited client can post only in private mode, to private accounts, for at most five users a day, and the audit excludes apps made for private use.
- **Reddit:** every access needs prior approval. **Kwai:** no public publishing API was found.

## Decision

1. **Each user registers their own app on each network** and gives Stuarty its credentials, which go to the vault (ADR-0033). A guided setup per network explains and checks each step.
2. **One connector per network behind one interface**, so a rule change of one network touches one adapter. Limits are read from the network at run time when it publishes them.
3. **Order of delivery:** YouTube, then Instagram and Facebook, then X (optional, with its cost shown before each post) and TikTok as a draft or a private post. Reddit and Kwai come later, if their rules allow.
4. **What a network forbids is stated, not worked around.** No browser automation, no unofficial API, no shared credentials.
5. **Reading metrics is not part of the first connectors.** It needs more permissions and, on X, each read is paid.
6. **An official Stuarty app is not planned.** It would lift the TikTok restriction and remove the setup, at the price of a central service; the question stays open for the author.

## Alternatives

- **An official audited app with a central OAuth service.** Rejected for now: it contradicts self-hosting and needs a company, audits and upkeep.
- **Unofficial automation of the networks' own applications.** Rejected: against their terms, and it risks the user's account.

## Consequences

Positive: no cost and no review for the networks that matter first.

Negative: TikTok cannot be published in public from a self-hosted installation.

Negative: on a desktop, Instagram images and cover images need a public URL that a desktop does not have; the Instagram epic must choose between a temporary tunnel, the user's own server or video only.

Negative: the networks change their rules often (YouTube and X both did in 2026); each connector needs contract tests and a visible status.
