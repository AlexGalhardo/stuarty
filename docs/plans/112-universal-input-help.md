# 112: Universal input placeholders, contextual tooltips and explanatory help cards

## Goal

Every single form input across Stuarty—from account setup and project creation to connections, AI generation, writing, publishing, asset details, settings, video editor, and image editor—shows a concrete example placeholder AND provides an accessible contextual tooltip or concise explanatory help card describing exactly what the input does and what format it expects, so first-time users never have to guess.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  - **Feedback #17**: Every input in the application must have example placeholders AND tooltips / alerts / cards explaining in detail what that input does and asks for, making every screen simple and self-explanatory for users.
- Current codebase audit:
  - Many inputs across `Account/` (`SignIn.tsx`, `Create.tsx`, `Recover.tsx`), `Projects/` (`Show.tsx`, `ProjectContextForm.tsx`, `Briefing.tsx`), `Connections/` (`Index.tsx`, `ProviderCards.tsx`, `SocialSection.tsx`, `BlogSection.tsx`), `AssetLibrary.tsx` (description, rights, tag add), `Publish/`, `Write/`, `Library/` (Skills, Prompts, Memory), `Settings/`, `Webhooks/`, `EditImage/`, and `editor/` currently have **no `placeholder`** attribute at all and **no tooltip / contextual help text** explaining what the field means or giving a concrete example!
- Design system & accessibility alignment (`docs/design-system.md`, WCAG 2.2 AA, SDD-012 §18.10):
  - Never rely on placeholder text alone as a label (keep every `<Label htmlFor>` intact).
  - Tooltips must be reachable by both mouse hover AND keyboard focus (Radix `<Tooltip>` with an accessible info trigger button `<button type="button" aria-label="...">` or `aria-describedby` pointing to helper text), never trapping focus and meeting 4.5:1 contrast in both light and dark themes.
  - Complex forms (connections/OAuth setup, SMART project goals, prompt `{{variables}}`, cost ceilings, webhooks, export bitrates/codecs, recurring calendar slots) pair input placeholders + field tooltips with a short top-of-form contextual help card (`<Notice>` / `<FieldHelpCard>`).

## Scope

- Modules owned:
  - `web/resources/js/components/FieldHelp.tsx` (new shared primitives: `<FieldLabel>` with integrated accessible `<Info>` tooltip trigger + `<FieldHint>` inline helper / `<HelpCard>` explanatory alert card, following `docs/design-system.md`).
  - `web/resources/js/pages/**` (audit and enrich every `<Input>`, `<Textarea>`, and `<select>` across: `Account/`, `Auth/`, `Projects/`, `Write/`, `Publish/`, `Approvals/`, `Connections/`, `Webhooks/`, `Library/`, `Skills/`, `Prompts/`, `Memory/`, `Settings/`, `EditImage/`, `EditVideo/`, `VideoAssets/`, `VisualAssets/`, `SoundAssets/`, `Dashboard/`).
  - `web/resources/js/assetlibrary/**`, `web/resources/js/dashboard/**`, `web/resources/js/editor/**`, `web/resources/js/image-editor/**`.
  - `web/scripts/check-input-help.ts` (new static AST/source check wired into `package.json` `check:input-help` to verify every text `<Input>` and `<Textarea>` in `web/resources/js/`—excluding hidden/file inputs—has a localized `placeholder` AND an associated tooltip or help description, preventing future regressions!).
  - `web/resources/lang/en.json` and `web/resources/lang/pt-BR.json` (complete English and Brazilian Portuguese placeholders, tooltips, and help cards).
  - `docs/design-system.md` and `docs/sdds/012-ui-architecture.md` (document the mandatory input-help pattern).
- Must not touch: `core/`, `service/`, `engine/`, `renderer/`, `desktop/`.

## Out of scope

- Changing backend validation rules or API schemas.
- Translating backend error messages (owned by epic 113).

## Dependencies

- Epics 74, 99, and 116 (UI foundation tokens/components, final design, and versioned FigCraft design in `docs/design/final/stuarty-redesign-final-trilha.figcraft` — add the `FieldHelp` tooltip/help-card variants to page `01 Design System`). Should run after (or be re-verified after) UI-heavy epics 103, 105, 106, 107, 109, 110, 111, 117, 118 so newly added forms are covered by `check:input-help`.

## Technical design

1. **Shared accessible field-help primitives (`web/resources/js/components/FieldHelp.tsx`)**:
   - `<FieldLabel htmlFor={id} label={...} help={...} />`: renders the `<Label>` alongside a subtle `?` / `Info` icon button with a Radix `<Tooltip>` (works on hover AND keyboard focus/tap, `aria-label={t("ui.common.more_info", { field: label })}`), showing the detailed explanation of what the input does, why it matters, and what format it expects.
   - `<FieldHint id={...}>`: concise visible helper line below complex inputs (wired via `aria-describedby` on the input so screen readers announce it automatically).
   - `<HelpCard title={...} description={...} examples={...} />`: clean collapsible or compact info card at the top of multi-field forms (e.g., Project SMART Goals, Prompt Templates `{{variable}}`, Connections & API Keys, Cost Ceilings, Webhooks, Export Options, Recurring Slots) giving concrete usage examples in plain language.

