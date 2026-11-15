# SDD-010: Documentation site and i18n

- Status: Accepted (2026-11-14). Drafted by a delegated engineer, audited independently, corrected under the architect's decisions.
- Depends on: SDD-000, SDD-009, ADR-0001, ADR-0014, ADR-0016, ADR-0017, ADR-0024, brainstorming D-008, D-015, D-016, D-018, D-019
- Implements: RF-211, RF-212, RF-214 (documentation part), NFR-040 (the getting-started page is the test of that requirement)
- Owner area: senior-d (agent surfaces, documentation site, releases and CI)

## Contents

1. [Starting point: what is copied, changed and left out](#1-starting-point-what-is-copied-changed-and-left-out)
2. [Locales, content tree and URLs](#2-locales-content-tree-and-urls)
3. [The navigation file](#3-the-navigation-file)
4. [Language switcher, fallback, hreflang and canonical links](#4-language-switcher-fallback-hreflang-and-canonical-links)
5. [Page template and writing rules](#5-page-template-and-writing-rules)
6. [The translation staleness check](#6-the-translation-staleness-check)
7. [Page list](#7-page-list)
8. [Generated reference pages](#8-generated-reference-pages)
9. [Agent readability](#9-agent-readability)
10. [Glossary and the boundary with UI strings](#10-glossary-and-the-boundary-with-ui-strings)
11. [Search, last updated and the changelog page](#11-search-last-updated-and-the-changelog-page)
12. [Build, checks and deployment](#12-build-checks-and-deployment)
13. [Implementation tasks owned by this SDD](#13-implementation-tasks-owned-by-this-sdd)
14. [Acceptance](#14-acceptance)

Citations of the form `path:line` refer to the vukem repository at `C:\dev\vukem-video\vukem\` at the tag `v0.43.0`, that is `v0.43.0:<path>:<line>`, read with `git -C C:\dev\vukem-video\vukem show v0.43.0:<path>` (the working tree of vukem is ahead of the tag), unless stated otherwise. **Decision of this SDD** marks a choice made where the ADRs or vukem were silent or contradictory. Values known only when a task runs are written `<recorded by SITE-...>`; the task identifiers are in section 13.

## 1. Starting point: what is copied, changed and left out

ADR-0024 item 1: the site starts from the **current** vukem `site/`, not from the Studio fork. Source of truth for "current": `vukem/site/` at the commit the task `SITE-COPY` records in its report.

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
| `site/public/logo.svg`, `site/public/icon.svg`, `site/public/.nojekyll` | The logo and icon (replaced by the product ones) and the marker that stops GitHub Pages from running Jekyll |
| `site/.gitignore` | Ignores `node_modules`, `out/` and local env files; checked against the real vukem file so the copied site builds from a clean clone |
| `site/package-lock.json` | Created by `npm install` in `SITE-COPY`; never copied from vukem |
| `scripts/refresh-last-updated.sh` (repository root) | The pre-push hook script (SDD-009 section 15.1) |

**Theme and visual identity of the site.** The site keeps the theme behaviour of the copied shell (its `globals.css` and theme toggle, unchanged): D-024's dark default is a rule of the editor interface (SDD-012 §17), not of the documentation. The site does not import the product's `web/resources/css/app.css`; it keeps the shell's typography, spacing and layout, and takes from `docs/design-system.md` §8.4 only the product mark and the `--brand` colour (when the mark exists), the content style and the shared glossary (section 10), and the rule for screenshots of the UI (dark theme, 1600 by 900, the fixture project of the end-to-end suite, no personal paths visible).

`package.json` dependencies are copied with the exact versions of `vukem/site/package.json` (astro 7.4.7, react 19.3.1, tailwindcss 4.3.3, @mdx-js/mdx 3.1.1, shiki 4.5.0 and the rest; `package.json:15-40`). `SITE-COPY` re-checks each against the newest stable release at copy time (the author's dependency rule: stable only, no `latest`, no pre-release) and records any difference; it does not add a package that vukem's site does not use. The package manager is **npm** with `package-lock.json`, as in vukem and in the Studio's `site/` (Decision of this SDD: the author's global rule prefers Bun unless the project already uses another manager, and both predecessors have an npm lockfile here; the React UI in `web/` stays on Bun). Node 24 in CI (`site.yml:35-37`).

### 1.2 Changed

| File | Change | Reason |
| --- | --- | --- |
| `package.json` | `name: vukem-video-docs`; scripts added: `reference`, `reference:check`, `check:translations`, `test:base-path`, `check:built` (section 12.1); `test:scripts` and `test:site` rewritten (section 12.1) | |
| `astro.config.mjs` | `site` and `base` from `SITE_URL`; `trailingSlash: "always"` and `output: "static"`, `outDir: "out"` kept; `redirects` replaced by `"/docs": "/docs/introduction/"` and `"/pt-br/docs": "/pt-br/docs/introduction/"`; the `/docs/0.20` redirect removed; an `i18n` block: `defaultLocale: "en"`, `locales: ["en", "pt-br"]`, `routing: { prefixDefaultLocale: false }` | the site is served under a base path unless a custom domain is set (section 12.4) |
| `lib/site.ts` | `name: "Vukem Video"`, `repo: "https://github.com/AlexGalhardo/vukem-video"`, `editBase: "https://github.com/AlexGalhardo/vukem-video/edit/main/site/content/docs"`, `url` from `SITE_URL`, `base` derived from the URL path | |
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
| `site/public/CNAME` | it names vukem.dev; the custom domain of this site, if any, is the author's decision (section 12.4) |
| `site/src/pages/index.astro` content | the landing page is rewritten with the structure of <https://www.freecut.net/> (hero with a run/download call to action, feature tour, agents, open source and local-first, links to docs and repository) and only Vukem Video copy, mark and colours (D-028; it replaced the vukem structure of hero, three cards and a command block) |

### 1.4 Taken from neither the Studio fork nor FreeCut, and why

From the Studio's `web/site` (older fork of the same shell, `docs/search/notes/vukem.md` section 4.2): nothing is taken. It lacks the Markdown twin, the anchor check and the trailing-slash tests, and its extras are not wanted: the standalone Node server `scripts/serve.ts` and `static-path.ts` and the `Dockerfile` for Railway (D-015: only a static site on GitHub Pages is hosted), the six Studio categories, and the `/` redirect to `/docs/introduction`. Its package name `studio-docs` and its Railway deployment notes are removed with the Studio (SDD-009 section 17).

From FreeCut's documentation (`docs/search/notes/vukem.md` section 4.3): the **outline** of the user guide only (section 7). Its **implementation** is not taken: the documentation is a route inside the React application, every URL returns the same 1,408-byte shell, and there is no sitemap, `robots.txt` or `llms.txt`, so neither a search engine nor an agent can read it without running JavaScript. This product's main readers are agents, so a page must be real HTML and real Markdown at its own URL.

### 1.5 What P1-DOC-002 built, and where it differs from sections 1.1 to 1.4

The shell was copied from vukem commit `c9a3fcca` (the last commit that touched `site/`; the clone was at `c4949ed0`, the head of vukem pull request 222), with the dependency versions of its `package.json` unchanged (Astro `7.4.7`, no package added). The site lives in `site/` and `.github/workflows/site.yml`.

- **Base path.** `lib/site.ts` reads `import.meta.env.SITE` and `import.meta.env.BASE_URL` (both set by `astro.config.mjs` from `SITE_URL`, default `https://alexgalhardo.github.io/vukem-video`) and exports `withBase`, used by `Link` and by every component that wrote a root-relative path. Astro does not add the base to the destination of a `redirects` entry, so `astro.config.mjs` writes `/docs` to `<base>/docs/introduction/` in full. `robots.txt` has no `Host:` line.
- **No versions, no i18n yet.** `lib/versions.ts`, the snapshot tree, `legacy-path-redirect` and the `/pt-br` redirect script of `404.astro` are not copied; `lib/content.ts`, the sidebar, the header, `doc-article` and `check-mdx` have no version parameter. Only English docs exist (an introduction and `quick-start`, "Getting started"); the locale layout of sections 2 to 4 comes with P1-DOC-001. The landing page has an English and a Brazilian Portuguese version (`lib/landing.ts`, `src/components/Landing.astro`, `src/pages/index.astro`, `src/pages/pt-br/index.astro`).
- **Landing page (D-028).** Hero, feature tour (each item marked Prototype or Later from PRD-001), how agents drive the editor, open source and local-first, footer. Copy, mark and colours are the product's own (violet `#7c5cff` mark; dark by default, D-024: the theme script and `ThemeToggle` default to `dark` instead of `system`); `--faint` was darkened in both themes and the primary button uses `#6446f0` so text meets contrast 4.5:1. The vukem gopher asset, `lib/landing.ts` of vukem (Go sample code) and `public/CNAME` are not copied; `public/.nojekyll` is.
- **`check:built` exists early.** `scripts/check-built.ts` (and its test) implements only the properties P1-DOC-002 needs: static files only, no resource from another origin and no host other than the inert ones listed in the script, and every root-relative `href` or `src` carrying the base path. The checks on twins, `llms.txt` and the section 4.3 table remain for P1-DOC-001. `test:scripts` includes its test.
- **Not copied yet:** `scripts/refresh-last-updated.sh` and the `docs-last-updated.yml` workflow (CI owner), `check:translations`, `reference`, `reference:check`, `test:base-path`. `site.yml` therefore has no `setup-go` step and no `reference:check`; they are added with P1-DOC-001.
- **What P1-DOC-001 built, and where it differs from sections 2 to 9.** Both locales are served (sections 2 and 4): the content is `content/docs/en/`, `content/docs/pt-br/` and `content/generated/`; `lib/page-files.ts` finds the file of a page (its own, else the generated one, else the English one as a fallback); `lib/locale.ts`, `lib/locale-context.tsx` and `lib/ui-text.ts` (a typed dictionary per locale) carry the locale to the components, and `Link` rewrites `/docs/...` to `/pt-br/docs/...` in a Portuguese page. The switcher is two plain links (`EN`, `PT`) in the header; hreflang, canonicals, the sitemap alternates, the fallback notice, the twins, `llms.txt`, `llms-full.txt` and the search index exist per locale, and the search folds accents. `navigation.ts` has `title` per locale and a flat `pages` list per category (no sub-groups yet). `check:translations` (section 6, with `translations.config.json`), the extended `check:mdx` and `check:built` (twins, `llms.txt`, the section 4.3 table, the bilingual 404) and `docs-last-updated.yml` exist. The reference is generated by `scripts/generate-reference.ts` into `content/generated/reference/` (committed; `reference:check` and `site.yml` fail on a difference). It reads `core/schema/commands.json` (one page per public command), `core/schema/errors.json` with the meanings of SDD-000 section 8, `core/schema/timeline.v1.schema.json`, the table of SDD-000 section 10, `content/glossary.json`, `site/generated/cli.json`, which `go run ./internal/tools/clidoc` writes from the declarations of `web/app/cli`, and `site/generated/openapi.json`, which `UPDATE_OPENAPI=1 go test ./web/contract/ -run OpenAPI` writes from the running API (the test fails when the file is stale). The warmup mode of vukem's `openapi:export` cannot see the routes that need the Service, so the document comes from the contract environment instead; the binary has no `command:help --json` with a `cli` block and no `configref` yet, so section 8.1 is met with these sources. Generated pages have English text in both locales and a note says so in Portuguese. The Prototype pages written in both locales: getting started (introduction, installation, quick start, workspace, projects), concepts (how it works, time, commands and revisions, tracks), guide (media library, timeline, editing tools, text and captions, undo, exporting) and agents (overview, connect, MCP, CLI, verify, batches). **Not written:** the other pages of section 7 (concepts and guide pages of ALPHA, self-hosting, troubleshooting, contributing, `reference/workspace-layout`), `check:translations` ran report-only then (`strict` is true since P3-DOC-002), and `scripts/locale-routing.test.ts` and `scripts/base-path.test.ts` of section 12.1 do not exist (`check:built` covers the locale properties on the built output).
- **Formatting.** The copied `.ts`, `.tsx`, `.astro` and `.mjs` files were reformatted to tabs with Biome (indent tab 4, line width 120) in the isolated commit `style(site)`; the `.astro` templates were re-indented by hand because Biome formats only their front matter.

## 2. Locales, content tree and URLs

### 2.1 Locales

| Locale | URL prefix | HTML `lang` | hreflang | Role |
| --- | --- | --- | --- | --- |
| English | none (`/docs/...`) | `en` | `en` and `x-default` | default and source of truth (D-008) |
| Brazilian Portuguese | `/pt-br` (`/pt-br/docs/...`) | `pt-BR` | `pt-BR` | translation |

The locale code in a URL and in a file path is lower case (`pt-br`); the code in HTML attributes uses the BCP 47 casing (`pt-BR`). No other locale exists; adding one is a change of this SDD (a table row, a `lib/ui-text.ts` dictionary, a glossary column, a page mirror).

### 2.2 Directory layout

```text
site/
├── package.json
├── astro.config.mjs
├── content/
│   ├── navigation.ts                      sidebar: sections, groups, slugs, titles per locale
│   ├── glossary.json                      the product terms in English and pt-BR (section 10)
│   ├── translations.config.json           { "strict": true } (section 6)
│   ├── last-updated.json                  dates from git, generated, committed
│   ├── docs/
│   │   ├── en/
│   │   │   ├── getting-started/introduction.mdx
│   │   │   ├── concepts/<page>.mdx
│   │   │   ├── guide/<page>.mdx
│   │   │   ├── agents/<page>.mdx
│   │   │   ├── reference/<page>.mdx       hand-written reference pages only
│   │   │   ├── self-hosting/<page>.mdx
│   │   │   ├── troubleshooting/<page>.mdx
│   │   │   └── contributing/<page>.mdx
│   │   └── pt-br/                         same relative paths as en/, same file names
│   └── generated/                         generated pages, shared by both locales (section 8)
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

1. A page is `content/docs/<locale>/<dir>/<slug>.mdx`. The English file is the source; the pt-BR file has the **same `<dir>` and `<slug>`**. Slugs are never translated, because the URL is the identity of the page across locales and the staleness check pairs files by path.
2. `<slug>` is lower-case ASCII kebab-case, at most 48 characters. `<dir>` is one of the eight directories of section 3.
3. Only `.mdx` files are pages. Images live in `site/public/img/<dir>/<slug>/` and are shared by both locales; text inside an image is avoided, and when unavoidable the image has a `-pt-br` twin referenced from the pt-BR page.
4. A file that is not listed in `navigation.ts` fails `check:mdx`; a slug listed in `navigation.ts` whose English file is missing fails `check:mdx`. A pt-BR file whose English twin is missing fails `check:mdx`.
5. A page enters `navigation.ts` in the pull request that adds its English file. A pt-BR page added without being in the navigation is impossible by rule 4.

### 2.4 URL scheme

| Resource | English | Brazilian Portuguese |
| --- | --- | --- |
| Landing page | `/` | `/pt-br/` |
| Docs entry | `/docs/` redirects to `/docs/introduction/` | `/pt-br/docs/` redirects to `/pt-br/docs/introduction/` |
| Getting started page | `/docs/<slug>/` (prefix is empty, as vukem `navigation.ts:19-22`) | `/pt-br/docs/<slug>/` |
| Any other page | `/docs/<dir>/<slug>/` | `/pt-br/docs/<dir>/<slug>/` |
| Markdown twin | `/docs/<dir>/<slug>.md` | `/pt-br/docs/<dir>/<slug>.md` |
| Generated page | `/docs/reference/commands/<command>/` | `/pt-br/docs/reference/commands/<command>/` |
| Search index | `/search-index.json` | `/pt-br/search-index.json` |
| `llms.txt`, `llms-full.txt` | `/llms.txt`, `/llms-full.txt` | `/pt-br/llms.txt`, `/pt-br/llms-full.txt` |
| Changelog | `/changelog/` | `/pt-br/changelog/` |
| Sitemap, robots, 404 | `/sitemap.xml` (one file listing the pages of both locales), `/robots.txt`, `/404.html` (one each for both locales) | |

All URLs have a trailing slash, as in vukem (`astro.config.mjs`: `trailingSlash: "always"`, tested by `scripts/trailing-slash.test.ts`). With a base path (section 12.4) every URL above is prefixed by it; no code builds an absolute URL without `site.base`, enforced by `scripts/base-path.test.ts`. With a base path, crawlers read `robots.txt` and `sitemap.xml` only at the host root, so `/vukem-video/robots.txt` and `/vukem-video/sitemap.xml` are not read: the sitemap is submitted by hand in the search engines' consoles, or a custom domain is used (an open point for the author, section 12.4).

## 3. The navigation file

`site/content/navigation.ts` keeps the shape of vukem's (`navigation.ts:1-14`: `{title, dir, prefix?, pages}`) and adds localized titles, sidebar groups and generated sources. Page titles are **not** in this file: a page title is the `title` of the front matter of each locale file, as in vukem (`lib/content.ts` `readDoc` requires `title` and `description`). Only section and group titles are localized here.

```ts
export type Locale = "en" | "pt-br";
export type LocalizedText = Record<Locale, string>;

/** Pages listed by hand, in sidebar order, or expanded from a generated source. */
export type NavPages = string[] | { generated: "commands" };

export type NavGroup = {
  /** Sidebar sub-heading; omitted when the section is not grouped. */
  title?: LocalizedText;
  pages: NavPages;
};

export type NavCategory = {
  /** Folder under content/docs/<locale>/ (generated sections live in content/generated/). */
  dir: string;
  /** URL segment after /docs/; defaults to dir; "" puts the pages at the root of /docs. */
  prefix?: string;
  title: LocalizedText;
  groups: NavGroup[];
};

export const navigation: NavCategory[] = [
  {
    dir: "getting-started",
    prefix: "",
    title: { en: "Getting started", "pt-br": "Primeiros passos" },
    groups: [{ pages: ["introduction", "installation", "quick-start", "workspace", "projects"] }],
  },
  {
    dir: "concepts",
    title: { en: "Concepts", "pt-br": "Conceitos" },
    groups: [{ pages: ["how-it-works", "time", "commands-and-revisions", "tracks-clips-items", "render", "local-and-network-mode"] }],
  },
  {
    dir: "guide",
    title: { en: "Editing guide", "pt-br": "Guia de edição" },
    groups: [
      { title: { en: "Core editing", "pt-br": "Edição básica" }, pages: ["media-library", "timeline", "editing-tools", "preview", "clip-properties", "undo-and-history"] },
      { title: { en: "Creative tools", "pt-br": "Ferramentas criativas" }, pages: ["text-and-captions", "shapes", "audio", "effects-and-color", "transitions", "keyframes", "transcription", "silence-removal", "safe-area", "templates", "template-vertical-cut", "template-product-teaser", "template-quote-card"] },
      { title: { en: "Output", "pt-br": "Saída" }, pages: ["exporting", "jobs", "encoders-and-presets"] },
      { title: { en: "Interface", "pt-br": "Interface" }, pages: ["keyboard-shortcuts", "settings"] },
    ],
  },
  // agents, reference, self-hosting, troubleshooting, contributing follow the page list of section 7
];
```

Rules:

1. `navigation.ts` has no logic beyond the type and the array. `lib/content.ts` resolves a `{ generated: "commands" }` entry by reading `site/generated/commands.json` and producing one page per command (section 8).
2. The order of `groups` and `pages` is the sidebar order and the previous/next order.
3. Validation (`check:mdx`): every `title` has both locales and neither is empty; `dir` and `prefix` are unique; a slug appears once; every listed slug has an English file or a generated source.
4. The URL of a page is `/docs/<prefix or dir>/<slug>/` for English and `/pt-br/docs/<prefix or dir>/<slug>/` for Portuguese.

## 4. Language switcher, fallback, hreflang and canonical links

### 4.1 Language switcher

A control in the header (`components/header.tsx`) next to the theme toggle: a two-item menu `English` and `Português (Brasil)`, each a plain `<a>` (no JavaScript needed). On a docs page the link goes to the **same page** in the other locale (same `<dir>/<slug>`); on the landing page, to `/` or `/pt-br/`; on the changelog and 404 pages likewise. The current locale is marked `aria-current="true"`. The switcher keeps no cookie and no `localStorage` value, and the site never redirects by `Accept-Language` or `navigator.language`: crawlers and agents must see stable content at each URL, and `hreflang` (section 4.3) already tells search engines which version suits which reader. The link to the other locale exists whether or not that page is translated, because an untranslated page renders the English text with a notice (section 4.2).

### 4.2 Fallback to English with a visible notice

For a pt-BR URL whose pt-BR file does not exist, the site builds the page anyway (so every English URL has a pt-BR twin and no link breaks), with these properties:

1. The body is the English page.
2. A notice box sits above the title, using the `Callout` component: English `This page has not been translated yet. You are reading the English version.`, Portuguese `Esta página ainda não foi traduzida. Você está lendo a versão em inglês.` The notice text comes from `lib/ui-text.ts`, not from the page.
3. The article element carries `lang="en"` while `<html>` carries `lang="pt-BR"`.
4. Head: `<link rel="canonical">` pointing to the **English** URL, and no `hreflang` alternate for pt-BR (it is not a translation). No `noindex`: a `noindex` together with a canonical to another URL sends mixed signals, so only the canonical is kept.
5. The page is omitted from the `sitemap.xml` entries of the pt-BR locale (the single `sitemap.xml` lists every non-fallback page of both locales) and from the pt-BR `llms.txt` page list; the pt-BR `llms.txt` lists the English URL of that page under an "Untranslated" heading.
6. The Markdown twin `/pt-br/docs/<dir>/<slug>.md` returns the English body with the first line `> Not translated yet: this is the English text.` (twins have no JavaScript to carry a notice).
7. A **stale** translation (section 6) is not a fallback: it renders the pt-BR text without a notice. The staleness is reported to maintainers by the check, not shown to readers, because a visible notice on every page edited in English would be noise.

### 4.3 hreflang and canonical links

Every HTML page emits, in `<head>`:

| Page state | `rel="canonical"` | `rel="alternate" hreflang` |
| --- | --- | --- |
| English page, translated | its own URL | `en` (itself), `pt-BR` (the pt-BR URL), `x-default` (the English URL) |
| English page, not translated | its own URL | `en` (itself), `x-default` (itself) |
| pt-BR page, translated | its own URL | `en`, `pt-BR` (itself), `x-default` (the English URL) |
| pt-BR page, fallback | the English URL | none |
| Generated page | its own URL, in each locale | `en`, `pt-BR`, `x-default` |
| Landing page, changelog | its own URL | `en`, `pt-BR`, `x-default` |
| 404 | none | none |

URLs in these tags are absolute (`site.url` plus `site.base` plus the path, with the trailing slash). `sitemap.xml` repeats the alternates with `<xhtml:link rel="alternate" hreflang="..." href="...">` for translated pages (section 9). `scripts/check-built.ts` (section 12) verifies the table on the built output: each translated pair references each other symmetrically, no page has two canonicals, every canonical URL exists in `out/`.

Every page also carries `<link rel="alternate" type="text/markdown" href="<twin URL>">` so an agent that fetches HTML can find the twin.

### 4.4 Interface strings of the site

`lib/ui-text.ts` becomes a dictionary per locale (vukem's file is one `t` object, `ui-text.ts:1-41`): `export const ui: Record<Locale, UiText>` with the same keys as vukem's `t` plus `language`, `fallbackNotice`, `notFoundTitle`, `notFoundBody`, `lastUpdated`, `editThisPage`, `previous`, `next`, `searchPlaceholder`, `noResults`, `agents`, `changelog`, `translationNote`. The type `UiText` makes a missing key in one locale a type error. These are the strings of the **site chrome**; they are not UI strings of the editor (section 10).

## 5. Page template and writing rules

### 5.1 Front matter and structure

Follows vukem's page style (`site/README.md` "Add a page"; `docs/search/vukem-practices-and-releases.md` section 7: "The idea", "Quick start", "Common mistakes", "Next steps", "You will learn").

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

1. `title` and `description` are required in both locales. `since` (the first release in which the page is true) is optional and, when present, is the same in both files. `generated: true` appears only in generated pages.
2. "The idea" and "Next steps" are required in every page. "Quick start" is required in every page that teaches an action. "Common mistakes" is required where an error code or a known failure exists. "You will learn" is required in guide and getting-started pages.
3. Every action that exists in the UI, the CLI and the agent tools is shown for each surface that has it: a `Tabs` block with `UI`, `CLI`, `MCP` and `HTTP` tabs, the command names being those of SDD-000 section 1. The CLI tab shows the exact command; the MCP tab shows the tool name and the arguments object.
4. A feature that is not implemented in the released version carries `<Callout type="warning">Planned for ALPHA</Callout>` (or BETA), as vukem's `site/README.md` says. The callout is removed in the pull request that ships the feature.
5. Page links are root-relative `/docs/...` links with a trailing slash; `check:mdx` fails a link to a page that does not exist (vukem's check, `scripts/check-mdx.ts`), and `link-anchors.ts` fails a fragment that does not exist. A pt-BR page links to `/docs/...`; the renderer rewrites it to `/pt-br/docs/...` (the author writes the same link in both locales, so a translator cannot break links).
6. No bare `{` or `<` in prose (MDX), no HTML other than the MDX components, code lines at most 100 characters (`MAX_CODE_LINE`, `scripts/check-mdx.ts:16`). A fenced block names its language.
7. English only in code, command names, flag names, error codes, field names and identifiers (`E_REVISION_CONFLICT`, `prj_...`), in both locales. Only prose is translated.
8. No marketing text, no unsupported performance claim. A number in a page is either measured (with the source) or taken from a requirement identifier.

### 5.2 Who writes what

| Work | Who | Notes |
| --- | --- | --- |
| English page of a feature | the senior who implements the feature, in the same pull request | a feature task is not done without its page (Definition of Done in the ROADMAP) |
| English pages of Agents, Reference (hand-written), Self-hosting, Contributing, Getting started | `senior-d` | |
| Generated reference | nobody: generated (section 8) | |
| pt-BR translation | `estagiario-1` and `estagiario-2` (Haiku), one section each, in a separate pull request titled `docs(site): translate <section> to pt-BR` | they use `content/glossary.json`, keep code and identifiers untranslated, keep the structure and link targets identical |
| Review of English and pt-BR | the orchestrator | reads the staleness report; a translation is accepted when `check:mdx` passes, the headings match the English page one to one and the glossary terms are used |
| Staleness follow-up | the orchestrator opens a translation task for every page in the report | |

Every user-facing page exists in both locales: the English file and its pt-BR twin. A pull request that adds or substantially changes an English page may merge before its translation (the check only reports during Prototype and ALPHA, section 6); it must list the pending translation in the pull request body. From the first BETA release the check is strict.

## 6. The translation staleness check

The ADR-0024 item 3 check: "lists pages whose English version changed after the pt-BR one", written in the style of vukem's `last-updated.json` check (`scripts/check-last-updated.ts`, `lib/git-dates.ts`): dates come from git, full history is required, and a squash-merge artefact is tolerated.

### 6.1 Algorithm

`site/scripts/check-translations.ts` (Node with type stripping, like the other scripts; run `npm run check:translations` from `site/`):

1. Read `content/translations.config.json`: `{ "strict": true }` (set by P3-DOC-002; it was `false` until every page had a current twin).
2. List English pages: every `content/docs/en/**/*.mdx`. Generated pages and the landing page are not paired (generated pages have no translated text, section 8).
3. For each English file `E` compute `pt = content/docs/pt-br/<same relative path>`.
4. If `pt` does not exist: status `missing`.
5. Otherwise:
   - `tEn` = the latest **author date** (`%aI`, as `lib/git-dates.ts:37` does, because a rebase merge rewrites the committer date) among the commits that touched `E`, **excluding** commits whose message has a line `Translation: not-needed` (typo fixes, formatting, link fixes). The message is read with `git log --format=%x00%H%n%aI%n%B --name-only -- <E>`.
   - `tPt` = the latest author date among the commits that touched `pt`.
   - status `stale` when `tEn > tPt` (strictly later); otherwise `ok`. Equal dates are `ok`: a pull request that changes both files in one squash commit has the same date on both.
6. A squash-merge artefact is tolerated exactly as vukem tolerates it for `last-updated.json` (`lib/git-dates.ts` `pageDateExplainedBySquash`): when `E` and `pt` were last changed by the same commit, the status is `ok`.
7. A file with no git history yet (an uncommitted new page) is treated as having the date of the checkout `HEAD` commit, so the check works in a pull request.
8. Output: a Markdown table `page | English date | pt-BR date | status`, sorted by status (`stale`, `missing`, `ok`) then path, written to `$GITHUB_STEP_SUMMARY` when set and to standard output; one `::warning file=<path>::<status>` annotation per `stale` or `missing` page.
9. Exit code: `0` when `strict` is `false`; when `strict` is `true`, `1` if any page is `stale` or `missing`. The flag `--strict` overrides the config for a one-off run (`task release:preflight` of SDD-009 passes `--strict` from the first BETA release on).

The check needs full history (`fetch-depth: 0`), like `check-last-updated` (`site.yml:55-70`). It runs in `site.yml` (job `Last updated`, after `check:last-updated`) and therefore on every pull request that touches `site/`, `CHANGELOG.md` or `core/schema/`.

### 6.2 Why dates and a footer, not a revision counter

A counter in the front matter would be exact but needs a human to bump it. Dates need nothing from the author except the `Translation: not-needed` footer when a change does not affect meaning. With squash merges the footer must be in the final squash commit message; `check-translations.ts` reads the message of the commit on `main`, so the footer is written in the **pull request body** and the orchestrator verifies it there before merging. This works only with the repository setting "Default commit message for squash merges: Pull request title and description" (SDD-009 section 16.1, which also requires the `BREAKING CHANGE:` and `Security:` footers to be in the body): GitHub's own default for a single-commit pull request is the commit message, and the body would be dropped.

### 6.3 Tests

`check-translations.test.ts` uses a temporary git repository: English and pt-BR changed in one commit (`ok`); English changed later (`stale`); English changed later with the footer (`ok`); pt-BR missing (`missing`); strict mode exit codes.

## 7. Page list

Phase: `P` Prototype, `A` ALPHA, `B` BETA (RF-211: skeleton and reference in the Prototype, guide in ALPHA, complete in BETA). A page is written, in English and pt-BR, in the phase shown; it may be updated in later phases as features land. Source: **H** hand-written, **G** generated (section 8). Slugs are `<dir>/<slug>`; the Getting started pages have no `<dir>` in the URL.

The user guide follows the FreeCut outline of `docs/search/notes/vukem.md` section 4.3 (Start, Core editing, Creative tools, Output, Reference), without the FreeCut pages for features this product does not have (source monitor, motion workspace, scene browser with AI analysis; their purpose is covered by Transcription, Silence removal and Templates). The Agents section is specific to this product.

### 7.1 Getting started (`getting-started`, no URL prefix)

| Page title | Slug | Phase | Source |
| --- | --- | --- | --- |
| Introduction | `introduction` | P | H |
| Installation | `installation` | P | H |
| Quick start: your first export | `quick-start` | P | H |
| Workspace and storage | `workspace` | P | H |
| Projects | `projects` | P | H |

`quick-start` is the page NFR-040 measures: from download to the first exported video in under ten minutes, written for a person and ending with the agent route in a "Next steps" link.

### 7.2 Concepts (`concepts`)

| Page title | Slug | Phase | Source |
| --- | --- | --- | --- |
| How Vukem Video works | `concepts/how-it-works` | P | H |
| Time and frames | `concepts/time` | P | H |
| Commands, batches and revisions | `concepts/commands-and-revisions` | P | H |
| Tracks, clips and items | `concepts/tracks-clips-items` | P | H |
| Render: one plan, two executors | `concepts/render` | A | H |
| Local mode and network mode | `concepts/local-and-network-mode` | A | H |

### 7.3 Editing guide (`guide`)

| Group | Page title | Slug | Phase | Source |
| --- | --- | --- | --- | --- |
| Core editing | Media library | `guide/media-library` | P | H |
| Core editing | Basic timeline editing | `guide/timeline` | P | H |
| Core editing | Editing tools: trim, split, move | `guide/editing-tools` | P | H |
| Core editing | Preview and playback | `guide/preview` | P | H |
| Core editing | Clip properties | `guide/clip-properties` | A | H |
| Core editing | Undo and history | `guide/undo-and-history` | P | H |
| Creative tools | Text and captions | `guide/text-and-captions` | P | H |
| Creative tools | Shapes | `guide/shapes` | A | H |
| Creative tools | Audio editing | `guide/audio` | A | H |
| Creative tools | Effects and color | `guide/effects-and-color` | A | H |
| Creative tools | Transitions | `guide/transitions` | A | H |
| Creative tools | Animate with keyframes | `guide/keyframes` | A | H |
| Creative tools | Transcription | `guide/transcription` | A | H |
| Creative tools | Remove silence | `guide/silence-removal` | A | H |
| Creative tools | The safe area | `guide/safe-area` | A | H |
| Creative tools | Templates | `guide/templates` | A | H |
| Creative tools | Template: vertical cut with captions | `guide/template-vertical-cut` | A | H |
| Creative tools | Template: product teaser | `guide/template-product-teaser` | A | H |
| Creative tools | Template: quote card | `guide/template-quote-card` | A | H |
| Output | Exporting | `guide/exporting` | P | H |
| Output | Jobs and the queue | `guide/jobs` | A | H |
| Output | Encoders and export presets | `guide/encoders-and-presets` | A | H |
| Interface | Keyboard shortcuts (the basic shortcuts that exist in ALPHA; RF-074, advanced and customizable shortcuts, is BETA and extends this page then) | `guide/keyboard-shortcuts` | A | H |
| Interface | Settings | `guide/settings` | A | H |
| Tutorials | Make a vertical short with captions | `guide/tutorial-vertical-short` | B | H |
| Tutorials | Edit by prompting an agent, end to end | `guide/tutorial-agent-edit` | B | H |
| Tutorials | The desktop application | `guide/desktop` | B | H |

(The three template pages are one per template of D-019; their names change if the template names change in SDD-005/SDD-002. The group `Tutorials` is added to `navigation.ts` by the BETA task.)

### 7.4 Agents (`agents`)

| Page title | Slug | Phase | Source |
| --- | --- | --- | --- |
| Agents overview | `agents/overview` | P | H |
| Connect an agent (Claude Code, Cursor and others) | `agents/connect` | P | H |
| The MCP server | `agents/mcp` | P | H |
| The CLI for agents | `agents/cli` | P | H |
| How an agent verifies its work: view and lint | `agents/verify` | P | H |
| Batches, revisions and idempotency | `agents/batches` | P | H |
| The agent skill | `agents/skill` | A | H |
| Safety: undo, trash and limits | `agents/safety` | A | H |
| Recipes: cut, caption, export | `agents/recipes` | A | H |

### 7.5 Reference (`reference`)

| Page title | Slug | Phase | Source |
| --- | --- | --- | --- |
| Command catalogue | `reference/commands` | P | G |
| One page per command (48 commands in SDD-002, 47 of them public; for example `Add a clip`) | `reference/commands/<command>` | P | G |
| Timeline schema | `reference/timeline-schema` | P | G |
| CLI reference | `reference/cli` | P | G |
| HTTP API | `reference/http-api` | P | G |
| Error codes | `reference/error-codes` | P | G |
| Configuration variables | `reference/configuration` | P | G |
| Workspace layout | `reference/workspace-layout` | P | H |
| Events stream | `reference/events` | A | H |
| Glossary | `reference/glossary` | P | G (from `glossary.json`) |

### 7.6 Self-hosting (`self-hosting`)

| Page title | Slug | Phase | Source |
| --- | --- | --- | --- |
| Run the binary | `self-hosting/binary` | P | H |
| Run the Docker image | `self-hosting/docker` | A | H |
| Network mode and login | `self-hosting/network-mode` | A | H |
| API keys and scopes | `self-hosting/api-keys` | A | H |
| Put Caddy in front | `self-hosting/caddy` | A | H |
| Backups and upgrades | `self-hosting/backups-and-upgrades` | A | H |
| FFmpeg, codecs and licences | `self-hosting/ffmpeg-and-licences` | A | H |
| Verify a release | `self-hosting/verify-a-release` | A | H |
| Logs and OpenTelemetry | `self-hosting/observability` | A | H |
| Privacy: nothing leaves your machine | `self-hosting/privacy` | A | H |

### 7.7 Troubleshooting (`troubleshooting`)

| Page title | Slug | Phase | Source |
| --- | --- | --- | --- |
| Common problems | `troubleshooting/common-problems` | P | H |
| FFmpeg is not found | `troubleshooting/ffmpeg-missing` | P | H |
| No H.264 encoder works | `troubleshooting/no-encoder` | P | H |
| The workspace is locked | `troubleshooting/workspace-locked` | P | H |
| An import fails | `troubleshooting/import-fails` | A | H |
| Export is slow | `troubleshooting/slow-export` | A | H |
| The diagnostic bundle | `troubleshooting/diagnostic-bundle` | A | H |

### 7.8 Contributing (`contributing`)

| Page title | Slug | Phase | Source |
| --- | --- | --- | --- |
| Contributing guide | `contributing/guide` | P | H |
| Development setup | `contributing/setup` | P | H |
| Repository layout | `contributing/repository-layout` | P | H |
| Commits and pull requests | `contributing/commits-and-prs` | P | H |
| Writing documentation | `contributing/writing-docs` | P | H |
| Translating the documentation | `contributing/translating` | P | H |
| The release process | `contributing/releases` | A | H |

### 7.9 Outside the sidebar

| Page | URL | Phase | Source |
| --- | --- | --- | --- |
| Landing page | `/`, `/pt-br/` | P | H (both locales) |
| Changelog | `/changelog/`, `/pt-br/changelog/` | P | generated from `CHANGELOG.md` (section 11.3) |
| 404 | `/404.html` | P | H (both locales in one file) |

## 8. Generated reference pages

ADR-0024 item 5: the reference is generated from the binary and the schemas at build time, so it cannot drift. The generated pages are **committed**, and CI regenerates them and fails on any difference, exactly as ADR-0014 item 2 does for the schemas.

### 8.1 What is generated, and from where

| Pages | Source | How the generator reads it |
| --- | --- | --- |
| Command catalogue and one page per command (`reference/commands`, `reference/commands/<command>`) | `vukem-video command:help --json` (SDD-000 operation `CommandHelp`, ADR-0016 item 8): all commands with summary, argument schema, example, errors, scope, phase and, for each, its CLI, MCP and HTTP forms | `site/generated/commands.json` |
| Timeline schema (`reference/timeline-schema`) | the JSON Schema files under `core/schema/` (ADR-0014 item 2; the file names are those of SDD-001) | read directly, no binary needed; rendered as a nested table per entity with `description`, type, constraints, default and `x-status` |
| CLI reference (`reference/cli`) | the `cli` block of every descriptor of `command:help --json`, plus the global flags and exit codes table (SDD-000 section 8: exit `0`, `2` for retryable codes, `1` otherwise) | `site/generated/commands.json` |
| HTTP API (`reference/http-api`) | `vukem-video openapi:export -prefix /api/v1`, the vukem command that prints the OpenAPI 3.1 document in warmup mode (no database, no FFmpeg; `docs/cli.md:93`, `docs/api.md:270`) | `site/generated/openapi.json`; one section per tag, one block per operation, with the request and response schemas |
| Error codes (`reference/error-codes`) | the error registry of SDD-002 section A.8 and SDD-000 section 8, exported as `site/generated/errors.json` by `go run ./internal/tools/contractgen -errors` (code, meaning, retryable, HTTP status, since; the plain `contractgen` run writes `core/schema/` and `web/resources/js/generated/`, SDD-009 section 9.4 job 6) | `site/generated/errors.json` |
| Configuration variables (`reference/configuration`) | the configuration struct, through `go run ./internal/tools/configref -json` (the vukem tool of the same name, `Taskfile.yml:151-159`; vukem's has only `-check` and writes `docs/configuration.md`, so `REL-TOOLS` adds the `-json` flag when it copies it) | `site/generated/config.json`: variable, default, meaning, matches SDD-000 section 10 |
| Glossary (`reference/glossary`) | `site/content/glossary.json` (section 10) | |

**Required of other SDDs** (gaps found while writing this one, listed in the final answer): the descriptor returned by `command:help --json` must carry a `cli` block (command name, flags, positional arguments); `contractgen` must also write `errors.json`; `core/schema/` file names must be stable. If a shape differs, the generator adapts and the difference is recorded; this SDD is not changed.

### 8.2 The generation script

`site/scripts/generate-reference.ts` (Node with type stripping; `npm run reference` in `site/`; `task site:reference` and `task generate` of SDD-009 run it). Steps:

1. Build the binary once to a temporary path: `go build -o "$TEMP/vukem-video" ./web/cmd/app` (from the repository root; the script finds the root with `git rev-parse --show-toplevel`).
2. Write the four JSON files into `site/generated/`:
   - `vukem-video command:help --json > site/generated/commands.json`
   - `vukem-video openapi:export -prefix /api/v1 > site/generated/openapi.json`
   - `go run ./internal/tools/configref -json > site/generated/config.json`
   - `go run ./internal/tools/contractgen -errors > site/generated/errors.json`
   The binary is run with `VUKEM_VIDEO_HOME` pointing at an empty temporary directory and with no network. JSON is written with sorted keys and two-space indentation so diffs are stable; no timestamp, no absolute path and no version string enters the output (the page shows the version only through the changelog).
3. Render the MDX pages into `site/content/generated/reference/`: one file per command, plus `commands.mdx`, `timeline-schema.mdx`, `cli.mdx`, `http-api.mdx`, `error-codes.mdx`, `configuration.mdx`, `glossary.mdx`. The front matter has `generated: true` and `title` and `description` from a dictionary in the script (a `Record<Locale, ...>`, so the page chrome exists in both locales). The body is the same English text for both locales (identifiers, schema descriptions and error meanings are English; they come from Go struct tags and the registry, D-008). The first line of the body is a comment `{/* Generated by site/scripts/generate-reference.ts. Do not edit. */}`.
4. Render command pages in the page template of section 5 where it applies: summary, "Arguments" table from the schema, "Example" with the CLI form and the MCP form, "Errors" table with links to `error-codes`, "Scope and phase".
5. Idempotent: running it twice produces no diff.

`npm run reference:check` runs the script and `git diff --exit-code -- generated content/generated` (it is the same check that the `Generated files` job of `ci.yml` runs from the repository root, SDD-009 section 9.4 job 6).

### 8.3 Locales of generated pages

Generated pages exist in both locales (every user-facing page does) but are **not translated**: `lib/content.ts` serves `content/generated/<path>.mdx` under `/docs/<path>/` and `/pt-br/docs/<path>/`, with the chrome (title, description, sidebar, notice line) in the page locale and the body in English. They show no "not translated" notice; instead a one-line note under the title says, in the page locale, that the reference is generated from the program and is in English. They are excluded from the staleness check (section 6) and from `check:last-updated` pairing (their date comes from `content/generated/` git history, section 11.2).

## 9. Agent readability

ADR-0024 item 6 is a requirement, in both locales, built by the same code as vukem's endpoints (`src/pages/llms.txt.ts`, `llms-full.txt.ts`, `docs/[...slug].md.ts`, `sitemap.xml.ts`, `robots.txt.ts`, `404.astro`).

| Artefact | URL | Content |
| --- | --- | --- |
| `llms.txt` | `/llms.txt`, `/pt-br/llms.txt` | llms.txt convention: `# Vukem Video`, a one-sentence summary as a blockquote, the line "Full documentation in one file: ...", the line "Every page is also available as Markdown: add `.md` to its URL", then one `##` heading per navigation section (title in the locale) with `- [Title](URL): description` per page (`llms.txt.ts:20-27`). The pt-BR file lists translated and generated pages with pt-BR URLs and, under an `Untranslated` heading, the English URLs of the pages not yet translated |
| `llms-full.txt` | `/llms-full.txt`, `/pt-br/llms-full.txt` | every page's Markdown twin concatenated in sidebar order, each preceded by a `---` line and its `Source:` URL. The pt-BR file holds translated and generated pages only and starts with a line pointing to the English file for the rest |
| Markdown twin | `/docs/<dir>/<slug>.md`, `/pt-br/docs/<dir>/<slug>.md` | `# Title`, a `> description` line, `Source: <HTML URL>`, then the page body with MDX components converted to Markdown by `lib/text.ts` `mdxToMarkdown` (`[...slug].md.ts:15`). Content type `text/markdown; charset=utf-8`. Every page of both locales has one, including generated pages and fallback pages (section 4.2 item 6) |
| `sitemap.xml` | `/sitemap.xml` | one URL entry per HTML page of both locales (not fallback pages, not the twins), the landing pages and the changelog pages, with `xhtml:link` alternates as in section 4.3 |
| `robots.txt` | `/robots.txt` | `User-agent: *`, `Allow: /`, `Sitemap: <site URL>/sitemap.xml`. Under a base path (the default `https://alexgalhardo.github.io/vukem-video`) this file is not at the host root and crawlers do not read it, nor the sitemap; the sitemap is then submitted by hand in the search consoles, or a custom domain is used (section 12.4) |
| 404 | `/404.html` | a real HTML page, bilingual: it contains both an English and a Portuguese paragraph and a link to each locale's docs entry; a one-line inline script hides the language that does not match the URL prefix (`/pt-br/`). GitHub Pages serves `404.html` with HTTP status 404 for every unknown path under the site, including `/pt-br/...`, so an agent gets a real 404, not the shell with status 200 that FreeCut returns |
| Alternate link | in every page `<head>` | `<link rel="alternate" type="text/markdown" href="...">` |

The site has no analytics and loads no external script, font or stylesheet, and it is hosted on GitHub Pages and nowhere else (D-015, D-016). Fonts are the self-hosted `@fontsource` packages that vukem's site already uses. A pull request that adds an analytics script, a CDN resource, an embedded third-party widget or another hosting target is rejected; property 7 below enforces the first three in CI.

Properties checked by `scripts/check-built.ts` after `astro build` (section 12) and by tests next to the endpoints (`markdown-twin.test.ts` of vukem, extended):

1. For every navigation page and generated page and each locale: `out/<path>/index.html` and the twin `.md` exist.
2. No twin contains `<`-prefixed JSX components or `import` lines.
3. Every URL in `llms.txt` (both) resolves to a file in `out/`.
4. `sitemap.xml` contains exactly the HTML pages that are not fallbacks.
5. `out/404.html` exists and contains both languages.
6. After deployment (acceptance check): `curl -s -o /dev/null -w "%{http_code}" <site>/does-not-exist/` and the same under `/pt-br/` print `404`.
7. **No external resource** (D-015, D-016): every `src` and `href` attribute of every built HTML page, and every `url(...)` of the built CSS, is same-origin (starts with the base path or is a relative path), a `mailto:` or `#` link, or an outbound link that is written in page content (a `[text](https://...)` link of an `.mdx` file) and is not a `<script>`, `<link rel="stylesheet">`, `<link rel="preload">`, `<img>`, `<iframe>` or `<source>` resource. A `<script src>`, a stylesheet, a font or an image served from another origin fails `check-built.ts`.

## 10. Glossary and the boundary with UI strings

The editor's UI strings are a different i18n concern: they are specified in the UI architecture (SDD-012), live with the React code, and are loaded by the application, not by the site. This SDD states only one rule about them: **the glossary of product terms is shared**. `site/content/glossary.json` is the single list (an array of `{ "id", "en", "pt-br", "note" }`); the documentation translators and the UI translators use it, and SDD-012 imports the same file for its UI dictionary instead of keeping a second copy. A term is changed in this file first, then everywhere (the rule SDD-000 states for its own names).

The 30 terms. Command, field and error-code names are never translated; this table is for prose and UI labels.

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

## 11. Search, last updated and the changelog page

### 11.1 Search

vukem builds a search index at build time and loads the dialog on first open (`src/pages/search-index.json.ts`, `lib/search.ts`, `components/search-dialog.tsx`; vukem `CHANGELOG` v0.41.0 "Load the search dialog on first open"). Ours builds one index per locale: `/search-index.json` and `/pt-br/search-index.json`, each with one entry per page of that locale (title, URL, description, headings, plain text; fallback pages are indexed in English under the pt-BR index with a `fallback: true` flag so a search finds them). The dialog fetches the index of the current locale. Matching is case-insensitive and accent-insensitive: both the query and the indexed text are normalized with `normalize("NFD")` and a removal of combining marks, so `transicao` finds `transição`. Keyboard shortcut and behaviour are vukem's (`Ctrl K`).

### 11.2 "Last updated" from git

Unchanged from vukem (`scripts/last-updated.ts`, `lib/git-dates.ts`, `scripts/check-last-updated.ts`): `content/last-updated.json` maps each page file to the ISO date of its last commit (author date, so a rebase merge keeps it), is generated by `npm run last-updated` and never edited by hand, and is checked in CI and by the pre-push hook (`lefthook.yml:40-42` equivalent in SDD-009 section 15.1). Changes: keys are `docs/en/<dir>/<slug>.mdx`, `docs/pt-br/<dir>/<slug>.mdx` and `generated/reference/...mdx`; `isDocPage` in `check-last-updated.ts` (vukem line 14-16: `.mdx` under `docs/`) also accepts `generated/`; the date is shown formatted with `Intl.DateTimeFormat` in the page locale (`en` or `pt-BR`), in UTC, as `Last updated: ...` or `Última atualização: ...`. Both checks run on every pull request through `docs-last-updated.yml` (SDD-009 section 9.6), so a page left out of the file cannot hide.

### 11.3 The changelog page

`src/pages/changelog.astro` and `lib/changelog.tsx` render `CHANGELOG.md` at build time (`site.yml` also triggers when `CHANGELOG.md` changes, `site.yml:8-11`). Both locales render the same English changelog inside the localized page chrome: release notes are English (D-008, SDD-009 section 3), and translating them would make two sources. The changelog page has a localized title and description and a note, in the page locale, that release notes are in English.

## 12. Build, checks and deployment

### 12.1 Scripts in `site/package.json`

Copied from vukem (`package.json:6-14`) and extended:

| Script | Command | Role |
| --- | --- | --- |
| `dev` | `astro dev` | local preview at `http://localhost:3000` |
| `build` | `astro build` | static export to `out/` |
| `lint` | `eslint .` | |
| `check:mdx` | `node scripts/check-mdx.ts` | MDX compiles, components exist, `/docs/` links resolve, no bare braces; extended: every nav slug has an English file, every pt-BR file has an English twin, titles exist in both locales |
| `last-updated` | `node scripts/last-updated.ts` | refresh `content/last-updated.json` |
| `check:last-updated` | `node scripts/check-last-updated.ts` | every page has a date at least as new as its last commit |
| `check:translations` | `node scripts/check-translations.ts` | section 6 |
| `reference` | `node scripts/generate-reference.ts` | section 8 |
| `reference:check` | `node scripts/generate-reference.ts && git diff --exit-code -- generated content/generated` | section 8.2 |
| `check:built` | `node scripts/check-built.ts` | section 9 properties (including the no-external-resource property 7) and the section 4.3 table on `out/` |
| `test:scripts` | `node --test scripts/check-last-updated.test.ts scripts/committed-dates.test.ts scripts/trailing-slash.test.ts scripts/check-translations.test.ts` | dependency-free tests, so the `Last updated` jobs need no `npm ci` of the whole site (vukem comment, `site.yml:25-26`); `base-path.test.ts` is not here because it builds the site |
| `test:site` | `node --test scripts/link-anchors.test.ts scripts/markdown-twin.test.ts scripts/locale-routing.test.ts scripts/base-path.test.ts` | tests that import site modules or build the site; needs `npm ci` |
| `test:base-path` | `node --test scripts/base-path.test.ts` | the base path test alone (builds the site under a base path and under `/`, section 12.4); it is also run by `test:site` |

(vukem's `legacy-path-redirect` and `current-equivalent` tests are not copied.)

### 12.2 CI: `site.yml` and `docs-last-updated.yml`

Both are specified in SDD-009 section 9.6; the steps of `site.yml` are, in order:

- Job `build`: `actions/checkout`; `actions/setup-go` (`go-version-file: go.mod`, needed by `reference:check`, which builds the binary); `actions/setup-node` (Node 24, npm cache on `site/package-lock.json`); `npm ci`; `npm run test:site` (which includes the base path test); `npm run lint`; `npm run check:mdx`; `npm run reference:check` (run with `working-directory: site`); `npm run build`; `npm run check:built`; on `main` or manual dispatch, `actions/upload-pages-artifact` with `path: site/out`. In the Taskfile the same list is `task site:build` (SDD-009 section 14).
- Job `last-updated`: `fetch-depth: 0`; `actions/setup-node` (Node 24); `npm run test:scripts`; `npm run check:last-updated`; `npm run check:translations`.

These step lists are the same as in SDD-009 section 9.6.

- Job `deploy` (only on `main` or manual dispatch; needs `build` and `last-updated`): `environment: github-pages`; `actions/deploy-pages`. Permissions: `pages: write`, `id-token: write`.

The job `build` runs the same `reference:check` as the `Generated files` job of `ci.yml` (SDD-009 9.4); the duplication is intended: `Generated files` runs on every pull request and is required, while `site.yml` is path-filtered and guards the deploy.

### 12.3 Deployment

GitHub Pages, source "GitHub Actions" (ADR-0024 item 2; D-015): a push to `main` that touches the paths of `site.yml` builds and deploys. No Railway, no container, no standalone server. The deployed site is exactly `out/`. The in-application help links to the site (ADR-0024 item 8); the editor embeds no documentation.

### 12.4 Site URL and base path

`SITE_URL` (a repository variable, optional) is the public URL. Default: `https://alexgalhardo.github.io/vukem-video`, a GitHub Pages project site, so `site.base` is `/vukem-video` (and `robots.txt` and the sitemap are not read at the host root, section 2.4: the author submits the sitemap by hand or chooses a custom domain, **an open point for the author**) (**Decision of this SDD**: the author has named no domain; the Studio's ROADMAP recorded a planned `studio.vukem.dev` that was never pointed). Setting `SITE_URL` to a custom domain (for example the author's) makes `base` `/`, and the author adds `site/public/CNAME` with that domain in the same pull request. Every internal link, asset path, canonical URL, sitemap URL and `llms.txt` URL is built from `site.url` and `site.base` (`lib/site.ts`); `scripts/base-path.test.ts` builds the site under both a base path and `/` and fails on any `href="/..."` or `src="/..."` that does not start with the base.

### 12.5 Local commands

Defined in SDD-009 section 14: `task site:dev`, `task site:build`, `task site:reference`, `task site:translations`, `task site:dates`.

## 13. Implementation tasks owned by this SDD

Identifiers for the ROADMAP; all `senior-d` unless stated.

| Id | Task | Done when |
| --- | --- | --- |
| `SITE-COPY` | Copy the files of section 1.1 from the current vukem `site/` (record the vukem commit in the report), apply 1.2, drop 1.3, run `npm install` to create `package-lock.json`, replace the icon and logo; then the isolated `style(site)` reformat to tabs (section 1.2) | `npm ci && npm run build` succeeds with one placeholder page per locale, from a clean clone (so `Base.astro`, `prerender.tsx`, `public/` and `.gitignore` are all copied) |
| `SITE-I18N` | Locale routing, content loader with fallback, switcher, notice, hreflang, canonical, per-locale search index, `lib/ui-text.ts` dictionary, bilingual 404 and landing | acceptance checks 2 to 8 |
| `SITE-NAV` | `navigation.ts` with the types and categories of section 3; the `check:mdx` extensions | acceptance check 3 |
| `SITE-REF` | `scripts/generate-reference.ts`, the four JSON sources, the generated pages | acceptance check 10 |
| `SITE-AGENT` | `llms.txt`, `llms-full.txt`, twins, sitemap, robots, `check-built.ts` in both locales, with the no-external-resource assertion | acceptance checks 11 to 13 and 18 |
| `SITE-TRANSLATE-CHECK` | `check-translations.ts` and its tests | acceptance check 9 |
| `SITE-CI` | `site.yml`, `docs-last-updated.yml` changes (with `REL-CI` of SDD-009), `translations.config.json` | acceptance check 14 |
| `SITE-P` | The Prototype pages of section 7 (English by `senior-d` and feature seniors, pt-BR by the interns) | every `P` row exists in both locales |
| `SITE-A`, `SITE-B` | The ALPHA and BETA pages; `SITE-B` also sets `strict` to `true` in `translations.config.json` | every `A` or `B` row exists in both locales; the check is strict |

## 14. Acceptance

Each check is verifiable from this document and the repository alone.

1. `cd site && npm ci && npm run build` succeeds and `out/` contains `index.html`, `pt-br/index.html`, `docs/introduction/index.html`, `pt-br/docs/introduction/index.html`, `404.html`, `llms.txt`, `pt-br/llms.txt`, `sitemap.xml`, `robots.txt`.
2. For every page listed in `navigation.ts` there is, in `out/`, an English page, a pt-BR page, an English twin and a pt-BR twin (`npm run check:built` passes). Deleting one pt-BR file still builds the pt-BR URL, now as a fallback page.
3. `npm run check:mdx` fails when: a slug is in `navigation.ts` with no English file; a pt-BR file has no English twin; a locale of a section title is empty; a link points to a page that does not exist.
4. A fallback pt-BR page shows the notice text of section 4.2 above its title, has `<html lang="pt-BR">` with an inner `lang="en"` article, a canonical link to the English URL, no `noindex` and no pt-BR hreflang; its twin starts with `> Not translated yet`.
5. A translated page has `canonical` to itself and `hreflang` `en`, `pt-BR` and `x-default` pointing to each other symmetrically (verified by `check-built.ts` for every pair).
6. The language switcher on every docs page links to the same `<dir>/<slug>` in the other locale; on `/` to `/pt-br/` and back; it needs no JavaScript (the page works with scripting disabled).
7. `sitemap.xml` lists every non-fallback page of both locales with `xhtml:link` alternates and lists no `.md` twin and no fallback page.
8. Searching `transicao` in the pt-BR dialog finds the page titled `Transições` once it exists; searching `trim` in the English dialog finds `Editing tools: trim, split, move`.
9. `npm run check:translations` on a scratch repository behaves as in section 6.3: `ok` for same-commit changes, `stale` when English is newer, `ok` when the newer English commit carries `Translation: not-needed`, `missing` when the pt-BR file is absent; exit `0` normally and `1` in strict mode when anything is stale or missing. In CI the summary table appears in the job summary.
10. `npm run reference` run twice produces no diff; changing a command description in the Go struct tags and rerunning changes only that command page and `commands.json`; a pull request that changes a command without regenerating fails the `Generated files` job and `reference:check`.
11. `llms.txt` and `pt-br/llms.txt` follow section 9 and every URL in them resolves in `out/`; `llms-full.txt` contains every page's twin in sidebar order.
12. No twin contains JSX tags or `import` lines.
13. After deployment: `curl -sI <site>/docs/introduction.md` returns `200` with `text/markdown`; `curl -s -o /dev/null -w "%{http_code}" <site>/nope/` and `<site>/pt-br/nope/` print `404`; the 404 body has both languages.
14. `site.yml` on a pull request that edits `site/content/docs/en/guide/exporting.mdx` runs `build` and `last-updated`, and the job summary lists `guide/exporting` as `stale` when its pt-BR twin is older and `strict` is `false`; the workflow still passes. With `strict` set to `true` it fails.
15. `npm run test:base-path` (or `scripts/base-path.test.ts`) passes with `SITE_URL=https://alexgalhardo.github.io/vukem-video` and with `SITE_URL=https://example.org`: no absolute link lacks the base.
16. `content/glossary.json` has the 30 terms of section 10, each with `en` and `pt-br`, and is the file SDD-012 imports (a grep of `web/resources/js` finds no second copy of the term list once SDD-012 is implemented).
17. Every `P` row of section 7 exists in both locales at the end of the Prototype; every `A` row at the end of ALPHA; every `B` row at the end of BETA, with `strict` equal to `true`.
18. No external resource (D-015, D-016): `npm run check:built` passes on the real `out/` and fails on a scratch change that adds `<script src="https://example.org/x.js">`, an external stylesheet or an external font to a page; the built site makes no request to another origin when opened with the network blocked (property 7 of section 9).
19. A person following only `getting-started/quick-start` reaches an exported MP4 in under ten minutes on a clean machine (NFR-040), timed by the author at the end of the Prototype and recorded in the ROADMAP.
