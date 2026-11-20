# Stuarty design system

- Status: Accepted (2026-11-14). Written by the designer, to be reviewed by the architect and by senior-e (editor UI).
- Applies to: the web UI under `web/resources/` (the Inertia pages and the editor), the desktop shell (`desktop/`, ADR-0023), and, where section 8.4 says so, the documentation site (`site/`, SDD-010).
- Sources of truth, which this document follows and never overrides: author decisions D-008, D-013, D-016, D-022, D-023, D-024, D-028, D-029; [SDD-012](sdds/012-ui-architecture.md) (editor UI architecture: §1 thin client, §3 kept front end, §6 layout, §7 timeline, §11 preview, §12 inspector, §16 i18n, §17 theme, §18 accessibility, §21 desktop); [SDD-002](sdds/002-command-catalogue.md) A.4 (time strings) and A.8 (the error envelope); [SDD-003](sdds/003-agent-interfaces.md) §1.4; [SDD-010](sdds/010-documentation-site.md) §1 and §10 (site shell, glossary); [ADR-0007](adrs/0007-thin-ui-client.md), [ADR-0012](adrs/0012-text-and-shape-rasterization.md), [ADR-0013](adrs/0013-browser-preview-stack.md), [ADR-0023](adrs/0023-desktop-shell-deferred.md); [PRD-001](prds/001-video-editor.md) §3 (use cases), RF-070 to RF-075.
- Code it describes: `web/resources/css/app.css` (the only token file), `web/components.json` (kept style record: base colour `neutral`, icon library `lucide`), `web/resources/js/components/ui/*` (`@base-ui/react`), `web/resources/js/layouts/AppLayout.tsx`, and the editor tree of SDD-012 §3.2.

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
| P2 | Content first | The picture and the timeline are the largest and most saturated things on screen. Chrome is neutral grey; colour in chrome is reserved for state (focus, selection, error, warning, success, agent activity). The preview surround is a neutral dark grey in both themes | D-024, SDD-012 §17 item 4 |
| P3 | Minimalist editor | One way to reach each command plus the palette; no decorative illustration, gradients, glass effects or interface animation beyond the two kinds of section 2.13. Fewer, larger regions beat many small panels | D-013, SDD-012 §17 item 5 |
| P4 | Agent actions are visible and attributed | Any change made by another actor shows who made it (actor label, `--agent` colour, icon), where (outlined items), and when (relative time). The UI never hides or merges another actor's work into "you" | RF-118, SDD-012 §5.8, §15 |
| P5 | Errors are the core's words | An error shows a localized headline for its code, then the core's `message` and `hint` exactly as received. The UI never paraphrases or invents a cause | SDD-002 A.8, SDD-012 §4.4, rule T9 |
| P6 | One look in browser and desktop | The desktop shell loads the same web UI with the same tokens, fonts and components. Nothing may depend on a browser-only or webview-only feature outside SDD-012 §21.1 | ADR-0023, SDD-012 §21 |
| P7 | Accessible by construction | Keyboard first, visible focus, names on every control, WCAG 2.2 AA contrast in both themes, nothing conveyed by colour alone | RF-075, SDD-012 §18 |
| P8 | Local and quiet | No asset, font or icon is loaded from another origin (fonts are bundled through `@fontsource`; icons are inline SVG from `lucide-react`). No telemetry, no "what's new" calls | D-016, RF-206, SDD-012 §2.2 (CSP) |

## 2. Design tokens

### 2.1 Where tokens live and how they are named

All tokens are CSS custom properties in `web/resources/css/app.css`, defined in `:root` (light) and overridden in `.dark` (dark), and exposed to Tailwind 4 through the existing `@theme inline` block so components use utilities (`bg-background`, `text-muted-foreground`, `bg-ed-panel`, `border-ed-track-line`). Dark is applied by the `dark` class on `<html>` (`@custom-variant dark (&:where(.dark, .dark *))`, kept), set before paint from `localStorage.theme`, dark when absent (SDD-012 §17 item 1).

Three groups, three prefixes:

| Group | Prefix of the CSS variable | Tailwind 4 name | Who uses it |
| --- | --- | --- | --- |
| shadcn base | none (`--background`, `--primary`, ...), names kept exactly as shadcn generated them | `--color-<name>` in `@theme inline` (already present) | every page and the editor; the kept components in `components/ui/` |
| product semantic (new) | none: `--success`, `--warning`, `--info`, `--agent`, `--brand` and their `-foreground` | `--color-success`, ... added to `@theme inline` | every page and the editor |
| editor (SDD-012 §17) | `--ed-` | `--color-ed-<name>` added to `@theme inline` | `web/resources/js/editor/**` only |

A raw Tailwind palette class (`bg-indigo-600`, `text-red-500`) or a literal colour in a component is not allowed; `AppLayout.tsx` currently uses `bg-indigo-600 text-white` for the mark and moves to `bg-brand text-brand-foreground` (section 5.3).

Values are OKLCH, as in the existing file. The column "Changed" says whether this document changes a value that already exists in `app.css` or in the SDD-012 §17 table, and why. Every value was checked with the WCAG 2.x formula after converting OKLCH to sRGB (section 2.6).

### 2.2 Neutral scale

The neutral scale is the Base UI editor palette (`components.json` records `style: base-ui`). It is a reference scale: components do not use these steps directly, the semantic tokens below are defined from them. Hex values are the sRGB rendering, for reading only.

| Step | OKLCH | Hex | Semantic tokens that use it |
| --- | --- | --- | --- |
| 0 | `oklch(1 0 0)` | `#ffffff` | light `--background`, `--card`, `--popover`, `--ed-panel` |
| 50 | `oklch(0.985 0 0)` | `#fafafa` | light `--sidebar`; dark `--foreground` |
| 100 | `oklch(0.97 0 0)` | `#f5f5f5` | light `--muted`, `--secondary`, `--accent`, `--ed-bg` |
| 150 | `oklch(0.95 0 0)` | `#eeeeee` | light `--ed-timeline` |
| 200 | `oklch(0.922 0 0)` | `#e5e5e5` | light `--border`; dark `--primary` |
| 400 | `oklch(0.708 0 0)` | `#a1a1a1` | dark `--muted-foreground` |
| 450 | `oklch(0.62 0 0)` | `#868686` | light `--input`, `--ed-track-line` |
| 500 | `oklch(0.556 0 0)` | `#737373` | (was light `--muted-foreground` and dark `--ring`) |
| 550 | `oklch(0.50 0 0)` | `#636363` | light `--muted-foreground`; dark `--ed-track-line` |
| 800 | `oklch(0.269 0 0)` | `#262626` | dark `--muted`, `--secondary`, `--accent` |
| 850 | `oklch(0.2 0 0)` | `#161616` | dark `--ed-panel`; `--ed-preview-surround` (both themes) |
| 900 | `oklch(0.205 0 0)` | `#171717` | light `--primary`; dark `--card`, `--popover` |
| 925 | `oklch(0.18 0 0)` | `#121212` | dark `--ed-timeline` |
| 950 | `oklch(0.145 0 0)` | `#0a0a0a` | light `--foreground`; dark `--background` |

### 2.3 Colour: surfaces, text, borders and focus (shadcn base)