2. **Full-surface input audit (zero unguided inputs)**:
   - Group 1 (Onboarding & Account): Create account, Sign In, Recover, Network Login, Settings (idle timeout, locale, password change, API keys, brand kit).
   - Group 2 (Projects, Writing & Publishing): New Project, Project Context / SMART Goal, Briefing, Add/Rename Item, Write editor & captions, Publication editor, Recurring Slots, Day Notes.
   - Group 3 (Assets, Library & Agents): Asset Library filters/tags/description/rights/rename, Generate dialog, Skills/Prompts/Memory editors & search, Chat setup & prompts, Approvals & cost ceiling rules.
   - Group 4 (Connections & Webhooks): Custom Connection, AI Provider API keys & model price editor, Social & Blog setup guides, Webhooks.
   - Group 5 (Video Editor & Image Editor): Export options, Inspector numeric/text/color/caption inputs, Image Editor create/import/layer text/dimensions/adjustments.

3. **Automated enforcement (`web/scripts/check-input-help.ts`)**:
   - Parses `web/resources/js/**/*.tsx` (excluding `*.test.tsx` and `components/ui/**`) and fails if any user-editable `<Input>` (not `type="file"` / `type="hidden"` / `type="checkbox"` / `type="radio"`) or `<Textarea>` lacks a `placeholder` prop or lacks an adjacent `<FieldLabel help={...}>` / `<FieldHint>` / `aria-describedby` help element.

## Tasks

- [x] 1. Document the mandatory input placeholder + tooltip/help-card standard in `docs/design-system.md` and `docs/sdds/012-ui-architecture.md`, and build `web/resources/js/components/FieldHelp.tsx` with unit and accessibility tests. Verify: `task lint:md` and `cd web && bunx vitest run resources/js/components/FieldHelp.test.tsx`.
- [x] 2. Enrich all inputs in Group 1 (Account, Auth, Settings, Brand Kit) and Group 2 (Projects, Write, Publish) with concrete localized placeholders, tooltips, and form help cards in `en.json` and `pt-BR.json`. Verify: `cd web && bun run typecheck && bun run check:i18n && bunx vitest run`.
- [ ] 3. Enrich all inputs in Group 3 (Assets, Library, Skills, Prompts, Memory, Chat, Approvals) and Group 4 (Connections, Providers, Social, Blog, Webhooks) with placeholders, tooltips, and help cards. Verify: `cd web && bun run typecheck && bun run check:i18n && bunx vitest run`.
- [ ] 4. Enrich all inputs in Group 5 (Video Editor & Image Editor inspectors, dialogs, forms) with placeholders and tooltips. Verify: `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n && bunx vitest run`.
- [ ] 5. Implement `web/scripts/check-input-help.ts` (wired to `bun run check:input-help` in `web/package.json`) and verify zero violations across the entire frontend, plus run axe accessibility checks on key pages in Playwright. Verify: `cd web && bun run check:input-help`, `task lint`, `task test:web`, and Playwright a11y specs pass.

## Test strategy

- Unit tests (Vitest): `FieldHelp.test.tsx` verifies tooltip opens on hover and keyboard focus, `aria-describedby` links hint text, and contrast/semantics pass; existing page tests updated cleanly.
- Static guard: `bun run check:input-help` guarantees 100% coverage of `<Input>` and `<Textarea>` elements across the codebase.
- E2E + Axe accessibility (Playwright): verify tooltips do not cause horizontal scroll at 320px/1280px, pass WCAG 2.2 AA axe checks in light and dark themes, and display Portuguese placeholders and explanations when locale is `pt-BR`.

## Acceptance criteria

1. `cd web && bun run check:input-help` exits 0 with zero unguided `<Input>` or `<Textarea>` elements across the entire UI.
2. Every input placeholder, tooltip, and help card has complete, natural wording in both `en.json` and `pt-BR.json` (`bun run check:i18n` exits 0).
3. Every tooltip trigger is keyboard-focusable and screen-reader accessible, and axe accessibility specs pass in both light and dark modes with zero regressions.
4. `task lint`, `task test:web`, and `cd web && bun run typecheck && bun run check:thin-client` exit 0.

## Risks and mitigation

- **Visual clutter on dense editor inspectors.** Mitigation: use compact inline `?` tooltip icons beside labels in dense video/image inspector panels (zero extra vertical space consumed!) and reserve full `<HelpCard>` banners for top-level multi-field forms.
- **Large diff touching `en.json` and `pt-BR.json`.** Mitigation: per `.agents/rules/plans.md` §4, never run in parallel with another epic modifying `web/resources/lang/*.json`; coordinate merge order with the orchestrator.

## Size

M

## Hand-off for review

The commits of `epic/112-universal-input-help`, output of `bun run check:input-help` and `bun run check:i18n`, Vitest/Playwright a11y outputs, and sample screenshots in `pt-BR` showing placeholders, tooltips, and help cards.
