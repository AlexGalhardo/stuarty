# 23: Blog posts and their four destinations

## Goal

A creator writes a blog post in Stuarty, with its images, and sends it where their blog lives: as files for a static site, to WordPress, to a webhook of their own automation, or by letting their own system fetch it from an API. The post is scheduled and approved like any other publication.

## Context and related decisions

- ADR-0046 (blog destinations), ADR-0032, ADR-0033.
- Author decision D-077 (the four destinations from the start). PRD-004 RF-353, RF-420, RF-421, RF-422, RF-423.
- Research note `web/02-redes-sociais` (the section on blog destinations) and `web/05-rotina-criadores`.

## Scope

- Modules: `core/writing/` (the `post` text kind: front matter, slug, images, validation), `core/publish/` (blog targets), `service/publish/destinations/` (new: `files`, `wordpress`, `webhook`, and the read API), `web/app/` (controllers, CLI, MCP, the read API routes), `web/resources/js/pages/Write/` (the post editor) and `Connections/` (the destination forms), SDD-019, SDD-020, SDD-007 rows.

## Out of scope

- A hosted blog served by Stuarty to the public.
- Other content systems (Ghost, Medium, dev.to): one more destination each, later.
- Comments, newsletters, SEO scoring.

## Dependencies

17, 20.

## Technical design

- **The post** (RF-353): a text of kind `post` with title, slug, summary, tags, cover image (an asset), body in Markdown with image references to assets, canonical URL, and a language. `core/writing` validates it and renders it to HTML with the Markdown renderer the repository already has, sanitised; if none exists, a renderer is a dependency decision (STOP).
- **Destinations implement the connector interface of epic 18**, so the calendar, approval, retries and results are the same.
  - **Files** (RF-420): writes `<slug>/index.md` with front matter and an `images/` folder, into a folder the user chose, optionally as a zip. No outbound. The front matter layout is a small template the user can edit (for Astro, Hugo, Jekyll).
  - **WordPress** (RF-421): the REST API with an application password created by the user; uploads images to the media library, then creates or updates the post as draft or published; stores the remote id so a second publish updates instead of duplicating.
  - **Webhook** (RF-422): a POST of a documented JSON payload (the post, its HTML, links to fetch the images from the read API) to a URL the user registered as a connection, signed with HMAC and a timestamp; retries with backoff; the response body is untrusted text.
  - **Read API** (RF-423): `GET /api/v1/posts` and `/api/v1/posts/{slug}` for published posts, with a read-only token scope `posts:read` created from a privileged session; pagination and `ETag`. It serves only what was approved and published to this destination.
- **Security**: the webhook URL host goes in the connection's destinations; private and loopback addresses are refused unless the user explicitly allows local delivery; redirects are not followed to another host. The read API token cannot reach anything else.

## Tasks

- [x] 1. Amend SDD-019 (the post) and SDD-020 (the four destinations, the payload, the signature, the API); add SDD-007 rows (webhook to an internal address, signature replay, HTML injection in a post, the read token reaching other routes). Verify: `task lint:md` exits 0.
- [x] 2. Implement the post kind, validation and sanitised rendering with table tests. Verify: `go test ./core/writing/... -run TestPost` passes.
- [x] 3. Implement the files destination with templates. Verify: `go test ./service/publish/destinations/... -run TestFiles` passes (round trip of a post with two images).
- [x] 4. Implement the WordPress destination with contract fixtures. Verify: `go test ./service/publish/destinations/... -run TestWordPress` passes with no network (create, update, media, auth failure).
- [x] 5. Implement the webhook destination with signing, retries and the address rules. Verify: `go test ./service/publish/destinations/... -run TestWebhook` passes.
- [ ] 6. Implement the read API and its token scope. Verify: `go test ./web/app/... -run TestPostsAPI` passes (only published posts; the token is refused elsewhere).
- [ ] 7. Expose post operations on HTTP, CLI and MCP with parity. Verify: `go test ./web/contract/...` passes.
- [ ] 8. Build the post editor with preview and the destination forms, in both languages. Verify: `task test:web` and `web/e2e/specs/blog.spec.ts` pass.
- [ ] 9. Security tests of the new rows. Verify: `go test -run TestSecurity_ ./...` and `go run ./internal/tools/threatcheck` exit 0.
- [ ] 10. Publish to a real WordPress in a local container, if the author approves pulling the image; otherwise marked as not verified. Verify: the result or the note is in the hand-off.

## Test strategy

- Unit: post validation, slug rules, sanitised HTML.
- Contract: WordPress endpoints from recorded pairs; the webhook payload against its JSON schema.
- Security: one `TestSecurity_` test per new row.
- End to end: write a post, export as files, schedule to a fake webhook.

## Acceptance criteria

1. A post with a cover and two images exports as files that a static site generator can read, with working image paths.
2. Publishing the same post to WordPress twice updates the first post instead of creating a second.
3. A webhook delivery carries a valid HMAC signature and a timestamp, and is retried on a transient failure.
4. A webhook to a private address is refused unless local delivery was explicitly allowed.
5. The read API returns only posts published to it, and its token is refused on every other route.
6. Every destination goes through approval and appears on the calendar.
7. `go test ./core/writing/... ./service/publish/... ./web/contract/...`, `go test -run TestSecurity_ ./...` and `task test:web` pass with no network access.

## Risks and mitigation

- **HTML from Markdown is an injection path.** Mitigation: sanitise in the core; tests with hostile posts.
- **A webhook as a way to reach internal services.** Mitigation: the address rules and no cross-host redirects.
- **STOP** for a Markdown renderer or sanitiser dependency, and before pulling a container image.

## Size

L

## Hand-off for review

The diff of `epic/23-blog-and-its-destinations`; the webhook payload schema; the outputs of the acceptance commands; what was verified against a real WordPress.