| Token (Tailwind) | Light | Dark | Use | Changed |
| --- | --- | --- | --- | --- |
| `--background` (`bg-background`) | `oklch(1 0 0)` | `oklch(0.145 0 0)` | page background of the Inertia pages | no |
| `--foreground` (`text-foreground`) | `oklch(0.145 0 0)` | `oklch(0.985 0 0)` | primary text | no |
| `--card`, `--card-foreground` | `oklch(1 0 0)`, `oklch(0.145 0 0)` | `oklch(0.205 0 0)`, `oklch(0.985 0 0)` | cards (project cards, settings groups) | no |
| `--popover`, `--popover-foreground` | `oklch(1 0 0)`, `oklch(0.145 0 0)` | `oklch(0.205 0 0)`, `oklch(0.985 0 0)` | popovers, menus, dialogs, toasts, select lists | no |
| `--primary`, `--primary-foreground` | `oklch(0.205 0 0)`, `oklch(0.985 0 0)` | `oklch(0.922 0 0)`, `oklch(0.205 0 0)` | the primary action: one per view (Export, Create project, Start export, Run). Neutral on purpose (P2): the product accent is not a hue | no |
| `--secondary`, `--secondary-foreground` | `oklch(0.97 0 0)`, `oklch(0.205 0 0)` | `oklch(0.269 0 0)`, `oklch(0.985 0 0)` | secondary buttons, the "you" actor chip | no |
| `--muted`, `--muted-foreground` | `oklch(0.97 0 0)`, **`oklch(0.50 0 0)`** | `oklch(0.269 0 0)`, `oklch(0.708 0 0)` | quiet surfaces; secondary text (meta lines, labels, placeholders, ruler labels) | **yes, light `--muted-foreground` from `0.556` to `0.50`**: `0.556` gave 4.34:1 on `--muted` and 4.09:1 on `--ed-timeline`, below 4.5:1 |
| `--accent`, `--accent-foreground` | `oklch(0.97 0 0)`, `oklch(0.205 0 0)` | `oklch(0.269 0 0)`, `oklch(0.985 0 0)` | hover and highlighted rows in menus and lists (shadcn meaning: a surface, not a brand colour) | no |
| `--destructive` | **`oklch(0.55 0.22 27.325)`** | `oklch(0.704 0.191 22.216)` | danger: error text and icons, destructive buttons (`bg-destructive/10 text-destructive`), failed state | **yes, light from `oklch(0.577 0.245 27.325)`**: the old value gave 4.37:1 on `--muted` and 3.99:1 on its own 10% tint used by the destructive button |
| `--border` (`border-border`) | `oklch(0.922 0 0)` | `oklch(1 0 0 / 10%)` | dividers between regions, card and panel edges. Decorative only: never the only cue that identifies a control | no (exempt from 3:1 because decorative; section 7.6) |
| `--input` (`border-input`) | **`oklch(0.62 0 0)`** | **`oklch(1 0 0 / 40%)`** | the boundary of text inputs, selects, text areas, checkboxes: the cue that identifies the field | **yes, both**: light `0.922` gave 1.26:1 and dark `/ 15%` gave 1.47:1 against the background; a field boundary needs 3:1 (WCAG 1.4.11). The dark fill `bg-input/30` becomes 12% white, still dark |
| `--ring` (`ring-ring`, `outline-ring`) | **`oklch(0.45 0.18 250)`** | **`oklch(0.85 0.12 230)`** | the focus indicator of every focusable element (section 2.10), the only interactive accent hue | **yes, both**: light `0.708` gave 2.59:1 and dark `0.556` gave 4.18:1 solid but 1.87:1 at the 50% the components use. The new values equal `--ed-selection`, so focus looks the same in pages and editor (SDD-012 §18.1 item 6) |
| `--sidebar`, `--sidebar-foreground`, `--sidebar-primary`, `--sidebar-primary-foreground`, `--sidebar-accent`, `--sidebar-accent-foreground`, `--sidebar-border` | kept | kept | the sidebar of `AppLayout` | no |
| `--sidebar-ring` | **as `--ring`** | **as `--ring`** | focus in the sidebar | **yes**, same reason as `--ring` |
| `--chart-1` to `--chart-5` | kept | kept | not used by the product; kept because shadcn components may reference them | no |

### 2.4 Colour: semantic states, agent attribution and brand (new)

Each state has one token usable as text and icon colour on `--background`, `--card`, `--popover`, `--muted` and `--ed-panel`, and as a tinted surface with the same pattern as shadcn's destructive (`bg-<state>/10` in light, `bg-<state>/20` in dark, with `text-<state>`). There is no solid fill for states except `--brand`.

| Token (Tailwind) | Light | Dark | Use |
| --- | --- | --- | --- |
| `--success` (`text-success`) | `oklch(0.50 0.13 150)` | `oklch(0.75 0.15 150)` | export succeeded, asset ready, connection back to live (shown once) |
| `--warning` (`text-warning`) | `oklch(0.52 0.12 70)` | `oklch(0.80 0.14 80)` | lint warnings, `reconnecting`, small-window notice, missing-media icon on an item, conflict notice |
| `--info` (`text-info`) | `oklch(0.50 0.15 250)` | `oklch(0.75 0.12 240)` | lint notes, `resyncing`, neutral notices that need attention |
| `--danger` | alias of `--destructive` | alias of `--destructive` | the name used in this document for the error state; code uses `destructive` (shadcn name kept) |
| `--agent` (`text-agent`, `bg-agent/10`) | `oklch(0.50 0.10 190)` | `oklch(0.78 0.11 190)` | attribution of another actor: the activity dot and label, actor chips of agents in history and notices, "changed by `<actor>`" notes under inspector fields, the 2 px remote-edit outline on timeline items (D-030) |
| `--brand`, `--brand-foreground` | `oklch(0.511 0.262 276.966)`, `oklch(0.985 0 0)` | same as light | the product mark placeholder only (section 5.3) and the documentation site's accent; never a button or a state |

Hues are spread so no state is mistaken for an item type: agent teal (190) sits between audio green (150) and clip blue (250); the playhead and danger are red (25 to 27), which no item type uses.

**Decision of this document.** `--agent` is a new token, used for every attribution label and chip and for the 2 px remote-edit outline on timeline items (D-030, SDD-012 §5.8 item 4 and §17), so a remote edit is not mistaken for a selection, which keeps `--ed-selection`. As an outline it meets 3:1 against `--ed-timeline` (4.82 light, 9.84 dark) and against every item fill (3.00 to 3.39 light, 3.22 to 4.43 dark).

### 2.5 Colour: editor (`--ed-*`)

Names and initial values are those of SDD-012 §17. Values changed here failed the contrast pairs of SDD-012 §18.3.

