# ADR-0024: Documentation site: vukem's Astro shell, static, English and pt-BR, readable by agents

- Status: Accepted (2026-11-14)
- Deciders: author (D-008, D-015), architect
- Links: vukem practices §7, author decision D-008, D-015, SDD-010
- Amendment (TASK-0265, 2026-11-16, decision of the author): the site is English only. Item 3 (two locales), the pt-BR pages, the translation staleness check and the `Translation:` commit footer no longer exist; the text below is the decision as taken on 2026-11-14.

## Context

The briefing requires a complete documentation site inspired by FreeCut's and by vukem's, prepared for pt-BR and English. vukem.dev is a static Astro 7 site with MDX, a typed navigation file, search, `llms.txt`, a Markdown twin of every page and "last updated" dates from git, deployed to GitHub Pages; it has no i18n. The Studio's `web/site` is an older fork of that shell served from Railway. FreeCut's documentation is a route inside its application: 30 pages in 5 sections, and every URL returns the same shell, so nothing can read it without running JavaScript. D-015 hosts only the documentation site; D-008 makes English the first language.

## Decision

1. The site lives in `site/` at the repository root and starts from the **current** vukem `site/`, not from the Studio fork.
2. Static build, deployed to **GitHub Pages** by a workflow on push to `main`. No Railway, no standalone server.
3. **Two locales from the first page**: English at `/docs/...` (default) and Brazilian Portuguese at `/pt-br/docs/...`. Content is one MDX file per page per locale under `site/content/docs/<locale>/<section>/<page>.mdx`. A page missing in pt-BR falls back to English with a visible notice. A CI check lists pages whose English version changed after the pt-BR one.
4. **Sections**, following FreeCut's outline for the user guide and adding what an agent-first product needs: Getting started; Concepts; Editing guide; Agents (MCP, CLI, skill, how an agent verifies its work); Reference (command catalogue, timeline schema, CLI, HTTP API, error codes); Self-hosting; Troubleshooting; Contributing.
5. **Reference pages are generated** from the command registry and the JSON Schemas at build time, so they cannot drift from the binary.
6. **Agent readability is a requirement**: `llms.txt`, `llms-full.txt`, a Markdown twin of every page, real 404 responses, `sitemap.xml`.
7. Kept from vukem: search index built at build time, "last updated" from git checked in CI, a changelog page rendered from `CHANGELOG.md`, link and anchor checks.
8. The in-application help links to the site; no documentation is embedded in the editor.

## Alternatives

- **Documentation as a route of the application**, as FreeCut does. Unreadable by agents and not indexable.
- **Keep the Studio fork.** It lacks the Markdown twin and the checks added to vukem since.
- **English only, pt-BR later.** Retrofitting locale routing is more work than starting with it; the briefing asks for both.

## Consequences

Positive: the same tooling and conventions as vukem; contributors to one know the other.

Positive: the reference is always the binary's truth.

Negative: i18n is new work on top of vukem's shell (routing, fallback, the staleness check) and should be offered back to vukem.

Negative: every user-facing page must be written twice; translation is assigned to the interns with the staleness check as the safety net.
