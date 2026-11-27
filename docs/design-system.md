# Stuarty design system

- Status: Accepted (2026-11-14). Written by the designer, to be reviewed by the architect and by senior-e (editor UI). Sections 2.1 to 2.14 and 4.2 to 4.4 were revised by epic 74 to the final redesign (semantic tokens, the type scale, radii, elevation, motion, the shared components); where an older passage of this document still names `--primary`, `--muted` or another former token, it is an alias of a token of section 2 (section 2.1).
- Applies to: the web UI under `web/resources/` (the Inertia pages and the editor), the desktop shell (`desktop/`, ADR-0023), and, where section 8.4 says so, the documentation site (`site/`, SDD-010).
- Sources of truth, which this document follows and never overrides: author decisions D-008, D-013, D-016, D-022, D-023, D-024, D-028, D-029; [SDD-012](sdds/012-ui-architecture.md) (editor UI architecture: §1 thin client, §3 kept front end, §6 layout, §7 timeline, §11 preview, §12 inspector, §16 i18n, §17 theme, §18 accessibility, §21 desktop); [SDD-002](sdds/002-command-catalogue.md) A.4 (time strings) and A.8 (the error envelope); [SDD-003](sdds/003-agent-interfaces.md) §1.4; [SDD-010](sdds/010-documentation-site.md) §1 and §10 (site shell, glossary); [ADR-0007](adrs/0007-thin-ui-client.md), [ADR-0012](adrs/0012-text-and-shape-rasterization.md), [ADR-0013](adrs/0013-browser-preview-stack.md), [ADR-0023](adrs/0023-desktop-shell-deferred.md); [PRD-001](prds/001-video-editor.md) §3 (use cases), RF-070 to RF-075.
- Code it describes: `web/resources/css/app.css` (the only token file), `web/components.json` (kept style record: base colour `neutral`, icon library `lucide`), `web/resources/js/components/ui/*` (`@base-ui/react`), `web/resources/js/layouts/AppLayout.tsx`, and the editor tree of SDD-012 §3.2.
- Canonical FigCraft design source: [docs/design/final/README.md](design/final/README.md) (`docs/design/final/stuarty-redesign-final-trilha.figcraft` and `docs/design/final/tokens-tailwind-final.css`). Every UI epic that adds a screen or changes a layout updates the versioned FigCraft source alongside code.

A paragraph headed **Decision of this document** fills a gap that no SDD or ADR covers. Each is listed again in section 10 with the specification that should absorb it.

## Contents