| Token | Light | Dark | Use | Changed |
| --- | --- | --- | --- | --- |
| `--ed-bg` | `oklch(0.97 0 0)` | `oklch(0.16 0 0)` | editor surround (grid gaps, behind panels) | no |
| `--ed-panel` | `oklch(1 0 0)` | `oklch(0.2 0 0)` | library, inspector, top bar, timeline toolbar | no |
| `--ed-timeline` | `oklch(0.95 0 0)` | `oklch(0.18 0 0)` | timeline background | no |
| `--ed-track-line` | **`oklch(0.62 0 0)`** | **`oklch(0.50 0 0)`** | row separators, ruler ticks, the 1 px border of item blocks | **yes, both**: `0.88` (light) gave 1.24:1 and `0.28` (dark) 1.29:1 against `--ed-timeline`; SDD-012 §18.3 requires 3:1 |
| `--ed-item-clip`, `--ed-item-clip-fg` | `oklch(0.80 0.08 250)`, `oklch(0.2 0 0)` | `oklch(0.42 0.12 250)`, `oklch(0.98 0 0)` | clip blocks (video and image) | no |
| `--ed-item-audio`, `-fg` | `oklch(0.80 0.08 150)`, `oklch(0.2 0 0)` | `oklch(0.42 0.10 150)`, `oklch(0.98 0 0)` | audio blocks | no |
| `--ed-item-text`, `-fg` | `oklch(0.84 0.10 80)`, `oklch(0.2 0 0)` | `oklch(0.50 0.12 70)`, `oklch(0.98 0 0)` | text blocks | no |
| `--ed-item-caption`, `-fg` | `oklch(0.82 0.08 300)`, `oklch(0.2 0 0)` | `oklch(0.44 0.14 300)`, `oklch(0.98 0 0)` | caption blocks | no |
| `--ed-item-shape`, `-fg` | `oklch(0.82 0.08 350)`, `oklch(0.2 0 0)` | `oklch(0.46 0.13 350)`, `oklch(0.98 0 0)` | shape blocks | no |
| `--ed-playhead` | `oklch(0.55 0.22 25)` | `oklch(0.70 0.20 25)` | playhead line and handle | no |
| `--ed-selection` | `oklch(0.45 0.18 250)` | `oklch(0.85 0.12 230)` | selection ring, focus ring in the timeline (SDD-012 §17); the remote-edit outline uses `--agent` (D-030) | no |
| `--ed-snap` | `oklch(0.50 0.15 90)` | `oklch(0.85 0.15 90)` | snap line | no |
| `--ed-ghost` | **`oklch(0.45 0.18 250 / 0.75)`** | `oklch(0.85 0.12 230 / 0.5)` | ghost outline and fill (fill at a further 15%) of a pending gesture | **yes, light alpha `0.5` to `0.75`**: `0.5` gave 2.33:1 against `--ed-timeline` |
| `--ed-safe-area` | `oklch(0.50 0.15 90)` | `oklch(0.80 0.15 90)` | safe-area guide | no (light passes at 3.03:1 on the surround, the smallest margin of the set; see section 10) |
| `--ed-error-flash` | `oklch(0.55 0.22 25)` | `oklch(0.70 0.20 25)` | outline of the item named by an overlap or conflict | no |
| `--ed-preview-surround` | `oklch(0.2 0 0)` | `oklch(0.2 0 0)` | the preview region around the canvas, identical in both themes | no |
| `--ed-handle` (new) | `oklch(0.85 0.12 230)` | `oklch(0.85 0.12 230)` | selection box, transform handles and the focus ring of elements drawn over the preview surround | **new**: the handles sit on `--ed-preview-surround`, which is dark in both themes; light `--ed-selection` there gives 2.50:1 |

Text placed on `--ed-preview-surround` (banners inside the preview, the canvas label) uses `oklch(0.985 0 0)` and `oklch(0.708 0 0)` in both themes (17.34:1 and 6.98:1), since the surround does not change with the theme.

### 2.6 Contrast ratios

