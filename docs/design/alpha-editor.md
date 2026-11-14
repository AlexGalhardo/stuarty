# ALPHA editor: screen specifications

- Status: Draft for review (2026-11-14). Written by the designer for senior-e (editor UI), to be reviewed by the architect.
- Scope: the ALPHA UI groups of `ROADMAP.md` G2.19 to G2.23, on top of the Prototype of G1.6 (P1-UI-001 to P1-UI-006).
- Binding sources, which this document follows and never overrides: [design system](../design-system.md) (tokens, components, states, D-028 and D-030), [SDD-012](../sdds/012-ui-architecture.md) (layout §6, timeline §7, gestures §8, palette §9.2, inspector §12, export §14, history §15, accessibility §18), [SDD-001](../sdds/001-timeline-schema.md) §3.6 and §3.7 (text, caption and shape fields), [SDD-003](../sdds/003-agent-interfaces.md) §3.6 (lint findings).
- Format: these specifications are text wireframes. The design tool (OpenPencil) was not available in the session that wrote them, so there are no PNG exports. The wireframes are not to scale; the pixel numbers next to them are binding.

A paragraph headed **Decision of this document** fills a gap that no SDD or the design system covers. Section 9 lists them again with where each should be recorded. Open questions are in section 10.

## Contents

0. [Conventions](#0-conventions)
1. [Editor main layout](#1-editor-main-layout)
2. [Command palette](#2-command-palette)
3. [Inspector and the schema form](#3-inspector-and-the-schema-form)
4. [History panel and activity badge](#4-history-panel-and-activity-badge)
5. [Export dialog and job progress](#5-export-dialog-and-job-progress)
6. [Timeline direct manipulation](#6-timeline-direct-manipulation)
7. [Cross-screen accessibility](#7-cross-screen-accessibility)
8. [Variant recommendations](#8-variant-recommendations)
9. [Decisions of this document](#9-decisions-of-this-document)
10. [Open questions](#10-open-questions)

## 0. Conventions

- **Names.** Boxes in the wireframes carry the React component name of SDD-012 §3.2 (`TopBar`, `Library`, `AssetCard`, `Inspector`, `ClipPanel`, `Timeline`, `TrackHeader`, `Palette`, `BatchTray`, `HistoryPanel`, `ExportDialog`) or a component of the design system §4 (`Button`, `IconButton`, `Badge`, `ErrorNotice`, `Toast`).
- **Tokens.** Colours are written without the `--` prefix and without `color-`: `ed-panel` means `var(--ed-panel)` and the Tailwind utility `bg-ed-panel`. A tinted state surface is written `agent/10` (light) and `agent/20` (dark), as in design system §2.4.
- **Wireframe glyphs.** `[Label]` a text button, `[i:Icon]` an icon-only button (Lucide name), `[__value__]` an input, `[v Value]` a select, `( )` and `(o)` a radio, `[x]` and `[ ]` a checkbox or switch, `==` a progress bar, `‖` a trim handle, `┆` the snap line, `▼` and `│` the playhead, `╌` a dashed outline, `▓` a selected item, `░` a hidden-track or ghost fill.
- **Layout.** "grid" and "flex" are the CSS that implements a box. Spacing values are Tailwind steps (`gap-2` = 8 px). Every size not stated here is the design system's.
- **Strings.** Visible English strings are shown as they will read in `en.json`; the key is given where it is new. Text from the core (messages, hints, history descriptions, diffs) is shown in a monospace style in the wireframes and is never translated (SDD-012 §16.4).

## 1. Editor main layout

### 1.1 The grid at the two sizes

The shell is the CSS grid of SDD-012 §6.1: `grid-template-rows: 48px 1fr var(--timeline-h)`, `grid-template-columns: var(--library-w) 1fr var(--inspector-w)`. Both target viewports fall in the 1280 to 1599 px band of design system §3.2, so the defaults are the same; only the preview grows.

| Measure | 1440 × 900 | 1280 × 720 | How it is obtained |
| --- | --- | --- | --- |
| Top bar | 1440 × 48 | 1280 × 48 | row 1, spans 3 columns |
| Library (`--library-w`) | 240 | 240 | default of the band; minimum 200 |
| Inspector (`--inspector-w`) | 280 | 280 | default of the band; minimum 200 |
| Timeline (`--timeline-h`) | 240 (variant A) or 300 (variant B, 1.4) | 240 | minimum 160 |
| Middle row height | 612 (A) or 552 (B) | 432 | 900 − 48 − 240; 720 − 48 − 240 |
| Preview region | 920 × 572 (A) | 760 × 392 | middle column × (middle row − 40 transport) |
| Canvas box | 904 × 556 (A) | 744 × 376 | region − 16 in each dimension (SDD-012 §11.4) |
| 9:16 canvas | 313 × 556 (A), 279 × 496 (B) | 211 × 376 | height-bound |
| 16:9 canvas | 904 × 509 (A) | 668 × 376 | width-bound at 1440, height-bound at 1280 |
| Timeline toolbar | 40 | 40 | `ed-panel` |
| Ruler | 28 | 28 | sticky |
| Rows area | 172 (A) or 232 (B) | 172 | timeline − 40 − 28 |
| Visible tracks at default heights | 3 (40 + 64 + 48 = 152) | 3 | caption 40, video 64, audio 48 (SDD-012 §7.4) |
| Rows width | 1264 | 1104 | viewport − 176 track header |

The library and the inspector heights equal the middle row; each scrolls on its own. The page never scrolls.

### 1.2 Wireframe at 1440 × 900 (variant A, recommended)

```text
x=0                 240                                                       1160                      1440
┌───────────────────────────────────────────────────────────────────────────────────────────────────────────┐ y=0
│TopBar  [i:ArrowLeft] Spring launch 9x16 ▾   rev 42   [i:Undo2][i:Redo2]   ● claude-code is editing        │
│                                   [i:Command Commands  Ctrl+K] [i:Moon][i:Languages]  2 jobs·64%  [Export] │
├──────────────────┬────────────────────────────────────────────────────────────┬──────────────────────────┤ y=48
│Library ▸ Tabs    │Preview (ed-preview-surround)                                │Inspector                 │
│[Library][History]│                                                             │ Clip · IMG_0421          │
│──────────────────│                 ┌─────────────────┐                         │──────────────────────────│
│ImportBar         │                 │                 │                         │ ▾ Name                   │
│[_C:\media\clip_] │                 │   9:16 canvas   │                         │ ▾ Timing                 │
│[Import]          │                 │   313 × 556     │                         │ ▾ Transform              │
│──────────────────│                 │                 │                         │ ▸ Transitions            │
│AssetCard         │                 │  ┌───────────┐  │                         │ ▸ Audio                  │
│┌────────────────┐│                 │  │SelectionBox│ │                         │ ▸ Framing                │
││ thumb 16:9     ││                 │  └───────────┘  │                         │ ▸ Speed                  │
│└────────────────┘│                 │                 │                         │ ▸ Color                  │
│IMG_0421.mov      │                 └─────────────────┘                         │ ▸ Silences               │
│00:12.480·1080×   │                                                             │ ▸ Auto captions          │
│1920·48 MB [Ready]│─────────────────────────────────────────────────────────────│ ▸ Keyframes              │
│[Add to timeline] │Transport [i:SkipBack][i:StepBack][ Play ][i:StepForward]    │ ▸ Template               │
│ ...              │   [i:SkipForward]   00:04.200 / 00:28.000  [Safe area][Fit]│                          │
├──────────────────┴────────────────────────────────────────────────────────────┴──────────────────────────┤ y=660
│Toolbar [Split][Trim start][Trim end] | [Add caption][Add text][Add shape v] | [Delete] | [Snap] | Zoom [-]══[+][Fit] │
├──────────────┬────────────────────────────────────────────────────────────────────────────────────────────┤ y=700
│ corner       │Ruler 0:00      0:05      0:10      0:15      0:20      0:25         ▼                      │ y=728
├──────────────┼────────────────────────────────────────────────────────────────────────────────────────────┤
│TrackHeader   │[Captions Hello there and wel..][Captions 12 words]                       │                    │ 40
│T Captions  ⋯ │                                                                           │                    │
│F Video [Eye]…│[Film IMG_0421 ──────────────────][Film IMG_0433 ─────────────]           │                    │ 64
│A Music [Vol]…│[AudioLines music.wav  ∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿]         │                    │ 48
└──────────────┴────────────────────────────────────────────────────────────────────────────────────────────┘ y=900
```

### 1.3 Wireframe at 1280 × 720

Same tree; only the preview shrinks. The top bar is the tightest region (pt-BR must not overflow, P2-I18N-004).

```text
x=0                 240                                             1000                      1280
┌─────────────────────────────────────────────────────────────────────────────────────────────┐ y=0
│[i:ArrowLeft] Spring launch 9x16  rev 42 [i:Undo2][i:Redo2] ● claude-code and 1 other are ed…│
│                                         [i:Command] [i:Moon][i:Languages] 2 jobs·64% [Export]│
├──────────────────┬─────────────────────────────────────────────────┬────────────────────────┤ y=48
│[Library][History]│            ┌──────────┐                          │ Clip · IMG_0421        │
│[_path__________] │            │ 9:16     │                          │ ▾ Name                 │
│[Import]          │            │ 211×376  │                          │ ▾ Timing               │
│┌──────┐          │            │          │                          │ ▾ Transform            │
││thumb │ (1 col)  │            └──────────┘                          │ ▸ ...                  │
│└──────┘          │ [|<][<][ Play ][>][>|] 00:04.200 / 00:28.000 [SA][Fit]│                    │
├──────────────────┴─────────────────────────────────────────────────┴────────────────────────┤ y=480
│[i:Scissors][i:ArrowLeftToLine][i:ArrowRightToLine] | [Add caption][Add text][Add shape v] | … │ y=480..520
│ ruler 28                                                                                       │
│ 3 tracks × (40, 64, 48)                                                                        │
└─────────────────────────────────────────────────────────────────────────────────────────────┘ y=720
```

**Top bar budget at 1280 px** (flex row, `px-3 gap-2`, items in this order; the numbers are maximum widths):

| Item | Width | Shrink rule |
| --- | --- | --- |
| Back `IconButton` | 32 | never |
| Project name (`UntrustedText`, button that turns into an input on click or `F2`) | 120 to 240 | truncates first, ellipsis and tooltip |
| Revision and connection state | 56 to 120 | never truncates; "rev 42" plus the state text |
| Undo, Redo | 2 × 32 + 4 | never |
| `ActivityIndicator` | 0 to 280 | truncates second; the actor label is kept, the trailing words go first |
| flexible space | `1fr` | |
| Palette button | 32 to 156 | below 1440 px the label "Commands" and the `Ctrl+K` hint collapse to the icon (same `aria-label`, tooltip "Commands (Ctrl+K)") |
| Theme, Language | 2 × 32 | never |
| `JobsChip` | 0 to 120 | hidden when no job runs |
| Export (`Button` `default`) | 88 to 112 | never; pt-BR "Exportar" |

The sum at the maxima, with the palette collapsed, is 1 196 px, under 1 280.

### 1.4 Variants: the timeline height at 900 px of height

The choice is open because SDD-012 §6.3 sets the timeline default by width only, and a 1440 × 900 window has 180 px more height than the minimum.

| | Variant A: width rule only (recommended) | Variant B: height-aware default |
| --- | --- | --- |
| Rule | `--timeline-h` = 240 in the 1280 to 1599 band, as SDD-012 §6.3 | 300 when the viewport is at least 880 px high, else 240 |
| 9:16 canvas at 1440 × 900 | 313 × 556 | 279 × 496 |
| Rows visible | 3 default tracks (172 px) | 4 tracks or 3 tracks and room for a fourth (232 px) |
| Spec change | none | SDD-012 §6.3 and design system §3.2 |

**Recommendation: A.** The two use cases (short vertical videos and commercials) are judged by the picture; a 9:16 canvas is height-bound, so every pixel of timeline costs preview height. The user who wants a taller timeline drags the splitter once, and `prefs.ts` keeps the size. Variant B is cheap to add later if users ask for it.

### 1.5 Components and tokens

| Region | Components | Tokens |
| --- | --- | --- |
| Top bar | `layout/TopBar`, `IconButton`, `UntrustedText`, `layout/ActivityIndicator`, `LocaleSelect`, `ThemeToggle`, `JobsChip`, `Button` `default` | `ed-panel`, bottom `border`; text `foreground`, `muted-foreground` (revision), state tones `warning`, `destructive`, `info`; `agent` (activity) |
| Library | `ui/Panel` `library`, `Tabs`, `ImportBar`, `AssetCard`, `Progress` 4 px, `Badge` | `ed-panel`, right `border`; thumbnails on `ed-preview-surround`, `rounded-sm` |
| Preview | canvas frame, `SafeArea`, `SelectionBox`, `Transport` | `ed-preview-surround`, canvas outline `oklch(1 0 0 / 15%)`, `ed-handle`, `ed-safe-area` |
| Inspector | `ui/Panel` `inspector`, sections with chevrons, fields of design system §4.3.8 | `ed-panel`, left `border` |
| Timeline | `Timeline`, toolbar `ui/Toolbar`, `Ruler`, `TrackHeader`, `TrackRow`, items, `Playhead`, `SnapLine`, `GhostLayer` | toolbar `ed-panel`; body `ed-timeline`; `ed-track-line`, `ed-item-*`, `ed-playhead`, `ed-selection`, `ed-snap`, `ed-ghost` |
| Splitters | `ui/Splitter` (vertical between columns, horizontal above the timeline) | 1 px `border`, 8 px hit area, focus outline `ring` |

### 1.6 States of the shell

| State | What changes | Tokens and components |
| --- | --- | --- |
| Loading (boot) | `Skeleton` blocks in library cards, inspector sections and two timeline rows; the regions have `aria-busy="true"`; the top bar shows the project name as soon as the bootstrap has it | `Skeleton`, no shimmer |
| Empty project | library empty state "No media yet"; timeline empty state "The timeline is empty"; inspector shows the project panel; Export opens and shows `L_EMPTY_TIMELINE` as a blocking error | design system §4.3.15 |
| Live | "rev 42" in `muted-foreground`, no icon | |
| Reconnecting | `[i:RefreshCw] Reconnecting` in `warning`; editing stays enabled | static icon |
| Offline | `Offline` in `destructive`; every edit control becomes `aria-disabled="true"` with the tooltip "Offline: changes cannot be sent"; a preview banner says the same | `ui.editor.connection.offline` |
| Resyncing | `Syncing` in `info` | |
| Trashed project (P2-UI-007) | `TrashedBanner` across the top of the middle row (z-30): "This project is in the trash. It opens read-only." with `Button` `secondary` "Restore"; edit controls `aria-disabled` | `warning` icon on `ed-panel` |
| Panel collapsed (`Alt+1`, `Alt+2`) | the column width becomes 0, a 32 px `IconButton` (`PanelLeftOpen`, `PanelRightOpen`) appears at the inner edge of the top bar; the preview takes the width | |
| 1024 to 1279 px | library and inspector are `Sheet` drawers opened from top-bar buttons; the inspector drawer opens on selection; a `warning` toast "Best used at 1280 pixels or wider" once per session | `Sheet` `left` and `right` |
| Below 1024 px | full-screen empty state "This window is too small" with "Back to projects" | |

### 1.7 Keyboard and focus order

- `Tab` order: top bar (left to right) → library tabs → library content → transport → timeline toolbar → timeline grid (one stop) → inspector. `F6` and `Shift+F6` jump between the five landmarks: `header`, `aside` Library, `main` preview, `main` timeline (a `region` inside `main` with `aria-label` "Timeline"), `aside` Inspector.
- Splitters are in the order after the region they follow; arrow keys resize by 16 px, `Home` and `End` go to the minimum and maximum.
- `Alt+1`, `Alt+2`, `Alt+3` toggle library, inspector and the History tab; focus goes to the opened panel, or back to the button that reopens it when a panel closes.

### 1.8 Contrast notes

- Chrome text is `foreground` on `ed-panel` (19.79 light, 17.34 dark) and `muted-foreground` on `ed-panel` (6.00 and at least 7.25).
- The timecode in the transport sits on `ed-preview-surround` in both themes and uses `oklch(0.985 0 0)` and `oklch(0.708 0 0)` (17.34 and 6.98, design system §2.5), not `foreground`, because the surround does not follow the theme.
- `ed-safe-area` in light is 3.03:1 on the surround, the smallest margin of the set (design system §10 item 11); see question Q7.

## 2. Command palette

### 2.1 List view

`Palette` is a `Dialog` (level 2, `bg-black/50` backdrop), 640 px wide, top edge at 20% of the viewport height (180 px at 900, 144 px at 720), `max-height: calc(100vh - 20vh - 24px)`.

```text
┌──────────────────────────────────────────────────────────────────────────┐ 640
│ [i:Search] [ Search commands______________________________ ]   Esc       │ h-10 input, px-3 pt-3
├──────────────────────────────────────────────────────────────────────────┤
│▌Move item                                  item.move          [edit]      │ 40, active row (accent + 2 px bar ring)
│ Trim item                                  item.trim          [edit]      │ 40
│ Split item                                 item.split         [edit]      │ 40
│ Add caption                                caption.add        [edit]      │
│ Generate captions from transcript          caption.from_transcript [edit] │
│ Set project settings                       settings.set       [edit]      │
│ Remove asset from project                  asset.remove       [media]     │
│ Set keyframe                               keyframe.set       [edit]      │ 8 rows visible, the list scrolls
├──────────────────────────────────────────────────────────────────────────┤
│ 47 commands  ·  ↑↓ to move  ·  Enter to open  ·  Esc to close    Batch: 0 │ 32, text-xs muted-foreground
└──────────────────────────────────────────────────────────────────────────┘
```

- Row (`role="option"`, grid `1fr auto auto`, `gap-3`, `px-3`, 40 px): localized title (`text-sm`, `ui.editor.commands.<name>.title`, falling back to `summary` with `lang="en"`), command name (`font-mono text-xs text-muted-foreground`), scope `Badge` `outline` (`text-xs`). A match is shown with `font-semibold` on the matched characters of the title, never colour alone.
- Active row: `bg-accent` and a 2 px leading bar in `ring`; the row is `aria-selected="true"` and the input's `aria-activedescendant` points at it.
- Footer: the command count, the key hints in `<kbd>` style and the batch count (variant A, 2.4).

### 2.2 Form view

Selecting a row replaces the list with the form; the search field becomes a back button and the title.

```text
┌──────────────────────────────────────────────────────────────────────────┐
│ [i:ArrowLeft] Move item                       item.move   [edit]          │ 48, text-lg 600 title
│ Moves an item to a new start and, optionally, another track.             │ summary, text-sm muted, lang="en"
├──────────────────────────────────────────────────────────────────────────┤
│ Item *      [v IMG_0421 · itm_m1n2p3q4r5           ]                      │ id select from the read model
│ Start *     [ 00:12.500        ]  s                                       │ TimeField
│ Track       [v Video · trk_v1                       ]                     │ optional
│                                                                           │
│ ▸ Advanced (JSON)  ← only when a sub-value has no renderer                │
├──────────────────────────────────────────────────────────────────────────┤
│ Preview result                                                            │ shown after Preview
│ ┌──────────────────────────────────────────────────────────────────────┐ │
│ │ ~ itm_m1n2p3q4r5 start 0s/0f -> 12.5s/375f                           │ │ font-mono text-xs, max 12 lines
│ └──────────────────────────────────────────────────────────────────────┘ │
├──────────────────────────────────────────────────────────────────────────┤
│ [Fill example]            [Add to batch] [Preview]  [Run  Ctrl+Enter]     │ ghost, secondary, outline, default
└──────────────────────────────────────────────────────────────────────────┘
```

- Field grid: `grid-template-columns: 128px 1fr`, `gap-x-3 gap-y-2`; label `text-xs font-medium`, a required field has a `*` and the text "required" in its accessible name (not the asterisk alone).
- The renderers of SDD-012 §9.2 map to the design system field atoms: id select → `SelectField` with free text (combobox), time → `TimeField`, enum → `SelectField`, number → `NumberField` with `min` and `max` in the hint line, boolean → `Checkbox`, scalar array → list editor (one row per value, `[i:Plus] Add` and `[i:X]` remove buttons with names "Add value" and "Remove value 2"), object → nested `fieldset` with a `legend` and 16 px left padding and a 1 px `border` leading line, `oneOf` and `anyOf` → `RadioGroup` of the variants, then the variant's fields, fallback → `Textarea` `font-mono` with the sub-schema path as label.
- The preview block shows the core's `diff` and `text` as received, `font-mono text-xs` on `muted`, at most 12 lines then "Show all".

### 2.3 Palette states

| State | Display |
| --- | --- |
| Loading the registry | 3 `Skeleton` rows; the input is usable and the query is kept |
| Empty query | all non-internal commands, alphabetical by title |
| No result | empty state "No command matches" with "Check the spelling or search by command name." |
| Registry failed to load | `ErrorNotice` `block` with the code (`E_NETWORK`) and "Try again" |
| Form, field error | `aria-invalid` on the field, `ErrorNotice` `inline` under it (the core's `path` `args.start` maps to Start) |
| Form, error without a path | `ErrorNotice` `block` at the top of the form; focus moves to it |
| Running | Run is `aria-busy`, the other buttons stay enabled for reading; "Running" text after 400 ms; no spinner |
| Success | the palette closes; a `status` toast with the core's description, and the live region announces it |
| Offline or read-only project | Run, Preview and Add to batch are `aria-disabled` with the reason as tooltip; reading the list and the forms stays possible |

### 2.4 Variants: where the batch lives

The batch (P2-UI-003, RF-105) is the only part of the palette with no layout in the specifications.

```text
Variant A: footer tray (recommended)            Variant B: side column
┌────────────────────────────── 640 ┐           ┌──────────────────────── 640 ──┬──────── 280 ┐
│ form or list                      │           │ form or list                  │ Batch (3)   │
│                                   │           │                               │ 1 item.move │
├───────────────────────────────────┤           │                               │ 2 item.trim │
│ Batch: 3  item.move · item.trim · │           │                               │ 3 text.add  │
│ text.add  [Clear] [Preview] [Run batch]│      │                               │ [Run batch] │
└───────────────────────────────────┘           └───────────────────────────────┴─────────────┘
```

- **A, footer tray (recommended):** a 48 px strip under the footer, visible only when the batch has at least one operation: the count, the operation names as removable chips (`Badge` `secondary` with `[i:X]`, name "Remove item.move from the batch"), "Clear", "Preview batch" and "Run batch" (`default`). The dialog stays 640 px, so it fits 1280 × 720 and the `Sheet` widths below 1280.
- **B, side column:** the dialog grows to 920 px with a 280 px column listing each operation with its arguments. Easier to review long batches, but at 1280 × 720 it covers most of the preview and the timeline the user wants to watch.

**Recommendation: A**, with "Preview batch" as the way to read the full batch: the core's `diff` is a better review than a list of arguments.

### 2.5 Keyboard and focus

- `Mod+K` opens with focus in the search field, and closes when the palette has focus (it works inside editable fields, SDD-012 §18.2).
- List: `Up` and `Down` move the active row (wrapping), `PageUp` and `PageDown` by 8, `Home` and `End` inside the input move the caret, not the row; `Enter` opens the form.
- Form: focus goes to the first empty required field; `Tab` follows the fields, then Fill example, Add to batch, Preview, Run; `Mod+Enter` runs; `Esc` goes back to the list (with the query and active row kept); `Esc` in the list closes and focus returns to the opener.
- The dialog traps focus; the tray chips are in the tab order after the footer buttons.

### 2.6 Contrast notes

The command name in `muted-foreground` on `popover` is 6.00 light and 7.63 dark (the `background` pair; `popover` equals `card`). The active row uses `accent` with `foreground`, 16.42 and 14.48. The scope badge relies on its text, not its border.

## 3. Inspector and the schema form

### 3.1 Shell

`Inspector` (`ui/Panel` `inspector`, 280 px at both target sizes): a 36 px header with the type icon, the type name and the item name (`UntrustedText`, truncates), then sections. A section header is 32 px (`text-sm font-semibold`, chevron `IconButton` `icon-xs` with `aria-expanded`); sections are 16 px apart; the body has `p-3`. Field rows follow design system §4.3.8: grid `96px 1fr`, `gap-x-2`, 32 px controls, message line `text-xs` under the control.

**Decision of this document.** At 280 px the control column is 152 px wide, too narrow for two time fields on one row. "Start and End share a row" (design system §4.3.8) is drawn as a two-column sub-grid spanning both columns, `grid-template-columns: 1fr 1fr`, with each label above its field (124 px fields, enough for `00:12.500` with its padding). From 360 px inspector width the same rule applies; nothing changes.

Which sections are open by default: Name, Timing and the type's main section (Transform for a clip, Text for a text, Caption words for a caption); the open state per section is kept in `prefs.ts`.

### 3.2 Clip panel

```text
┌──────────────────────────────── 280 ┐
│ [Film] Clip · IMG_0421              │ 36 header
├─────────────────────────────────────┤
│ ▾ Name                              │ 32
│ Name      [ IMG_0421____________ ]  │
│ ▾ Timing                            │
│ Start              End              │ labels above (sub-grid)
│ [ 00:00.000 ]      [ 00:06.000 ]    │ TimeField × 2
│ Duration  00:06.000                 │ read only, plain text
│ In point  00:01.200                 │ read only
│ ▾ Transform                         │
│ X         [ 540     ] px  ◇         │ NumberField + keyframe marker (16 px diamond, aria-pressed)
│ Y         [ 960     ] px  ◇         │
│ Scale     [ 1       ] x   ◆         │ ◆ = keyframe at the playhead
│ Rotation  [ 0       ] °   ◇         │
│ Opacity   ══════════●═ 100 %  ◇     │ Slider + value
│ ▸ Transitions                       │
│ ▸ Audio                             │
│ ▸ Framing                           │
│ ▸ Speed                             │
│ ▸ Color                             │
│ ▸ Silences                          │
│ ▸ Auto captions                     │
│ ▸ Keyframes                         │
│ ▸ Template                          │
└─────────────────────────────────────┘
```

Sections, fields and commands are those of SDD-012 §12.1 (clip rows). Speed shows `NumberField` with the `x` suffix and a `Switch` "Ripple later items"; Auto captions shows max words, max chars, max gap, a `Switch` "Replace", the track `SelectField` and `Button` `secondary` "Generate"; while a transcription job runs, a `Progress` 4 px with the percentage replaces the button.

### 3.3 Text panel

```text
┌──────────────────────────────── 280 ┐
│ [Type] Text · Big sale              │
├─────────────────────────────────────┤
│ ▸ Name   ▾ Timing (as clip)         │
│ ▾ Text                              │
│ Content                             │ label above: full-width Textarea, 4 rows
│ [ Big sale                       ]  │ Ctrl+Enter or blur commits text.set {text}
│ [ this weekend                   ]  │
│ Font      [v Inter              ]   │ bundled families first, names in the UI font
│ Weight    [v 700                ]   │ only weights that exist for the family
│ Size      [ 72      ] px            │
│ Color     [■][ #FFFFFF    ]         │ ColorField: swatch opens native input, hex text
│ Align     [L][C][R]                 │ ToggleGroup, one value
│ Line h.   [ 1.2     ] x             │ label "Line height" in the accessible name
│ Max width [ 929     ] px            │
│ Uppercase [ ]                       │ Switch
│ ▸ Stroke  (color, width)            │ sub-fieldsets, collapsed
│ ▸ Shadow  (color, dx, dy, blur)     │
│ ▸ Background (color, padding, radius)│
│ ▸ Animation                         │
│ ▸ Transform  ▸ Transitions          │
│ ▸ Keyframes  ▸ Template             │
└─────────────────────────────────────┘
```

- The style fields are the TextStyle fields of SDD-001 §3.6; each commit sends the partial `style` with only the changed field (`text.set {item_id, style: {size: 80}}`).
- `E_FONT_UNKNOWN` reloads the font list and shows the `inline` error under Font with `details.available` as the select's first group, "Available".
- **Decision of this document.** Stroke, Shadow and Background are collapsible sub-groups inside the Text section (nested `fieldset` with `legend`), not separate sections, so the Text section reads as one unit and the 280 px panel is not a wall of 17 fields.

### 3.4 Caption panel and its variants

The caption panel has Name, Timing, Caption words, Caption style, Transform, Animation, Keyframes, Template. The word editor is the open choice: SDD-012 §12.1 lists a table with index, text, start, duration and a line-break toggle, and at 280 px those five columns leave the text 40 px.

```text
Variant A: table                              Variant B: list and word detail (recommended)
┌──────────────────────────── 280 ┐           ┌──────────────────────────── 280 ┐
│ ▾ Caption words (12)            │           │ ▾ Caption words (12)            │
│ #  Text        Start      ↵     │           │ 1  Hello        00:00.400  ↵    │ 32 rows, listbox
│ 1  Hello       00:00.400  [ ]   │           │▌2  there        00:00.720       │ selected (accent, bar)
│ 2  there       00:00.720  [ ]   │           │ 3  and          00:01.050       │
│ 3  and         00:01.050  [x]   │           │ ...  (rows scroll inside, max 8)│
│ ...                             │           │─────────────────────────────────│
│ (duration only at 360 px)       │           │ Word 2                          │
│ [Edit all words]                │           │ Text      [ there__________ ]   │
└─────────────────────────────────┘           │ Start     [ 00:00.720 ]          │
                                              │ Duration  [ 00:00.330 ]          │
                                              │ Line break before [ ]           │
                                              │ [Split before] [Merge with next]│
                                              │ [Edit all words]                │
                                              └─────────────────────────────────┘
```

- **A, table:** every word editable in place (a grid of inputs, roving focus). At 280 px the Duration column is dropped and appears only from 360 px; editing a word's duration needs the bulk editor. Dense, but many small inputs (12 to 60 words × 3 fields) in one tab sequence.
- **B, list and detail (recommended):** a `listbox` of words (index, text with `UntrustedText`, start in `tabular-nums`, `↵` icon with the text "line break" for words that start a line), then a detail form for the selected word with all fields of the row: Text (`caption.edit_word`; empty removes the word), Start and Duration (`caption.set_words` with the two neighbours adjusted, request composition only), Line break before (`caption.set_breaks`), "Split before" (`caption.split`) and "Merge with next" (`caption.merge`, enabled only when the next caption on the track exists). The word selected in the timeline (design system §4.3.6) and the word selected in the list are the same `uiStore.selection.word`.

**Recommendation: B.** It keeps every field at 280 px, makes the timeline word selection and the inspector one model, and gives the list one tab stop. "Edit all words" opens the bulk editor (a `Dialog` 640 px with a `font-mono` `Textarea` of `start duration text` lines, sent as one `caption.set_words`) in both variants.

Caption style: the CaptionStyle fields (TextStyle plus Position `SelectField`, Highlight `RadioGroup` None or Word, Highlight color and Highlight background `ColorField`, shown only when Highlight is Word), with a `Switch` "Use track style" at the top: when on, the fields show the track values as read-only text and the item sends `caption.set_style {style: null}`.

### 3.5 Multi-selection and the project panel

- Several items: header "3 items" with the type icons; Name and Timing hidden; Transform and Audio shown with the placeholder "Mixed" (`muted-foreground`) where values differ; a commit sends one batch with one operation per selected item.
- Nothing selected: the project panel (P2-UI-024): Name, Size preset `ToggleGroup` (9:16, 1:1, 16:9, 4:5), Width, Height, Background `ColorField`, Loudness target, Frame rate (opens `FpsConfirmDialog`), Template info.

### 3.6 Field states

| State | Display |
| --- | --- |
| Default | value from the read model |
| Focus | 2 px `ring` outline |
| Draft (typing) | the typed text; nothing sent |
| Busy (in flight) | `aria-busy="true"`, the draft kept, "Saving" text after 400 ms in the message line |
| Error | `aria-invalid`, `destructive` border, `ErrorNotice` `inline` (message and hint as received) in the message line, `aria-describedby` to it; the stored value returns |
| Changed by another actor while focused | the draft kept; message line: actor chip and "Changed by claude-code" in `agent`; commit then follows §4.5 |
| Mixed | empty value, "Mixed" placeholder |
| Read only | plain text, no field boundary |
| Disabled (offline, trashed, gating) | `aria-disabled`, tooltip with the reason |
| Animated property | keyframe marker filled when a keyframe exists at the playhead, outlined otherwise; the value shown is the plan's value at the playhead or empty, never a browser interpolation |

### 3.7 Keyboard and focus

- `Enter` on a selected timeline item moves focus to the first field of the inspector; `Esc` in the inspector (outside a draft) returns focus to the item.
- Inside: `Tab` through section headers and fields in visual order; `Enter` or `Space` on a header toggles it; `Enter` commits, `Esc` reverts, `Up` and `Down` step numbers (coalesced 400 ms), sliders step with arrows and commit on release.
- The word list (variant B) is one tab stop; `Up` and `Down` move the selected word, which also selects it in the timeline.

### 3.8 Contrast notes

Labels are `muted-foreground` on `ed-panel`: 6.00 light, 7.25 or more dark. Field boundaries use `input`: 3.64 and 3.77 against the background. The keyframe marker is a graphical object in `foreground`, well above 3:1. The "changed by" note in `agent` is 5.58 and 9.47 on `ed-panel`.

## 4. History panel and activity badge

### 4.1 Activity badge in the top bar (D-030)

`ActivityIndicator` is one `Button` `ghost` `sm`; it never splits into several badges.

| Variant | Text | Dot | Text colour |
| --- | --- | --- | --- |
| One actor active (younger than 3 s) | "claude-code is editing" | 8 px filled `agent` | `agent` |
| Several active | "claude-code and 1 other are editing" (the latest remote batch names the actor; `params.count` for the others; pt-BR "claude-code e mais 1 estão editando") | filled | `agent` |
| Settled | "Last edit by claude-code, 12 s ago" (updated every 10 s) | 8 px outlined `agent` | `muted-foreground` |
| Hidden | nothing after 5 min, or when only "you" edited | | |

Popover (level 1, 360 px wide, opened by the badge):

```text
┌──────────────────────────────────────── 360 ┐
│ Active now                                   │ text-xs 600 muted
│ [Bot claude-code]  changed 3 items   2 s ago │ one row per active actor, 40 px
│ [KeyRound key ab12] added 1 track    1 s ago │
├──────────────────────────────────────────────┤
│ Latest changes                               │
│ r42 [Bot claude-code] moved itm_m1n2…  2 s   │ 20 latest entries, 32 px
│ r41 [KeyRound key ab12] track.add …   1 s    │
│ r40 [User you] split IMG_0421         1 min  │
│ ...                                          │
├──────────────────────────────────────────────┤
│ [Open history]                               │ link button: opens the History tab
└──────────────────────────────────────────────┘
```

### 4.2 History panel

The `History` tab of the left panel (240 px), `HistoryPanel`, 50 rows per page, newest first.

```text
┌──────────────────── 240 ┐
│ [Library][History]      │ Tabs
├─────────────────────────┤
│ r42 [Bot claude-code]   │ row, line 1: revision, actor chip, time (right)
│              2 s ago    │
│ Moved IMG_0421 to 00:1… │ line 2: first line of description (lang="en")
├─────────────────────────┤ 56 per collapsed row
│ r41 [KeyRound key ab12] │
│ Added track Captions    │
├─────────────────────────┤
│ r40 [User you]   1 min  │
│ Split IMG_0421 at 00:0… │
├─────────────────────────┤
│ r39 [User you]   2 min  │
│ ~~Trimmed music.wav~~   │ undone: strikethrough + "Undone" Badge
│ [Undone]                │
├─────────────────────────┤
│ ▾ r38 [Bot claude-code] │ expanded: full description, text-sm, wraps
│ Generated 14 captions   │
│ from IMG_0421 on track  │
│ Captions; removed 2 old │
├─────────────────────────┤
│ [Load more]             │ secondary, full width
└─────────────────────────┘
```

- Row (`role="listitem"` holding a `button` with `aria-expanded`): a two-line grid `grid-template-columns: auto 1fr auto` for line 1 (revision `tabular-nums` `muted-foreground`, actor chip, relative time `text-xs muted-foreground tabular-nums`) and line 2 spanning all columns (`UntrustedText`, `text-sm`, one line, ellipsis). Collapsed height 56 px; see Q3.
- Undo and redo steps (`kind` `undo`, `redo`) show a leading `Undo2` or `Redo2` 12 px icon and the description the core gives.
- A row prepended from a `batch.committed` event is provisional and drawn the same way; the refetch replaces it within 250 ms. No visual "pending" mark: the data already comes from the core.
- Other actors' rows are not dimmed (SDD-012 §15).

States:

| State | Display |
| --- | --- |
| Loading | 4 `Skeleton` rows, `aria-busy` |
| Empty | "No changes yet" / "Every change by you or an agent is listed here." |
| Error loading | `ErrorNotice` `block` with "Try again" |
| Loading more | "Load more" is `aria-busy`, keeps its label |
| End of history | "Load more" removed; text "Start of the project" in `muted-foreground` |
| Gap event | one refetch; no visual change except the new rows |

Undo with skipped changes: a `status` toast "Undo applied, 2 changes were left alone because claude-code changed them" (`ui.editor.history.undoSkipped`, plural object), with the actor chip.

### 4.3 Keyboard and focus

- The tabs are a `tablist` (arrows switch, automatic activation); `Alt+3` opens History and moves focus to the first row.
- The rows form a list; `Tab` reaches each row button, `Enter` or `Space` expands and collapses. Proposal for the accessibility pass: one tab stop with `Up` and `Down` between rows (Q4).
- The badge opens the popover with `Enter` or `Space`; focus goes to the first row of the popover; `Esc` closes and returns focus to the badge.
- Live region: "claude-code changed 3 items", at most once every 2 s.

### 4.4 Contrast notes

The agent chip `bg-agent/10 text-agent` and the state tints are 4.84 to 5.15 light and 5.68 to 6.31 dark (design system §2.6). The "you" chip uses `secondary` (16.42 and 14.48). Undone rows keep full text colour; the strikethrough and the "Undone" badge carry the state, so no contrast is lost.

## 5. Export dialog and job progress

### 5.1 Layout

`ExportDialog`: `Dialog` level 2, 560 px wide, centred, `max-height: calc(100vh - 48px)`; header 56 px, body scrolls inside, footer 64 px. At 1280 × 720 the body holds up to 504 px. The dialog has one layout and seven states (SDD-012 §14.1); there is no open variant.

```text
Ready, with warnings
┌───────────────────────────────────────────────────────── 560 ┐
│ Export                                                 [i:X] │ text-lg 600
│ MP4 · 1080×1920 · 30 fps · 00:28.000 · revision 42           │ text-sm muted, tabular-nums
├──────────────────────────────────────────────────────────────┤
│ [i:TriangleAlert] Warnings (2)                               │ group header, text-sm 600, warning icon
│   L_SAFE_AREA  Text outside the safe area                    │ rule (font-mono xs) + ui.editor.lint.<rule>
│   caption text spans y 1180-1330 of 1920; the safe area...   │ core message, lang="en"
│   How to fix: set the caption track style position...        │ hint, muted
│   [Go to itm_s6t7v8w9x0]                                     │ link: closes the dialog, selects the item
│   L_SILENT_EXPORT  No audio in the export                    │
│   ...                                                        │
│ [i:Info] Notes (0)  ← group omitted when empty               │
├──────────────────────────────────────────────────────────────┤
│                                   [Re-check]  [Start export] │ secondary, default
└──────────────────────────────────────────────────────────────┘
```

### 5.2 The seven states

```text
Checking                                  Blocked
│ [i:Clock] Checking the project…        ││ [i:CircleAlert] Errors (1)                         │
│ (no bar, no spinner)                   ││   L_EMPTY_TIMELINE  The timeline is empty          │
│                [Re-check: disabled]    ││   How to fix: import media and place it with...    │
│                [Start export: disabled]││ Fix the errors above, then check again.            │ reason line, text-sm
                                          │              [Re-check]  [Start export: disabled]  │

Running                                   Succeeded
│ Exporting · revision 42                ││ [i:CircleCheck] Export finished                    │ success icon
│ ════════════════════░░░░░░░░  42%      ││ C:\Users\…\exports\spring-launch.mp4  [i:Copy]     │ font-mono, copy button
│ encoding video, frame 352 of 840       ││ 18.4 MB · 00:28.000 · revision 42                  │
│ You can close this dialog; the export  ││ Checks: duration ✓  streams ✓  loudness ✓          │ verification checks as text
│ keeps running.                         ││ [i:TriangleAlert] The project changed after this   │ only when the read model is newer
│                   [Cancel export]      ││ export (now revision 45).                          │
                                          │                      [Close]  [Download]            │

Failed                                    Cancelled
│ ErrorNotice block:                     ││ [i:CircleSlash] Export cancelled                   │ muted icon, not an error
│ [i:CircleAlert] Verification failed    ││ No file was written.                               │ text from the core when present
│ the exported file has 838 frames...    ││                                                    │
│ How to fix: …                          ││                                                    │
│ E_VERIFY_FAILED                        ││                                                    │
│ Failed checks: frame_count, duration   ││                                                    │ details.checks as a list
│      [Copy details] [Try again]        ││                       [Close]  [Export again]      │
```

| State | Primary button | Secondary | Progress | Notes |
| --- | --- | --- | --- | --- |
| Checking | Start export, `aria-disabled` | Re-check, `aria-disabled` | none | "Checking the project" text, static `Clock` icon |
| Ready | Start export | Re-check | none | the empty state "No problems found" when there are no findings |
| Blocked | Start export, `aria-disabled`, with the reason line | Re-check | none | errors first, then warnings, then notes |
| Running | none | Cancel export (`outline`) | `Progress` 8 px with percentage and the core's `message`; "Waiting for the server" with an empty bar before the first event | the dialog may be closed; the jobs chip keeps the job |
| Succeeded | Download | Close | none | path, size, duration, revision, checks, newer-revision warning |
| Failed | Try again (goes to Checking) | Copy details, Close | indicator `destructive` at the last value | `ErrorNotice` `block` |
| Cancelled | Export again (goes to Checking) | Close | none | not a toast, not an error |

Errors at start: `E_LINT_BLOCKING` replaces the findings with `details.findings` and returns to Blocked; `E_QUEUE_FULL` shows the running jobs from `details.running` and "Try again"; `E_FFMPEG_MISSING` and `E_NO_ENCODER` show their hint and no retry.

### 5.3 Jobs chip

`JobsChip` in the top bar: `Button` `ghost` `sm` "2 jobs · 64%" (`tabular-nums`) with a 2 px bar on its bottom edge (`primary` on `muted`). With one export it opens the dialog at Running; otherwise a popover (level 1, 320 px) lists each job: kind and name, `Progress` 4 px, percentage, and Cancel for exports. After a reload, rehydrated jobs appear in the chip at once (P2-UI-028).

### 5.4 Keyboard and focus

- Opening: focus goes to the primary button when enabled, else to Re-check, else to the first finding link. In Running, to Cancel export.
- `Tab` order: close button, findings (each link), Re-check, primary. `Esc` closes in every state (an export keeps running).
- Live region: "Exporting, 40%" at 10% steps; "Export finished" or the error headline once.
- After a finding link, the dialog closes and focus goes to the selected timeline item.

### 5.5 Contrast notes

The `Progress` indicator `primary` on its `muted` track is at least 3:1 in both themes (design system §4.3.10). The group icons carry the severity together with the group title, never colour alone. Paths in `font-mono` use `foreground`.

## 6. Timeline direct manipulation

Everything here follows the gesture life cycle of SDD-012 §8.1: the read model does not change until the core answers; the UI draws a ghost of the requested value, never a predicted result.

### 6.1 Selection, focus and marquee

```text
Ruler  0:00      0:05      0:10      0:15
Video  [Film IMG_0421 ───────][▓▓ IMG_0433 ▓▓▓▓▓▓][Film IMG_0440 ──]   ▓ = selected (2 px inset ring ed-selection)
Music  [AudioLines music.wav ∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿∿]
                 ┌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌┐                                marquee: 1 px dashed ring, fill ring/10
                 ┆   (empty space drag)  ┆                                selects every item it touches
                 └╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌┘
```

| State | Drawing |
| --- | --- |
| Default | `ed-item-<type>` fill, 1 px `ed-track-line` border, `rounded-xs`, type icon and label |
| Hover | border `-fg` at 60%; trim handles appear (6.2) |
| Focus (grid cell) | 2 px `ed-selection` outline, 1 px offset |
| Selected | 2 px `ed-selection` inset ring, `aria-selected="true"`; selected and focused can show together (ring inside, outline outside) |
| Selected track | header `bg-accent` and a 2 px leading bar in `ed-selection` |
| Marquee | 1 px dashed `ring`, fill `ring` at 10%; screen hit test only |
| Hidden track | fill mixed 50% with `ed-timeline`, label at full `-fg` |

### 6.2 Trim handles

```text
             ‖◀ 24 px hit area ▶‖
Video  [Film ‖ IMG_0433 ───────────── ‖]          handles: 8 px visual edge zones inside the block
             ^ start handle           ^ end handle
             cursor ew-resize, visible on hover and when selected

During a trim of the end edge:
Video  [Film IMG_0433 ──────────────]╌╌╌╌╌╌╌╌╌╌┆          ghost: 2 px dashed ed-ghost, fill ed-ghost at 15%
                                     ╰ ghost to ┆ 00:11.000 (+00:02.000)    readout chip (6.3)
                                                ┆ snap line at IMG_0440 start
Clamp: the ghost stops at limits.trim_end_max from the snap points; past it the ghost does not grow (display only).
```

- Handle visual: an 8 px zone at each edge of the block in `-fg` at 30% fill with a 2 px vertical grip line in `-fg` (graphical object, 3:1 or more against the fill); not drawn when the block is narrower than 32 px (then only the 24 px hit area outside the block edge works, and `[` `]` remain).
- Trim handles have no tab stop: the keyboard equivalent is `[` and `]` (to the playhead) and the Timing fields of the inspector.

### 6.3 Move, snapping and the drag readout

```text
Ruler  0:00      0:05      0:10┆     0:15
Video  [Film IMG_0421 ───────] ┆[▓▓ IMG_0433 ▓▓▓▓▓]     original stays where the read model has it
                               ┆╭╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╮
                               ┆┆ IMG_0433 (ghost) ┆      ghost at the snapped frame, on the target row
                               ┆╰╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╯
                               ┆[ 00:08.000 · snap: IMG_0421 end ]   readout chip, popover surface, text-xs tabular-nums
                               ┆ snap line: 1 px ed-snap, full height, above items, under playhead
```

- The ghost layer (z 3) is drawn over the items; the original block keeps its full colour (it is still the truth).
- Snap targets by precedence: playhead, frame 0, core points (SDD-012 §7.7); `Alt` held suspends snapping and the readout says "snap off"; the toolbar `Snap` toggle (`Magnet`, `aria-pressed`) and `N` switch it.
- **Decision of this document.** A drag readout chip follows the ghost: the requested start (or edge) as a timecode and, when snapped, the target kind ("snap: playhead", "snap: IMG_0421 end"). It shows the value the gesture will send, formatted by `formatTime` in `units.ts`, never a result. The delta is shown only for trims, as "+00:02.000", computed by `units.ts` (see Q5).
- Edge auto-scroll within 40 px of the timeline edge jumps by one column per frame of the pointer loop (no smooth scroll).

### 6.4 After release: pending, success, snap-back

```text
Pending (sent, waiting for the core)        E_OVERLAP: ghost removed, item back, conflict outlined
Video [Film IMG_0421]╭╌╌╌╌╌╌╌╌╌╌╌╮         Video [Film IMG_0421 ───][▓ IMG_0433 ▓]┃IMG_0440 (2 px ed-error-flash, 2 s)┃
                     ┆ pending   ┆
                     ╰╌╌╌╌╌╌╌╌╌╌╌╯          Toast (error, stays):
the dashed ghost stays at the released     │ [i:CircleAlert] Items would overlap                       │
place; the original is unchanged           │ itm_… would overlap itm_… on trk_v1 from 9s/270f to 10s/300f│ core message
                                           │ How to fix: move it to 12.4s/372f or later                 │ core hint
Success: the ghost is removed in the same  │ E_OVERLAP                                                  │
React batch that draws the item at its new │ [Move to next free position]  [Dismiss]                    │ sends item.move start "<next_free>f"
place.
```

| Answer | Timeline | Message |
| --- | --- | --- |
| Success | ghost removed, item at the core's place; live region "Moved to 00:08.000" | none, or a `status` toast with the `effective` value for inspector timing fields |
| `E_OVERLAP` (move, trim, add) | ghost removed; the conflicting item outlined 2 px `ed-error-flash` for 2 s and scrolled into view | error toast with "Move to next free position" when `details.next_free` exists |
| `E_OUT_OF_SOURCE` (trim) | ghost removed, item unchanged | error toast; headline "Not enough source media", body "the source has only `<details.available>`" |
| `E_TYPE_MISMATCH` (move to another track kind) | ghost removed | error toast, no action |
| `E_INVALID_STATE` (trim with a transition that no longer fits) | ghost removed | error toast with the hint |
| `E_REVISION_CONFLICT` | ghost removed; item outlined `ed-error-flash` 2 s | conflict notice (6.5) |

### 6.5 Revision conflict and remote edits

```text
Remote edit lands (no gesture)               Conflict notice (warning toast, stays until acted on)
Video [Film IMG_0421]┃IMG_0433┃              ┌──────────────────────────────────────────── 360 ┐
            2 px agent outline, 1.5 s        │ [i:TriangleAlert] Changed by [Bot claude-code]   │
            gridcell name gains              │ Your move of IMG_0433 was not applied.           │ UI text
            ", changed by claude-code"       │ ┌──────────────────────────────────────────────┐ │
                                             │ │ ~ itm_… start 8s/240f -> 9s/270f             │ │ details.diff, font-mono xs,
Remote edit of the item being dragged:       │ └──────────────────────────────────────────────┘ │ max 6 lines, then "Show all"
gesture cancelled, ghost removed, toast      │                       [Dismiss]  [Apply again]   │ Apply again: new key, base = current
"claude-code changed IMG_0433; your drag     └──────────────────────────────────────────────────┘
was cancelled" with the actor chip.
```

- "Apply again" sends the same operations as a new logical edit; nothing is resent automatically (RF-119). While it is in flight the button is `aria-busy`.
- Several conflicts in a row replace the notice (one notice at a time, the latest), so the user never acts on a stale diff.

### 6.6 Keyboard equivalents

| Pointer gesture | Keyboard |
| --- | --- |
| Click, `Shift+click`, `Mod+click` | `Enter` selects, `Shift+Left` and `Shift+Right` extend, `Mod+A` selects all |
| Move | `Alt+Left` and `Alt+Right` (1 frame), `Shift+Alt` (1 second), `Alt+Up` and `Alt+Down` (track) |
| Trim | `[` and `]` to the playhead; inspector Start and End |
| Cancel gesture | `Esc` (nothing sent) |
| Snap toggle | `N` |
| Conflict notice | `F8` focuses the toast viewport; `Tab` to "Apply again"; `Esc` dismisses |

Focus after an error stays on the item that was moved; after `Delete` it goes to the next item on the track, else the previous, else the header.

### 6.7 Contrast notes

Ghost 3.82 and 3.78, snap line 5.17 and 11.86, error outline 4.70 and 6.30, agent outline 4.82 and 9.84 against `ed-timeline`, and 3.00 to 4.43 against the item fills (design system §2.6). The selection ring against the closest fill (text items) is 4.40 light and 3.93 dark. The readout chip uses `popover` and `foreground`. Every state also has a non-colour cue: dashed versus solid outline, `aria-selected`, the toast text, and the changed-by name.

## 7. Cross-screen accessibility

- Landmarks and `F6` order: `header` top bar, `aside` "Library", `main` (preview `region` "Preview", timeline `region` "Timeline"), `aside` "Inspector". Dialogs (palette, export, bulk word editor) trap focus and return it to the opener.
- Every icon-only button has an `aria-label` from `t()` and a tooltip "Label (Shortcut)".
- Targets: 32 px default controls; 24 px minimum (`icon-xs` toggles in track headers); handles 8 px visual with a 24 px hit area.
- Nothing is conveyed by colour alone: item type by icon and label; selection by ring and `aria-selected`; connection state by text; severities by icon and group title; actors by chip text and icon.
- pt-BR: no fixed widths on text buttons; the top bar budget of 1.3 holds with 35% expansion; dialog footers wrap to a second row instead of truncating.
- Forced colours: selection and focus use `outline`; the inset ring adds `outline: 2px solid Highlight`.

## 8. Variant recommendations

| Screen | Variants | Recommended | Reason |
| --- | --- | --- | --- |
| Main layout | A: timeline height by width only; B: 300 px when the viewport is 880 px high or more | A | the 9:16 preview is height-bound; no spec change |
| Command palette | A: batch footer tray; B: side column 280 px | A | fits 1280 × 720 and the drawer widths; "Preview batch" shows the core's diff |
| Inspector, caption words | A: inline table; B: list and word detail | B | keeps every field at 280 px; one selection model with the timeline |
| Inspector clip and text, history, export, timeline states | one design each | | the specifications leave no real choice |

## 9. Decisions of this document

| # | Decision | Section | Should be recorded in |
| --- | --- | --- | --- |
| AE-1 | Top-bar width budget and shrink order; the palette button collapses to its icon below 1440 px | 1.3 | design system §4.3.3 |
| AE-2 | Start and End share a row as a two-column sub-grid with labels above, at every inspector width | 3.1 | design system §4.3.8 |
| AE-3 | Stroke, Shadow and Background are collapsible sub-groups of the Text section | 3.3 | SDD-012 §12.1 |
| AE-4 | Palette batch as a footer tray (variant A) with removable chips and "Preview batch" | 2.4 | design system §4.3.11 |
| AE-5 | Caption words as a list with a word detail form (variant B) and a bulk editor dialog | 3.4 | SDD-012 §12.1, design system §4.3.8 |
| AE-6 | History rows are two lines, 56 px collapsed, at the 240 px panel width | 4.2 | design system §3.3 and §4.3.16 (Q3) |
| AE-7 | Drag readout chip with the requested time and the snap target | 6.3 | SDD-012 §8.1 (Q5) |
| AE-8 | One conflict notice at a time, the latest replaces the previous | 6.5 | design system §4.3.14 |
| AE-9 | The Cancelled export state shows "Export again" and does not raise a toast | 5.2 | SDD-012 §14.1 |
| AE-10 | Activity popover: "Active now" rows first, then "Latest changes" with revision numbers, then "Open history" | 4.1 | design system §4.3.4 |

## 10. Open questions

1. **Q1, visual check.** These are text wireframes; no PNG and no high-fidelity frames exist because OpenPencil was not available. Should the designer produce the visual version in OpenPencil before senior-e starts P2-UI-003, or does senior-e build from this text and the design system, and design reviews the running build?
2. **Q2, timeline height (variant B).** Keep the width-only rule of SDD-012 §6.3 (recommended), or add the height-aware default?
3. **Q3, history row height.** Design system §3.3 says 40 px. At 240 px the revision, the actor chip, the time and the description do not fit on one line, so this document uses 56 px two-line rows. Accept 56 px and update the design system, or show the history at 40 px only when the panel is 320 px wide?
4. **Q4, history list keyboard.** One tab stop per row (simple, many stops) or a single list stop with `Up` and `Down` (consistent with the palette and the word list)? This document proposes the single stop for P2-UI-030.
5. **Q5, drag readout and rule R5.** The readout formats the requested frame with `formatTime`, and the trim delta needs one subtraction of two frames. Does the architect accept the delta inside `units.ts` (the allowlisted file), or is the readout limited to the absolute time?
6. **Q6, filter by actor.** With several agents, filtering the history by actor would help reading. It is not in SDD-012 §15 and the history endpoint has no actor filter. Leave it for BETA?
7. **Q7, safe-area guide colour.** The light `ed-safe-area` is 3.03:1 on the preview surround. Since the surround is the same in both themes, use the dark value (9.65:1) in both themes (design system §10 item 11)?
8. **Q8, palette scope badge text.** The badge shows the raw scope (`edit`, `media`, `read`). Should it be localized (`ui.editor.scopes.<scope>`) or stay as a code-like token in `font-mono`, like command names?
9. **Q9, bulk word editor format.** The proposed text format is one `start duration text` line per word. Is a plain text format acceptable for `caption.set_words`, or should the bulk editor be the schema form's array editor?

### 10.1 Answers (orchestrator, 2026-11-14)

The open questions are cheap to undo, so they are decided here and binding for P2-UI tasks until a review of the running build says otherwise.

1. **Q1:** senior-e builds from this text and the design system; design reviews the running build. No OpenPencil pass is required first.
2. **Q2:** keep the width-only rule (variant A); SDD-012 §6.3 is unchanged.
3. **Q3:** accept 56 px two-line history rows; design system §3.3 is updated with the history panel as the exception when P2-UI-030 lands.
4. **Q4:** one tab stop for the list, `Up` and `Down` move inside it (roving tabindex), as in the palette and the word list.
5. **Q5:** the readout shows only the absolute requested time and the snapped target; no delta, so rule R5 needs no exception.
6. **Q6:** filter by actor is left for BETA.
7. **Q7:** use the dark `ed-safe-area` value in both themes; design system §10 item 11 is closed by this.
8. **Q8:** the scope badge stays a code token in `font-mono`, not localized, like command names.
9. **Q9:** the bulk word editor is the schema form's array editor; no plain-text format.

Variants: main layout A, palette batch A (footer tray), caption words B (word list with a detail form).