1. [Principles](#1-principles)
2. [Design tokens](#2-design-tokens)
3. [Layout](#3-layout)
4. [Components](#4-components)
5. [Iconography, illustration and the product mark](#5-iconography-illustration-and-the-product-mark)
6. [Content style](#6-content-style)
7. [Accessibility](#7-accessibility)
8. [Web and desktop](#8-web-and-desktop)
9. [Governance](#9-governance)
10. [Decisions of this document and open points for the specifications](#10-decisions-of-this-document-and-open-points-for-the-specifications)
11. [Brand refresh: the identity from the official icon (2026-11-20)](#11-brand-refresh-the-identity-from-the-official-icon-2026-11-20)

## 1. Principles

Every rule below has a reason in a decision of the author or an ADR. When two principles pull apart, the earlier one wins.

| # | Principle | What it means in practice | Source |
| --- | --- | --- | --- |
| P1 | The UI never computes an edit | Components draw what the core returns and send commands. A component shows a ghost while a request is pending, never the predicted result. No visual state may imply that an edit succeeded before the core answered (no optimistic "done" check marks, no item moved in the read model) | ADR-0007, SDD-012 §1 rules T1 to T10 |
| P2 | Content first, calm chrome | The picture and the timeline are the largest and most saturated things on screen. Chrome is a calm ink (dark) or paper (light) neutral; the accent (indigo) marks the main action, teal is what comes from the agent, amber is what waits for a person, and colour means something or it is not used. The preview surround is the canvas colour of each theme (section 2.5). The earlier ban on a hue in chrome and on a brand-coloured button is lifted, as the note under this table explains | D-024, SDD-012 §17 item 4 |
| P3 | Minimalist editor | One way to reach each command plus the palette; no decorative illustration, gradients or glass effects; fewer, larger regions beat many small panels. Motion is only what the tokens of section 2.13 allow, and the editor components still add none of their own (SDD-012 §17 item 5). Loading is a static skeleton and a sentence, never a spinner | D-013, SDD-012 §17 item 5 |
| P4 | Agent actions are visible and attributed | Any change made by another actor shows who made it (actor label, `--agent` colour, icon), where (outlined items), and when (relative time). The UI never hides or merges another actor's work into "you" | RF-118, SDD-012 §5.8, §15 |
| P5 | Errors are the core's words | An error shows a localized headline for its code, then the core's `message` and `hint` exactly as received. The UI never paraphrases or invents a cause | SDD-002 A.8, SDD-012 §4.4, rule T9 |
| P6 | One look in browser and desktop | The desktop shell loads the same web UI with the same tokens, fonts and components. Nothing may depend on a browser-only or webview-only feature outside SDD-012 §21.1 | ADR-0023, SDD-012 §21 |
| P7 | Accessible by construction | Keyboard first, visible focus, names on every control, WCAG 2.2 AA contrast in both themes, nothing conveyed by colour alone | RF-075, SDD-012 §18 |
| P8 | Local and quiet | No asset, font or icon is loaded from another origin (fonts are bundled through `@fontsource`; icons are inline SVG from `lucide-react`). No telemetry, no "what's new" calls | D-016, RF-206, SDD-012 §2.2 (CSP) |

**What may be lifted.** By decision of the author for the final redesign, an aesthetic prohibition of the former design system may be lifted when the change states its justification. Five rules are not aesthetic and stay mandatory: WCAG 2.2 AA in both themes, tokens only, nothing conveyed by colour alone, reduced motion, and no external origin. One prohibition is lifted so far: a hue in chrome and a brand-coloured button, because the redesign needs one colour for the main action of a screen and another for what comes from the agent.

## 2. Design tokens

### 2.1 Where tokens live and how they are named

All tokens are CSS custom properties in `web/resources/css/app.css`. The semantic tokens of the final redesign are defined in `:root` (light) and overridden in `.dark` (dark), in OKLCH, with the hex of the redesign in a comment. Each name is also listed in `@theme inline`, mapped to itself, so Tailwind 4 turns it into utilities (`bg-surface`, `text-text-muted`, `border-border-strong`, `bg-accent-soft`); the unlayered rule in `:root` and `.dark` wins over the copy Tailwind writes in its theme layer. Dark is applied by the `dark` class on `<html>` (`@custom-variant dark (&:where(.dark, .dark *))`), set before paint from `localStorage.theme`, **dark when absent** (SDD-012 §17 item 1).

| Group | Names | Who uses it |
| --- | --- | --- |
| Semantic, final redesign | `--color-background`, `surface`, `surface-raised`, `surface-sunken`, `surface-hover`, `surface-selected`, `text`, `text-muted`, `text-subtle`, `text-disabled`, `text-on-accent`, `text-on-agent`, `text-on-danger`, `border`, `border-strong`, `focus`, `icon`, and the families `accent`, `agent`, `success`, `warning`, `danger` (each with `-soft` and `-text`; `accent`, `agent` and `danger` also `-hover`; `agent` also `-border`), plus `shadow-sm`, `shadow-md`, `shadow-lg` as colours | every page, component and editor chrome |
| Aliases of the former system | `--background`, `--foreground`, `--card`, `--popover`, `--primary`, `--secondary`, `--muted`, `--muted-foreground`, `--accent-foreground`, `--destructive`, `--border`, `--input`, `--ring`, `--info`, `--sidebar*`, and their `--color-*` utilities (`bg-primary`, `text-muted-foreground`) | pages not yet migrated |
| Brand | `--brand`, `--brand-foreground`, `--brand-accent`, `--brand-subtle`, `--brand-text` (section 11.3) | the mark, the one primary action of a page, the active navigation item; the logo stays |
| Editor (SDD-012 §17) | `--ed-*` | `web/resources/js/editor/**` and the image editor only |

**Aliases.** A page that was not migrated repaints with no change to its code, because each former name points to a new token, in one rule for both themes. An alias is deleted by the last epic that stops using it.

| Former name | Points to | Why |
| --- | --- | --- |
| `--background`, `--foreground` | `--color-background`, `--color-text` | |
| `--card` | `--color-surface` | |
| `--popover` | `--color-surface-raised` | menus, dialogs |
| `--primary`, `--primary-foreground` | `--color-accent`, `--color-text-on-accent` | the main action is the accent |
| `--secondary`, `--secondary-foreground` | `--color-surface-raised`, `--color-text` | |
| `--muted`, `--muted-foreground` | `--color-surface-sunken`, `--color-text-muted` | |
| `--destructive` | `--color-danger-text` | it is used as text and as a tint, so it takes the text variant of danger |
| `--border`, `--input`, `--ring` | `--color-border`, `--color-border-strong`, `--color-focus` | |
| `--info` | `--color-accent-text` | |
| `--sidebar*` | `--color-surface-sunken`, `--color-surface-hover`, `--color-accent`, `--color-border`, `--color-focus` | |

**A name that changed meaning.** `--color-accent` used to be a hover surface (shadcn `bg-accent`); it is now the accent hue. The few places that used `bg-accent` as a hover or pressed surface use `bg-surface-hover` or `bg-surface-selected`.

A raw Tailwind palette class (`bg-indigo-600`, `text-red-500`) or a literal colour in a component is not allowed; every colour is a token.

Values are OKLCH. Every pair was checked with the WCAG 2.x formula after converting OKLCH to sRGB (section 2.6), in both themes.

### 2.2 The two palettes

The tokens are drawn from two neutral scales and four hues, with their hex values in the comments of `app.css`. Components never use these steps directly: they use the semantic tokens.

| Scale | Steps used | Role |
| --- | --- | --- |
| Ink (cool, dark theme) | 950 `#0F1015`, 900 `#15161C`, 850 `#1B1C24`, 800 `#22242D`, 750 `#2A2C37`, 700 `#363947`, 500 `#747A8F`, 400 `#9A9EB1`, 300 `#BCBFCD`, 100 `#EEEFF4` | backgrounds, surfaces, borders and text of the dark theme |
| Paper (warm, light theme) | 0 `#FFFFFF`, 50 `#FAF9F5`, 100 `#F3F1EA`, 200 `#EAE7DD`, 300 `#DCD8CB`, 500 `#85816F`, 600 `#66634F`, 700 `#4E4C3F`, 900 `#1E1D18` | the same roles in the light theme |
| Indigo | 100, 300, 550, 600, 650, 700, 900, 950 | the accent, derived from the logo |
| Teal | 100, 200, 300, 400, 700, 800, 950 | what comes from the agent |
| Green, amber, red | 100, 300, 500 or 700, 950 | success, waiting for a person, danger |

### 2.3 Colour: surfaces, text, borders and focus

| Token (Tailwind) | Dark | Light | Use |
| --- | --- | --- | --- |
| `--color-background` (`bg-background`) | `#15161C` | `#FAF9F5` | page background |
| `--color-surface` (`bg-surface`) | `#1B1C24` | `#FFFFFF` | cards and panels |
| `--color-surface-raised` | `#22242D` | `#FFFFFF` | dialogs, menus, popovers, the secondary button |
| `--color-surface-sunken` | `#0F1015` | `#F3F1EA` | the sidebar, the fields, quiet areas |
| `--color-surface-hover` | `#2A2C37` | `#EAE7DD` | the hover of a row or a control, a skeleton, a progress track |
| `--color-surface-selected` | `#1C1C45` | `#E9E8FC` | the active item, a pressed toggle |
| `--color-text` | `#EEEFF4` | `#1E1D18` | main text |
| `--color-text-muted` | `#BCBFCD` | `#4E4C3F` | supporting text |
| `--color-text-subtle` | `#9A9EB1` | `#66634F` | metadata and placeholders |
| `--color-text-disabled` | `#747A8F` | `#85816F` | a disabled control (exempt from contrast) |
| `--color-border` | `#363947` | `#DCD8CB` | dividers and card edges, decorative |
| `--color-border-strong` | `#747A8F` | `#85816F` | the boundary of a control: fields, checkboxes, the unchecked switch (3:1) |
| `--color-focus` | `#B3B5FF` | `#4339CC` | the focus ring (3:1) |
| `--color-icon` | `#BCBFCD` | `#4E4C3F` | an icon with no text beside it |

Contrast pairs: every text token on every surface that can hold it, and the focus and border tokens at 3:1, are in section 2.6 and in `check:contrast`.

### 2.4 Colour: the accent, the agent, and the states

Each family has a solid token (`--color-<family>`), a `-soft` surface and a `-text` colour that is safe as text on the page, a card and its own `-soft` surface. A status is always an icon and a word (`StatusBadge`, section 4.4).

| Family | Meaning | Dark | Light |
| --- | --- | --- | --- |
| `accent` (`-hover`, `-soft`, `-text`) | the one main action of an area, a link, the current step; derived from the logo | `#5B5DEA`, text `#B3B5FF`, soft `#26265C` | `#4339CC`, text `#4339CC`, soft `#E9E8FC` |
| `agent` (`-hover`, `-soft`, `-text`, `-border`) | what the agent does or proposes: its name, its skill, its tool, the remote-edit outline (D-030) | `#3DDCC8`, text `#7CEBDB`, soft `#0D2826` | `#0F6F67`, text `#0F6F67`, soft `#D8F3EE` |
| `success` (`-soft`, `-text`) | done, ready, connected | `#35A85F`, text `#8BE3A6`, soft `#10291B` | `#166A38`, text `#166A38`, soft `#DDF5E4` |
| `warning` (`-soft`, `-text`) | waits for a person, a warning | `#E3A32B`, text `#F6CC7C`, soft `#2E2410` | `#855700`, text `#855700`, soft `#FBEFD2` |
| `danger` (`-hover`, `-soft`, `-text`) | an error, a refusal, a destructive action | `#C9303E`, text `#FFA8A8`, soft `#351A1F` | `#B12532`, text `#B12532`, soft `#FBE3E5` |

Teal is the agent's. Amber is "waits for you". The solid `danger` is a button fill with `--color-text-on-danger`; as text or as a border use `-text`. `--color-agent` and `--color-success` and `--color-warning` can also be used as text on the page (4.5:1, section 2.6).

**Decision of this document.** `--agent` attributes every label and chip of another actor and the 2 px remote-edit outline on timeline items (D-030, SDD-012 §5.8 item 4 and §17), so a remote edit is not mistaken for a selection, which keeps `--ed-selection`.

**Calendar themes (epic 97, SDD-020 section 30A.1).** A day of the publishing calendar may carry a theme, one of six names the service keeps: `blue`, `green`, `amber`, `red`, `teal` and `gray`. A theme adds no token: it is a family of this section drawn as its `-text` on its `-soft` surface, the pairs that section 2.6 and `check:contrast` already measure (`blue` is `accent`, `green` is `success`, `amber` is `warning`, `red` is `danger`, `teal` is `agent`, and `gray` is `--color-text` on `--color-surface-sunken`). The colour never stands alone: the theme is named in words where it is chosen and in the header of the day view, and the quiet mark is an icon with the words "No posts". `scripts/calendar-themes.test.ts` fails if a theme is drawn in a pair that the check does not measure.

### 2.5 Colour: editor (`--ed-*`)

Names are those of SDD-012 §17. The chrome of the editor follows the semantic tokens; the colours of the item types stay their own, because a clip, a sound, a text, a caption and a shape must differ from each other and from the state colours.

| Token | Dark | Light | Use |
| --- | --- | --- | --- |
| `--ed-bg` | `--color-background` | `--color-background` | editor surround |
| `--ed-panel` | `--color-surface` | `--color-surface` | library, inspector, top bar, timeline toolbar |
| `--ed-timeline` | `--color-surface-sunken` | `--color-surface-sunken` | timeline background |
| `--ed-track-line` | `--color-border-strong` | `--color-border-strong` | row separators, ruler ticks, the 1 px border of item blocks (3:1) |
| `--ed-item-clip`, `-fg` | `oklch(0.500 0.180 259.8)`, white | `oklch(0.546 0.215 262.9)`, white | clip blocks |
| `--ed-item-audio`, `-fg` | `oklch(0.620 0.170 149.6)`, `oklch(0.122 0.006 261.6)` | `oklch(0.527 0.137 150.1)`, white | audio blocks |
| `--ed-item-text`, `-fg` | `oklch(0.500 0.200 303.9)`, white | `oklch(0.496 0.237 301.9)`, white | text blocks |
| `--ed-item-caption`, `-fg` | `oklch(0.500 0.190 354.3)`, white | `oklch(0.525 0.199 4.0)`, white | caption blocks |
| `--ed-item-shape`, `-fg` | `oklch(0.769 0.165 70.1)`, `oklch(0.122 0.006 261.6)` | `oklch(0.553 0.174 38.4)`, white | shape blocks |
| `--ed-playhead` | white | `oklch(0.210 0.032 264.7)` | playhead line and handle |
| `--ed-marker`, `--ed-snap` | `--color-focus` | `--color-focus` | markers and the snap line |
| `--ed-selection` | white | white | selection ring on an item; the remote-edit outline uses `--agent` (D-030) |
| `--ed-ghost` | focus at 55% | focus at 75% | ghost outline and fill of a pending gesture |
| `--ed-safe-area`, `--ed-handle` | `--color-focus` | `--color-focus` | safe-area guide, selection box and handles, focus ring over the preview surround |
| `--ed-error-flash` | `oklch(0.645 0.215 16.4)` | `oklch(0.586 0.222 17.6)` | outline of the item named by an overlap or conflict |
| `--ed-preview-surround` | `oklch(0.174 0.010 276.4)` (`#0F1015`) | `oklch(0.928 0.014 93.0)` (`#EAE7DD`) | the preview region around the canvas: the canvas colour of the redesign, dark in the dark theme and a warm grey in the light one |

Text placed on `--ed-preview-surround` (banners inside the preview, the canvas label, the transport row) uses `--color-text` and `--color-text-muted`, which pass on the canvas colour in both themes (section 2.6).

### 2.6 Contrast ratios

Method: OKLCH to linear sRGB (Björn Ottosson's matrices), gamut clamp, sRGB encoding, WCAG 2.x relative luminance; a colour with alpha is composited over the surface below it in sRGB, as the browser paints it. This is the algorithm the CI script `bun run check:contrast` of SDD-012 §18.4 implements; the numbers below are what that script must reproduce within 0.02. Thresholds: 4.5:1 text, 3:1 large text, UI components and graphical objects.

The pairs cover: the three text tokens on the six surfaces; the focus ring and the strong border at 3:1; each family as text on the page, on a card and on its soft surface, and its fill with its text; the former names (aliases) that unmigrated pages still use; and the editor. The "on its tint" rows are the older pattern `bg-<state>/10` in light and `/20` in dark with the state as text, kept while a page still draws it.

| Pair (foreground on background) | Need | Light | Dark |
| --- | --- | --- | --- |
| `color-text` on `color-background` | 4.5 | 16.03 | 15.72 |
| `color-text-muted` on `color-background` | 4.5 | 8.22 | 9.87 |
| `color-text-subtle` on `color-background` | 4.5 | 5.75 | 6.78 |
| `color-text` on `color-surface` | 4.5 | 16.88 | 14.78 |
| `color-text-muted` on `color-surface` | 4.5 | 8.66 | 9.28 |
| `color-text-subtle` on `color-surface` | 4.5 | 6.05 | 6.38 |
| `color-text` on `color-surface-raised` | 4.5 | 16.88 | 13.48 |
| `color-text-muted` on `color-surface-raised` | 4.5 | 8.66 | 8.46 |
| `color-text-subtle` on `color-surface-raised` | 4.5 | 6.05 | 5.81 |
| `color-text` on `color-surface-sunken` | 4.5 | 14.95 | 16.56 |
| `color-text-muted` on `color-surface-sunken` | 4.5 | 7.67 | 10.4 |
| `color-text-subtle` on `color-surface-sunken` | 4.5 | 5.36 | 7.14 |
| `color-text` on `color-surface-hover` | 4.5 | 13.66 | 12.08 |
| `color-text-muted` on `color-surface-hover` | 4.5 | 7.01 | 7.58 |
| `color-text-subtle` on `color-surface-hover` | 4.5 | 4.9 | 5.21 |
| `color-text` on `color-surface-selected` | 4.5 | 14.01 | 14.08 |
| `color-text-muted` on `color-surface-selected` | 4.5 | 7.19 | 8.84 |
| `color-text-subtle` on `color-surface-selected` | 4.5 | 5.02 | 6.07 |
| `color-focus` on `color-background` | 3 | 7.4 | 9.44 |
| `color-focus` on `color-surface` | 3 | 7.79 | 8.87 |
| `color-focus` on `color-surface-raised` | 3 | 7.79 | 8.09 |
| `color-focus` on `color-surface-sunken` | 3 | 6.9 | 9.94 |
| `color-focus` on `color-surface-selected` | 3 | 6.47 | 8.45 |
| `color-border-strong` on `color-background` | 3 | 3.71 | 4.23 |
| `color-border-strong` on `color-surface` | 3 | 3.91 | 3.98 |
| `color-border-strong` on `color-surface-raised` | 3 | 3.91 | 3.63 |
| `color-border-strong` on `color-surface-sunken` | 3 | 3.46 | 4.46 |
| `color-accent-text` on `color-accent-soft` | 4.5 | 6.47 | 7.26 |
| `color-accent-text` on `color-background` | 4.5 | 7.4 | 9.44 |
| `color-accent-text` on `color-surface` | 4.5 | 7.79 | 8.87 |
| `color-agent-text` on `color-agent-soft` | 4.5 | 5.15 | 10.96 |
| `color-agent-text` on `color-background` | 4.5 | 5.72 | 12.7 |
| `color-agent-text` on `color-surface` | 4.5 | 6.02 | 11.93 |
| `color-success-text` on `color-success-soft` | 4.5 | 5.8 | 10.06 |
| `color-success-text` on `color-background` | 4.5 | 6.34 | 11.71 |
| `color-success-text` on `color-surface` | 4.5 | 6.67 | 11.01 |
| `color-warning-text` on `color-warning-soft` | 4.5 | 5.47 | 10.06 |
| `color-warning-text` on `color-background` | 4.5 | 5.94 | 11.91 |
| `color-warning-text` on `color-surface` | 4.5 | 6.26 | 11.19 |
| `color-danger-text` on `color-danger-soft` | 4.5 | 5.43 | 8.67 |
| `color-danger-text` on `color-background` | 4.5 | 6.28 | 9.82 |
| `color-danger-text` on `color-surface` | 4.5 | 6.61 | 9.23 |
| `color-text-on-accent` on `color-accent` | 4.5 | 7.79 | 5.01 |
| `color-text-on-agent` on `color-agent` | 4.5 | 6.02 | 11.12 |
| `color-text-on-danger` on `color-danger` | 4.5 | 6.61 | 5.28 |
| `color-text-on-accent` on `color-accent-hover` | 4.5 | 8.95 | 6.28 |
| `color-text-on-danger` on `color-danger-hover` | 4.5 | 5.28 | 6.61 |
| `color-success` on `color-background` | 4.5 | 6.34 | 5.95 |
| `color-success` on `color-surface` | 4.5 | 6.67 | 5.59 |
| `color-warning` on `color-background` | 4.5 | 5.94 | 8.19 |
| `color-warning` on `color-surface` | 4.5 | 6.26 | 7.7 |
| `color-agent` on `color-background` | 4.5 | 5.72 | 10.56 |
| `color-agent` on `color-surface` | 4.5 | 6.02 | 9.92 |
| `muted-foreground` on `muted` | 4.5 | 7.67 | 10.4 |
| `primary-foreground` on `primary` | 4.5 | 7.79 | 5.01 |
| `secondary-foreground` on `secondary` | 4.5 | 16.88 | 13.48 |
| `destructive` on `background` | 4.5 | 6.28 | 9.82 |
| destructive on its tint (`destructive` at 10% in light, 20% in dark, over `background`) | 4.5 | 5.34 | 6.42 |
| destructive on its tint over popover (`destructive` at 10% in light, 20% in dark, over `popover`) | 4.5 | 5.61 | 5.41 |
| `info` on `background` | 4.5 | 7.4 | 9.44 |
| agent on its tint over ed-panel (`color-agent` at 10% in light, 20% in dark, over `ed-panel`) | 4.5 | 5.22 | 6.33 |
| `color-agent` on `ed-panel` | 4.5 | 6.02 | 9.92 |
| `brand-foreground` on `brand` | 4.5 | 8.28 | 5.66 |
| `brand-text` on `background` | 4.5 | 8 | 9.23 |
| `brand-text` on `brand-subtle` over `background` (the active sidebar item) | 4.5 | 7.67 | 8.66 |
| `ring` on `background` | 3 | 7.4 | 9.44 |
| `ring` on `ed-timeline` | 3 | 6.9 | 9.94 |
| `input` on `background` | 3 | 3.71 | 4.23 |
| `ed-track-line` on `ed-timeline` | 3 | 3.46 | 4.46 |
| `ed-playhead` on `ed-timeline` | 3 | 15.71 | 19.01 |
| `ed-snap` on `ed-timeline` | 3 | 6.9 | 9.94 |
| `ed-ghost` on `ed-timeline` | 3 | 4.08 | 3.74 |
| `ed-error-flash` on `ed-timeline` | 3 | 4.16 | 5.18 |
| `ed-safe-area` on `ed-preview-surround` | 3 | 6.31 | 9.94 |
| `ed-handle` on `ed-preview-surround` | 3 | 6.31 | 9.94 |
| `ed-item-clip-fg` on `ed-item-clip` | 4.5 | 5.17 | 6.18 |
| `ed-item-audio-fg` on `ed-item-audio` | 4.5 | 5.02 | 5.98 |
| `ed-item-text-fg` on `ed-item-text` | 4.5 | 6.97 | 6.66 |
| `ed-item-caption-fg` on `ed-item-caption` | 4.5 | 6.03 | 6.69 |
| `ed-item-shape-fg` on `ed-item-shape` | 4.5 | 5.19 | 9.45 |
| `ed-selection` on `ed-item-clip` | 3 | 5.17 | 6.18 |
| `ed-selection` on `ed-item-text` | 3 | 6.97 | 6.66 |

Item fills against `--ed-timeline` are below 3:1; this is why every item block has a 1 px `--ed-track-line` border (3.46:1 in light, 4.46:1 in dark against the timeline), which carries the boundary of the block (section 4.3.6).

### 2.7 Typography

**UI typeface.** Inter Variable (SIL OFL 1.1, `@fontsource-variable/inter` 5.3.0, approved by D-138), bundled and served from the same origin with the bundle (P8; the default policy `default-src 'self'` of SDD-012 §2.2 allows nothing else). It is the same in the browser and in the desktop webview because it is bundled, not a system font. It replaces Geist Variable.

| Token (Tailwind) | Value |
| --- | --- |
| `--font-sans` (`font-sans`) | `'Inter Variable', ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif` |
| `--font-heading` | `var(--font-sans)` |
| `--font-mono` (`font-mono`) | the system stack of Tailwind 4: `ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace` |

**Fonts of the video are not UI fonts.** The six families of D-022 (Inter, Montserrat, Anton, Bebas Neue, Playfair Display, JetBrains Mono) are bundled in the Go binary for rasterizing text into the picture (ADR-0012). The browser never loads them for the picture and never lays out video text (SDD-012 rule T6): the inspector's font select lists the family names in the UI font, with no rendered samples (D-030).

**Timecodes and numbers.** Every time, duration, revision number, frame count, percentage and size uses `tabular-nums` so digits do not jump while the playhead moves. Timecodes are set in `font-sans` with tabular figures, not in a monospace font. `font-mono` is used only for code-like content: file paths, command names, ids, error codes, the install command, diff text from the core.

**The text scale.** One scale for the dashboard, written in `app.css` under `html[data-text-scale="dashboard"]`, at every width. `DashboardLayout` sets the attribute on `<html>`, so dialogs and menus portaled to the body follow. The editors keep the Tailwind default, where `text-xs` is 12 px, their minimum.

| Style | Utility | Size / line height | Weight | Where |
| --- | --- | --- | --- | --- |
| h1 | `text-3xl` | 30 / 38 | 600 | the title of a page (`PageHeader`) |
| h2 | `text-2xl` | 24 / 32 | 600 | the title of a block (`SectionTitle`) |
| h3 | `text-lg` | 18 / 26 | 600 | the title of a card |
| Body large | `text-lg` | 18 / 28 | 400 | an opening sentence |
| Body | `text-base` | 16 / 24 | 400 | text |
| Small, label, mono | `text-sm`, `text-xs` | 14 / 20 | 400 or 500 | the smallest size of the dashboard: `text-xs` resolves to 14 px there |
| Editor small | `text-xs` | 12 / 16 | 400 or 500 | the video and image editors only |
| Display | `text-4xl` to `text-6xl` | 38 / 46, 52 / 60, 68 / 72 | 600 | numbers and the landing |

Rules: 14 px is the minimum on the dashboard and 12 px in the editors; weights are 400, 500 and 600 (Inter Regular, Medium or SemiBold, Bold for display only); no italics in UI chrome; text never sits on a picture without a solid surface behind it. This scale replaces the 1.5 times scale of D-123.

### 2.8 Spacing

The Tailwind 4 spacing unit stays `--spacing: 0.25rem` (4 px), the base of the redesign. Only these steps are used, so the grid stays regular:

| Step | px | Typical use |
| --- | --- | --- |
| `0.5` | 2 | gap between a ring and its element |
| `1` | 4 | gap inside a toolbar group, icon to label inside a badge |
| `1.5` | 6 | icon to label inside a button |
| `2` | 8 | gap between controls, field rows |
| `3` | 12 | padding of a field, of a row, of an editor panel |
| `4` | 16 | gap between sections, card padding |
| `6` | 24 | page padding, dialog padding |
| `8` | 32 | gap between page sections |
| `12` | 48 | empty-state vertical padding |

### 2.9 Radii

| Token (Tailwind) | Value | Use |
| --- | --- | --- |
| `--radius-xs` (`rounded-xs`) | 4 px | timeline item blocks, caption word segments, keyframe diamonds |
| `--radius-sm` | 6 px | small chips, thumbnails |
| `--radius-md` | 8 px | buttons, fields, tooltips, menu items |
| `--radius-lg` | 12 px | cards, popovers, menus |
| `--radius-xl` | 16 px | dialogs and sheets |
| `--radius-2xl` | 24 px | large framed areas |
| `rounded-full` | 9999 px | badges, pills, switches |
| none | 0 | the preview canvas, the panels of the editor shell (they meet edge to edge) |

`--radius-3xl` (32 px) and `--radius-4xl` (40 px) exist for the old classes that name them; new code uses the five steps above. `--radius` (12 px) is kept as a plain property for code that reads it.

### 2.10 Borders, rings and outlines

| Element | Width and style | Colour | Note |
| --- | --- | --- | --- |
| Region divider, card edge | 1 px solid | `--color-border` | decorative |
| Field boundary, checkbox, unchecked switch | 1 px solid | `--color-border-strong` | identifies the control, 3:1 |
| Invalid field | 1 px solid | `--color-danger-text` | plus the icon and the message under the field |
| Focus indicator (everything focusable) | `outline: 2px solid var(--color-focus); outline-offset: 2px` | `--color-focus` | 3:1 against every surface in both themes |
| Focus over the preview surround | same | `--ed-handle` | |
| Focused timeline cell | `outline: 2px solid; outline-offset: 1px` | `--ed-selection` | offset 1 px so it does not overlap the next row |
| Selected item | 2 px inset ring (`box-shadow: inset 0 0 0 2px`) | `--ed-selection` | selected and focused can show together: ring inside, outline outside |
| Item block edge | 1 px solid | `--ed-track-line` | carries the boundary of the block |
| Pending ghost | 2 px dashed | `--ed-ghost` | fill `--ed-ghost` at 15% |
| Remote-edit highlight | 2 px solid, 1.5 s | `--color-agent` | SDD-012 §5.8 item 4 and D-030; class toggle, not an animation |
| Conflict or overlap highlight | 2 px solid, 2 s | `--ed-error-flash` | SDD-012 §8.4 |
| Snap line | 1 px solid, full height | `--ed-snap` | |
| Safe-area guide | 1 px dashed (6 px dash, 4 px gap) | `--ed-safe-area` | band outside it: black at 40% |

**The focus rule lives in `app.css`, and components draw no halo of their own.** The primitives carry `outline-none`; one **unlayered** rule (outside every `@layer`) wins over Tailwind's utilities, so no component can weaken it and none needs a ring class:

```css
:where(a, button, input, select, textarea, summary, [tabindex], [contenteditable="true"], [data-slot]):focus-visible {
	outline: 2px solid var(--color-focus);
	outline-offset: 2px;
}
```

The rule above is the contract; narrow the selector, never weaken the outline. A field also changes its border to `--color-focus` on focus, which is a second cue, not the only one.

### 2.11 Elevation

The editor is flat: regions meet edge to edge, separated by `--color-border`. Elevation exists only for things that float over content, with three shadows per theme (the shadow colour is a token of the theme, `--color-shadow-sm`, `-md`, `-lg`).

| Level | What | Treatment |
| --- | --- | --- |
| 0 | page, panels, timeline, cards on a page | no shadow; a `--color-border` edge |
| 1 | tooltip, popover, menu, select list, toast | `shadow-md` plus a 1 px `--color-border`, on `--color-surface-raised` |
| 2 | dialog, sheet, command palette | `shadow-lg` plus a 1 px `--color-border`; the backdrop is the text colour at 30% in light and the sunken surface at 80% in dark |

| Utility | Value |
| --- | --- |
| `shadow-sm` | `0 1px 2px` in `--color-shadow-sm` |
| `shadow-md` | `0 4px 12px` in `--color-shadow-md` |
| `shadow-lg` | `0 16px 40px` in `--color-shadow-lg` |

In dark, "higher" also means a lighter surface (`surface`, then `surface-raised`).

### 2.12 Z-index layers

Global layers (Tailwind `z-*`):

| Layer | Value | What |
| --- | --- | --- |
| base | `z-0` | page and editor regions |
| splitters | `z-10` | `ui/Splitter.tsx` hit areas over region edges |
| sticky page chrome | `z-20` | the sidebar rail (kept value) |
| in-page banners | `z-30` | offline, unsupported, read-only banners pinned to the top of a region |
| shell panels | `z-40` | the agent chat panel over the video editor (`EditorSceneBar`, VV-427): drawn in `document.body`, outside the stacking contexts of the editor regions, over every region and under the portals below, so a dialog or a menu opened from it still covers it |
| portals | `z-50` | Base UI and UI SDK popups: tooltip, popover, menu, select, dialog, sheet, palette (the `z-50` value is kept) |
| toasts | `z-[60]` | the toast viewport, so a toast raised while a dialog is open is visible |

Inside `#timeline`, which is its own stacking context (`isolation: isolate`):

| Order | Element |
| --- | --- |
| 0 | rows (`.tl-row`) |
| 1 | items (`.tl-item`) |
| 2 | the selected and the focused item |
| 3 | ghost layer (`.tl-ghost-layer`) |
| 4 | snap line |
| 5 | playhead |
| 6 | sticky ruler |
| 7 | sticky track headers (the playhead passes under them when scrolled) |
| 8 | top-left corner cell |

Inside the preview box: canvas 0, truth-on-pause `<img>` 1, safe-area guide 2, selection box and handles 3, inline text editor 4, preview banners 5.

### 2.13 Motion

The motion tokens are the one set of values for every transition and animation of the product, including what the shared primitives carry.

| Token | Value | Use |
| --- | --- | --- |
| `--duration-none` | `0ms` | every editor component |
| `--duration-fast` | `120ms` | hover and press, the close of a popup |
| `--duration-base` | `200ms` | state changes, the open of a popup, a bare Tailwind `transition` (`--default-transition-duration`) |
| `--duration-slow` | `320ms` | a highlight outline, a sheet |
| `--duration-reduced` | `1ms` | what the three above become under `prefers-reduced-motion` |
| `--ease-standard` | `cubic-bezier(0.2, 0, 0, 1)` | the default easing; `--ease-enter` and `--ease-exit` for entrances and exits |

Rules:

1. A component takes its duration from a token (`duration-(--duration-fast)`), never from a literal time. Editor components add no `transition`, `animation` or `@keyframes` of their own: motion comes from `.ed-motion` (colour, border, shadow, opacity and outline colour fade in `--duration-base`; position and size never transition, so a drag, a zoom or a row resize never lags the pointer) and `.ed-highlight` (the outline of a remote edit or a conflict eases in over `--duration-slow`). A unit test (`web/scripts/motion.test.ts`) fails on any other `transition`, `animate-` or `duration-` class in an editor component.
2. No spinner rotates. A running job shows a determinate bar; an unknown duration shows a static skeleton and a sentence (`LoadingState`, section 4.4).
3. Scrolling is never smooth: page-follow of the playhead jumps (SDD-012 §7.5), `scroll-behavior` stays `auto`.
4. Under `prefers-reduced-motion: reduce` every transition and animation lasts 1 ms and the three duration tokens equal `--duration-reduced`. It is not 0: popup presence waits for `animationend`, which an animation of exactly `0s` does not fire in every engine.

```css
@media (prefers-reduced-motion: reduce) {
	:root {
		--duration-fast: var(--duration-reduced);
		--duration-base: var(--duration-reduced);
		--duration-slow: var(--duration-reduced);
	}
	*, ::before, ::after {
		animation-duration: var(--duration-reduced) !important;
		animation-iteration-count: 1 !important;
		transition-duration: var(--duration-reduced) !important;
		transition-delay: 0s !important;
	}
}
```

### 2.14 What `app.css` holds

For the implementer, in one place.

1. `@theme inline`: the semantic names mapped to themselves, the aliases of the former system, the radii, the three shadows, the default transition.
2. `:root` and `.dark`: the values of both themes; the aliases in one rule for both.
3. The base layer: the text scale of the dashboard and its headings, the `outline-focus` default, the native select options.
4. Unlayered: the focus rule (section 2.10), the 24 px target of native checkboxes and radios (section 7.8), `.ed-motion`, `.ed-highlight`, the reduced-motion rule (section 2.13) and the forced-colours rule (section 7.7).
5. A custom variant, `dashboard`, so a control that is compact in the editors takes 32 px on the dashboard (`md:dashboard:h-8`).
6. After a change of a value, run `bun run check:contrast` and keep the numbers of section 2.6.

## 3. Layout

### 3.1 Editor regions

The editor shell is the CSS grid of SDD-012 §6.1: rows `48px 1fr var(--timeline-h)`, columns `var(--library-w) var(--history-w) 1fr var(--inspector-w)`; the top bar and the timeline span all columns.

| Region | Landmark | Surface | Contents | Size |
| --- | --- | --- | --- | --- |
| Top bar | `header` | `--ed-panel`, bottom `--border` | back, project name, revision and connection state, Undo and Redo, activity indicator, palette button, theme, language, jobs chip, Export (SDD-012 §6.2) | 48 px high, padding `px-3`, groups separated by `gap-2`, the primary action (Export) at the far right |
| Library | `aside` with `aria-label` | `--ed-panel`, right `--border` | tab rail, import bar, asset cards, trash group | default 640 or 480 px (section 3.2), so that the preview keeps 360 px; collapsible to its rail (`Alt+1`) |
| History sidebar | `aside` named "History sidebar" | `--ed-panel`, left `--border` | its button on a 40 px rail, then the history rows, newest on top (section 4.3.16) | 40 px collapsed, 320 px open; below 1280 px it and the library share the room, at least 192 px each; collapsible (`Alt+3`); open state remembered |
| Preview | inside `main` | `--ed-preview-surround` | canvas box (16 px padding), overlays, transport row (40 px) | the remaining width; minimum 240 px high |
| Inspector | `aside` with `aria-label` | `--ed-panel`, left `--border` | panel for the selection (SDD-012 §12.1) | default 360 or 280 px; minimum 200 px; collapsible (`Alt+2`) |
| Timeline | inside `main` | toolbar `--ed-panel` (40 px), body `--ed-timeline` | toolbar, ruler (28 px), track headers (176 px), rows | default 300 or 240 px high; minimum 160 px; resized by its splitter |
| Status | inside the top bar | | revision, connection state and jobs live in the top bar; there is no separate status bar, so the timeline keeps the height | |

**Decision of this document.** There is no bottom status bar: SDD-012 §6.1 puts revision, connection and jobs in the top bar, and a 24 px status row would take height from the timeline, the scarcest dimension at 1280 by 720.

Splitters: 1 px visible line (`--border`), 8 px hit area, `role="separator"`, focusable, arrow keys resize by 16 px; the focus outline follows section 2.10. Each panel scrolls on its own; the page never scrolls (SDD-012 §6.3).

### 3.2 Breakpoints and minimum sizes

Web, editor (from SDD-012 §6.3):

| Viewport width | Library | Inspector | Timeline height | Behaviour |
| --- | --- | --- | --- | --- |
| 1600 px and wider | 640 px | 360 px | 300 px | supported |
| 1280 to 1599 px | 480 px | 280 px | 240 px | supported; the minimum supported size is 1280 by 720 |
| 1024 to 1279 px | `Sheet` drawer from a top-bar button | `Sheet` drawer, opens on selection | 240 px | tolerated: a notice "best used at 1280 pixels or wider" once per session (`--warning` tone, dismissible) |
| below 1024 px | | | | not supported: a full-screen empty state with a link back to the projects list |

Height below 600 px is allowed: regions get smaller and panels scroll.

Web, Inertia pages (projects, settings, login, error): responsive from 360 px wide. Below 768 px (`MOBILE_BREAKPOINT` of `hooks/use-mobile.ts`) the sidebar becomes a sheet; content is one column. The project grid uses `repeat(auto-fill, minmax(240px, 1fr))`.

Desktop window (ADR-0023):

| Property | Value | Why |
| --- | --- | --- |
| Default inner size | 1600 by 900, or maximized when the screen is smaller | the first supported breakpoint with room for the panels |
| Minimum inner size | 1280 by 640 | 1280 is the supported minimum width; 640 fits a 1366 by 768 laptop after the taskbar and title bar, and SDD-012 §6.3 allows heights down to 600 |
| Remembered | position and size, by the shell | |

**Decision of this document.** The desktop window's minimum inner size is 1280 by 640. Neither ADR-0023 nor SDD-012 §21 states one; the shell enforces it, so the tolerated and unsupported widths of the web never occur in the desktop window.

### 3.3 Density

The editor is a compact tool; the Inertia pages use shadcn's comfortable defaults.

| Element | Editor | Inertia pages |
| --- | --- | --- |
| Control height | 32 px (`h-8`, shadcn default); 28 px (`sm`) in the timeline toolbar and track headers; 24 px (`icon-xs`) only for toggles inside track headers | 32 to 36 px (`default`, `lg`) |
| Row height | inspector field row 32 px; menu item 32 px; history row 40 px; palette row 40 px | list rows 48 px |
| Panel padding | 12 px | 16 to 24 px |
| Text | `text-sm` default, `text-xs` meta | `text-base` body |
| Timeline rows | 64 px visual tracks with thumbnails, 48 px audio, 40 px text and captions only (SDD-012 §7.4) | |

Every target is at least 24 by 24 CSS px (WCAG 2.2 target size minimum, SDD-012 §18.1 item 7); timeline handles are 8 px wide visually with a 24 px hit area.

### 3.4 Editor layout familiar to users of other editors

Decision D-038: a person who already edits short videos in a popular desktop editor should find each thing in the same place here. This section describes, screen by screen, the regions, the components inside them, the menus and the flows of that familiar layout, using only the tokens of section 2 and the components of section 4. It adds no token. The behaviour and the commands are those of SDD-012 §6.4 (the layout) and §6.5 (which familiar patterns are adopted now, later, or not at all); where this section and SDD-012 differ, SDD-012 wins and this section is fixed. No asset, icon, colour, name or text of another product is used: icons are lucide (section 5.1), labels come from the catalogues (section 6).

Rules that hold on every screen below:

- A control whose command does not exist in the registry is not drawn (absent, not disabled); a control whose command exists but cannot act on the current selection is `aria-disabled` with the reason in a tooltip (section 4.1).
- Lists of effects, transitions, filters, presets and fonts are read from the core, never written in the UI (P1).
- The page never scrolls; each panel body scrolls on its own (SDD-012 §6.3).
- Every composite widget (tab rail, tab strip, toolbar, menu) is one tab stop with arrow keys inside; `F6` moves between regions in the order top bar, library, preview, timeline, inspector (section 7.1).

#### 3.4.1 Screens

| Screen | Where | Regions and components | Primary action |
| --- | --- | --- | --- |
| Start | `/projects` (Inertia page) | page title; a full-width `Button` `default` `lg` "Create project"; the project grid (`repeat(auto-fill, minmax(240px, 1fr))`, section 3.2) of `Card`s: thumbnail box `aspect-video` on `--ed-preview-surround`, name (`UntrustedText`), meta line "edited 2 h ago · 00:30.000" in `text-xs tabular-nums text-muted-foreground`, and a `DropdownMenu` (Rename, Duplicate when the API has it, Delete with an `AlertDialog`) | Create project |
| Editor | `/editor/{project_id}` | top bar, library with tab rail, preview with transport row, inspector with tab strip, timeline with its toolbar (3.4.2 to 3.4.7) | Export (top bar) |
| Export | `Dialog` over the editor (section 4.3.10, SDD-012 §14) | name and destination at the top, then one group per output the engine supports (video; others only when the engine lists them), lint findings, progress | Start export |
| Shortcuts and settings | `Dialog` opened by `?` or Help > Keyboard shortcuts (SDD-012 §18.2) | the cheat sheet (grouped by context, Timeline, Player and General, when SDD-012 §6.5 row 10 brings contexts), rebind and reset per row with the conflict named inline; below, language, theme, "Replay the tour" and the version | Close (`Esc` closes it) |
| Palette | `Dialog` (section 4.3.11) | unchanged | Run |

There is no separate settings window and no second browser window: the export and every dialog stay inside the page (SDD-012 §6.5, what to avoid).

#### 3.4.2 Editor regions

```text
┌ TopBar (48) ─ [Menu▾] [<] Project name · Saved automatically: 14:32 · [Undo][Redo] · activity ······ [Jobs] [Export] ┐
├ Library ──────────────────┬ Preview ─────────────────────────────────────┬ Inspector ──────────────────────────────┤
│ rail │ Panel header (36)  │                                              │ Panel header (36): selection name        │
│ 40px │ Panel body          │          canvas box (16 px padding)          │ Tab strip (36): Video Audio Speed ...    │
│ [▣]  │ (asset cards,       │                                              │ Sections of the tab (scroll)             │
│ [♪]  │  text presets,      │                                              │  Field row: label | control | ◇ | ↺      │
│ [T]  │  caption actions,   │                                              │                                          │
│ ...  │  ...)               │ Transport (40): time · [⏮][◀][▶][▶][⏭] · ratio fit safe full │                          │
├──────┴─────────────────────┴──────────────────────────────────────────────┴──────────────────────────────────────────┤
│ Timeline toolbar (40):  [Edit tools: select blade · undo redo · split trim delete · caption freeze] ··· [Modes and zoom: snap magnet link · − slider + fit] │
│ Ruler (28) · track headers (176) · rows                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

The grid, sizes and breakpoints are those of sections 3.1 and 3.2; this layout keeps them. At 1600 px wide the library is 640 px (40 px rail and a 600 px panel), the inspector 360 px and the timeline 300 px high; at 1280 px they are 480 (40 and 440), 280 and 240 px. The history sidebar sits between the library and the player: a 40 px rail, 320 px open. The player is wider than in the reference layout on purpose: a wider player serves vertical video, and the panels are resizable (SDD-012 §6.5 row 1).

| Region | Surface | Border | Text tokens on it |
| --- | --- | --- | --- |
| Top bar | `--ed-panel` | bottom `--border` | `--foreground`, `--muted-foreground` (save time, revision) |
| Library rail and panel | `--ed-panel` | right `--border`; the rail has its own right `--border` | `--foreground`, `--muted-foreground` |
| Preview | `--ed-preview-surround` | none | `--color-text`, `--color-text-muted` |
| Transport row | `--ed-panel` | top `--border` | `--foreground`; total time `--muted-foreground` |
| Inspector | `--ed-panel` | left `--border` | `--foreground`, `--muted-foreground` (labels, units) |
| Timeline toolbar | `--ed-panel` | top and bottom `--border` | `--foreground` |
| Timeline body | `--ed-timeline` | rows `--ed-track-line` | section 4.3.6 |

#### 3.4.3 Top bar

Left to right: **Menu** (`Button` `ghost` `icon-sm`, a `Menu` icon named by `aria-label`, web only; in the desktop shell the same items are the native menu bar, section 8.3), back to projects, project name, the **revision** in a `Badge` (`History` icon, "Rev 42") and the **connection** in a `Badge` of its tone (section 4.3.3), the **save indicator** ("Saved automatically: 14:32", `text-xs tabular-nums text-muted-foreground`, a `Check` icon before it, hidden while the editor is not live), flexible space, activity indicator, palette, the **bell**, **Chat** (`Button` `outline` `sm`, the same panel as the scene bar, section 4.4) and **Export** (`Button` `default`, the one primary action of the editor), then the shortcuts, language and theme controls of SDD-012 §6.2, which stay in the bar. The bell and Chat are drawn when the page has a session of the agent (`EditorShellFrame` provides the chat and the inbox to the bar, the scene bar and the panel). Undo and Redo are in the timeline toolbar.

The menu (`DropdownMenu`, level 1 elevation) has the groups and actions of SDD-012 §6.4 Menu: File, Edit, Clip, Timeline, View, Help. Each item shows its key at the right from `SHORTCUTS`, in `text-xs text-muted-foreground`; submenus open with `Right`, close with `Left` or `Esc`. A group with no drawable item is not drawn. Context menus of a clip and of an empty part of the timeline (`ContextMenu`) repeat the Clip group plus Copy, Cut, Paste, Duplicate, Delete and Ripple delete, and stay short: no item is shown disabled only to advertise a feature.

#### 3.4.4 Library: tab rail and panel

- **Tab rail**, 40 px wide, at the left edge of the library region: `role="tablist"`, `aria-orientation="vertical"`, one `IconButton` `icon` (32 px) per tab with `aria-label` and a tooltip from `t()`, roving `tabindex` with `Up`, `Down`, `Home` and `End`. Tabs in this order, each drawn only when its command exists (SDD-012 §6.4): Media (`Film`), Audio (`AudioLines`), Text (`Type`), Captions (`Captions`), Effects (`Sparkles`), Transitions (`ArrowLeftRight`), Filters (`SlidersHorizontal`), Templates (`LayoutTemplate`). The history is not a tab: it is its own sidebar (section 3.1).
- **Selected tab:** `bg-accent`, icon `--foreground`, and a 2 px leading bar in `--ring` at the rail's left edge, with `aria-selected="true"`. The bar, not the fill, carries the state (3:1 rule for state indicators, section 7.6).
- **Panel:** `ui/Panel` (section 4.3.2) with the tab's name as the header title. Bodies:

| Tab | Body |
| --- | --- |
| Media | the import bar and asset cards (section 4.3.9); the empty state of section 4.3.15 |
| Audio | asset cards of kind audio, then ducking and beat markers for the selection |
| Text | "Add text" (`Button` `secondary`), the text presets as a list of rows, the fonts, "Add shape" |
| Captions | "Generate captions", "Import subtitles", caption styles, track style |
| Effects, Transitions, Filters | a grid of cards (two columns at 280 px, one at 200 px): name, optional thumbnail from the same frame request as the player, and an amount `Slider` for filters. With nothing selected the cards are `aria-disabled` with the reason "Select a clip first" |
| Templates | slots of the project and the template info |

`Alt+1` toggles the whole region; the collapsed region keeps the rail visible, so the tabs stay one click away.

#### 3.4.5 Preview and transport

The canvas box is unchanged (section 4.3.7). The transport row (40 px) is laid out in three groups: at the left the timecode `current / total`; in the centre go to start, step back, Play and Pause (`icon-lg`), step forward, go to end; at the right the aspect-ratio `Select` `sm` (the size presets of the core), Fit, the Safe area `Toggle` and Full screen. In full screen the canvas fills the screen on `--ed-preview-surround` and `Esc` leaves. Controls drawn over the surround use `--ed-handle` for focus (section 2.10).

#### 3.4.6 Inspector: tab strip and property rows

- **With no selection:** the project panel (name, size preset, frame rate, duration, the template info), no tab strip.
- **With a selection:** the panel header names the selection; under it a **tab strip** of 36 px (`Tabs`, horizontal, automatic activation) with the tabs of SDD-012 §6.4 (Video, Audio, Speed, Animation, Adjust, Text), each drawn only when the selection has its sections; a multi-selection shows the tabs the items share. The selected tab has `--foreground` text and a 2 px bottom bar in `--ring`; the others `--muted-foreground`. When the labels do not fit (pt-BR runs longer, section 6.5) the strip scrolls horizontally inside itself with the focused tab kept in view; labels never truncate.
- **Property row:** the field of section 4.3.8 followed by the keyframe diamond (`aria-pressed`) and a **reset** `IconButton` `icon-xs` (`RotateCcw`, `aria-label` "Reset `<property>`"), present only where the core documents a clearing value and `aria-disabled` at the default (SDD-012 §6.5 row 4). Each pair keeps the 24 px target size and 24 px between centres.
- Sections inside a tab stack and scroll; a section header is collapsible (section 4.3.8).

#### 3.4.7 Timeline toolbar and tracks

The toolbar (40 px, `ui/Toolbar`) has two labelled groups (`role="group"`, SDD-012 §6.5 row 5): **Edit tools** at the left (Select and Blade as a `ToggleGroup`, Undo, Redo, Split, Trim start, Trim end, Delete, Add caption, Freeze frame when it exists) and **Modes and zoom** at the right (Snap, Magnet and Link as `Toggle`s, then zoom out, the slider, zoom in, Fit). Text buttons keep a visible label and collapse to icons only when the toolbar is narrow (section 4.3.1): Trim start, Trim end, Freeze frame, Add caption and the modes keep it from 1440 px (the width the final design is drawn at), the others from 1536 px; every tooltip names the current key ("Split (S)"). A pressed toggle shows `bg-accent`, a border of at least 3:1 against `--ed-panel` and `aria-pressed="true"` (see open point 13 of section 10).

Track headers (176 px) and items follow section 4.3.6. Empty timeline: the empty state of section 4.3.15 inside the rows area, with the toolbar still visible.

#### 3.4.8 Flows

| Flow | Steps on screen |
| --- | --- |
| First edit | Start > Create project (size preset) > editor opens on the Media tab with the library empty state > paste a path and Import > the asset card shows Ingesting then Ready > "Add to timeline" or drag to a track > the clip is selected and the inspector opens on Video |
| Add text or captions | rail Text or Captions > "Add text" or "Generate captions" > the new item is selected > inspector Text tab |
| Apply a look | select a clip > rail Effects, Transitions or Filters > a card > the inspector shows the new section, with reset and keyframe on its rows |
| Cut at the playhead | place the playhead (ruler, arrows) > Split in the toolbar, the Clip menu or `S` > a short notice in the timeline if the core refuses |
| Export | top bar Export > the dialog checks the project > Start export > progress in the dialog and the jobs chip > Succeeded with path, size and Download |
| Find a shortcut | `?` or Help > Keyboard shortcuts > the context group > rebind or reset |

#### 3.4.9 Contrast and focus of this layout

Every pair below is WCAG 2.2 AA, computed with the method of section 2.6 (light / dark):

| Pair | Need | Light | Dark |
| --- | --- | --- | --- |
| `text` on `surface` (`ed-panel`: labels, tab names, menu items) | 4.5 | 16.88 | 14.78 |
| `text-muted` on `surface` (save time, units, shortcut hints, unselected inspector tabs) | 4.5 | 8.66 | 9.28 |
| `text` on `surface-hover` (highlighted menu row) | 4.5 | 13.66 | 12.08 |
| `text-muted` on `surface-hover` (shortcut hint in a highlighted menu row) | 4.5 | 7.01 | 7.58 |
| `text` on `surface-selected` (selected rail tab, pressed toggle) | 4.5 | 14.01 | 14.08 |
| `focus` on `surface` (focus outline; selected rail and tab bars) | 3 | 7.79 | 8.87 |
| `focus` on `surface-selected` (focus outline on a selected tab) | 3 | 6.47 | 8.45 |
| `focus` on `background` (focus outline crossing a region edge) | 3 | 7.40 | 9.44 |
| `text-on-accent` on `accent` (Export, Create project, Start export) | 4.5 | 7.79 | 5.01 |
| `ed-handle` on `ed-preview-surround` (focus in the preview and full screen) | 3 | 6.31 | 9.94 |

Focus is the 2 px `--color-focus` outline with 2 px offset everywhere (section 2.10); inside the preview it is `--ed-handle`, inside the timeline grid `--ed-selection`. A selected tab is shown by the bar and `aria-selected`, a pressed toggle by the fill, the border and `aria-pressed`, never by colour alone.

#### 3.4.10 Judgement calls of the first build

The comparison with the real CapCut app was made by TASK-0296 (CapCut 9.8.0.4213, walked with the window on screen); its marks, row by row, are in SDD-012 section 6.4, and this section keeps only the calls the build made, as that section lists them: the Adjust tab holds Color, Filters, Chroma key and Effects, and the Text tab holds Auto captions; several items show no inspector strip; the Transitions panel has no length field; Select and Blade are two `aria-pressed` buttons in the toolbar, with Undo and Redo moved there from the top bar; the ratio in the player row is a native `select` that sends `settings.reframe`; the toolbar and menu draw only what has an action (no Duplicate on a project card, no Zoom or Theme item in the menu); a text button collapses to its icon below 1536 px and keeps its name.

### 3.5 Dashboard sidebar

The frame of every page except the editor and the login (SDD-012 §23). It uses tokens whose contrast pairs section 2.6 already checks (`--background`, `--foreground`, `--muted`, `--muted-foreground`, `--border`) and adds none; the kept `--sidebar*` tokens stay unused.

| Part | Element | Surface | Contents | Size |
| --- | --- | --- | --- | --- |
| Sidebar | `header` (the banner) holding a `nav` with `aria-label` for the sections | `--background`, right `--border` | the product mark, the eleven sections, then Settings and Sign out at the bottom, the account name and the controls | 15 rem expanded, 3.5 rem collapsed; collapsible; below 768 px a strip above the page, always collapsed, in the same order, with no collapse button |
| Item | `a` | transparent | an icon (`size-4`, `aria-hidden`) and the label | 36 px high, `px-3`, `gap-2` |
| Main | `main` | `--background` | the page | the remaining width, scrolls on its own |
| Right dock | `div`, no landmark | `--background`, left `--border` once it has content | empty; reserved for the agent chat | 0 px until epic 14 |

States of an item: rest `--muted-foreground` text; hover `--muted` fill and `--foreground` text; current `--muted` fill, `--foreground` text, weight 500 and `aria-current="page"` (never colour alone); focus the 2 px `--ring` outline of section 2.10, inside the item so the sidebar edge does not clip it; disabled is not used, a section that is not built opens an empty state. Collapsed: the label is visually hidden, the icon stays in an item at least 36 px wide (a target above the 24 px of WCAG 2.5.8, also in the narrow strip, where the items wrap instead of scrolling), a `Tooltip` shows the label on hover and focus.

The bottom block, under a rule, holds in this order: a link to Settings, Sign out, the account name (the email in network mode, "Local" in local mode), the language select, the theme toggle and the collapse button. Their contrast pairs are `muted-foreground` on `background` and on `muted`, and `foreground` on `background`, all in section 2.6 and `check:contrast`.

**Decision of this document.** The dashboard has its own small sidebar instead of the shadcn `Sidebar` of section 4.2: the kept component stores its state in a cookie and carries a mobile sheet the dashboard does not need. The row of section 4.2 is amended: the shadcn `Sidebar` stays unused until a page needs it.

## 4. Components

### 4.1 The state model

All interactive components use the same visual states. "Disabled" never relies on opacity alone for meaning; the reason is given by a tooltip or nearby text when it is not obvious.

| State | Selector | Treatment |
| --- | --- | --- |
| default | | as the variant says |
| hover | `:hover` (pointer only; never the only cue) | one surface step (`bg-surface-hover`), or, for the primary button, `--color-accent-hover` |
| focus-visible | `:focus-visible` | 2 px `--color-focus` outline, 2 px offset (section 2.10); never removed |
| active (pressed) | `:active`, `aria-pressed="true"`, `data-state="active"` | a 1 px press offset for buttons; a toggle shows `bg-surface-selected`, a `--color-focus` border, the accent text and `aria-pressed` |
| disabled | `disabled`, `aria-disabled="true"` | 50% to 60% opacity, `pointer-events: none`; exempt from contrast; keyboard focus skips native disabled controls; controls that must explain why use `aria-disabled` with a tooltip |
| selected | `aria-selected="true"`, `aria-current` | `--ed-selection` ring for timeline objects; `bg-surface-selected` plus a 2 px leading bar in `--color-accent` for list rows (`ListRow`); `aria-current="page"` for navigation |
| error | `aria-invalid="true"` | a `--color-danger-text` border, an icon and the message under the field (`aria-describedby`) |
| busy | `aria-busy="true"` | the control keeps its value and stays enabled for reading; no spinner; a "Saving" text label appears after 400 ms only if the request is still in flight |
| read-only | `readonly`, or plain text | plain text in `text-text`, no field boundary |

### 4.2 Shared primitives

The files in `components/ui/` (SDD-012 §3.1). Dialogs, popovers, tooltips, tabs, sheets, menus, `Select` and form controls are `@base-ui/react`. This section says how the product uses them.

| Component | Variants and sizes used | States and keyboard | Product rules |
| --- | --- | --- | --- |
| `Button` | variants `default` (the accent: primary, one per view), `secondary` (raised surface and a border), `outline` (strong border), `ghost` (toolbars, icon buttons), `destructive` (a solid `--color-danger`, with `--color-text-on-danger`), `link` (inline), `brand` (section 11.6). Sizes `default` 32 px, `sm` 28 px, `xs` 24 px, `lg` 40 px, `icon` 32 px, `icon-sm` 28 px, `icon-xs` 24 px, `icon-lg` 40 px. On the dashboard the compact sizes (`sm`, `xs`, `icon-sm`, `icon-xs`) are 32 px, and at phone width (below 768 px) every size is 44 px | all of 4.1; `Enter` and `Space` activate; icon-only buttons have `aria-label` from `t()` and a `Tooltip` with the shortcut | one `default` per view; `destructive` only after a confirmation when the action cannot be undone |
| `Badge` | `default` (accent), `secondary` (raised surface, border), `destructive` (`danger-soft` and `danger-text`), `outline`, `ghost`, `link`; a state with an icon and a word is a `StatusBadge` (section 4.4) | not focusable unless it is a link | always text, optionally an icon; never colour alone |
| `Card` | default | | project cards on `/projects`, settings groups; not used inside the editor (panels are flat) |
| `Input` | default (`--color-surface-sunken` fill, `--color-border-strong` boundary, 32 px; 44 px at phone width); `font-mono` for paths | 4.1; error state through `aria-invalid` | always with a `Label` or an `aria-label`; placeholder is an example, never the label |
| `Textarea` | default | 4.1 | caption and text content; `Ctrl+Enter` commits (SDD-012 §12.3) |
| `Label` | default | | `text-xs font-medium text-text-muted` in the inspector, `text-sm` on pages |
| `Select` | default trigger `h-8`; `sm` in toolbars | UI SDK: `Space` and `Enter` open, arrows move, typing selects, `Esc` closes | used for enums (preset, fit, font family, track); options rendered with `UntrustedText` when they show user content |
| `Separator` | horizontal and vertical | decorative (`role="none"`) unless it separates toolbar groups (`role="separator"`) | |
| `Sheet` | `left` (library drawer), `right` (inspector drawer) | focus trap, `Esc` closes, focus returns to the opener | only at 1024 to 1279 px (section 3.2) and in the mobile Inertia sidebar |
| `Sidebar` | `collapsible="icon"`, 16 rem / 3 rem | `Ctrl+B` toggles (kept) | kept in `components/ui/` but unused; the dashboard sidebar of section 3.5 is its own component |
| `Skeleton` | default | `aria-hidden`; the region has `aria-busy` | first load of the project list and of the document; never longer than the request; static, no pulse and no shimmer (`LoadingState`, section 4.4) |
| `Tabs` | default | Base UI: arrows move, `Home` and `End`, automatic activation | the vertical tab rail of the library and the tab strip of the inspector (section 3.4, SDD-012 §6.4); inside one inspector tab the sections stack and scroll, with no further level of tabs |
| `Tooltip` | default | opens on hover and on focus, `Esc` closes | every icon-only button; text is the label plus the shortcut in `<kbd>` style (`Split (S)`); never the only place of essential information |

These primitives live in `components/ui/`. Dialogs, popovers, sliders, switches, checkboxes, radio groups, progress, toggles, sheets and menus are `@base-ui/react`. Each is listed in SDD-012 §3.1.

| Component | Used by |
| --- | --- |
| `Dialog`, `AlertDialog` | caption dialog, export dialog, install dialog, `E_IN_USE` and `E_NOT_EMPTY` confirmations, command palette |
| `Popover` | activity popover, jobs list, colour field alpha |
| `DropdownMenu`, `ContextMenu` | asset card menu, track header menu, item and gap context menus |
| `Slider` | opacity, volume, colour adjustments (commit on release) |
| `Switch`, `Checkbox`, `RadioGroup` | inspector toggles, schema form booleans and `oneOf` |
| `Progress` | ingest, transcription, export, install |
| `Toggle`, `ToggleGroup` | Snap, Safe area, track hide and mute |
| `ScrollArea` | not used: native scroll keeps the webview floor simple (SDD-012 §21.1) |

### 4.3 Editor components

Names are the files of SDD-012 §3.2. Each entry gives anatomy, variants, states, size and keyboard. States not listed follow section 4.1.

#### 4.3.1 `ui/Toolbar` and `ui/IconButton`

- Anatomy: a `role="toolbar"` row with `aria-label`; groups separated by a vertical `Separator`; `IconButton` is `Button` `ghost` `icon` (32 px) or `icon-sm` (28 px) with icon, `aria-label` and tooltip.
- Keyboard: one tab stop for the toolbar; `Left` and `Right` move between controls (roving `tabindex`), `Home` and `End` jump to the ends.
- Overflow: when the timeline toolbar is narrower than its content, text buttons collapse to icon-only (same `aria-label`), never wrap.

#### 4.3.2 `ui/Panel`

- Anatomy: header (36 px: title `text-sm font-semibold`, optional actions at the right), body (scrolls, padding 12 px).
- Variants: `library`, `inspector`, `history`; collapsed state hides the body and keeps a 32 px reopen button in the top bar.
- Keyboard: `Alt+1`, `Alt+2`, `Alt+3` toggle; `F6` moves between regions.

#### 4.3.3 `layout/TopBar`

- Anatomy, left to right: back link (`ArrowLeft` icon button to `/projects`), project name (`UntrustedText`, `text-sm font-medium`, truncates with a tooltip; inline edit), revision and connection state, Undo and Redo, activity indicator, flexible space, palette button, theme, language, jobs chip, Export (`Button` `default`).
- Connection state (text and icon, never colour alone): `live` (`Wifi` icon in a `success-soft` badge, "Live"; the revision is its own badge, "Rev 42"), `reconnecting` (`RefreshCw` icon, `--warning`, "Reconnecting"), `offline` (`--destructive`, "Offline", the edit controls become `aria-disabled`), `resyncing` (`--info`, "Syncing").
- Undo and Redo: `IconButton`; disabled after `E_NOTHING_TO_UNDO` or `E_NOTHING_TO_REDO` until the next batch of this actor (SDD-012 §15).

#### 4.3.4 `layout/ActivityIndicator` (agent activity badge)

- Anatomy: a button containing a 8 px dot (`--agent`, static), the actor label (`UntrustedText`) and a text: "claude-code is editing" while active (3 s), then "Last edit by claude-code, 12 s ago" (relative time, updated every 10 s), cleared after 5 min (SDD-012 §5.8).
- Variants: `active` (dot filled, text `text-agent`), `settled` (dot outlined, text `text-muted-foreground`), `hidden` (nothing rendered, no empty space reserved beyond the button's minimum).
- Popover (level 1): first one row per active actor (actor chip, its last action, relative time), then the 20 latest entries, each with actor chip, relative time and summary; a link "Open history".
- Keyboard: `Enter` and `Space` open the popover, `Esc` closes; the live region announces remote edits at most every 2 s (SDD-012 §18.1 item 5).
- Several actors at once (D-030, SDD-012 §5.8): one badge, naming the actor of the latest remote batch and counting the other active actors ("claude-code and 1 other are editing", plural through `params.count`); the popover lists each active actor with its last action.

#### 4.3.5 Actor chip

- Anatomy: `Badge` with icon and label. "you" (the UI's own actor): `secondary`, `User` icon. Agents and keys: `bg-agent/10 text-agent`, `Bot` icon for `local:*` agents, `KeyRound` for `key:*`. Labels follow SDD-012 §5.8 item 6 (`local:claude-code` shows as `claude-code`).
- Used in: activity popover, history rows, conflict notice, "changed by" notes.

#### 4.3.6 Timeline: `Timeline`, `Ruler`, `TrackHeader`, `TrackRow`, items, `Playhead`

Timeline item (`tl-item` with `tl-clip`, `tl-audio`, `tl-text`, `tl-caption`, `tl-shape`):

| Part | Rule |
| --- | --- |
| Block | fill `--ed-item-<type>`, 1 px `--ed-track-line` border, `rounded-xs`, full row height minus 4 px (2 px top and bottom), 1 px gap visible between adjacent items is not required: the borders separate them |
| Type cue | a 12 px type icon before the label (`Film` clip, `AudioLines` audio, `Type` text, `Captions` caption, `Square` shape), so type is not colour alone |
| Label | `text-xs font-medium`, `--ed-item-<type>-fg`, padding `px-1.5 py-1`, one line, truncates; rendered with `UntrustedText`. When the clip shows a thumbnail strip the label sits on a chip of the item's fill colour so its contrast stays 4.5:1 over any picture |
| Badges | right-aligned, `text-xs tabular-nums`: speed (`1.5x`), keyframe diamonds (6 px, `-fg` colour), slot icon, missing-media icon (`TriangleAlert` in `-fg`, plus `aria-label` text) |
| Transition marks | 8 px corner triangles in `-fg` at 60% at the start or end edge |
| Audio waveform | drawn in `-fg` at full opacity (a graphical object, 3:1 against the fill); fade ramps as 1 px lines in `-fg` |
| Caption block | below 120 px wide or `ppf` under 0.5: one block with the word count ("12 words"); else one segment per word, separated by 1 px lines in `-fg` at 40% (decorative); the selected word gets a 2 px `--ed-selection` inset ring |
| Handles | 8 px wide visual edge zones, 24 px hit area, cursor `ew-resize`, visible on hover and when selected |

Item states: default; hover (border becomes `-fg` at 60%); focus-visible (2 px `--ed-selection` outline, 1 px offset); selected (2 px `--ed-selection` inset ring plus `aria-selected="true"`); ghost pending (2 px dashed `--ed-ghost`, fill at 15%); remote-changed (2 px `--agent` outline for 1.5 s, D-030); conflicting (2 px `--ed-error-flash` outline for 2 s); disabled does not exist (an item is always selectable); on a hidden track the fill is mixed 50% with `--ed-timeline` and the label keeps full `-fg` colour.

Ruler: 28 px high, `<canvas>`, ticks in `--ed-track-line`, labels `text-xs tabular-nums` in `--muted-foreground`, at least 70 px apart (SDD-012 §7.3); the `role="slider"` wrapper shows the focus outline around the whole ruler.

Playhead: 2 px line in `--ed-playhead` across ruler and rows; a 12 by 8 px downward triangle handle on the ruler; 24 px hit area on the handle.

Snap line: 1 px `--ed-snap`, full height, only during a gesture.

Track header: 176 px wide (`TRACK_HEADER_PX`), the row's height; type icon, name (`UntrustedText`, inline rename), hide toggle (visual tracks, `Eye` and `EyeOff`) and mute toggle (`Volume2` and `VolumeX`) as `Toggle` `icon-xs` with `aria-pressed`, drag handle (`GripVertical`), menu (`Ellipsis`). Selected track: `bg-accent` and a 2 px leading bar in `--ed-selection`.

Keyboard: the grid keys of SDD-012 §7.10 (one tab stop, roving `tabindex`), the global keys of §18.2.

#### 4.3.7 Preview: canvas frame, overlays and `Transport`

- Canvas frame: the region is `--ed-preview-surround`; the canvas sits centred with 16 px padding, no radius, a 1 px `oklch(1 0 0 / 15%)` outline (decorative) so a black picture is still seen against the surround; `role="img"` with the label of SDD-012 §18.1 item 3.
- Truth on pause: the exact frame `<img>` replaces the canvas pixels without a visual cue (it is the same picture, sharper).
- Safe-area guide: section 2.10; toggled by a `Toggle` "Safe area" in the transport row, off by default.
- Selection box and handles: 1 px `--ed-handle` box; four 8 by 8 px corner squares filled `--ed-handle` with a 1 px `--ed-preview-surround` border; a 10 px rotation circle 24 px above the top edge; every handle has a 24 px hit area.
- Preview banners: unsupported WebGL2 (SDD-012 §11.2 item 6) and offline, at the top of the preview, `text-sm` on `oklch(0.2 0 0)` with a 1 px `--border`, text in the surround colours of section 2.5.
- Transport row, 40 px, centred: go to start (`SkipBack`), step back one frame (`StepBack`), Play and Pause (`Play` and `Pause`, `icon-lg` 36 px, `aria-pressed` reflects playing), step forward (`StepForward`), go to end (`SkipForward`), timecode `current / total` (`text-sm font-medium tabular-nums`; `total` in `--muted-foreground`). At the right: Safe area and Fit. Play is disabled with a tooltip when the executor reports `unsupported`.
- Keyboard: global (`Space`, `Left`, `Right`, `Shift` variants, `Home`, `End`); buttons are also reachable with `Tab`.

#### 4.3.8 Inspector field (`inspector/fields/*`, `ui/TimeField`)

- Anatomy: a two-column grid row, label column 96 px (`Label`, `text-xs`, top-aligned to the control) and control column `1fr`; under the control a message line (`text-xs`) for the error or the "changed by" note; optional unit suffix inside the field (`s`, `x`, `%`, `px`) in `--muted-foreground`; optional keyframe marker button (a 16 px diamond, filled when a keyframe exists at the playhead, outlined otherwise, `aria-pressed`).
- Variants: `NumberField`, `TimeField` (tabular figures, accepts the three forms of SDD-002 A.4), `SelectField`, `TextField`, `ColorField` (hex text, native colour input, alpha slider), read-only value (Duration, In point: plain text).
- States: section 4.1 plus `mixed` (placeholder "Mixed" in `--muted-foreground`, empty value, multi-selection) and `changed by <actor>` (the draft is kept; note in `text-agent` with the actor chip; SDD-012 §12.3).
- Commit and revert: SDD-012 §12.3 (`Enter` or blur commits, `Esc` reverts, sliders commit on release).
- Sections: header 32 px (`text-sm font-semibold`, collapsible with a chevron button), 16 px between sections; Start and End share a row.

#### 4.3.9 Media library: `ImportBar`, `AssetCard`

- Import bar: `Input` (`font-mono`, placeholder an example path for the platform, `aria-label` "File path") and `Button` `secondary` "Import"; `Enter` submits. In the desktop shell, a `Button` `outline` "Choose files" appears only when `window.vukemDesktop?.pickFiles` exists (SDD-012 §13.1).
- Drop target: when a drag enters the library, a 2 px dashed `--ring` outline and the label "Drop to import" (desktop) or the message "A browser page cannot see the path of a file. Paste the path into the field." (browser, SDD-012 §13.1 item 1).
- Asset card (a grid of as many columns of at least 16rem as the panel holds: two in the 640 px library, one in the 480 px one): thumbnail box `aspect-video`, `object-contain` on `--ed-preview-surround`, `rounded-sm`; name (`UntrustedText`, one line, tooltip with the full name); meta line `text-xs tabular-nums text-muted-foreground`: "00:05.000 · 1080×1920 · 12 MB"; state badge with text (Pending, Ingesting, Ready, Failed); `Progress` (4 px) while ingesting; actions: "Add to timeline" (`Button` `sm` `secondary`, enabled only when ready and present in the read model) and a menu.
- Failed card: the error block of section 4.3.12 inside the card, with "Try again" and "Dismiss".
- Keyboard: each card is a group with its buttons in tab order; the menu opens with `Shift+F10` and the context-menu key as well as its button.

#### 4.3.10 Job progress, `JobsChip`, export dialog

- `Progress`: track `--muted`, indicator `--primary` (3:1 against the track in both themes); 4 px in cards and in the chip popover, 8 px in dialogs; `role="progressbar"` with `aria-valuenow`, `aria-valuemin="0"`, `aria-valuemax="100"` and `aria-valuetext` ("Exporting, 42%"). Text beside it: percentage (`tabular-nums`) and the core's `message`. Failed: indicator `--destructive` and the error block. Succeeded: `CircleCheck` in `--success` and the result.
- Waiting with no progress yet: an empty bar and the text "Waiting for the server", no animation.
- Jobs chip: `Button` `ghost` `sm` with the count and the highest percentage ("2 jobs · 64%"), and a 2 px bar along its bottom edge; hidden when nothing runs.
- Export dialog (`Dialog`, 560 px wide): states Checking, Ready, Blocked, Running, Succeeded, Failed, Cancelled (SDD-012 §14.1). Lint findings grouped Errors, Warnings, Notes, each with an icon and count; a finding with an item id is a link. Footer: secondary "Re-check", primary "Start export" (disabled with a reason line when blocked). Success shows the path (`font-mono`, a copy button), size, duration and "Download".

#### 4.3.11 Command palette (`commands/palette/`)

- Anatomy: `Dialog` 640 px wide, top-aligned (20% from the top), search `Input` (`h-10`, `Search` icon, placeholder "Search commands"), results as a `listbox` (max 8 visible rows of 40 px, then the list scrolls inside the dialog): localized title, command name (`font-mono text-xs text-muted-foreground`), scope `Badge` `outline`.
- Form view: replaces the list; title, generated fields (section 4.3.8 rules), buttons "Fill example" (`ghost`), "Add to batch" (`secondary`), "Preview" (`outline`, dry run, shows `diff` and `text` in a `font-mono` block), "Run" (`default`).
- States: empty query (all commands, alphabetical by title), no results (empty state "No command matches"), error (field-level from `path`, else the error block at the top of the form).
- Keyboard: `Mod+K` opens and closes; focus starts in the search field; `Up` and `Down` move the active row (`aria-activedescendant`), `Enter` opens the form, `Esc` goes back from the form to the list and from the list closes; `Mod+Enter` runs the form.

**Decision of this document.** The palette is built on `Dialog` and a listbox, not on the `cmdk` package that shadcn's `Command` component uses, because `cmdk` would be a new dependency (author's approval needed). If the author approves it later, the anatomy and keyboard above stay the same.

#### 4.3.12 `ui/ErrorNotice` (the error block)

The one way an error from the core is shown, inside a toast, a card, a dialog or under a field.

- Anatomy: icon (`CircleAlert`, `--destructive`); headline (`text-sm font-semibold`, `t("ui.editor.errors.<CODE>")`); the core's `message` (`text-sm`, as received); the core's `hint` (`text-sm text-muted-foreground`, prefixed by the localized label "How to fix:"); the code (`font-mono text-xs text-muted-foreground`, for example `E_OVERLAP`); actions (at most two, plus "Copy details" for codes that are bugs, SDD-012 §4.4).
- Variants: `block` (dialogs, cards), `inline` (under a field: headline omitted, message and hint only), `toast`.
- `role="alert"` when raised by the user's own action; the live region announces the headline only.

#### 4.3.13 Notices

- The editor draws notices in `web/resources/js/editor/timeline/Notices.tsx`. There is no toast primitive and no `radix-ui` toast.
- The stack is fixed bottom-right (`right-3 bottom-3`), 384 px wide (`w-96`), at most 3 (`MAX_NOTICES_SHOWN`), `z-50`. The stack itself ignores pointer events; the buttons do not.
- Each notice is `role="alert"` when `kind` is `error`, otherwise `role="status"`. The body is the error block. One optional action button calls `actions.applyNotice`. Dismiss is a ghost icon button with `aria-label` from `t("ui.editor.notice.dismiss")`. Notices stay until dismissed.

#### 4.3.14 Conflict notice

The non-blocking notice of SDD-012 §4.5 for `E_REVISION_CONFLICT`.

- A `warning` toast that stays until acted on: title "Changed by `<actor>`" with the actor chip; body: the core's `details.diff` in a `font-mono text-xs` block (max 6 lines, then "Show all"); actions: primary "Apply again", secondary "Dismiss".
- The conflicting object gets the 2 s `--ed-error-flash` outline and is scrolled into view.
- The notice never applies anything by itself; "Apply again" sends one new edit (SDD-012 §4.5 item 3).

#### 4.3.15 Empty states

- Anatomy: a 32 px lucide icon in `--muted-foreground`, a title (`text-sm font-semibold`), one sentence (`text-sm text-muted-foreground`, at most 2 lines), at most one action (`Button` `secondary`). Centred in its region, 48 px vertical padding. No illustration (section 5.2).

| Where | Title | Sentence | Action |
| --- | --- | --- | --- |
| Projects list | No projects yet | Create a project, or let an agent create one; it appears here. | Create project |
| Library | No media yet | Paste the path of a video, audio or image file above and press Import. | none (the import bar is the action) |
| Timeline | The timeline is empty | Add media from the library, or ask an agent to edit. Changes from any actor appear here. | none |
| History | No changes yet | Every change by you or an agent is listed here. | none |
| Export, no findings | No problems found | The project is ready to export. | none (Start export is in the footer) |
| Palette, no results | No command matches | Check the spelling or search by command name. | none |
| Trash | The trash is empty | | none |
| Window too small | This window is too small | The editor needs at least 1024 pixels of width. | Back to projects |

Keys: `ui.editor.<area>.empty.title` and `.body` (SDD-012 §16.2).

An empty state is shown only when the region has been told it is empty: while the project is still loading, the library and the timeline draw two `Skeleton` blocks with `aria-busy="true"` instead (state model of section 4.1), so a person never reads "No media yet" about a project that has media.

#### 4.3.16 History row

- 40 px collapsed: revision (`tabular-nums`, `--muted-foreground`), actor chip, relative time, first line of the description (`UntrustedText`); expanded shows the whole text in `text-sm`. Undone steps: strikethrough plus the label "Undone" (not strikethrough alone).

#### 4.3.18 Tour card

- The first-run tour (SDD-012 §18.5): a card of at most `w-80` (`bg-popover`, `text-popover-foreground`, `rounded-md`, `shadow-md`, z-index of the popover layer) beside the region of the step, which gets a 2 px `--ring` outline drawn over it with `pointer-events: none`. Content: the step counter ("2 of 5", `text-xs text-muted-foreground`), a title (`text-sm font-semibold`), one sentence (`text-sm`), and the buttons "Back" (`ghost`, hidden on step 1), "Next" or "Done" (`default`) and "Skip tour" (`ghost`). Focus goes to "Next" when a step opens; `Esc` skips. No animation. Below 1280 px the card sits at the bottom centre instead of beside the region.
- Keys: `ui.editor.tour.*`.

#### 4.3.19 Shortcuts dialog

- A search field at the top (`Input`, `type="search"` role searchbox, label "Search shortcuts"), the preset choice (a native `select`, like the ratio of the player row) beside it, then the table of rows (action, key button), a `text-sm text-muted-foreground` line "No shortcut matches" when the filter is empty, the conflict message in `role="alert"`, and a footer with "Reset to defaults" and "Replay the tour". A key button shows the key as `kbd`; while capturing it reads "Press a key".

#### 4.3.17 `ui/UntrustedText`

Every name or text from an agent or a file (project, track, item, asset names, caption words, history text) is rendered through it: `dir="auto"`, `unicode-bidi: isolate` (SDD-012 §1.2). Visually identical to plain text; truncation with an ellipsis and a tooltip carrying the full text.

#### 4.3.18 Image editor components

The screen of SDD-012 section 24. It is the editor shell of section 3.1 over an image: a toolbar on top, the layer panel on the left, the canvas in the middle, the inspector and the history on the right.

- `image-editor/toolbar/Toolbar`: the `role="toolbar"` of 4.3.1 with groups (add: text, shape, fill, image, logo; arrange: group, ungroup, forward, back, duplicate, remove; align: six buttons; history: undo, redo; view: zoom out, zoom in, fit, 100%; export). Every button has an icon, an `aria-label` and a tooltip; the disabled ones keep their name.
- `image-editor/canvas/Canvas`: one focusable region (`role="application"` is not used; it is a `group` named by `ui.image.canvas.label`) over the picture. The picture sits on `--ed-preview-surround`; the selection outline is 2 px of `--ed-handle`; the eight handles are 10 px squares with a 24 px hit area and the rotate handle is 24 px above the top edge; guides are 1 px of `--ed-snap` (a 2:1 dash as well as a colour); the outline of a layer an agent changed is 2 px of `--agent`. Nothing on the canvas is conveyed by colour alone: the selection also shows its size as a text badge, and the agent outline comes with the live-region message.
- `image-editor/layers/LayerPanel`: a `role="tree"` (4.3.2 panel, variant `layers`). A row is 32 px: indent 16 px per level, disclosure for a group, kind icon, name, visibility and lock toggles (`aria-pressed`, 24 px targets with 32 px rows). The selected row uses `--ed-selection` as background with the text token of that pair (2.6); a dragged row shows an insertion line, and the same move is available from the keyboard.
- `image-editor/inspector/Inspector`: the inspector of 12.1 with sections per kind. Numeric fields are `ui/TimeField`-style fields with `inputMode="numeric"`, a visible label and a unit suffix; a colour field is a text field with a swatch button that opens the brand swatches.
- `image-editor/export/ExportDialog`: the dialog of 14.1 with a findings list (each finding a button that selects the layer) and a download link when the file is ready.
- `image-editor/history/HistoryPanel`: the history row of 4.3.16 with the actor chip; an undone row says "Undone" in text.
- Empty and error states: no layer ("The image has no layers", "Add a text, a shape or a photo from the toolbar"), no frame yet (a skeleton the size of the canvas), a frame that failed (the core's message with a Retry button).
- Keys: `ui.image.<area>.<name>` (SDD-012 section 16.2).

### 4.4 Shared components of the final redesign

Data-free components under `web/resources/js/components/`, built on the tokens of section 2 and with no copy of their own: every word is a prop, so it is translated by the caller (`t()`). They hold no behaviour: a link, a button or a menu is the caller's, passed as children or in a slot. Each has a unit test of its accessible name and its states (`SharedComponents.test.tsx`).

| Component | Anatomy | Accessible name and states |
| --- | --- | --- |
| `PageHeader` | the one `h1` (30/38), a sentence, the page actions on the right. The one primary action of the page is `PageHeaderAction`: a `Button` of size `lg`, 40 px high (44 px on a phone), with an icon when it has one | named by its `h1`; wraps below 768 px |
| `SectionTitle` | `h2` (24/32) or `h3` (18/26) by `level`, a sentence, actions | the level keeps the outline of the page right |
| `EmptyState` | an icon in an accent-soft disc, a title, one sentence, optional `steps` (up to three numbered first steps, `{ title, description? }`), the actions as children | a region named by its title (`aria-labelledby`); the icon is `aria-hidden`; a dashed strong border |
| `GuideStrip` | a title and the steps (`NumberedStep`) as an ordered list, three columns from `sm` | a region named by its title (`aria-label`) |
| `NumberedStep` | a numbered disc, a title, a sentence; `state` is `todo`, `current` or `done` | `current` has `aria-current="step"` and a bold title; `done` shows a check and the word given in `stateLabel`; never colour alone |
| `DataList`, `DataRow` | a description list; a row is a name (with an optional hint) and a value | a `dl` with `dt` and `dd`, so a screen reader reads each value with its name |
| `ListRow` | an optional leading icon or thumbnail, a title, a sentence, a meta line, trailing actions; an `li` | the selected row has `aria-current="true"`, `bg-surface-selected` and a 2 px bar on its left edge |
| `StatusBadge` | an icon and a word, in the tone of the status: `neutral`, `info`, `agent`, `waiting`, `success` or `danger` | the word is the name; the icon is `aria-hidden`; always icon and word |
| `LoadingState` | a static skeleton of three lines and one sentence, no spinner | `role="status"`; the skeleton is `aria-hidden` |
| `ErrorState` | `{ title, description?, steps?, onRetry?, retryLabel?, level? }`: what failed in a person's words, a "Try again" button (`ui.state.retry`) and "How to fix" (`ui.state.how_to_fix`) as numbered steps | `role="alert"` named by its title; never a raw server or network message |
| `LoadError` | `{ failure, onRetry? }`: an `ErrorState` whose words are chosen by the code of a `Failure` (`E_NETWORK`, `E_LOCKED` or `E_UNAUTHORIZED`, any other), from `ui.state.error.<kind>` | the raw message of the server is not drawn; use it for the read that fills a page |
| `OptionSelect` | `{ value, onChange, options: {value,label}[], label, size?, className?, disabled?, name? }`: the styled `Select` (Base UI) for a short list; replaces a native `<select>` | the trigger is named by `label`; the chosen option is shown by its label |
| `Segment` | `{ value, onChange, options, label, className? }`: a segmented single choice (outline toggle group, no gap) | a group named by `label`; the chosen one has `aria-pressed`; choosing it again keeps it |
| `TabBar` | `{ value, onChange, tabs: {value,label,count?}[], label, className? }`: the `line` tabs for one list in several views; it draws no panel, the page shows the view the value names | a `tablist` named by `label`; arrow keys select (`activateOnFocus`) |
| `FilterBar` | `{ label, children }`: the row of a search `Input`, `OptionSelect`s and toggles above a list or grid, no apply button, wraps | a `search` region named by `label` |
| `DetailPanel` | `{ title?, actions?, empty?, children }`: the detail side of a master-detail screen, beside a list of `ListRow`; with `empty` it says what to do when no row is chosen | an `aside` named by its title |
| `Notice` | an icon, a title, a sentence and optional actions, in a `-soft` surface of its tone: `info`, `success`, `warning` or `danger` (added by epic 84) | `role` is the caller's (`note` when it is always there, `status` or `alert` when it appears); the icon is `aria-hidden` and the title carries the tone |
| `AuthCard` | the frame of the pages before the studio opens: the centred `Wordmark`, an optional row of three `NumberedStep`, a card with the one `h1` (an `icon` tile beside it, `iconTone` `accent` or `warning`), and a closing line (added by epic 84; the wordmark, the row and the tile by the final redesign). The pages of this frame and the error page have no sidebar and no top bar; the language and the theme sit in the top corner | the steps are a labelled ordered list, the current one `aria-current="step"` |

**Publish and Connect screens** (final design, group 4). The request rows of Approvals and of the notification centre never show an id or the service's English summary: `dashboard/approval/humanize.ts` words the title from the kind of the job, the item and the name of the connection ("Publish “title” to Blog da Marina"), and the original text stays behind a disclosure. A publication list shows its time in the zone the person chose, in their language, and the account by the name they gave it. Master-detail screens (Webhooks) use `ListRow` and `DetailPanel`; the form that adds one opens in the detail side. A reject asks its reason only when a person starts to reject. The week of the calendar is a grid of hours (a subgrid keeps the rows aligned across the days), a post has a left edge in the colour of its state beside the state word, and the mini month and the guide sit in a column beside the grid, so the grid is never pushed below the fold.

**The scene bar** (`shell/SceneBar.tsx`, in every page and in the editors through `EditorSceneBar`) has, in order: the agent and its state, "Skill", "Tool", "Next" (the step after this one, in words, when the page passes `next`; otherwise "No next step reported", never a guess) and, for a request that waits, "Recusar" and "Permitir" (a person's session only). The label is "Skill", not a translation of it. `SceneBar` keeps the `none`, `idle`, `working`, `waiting`, `denied`, `done` and `failed` variants.

**The trail** (`workflow/`) draws the steps the service reports and decides nothing. The rule that no step is done after the current one is in `core/project` (SDD-005 section 5.12.4).

## 5. Iconography, illustration and the product mark

### 5.1 Icons

- Set: Lucide, through `lucide-react` 1.68.0 (`components.json` `iconLibrary: "lucide"`), inline SVG, no icon font, no remote sprite.
- Grid and sizes: 24 px design grid with 2 px stroke (Lucide default), rendered at 16 px (`size-4`, the default inside buttons), 14 px (`size-3.5`, `sm` buttons), 12 px (`size-3`, `xs` buttons, item type icons, badges), 20 px (`size-5`, transport Play) and 32 px (empty states). Stroke stays 2 at every size; icons are never filled except the activity dot and the keyframe diamond, which are drawn shapes, not icons.
- Colour: `currentColor`; an icon takes the colour of its text.
- Decorative icons have `aria-hidden="true"`; an icon-only button gets its name from `aria-label`.
- One concept, one icon:

| Concept | Lucide icon | Concept | Lucide icon |
| --- | --- | --- | --- |
| back to projects | `ArrowLeft` | split | `Scissors` |
| play, pause | `Play`, `Pause` | add caption | `Captions` |
| go to start, end | `SkipBack`, `SkipForward` | add text | `Type` |
| step one frame | `StepBack`, `StepForward` | clip, video track | `Film` |
| undo, redo | `Undo2`, `Redo2` | audio | `AudioLines` |
| export | `Download` | shape | `Square` |
| zoom in, out, fit | `ZoomIn`, `ZoomOut`, `Scan` | snap | `Magnet` |
| command palette | `Command` | hide, show | `EyeOff`, `Eye` |
| search | `Search` | mute | `VolumeX`, `Volume2` |
| theme | `Sun`, `Moon` (kept) | menu | `Ellipsis` |
| language | `Languages` | drag handle | `GripVertical` |
| agent actor | `Bot` | person actor | `User` |
| API key actor | `KeyRound` | reconnecting | `RefreshCw` (static) |
| error | `CircleAlert` | warning | `TriangleAlert` |
| success | `CircleCheck` | info | `Info` |
| trim start and end | `ArrowLeftToLine`, `ArrowRightToLine` | import | `FileInput` |

The implementer checks each name against `lucide-react` 1.68.0 when importing; a renamed icon is replaced in this table in the same pull request.

### 5.2 Illustration

None. Empty states use one icon (section 4.3.15); there are no mascots, stock photos or decorative images in the product UI. The only images are the user's own media (thumbnails, frames, waveforms).

### 5.3 The product mark

There is no product mark yet. Until the author approves one, the **placeholder** is the one `AppLayout.tsx` already draws: a 32 px square with `rounded-lg` corners in `--brand` holding the Lucide `Video` icon at 16 px in `--brand-foreground`, followed by the name "Stuarty" in `text-lg font-semibold tracking-tight`.

- The placeholder is not a logo: it is not used as the favicon of a release, in the installers, on the site's landing page as a brand, or in any third-party listing. Those wait for the real mark (SDD-010 §1.1 replaces vukem's logo and icon "by the product ones").
- Minimum size 16 px; clear space around it equal to a quarter of its size; never recoloured, stretched or placed on a picture.
- When the mark exists, it is added as `web/resources/public/` SVG files and to `site/public/`, and this section is rewritten.

## 6. Content style

### 6.1 Voice

Plain, direct, calm. The UI talks to a person who edits video and works next to agents. Present tense, active voice, second person when addressing the user ("Paste the path"). No exclamation marks, no jokes, no "Oops", no "please" in instructions, no blame ("You entered an invalid value" becomes "Use a time such as 12.5s, 375f or 00:12.500"). Agents are named by their actor label, never "the AI" or "the robot".

### 6.2 Capitalization and labels

- Sentence case everywhere: buttons, menu items, titles, tabs, toasts ("Add caption", "Export dialog", "Move to next free position").
- Product and proper names keep their case: Stuarty, MCP, CLI, FFmpeg, MP4.
- Buttons start with a verb and name the result: "Export", "Start export", "Add to timeline", "Try again", "Apply again". "OK" and "Yes" are not labels.
- Terms come from the shared glossary (`site/content/glossary.json`, SDD-010 §10): track, clip, item, caption, playhead, trim, split, safe area. "Asset" is "media" in the UI. Command names, field names and error codes are never translated and are shown in `font-mono`.

### 6.3 Errors from the core

The core's envelope (SDD-002 A.8, carried on every surface as SDD-003 §1.4 says) is shown by the error block (section 4.3.12):

1. The headline is the localized `ui.editor.errors.<CODE>`, written as what happened, in a few words ("Items would overlap", "Export failed").
2. Then `message` and `hint` exactly as received, in English in both locales (SDD-012 §16.4), never edited, truncated only with "Show all".
3. Then the code in `font-mono`.
4. UI-only errors (`E_NETWORK`) follow the same pattern with a localized message and hint.

### 6.4 Numbers, times and durations

| What | Format | Rule |
| --- | --- | --- |
| Timecode | `00:12.500` (`mm:ss.mmm`, hours when the project is an hour or longer: `1:02:03.500`) | `formatTime` of SDD-012 §12.2; never localized: the decimal separator is always `.` |
| Frames | `375f` | |
| Seconds | `12.500s` | three decimals |
| Asset duration | as the user's time display mode, from `duration_seconds` through `formatSeconds` | never derived from frames (SDD-012 §13.2) |
| Ruler labels | `m:ss`, or `s+ff` below one second (`12+15`) | SDD-012 §7.3 |
| Speed | `1.5x` | |
| Dimensions | `1080×1920` with the multiplication sign | |
| File sizes | decimal units, `12 MB`, `1.4 GB`, one decimal below 10 | `Intl.NumberFormat` with the locale for the number |
| Percentages | `42%` | `Intl.NumberFormat` `style: "percent"`, no decimals |
| Relative time | "12 s ago", "há 12 s" | `Intl.RelativeTimeFormat`, `numeric: "auto"`, style `short` |
| Dates | locale medium date | `Intl.DateTimeFormat` |
| Revision | "Rev 42" | |
| Counts | "3 items", "1 item" | plural objects (SDD-012 §16.2) |

All of these use `tabular-nums` when they are in a column or change while visible.

### 6.5 Internationalization

- Locales: `en` (source) and `pt-BR` (RF-073). Every string goes through `t(key)` (rule T9); keys follow SDD-012 §16.2; placeholders `{name}`; plurals by `Intl.PluralRules`.
- Never build a sentence by concatenating translated pieces; one key per sentence, with placeholders.
- Text expansion: Portuguese runs about 20 to 30% longer than English. Every label is designed for 35% more width: no fixed widths on text buttons, tabs or badges; toolbars collapse text to icons rather than wrap (section 4.3.1); dialog buttons may wrap to a second row, never truncate. User content truncates with an ellipsis and a tooltip; UI labels in menus and dialogs wrap instead of truncating.
- The test `i18n-switch` (SDD-012 §20.1) checks pt-BR in both themes at 1280 by 720.
- Timecodes, frame counts and time strings are never localized (section 6.4). Other numbers use the locale (`1,5` in pt-BR for a displayed decimal). In pt-BR, number and time fields also accept a typed decimal comma and send it as `.` (`12,5` becomes `"12.5s"`); timecodes are always shown with `.` (D-030, SDD-012 §12.2).
- Core text (messages, hints, history descriptions) stays English in both locales and is marked `lang="en"` inside a pt-BR page, so screen readers pronounce it correctly.

## 7. Accessibility

Target: WCAG 2.2 level AA for every control outside the canvas (RF-075); keyboard operation and automated checks, then a sign-off (SDD-012 §18).

### 7.1 Focus order

`Tab` follows the regions in the order of SDD-012 §18.2: top bar, library, preview (transport), timeline, inspector. Inside a region, the visual order left to right, top to bottom. Composite widgets (toolbar, timeline grid, tabs, listbox, menus) take one tab stop and use arrow keys inside. `F6` jumps between regions. Dialogs trap focus and return it to their opener; after `Delete` focus moves to the next item on the track (SDD-012 §18.1 item 4).

### 7.2 Visible focus

Every focusable element shows the 2 px `--color-focus` outline with 2 px offset (section 2.10), at 3:1 or more against what is around it in both themes. Focus is never hidden by sticky elements: the timeline scrolls a focused item clear of the sticky headers and ruler (WCAG 2.4.11).

### 7.3 Keyboard map

The keyboard map is SDD-012 §18.2 (global keys) and §7.10 (timeline grid keys); there is no separate shortcut SDD. Principles that any new shortcut follows:

1. Every shortcut calls an action of the action registry, so the same action is reachable by a button or the palette.
2. Single letters (`S`, `C`, `T`, `N`) are ignored while focus is in an editable field; `Mod` combinations follow the editable-field exceptions of §18.2.
3. `Mod` is `Ctrl` on Windows and Linux and `Cmd` on macOS; tooltips show the platform's form (`Ctrl+K`, `⌘K`).
4. No shortcut needs a key that a common keyboard layout lacks without a modifier; `[` and `]` have toolbar buttons as an alternative.
5. Customization and the cheat sheet (`?`) are specified by RF-074 and SDD-012 §18.9: one registry, two presets (`short`, `classic`), every row remappable, a conflicting key refused with the name of the row it collides with.
6. A handler never names a key: it asks the registry. Tooltips show the key that is current (`Split (Ctrl+B)`), so a rebound key is never stated wrong.

### 7.4 Screen reader names

| Object | Role | Accessible name pattern (localized) |
| --- | --- | --- |
| Timeline | `grid` | "Timeline, `<n>` tracks" |
| Track header | `rowheader` | "Track `<name>`, `<kind>`", plus ", hidden" and ", muted" when they apply |
| Item | `gridcell`, `aria-selected` | "`<Type>` `<label>`, `<start>` to `<end>`, track `<track name>`" plus ", changed by `<actor>`" for 1.5 s after a remote edit |
| Caption word | button inside the cell | "Word `<text>`, `<start>`" |
| Ruler | `slider` | "Playhead", `aria-valuetext` the timecode |
| Preview canvas | `img` | "Preview, frame `<n>` at `<timecode>`" |
| Asset card | `group` | "`<name>`, `<kind>`, `<duration>`, `<state>`" |
| Progress | `progressbar` | "`<job kind>` `<name>`", `aria-valuetext` "42%" |
| Activity indicator | button | its visible text |

Times inside names use the timecode form. Results of actions are announced through the polite live region (SDD-012 §18.1 item 5).

### 7.5 Motion

Motion comes only from the tokens of section 2.13, and the editor components add only `.ed-motion` and `.ed-highlight`; under `prefers-reduced-motion` every transition and animation lasts 1 ms (`--duration-reduced`). Nothing flashes more than three times per second; the timed outlines appear once and disappear once.

### 7.6 Contrast

The pairs and ratios of section 2.6, in both themes, checked in CI by `bun run check:contrast` and axe (`a11y-axe-both-themes`, SDD-012 §18.4). Decorative dividers (`--border`) are exempt and never the only cue. Nothing is conveyed by colour alone: selection has a ring and `aria-selected`; states have an icon and text; item types have an icon and a label.

### 7.7 Forced colours

In Windows high-contrast mode (`forced-colors: active`) the system replaces colours. Rules: focus and selection use `outline` and `border`, which survive forced colours (not `box-shadow` alone; the inset selection ring adds `outline: 2px solid Highlight` under `@media (forced-colors: active)`); item blocks keep their 1 px border; icons use `currentColor`; no component sets `forced-color-adjust: none` except the preview canvas, which shows the picture.

### 7.8 Target sizes

At least 24 by 24 CSS px for every target (WCAG 2.5.8); the design minimum is 32 px on desktop and 44 px at phone width (below 768 px), met by the shared controls (the compact sizes grow on the dashboard through the `dashboard` variant, section 2.14). Timeline handles: 8 px visual, 24 px hit area; preview handles: 8 px visual, 24 px hit area. Adjacent small targets keep at least 24 px between centres.

### 7.9 Accessibility of each component

Epic 41. What each shared component owes beyond sections 7.1 to 7.8; the checklist per section is SDD-012 §18.10 and the specs that decide it are `a11y-*` and `wcag22` in `web/e2e/specs/`.

| Component | Name and role | Keyboard | Target and motion | Notes |
| --- | --- | --- | --- | --- |
| Button, link button | the visible text, or `aria-label` for an icon; a link that looks like a button has `role="link"` (`Button asChild` with an `<a href>`) | `Enter`, and `Space` for a button | 24 px at least, 32 px by default | a disabled button is out of the target size check; the label is never colour alone |
| Input, textarea, native select | a `<label>` (visible) or `aria-label`; the error and the hint through `aria-describedby`; `aria-invalid` when refused | native; a select takes the typeahead and the arrows | 32 px high | native selects stay (D-099): they are named, keyboard operable and read by every screen reader; a Base UI select is used only where a custom list is needed (the palette) |
| Checkbox and radio | native `<input>` or the Base UI checkbox, with a visible label | `Space` | a 24 px target: the native inputs by the rule of `app.css`, the Base UI checkbox draws its 16 px box inside a 24 px target | a checkbox inside a label is also a target; the label text is not colour alone |
| Switch, toggle, toggle group | `role="switch"` or `aria-pressed`, with the name of what it turns | `Space`, arrows in a group | 24 px | the state is in the accessible state, and the look adds an icon or text |
| Dialog, alert dialog | `role="dialog"` or `alertdialog`, named by its title, described by its description | the focus goes in, `Tab` stays inside, `Esc` closes, the focus returns to the opener | the open and close animation is at most 100 ms and is removed with reduced motion | after a menu item opens a dialog, the focus returns to the button of the menu |
| Menu, context menu | `menu`, `menuitem`, `menuitemcheckbox` with names | arrows, `Enter`, `Esc`; the menu button has `aria-haspopup` | 24 px rows | a context menu has a keyboard twin (the `Menu` key or the actions button) |
| Tabs | `tablist` named, `tab` with `aria-selected`, `tabpanel` named by its tab | one tab stop, arrows move between tabs | | the selected tab keeps its place across selections |
| Tooltip | describes, never the only name | opens on focus and on hover, `Esc` closes | | holds the key of the action from the registry |
| Progress | `role="progressbar"` with a name and `aria-valuenow`; the text beside it is hidden from a reader (`aria-hidden`) and a second text, in steps of 10%, is read in the polite region | | no spinner turns | `export/ProgressText.tsx` |
| Badge | text, never a colour alone; it grows with the text spacing (`min-h-5`) | | | a state badge adds an icon or a word |
| Status, alert | `role="status"` for a result, `role="alert"` for an error that stops the person; an error note has an `id` that its fields name in `aria-describedby` | | | `components/Failure.tsx` |
| Sidebar | a `nav` named "Main navigation"; the current page has `aria-current="page"`; the collapse button has `aria-expanded` | `Tab` through the links | 36 px rows | the skip link of the layout is before it |
| Skip link and main region | the first focusable element of a dashboard page, "Skip to the main content", goes to `main#main` (`tabindex="-1"`, no ring: it is not a control) | `Enter` | | a change of page moves the focus to `main` |
| Chat dock | the button has `aria-expanded` and `aria-controls`; the panel is an `aside` named "Chat" | the panel takes the focus when it opens, `Esc` closes it and the button gets the focus back | | the conversation is a log read in the polite region |
| Timeline list view | a `table` with `th scope`; the name of a row is a button | `Tab` | | SDD-012 §18.10 |

## 8. Web and desktop

### 8.1 The rule

The desktop shell is a window around the same binary and the same UI (ADR-0023 item 2): it loads `web/resources/` from the local address, with the same `app.css`, the same tokens, the same bundled Geist font and the same components. It contains no editing logic, no renderer and no second stylesheet. Nothing in this document has a desktop variant unless section 8.3 lists it.

### 8.2 Identical in both

Tokens and both themes; typography (bundled, not system fonts); layout and breakpoints; components and their states; icons; keyboard map; i18n; the error block; the projects page; contrast and focus.

### 8.3 What differs

| Topic | Browser (now) | Desktop shell  |
| --- | --- | --- |
| Window chrome | the browser's | native title bar and decorations of the operating system; no custom title bar, no draggable CSS region (`app-region`), so the web UI needs no desktop-only layout |
| Window size | any; section 3.2 notices below 1280 px | minimum inner size 1280 by 640 (section 3.2) |
| Menus | the "Menu" button of the top bar (section 3.4.3), the top bar and the palette | native menu bar, with the same items as the in-page Menu button, which is then not drawn (SDD-012 §6.4): File (Import files, Export), Edit (Undo, Redo, Select all), View (toggle panels, zoom, theme, language), Help (documentation, version). Each item calls the same action as its in-page control through `window.vukemDesktop.onMenuAction(cb)`. **`Mod+Z`, `Mod+Shift+Z` and `Mod+A` are shown in the menu but not registered as native accelerators** (D-030), so text fields keep their native undo and select-all and project undo applies elsewhere (SDD-012 §18.2, §21.3) |
| File choice | path field only (SDD-012 §13.1 item 1) | "Choose files" button through `window.vukemDesktop.pickFiles()`; the path field stays |
| Drag and drop from the OS | drop shows the "cannot see the path" message | drop imports through `onDropPaths`; the library shows the drop target of section 4.3.9 |
| Context menu | the in-app menus; the browser's own menu elsewhere | the in-app menus; the webview's native menu is kept only in editable fields (cut, copy, paste) |
| Theme | D-024: dark by default, the toggle, `localStorage.theme`; `prefers-color-scheme` ignored | the same, inside the webview. The shell paints the window background `oklch(0.16 0 0)` (`#0d0d0d`, dark `--ed-bg`) before the page loads, so there is no white flash, and sets the native title bar to the theme the page passes to `window.vukemDesktop.setTheme(theme)` at boot and on every toggle (D-030) |
| Zoom | browser zoom applies; `Ctrl+wheel` on the timeline is taken by the timeline (non-passive listener) | page zoom fixed at 100% by the shell; `Ctrl+wheel` belongs to the timeline |
| Text rendering | the browser's | the webview's; small anti-aliasing differences are accepted, layout must not depend on them |
| Scrollbars | the browser's | the webview's (overlay scrollbars on macOS); no component depends on scrollbar width or styled scrollbars |

The only coupling between the UI and the shell is `window.vukemDesktop`, feature-detected (SDD-012 §13.1, §21.1). Its members are `pickFiles()`, `onDropPaths(cb)`, `onMenuAction(cb)` and `setTheme(theme)`, all optional (D-030).

### 8.4 The documentation site

The site (`site/`, SDD-010, D-028) is a separate Astro package that starts from the vukem site shell: its `globals.css` Tailwind 4 theme, components, layout and theme toggle are copied from vukem (SDD-010 §1.1). It does not import `web/resources/css/app.css`.

| The site follows the product in | The site departs from the product in |
| --- | --- |
| the product mark and `--brand` colour (when the mark exists, section 5.3); the landing page uses "the copy, mark and colours of Stuarty only" (D-028) | typography, spacing and layout: those of the vukem shell |
| the content style of section 6 and the shared glossary (SDD-010 §10) | theme behaviour: the shell's toggle as copied; D-024's dark default is a rule of the editor interface |
| screenshots of the UI: dark theme, 1600 by 900, the fixture project of the e2e suite, no personal paths visible | components: MDX components of the shell (callout, card, code block, steps, tabs) |
| no remote fonts or trackers (the site is static on GitHub Pages) | density: comfortable reading layout |

The header, the popups it opens and the documentation use the tokens of the final redesign. The landing pages keep their previous palette until the new landing ships (D-137): `Base.astro` marks them with `data-surface="landing"` on `<html>`, `globals.css` restores the previous values for that surface, and `data-site-chrome` keeps the redesign tokens on the header and its popups.

## 9. Governance

### 9.1 One source per thing

| Thing | Lives in | Owner |
| --- | --- | --- |
| Token names and values | `web/resources/css/app.css` (`:root`, `.dark`, `@theme inline`); this document records them; the pairs are in `web/scripts/check-contrast.ts` | the senior of the UI foundation |
| Token rules, components, content style | this document | architect, with senior-e |
| Editor token names | SDD-012 §17 (names), values here | senior-e |
| UI strings | `web/resources/lang/en.json`, `pt-BR.json` (SDD-012 §16) | senior-e, interns for translation |
| Product terms | `site/content/glossary.json` (SDD-010 §10) | senior-d |
| Site theme | `site/app/globals.css` (takes `--brand` from here) | senior-d |
| Desktop shell | no tokens of its own | the desktop owner |

### 9.2 How a change is made

1. **This document first.** Change the token, component rule or style rule here, with the reason (a contrast number, a decision, a requirement). A change of an editor token name also changes SDD-012 §17 in the same pull request.
2. **Then `app.css`.** Edit the value or add the variable and its `@theme inline` mapping. Run `bun run check:contrast`; if a pair fails, go back to step 1.
3. **Then the components.** Use the new utility; never a literal colour or a raw palette class. A visual change that the shared primitive does not need goes into `app.css` or a wrapper (SDD-012 §3.1).
4. **Then the tests.** The `@themes` Playwright scenarios and `a11y-axe-both-themes` pass in both themes.

A new shared primitive is built on `@base-ui/react` or `@ui-sdk/react` and listed in SDD-012 §3.1. Any other npm package needs the author's approval first.

### 9.3 Checklist for UI pull requests

Copy into the pull request description and tick each line that applies.

- [ ] No literal colour, raw Tailwind palette class or inline style colour; only tokens of section 2.
- [ ] Both themes checked; `bun run check:contrast` passes; new text and background pairs are added to the script and to section 2.6.
- [ ] Every interactive element shows the focus outline and works with the keyboard alone; the composite widget pattern (one tab stop, arrows inside) is used where section 4 says so.
- [ ] Every icon-only control has an `aria-label` from `t()` and a tooltip with its shortcut.
- [ ] Targets are at least 32 px on desktop and 44 px at phone width (24 by 24 CSS px at the very least).
- [ ] Text is at least 14 px on the dashboard and 12 px in the editors; the focus ring is the global `--color-focus` outline, with no halo of the component.
- [ ] Nothing is conveyed by colour alone.
- [ ] Durations from the motion tokens, never a literal time; no `transition`, `animation` or `@keyframes` in an editor component besides `.ed-motion` and `.ed-highlight` (D-013); no spinner; reduced motion respected.
- [ ] Every string through `t()`; no concatenated sentences; the layout holds in pt-BR at 1280 by 720.
- [ ] User content rendered with `UntrustedText`.
- [ ] Errors from the core shown with the error block: headline, message and hint as received, code.
- [ ] No optimistic result: pending states are ghosts or busy fields (SDD-012 §1).
- [ ] Agent actions are attributed with the actor chip or label.
- [ ] Empty, loading, error and disabled states designed and present.
- [ ] Only features of SDD-012 §21.1 used, so the desktop webview shows the same.
- [ ] No asset loaded from another origin; no new dependency without the author's approval.
- [ ] This document, SDD-012 §17 and `app.css` agree.

## 10. Decisions of this document and open points for the specifications

Decisions taken here (each is cheap to undo). "Recorded in" names where the specifications now say the same; D-030 is the author's answer of 2026-11-14 to the questions this document raised.

| # | Decision | Section | Recorded in |
| --- | --- | --- | --- |
| DS-1 | `--ring` equals `--ed-selection`; a solid 2 px outline in `app.css` makes the focus indicator meet 3:1 without editing the kept components | 2.3, 2.10 | SDD-012 §17, §18.1 item 6 |
| DS-2 | Changed values: light `--muted-foreground`, light `--destructive`, `--input`, `--ring`, `--ed-track-line`, light `--ed-ghost` (contrast) | 2.3, 2.5 | SDD-012 §17 table |
| DS-3 | New tokens `--success`, `--warning`, `--info`, `--agent`, `--brand`, `--ed-handle`, `--radius-xs` at 4 px, motion tokens | 2.4, 2.5, 2.9, 2.13 | SDD-012 §17 |
| DS-4 | Timecodes in the UI font with tabular figures, no monospace UI font (measured on Geist, 24.48 px per digit at 40 px; Inter Variable has tabular figures too and replaced Geist in epic 74, DS-15) | 2.7 | SDD-012 §11.5, D-030 |
| DS-5 | No bottom status bar | 3.1 | this document only (SDD-012 §6.1 has none) |
| DS-6 | Desktop window minimum inner size 1280 by 640, default 1600 by 900 | 3.2 | SDD-012 §21.3 |
| DS-7 | Several active agents: one badge with the latest actor and a count, a popover listing each actor and its last action | 4.3.4 | decided by D-030; SDD-012 §5.8 |
| DS-8 | The command palette is built on `Dialog` and a listbox, without `cmdk` | 4.3.11 | SDD-012 §3.1, §9.2 |
| DS-9 | Notice stack placement (bottom-right, at most 3) and the persistent conflict notice | 4.3.13, 4.3.14 | this document only |
| DS-10 | Shared primitives (`Dialog`, `AlertDialog`, `Popover`, `DropdownMenu`, `ContextMenu`, `Slider`, `Switch`, `Checkbox`, `RadioGroup`, `Progress`, `Toggle`, `ToggleGroup`, `Select`, `Tabs`, `Tooltip`) from `@base-ui/react` and `@ui-sdk/react` | 4.2 | SDD-012 §3.1 |
| DS-11 | Native menus show `Mod+Z`, `Mod+Shift+Z` and `Mod+A` without registering them as accelerators; menu items and the title-bar theme go through `onMenuAction` and `setTheme` | 8.3 | decided by D-030; SDD-012 §13.1, §21.3 |
| DS-12 | The familiar layout (D-038): a 40 px tab rail inside the library width, a 36 px tab strip in the inspector, a selected tab shown by a 2 px `--ring` bar, a save indicator and a Menu button in the top bar, the timeline toolbar in two labelled groups; no new token | 3.4 | SDD-012 §6.4, §6.5 |
| DS-13 | The dashboard has its own sidebar built from checked tokens; the shadcn `Sidebar` stays unused | 3.5 | SDD-012 §23 |
| DS-14 | The first-run tour is a card beside the region of each step (outline in `--ring`), not a full-screen overlay, so the keyboard and the screen reader are never trapped; the shortcuts dialog gets a search field and a preset choice | 4.3.18, 4.3.19 | SDD-012 §18.5, §18.9 |
| DS-15 | The final redesign (epic 74): semantic tokens `--color-*` in OKLCH with aliases for the former names, the 14 px dashboard scale, radii 6 to 24 px, three shadows, motion of 120, 200 and 320 ms with a 1 ms reduced value, Inter, and the shared components of section 4.4 | 2.1 to 2.14, 4.4 | the final redesign of the author; D-138 for Inter |

Open points found in the specifications while writing this document, and how each was closed:

1. **SDD-012 §17 initial values failed its own §18.3 pairs** (`--ed-track-line` in both themes, light `--ed-ghost`, no handle token). Decided: SDD-012 §17 now has the values of section 2.5 and `--ed-handle`. Still open as a suggestion: §18.3 could mark row separators as decorative, which would let `--ed-track-line` stay subtle and keep the 3:1 rule for ruler ticks only.
2. **The remote-edit outline used `--ed-selection`**, so a remote edit looked like a selection. Decided by D-030: it uses `--agent` (sections 2.4, 2.5, 2.10, 4.3.6; SDD-012 §5.8, §17).
3. **Shadcn animations versus "no interface animations".** Decided: SDD-012 §17 item 5 keeps what the kept components carry and adds the reduced-motion rule of section 2.13.
4. **Components missing from SDD-012 §3.1.** Decided: §3.1 lists the components of DS-10.
5. **No timecode font.** Decided by D-030 and measured: Geist with tabular figures (DS-4, SDD-012 §11.5).
6. **Typed decimals in pt-BR.** Decided by D-030: the comma is accepted and sent as `.`; timecodes are always shown with `.` (section 6.5, SDD-012 §12.2).
7. **Font select samples.** Decided by D-030: names in the UI font, no samples (section 2.7, SDD-012 §12.1).
8. **Desktop shell details.** Decided: SDD-012 §21.3 (window size, chrome, pre-paint background) and D-030 (menus, accelerators, `onMenuAction`, `setTheme`, SDD-012 §13.1).
9. **Connection-state colours and several active agents.** Decided: SDD-012 §6.2 (tones of section 4.3.3) and D-030 (section 4.3.4, SDD-012 §5.8).
10. **The site's theme default.** Decided: SDD-010 §1.1 keeps the copied shell's behaviour (section 8.4).
11. **Light `--ed-safe-area` on the preview surround is 3.03:1**, the smallest margin of the whole set. Open: since the surround is the same in both themes, the overlay colours drawn on it could be theme-independent (the dark value, 9.65:1).
12. **The error envelope** is specified in SDD-002 A.8; SDD-003 §1.4 carries it across surfaces. A note for briefings, no change needed.
13. **The border of a pressed toggle (resolved).** `Button` draws a pressed toggle with `border-foreground/50` over `--accent`. Composited in sRGB, that border is 3.98:1 against `--ed-panel` in light and 5.67:1 in dark, above the 3:1 of a state indicator (WCAG 1.4.11). At 40% it was 2.92:1 in light. The `--accent` fill alone cannot carry the state, because it is barely lighter or darker than the panel.

## 11. Brand refresh: the identity from the official icon (2026-11-20)

- Status: Proposed (2026-11-20), from the author's decision to redefine the identity from the official icon and to apply it to the landing, the documentation and the product, in the product only through tokens. The reasoning is in [brainstorming-design-system-and-landing-page.md](brainstorming-design-system-and-landing-page.md).
- This section adds to sections 1 to 10 and wins over them where it says so. Every other rule stays.

### 11.1 The mark and its colours

The official icon is the clapperboard with eyes (`.config/installers/assets/stuarty-icon.png`, scaled by `internal/tools/installericon` into every copy the project ships, section 5). Its three colours, measured on the source file:

| Name | Hex | OKLCH | Role |
| --- | --- | --- | --- |
| Indigo | `#3534CA` | `oklch(0.442 0.222 272.9)` | the body of the clapperboard; the brand in the light theme |
| Lilac | `#8078FB` | `oklch(0.645 0.189 282.5)` | the clapper bar; the brand in the dark theme and the accent of gradients |
| White | `#FDFDFD` | `oklch(0.994 0 0)` | the eyes; text on the indigo brand |

The logo lockup (the icon and the word "Stuarty", chosen by the author on 2026-11-20: `.config/brand/logo/stuarty-logo-3.png`, with the three other candidates kept beside it) is the mark of the navbar of the documentation and the landing, the top of the README and the title of the dashboard (web and desktop). It ships as two transparent PNGs, `logo-wordmark-light.png` (indigo word, for light backgrounds) and `logo-wordmark-dark.png` (white word, for dark backgrounds), in `site/public/` and `web/resources/public/`, swapped with the `dark` variant; the collapsed sidebar shows the icon alone. The product name stays in the page as text for screen readers.

### 11.2 The brand scale

One scale, hue 272 to 283, built so that step 500 is the lilac and step 700 the indigo of the icon. Contrast is WCAG 2.x against white and against the dark `--background` (`oklch(0.145 0 0)`).

| Step | OKLCH | Hex | vs white | vs dark background |
| --- | --- | --- | --- | --- |
| 50 | `oklch(0.97 0.025 280)` | `#F1F4FF` | 1.10 | 18.01 |
| 100 | `oklch(0.94 0.045 280)` | `#E5E9FF` | 1.21 | 16.38 |
| 200 | `oklch(0.88 0.08 280)` | `#CCD2FF` | 1.47 | 13.46 |
| 300 | `oklch(0.8 0.13 281)` | `#AFB3FF` | 1.96 | 10.12 |
| 400 | `oklch(0.72 0.17 282)` | `#9694FF` | 2.64 | 7.49 |
| 500 (lilac) | `oklch(0.645 0.189 282.5)` | `#8078FB` | 3.50 | 5.66 |
| 600 | `oklch(0.55 0.21 278)` | `#5C58E8` | 5.22 | 3.79 |
| 700 (indigo) | `oklch(0.442 0.222 272.9)` | `#3534CA` | 8.43 | 2.35 |
| 800 | `oklch(0.37 0.18 272)` | `#26299C` | 11.13 | 1.78 |
| 900 | `oklch(0.29 0.13 272)` | `#191E6A` | 14.63 | 1.35 |
| 950 | `oklch(0.21 0.09 272)` | `#0C1040` | 17.99 | 1.10 |

The neutral scale of section 2.2 stays the scale of the chrome.

### 11.3 Brand tokens

| Token (Tailwind) | Light | Dark | Use |
| --- | --- | --- | --- |
| `--brand`, `--brand-foreground` | 700, white (8.43:1) | 500, `oklch(0.145 0 0)` (5.66:1) | the mark, the primary action of the dashboard pages and of the documentation, the active navigation item |
| `--brand-accent` (new) | 500 | 400 | gradients, highlights, the clapper bar of the 3D mascot; never text on white (3.50:1) |
| `--brand-subtle` (new) | 50 | 900 at 40% | tinted surfaces: a selected card, a callout, a badge background |
| `--brand-text` (new) | 700 | 300 | brand-coloured text and links on `--background` (8.43:1 light, 10.12:1 dark) |

**What changes from section 2.4.** `--brand` was a placeholder, `oklch(0.511 0.262 276.966)` in both themes, "never a button". It becomes the icon's indigo (light) and lilac (dark), and it may now carry the one primary action of a dashboard page and of the documentation. Inside the editors (`web/resources/js/editor/**` and the image editor) the primary action is the accent of section 2.4, and no brand colour is used near the picture, the timeline or the canvas.

### 11.4 Two registers

| | Quiet register (the product) | Expressive register (the landing and the documentation home) |
| --- | --- | --- |
| Where | dashboard, editors, settings, the desktop app, the documentation pages | `/` and `/landing/*` of the documentation site |
| Colour | neutral chrome; brand on the mark, the primary action, the active navigation item; states as section 2.4 | brand gradients (700 to 500, at most two stops plus neutral), the brand scale for surfaces |
| Type | the scale of section 2.7 | adds the display scale of section 11.5 |
| Motion | the two kinds of section 2.13 | adds the scene motion of section 11.8 |
| 3D | none | three.js scenes and a living background (section 11.9) |

The two registers share every token; the expressive one only uses more of them.

### 11.5 Typography

Inter Variable is the only family (section 2.7; bundled, no other origin). The expressive register adds a display scale:

| Role | Utility | Line height | Weight and tracking |
| --- | --- | --- | --- |
| Hero | `text-6xl` to `text-8xl` (`clamp(2.75rem, 7vw, 6rem)`) | 1.0 | 600, `tracking-tight` (-0.025em) |
| Section title | `text-4xl` to `text-5xl` | 1.1 | 600, `tracking-tight` |
| Lead | `text-lg` to `text-xl` | 1.6 | 400, `text-muted-foreground` |
| Eyebrow | `text-xs`, uppercase | 1.4 | 500, `tracking-widest`, `--brand-text` |
| Code and terminals | `font-mono`, `text-sm` | 1.7 | 400 |

The 12 px minimum and the weights 400, 500 and 600 of section 2.7 hold in both registers.

### 11.6 Buttons

All buttons are the Base UI `Button` of section 4.2, with `rounded-md` (8 px), the sizes of section 4.2, `h-12` for the landing `lg`, and a 6 px gap between icon and label.

| Variant | Light | Dark | Use |
| --- | --- | --- | --- |
| `brand` (new) | `bg-brand text-brand-foreground`, hover 800 | `bg-brand text-brand-foreground`, hover 400 | the one primary action of a dashboard or documentation page; the main call to action of the landing |
| `default` | the accent of section 2.4 (`--color-accent`) | same | the primary action, in the editors and wherever a page has no brand button |
| `secondary`, `outline`, `ghost`, `link`, `destructive` | as section 4.2 | as section 4.2 | as section 4.2 |

At most one `brand` button per view. Focus is the 2 px `--ring` outline of section 2.10 on every variant.
**The call-to-action orange (landing only).** The author asked for one striking "Getting Started" button on the landing pages. Orange is the one hue added to the palette, and only for that button: it never appears in the product or in the documentation pages.

| Token | Both themes | Hex | Use |
| --- | --- | --- | --- |
| `--cta` | `oklch(0.7 0.2 48)` | `#FC6D00` | the "Getting Started" button of the landing hero and footer |
| `--cta-foreground` | `oklch(0.145 0 0)` | `#0A0A0A` | its label |
| `--cta-hover` | light `oklch(0.64 0.2 45)`, dark `oklch(0.76 0.18 55)` | `#E85600`, `#FF8C1E` | hover; the focus ring stays the 2 px outline |

Contrast (WCAG 2.x): `--cta-foreground` on `--cta` 6.91:1, on the light hover 5.45:1, on the dark hover 8.52:1, all above 4.5:1. The button fill against the page is 2.86:1 on white and 6.91:1 on the dark background; the label, not the fill, identifies the control, so WCAG 1.4.11 does not apply to the fill.

**The landing actions (glow buttons).** Getting Started, the two Docker guides and the three downloads share one style (`.btn-glow` in `site/src/landing/landing.css`): a neutral face (`--bg`) with the label in `--fg`, a 1.5 px border in the colour of the button, a small light that travels along the border (a conic gradient whose angle is animated, still under reduced motion), depth from a tinted shadow, the colour filling the face on hover, a 2 px press on `:active` and a 2 px focus outline in the border colour. All six share one layout: a 22 px filled mark (an open book for Getting Started, a container for the local Docker guide, two stacked server units for the VPS guide, the system logo for a download), the label and a second line (`Read Documentation`, or `ALPHA Release vX.Y.Z` from the latest release read at build time), 56 px high. On the home they sit below the hero and in the closing section (right under the question What are you waiting?), spread with `justify-content: space-between`: one row from 1440 px, three per row below it, and one per row at the same width below 40rem; Setup Docker Local links to `/docs/self-hosting/docker/` and Setup Docker VPS to `/docs/self-hosting/vps/`. Each button has its own colour:

| Button | Fill on hover (`--c`) | Border light / dark (`--c-edge`) | Label on the fill (`--c-on`) | Label on the fill |
| --- | --- | --- | --- | --- |
| Getting Started | `--cta` `oklch(0.7 0.2 48)` `#FC6D00` | `--cta` / `--cta` | `oklch(0.145 0 0)` | 6.91:1 |
| Setup Docker Local | grape `#A21CAF` | `#A21CAF` / `#E879F9` | white | 6.32:1 |
| Setup Docker VPS | `oklch(0.5 0.17 252)` `#0062BF` | `oklch(0.5 0.17 252)` / `oklch(0.65 0.15 250)` | white | 5.97:1 |
| Windows | emerald `#047857` | `#047857` / `#34D399` | white | 5.48:1 |
| macOS | hot pink `#FF1F8E` (about `oklch(0.65 0.26 0)`) | `#FF1F8E` / `#FF1F8E` | `oklch(0.145 0 0)` | 5.50:1 |
| Linux | `oklch(0.84 0.15 85)` `#F7C243` | `oklch(0.72 0.15 75)` / `oklch(0.84 0.15 85)` | `oklch(0.145 0 0)` | 12.01:1 |

At rest the label is `--fg` on `--bg` (above 15:1 in both themes) and the version line `--muted` on `--bg` (7.9:1 light, 7.7:1 dark); on hover the version line takes the label colour. The border is decorative: the label identifies the control. The macOS pink takes near-black text on its fill because white on `#FF1F8E` is only 3.60:1; its border is 3.60:1 against the white page and 5.50:1 against the dark one.

**Navbar hover (docs and landing).** The links and buttons of the main navbar take `--nav-hover` under the pointer and on keyboard focus, a vivid violet that says the item can be clicked: light `#8A00D4` (6.93:1 on white, 6.35:1 on the `--hover` fill), dark `#C060FF` (5.99:1 on the dark page, 5.27:1 on the `--hover` fill), both above 4.5:1 for normal text. The rule is one selector in `site/app/globals.css` (`header[data-variant] :is(a, button):is(:hover, :focus-visible)`), so the home and the docs share it.

### 11.7 Radii, borders and shadows

- Radii: the scale of section 2.9 (6, 8, 12, 16 and 24 px). The expressive register adds `rounded-3xl` (24 px) for large showcase frames (the product window of the hero) and `rounded-full` for pills.
- Borders: as section 2.10. On dark expressive surfaces a hairline `border-white/10` frames floating panels.
- Shadows: the product keeps the elevation of section 2.11. The expressive register adds one brand glow for showcase frames only, `0 30px 80px -20px oklch(0.442 0.222 272.9 / 45%)`, and no other coloured shadow.

### 11.8 Motion

The product keeps section 2.13. The expressive register adds:

| Token | Value | Use |
| --- | --- | --- |
| `--ease-out-expo` | `cubic-bezier(0.22, 1, 0.36, 1)` | entrances, scene changes |
| `--ease-in-out` | `cubic-bezier(0.65, 0, 0.35, 1)` | camera moves |
| `--duration-scene` | 700 ms (500 to 900) | the change between two scenes of the viewport version |
| `--duration-reveal` | 500 ms, 60 ms stagger | text and cards entering on scroll |
| `--duration-hover` | 150 ms | hover and press |

`prefers-reduced-motion: reduce` turns every entrance and scene change into a 150 ms crossfade, stops the camera and the background on their first frame, and disables scroll-linked effects. Motion never carries information alone.

**The home (epic 88).** The home has no looping or automatic motion: its sections and cards settle in once as they scroll into view (a scroll-driven `animation-timeline: view()`, only under `prefers-reduced-motion: no-preference` and where the browser supports it), so it needs no pause control.

**The pause control (WCAG 2.2.2, D-126).** The two backups of the landing (`/landing/classic`, `/landing/creative`) carry one small round button fixed in the bottom-right corner (`MotionToggle.astro`; 36 px, an icon, `aria-pressed`, accessible name "Pause animations" or "Play animations", a 2 px focus ring). Pressing it sets `html[data-motion="paused"]`: CSS then runs no animation (`animation: none !important`, so an entrance ends in its final state, the light of the buttons rests at a fixed angle, the two product pictures stop on the one of the current theme), the hero typer shows phrase 1 complete, and the WebGL stage stops its loop and keeps the last frame. Pressing it again resumes. The choice is kept in `localStorage` (`stuarty-motion`, reads and writes in try/catch) and applies on the next visit before the first paint. Under `prefers-reduced-motion` the button shows as paused and is disabled. Docs pages do not have it.

### 11.9 3D and the living background (expressive register only)

- three.js 0.187.0 (r187, the current stable release), one renderer per page, `devicePixelRatio` capped at 2 (1.5 on small screens), the render loop paused when the tab is hidden or the canvas is off screen, loaded as a client-only island after the text has painted.
- The living background: one full-screen fragment shader, a slow noise flow of brand 700 and 500 on the background of the theme, the pointer and the scroll velocity adding a gentle disturbance; at most 10% of the frame budget.
- The mascot: the icon in 3D from primitives (a rounded box in 700, the clapper bar in 500, two capsule eyes in white), no model file; it may blink, follow the pointer and snap the clapper shut on a scene change.
- A WebGL failure, a very slow device or reduced motion falls back to a static brand gradient and a still image of the mascot. All text is HTML over the canvas, never drawn in WebGL.

### 11.10 Layout of the expressive register

- Content width `max-w-6xl` (72 rem) with 24 px gutters (`px-6`), 16 px below `sm`; the hero and showcase frames may bleed to the full width.
- A 12-column grid from `lg`; sections separated by 128 px (`py-32`) from `lg`, 80 px below.
- One idea per section: an eyebrow, a title, a lead of at most two lines, then the proof (a clip, a code sample, a card grid).

**The documentation layout.** The site container is 1440 px (`--site-container`, 30 px gutters from `sm`), shared by the home and the documentation. The documentation root font is 125% (20 px), so 1 rem is 20 px there. Inside the container:

| Part | Width | Source |
| --- | --- | --- |
| Left sidebar | `w-52` (13 rem, 260 px), `pr-4`, items 0.8125 rem (16.25 px) with `px-2.5` | `src/components/DocPage.astro`, `components/sidebar.tsx`, `components/changelog-page.tsx` |
| Header left column | 13 rem, the same as the sidebar, so the search starts above the content | `components/header.tsx` (`lg:grid-cols-[13rem_minmax(0,1fr)_auto]`) |
| Gap before the content | `lg:pl-6` (30 px) | `components/doc-article.tsx` |
| Content column | the rest: 704 px at 1024, 720 px at 1280, 880 px at 1440 and above | `components/doc-article.tsx` |
| Gap before the table of contents | `gap-8` (40 px) | `components/doc-article.tsx` |
| On this page (table of contents) | `w-40` (10 rem, 200 px), from `xl` (1280 px) | `components/doc-article.tsx` |
| Reference samples (wide pages, no table of contents) | `minmax(0, 26rem)` from `xl` | `components/mdx/reference.tsx` |

Type sizes of the documentation (1 rem = 20 px):

| Text | Size | Source |
| --- | --- | --- |
| Body (`.prose`) | 0.875 rem (17.5 px), line height 1.75 | `app/globals.css` |
| Headings h2 / h3 / h4 | 1.3 rem / 1 rem / 0.875 rem | `app/globals.css` |
| Tables | 0.75 rem (15 px), line height 1.6 | `app/globals.css` |
| Code blocks (Shiki) | 0.75 rem (15 px), line height 1.7; inline code 0.82 em | `app/globals.css` |
| Sidebar items, On this page | 0.8125 rem (16.25 px) | `components/sidebar.tsx`, `components/toc.tsx` |
| Language tabs of a code sample | 0.75 rem mono (`text-xs`), `px-1.5` | `components/mdx/tabs-client.tsx` |

The colours do not change, so every pair keeps its contrast (body `--muted` on `--bg` above 7:1 in both themes). The samples column of a reference page scrolls on its own with no edge fade (`.scroll-nofade`), and the language tabs scroll sideways without a scrollbar and stop before the copy button.

Prose (paragraphs, lists, quotes, headings, steps) is capped at `75ch` (`.prose` in `app/globals.css`); measured lines run 61 to 62 characters at 1024 and 1280 px and reach the 75-character cap at 1440 and 1920 px. Code blocks and tables use the whole content column, and a table wider than it scrolls inside its own frame, never the page. A name in the first column of a table does not break at its hyphens. The API Reference and MCP links of the header appear from `xl`; between `lg` and `xl` they stay in the sidebar, so the header never overflows at 1024 px.

### 11.11 Marketing components

| Component | Built from | Notes |
| --- | --- | --- |
| Navbar | the documentation `Header` (structure and behaviour kept: the mark, the name, search on Ctrl+K, the links, the theme toggle) | translucent `bg-background/70` with `backdrop-blur` over a scene, a hairline bottom border |
| Hero | display title, lead, `brand` and `outline` buttons, a showcase frame | the frame holds a screenshot or a clip placeholder |
| Feature card | `Card` (`rounded-xl`, `--card`) with a lucide icon in a `--brand-subtle` square | one clip placeholder per card |
| Code tabs | Base UI `Tabs` and a code block with a copy button | Agent, CLI, HTTP API, MCP; the code is copied from the build |
| Terminal | a `font-mono` panel on `oklch(0.145 0 0)` with a typed reveal (instant with reduced motion) | real commands only |
| Media placeholder | a `rounded-2xl` frame with the brand gradient and a play glyph, in the aspect ratio of the real asset | replaced by the real clip later |
| Scene rail (viewport version) | a vertical list of buttons with names, `aria-current` on the active scene | reachable with the keyboard |

### 11.12 Iconography

lucide (section 5), 1.5 px stroke at 16 and 20 px, 2 px at 24 px and above. Icons in the expressive register sit in a 40 px `--brand-subtle` rounded square, coloured `--brand-text`. No emoji in the interface; no icon without a text label or an accessible name.

### 11.13 References that shaped this section

Researched on 2026-11-20 (sources in the brainstorming document, section 3): real product interfaces as the hero instead of screenshots (Linear); a simulated agent chat with visible tool steps and tabs of previews (Raycast); scroll-led 3D storytelling with an explicit "scroll to explore" cue (Lusion); one typeface family for sans and mono (Vercel Geist); tokens in CSS through `@theme`, OKLCH colours and OKLab gradients (Tailwind CSS v4).

### 11.14 Applying the tokens

1. `web/resources/css/app.css`: set `--brand` and `--brand-foreground` per theme as section 11.3, add `--brand-accent`, `--brand-subtle` and `--brand-text` to `:root`, `.dark` and `@theme inline`; add the `brand` button variant; move the sidebar mark and the active navigation item to the brand tokens. `bun run check:contrast` gets the new pairs; nothing in `editor/**` changes.
2. `site/app/globals.css`: the same tokens, so the documentation and the landing share them.
3. The desktop shell has no styles of its own (principle P6): it follows `app.css`.
4. The landing versions (`/landing/viewport`, `/landing/classic`, `/landing/creative`) use the expressive register; the chosen one becomes the home.