Method: OKLCH to linear sRGB (Björn Ottosson's matrices), gamut clamp, sRGB encoding, WCAG 2.x relative luminance; a colour with alpha is composited over the surface below it in sRGB, as the browser paints it. This is the algorithm the CI script `bun run check:contrast` of SDD-012 §18.4 implements; the numbers below are what that script must reproduce within 0.02. Thresholds: 4.5:1 text, 3:1 large text, UI components and graphical objects.

| Pair (foreground on background) | Need | Light | Dark |
| --- | --- | --- | --- |
| `foreground` on `background` | 4.5 | 15.35 | 17.57 |
| `foreground` on `ed-panel` | 4.5 | 16.89 | 16.07 |
| `muted-foreground` on `background` | 4.5 | 6.87 | 6.69 |
| `muted-foreground` on `muted` | 4.5 | 6.17 | 5.38 |
| `muted-foreground` on `ed-timeline` | 4.5 | 6.17 | 6.69 |
| `primary-foreground` on `primary` | 4.5 | 5.35 | 11.21 |
| `secondary-foreground` on `secondary` | 4.5 | 13.79 | 14.12 |
| `destructive` on `background` | 4.5 | 6.08 | 8.26 |
| `destructive` on its tint (`/10` light, `/20` dark) | 4.5 | 5.07 | 5.85 |
| `destructive` on its tint over `popover` (the destructive button of a dialog or a menu; it set the dark `--destructive` to `oklch(0.790 0.170 16.4)`, 4.34:1 at `0.750`) | 4.5 | 5.56 | 4.59 |
| `success` on `background` | 4.5 | 5.59 | 8.29 |
| `warning` on `background` | 4.5 | 6.37 | 8.80 |
| `info` on `background` | 4.5 | 4.86 | 10.44 |
| `agent` on `background` | 4.5 | 6.33 | 10.44 |
| `agent` on `ed-panel` | 4.5 | 6.97 | 9.55 |
| each state on its tint (`/10` light, `/20` dark over the panel) | 4.5 | 4.66 to 5.98 | 5.28 to 6.22 |
| `brand-foreground` on `brand` | 4.5 | 8.28 | 5.66 |
| `brand-text` on `background` | 4.5 | 7.66 | 9.65 |
| `brand-text` on `brand-subtle` over `background` (the active sidebar item) | 4.5 | 7.67 | 8.95 |
| `ring` on `background` | 3 | 4.86 | 10.44 |
| `ring` on `ed-timeline` | 3 | 4.37 | 10.44 |
| `input` on `background` | 3 | 6.87 | 6.69 |
| `ed-track-line` on `ed-timeline` | 3 | 6.17 | 6.69 |
| `ed-playhead` on `ed-timeline` | 3 | 14.48 | 18.88 |
| `ed-snap` on `ed-timeline` | 3 | 4.37 | 10.44 |
| `ed-ghost` on `ed-timeline` | 3 | 11.29 | 3.85 |
| `ed-error-flash` on `ed-timeline` | 3 | 3.83 | 5.14 |
| `ed-safe-area` on `ed-preview-surround` | 3 | 4.32 | 11.21 |
| `ed-handle` on `ed-preview-surround` | 3 | 4.32 | 11.21 |
| `ed-item-clip-fg` on `ed-item-clip` | 4.5 | 5.17 | 6.18 |
| `ed-item-audio-fg` on `ed-item-audio` | 4.5 | 5.02 | 5.98 |
| `ed-item-text-fg` on `ed-item-text` | 4.5 | 6.97 | 6.66 |
| `ed-item-caption-fg` on `ed-item-caption` | 4.5 | 6.03 | 6.69 |
| `ed-item-shape-fg` on `ed-item-shape` | 4.5 | 5.19 | 9.45 |
| `ed-selection` on `ed-item-clip` (ring against the fill it surrounds) | 3 | 5.17 | 6.18 |
| `ed-selection` on `ed-item-text` (the closest item colour) | 3 | 6.97 | 6.66 |

Item fills against `--ed-timeline` are 1.61:1 (light clip) to 2.22:1 (dark clip), below 3:1; this is why every item block has a 1 px `--ed-track-line` border (3.15:1 and 3.13:1), which carries the boundary of the block (section 4.3.6).

### 2.7 Typography

**UI typeface.** Geist Variable, already a dependency (`@fontsource-variable/geist` 5.3.0, imported in `app.css`), served from the same origin with the bundle (P8; the default policy `default-src 'self'` of SDD-012 §2.2 allows nothing else). It is the same in the browser and in the desktop webview because it is bundled, not a system font.

| Token (Tailwind) | Value | Changed |
| --- | --- | --- |
| `--font-sans` (`font-sans`) | `'Geist Variable', ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif` | **yes**: the fallback was only `sans-serif`; the stack keeps metrics close while the bundled file loads and if it ever fails |
| `--font-heading` | `var(--font-sans)` | no |
| `--font-mono` (`font-mono`) | Tailwind 4 default: `ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace` | no (Tailwind default, not redefined) |

**Fonts of the video are not UI fonts.** The six families of D-022 (Inter, Montserrat, Anton, Bebas Neue, Playfair Display, JetBrains Mono) are bundled in the Go binary for rasterizing text into the picture (ADR-0012). The browser never loads them and never lays out video text (SDD-012 rule T6): the inspector's font select lists the family names in the UI font, with no rendered samples (D-030).

**Timecodes and numbers.** Every time, duration, revision number, frame count, percentage and size uses `tabular-nums` (`font-variant-numeric: tabular-nums`) so digits do not jump while the playhead moves. Timecodes are set in `font-sans` with tabular figures, not in a monospace font: the mono stack above is a system font and differs between Windows, macOS and Linux, which would break P6. `font-mono` is used only for code-like content: file paths, command names, ids, error codes, the install command, diff text from the core.

**Decision of this document, confirmed by D-030.** Timecodes use Geist with `tabular-nums` instead of a monospace face. The digit-width test of SDD-012 §11.5 passes on the bundled Geist Variable: senior-e measured equal advance widths for the digits with tabular figures (24.48 px each at 40 px), so `@fontsource-variable/geist-mono` is not needed and is not added. D-030 approves it in advance only if that test ever fails.

Type scale (Tailwind 4 defaults, rem at 16 px):

| Role | Utility | Size / line height | Weight | Where |
| --- | --- | --- | --- | --- |
| Micro label | `text-xs` | 12 / 16 px | 500 | item labels in the timeline, ruler labels (canvas, drawn at 12 px), badges, meta lines, field labels in the inspector |
| UI text | `text-sm` | 14 / 20 px | 400; 500 for buttons and tabs | the default in the editor: buttons, inputs (from `md`), menus, toasts, table rows |
| Body | `text-base` | 16 / 24 px | 400 | Inertia page body text; inputs below the `md` breakpoint (avoids mobile zoom on focus, kept from shadcn) |
| Section title | `text-sm` | 14 / 20 px | 600 | inspector section headers, panel titles, dialog section titles |
| Dialog title | `text-lg` | 18 / 28 px | 600 | dialog and sheet titles |
| Page title | `text-xl` to `text-2xl` | 20 / 28 to 24 / 32 px | 600 | the projects and settings pages |
| Timecode, transport | `text-sm tabular-nums` | 14 / 20 px | 500 | transport row, top bar revision |

Rules: 12 px is the minimum size anywhere; weights are 400, 500 and 600 only; no italics in UI chrome; letter spacing stays default except `tracking-tight` on the product name (kept from `AppLayout`); text never sits on a picture without a solid surface behind it.

### 2.8 Spacing

The Tailwind 4 spacing unit stays `--spacing: 0.25rem` (4 px). Only these steps are used, so the grid stays regular:

| Step | px | Typical use |
| --- | --- | --- |
| `0.5` | 2 | gap between a ring and its element, keyframe diamond inset |
| `1` | 4 | gap inside a toolbar group, icon to label inside a badge |
| `1.5` | 6 | icon to label inside a button (shadcn), item label padding |
| `2` | 8 | gap between controls, field rows, card internal gap |
| `3` | 12 | panel padding in the editor, top bar horizontal padding |
| `4` | 16 | gap between inspector sections, preview padding (SDD-012 §11.4), page padding on small screens |
| `6` | 24 | page padding from `md` (kept from `AppLayout`), dialog padding |
| `8` | 32 | gap between page sections |
| `12` | 48 | empty-state vertical padding |

### 2.9 Radii

`--radius: 0.625rem` (10 px) is kept, with the derived sizes of `@theme inline`.

| Token (Tailwind) | Value | Use |
| --- | --- | --- |
| `--radius-xs` (`rounded-xs`), **new in `@theme inline`** | `calc(var(--radius) * 0.4)` = 4 px | timeline item blocks, caption word segments, keyframe diamonds' container, progress bars |
| `--radius-sm` | 6 px | small buttons (`xs`, `sm` sizes use `min(var(--radius-md), 10px)`), asset thumbnails |
| `--radius-md` | 8 px | tooltips, menu items |
| `--radius-lg` | 10 px | buttons, inputs, selects, toasts (shadcn `rounded-lg`) |
| `--radius-xl` | 14 px | cards, dialogs, popovers |
| `--radius-4xl` | 26 px | badges and chips (pill) |
| none | 0 | the preview canvas (the picture is shown with its own edges), the panels of the editor shell (they meet edge to edge) |

**Decision of this document.** `--radius-xs` is redefined from Tailwind's default 2 px to 4 px, following the `calc(var(--radius) * n)` pattern of the other sizes, because timeline blocks 40 to 64 px high read better with 4 px corners.

### 2.10 Borders, rings and outlines

| Element | Width and style | Colour | Note |
| --- | --- | --- | --- |
| Region divider, card edge | 1 px solid | `--border` | decorative |
| Field boundary | 1 px solid | `--input` | identifies the control, 3:1 |
| Focus indicator (everything focusable) | `outline: 2px solid var(--ring); outline-offset: 2px` | `--ring` | 3:1 against the background in both themes. The shadcn halo (`ring-3 ring-ring/50` and `border-ring`) stays as extra decoration |
| Focus over the preview surround | same | `--ed-handle` | |
| Focused timeline cell | `outline: 2px solid; outline-offset: 1px` | `--ed-selection` | offset 1 px so it does not overlap the next row |
| Selected item | 2 px inset ring (`box-shadow: inset 0 0 0 2px`) | `--ed-selection` | selected and focused can show together: ring inside, outline outside |
| Item block edge | 1 px solid | `--ed-track-line` | carries the boundary of the block |
| Pending ghost | 2 px dashed | `--ed-ghost` | fill `--ed-ghost` at 15% |
| Remote-edit highlight | 2 px solid, 1.5 s | `--agent` | SDD-012 §5.8 item 4 and D-030; class toggle, not an animation |
| Conflict or overlap highlight | 2 px solid, 2 s | `--ed-error-flash` | SDD-012 §8.4 |
| Snap line | 1 px solid, full height | `--ed-snap` | |
| Safe-area guide | 1 px dashed (6 px dash, 4 px gap) | `--ed-safe-area` | band outside it: black at 40% |

**Why the focus rule lives in `app.css`.** The kept components carry `outline-none` and a 50% ring (`focus-visible:ring-3 focus-visible:ring-ring/50`), which no ring colour can bring to 3:1 against white at 50% alpha. SDD-012 §3.1 keeps `components/ui/*` unchanged, so the solid outline is added in `app.css` as an **unlayered** rule (outside every `@layer`), which wins over Tailwind's utilities layer without touching the components:

```css
:where(a, button, input, select, textarea, summary, [tabindex], [contenteditable="true"], [data-slot]):focus-visible {
	outline: 2px solid var(--ring);
	outline-offset: 2px;
}
```

The rule above is the contract; the senior who implements it may narrow the selector, not weaken the outline.

### 2.11 Elevation

The editor is flat: regions meet edge to edge, separated by `--border`, with no shadow. Elevation exists only for things that float over content.

| Level | What | Light | Dark |
| --- | --- | --- | --- |
| 0 | page, panels, timeline, cards on a page | no shadow; `--border` edge | same; surface steps (`--ed-bg` 0.16, `--ed-timeline` 0.18, `--ed-panel` 0.2) do the separating |
| 1 | tooltip, popover, menu, select list, toast, the activity popover | `shadow-md` plus 1 px `--border` | `shadow-md` (barely visible) plus 1 px `--border`, on `--popover` (0.205), lighter than what is under it |
| 2 | dialog, sheet, command palette | `shadow-lg` plus 1 px `--border`; backdrop `bg-black/10` (shadcn sheet, kept) for sheets, `bg-black/50` for modal dialogs | same |

In dark, "higher" means a lighter surface, never a stronger shadow.

### 2.12 Z-index layers

Global layers (Tailwind `z-*`):

| Layer | Value | What |
| --- | --- | --- |
| base | `z-0` | page and editor regions |
| splitters | `z-10` | `ui/Splitter.tsx` hit areas over region edges |
| sticky page chrome | `z-20` | the sidebar rail (kept value) |
| in-page banners | `z-30` | offline, unsupported, read-only banners pinned to the top of a region |
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

D-013 and SDD-012 §17 item 5: interface animation is limited to the two kinds below (P3-UI-003), **and nothing else.** The tokens below are the one set of values for them and for the animations shadcn components carry.

| Token | Value | Use now |
| --- | --- | --- |
| `--duration-none` | `0ms` | every editor component |
| `--duration-fast` | `100ms` | already carried by shadcn select and sheet close (`duration-100`) |
| `--duration-base` | `150ms` | `.ed-motion` state transitions  |
| `--duration-slow` | `200ms` | `.ed-highlight` ; already carried by shadcn sheet open and sidebar collapse (`duration-200`) |
| `--ease-standard` | `cubic-bezier(0.2, 0, 0, 1)` | `.ed-motion` and `.ed-highlight` ; shadcn components keep their own easing |

Rules:

1. Editor components add no `transition`, `animation` or `@keyframes` of their own. Motion comes from two classes of `app.css`: `.ed-motion` (colour, border, shadow, opacity and outline colour fade in `--duration-base`; position and size never transition, so a drag, a zoom or a row resize never lags the pointer) on timeline items, rows, track headers and the row splitter, and `.ed-highlight` (the outline of a remote edit or a conflict eases in over `--duration-slow`). Timed highlights (remote-edit outline 1.5 s, conflict outline 2 s, activity "is editing" 3 s) stay class toggles with a timer; the class only adds the ease-in. A unit test (`web/scripts/motion.test.ts`) fails on any other `transition`, `animate-` or `duration-` class in an editor component.
2. No spinner rotates. A running job shows a determinate bar; an unknown duration shows a static icon and text ("Waiting for the server").
3. Scrolling is never smooth: page-follow of the playhead jumps (SDD-012 §7.5), `scroll-behavior` stays `auto`.
4. `prefers-reduced-motion: reduce` turns off the two classes above and what the shared primitives carry, with this rule in `app.css`, after them. Transitions are off at `0s`; animations keep `0.01ms` because popup presence waits for `animationend`, which an animation of exactly `0s` does not fire in every engine:

```css
@media (prefers-reduced-motion: reduce) {
	*, ::before, ::after {
		animation-duration: 0.01ms !important;
		animation-iteration-count: 1 !important;
		transition-duration: 0s !important;
		transition-delay: 0s !important;
	}
}
```

### 2.14 Summary of the changes to `app.css`

For the implementer (senior-e), in one place. Nothing here is implemented by this document.

1. `:root`: `--muted-foreground: oklch(0.50 0 0)`; `--destructive: oklch(0.55 0.22 27.325)`; `--input: oklch(0.62 0 0)`; `--ring` and `--sidebar-ring: oklch(0.45 0.18 250)`.
2. `.dark`: `--input: oklch(1 0 0 / 40%)`; `--ring` and `--sidebar-ring: oklch(0.85 0.12 230)`.
3. Both: add `--success`, `--warning`, `--info`, `--agent`, `--brand`, `--brand-foreground` (section 2.4) and the `--ed-*` group (section 2.5, with the changed `--ed-track-line`, light `--ed-ghost` and the new `--ed-handle`).
4. `@theme inline`: map every new variable to `--color-<name>`; add `--radius-xs: calc(var(--radius) * 0.4)`; extend `--font-sans` with the fallback stack; add the motion tokens as plain custom properties.
5. Add the unlayered focus rule (section 2.10), the reduced-motion rule (section 2.13) and the forced-colours rule (section 7.7).
6. Run `bun run check:contrast` (SDD-012 §18.4) and keep the numbers of section 2.6.

## 3. Layout

### 3.1 Editor regions

The editor shell is the CSS grid of SDD-012 §6.1: rows `48px 1fr var(--timeline-h)`, columns `var(--library-w) 1fr var(--inspector-w)`; the top bar and the timeline span all columns.

| Region | Landmark | Surface | Contents | Size |
| --- | --- | --- | --- | --- |
| Top bar | `header` | `--ed-panel`, bottom `--border` | back, project name, revision and connection state, Undo and Redo, activity indicator, palette button, theme, language, jobs chip, Export (SDD-012 §6.2) | 48 px high, padding `px-3`, groups separated by `gap-2`, the primary action (Export) at the far right |
| Library (tabs Library and History) | `aside` with `aria-label` | `--ed-panel`, right `--border` | import bar, asset cards, trash group; History tab | default 320 or 240 px (section 3.2); minimum 200 px; collapsible (`Alt+1`) |
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
| 1600 px and wider | 320 px | 360 px | 300 px | supported |
| 1280 to 1599 px | 240 px | 280 px | 240 px | supported; the minimum supported size is 1280 by 720 |
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

The grid, sizes and breakpoints are those of sections 3.1 and 3.2; this layout keeps them. At 1600 px wide the library is 320 px (40 px rail and a 280 px panel), the inspector 360 px and the timeline 300 px high; at 1280 px they are 240 (40 and 200), 280 and 240 px. The player is wider than in the reference layout on purpose: a wider player serves vertical video, and the panels are resizable (SDD-012 §6.5 row 1).

| Region | Surface | Border | Text tokens on it |
| --- | --- | --- | --- |
| Top bar | `--ed-panel` | bottom `--border` | `--foreground`, `--muted-foreground` (save time, revision) |
| Library rail and panel | `--ed-panel` | right `--border`; the rail has its own right `--border` | `--foreground`, `--muted-foreground` |
| Preview | `--ed-preview-surround` (both themes) | none | the surround text colours of section 2.5 |
| Transport row | `--ed-panel` | top `--border` | `--foreground`; total time `--muted-foreground` |
| Inspector | `--ed-panel` | left `--border` | `--foreground`, `--muted-foreground` (labels, units) |
| Timeline toolbar | `--ed-panel` | top and bottom `--border` | `--foreground` |
| Timeline body | `--ed-timeline` | rows `--ed-track-line` | section 4.3.6 |

#### 3.4.3 Top bar

Left to right: **Menu** (`Button` `ghost` `sm` with a `Menu` icon and the label, web only; in the desktop shell the same items are the native menu bar, section 8.3), back to projects, project name, **save indicator** ("Saved automatically: 14:32", `text-xs tabular-nums text-muted-foreground`, a `Check` icon before it, hidden while the editor is not live), revision and connection state (section 4.3.3), Undo and Redo, activity indicator, flexible space, palette, jobs chip, **Export** (`Button` `default`, the one primary action of the editor). The theme and language controls of SDD-012 §6.2 stay in the bar and are also reachable from the View menu and the Shortcuts and settings dialog.

The menu (`DropdownMenu`, level 1 elevation) has the groups and actions of SDD-012 §6.4 Menu: File, Edit, Clip, Timeline, View, Help. Each item shows its key at the right from `SHORTCUTS`, in `text-xs text-muted-foreground`; submenus open with `Right`, close with `Left` or `Esc`. A group with no drawable item is not drawn. Context menus of a clip and of an empty part of the timeline (`ContextMenu`) repeat the Clip group plus Copy, Cut, Paste, Duplicate, Delete and Ripple delete, and stay short: no item is shown disabled only to advertise a feature.

#### 3.4.4 Library: tab rail and panel

- **Tab rail**, 40 px wide, at the left edge of the library region: `role="tablist"`, `aria-orientation="vertical"`, one `IconButton` `icon` (32 px) per tab with `aria-label` and a tooltip from `t()`, roving `tabindex` with `Up`, `Down`, `Home` and `End`. Tabs in this order, each drawn only when its command exists (SDD-012 §6.4): Media (`Film`), Audio (`AudioLines`), Text (`Type`), Captions (`Captions`), Effects (`Sparkles`), Transitions (`ArrowLeftRight`), Filters (`SlidersHorizontal`), Templates (`LayoutTemplate`), History (`History`).
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
| History | history rows (section 4.3.16) |

`Alt+1` toggles the whole region; the collapsed region keeps the rail visible, so the tabs stay one click away.

#### 3.4.5 Preview and transport

The canvas box is unchanged (section 4.3.7). The transport row (40 px) is laid out in three groups: at the left the timecode `current / total`; in the centre go to start, step back, Play and Pause (`icon-lg`), step forward, go to end; at the right the aspect-ratio `Select` `sm` (the size presets of the core), Fit, the Safe area `Toggle` and Full screen. In full screen the canvas fills the screen on `--ed-preview-surround` and `Esc` leaves. Controls drawn over the surround use `--ed-handle` for focus (section 2.10).

#### 3.4.6 Inspector: tab strip and property rows

- **With no selection:** the project panel (name, size preset, frame rate, duration, the template info), no tab strip.
- **With a selection:** the panel header names the selection; under it a **tab strip** of 36 px (`Tabs`, horizontal, automatic activation) with the tabs of SDD-012 §6.4 (Video, Audio, Speed, Animation, Adjust, Text), each drawn only when the selection has its sections; a multi-selection shows the tabs the items share. The selected tab has `--foreground` text and a 2 px bottom bar in `--ring`; the others `--muted-foreground`. When the labels do not fit (pt-BR runs longer, section 6.5) the strip scrolls horizontally inside itself with the focused tab kept in view; labels never truncate.
- **Property row:** the field of section 4.3.8 followed by the keyframe diamond (`aria-pressed`) and a **reset** `IconButton` `icon-xs` (`RotateCcw`, `aria-label` "Reset `<property>`"), present only where the core documents a clearing value and `aria-disabled` at the default (SDD-012 §6.5 row 4). Each pair keeps the 24 px target size and 24 px between centres.
- Sections inside a tab stack and scroll; a section header is collapsible (section 4.3.8).

#### 3.4.7 Timeline toolbar and tracks

The toolbar (40 px, `ui/Toolbar`) has two labelled groups (`role="group"`, SDD-012 §6.5 row 5): **Edit tools** at the left (Select and Blade as a `ToggleGroup`, Undo, Redo, Split, Trim start, Trim end, Delete, Add caption, Freeze frame when it exists) and **Modes and zoom** at the right (Snap, Magnet and Link as `Toggle`s, then zoom out, the slider, zoom in, Fit). Text buttons keep a visible label and collapse to icons only when the toolbar is narrow (section 4.3.1); every tooltip names the current key ("Split (S)"). A pressed toggle shows `bg-accent`, a border of at least 3:1 against `--ed-panel` and `aria-pressed="true"` (see open point 13 of section 10).

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
| `foreground` on `ed-panel` (labels, tab names, menu items) | 4.5 | 19.79 | 17.34 |
| `muted-foreground` on `ed-panel` (save time, units, shortcut hints, unselected inspector tabs) | 4.5 | 6.00 | 6.98 |
| `foreground` on `accent` (selected rail tab, pressed toggle, highlighted menu row) | 4.5 | 18.15 | 14.48 |
| `muted-foreground` on `accent` (shortcut hint in a highlighted menu row) | 4.5 | 5.50 | 5.83 |
| `ring` on `ed-panel` (focus outline; selected rail and tab bars) | 3 | 7.25 | 11.56 |
| `ring` on `accent` (focus outline on a selected tab) | 3 | 6.65 | 9.66 |
| `ring` on `ed-bg` (focus outline crossing a region edge) | 3 | 6.65 | 12.40 |
| `primary-foreground` on `primary` (Export, Create project, Start export) | 4.5 | 17.16 | 14.22 |
| `ed-handle` on `ed-preview-surround` (focus in the preview and full screen) | 3 | 11.56 | 11.56 |

Focus is the 2 px `--ring` outline with 2 px offset everywhere (section 2.10); inside the preview it is `--ed-handle`, inside the timeline grid `--ed-selection`. A selected tab is shown by the bar and `aria-selected`, a pressed toggle by the fill, the border and `aria-pressed`, never by colour alone.

#### 3.4.10 Judgement calls of the first build

The comparison with the real CapCut app was made by TASK-0296 (CapCut 9.8.0.4213, walked with the window on screen); its marks, row by row, are in SDD-012 section 6.4, and this section keeps only the calls the build made, as that section lists them: the Adjust tab holds Color, Filters, Chroma key and Effects, and the Text tab holds Auto captions; several items show no inspector strip; the Transitions panel has no length field; Select and Blade are two `aria-pressed` buttons in the toolbar, with Undo and Redo moved there from the top bar; the ratio in the player row is a native `select` that sends `settings.reframe`; the toolbar and menu draw only what has an action (no Duplicate on a project card, no Zoom or Theme item in the menu); a text button collapses to its icon below 1536 px and keeps its name.

### 3.5 Dashboard sidebar

The frame of every page except the editor and the login (SDD-012 §23). It uses tokens whose contrast pairs section 2.6 already checks (`--background`, `--foreground`, `--muted`, `--muted-foreground`, `--border`) and adds none; the kept `--sidebar*` tokens stay unused.

| Part | Element | Surface | Contents | Size |
| --- | --- | --- | --- | --- |
| Sidebar | `header` (the banner) holding a `nav` with `aria-label` for the sections | `--background`, right `--border` | the product mark, the eleven sections, then the profile and the controls | 16 rem expanded, 3.5 rem collapsed; collapsible; below 768 px a strip above the page, always collapsed, in the same order, with no collapse button |
| Item | `a` | transparent | an icon (`size-4`, `aria-hidden`) and the label | 36 px high, `px-3`, `gap-2` |
| Main | `main` | `--background` | the page | the remaining width, scrolls on its own |
| Right dock | `div`, no landmark | `--background`, left `--border` once it has content | empty; reserved for the agent chat | 0 px until epic 14 |

States of an item: rest `--muted-foreground` text; hover `--muted` fill and `--foreground` text; current `--muted` fill, `--foreground` text, weight 500 and `aria-current="page"` (never colour alone); focus the 2 px `--ring` outline of section 2.10, inside the item so the sidebar edge does not clip it; disabled is not used, a section that is not built opens an empty state. Collapsed: the label is visually hidden, the icon stays in an item at least 36 px wide (a target above the 24 px of WCAG 2.5.8, also in the narrow strip, where the items wrap instead of scrolling), a `Tooltip` shows the label on hover and focus.

The bottom block holds, in this order: the profile (the email in network mode, "Local" in local mode, a link to Settings, Sign out in network mode), the language select, the theme toggle and the collapse button. Their contrast pairs are `muted-foreground` on `background` and on `muted`, and `foreground` on `background`, all in section 2.6 and `check:contrast`.

**Decision of this document.** The dashboard has its own small sidebar instead of the shadcn `Sidebar` of section 4.2: the kept component stores its state in a cookie and carries a mobile sheet the dashboard does not need. The row of section 4.2 is amended: the shadcn `Sidebar` stays unused until a page needs it.

## 4. Components

### 4.1 The state model

All interactive components use the same visual states. "Disabled" never relies on opacity alone for meaning; the reason is given by a tooltip or nearby text when it is not obvious.

| State | Selector | Treatment |
| --- | --- | --- |
| default | | as the variant says |
| hover | `:hover` (pointer only; never the only cue) | surface one step (`bg-muted`, `bg-accent`, or, for the primary button, `--primary` mixed with 10% black in OKLab: a fade to `primary/80` left white text at 3.7:1 in the light theme) |
| focus-visible | `:focus-visible` | 2 px `--ring` outline, 2 px offset (section 2.10); never removed |
| active (pressed) | `:active`, `aria-pressed="true"`, `data-state="active"` | shadcn's 1 px press offset for buttons; toggles show the pressed surface `bg-accent` plus `aria-pressed` |
| disabled | `disabled`, `aria-disabled="true"` | 50% opacity, `pointer-events: none` (shadcn); exempt from contrast; keyboard focus skips native disabled controls; controls that must explain why use `aria-disabled` with a tooltip |
| selected | `aria-selected="true"`, `aria-current` | `--ed-selection` ring for timeline objects; `bg-accent` plus a 2 px leading bar for list rows; `aria-current="page"` for navigation (shadcn sidebar `isActive`) |
| error | `aria-invalid="true"` | `--destructive` border and ring (shadcn), an icon and the message under the field (`aria-describedby`) |
| busy | `aria-busy="true"` | the control keeps its value and stays enabled for reading; no spinner; a "Saving" text label appears after 400 ms only if the request is still in flight |
| read-only | `readonly`, or plain text | plain text in `text-foreground`, no field boundary |

### 4.2 Shared primitives

The files in `components/ui/` (SDD-012 §3.1). Dialogs, popovers, tooltips, tabs, sheets, menus, `Select` and form controls are `@base-ui/react`. This section says how the product uses them.

| Component | Variants and sizes used | States and keyboard | Product rules |
| --- | --- | --- | --- |
| `Button` | variants `default` (primary, one per view), `secondary`, `outline`, `ghost` (toolbars, icon buttons), `destructive` (remove, delete), `link` (inline). Sizes `default` 32 px, `sm` 28 px, `xs` 24 px, `lg` 36 px, `icon` 32 px, `icon-sm` 28 px, `icon-xs` 24 px, `icon-lg` 36 px | all of 4.1; `Enter` and `Space` activate; icon-only buttons have `aria-label` from `t()` and a `Tooltip` with the shortcut | labels are verbs (section 6.2); the icon leads the label (`data-icon="inline-start"`); an anchor given to `asChild` stays a link (`role="link"`), never announced as a button; no button wider than its label plus padding, so pt-BR text fits |
| `Badge` | `secondary` (counts, neutral states), `outline` (scope in the palette), `destructive` (failed); product tints through classes `bg-success/10 text-success`, and so on, for ready, warning, agent | not focusable unless it is a link | always text, optionally an icon; never colour alone |
| `Card` | default | | project cards on `/projects`, settings groups; not used inside the editor (panels are flat) |
| `Input` | default; `font-mono` for paths | 4.1; error state through `aria-invalid` | always with a `Label` or an `aria-label`; placeholder is an example, never the label |
| `Textarea` | default | 4.1 | caption and text content; `Ctrl+Enter` commits (SDD-012 §12.3) |
| `Label` | default | | `text-xs font-medium text-muted-foreground` in the inspector, `text-sm` on pages |
| `Select` | default trigger `h-8`; `sm` in toolbars | UI SDK: `Space` and `Enter` open, arrows move, typing selects, `Esc` closes | used for enums (preset, fit, font family, track); options rendered with `UntrustedText` when they show user content |
| `Separator` | horizontal and vertical | decorative (`role="none"`) unless it separates toolbar groups (`role="separator"`) | |
| `Sheet` | `left` (library drawer), `right` (inspector drawer) | focus trap, `Esc` closes, focus returns to the opener | only at 1024 to 1279 px (section 3.2) and in the mobile Inertia sidebar |
| `Sidebar` | `collapsible="icon"`, 16 rem / 3 rem | `Ctrl+B` toggles (kept) | kept in `components/ui/` but unused; the dashboard sidebar of section 3.5 is its own component |
| `Skeleton` | default | `aria-hidden`; the region has `aria-busy` | first load of the project list and of the document; never longer than the request; no shimmer animation |
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
- Connection state (text and icon, never colour alone): `live` (no icon, `text-muted-foreground` "rev 42"), `reconnecting` (`RefreshCw` icon, `--warning`, "Reconnecting"), `offline` (`--destructive`, "Offline", the edit controls become `aria-disabled`), `resyncing` (`--info`, "Syncing").
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
- Asset card (grid of two columns at 320 px, one at 240 px): thumbnail box `aspect-video`, `object-contain` on `--ed-preview-surround`, `rounded-sm`; name (`UntrustedText`, one line, tooltip with the full name); meta line `text-xs tabular-nums text-muted-foreground`: "00:05.000 · 1080×1920 · 12 MB"; state badge with text (Pending, Ingesting, Ready, Failed); `Progress` (4 px) while ingesting; actions: "Add to timeline" (`Button` `sm` `secondary`, enabled only when ready and present in the read model) and a menu.
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
| Revision | "rev 42" | |
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

Every focusable element shows the 2 px `--ring` outline with 2 px offset (section 2.10), at 3:1 or more against what is around it in both themes. Focus is never hidden by sticky elements: the timeline scrolls a focused item clear of the sticky headers and ruler (WCAG 2.4.11).

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

Interface motion is limited to `.ed-motion` and `.ed-highlight` (section 2.13); `prefers-reduced-motion` turns off those and the transitions shadcn carries. Nothing flashes more than three times per second; the timed outlines appear once and disappear once.

### 7.6 Contrast

The pairs and ratios of section 2.6, in both themes, checked in CI by `bun run check:contrast` and axe (`a11y-axe-both-themes`, SDD-012 §18.4). Decorative dividers (`--border`) are exempt and never the only cue. Nothing is conveyed by colour alone: selection has a ring and `aria-selected`; states have an icon and text; item types have an icon and a label.

### 7.7 Forced colours

In Windows high-contrast mode (`forced-colors: active`) the system replaces colours. Rules: focus and selection use `outline` and `border`, which survive forced colours (not `box-shadow` alone; the inset selection ring adds `outline: 2px solid Highlight` under `@media (forced-colors: active)`); item blocks keep their 1 px border; icons use `currentColor`; no component sets `forced-color-adjust: none` except the preview canvas, which shows the picture.

### 7.8 Target sizes

At least 24 by 24 CSS px for every target (WCAG 2.5.8); the design default is 32 px (`h-8`). Timeline handles: 8 px visual, 24 px hit area; preview handles: 8 px visual, 24 px hit area. Adjacent small targets keep at least 24 px between centres.

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

## 9. Governance

### 9.1 One source per thing

| Thing | Lives in | Owner |
| --- | --- | --- |
| Token names and values | `web/resources/css/app.css` (`:root`, `.dark`, `@theme inline`); this document records them | senior-e |
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
- [ ] Targets are at least 24 by 24 CSS px.
- [ ] Nothing is conveyed by colour alone.
- [ ] No `transition`, `animation` or `@keyframes` besides `.ed-motion` and `.ed-highlight` (D-013); reduced motion respected.
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
| DS-4 | Timecodes in Geist with tabular figures, no monospace UI font; the digit-width test passes (24.48 px per digit at 40 px), so Geist Mono is not added | 2.7 | SDD-012 §11.5, D-030 |
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

**What changes from section 2.4.** `--brand` was a placeholder, `oklch(0.511 0.262 276.966)` in both themes, "never a button". It becomes the icon's indigo (light) and lilac (dark), and it may now carry the one primary action of a dashboard page and of the documentation. Inside the editors (`web/resources/js/editor/**` and the image editor) the primary action stays the neutral `--primary` of section 2.3, and principle P2 is unchanged: no brand colour near the picture, the timeline or the canvas.

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

Geist Variable stays the only family (section 2.7; bundled, no other origin). The expressive register adds a display scale:

| Role | Utility | Line height | Weight and tracking |
| --- | --- | --- | --- |
| Hero | `text-6xl` to `text-8xl` (`clamp(2.75rem, 7vw, 6rem)`) | 1.0 | 600, `tracking-tight` (-0.025em) |
| Section title | `text-4xl` to `text-5xl` | 1.1 | 600, `tracking-tight` |
| Lead | `text-lg` to `text-xl` | 1.6 | 400, `text-muted-foreground` |
| Eyebrow | `text-xs`, uppercase | 1.4 | 500, `tracking-widest`, `--brand-text` |
| Code and terminals | `font-mono`, `text-sm` | 1.7 | 400 |

The 12 px minimum and the weights 400, 500 and 600 of section 2.7 hold in both registers.

### 11.6 Buttons

All buttons are the Base UI `Button` of section 4.2, with `rounded-lg` (10 px), heights `h-9` (`sm`), `h-10` (default) or `h-12` (landing `lg`), and a 6 px gap between icon and label.

| Variant | Light | Dark | Use |
| --- | --- | --- | --- |
| `brand` (new) | `bg-brand text-brand-foreground`, hover 800 | `bg-brand text-brand-foreground`, hover 400 | the one primary action of a dashboard or documentation page; the main call to action of the landing |
| `default` | the neutral `--primary` of section 2.3 | same | the primary action inside the editors |
| `secondary`, `outline`, `ghost`, `link`, `destructive` | as section 4.2 | as section 4.2 | as section 4.2 |

At most one `brand` button per view. Focus is the 2 px `--ring` outline of section 2.10 on every variant.
**The call-to-action orange (landing only).** The author asked for one striking "Getting Started" button on the landing pages. Orange is the one hue added to the palette, and only for that button: it never appears in the product or in the documentation pages.

| Token | Both themes | Hex | Use |
| --- | --- | --- | --- |
| `--cta` | `oklch(0.7 0.2 48)` | `#FC6D00` | the "Getting Started" button of the landing hero and footer |
| `--cta-foreground` | `oklch(0.145 0 0)` | `#0A0A0A` | its label |
| `--cta-hover` | light `oklch(0.64 0.2 45)`, dark `oklch(0.76 0.18 55)` | `#E85600`, `#FF8C1E` | hover; the focus ring stays the 2 px outline |

Contrast (WCAG 2.x): `--cta-foreground` on `--cta` 6.91:1, on the light hover 5.45:1, on the dark hover 8.52:1, all above 4.5:1. The button fill against the page is 2.86:1 on white and 6.91:1 on the dark background; the label, not the fill, identifies the control, so WCAG 1.4.11 does not apply to the fill.

**The four landing actions (glow buttons).** Getting Started and the three downloads share one style (`.btn-glow` in `site/src/landing/landing.css`): a neutral face (`--bg`) with the label in `--fg`, a 1.5 px border in the colour of the button, a small light that travels along the border (a conic gradient whose angle is animated, still under reduced motion), depth from a tinted shadow, the colour filling the face on hover, a 2 px press on `:active` and a 2 px focus outline in the border colour. Each button has its own colour:

| Button | Fill on hover (`--c`) | Border light / dark (`--c-edge`) | Label on the fill (`--c-on`) | Label on the fill |
| --- | --- | --- | --- | --- |
| Getting Started | `--cta` `oklch(0.7 0.2 48)` `#FC6D00` | `--cta` / `--cta` | `oklch(0.145 0 0)` | 6.91:1 |
| Windows | `oklch(0.5 0.17 252)` `#0062BF` | `oklch(0.5 0.17 252)` / `oklch(0.65 0.15 250)` | white | 5.97:1 |
| macOS | `oklch(0.82 0.01 260)` `#C0C4CB` | `oklch(0.62 0.01 260)` / `oklch(0.82 0.01 260)` | `oklch(0.145 0 0)` | 11.34:1 |
| Linux | `oklch(0.84 0.15 85)` `#F7C243` | `oklch(0.72 0.15 75)` / `oklch(0.84 0.15 85)` | `oklch(0.145 0 0)` | 12.01:1 |

At rest the label is `--fg` on `--bg` (above 15:1 in both themes) and the version line `--muted` on `--bg` (7.8:1 light, 7.4:1 dark); on hover the version line takes the label colour. The border is decorative: the label identifies the control.

### 11.7 Radii, borders and shadows

- Radii: `--radius` (10 px) and its scale (section 2.9) stay. The expressive register adds `rounded-3xl` (24 px) for large showcase frames (the product window of the hero) and `rounded-full` for pills.
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

### 11.9 3D and the living background (expressive register only)

- three.js 0.187.0 (r187, the current stable release), one renderer per page, `devicePixelRatio` capped at 2 (1.5 on small screens), the render loop paused when the tab is hidden or the canvas is off screen, loaded as a client-only island after the text has painted.
- The living background: one full-screen fragment shader, a slow noise flow of brand 700 and 500 on the background of the theme, the pointer and the scroll velocity adding a gentle disturbance; at most 10% of the frame budget.
- The mascot: the icon in 3D from primitives (a rounded box in 700, the clapper bar in 500, two capsule eyes in white), no model file; it may blink, follow the pointer and snap the clapper shut on a scene change.
- A WebGL failure, a very slow device or reduced motion falls back to a static brand gradient and a still image of the mascot. All text is HTML over the canvas, never drawn in WebGL.

### 11.10 Layout of the expressive register

- Content width `max-w-6xl` (72 rem) with 24 px gutters (`px-6`), 16 px below `sm`; the hero and showcase frames may bleed to the full width.
- A 12-column grid from `lg`; sections separated by 128 px (`py-32`) from `lg`, 80 px below.
- One idea per section: an eyebrow, a title, a lead of at most two lines, then the proof (a clip, a code sample, a card grid).

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
