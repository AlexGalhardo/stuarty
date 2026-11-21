# SDD-010: Documentation site

- Status: Accepted (2026-11-14). Drafted by a delegated engineer, audited independently, corrected under the architect's decisions.
- Depends on: SDD-000, SDD-009, ADR-0001, ADR-0014, ADR-0016, ADR-0017, ADR-0024, author decisions D-008, D-015, D-016, D-018, D-019
- Implements: RF-211, RF-212, RF-214 (documentation part), NFR-040 (the getting-started page is the test of that requirement)
- Owner area: senior-d (agent surfaces, documentation site, releases and CI)
- Amendment (TASK-0265, 2026-11-16, decision of the author): the site is **English only**. The Brazilian Portuguese tree, the language switcher, the fallback pages, the translation staleness check and the per-locale endpoints were removed; sections 2, 4 and 6 were rewritten for that, and the old `/pt-br/...` addresses redirect to the English pages. The editor keeps its own pt-BR strings (SDD-012); this document does not govern them.

## Contents

1. [Starting point: what is copied, changed and left out](#1-starting-point-what-is-copied-changed-and-left-out)
2. [Content tree and URLs](#2-content-tree-and-urls)
3. [The navigation file](#3-the-navigation-file)
4. [Canonical links and interface strings](#4-canonical-links-and-interface-strings)
5. [Page template and writing rules](#5-page-template-and-writing-rules)
6. [Removed: the translation check](#6-removed-the-translation-check)
7. [Page list](#7-page-list)
8. [Generated reference pages](#8-generated-reference-pages)
9. [Agent readability](#9-agent-readability)
10. [Glossary and the boundary with UI strings](#10-glossary-and-the-boundary-with-ui-strings)
11. [Search, last updated and the changelog page](#11-search-last-updated-and-the-changelog-page)
12. [Build, checks and deployment](#12-build-checks-and-deployment)
13. [Implementation tasks owned by this SDD](#13-implementation-tasks-owned-by-this-sdd)
14. [Acceptance](#14-acceptance)

Citations of the form `path:line` refer to the vukem repository at `C:\dev\stuarty\vukem\` at the tag `v0.43.0`, that is `v0.43.0:<path>:<line>`, read with `git -C C:\dev\stuarty\vukem show v0.43.0:<path>` (the working tree of vukem is ahead of the tag), unless stated otherwise. **Decision of this SDD** marks a choice made where the ADRs or vukem were silent or contradictory. Values known only when a task runs are written `<recorded by SITE-...>`; the task identifiers are in section 13.

## 1. Starting point: what is copied, changed and left out

ADR-0024 item 1: the site starts from the **current** vukem `site/`, not from the Studio fork. Source of truth for "current": `vukem/site/` at the commit the task `SITE-COPY` records in its report.

> **Note (TASK-0265, 2026-11-16).** Sections 1.2 and 1.5 record the state at the copy and at P1-DOC-001, when the site had a pt-BR mirror. Where they speak of locales, the language switcher, fallback pages or the translation check, those were removed and the site is English only: read sections 2, 4 and 6 for what holds today.

### 1.1 Copied unchanged (then adapted only where section 1.2 says)

| vukem path | Role |
| --- | --- |
| `site/tsconfig.json`, `site/eslint.config.mjs` | TypeScript and ESLint configuration |
| `site/app/globals.css`, `site/app/icon.svg` | Tailwind 4 theme; the icon is replaced by the product icon in `SITE-COPY` |
| `site/components/*.tsx`, `site/components/mdx/*` (`callout`, `card`, `code-block`, `quote`, `steps`, `tabs`, `term`) | Page frame, sidebar, header, search dialog, theme toggle, MDX components |
| `site/lib/{committed-dates,git-dates,highlight,last-updated,mdx,mdx-plugins,remark-code-blocks,search,text,toc,trailing-slash,changelog}.ts(x)` | Rendering, git dates, search, table of contents |
| `site/scripts/{check-last-updated,check-mdx,committed-dates,last-updated,link-anchors}.ts` and their tests | Checks of section 12 |
| `site/src/pages/{404,changelog,index,robots.txt,search-index.json,sitemap.xml,llms.txt,llms-full.txt}` and `site/src/pages/docs/[...slug].{astro,md.ts}` | Pages and endpoints (changed for locales) |
| `site/src/layouts/Base.astro`, `site/src/prerender.tsx` | The page layout every `.astro` page imports and the React prerender entry; without them the copied site does not build |
| `site/src/assets/` | present on vukem main after `v0.43.0` (the animated gopher `svg` of the landing page, commit `0d9fb139`); not in the `v0.43.0` tree. Copied only if the copied landing page imports it, and then replaced by the product mark (the `SITE-COPY` report records which) |
| `site/public/logo.png`, `site/public/icon.png`, `site/public/.nojekyll` | The logo (256 px) and the favicon (64 px), scaled from the official icon `.config/installers/assets/stuarty-icon.png` by `internal/tools/installericon`, and the marker that stops GitHub Pages from running Jekyll |
| `site/.gitignore` | Ignores `node_modules`, `out/` and local env files; checked against the real vukem file so the copied site builds from a clean clone |
| `site/package-lock.json` | Created by `npm install` in `SITE-COPY`; never copied from vukem |
| `scripts/refresh-last-updated.sh` (repository root) | The pre-push hook script (SDD-009 section 15.1) |

**Theme and visual identity of the site.** The site keeps the theme behaviour of the copied shell (its `globals.css` and theme toggle, unchanged): D-024's dark default is a rule of the editor interface (SDD-012 §17), not of the documentation. The site does not import the product's `web/resources/css/app.css`; it keeps the shell's typography, spacing and layout, and takes from `docs/design-system.md` §8.4 only the product mark and the `--brand` colour (when the mark exists), the content style and the shared glossary (section 10), and the rule for screenshots of the UI (dark theme, 1600 by 900, the fixture project of the end-to-end suite, no personal paths visible).

`package.json` dependencies are copied with the exact versions of `vukem/site/package.json` (astro 7.4.7, react 19.3.1, tailwindcss 4.3.3, @mdx-js/mdx 3.1.1, shiki 4.5.0 and the rest; `package.json:15-40`). `SITE-COPY` re-checks each against the newest stable release at copy time (the author's dependency rule: stable only, no `latest`, no pre-release) and records any difference; it does not add a package that vukem's site does not use. The package manager is **npm** with `package-lock.json`, as in vukem and in the Studio's `site/` (Decision of this SDD: the author's global rule prefers Bun unless the project already uses another manager, and both predecessors have an npm lockfile here; the React UI in `web/` stays on Bun). Node 24 in CI (`site.yml:35-37`).

### 1.2 Changed

| File | Change | Reason |
| --- | --- | --- |
| `package.json` | `name: stuarty-docs`; scripts added: `reference`, `reference:check`, `check:translations`, `test:base-path`, `check:built` (section 12.1); `test:scripts` and `test:site` rewritten (section 12.1) | |
| `astro.config.mjs` | `site` and `base` from `SITE_URL`; `trailingSlash: "always"` and `output: "static"`, `outDir: "out"` kept; `redirects` replaced by `"/docs": "/docs/introduction/"` and `"/pt-br/docs": "/pt-br/docs/introduction/"`; the `/docs/0.20` redirect removed; an `i18n` block: `defaultLocale: "en"`, `locales: ["en", "pt-br"]`, `routing: { prefixDefaultLocale: false }` | the site is served under a base path unless a custom domain is set (section 12.4) |
| `lib/site.ts` | `name: "Stuarty"`, `repo: "https://github.com/AlexGalhardo/stuarty"`, `editBase: "https://github.com/AlexGalhardo/stuarty/edit/main/site/content/docs"`, `url` from `SITE_URL`, `base` derived from the URL path | |
| `lib/content.ts` (135 lines in vukem) | loads by locale; no version snapshots; fallback to English; generated pages (sections 2, 4, 8) | i18n |
| `lib/ui-text.ts` | a dictionary per locale instead of one `t` object (section 4.4) | i18n |
| `content/navigation.ts` | localized titles and groups (section 3) | i18n |
| `src/pages/**` | a `pt-br/` mirror of every page; `404.astro` bilingual | i18n |
| `components/header.tsx`, `sidebar.tsx`, `doc-article.tsx` | language switcher, fallback notice, locale-aware links and "Last updated" format | i18n |
| `scripts/check-last-updated.ts` | `isDocPage` also accepts `generated/` paths (section 11.2) | |
| `README.md` of `site/` | rewritten for this repository (how to run, add a page in both locales, regenerate the reference) | |
| every copied `.ts`, `.tsx`, `.astro` and `.mjs` file | reformatted from vukem's two spaces to tabs of width 4, in one isolated `style(site)` commit made right after `SITE-COPY` (and apart from the `style(web)` commit of `web/`, SDD-009 section 15.2), so that no content commit mixes formatting | the repository rule is tabs everywhere except YAML, Markdown and tool-written JSON (SDD-009 section 15.2); acceptance 17 of SDD-009 covers `site/` |

### 1.3 Left out

| vukem path | Why |
| --- | --- |
| `site/content/docs/**` | vukem's pages are about vukem |
| `site/content/versions/0.20/**`, `lib/versions.ts`, `components/legacy-path-redirect.tsx`, `scripts/{legacy-path-redirect,current-equivalent}.test.ts` | versioned snapshots are out of scope until after 1.0 (PRD-003 section 6) |
| `site/public/CNAME` | not used here: with `build_type: workflow` the custom domain (`stuarty.com`) is a repository setting (section 12.4) |
| `site/src/pages/index.astro` content | the landing page is rewritten with the structure of <https://www.freecut.net/> (hero with a run/download call to action, feature tour, agents, open source and local-first, links to docs and repository) and only Stuarty copy, mark and colours (D-028; it replaced the vukem structure of hero, three cards and a command block) |

### 1.4 Taken from neither the Studio fork nor FreeCut, and why

From the Studio's `web/site` (older fork of the same shell, research note `notes/vukem` section 4.2): nothing is taken. It lacks the Markdown twin, the anchor check and the trailing-slash tests, and its extras are not wanted: the standalone Node server `scripts/serve.ts` and `static-path.ts` and the `Dockerfile` for Railway (D-015: only a static site on GitHub Pages is hosted), the six Studio categories, and the `/` redirect to `/docs/introduction`. Its package name `studio-docs` and its Railway deployment notes are removed with the Studio (SDD-009 section 17).

From FreeCut's documentation (research note `notes/vukem` section 4.3): the **outline** of the user guide only (section 7). Its **implementation** is not taken: the documentation is a route inside the React application, every URL returns the same 1,408-byte shell, and there is no sitemap, `robots.txt` or `llms.txt`, so neither a search engine nor an agent can read it without running JavaScript. This product's main readers are agents, so a page must be real HTML and real Markdown at its own URL.

### 1.5 What P1-DOC-002 built, and where it differs from sections 1.1 to 1.4

The shell was copied from vukem commit `c9a3fcca` (the last commit that touched `site/`; the clone was at `c4949ed0`, the head of vukem pull request 222), with the dependency versions of its `package.json` unchanged (Astro `7.4.7`, no package added). The site lives in `site/` and `.github/workflows/site.yml`.

- **Base path.** `lib/site.ts` reads `import.meta.env.SITE` and `import.meta.env.BASE_URL` (both set by `astro.config.mjs` from `SITE_URL`, default `https://stuarty.com`, so `BASE_URL` is `/`) and exports `withBase`, used by `Link` and by every component that wrote a root-relative path. Astro does not add the base to the destination of a `redirects` entry, so `astro.config.mjs` writes `/docs` to `<base>/docs/introduction/` in full. `robots.txt` has no `Host:` line.
- **No versions, no i18n yet.** `lib/versions.ts`, the snapshot tree, `legacy-path-redirect` and the `/pt-br` redirect script of `404.astro` are not copied; `lib/content.ts`, the sidebar, the header, `doc-article` and `check-mdx` have no version parameter. Only English docs exist (an introduction and `quick-start`, "Getting started"); the locale layout of sections 2 to 4 comes with P1-DOC-001. The landing page has an English and a Brazilian Portuguese version (`lib/landing.ts`, `src/components/Landing.astro`, `src/pages/index.astro`, `src/pages/pt-br/index.astro`).
- **Landing page (D-028).** Hero, feature tour (each item marked built or planned from PRD-001), how agents drive the editor, open source and local-first, footer. Copy, mark and colours are the product's own (violet `#7c5cff` mark; dark by default, D-024: the theme script and `ThemeToggle` default to `dark` instead of `system`); `--faint` was darkened in both themes and the primary button uses `#6446f0` so text meets contrast 4.5:1. The vukem gopher asset, `lib/landing.ts` of vukem (Go sample code) and `public/CNAME` are not copied; `public/.nojekyll` is.
- **Landing page, one viewport (TASK-0264, 2026-11-16).** The structure follows <https://vukem.dev> (hero with a terminal block and four check-marked points, a link to the quick start), with the Stuarty tokens, and replaces the long page of D-028. On a window at least 1024 px wide and 700 px tall it is exactly one viewport with two columns and no scroll (`fit` variant in `app/globals.css`; on a window up to 896 px tall the base type scale drops to 100%, and the point list is hidden under 760 px); on a smaller window the columns stack and the page scrolls. Left column: the status pill with the latest version, the title, the lead, and the one command that starts the latest release with Docker. Right column: a tabbed gallery of the editor with five features (captions; cuts; chroma key, blend modes and masks; agents through MCP and the CLI; export), each with a screenshot, a short text and the operation it is driven by. The screenshots are placeholders (`public/placeholders/*.svg`, 1600 x 1000, marked in the image) to be replaced by real ones. `lib/landing.ts` holds the copy (English only), `lib/latest-release.ts` reads the newest `## [X.Y.Z]` heading of `CHANGELOG.md` at build time (no network) and builds the command from the documented flags of the Docker page (port published on `127.0.0.1`, the `/data` volume, `APP_KEY`, the first administrator): the version cannot be older than the changelog that the release commit updates.
- **Landing page, the studio in takes (2026-11-21).** The author chose the viewport version of `docs/brainstorming-design-system-and-landing-page.md` section 5.1; it replaces the one-viewport page above, and its look is `docs/design-system.md` section 11. `src/pages/index.astro` has four takes (the hero, the timeline of the product screens, the agents and the closing lines). On a window at least 64 rem wide and 45 rem tall one take fills the screen with no scrollbar, and the wheel, a swipe, the arrows, Page Up and Down, Space, Home, End, Tab and the URL hash move between them (`src/landing/viewport.ts`); on a smaller window, and without JavaScript, the page is a normal scroll with every take and every screen of the timeline in it. Under `prefers-reduced-motion` the takes crossfade, the title is not typed and nothing loops. three.js 0.187.0 (`src/landing/stage.ts`, imported after the load event when the browser is idle, with a static gradient when WebGL fails) and GSAP 3.15.1 (`src/landing/motion.ts`) are dependencies of the site only. The buttons (Getting Started, Setup Docker Local, Setup Docker VPS and a download per system) show the latest release: `lib/releases.ts` reads it from the GitHub API when the site is built, never in the browser, with `GITHUB_TOKEN` when the environment has one, and falls back to its committed `FALLBACK_TAG` on any failure, so the build never fails on it; `site.yml` also runs on `release: published`. `/landing/viewport/` redirects to `/`. `/landing/classic/` and `/landing/creative/` stay as backups out of every index: `noindex, nofollow`, a `Disallow` line each in `robots.txt`, absent from `sitemap.xml`, `llms.txt` and the search index, and linked from no page. `check:built` lists `gsap.com` among its inert hosts: a warning inside the GSAP bundle names it, and nothing loads it.
- **`check:built` exists early.** `scripts/check-built.ts` (and its test) implements only the properties P1-DOC-002 needs: static files only, no resource from another origin and no host other than the inert ones listed in the script, and every root-relative `href` or `src` carrying the base path. The checks on twins, `llms.txt` and the section 4.3 table remain for P1-DOC-001. `test:scripts` includes its test.
- **Not copied yet:** `scripts/refresh-last-updated.sh` and the `docs-last-updated.yml` workflow (CI owner), `check:translations`, `reference`, `reference:check`, `test:base-path`. `site.yml` therefore has no `setup-go` step and no `reference:check`; they are added with P1-DOC-001.
- **What P1-DOC-001 built, and where it differs from sections 2 to 9.** Both locales are served (sections 2 and 4): the content is `content/docs/en/`, `content/docs/pt-br/` and `content/generated/`; `lib/page-files.ts` finds the file of a page (its own, else the generated one, else the English one as a fallback); `lib/locale.ts`, `lib/locale-context.tsx` and `lib/ui-text.ts` (a typed dictionary per locale) carry the locale to the components, and `Link` rewrites `/docs/...` to `/pt-br/docs/...` in a Portuguese page. The switcher is two plain links (`EN`, `PT`) in the header; hreflang, canonicals, the sitemap alternates, the fallback notice, the twins, `llms.txt`, `llms-full.txt` and the search index exist per locale, and the search folds accents. `navigation.ts` has `title` per locale and a flat `pages` list per category (no sub-groups yet). `check:translations` (section 6, with `translations.config.json`), the extended `check:mdx` and `check:built` (twins, `llms.txt`, the section 4.3 table, the bilingual 404) and `docs-last-updated.yml` exist. The reference is generated by `scripts/generate-reference.ts` into `content/generated/reference/` (committed; `reference:check` and `site.yml` fail on a difference). It reads `core/schema/commands.json` (one page per public command), `core/schema/errors.json` with the meanings of SDD-000 section 8, `core/schema/timeline.v1.schema.json`, the table of SDD-000 section 10, `content/glossary.json`, `site/generated/cli.json`, which `go run ./internal/tools/clidoc` writes from the declarations of `web/app/cli`, and `site/generated/openapi.json`, which `UPDATE_OPENAPI=1 go test ./web/contract/ -run OpenAPI` writes from the running API (the test fails when the file is stale). The warmup mode of vukem's `openapi:export` cannot see the routes that need the Service, so the document comes from the contract environment instead; the binary has no `command:help --json` with a `cli` block and no `configref` yet, so section 8.1 is met with these sources. Generated pages have English text in both locales and a note says so in Portuguese. The first pages written in both locales: getting started (introduction, installation, quick start, workspace, projects), concepts (how it works, time, commands and revisions, tracks), guide (media library, timeline, editing tools, text and captions, undo, exporting) and agents (overview, connect, MCP, CLI, verify, batches). **Not written:** the other pages of section 7 (concepts and guide pages, self-hosting, troubleshooting, contributing, `reference/workspace-layout`), `check:translations` ran report-only then (`strict` is true since P3-DOC-002), and `scripts/locale-routing.test.ts` of section 12.1 does not exist (`check:built` covers the locale properties on the built output); `scripts/base-path.test.ts` exists since TASK-0168.
- **Formatting.** The copied `.ts`, `.tsx`, `.astro` and `.mjs` files were reformatted to tabs with Biome (indent tab 4, line width 120) in the isolated commit `style(site)`; the `.astro` templates were re-indented by hand because Biome formats only their front matter.

## 2. Content tree and URLs

### 2.1 Language

The site is in English (`<html lang="en">`), the default and only language (D-008). The editor has its own interface languages, specified in SDD-012; they have no pages here. Adding a language to the site would be a change of this SDD.

### 2.2 Directory layout

```text
site/
├── package.json
├── astro.config.mjs
├── content/
│   ├── navigation.ts                      sidebar: sections, slugs, titles
│   ├── glossary.json                      the product terms (section 10)
│   ├── last-updated.json                  dates from git, generated, committed
│   ├── docs/en/
│   │   ├── getting-started/introduction.mdx
│   │   ├── concepts/<page>.mdx
│   │   ├── guide/<page>.mdx
│   │   ├── agents/<page>.mdx
│   │   ├── reference/<page>.mdx           hand-written reference pages only
│   │   ├── self-hosting/<page>.mdx
│   │   ├── troubleshooting/<page>.mdx
│   │   └── contributing/<page>.mdx
│   └── generated/                         generated pages (section 8)
│       └── reference/
│           ├── commands/<command>.mdx
│           ├── commands.mdx  timeline-schema.mdx  cli.mdx
│           ├── http-api.mdx  error-codes.mdx  configuration.mdx
│           ├── glossary.mdx                   generated from glossary.json (section 8.2 step 3)
├── generated/                             intermediate JSON written by the binary (section 8)
│   ├── commands.json  openapi.json  config.json  errors.json
├── lib/  components/  scripts/  src/pages/  app/  public/
└── out/                                   build output, git-ignored
```

### 2.3 File naming rules

1. A page is `content/docs/en/<dir>/<slug>.mdx`.
2. `<slug>` is lower-case ASCII kebab-case, at most 48 characters. `<dir>` is one of the directories of section 3.
3. Only `.mdx` files are pages. Images live in `site/public/img/<dir>/<slug>/`; text inside an image is avoided.
4. A file that is not listed in `navigation.ts` fails `check:mdx`; a slug listed in `navigation.ts` whose file is missing fails `check:mdx`.
5. A page enters `navigation.ts` in the pull request that adds its file.

### 2.4 URL scheme

| Resource | URL |
| --- | --- |
| Landing page | `/` |
| Docs entry | `/docs/` redirects to `/docs/introduction/` |
| Getting started page | `/docs/<slug>/` (prefix is empty, as vukem `navigation.ts:19-22`) |
| Any other page | `/docs/<dir>/<slug>/` |
| Markdown twin | `/docs/<dir>/<slug>.md` |
| Generated page | `/docs/reference/commands/<command>/` |
| Search index | `/search-index.json` |
| `llms.txt`, `llms-full.txt` | `/llms.txt`, `/llms-full.txt` |
| Changelog | `/changelog/` |
| Sitemap, robots, 404 | `/sitemap.xml`, `/robots.txt`, `/404.html` |
| Old Portuguese addresses | `/pt-br/`, `/pt-br/docs/...` and `/pt-br/changelog/` redirect to the English page at the same path (static redirects of `astro.config.mjs`: an HTML page with a refresh and a canonical link; GitHub Pages cannot send an HTTP 301). Other old `/pt-br/...` files (twins, `llms.txt`, the search index) are gone and answer 404 |

All URLs have a trailing slash, as in vukem (`astro.config.mjs`: `trailingSlash: "always"`, tested by `scripts/trailing-slash.test.ts`). With a base path (section 12.4) every URL above is prefixed by it; no code builds an absolute URL without `site.base`, enforced by `scripts/base-path.test.ts`. With a base path, crawlers read `robots.txt` and `sitemap.xml` only at the host root, so under a project site they are not read; the production site is on a custom domain (section 12.4), so they are.

## 3. The navigation file

`site/content/navigation.ts` keeps the shape of vukem's (`navigation.ts:1-14`: `{title, dir, prefix?, pages}`). Page titles are **not** in this file: a page title is the `title` of the front matter of the file, as in vukem (`lib/content.ts` `readDoc` requires `title` and `description`). Only section titles are here, as plain strings. The shipped file lists each section's pages in one flat `pages: string[]`; the `groups` of the sketch below are the intended shape once a section needs sub-headings.

```ts
/** Pages listed by hand, in sidebar order, or expanded from a generated source. */
export type NavPages = string[] | { generated: "commands" };

export type NavGroup = {
  /** Sidebar sub-heading; omitted when the section is not grouped. */
  title?: string;
  pages: NavPages;
};

export type NavCategory = {
  /** Folder under content/docs/en/ (generated sections live in content/generated/). */
  dir: string;
  /** URL segment after /docs/; defaults to dir; "" puts the pages at the root of /docs. */
  prefix?: string;
  title: string;
  groups: NavGroup[];
};

export const navigation: NavCategory[] = [
  {
    dir: "getting-started",
    prefix: "",
    title: "Getting started",
    groups: [{ pages: ["introduction", "installation", "quick-start", "workspace", "projects"] }],
  },
  {
    dir: "guide",
    title: "Editing guide",
    groups: [
      { title: "Core editing", pages: ["media-library", "timeline", "editing-tools", "preview", "clip-properties", "undo-and-history"] },
      { title: "Output", pages: ["exporting", "jobs", "encoders-and-presets"] },
    ],
  },
  // concepts, agents, reference, self-hosting, troubleshooting, contributing follow the page list of section 7
];
```

Rules:

1. `navigation.ts` has no logic beyond the type and the array. `lib/content.ts` resolves a `{ generated: "commands" }` entry by reading `site/generated/commands.json` and producing one page per command (section 8).
2. The order of `groups` and `pages` is the sidebar order and the previous/next order.
3. Validation (`check:mdx`): every `title` is non-empty; `dir` and `prefix` are unique; a slug appears once; every listed slug has a file or a generated source.
4. The URL of a page is `/docs/<prefix or dir>/<slug>/`.

### 3.1 The sidebar hierarchy (TASK-0268, 2026-11-16)

The sidebar follows the order of the work, as the documentation of other editors does (FreeCut, Kdenlive, OpenShot and Final Cut Pro; the research note is private): Getting started, Tutorials, Concepts, Core editing, Creative tools, Templates, Output, The editor, Agents, Self-hosting, Reference, Commands, Glossary (TASK-0267, added when the section exists), Troubleshooting, Contributing. A category is a sidebar title plus a list of pages, and several categories may share one folder: Tutorials, Core editing, Creative tools, Templates, Output and The editor are all `guide/<slug>`, so a page URL never depends on its group and moving a page between groups changes no link. The section 7 tables list the pages by folder; the sidebar groups are in `content/navigation.ts`.

## 4. Canonical links and interface strings

### 4.1 Canonical links

Every HTML page emits, in `<head>`, one `<link rel="canonical">` with its own absolute URL (`site.url` plus `site.base` plus the path, with the trailing slash) and `<link rel="alternate" type="text/markdown" href="<twin URL>">` when it has a Markdown twin, so an agent that fetches HTML can find the twin. There is no `hreflang`: the site has one language. The 404 page has no canonical. `scripts/check-built.ts` verifies on the built output that each docs page has `lang="en"`, no hreflang alternate and exactly one canonical that is the page itself and exists in `out/`.

### 4.2 Interface strings of the site

`lib/ui-text.ts` exports the English strings of the **site chrome** as one typed object (`export const ui: UiText`): skip link, header and navigation labels, search texts, last-updated and edit labels, pagination, 404 texts. They are not UI strings of the editor (section 10).

## 5. Page template and writing rules

### 5.1 Front matter and structure

Follows vukem's page style (`site/README.md` "Add a page"; research note `vukem-practices-and-releases` section 7: "The idea", "Quick start", "Common mistakes", "Next steps", "You will learn").

```mdx
---
title: Trim a clip
description: One sentence shown under the title and in search results.
since: "0.5.0"
---

## You will learn

- one to four bullets, each a verifiable outcome

## The idea

Two to five sentences: what the thing is and why it exists, in the product's own terms (see the glossary).

## Quick start

<Steps>
1. A numbered step with the exact action or command.
2. The expected result, stated, so the reader knows it worked.
</Steps>

## Common mistakes

- What goes wrong, why, and how to fix it. Each item names the error code when one exists.

## Next steps

- [Link to the next page](/docs/guide/exporting/)
```

Rules:

1. `title` and `description` are required. `since` (the first release in which the page is true) is optional. `generated: true` appears only in generated pages.
2. "The idea" and "Next steps" are required in every page. "Quick start" is required in every page that teaches an action. "Common mistakes" is required where an error code or a known failure exists. "You will learn" is required in guide and getting-started pages.
3. Every action that exists in the UI, the CLI and the agent tools is shown for each surface that has it: a `Tabs` block with `UI`, `CLI`, `MCP` and `HTTP` tabs, the command names being those of SDD-000 section 1. The CLI tab shows the exact command; the MCP tab shows the tool name and the arguments object.
4. A feature that is not implemented in the released version carries `<Callout type="warning">Planned</Callout>`, as vukem's `site/README.md` says. The callout is removed in the pull request that ships the feature.
5. Page links are root-relative `/docs/...` links with a trailing slash; `check:mdx` fails a link to a page that does not exist (vukem's check, `scripts/check-mdx.ts`), and `link-anchors.ts` fails a fragment that does not exist.
6. No bare `{` or `<` in prose (MDX), no HTML other than the MDX components, code lines at most 100 characters (`MAX_CODE_LINE`, `scripts/check-mdx.ts:16`). A fenced block names its language.
7. Code, command names, flag names, error codes, field names and identifiers (`E_REVISION_CONFLICT`, `prj_...`) are written exactly as the program prints them.
8. No marketing text, no unsupported performance claim. A number in a page is either measured (with the source) or taken from a requirement identifier.

### 5.2 Who writes what

| Work | Who | Notes |
| --- | --- | --- |
| English page of a feature | the senior who implements the feature, in the same pull request | a feature task is not done without its page (Definition of Done in the ROADMAP) |
| English pages of Agents, Reference (hand-written), Self-hosting, Contributing, Getting started | `senior-d` | |
| Generated reference | nobody: generated (section 8) | |
| Review | the orchestrator | accepts a page when `check:mdx` passes, the facts match the code and the generated reference, and the glossary terms are used |

Every user-facing page has an English file. A feature page is part of the Definition of Done of the feature.

## 6. Removed: the translation check

The translation staleness check of ADR-0024 item 3 (`check-translations.ts`, `translations.config.json`, the `Translation: not-needed` commit footer and the pt-BR twin of every page) was removed with the Portuguese tree (TASK-0265). Nothing replaces it: with one language there are no pairs to compare. `check:headings`, which compared the headings of the two trees, was removed for the same reason. The "Last updated" check (section 11.2) stays.

## 7. Page list

A page is written when its feature exists and is updated as features land (RF-211). Source: **H** hand-written, **G** generated (section 8). Slugs are `<dir>/<slug>`; the Getting started pages have no `<dir>` in the URL.

The user guide follows the FreeCut outline of research note `notes/vukem` section 4.3 (Start, Core editing, Creative tools, Output, Reference), without the FreeCut pages for features this product does not have (source monitor, motion workspace, scene browser with AI analysis; their purpose is covered by Transcription, Silence removal and Templates). The Agents section is specific to this product.

### 7.1 Getting started (`getting-started`, no URL prefix)

| Page title | Slug | Source |
| --- | --- | --- |
| Introduction | `introduction` | H |
| Installation | `installation` | H |
| Quick start: your first export | `quick-start` | H |
| Workspace and storage | `workspace` | H |
| Projects | `projects` | H |

`quick-start` is the page NFR-040 measures: from download to the first exported video in under ten minutes, written for a person and ending with the agent route in a "Next steps" link.

### 7.2 Concepts (`concepts`)

| Page title | Slug | Source |
| --- | --- | --- |
| How Stuarty works | `concepts/how-it-works` | H |
| Time and frames | `concepts/time` | H |
| Commands, batches and revisions | `concepts/commands-and-revisions` | H |
| Tracks, clips and items | `concepts/tracks-clips-items` | H |
| Render: one plan, two executors | `concepts/render` | H |
| Local mode and network mode | `concepts/local-and-network-mode` | H |

### 7.3 Editing guide (`guide`)

| Group | Page title | Slug | Source |
| --- | --- | --- | --- |
| Core editing | Media library | `guide/media-library` | H |
| Core editing | Basic timeline editing | `guide/timeline` | H |
| Core editing | Trim, split and move clips | `guide/editing-tools` | H |
| Core editing | Preview and playback | `guide/preview` | H |
| Core editing | Clip properties | `guide/clip-properties` | H |
| Core editing | Undo and history | `guide/undo-and-history` | H |
| Creative tools | Text and captions | `guide/text-and-captions` | H |
| Creative tools | Shapes | `guide/shapes` | H |
| Creative tools | Audio editing | `guide/audio` | H |
| Creative tools | Effects and color | `guide/effects-and-color` | H |
| Creative tools | Transitions | `guide/transitions` | H |
| Creative tools | Animate with keyframes | `guide/keyframes` | H |
| Creative tools | Transcription | `guide/transcription` | H |
| Creative tools | Remove silence | `guide/silence-removal` | H |
| Creative tools | The safe area | `guide/safe-area` | H |
| Creative tools | Templates | `guide/templates` | H |
| Creative tools | Template: vertical cut with captions | `guide/template-vertical-cut` | H |
| Creative tools | Template: product teaser | `guide/template-product-teaser` | H |
| Creative tools | Template: quote card | `guide/template-quote-card` | H |
| Output | Exporting | `guide/exporting` | H |
| Output | Jobs and the queue | `guide/jobs` | H |
| Output | Encoders and export presets | `guide/encoders-and-presets` | H |
| Interface | Keyboard shortcuts (the basic shortcuts; RF-074, advanced and customizable shortcuts, extends this page) | `guide/keyboard-shortcuts` | H |
| Interface | Settings | `guide/settings` | H |
| Tutorials | Make a vertical short with captions | `guide/tutorial-vertical-short` | H |
| Tutorials | Edit by prompting an agent, end to end | `guide/tutorial-agent-edit` | H |
| Tutorials | The desktop application | `guide/desktop` | H |

(The three template pages are one per template of D-019; their names change if the template names change in SDD-005/SDD-002. The group `Tutorials` is added to `navigation.ts` by its own task.)

### 7.4 Agents (`agents`)

| Page title | Slug | Source |
| --- | --- | --- |
| Agents overview | `agents/overview` | H |
| Connect an agent (Claude Code, Cursor and others) | `agents/connect` | H |
| The MCP server | `agents/mcp` | H |
| The CLI for agents | `agents/cli` | H |
| How an agent verifies its work: view and lint | `agents/verify` | H |
| Batches, revisions and idempotency | `agents/batches` | H |
| The agent skill | `agents/skill` | H |
| Safety: undo, trash and limits | `agents/safety` | H |
| Recipes: cut, caption, export | `agents/recipes` | H |

### 7.5 Reference (`reference`)

| Page title | Slug | Source |
| --- | --- | --- |
| Command catalogue | `reference/commands` | G |
| One page per command (48 commands in SDD-002, 47 of them public; for example `Add a clip`) | `reference/commands/<command>` | G |
| Timeline schema | `reference/timeline-schema` | G |
| CLI reference | `reference/cli` | G |
| HTTP API | `reference/http-api` | G |
| Error codes | `reference/error-codes` | G |
| Configuration variables | `reference/configuration` | G |
| Workspace layout | `reference/workspace-layout` | H |
| Events stream | `reference/events` | H |
| Glossary | `reference/glossary` | G (from `glossary.json`) |

### 7.6 Self-hosting (`self-hosting`)

| Page title | Slug | Source |
| --- | --- | --- |
| Run the binary | `self-hosting/binary` | H |
| Run the Docker image | `self-hosting/docker` | H |
| Network mode and login | `self-hosting/network-mode` | H |
| API keys and scopes | `self-hosting/api-keys` | H |
| Put Caddy in front | `self-hosting/caddy` | H |
| Backups and upgrades | `self-hosting/backups-and-upgrades` | H |
| FFmpeg, codecs and licences | `self-hosting/ffmpeg-and-licences` | H |
| Verify a release | `self-hosting/verify-a-release` | H |
| Logs and OpenTelemetry | `self-hosting/observability` | H |
| Privacy: nothing leaves your machine | `self-hosting/privacy` | H |

### 7.7 Troubleshooting (`troubleshooting`)

| Page title | Slug | Source |
| --- | --- | --- |
| Common problems | `troubleshooting/common-problems` | H |
| FFmpeg is not found | `troubleshooting/ffmpeg-missing` | H |
| No H.264 encoder works | `troubleshooting/no-encoder` | H |
| The workspace is locked | `troubleshooting/workspace-locked` | H |
| An import fails | `troubleshooting/import-fails` | H |
| Export is slow | `troubleshooting/slow-export` | H |
| The diagnostic bundle | `troubleshooting/diagnostic-bundle` | H |

### 7.8 Contributing (`contributing`)

| Page title | Slug | Source |
| --- | --- | --- |
| Contributing guide | `contributing/guide` | H |
| Development setup | `contributing/setup` | H |
| Repository layout | `contributing/repository-layout` | H |
| Commits and pull requests | `contributing/commits-and-prs` | H |
| Writing documentation | `contributing/writing-docs` | H |
| The release process | `contributing/releases` | H |

### 7.9 Outside the sidebar

| Page | URL | Source |
| --- | --- | --- |
| Landing page | `/` | H |
| Changelog | `/changelog/` | generated from `CHANGELOG.md` (section 11.3) |
| 404 | `/404.html` | H |

## 8. Generated reference pages

ADR-0024 item 5: the reference is generated from the binary and the schemas at build time, so it cannot drift. The generated pages are **committed**, and CI regenerates them and fails on any difference, exactly as ADR-0014 item 2 does for the schemas.

### 8.1 What is generated, and from where

| Pages | Source | How the generator reads it |
| --- | --- | --- |
| Command catalogue and one page per command (`reference/commands`, `reference/commands/<command>`) | `stuarty command:help --json` (SDD-000 operation `CommandHelp`, ADR-0016 item 8): all commands with summary, argument schema, example, errors, scope and, for each, its CLI, MCP and HTTP forms | `site/generated/commands.json` |
| Timeline schema (`reference/timeline-schema`) | the JSON Schema files under `core/schema/` (ADR-0014 item 2; the file names are those of SDD-001) | read directly, no binary needed; rendered as a nested table per entity with `description`, type, constraints, default and `x-status` |
| CLI reference (`reference/cli`) | the `cli` block of every descriptor of `command:help --json`, plus the global flags and exit codes table (SDD-000 section 8: exit `0`, `2` for retryable codes, `1` otherwise) | `site/generated/commands.json` |
| HTTP API (`reference/http-api`) | `stuarty openapi:export -prefix /api/v1`, the vukem command that prints the OpenAPI 3.1 document in warmup mode (no database, no FFmpeg; `docs/cli.md:93`, `docs/api.md:270`) | `site/generated/openapi.json`; one section per tag, one block per operation, with the request and response schemas |
| Error codes (`reference/error-codes`) | the error registry of SDD-002 section A.8 and SDD-000 section 8, exported as `site/generated/errors.json` by `go run ./internal/tools/contractgen -errors` (code, meaning, retryable, HTTP status, since; the plain `contractgen` run writes `core/schema/` and `web/resources/js/generated/`, SDD-009 section 9.4 job 6) | `site/generated/errors.json` |
| Configuration variables (`reference/configuration`) | the configuration struct, through `go run ./internal/tools/configref -json` (the vukem tool of the same name, `Taskfile.yml:151-159`; vukem's has only `-check` and writes `docs/configuration.md`, so `REL-TOOLS` adds the `-json` flag when it copies it) | `site/generated/config.json`: variable, default, meaning, matches SDD-000 section 10 |
| Glossary (`reference/glossary`) | `site/content/glossary.json` (section 10) | |
| Keyboard shortcuts (`guide/keyboard-shortcuts-list`) | `SHORTCUTS` of `web/resources/js/editor/commands/keymap.ts` (imported as it is: the file has only type imports, which Node strips) and the labels `ui.editor.shortcuts.rows` of `web/resources/lang/en.json` | `scripts/shortcuts-reference.ts`; one table per group, `Mod` spelled as `Ctrl` and `Cmd`; a row without a label fails the run; a row no group names lands in "More shortcuts" |

**Required of other SDDs** (gaps found while writing this one, listed in the final answer): the descriptor returned by `command:help --json` must carry a `cli` block (command name, flags, positional arguments); `contractgen` must also write `errors.json`; `core/schema/` file names must be stable. If a shape differs, the generator adapts and the difference is recorded; this SDD is not changed.

### 8.2 The generation script

`site/scripts/generate-reference.ts` (Node with type stripping; `npm run reference` in `site/`; `task site:reference` and `task generate` of SDD-009 run it). Steps:

1. Build the binary once to a temporary path: `go build -o "$TEMP/stuarty" ./web/cmd/app` (from the repository root; the script finds the root with `git rev-parse --show-toplevel`).
2. Write the four JSON files into `site/generated/`:
   - `stuarty command:help --json > site/generated/commands.json`
   - `stuarty openapi:export -prefix /api/v1 > site/generated/openapi.json`
   - `go run ./internal/tools/configref -json > site/generated/config.json`
   - `go run ./internal/tools/contractgen -errors > site/generated/errors.json`
   The binary is run with `STUARTY_HOME` pointing at an empty temporary directory and with no network. JSON is written with sorted keys and two-space indentation so diffs are stable; no timestamp, no absolute path and no version string enters the output (the page shows the version only through the changelog).
3. Render the MDX pages into `site/content/generated/reference/`: one file per command, plus `commands.mdx`, `timeline-schema.mdx`, `cli.mdx`, `http-api.mdx`, `error-codes.mdx`, `configuration.mdx`, `glossary.mdx`. The front matter has `generated: true` and `title` and `description` from a dictionary in the script. The body is English (identifiers, schema descriptions and error meanings are English; they come from Go struct tags and the registry, D-008). The first line of the body is a comment `{/* Generated by site/scripts/generate-reference.ts. Do not edit. */}`.
4. Render command pages in the page template of section 5 where it applies: summary, "Arguments" table from the schema, "Example" with the CLI form and the MCP form, "Errors" table with links to `error-codes`, "Scope".
5. Idempotent: running it twice produces no diff.

`npm run reference:check` runs the script and `git diff --exit-code -- generated content/generated` (it is the same check that the `Generated files` job of `ci.yml` runs from the repository root, SDD-009 section 9.4 job 6).

### 8.3 Generated pages

Generated pages are English like every other page. `lib/content.ts` serves `content/generated/<path>.mdx` under `/docs/<path>/`. Their date comes from the `content/generated/` git history (section 11.2).

## 9. Agent readability

ADR-0024 item 6 is a requirement, built by the same code as vukem's endpoints (`src/pages/llms.txt.ts`, `llms-full.txt.ts`, `docs/[...slug].md.ts`, `sitemap.xml.ts`, `robots.txt.ts`, `404.astro`).

| Artefact | URL | Content |
| --- | --- | --- |
| `llms.txt` | `/llms.txt` | llms.txt convention: `# Stuarty`, a one-sentence summary as a blockquote, the line "Full documentation in one file: ...", the line "Every page is also available as Markdown: add `.md` to its URL", then one `##` heading per navigation section with `- [Title](URL): description` per page (`llms.txt.ts:20-27`) |
| `llms-full.txt` | `/llms-full.txt` | every page's Markdown twin concatenated in sidebar order, each preceded by a `---` line and its `Source:` URL |
| Markdown twin | `/docs/<dir>/<slug>.md` | `# Title`, a `> description` line, `Source: <HTML URL>`, then the page body with MDX components converted to Markdown by `lib/text.ts` `mdxToMarkdown` (`[...slug].md.ts:15`). Content type `text/markdown; charset=utf-8`. Every page has one, including generated pages |
| `sitemap.xml` | `/sitemap.xml` | one URL entry per HTML page (not the twins, not the two backup landings of section 1.5), the landing page and the changelog page; no alternates |
| `robots.txt` | `/robots.txt` | `User-agent: *`, `Allow: /`, `Disallow: <base>/landing/classic/` and `Disallow: <base>/landing/creative/` (the backup landings), `Sitemap: <site URL>/sitemap.xml`. Under a base path (a project site such as `https://<user>.github.io/<repo>`; the default is the custom domain `https://stuarty.com`, base `/`) this file is not at the host root and crawlers do not read it, nor the sitemap; the sitemap is then submitted by hand in the search consoles, or a custom domain is used (section 12.4) |
| 404 | `/404.html` | a real HTML page with a link to the docs entry. GitHub Pages serves `404.html` with HTTP status 404 for every unknown path under the site, so an agent gets a real 404, not the shell with status 200 that FreeCut returns |
| Alternate link | in every page `<head>` | `<link rel="alternate" type="text/markdown" href="...">` |

The site has no analytics and loads no external script, font or stylesheet, and it is hosted on GitHub Pages and nowhere else (D-015, D-016). Fonts are the self-hosted `@fontsource` packages that vukem's site already uses. A pull request that adds an analytics script, a CDN resource, an embedded third-party widget or another hosting target is rejected; property 7 below enforces the first three in CI.

Properties checked by `scripts/check-built.ts` after `astro build` (section 12) and by tests next to the endpoints (`markdown-twin.test.ts` of vukem, extended):

1. For every navigation page and generated page: `out/<path>/index.html` and the twin `.md` exist.
2. No twin contains `<`-prefixed JSX components or `import` lines.
3. Every URL in `llms.txt` resolves to a file in `out/`.
4. `sitemap.xml` lists every docs page and no twin.
5. `out/404.html` exists.
6. After deployment (acceptance check): `curl -s -o /dev/null -w "%{http_code}" <site>/does-not-exist/` prints `404`.
7. **No external resource** (D-015, D-016): every `src` and `href` attribute of every built HTML page, and every `url(...)` of the built CSS, is same-origin (starts with the base path or is a relative path), a `mailto:` or `#` link, or an outbound link that is written in page content (a `[text](https://...)` link of an `.mdx` file) and is not a `<script>`, `<link rel="stylesheet">`, `<link rel="preload">`, `<img>`, `<iframe>` or `<source>` resource. A `<script src>`, a stylesheet, a font or an image served from another origin fails `check-built.ts`.

## 10. Glossary and the boundary with UI strings

The editor's UI strings are a different concern: they are specified in the UI architecture (SDD-012), live with the React code, and are loaded by the application, not by the site. This SDD states only one rule about them: **the glossary of product terms is shared**. `site/content/glossary.json` is the single list (an array of `{ "id", "en", "pt-br", "note" }`); the `pt-br` field exists for the editor's Portuguese interface, and the site prints only the English term and the note (the generated page `reference/glossary`). SDD-012 imports the same file for its UI dictionary instead of keeping a second copy. A term is changed in this file first, then everywhere (the rule SDD-000 states for its own names).

The 30 terms. Command, field and error-code names are never translated in the editor; this table is for prose and UI labels.

| # | English | Portuguese (Brazil) | Note |
| --- | --- | --- | --- |
| 1 | project | projeto | |
| 2 | timeline | linha do tempo | |
| 3 | track | faixa | video and audio tracks |
| 4 | clip | clipe | |
| 5 | item | item | the generic element of a track |
| 6 | asset (imported media) | mídia | "asset" in the code, "mídia" in prose and UI |
| 7 | media library | biblioteca de mídia | |
| 8 | caption | legenda | styled, word-timed text on screen |
| 9 | subtitle | legenda | same word; "caption" and "subtitle" are one concept in the UI |
| 10 | transcript | transcrição | |
| 11 | transcription | transcrição | the act; context tells it apart |
| 12 | trim | aparar | change the start or end of a clip |
| 13 | split | dividir | cut one clip into two |
| 14 | ripple delete | excluir e fechar o espaço | |
| 15 | keyframe | quadro-chave | |
| 16 | transition | transição | |
| 17 | overlay | sobreposição | |
| 18 | layer | camada | |
| 19 | playhead | cabeçote de reprodução | |
| 20 | frame | quadro | a video frame |
| 21 | frame rate | taxa de quadros | |
| 22 | preview | pré-visualização | |
| 23 | export | exportar | verb; noun "exportação" |
| 24 | render | renderizar | verb; noun "renderização" |
| 25 | undo / redo | desfazer / refazer | |
| 26 | trash | lixeira | a database state, not a folder (SDD-000 section 9) |
| 27 | template | modelo | |
| 28 | silence removal | remoção de silêncio | |
| 29 | safe area | área segura | |
| 30 | workspace | espaço de trabalho | the data directory |

Further terms (job = tarefa, batch = lote, revision = revisão, operation log = registro de operações, API key = chave de API, scope = escopo, agent = agente, aspect ratio = proporção de tela, fade = esmaecimento, mute = silenciar, volume = volume, font = fonte, resolution = resolução, lint = verificação (lint)) are added to `glossary.json` by the translators as they meet them, each in a pull request that touches only that file.

### 10.1 The learning glossary (TASK-0267, 2026-11-16)

A second glossary, for a reader who is learning to edit video (written for a ten-year-old), is the sidebar category **Glossary**. Its data is hand-written JSON in `site/content/glossary/`: `topics.json` (ten topics: shooting and footage, editing and timeline, cuts and transitions, audio, color, text and captions, effects and compositing, formats/codecs/containers, delivery and platforms, industry slang), one `<topic>.json` per topic (each term has `term`, a one-sentence `tip`, `meaning`, `example`, and optionally `vukem`, `aka`, `see`, `sources`) and `sources.json` (authorities cited by key: ITU, EBU, SMPTE, W3C, FFmpeg, MDN, the Library of Congress). A source key is `ref:<key>` or `wiki:<English Wikipedia article title>`. The pages are generated by `scripts/generate-reference.ts` through `scripts/glossary-pages.ts` into `content/generated/glossary/` (one page per topic and an A to Z list); only `glossary/how-to-use` is hand-written. The anchor of a term is the slug of its heading.

`<Term id="keyframe">keyframes</Term>` (`components/mdx/term.tsx`) shows the tip of the term on hover and keyboard focus and links to its entry; `<Term tip="...">` keeps the one-off form. `scripts/glossary.test.ts` (in `test:site`) checks the field rules, unique anchors, that every see-also and source resolves and that every `Term` of the docs names a term. External hosts of the registry and Wikipedia are allowed as text links by `check:built`. The older generated page `reference/glossary` ("Product terms") stays: it is the term list shared with the editor (above).

## 11. Search, last updated and the changelog page

### 11.1 Search

vukem builds a search index at build time and loads the dialog on first open (`src/pages/search-index.json.ts`, `lib/search.ts`, `components/search-dialog.tsx`; vukem `CHANGELOG` v0.41.0 "Load the search dialog on first open"). Ours builds one index, `/search-index.json`, with one entry per page and per h2/h3 section (title, URL, description, heading, plain text). Matching is case-insensitive and accent-insensitive: both the query and the indexed text are normalized with `normalize("NFD")` and a removal of combining marks. Keyboard shortcut and behaviour are vukem's (`Ctrl K`).

### 11.2 "Last updated" from git

Unchanged from vukem (`scripts/last-updated.ts`, `lib/git-dates.ts`, `scripts/check-last-updated.ts`): `content/last-updated.json` maps each page file to the ISO date of its last commit (author date, so a rebase merge keeps it), is generated by `npm run last-updated` and never edited by hand, and is checked in CI and by the pre-push hook (`lefthook.yml:40-42` equivalent in SDD-009 section 15.1). Changes: keys are `docs/en/<dir>/<slug>.mdx` and `generated/reference/...mdx`; `isDocPage` in `check-last-updated.ts` (vukem line 14-16: `.mdx` under `docs/`) also accepts `generated/`; the date is shown formatted with `Intl.DateTimeFormat` in English, in UTC, as `Last updated: ...`. Both checks run on every pull request through `docs-last-updated.yml` (SDD-009 section 9.6), so a page left out of the file cannot hide.

### 11.3 The changelog page

`src/pages/changelog.astro` and `lib/changelog.tsx` render `CHANGELOG.md` at build time (`site.yml` also triggers when `CHANGELOG.md` changes, `site.yml:8-11`). Release notes are English (D-008, SDD-009 section 3); the page has a title, a description and a note saying so.

## 12. Build, checks and deployment

### 12.1 Scripts in `site/package.json`

Copied from vukem (`package.json:6-14`) and extended:

| Script | Command | Role |
| --- | --- | --- |
| `dev` | `astro dev` | local preview at `http://localhost:3000` |
| `build` | `astro build` | static export to `out/` |
| `lint` | `eslint .` | |
| `check:mdx` | `node scripts/check-mdx.ts` | MDX compiles, components exist, `/docs/` links resolve, no bare braces; extended: every nav slug has a file, every file is listed, every section title is non-empty |
| `last-updated` | `node scripts/last-updated.ts` | refresh `content/last-updated.json` |
| `check:last-updated` | `node scripts/check-last-updated.ts` | every page has a date at least as new as its last commit |
| `reference` | `node scripts/generate-reference.ts` | section 8 |
| `reference:check` | `node scripts/generate-reference.ts && git diff --exit-code -- generated content/generated` | section 8.2 |
| `check:built` | `node scripts/check-built.ts` | section 9 properties (including the no-external-resource property 7) and the section 4.3 table on `out/` |
| `test:scripts` | `node --test scripts/check-last-updated.test.ts scripts/committed-dates.test.ts scripts/trailing-slash.test.ts scripts/check-built.test.ts scripts/check-links-live.test.ts` | dependency-free tests, so the `Last updated` jobs need no `npm ci` of the whole site (vukem comment, `site.yml:25-26`); `base-path.test.ts` is not here because it builds the site |
| `test:site` | `node --test scripts/link-anchors.test.ts scripts/markdown-twin.test.ts scripts/generate-reference.test.ts scripts/check-doc-claims.test.ts scripts/accessibility-page.test.ts scripts/latest-release.test.ts scripts/base-path.test.ts` | tests that import site modules or build the site; needs `npm ci` |
| `test:base-path` | `node --test scripts/base-path.test.ts` | the base path test alone (builds the site under a base path and under `/`, section 12.4); it is also run by `test:site` |

(vukem's `legacy-path-redirect` and `current-equivalent` tests are not copied.)

### 12.2 CI: `site.yml` and `docs-last-updated.yml`

Both are specified in SDD-009 section 9.6; the steps of `site.yml` are, in order:

- Job `build`: `actions/checkout`; `actions/setup-go` (`go-version-file: go.mod`, needed by `reference:check`, which builds the binary); `actions/setup-node` (Node 24, npm cache on `site/package-lock.json`); `npm ci`; `npm run test:site` (which includes the base path test); `npm run lint`; `npm run check:mdx`; `npm run reference:check` (run with `working-directory: site`); `npm run build`; `npm run check:built`; on `main` or manual dispatch, `actions/upload-pages-artifact` with `path: site/out`. In the Taskfile the same list is `task site:build` (SDD-009 section 14).
- Job `last-updated`: `fetch-depth: 0`; `actions/setup-node` (Node 24); `npm run test:scripts`; `npm run check:last-updated`.

These step lists are the same as in SDD-009 section 9.6.

- Job `deploy` (only on `main` or manual dispatch; needs `build` and `last-updated`): `environment: github-pages`; `actions/deploy-pages`. Permissions: `pages: write`, `id-token: write`.

The job `build` runs the same `reference:check` as the `Generated files` job of `ci.yml` (SDD-009 9.4); the duplication is intended: `Generated files` runs on every pull request and is required, while `site.yml` is path-filtered and guards the deploy.

### 12.3 Deployment

GitHub Pages, source "GitHub Actions" (ADR-0024 item 2; D-015): a push to `main` that touches the paths of `site.yml` builds and deploys. No Railway, no container, no standalone server. The deployed site is exactly `out/`. The in-application help links to the site (ADR-0024 item 8); the editor embeds no documentation.

### 12.4 Site URL and base path

`SITE_URL` (a repository variable, optional) is the public URL. Default: `https://stuarty.com` (TASK-0263, decision of the author, 2026-11-16), a custom domain on GitHub Pages, so `site.base` is `/` and `robots.txt` and the sitemap are at the host root. The earlier default was the project site `https://alexgalhardo.github.io/stuarty` (base `/stuarty`); an address with a path still works and sets that path as the base. Every internal link, asset path, canonical URL, sitemap URL and `llms.txt` URL is built from `site.url` and `site.base` (`lib/site.ts`); `scripts/base-path.test.ts` builds the site under both a base path (`https://example.org/stuarty`) and `/` (`https://stuarty.com`) and fails on any `href="/..."` or `src="/..."` that does not start with the base.

**The custom domain is a repository setting, not a file.** The site is deployed with `actions/deploy-pages` (source "GitHub Actions", `build_type: workflow`), and a deployment of that kind ignores a `CNAME` file in the artifact, so `site/public/CNAME` is not added. The author sets the domain once: a DNS `CNAME` record `video` pointing to `alexgalhardo.github.io` (at the DNS host of `vukem.dev`), then Settings, Pages, Custom domain `stuarty.com`, and Enforce HTTPS once the certificate is issued (or `gh api -X PUT repos/AlexGalhardo/stuarty/pages -f cname=stuarty.com`). The repository variable `SITE_URL` may stay unset, because the default is already the domain; if it is set to the old project address it must be removed or changed, since `site.yml`, `desktop.yml` and `release.yml` read it. GitHub then answers the old project address with a redirect to the domain; the update client of desktop applications up to v0.20.0 does not follow it (SDD-009 section 10.5).

### 12.5 Local commands

Defined in SDD-009 section 14: `task site:dev`, `task site:build`, `task site:reference`, `task site:dates` (`task site:translations` was removed with the translation check).

## 13. Implementation tasks owned by this SDD

Identifiers for the ROADMAP; all `senior-d` unless stated.

| Id | Task | Done when |
| --- | --- | --- |
| `SITE-COPY` | Copy the files of section 1.1 from the current vukem `site/` (record the vukem commit in the report), apply 1.2, drop 1.3, run `npm install` to create `package-lock.json`, replace the icon and logo; then the isolated `style(site)` reformat to tabs (section 1.2) | `npm ci && npm run build` succeeds with one placeholder page per locale, from a clean clone (so `Base.astro`, `prerender.tsx`, `public/` and `.gitignore` are all copied) |
| `SITE-I18N` | (superseded by TASK-0265: the site is English only) Locale routing, content loader with fallback, switcher, notice, hreflang, canonical, per-locale search index, `lib/ui-text.ts` dictionary, bilingual 404 and landing | removed |
| `SITE-NAV` | `navigation.ts` with the types and categories of section 3; the `check:mdx` extensions | acceptance check 3 |
| `SITE-REF` | `scripts/generate-reference.ts`, the four JSON sources, the generated pages | acceptance check 10 |
| `SITE-AGENT` | `llms.txt`, `llms-full.txt`, twins, sitemap, robots, `check-built.ts`, with the no-external-resource assertion | acceptance checks 11 to 13 and 18 |
| `SITE-TRANSLATE-CHECK` | (superseded by TASK-0265) `check-translations.ts` and its tests | removed |
| `SITE-CI` | `site.yml`, `docs-last-updated.yml` changes (with `REL-CI` of SDD-009) | acceptance check 14 |
| `SITE-PAGES` | The pages of section 7 (English, by `senior-d` and feature seniors) | every row exists |

## 14. Acceptance

Each check is verifiable from this document and the repository alone.

1. `cd site && npm ci && npm run build` succeeds and `out/` contains `index.html`, `docs/introduction/index.html`, `404.html`, `llms.txt`, `sitemap.xml`, `robots.txt`.
2. For every page listed in `navigation.ts` there is, in `out/`, a page and a twin (`npm run check:built` passes).
3. `npm run check:mdx` fails when: a slug is in `navigation.ts` with no file; a file is not listed; a section title is empty; a link points to a page that does not exist.
4. (removed with the fallback pages, TASK-0265)
5. Every docs page has `<html lang="en">`, one `canonical` pointing to itself and no hreflang (verified by `check-built.ts`).
6. (removed with the language switcher, TASK-0265) The header has no language control, and `/pt-br/...` addresses redirect to the English page (a built `out/pt-br/docs/guide/audio/index.html` refreshes to `docs/guide/audio/`).
7. `sitemap.xml` lists every page and the landing page and the changelog, no `.md` twin and no `pt-br` URL.
8. Searching `trim` in the dialog finds `Trim, split and move clips`.
9. (removed with the translation check, TASK-0265)
10. `npm run reference` run twice produces no diff; changing a command description in the Go struct tags and rerunning changes only that command page and `commands.json`; a pull request that changes a command without regenerating fails the `Generated files` job and `reference:check`.
11. `llms.txt` follows section 9 and every URL in it resolves in `out/`; `llms-full.txt` contains every page's twin in sidebar order.
12. No twin contains JSX tags or `import` lines.
13. After deployment: `curl -sI <site>/docs/introduction.md` returns `200` with `text/markdown`; `curl -s -o /dev/null -w "%{http_code}" <site>/nope/` prints `404`.
14. `site.yml` on a pull request that edits `site/content/docs/en/guide/exporting.mdx` runs `build` and `last-updated`; there is no translation step.
15. `npm run test:base-path` (or `scripts/base-path.test.ts`) passes with `SITE_URL=https://example.org/stuarty` (a base path) and with `SITE_URL=https://stuarty.com` (base `/`): no absolute link lacks the base.
16. `content/glossary.json` has the 30 terms of section 10, each with `en` and `pt-br` (the editor's Portuguese), and is the file SDD-012 imports (a grep of `web/resources/js` finds no second copy of the term list once SDD-012 is implemented).
17. Every row of section 7 exists.
18. No external resource (D-015, D-016): `npm run check:built` passes on the real `out/` and fails on a scratch change that adds `<script src="https://example.org/x.js">`, an external stylesheet or an external font to a page; the built site makes no request to another origin when opened with the network blocked (property 7 of section 9).
19. A person following only `getting-started/quick-start` reaches an exported MP4 in under ten minutes on a clean machine (NFR-040), timed by the author and recorded in the ROADMAP.

## 15. The site of Stuarty (epic 40, 2026-11-20)

The site was written for the editor alone. Epic 40 turns it into the site of the product of ADR-0044 and ADR-0031: it documents the sections that are merged when the epic runs and lists the ones that are not. Sections 1 to 14 still describe the shell, the checks and the generators; this section replaces the page list of section 7 and the page order of section 3 where it says otherwise.

### 15.1 Language

Epic 40 asks for English and Brazilian Portuguese and for a check that fails on a missing translation. The amendment of ADR-0024 and the amendment at the top of this document (TASK-0265, the author, 2026-11-16) removed the second language, the switcher and that check. The later decision of the author wins over the plan, and bringing the language back is a change of scope (a second tree of about 150 pages, a staleness check, per-locale endpoints). **Decision of this SDD, cheap to undo: the site stays English only.** What epic 40 gives the reader of Portuguese is two guides written once in Portuguese and kept next to the repository documents, as `deploy/README.pt-BR.md` already is: `docs/guides/pt-BR/comecar.md` (what Stuarty is, what stays on the machine, install) and `docs/guides/pt-BR/primeiro-short.md` (the first short video). The introduction links to them and to the two Portuguese recipe guides of epic 39 (`docs/guides/pt-BR/recipe-product-commercial.md`, `docs/guides/pt-BR/recipe-long-talking-video.md`), and `site/scripts/pt-br-guides.test.ts` fails when one of the four is missing, is not linked, or names a command that is not in the CLI. Task 8 of the epic is met in that form, and the question is recorded for the author.

### 15.2 Structure

| Section | Directory and pages |
| --- | --- |
| Start | `getting-started`: `introduction`, `what-leaves-your-machine`, `installation` (which one to choose, and the measured time), `install-windows`, `install-docker`, `install-server`, `quick-start`, `workspace`, `projects`, `migration` |
| First short video | `guide/first-short-video`: the flow of epic 38 (`scripts/first-flow.sh`) as a tutorial, the scripted commands and the path in the UI; `guide/tutorial-vertical-short` stays as the editor-only tutorial, and the recipes of epic 39 (`guide/recipe-product-commercial`, `guide/recipe-long-talking-video`) follow in the same sidebar group |
| Guides | one page per section of the sidebar of the product: the video editor (the existing `guide/` pages), `guide/dashboard`, `guide/asset-library`, `guide/image-editor`, `guide/writing`, publishing (`guide/approvals`, `guide/publishing-calendar`, `guide/blog-posts` and `publish/<network>`: `youtube`, `instagram`, `facebook`, `x`, `tiktok`), `guide/ai-providers`, `guide/agent-chat`, `guide/memory-skills-prompts` |
| Agents | `agents`: connect over MCP, the CLI, the HTTP API, the generated page per domain, the approval queue from an agent's point of view (`agents/approvals`) |
| Reference | generated: operations (`api/`, `mcp/`), CLI, MCP tools, errors, configuration, and the limits of the networks (`reference/network-limits`) |
| Security and privacy | `self-hosting/privacy` (every way out, one row each) and `self-hosting/security` (the vault, the master key, the approval queue, what an agent cannot do) |
| Troubleshooting, Glossary | as they are, with the pages of the new domains added |

The landing page and `getting-started/what-leaves-your-machine` state the outbound rule of ADR-0032 in plain words: with no connection configured nothing leaves the machine; a connection a person creates is the only thing that opens one; publishing waits for the person's approval; there is no telemetry.

### 15.3 Generated by this epic

- `internal/tools/clidoc -networks` writes `site/generated/networks.json`: the `Description` of each social connector of the build (limits with the official page and the date they were read, the scopes, the hosts) and the export preset the network's `Preset` names, from the same data `PublishValidate` checks against. `scripts/network-reference.ts` (called by `generate-reference.ts`) turns it into `content/generated/reference/network-limits.mdx` and one generated page per network, `content/generated/publish/<network>.mdx`: the hand-written steps of the developer app (`content/network-setup/<network>.md`), the permissions and hosts, and the limits table with the official page and the date each number was read (and the price of a post where the network charges one). The hosts those pages cite are read from the same file by `check-built.ts`.
- `task generate:check` runs `task codegen`, the OpenAPI export of `web/contract`, and the reference script (which runs `clidoc` and `clidoc -networks`), and fails when a committed generated file differs. `scripts/network-reference.test.ts` fails when an operation of the registry has no generated page.
- The former name is generated too: `content/generated/getting-started/migration.mdx` is the one page that says it, from `content/fragments/migration.md` and the table of SDD-000 section 13.1. Only its body says it: its title and description show on every page (the sidebar, the search, `llms.txt`), and `scripts/generate-reference.test.ts` fails when the front matter of any page names it.

### 15.4 Every command on the site runs, and the ten minutes

`scripts/docs-commands-smoke.sh` runs the bash blocks of the install pages, of `guide/first-short-video`, of the two older tutorials, of `guide/image-editor`, `guide/asset-library` and `guide/memory-skills-prompts` against a local build with the stub transcriber. A block that needs Docker or a server is checked instead (bash syntax, every `STUARTY_*` and `APP_*` variable in the configuration reference, the flags of `deploy/` scripts). The steps that need an account of the reader are `console` blocks: every `stuarty` command in them must be a command of the CLI, and `scripts/first-flow.sh` runs the whole flow with a fake provider, a fake network and a person approval. `scripts/first-export-timing.sh --binary <path>` (or an image) measures the first export on a clean workspace; the number, the date, the machine and the condition are published on `getting-started/installation`.

### 15.5 The old pages: keep, rewrite, remove

| Pages | Decision |
| --- | --- |
| `getting-started/introduction`, `installation`, `quick-start`, `projects` | rewrite: they describe an editor alone; `quick-start` becomes the first export in the editor and points to the first short video |
| `getting-started/workspace` | keep, with the name of the folder checked |
| `concepts/*` (7 pages) | keep |
| `guide/*` of the video editor (media library to exporting, 40 pages) | keep; `writing`, `ai-providers`, `agent-chat`, `publishing-calendar` are checked against the merged epics |
| `guide/tutorial-vertical-short`, `tutorial-agent-edit` | keep (the editor-only tutorials the smoke script already runs) |
| `agents/*`, `mcp/*`, `api/overview`, `prompts/*`, `reference/events` | keep; `agents/overview` rewritten for the domains |
| `self-hosting/*` | keep; `privacy` rewritten; `vps`, `docker`, `binary` stay as the detail behind the three install pages |
| `troubleshooting/*`, `glossary/*`, `contributing/*` | keep |
| the former name | `self-hosting/backups-and-upgrades` and the configuration reference point to the migration page instead of saying it; the only page that names it is `getting-started/migration` (SDD-000 section 13). The address of the site and of the update feed still carries the former name as a host name: that is the author's to change with the domain |
| wording of stages | the callouts "still before 1.0" and the sentences that put the product before a 1.0 are removed (D-049); a version number as a version (the deprecation of the former variables "until 1.0.0", Conventional Commits 1.0) stays |
| removed | none: no page is deleted, so no redirect is needed; a page that moves gets an entry in `astro.config.mjs` |

### 15.6 Not documented, because not merged when the epic ran

The video generation that needs a provider (11, only the part without a provider is merged) and the agent's completion rate in the first flow (38 tasks 4 and 6). `getting-started/introduction` lists them as "in progress".

The blog and its destinations (epic 23), the recipes for commercials and long videos (39) and the revert of an image (50) were merged into the integration branch while the epic ran, with their own pages (`guide/blog-posts`, `guide/recipe-*`, the revert in `concepts/commands-and-revisions` and `guide/image-editor`): the review of the epic put them in the sidebar, in the introduction and, for the blog destinations, in the list of what leaves the machine.
