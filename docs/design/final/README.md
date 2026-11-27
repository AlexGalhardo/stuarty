# Final Design Source (`Trilha`)

This directory holds the canonical, version-controlled UI design source for Stuarty so that every new screen and layout change stays synchronized with the shipped product.

## Files

| File | Purpose |
| --- | --- |
| `stuarty-redesign-final-trilha.figcraft` | Master editable FigCraft file containing the design system (`01 Design System`), desktop (`1440x900`), compact (`1280x800`), and mobile (`390x844`) frames in both dark and light themes, state galleries, screen-recording frames (`24a`–`24f`), text-project flows (`33a`–`33h`), and landing frames. |
| `tokens-tailwind-final.css` | Canonical Tailwind CSS v4 semantic token sheet matching `web/resources/css/app.css` (OKLCH semantic surfaces, text, borders, focus ring, indigo accent, teal agent hue, semantic statuses, shadows, radii, and motion tokens). |

Bulk exported PNG captures are intentionally omitted from version control to keep repository size small; `stuarty-redesign-final-trilha.figcraft` is the single editable source of truth.

## Design principles

1. **One primary action per screen.** Every screen surfaces one clear main action in plain language.
2. **Explain in place.** Guidance, empty states (3 numbered steps), and next steps live directly inside each screen rather than in pop-up tours.
3. **Agent visibility at a glance.** The persistent `SceneBar`, `SkillBadge`, and `ToolBadge` show the active agent, loaded skill, tool call, and waiting requests without opening a separate drawer.
4. **Human approval boundary.** Agents prepare and request; only a person approves, publishes, starts capture, or raises a cost ceiling.
5. **Confirmed state only.** The UI draws what the core and service confirmed—never optimistic edit results (ADR-0007).
6. **Calm hierarchy.** Inter (SIL OFL 1.1) for UI and headings, Liberation Mono (SIL OFL 1.1) for tools, code, and timecodes, WCAG 2.2 AA contrast in both dark (default) and light themes, and colour used only when it carries meaning.

## Frame inventory in `stuarty-redesign-final-trilha.figcraft`

- **Design system (`01 Design System`)**: `primitive`, `theme` (`dark` / `light`), and `scale` variable collections, typography ramp (`h1` 30/38 down to `body-sm` 14/20 in dashboard and `editor-sm` 12/16 in editors), and 69 component sets (`SceneBar`, `TrailDetail`, `ApprovalNotice`, `NotificationBell`, `Sidebar`, `TopBar`, `EmptyState`, `NumberedStep`, `MediaCard`, `SoundRow`, editor panels, and site components).
- **Product screens (104 frames across dark and light at `1440x900`)**: core sections 1–21 (including Publishing in month, week, and list views), Production workflow (`22`), Notification center (`23`), Screen recording (`24a`–`24f`), Finishing & editor stages (`25`–`30`), Local models (`31`), External motion (`32`), and Text-only project flow (`33a`–`33h`).
- **States (133 frames)**: empty, loading, error, and all five agent states (`working`, `waiting`, `denied`, `done`, `failed`).
- **Responsive (`1280x800` and `390x844`)**: Home, Chat, Approvals, Publishing, Notification center, drawer navigation, and mobile Screen Recording notice.
- **Activity History (`34a` and `34b`)**: global and per-project views at `1440x900` and mobile `390x844`, in English and Brazilian Portuguese, in both themes. The editable frames use the shared color tokens.
- **Landing & docs frames**: English marketing and documentation shells at `1440` and `390` in both themes.

The 390 px screens use the top bar for Chat and Notifications. The agent scene bar appears on desktop and wider layouts.

## Updating the design in future UI epics

Every epic that introduces a new screen or reshapes an existing layout (for example standalone editors, OAuth-first agent connections, branded provider cards, Activity History, `/record` Screen Recording studio, or `/edit-sound` Sound Editor) updates `stuarty-redesign-final-trilha.figcraft` in the same epic before final review, and keeps `tokens-tailwind-final.css` aligned with `web/resources/css/app.css` and `docs/design-system.md`.
