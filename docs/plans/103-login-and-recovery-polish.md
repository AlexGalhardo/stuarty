# 103: Login, password recovery and account switching polish

## Goal

The Sign In and Password Recovery screens stay clean and focused: redundant lock warning banners and marketing taglines are removed, users can return directly from Password Recovery to Sign In with one click, and users can always switch accounts from the Login screen even when the studio was locked.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  1. **Feedback #3 (Login)**:
     - Remove `"O Stuarty está bloqueado"` + `"Bloqueio por inatividade. Seu trabalho está salvo."` (`ui.account.signin.locked_title` / `ui.account.signin.locked_intro` in `SignIn.tsx`).
     - Remove `"Bloqueado, nada do espaço fica acessível: nem agentes, CLI ou API."` (`ui.account.signin.locked_footer` in `SignIn.tsx`).
     - Add a "Back to Login" ("Voltar para Login") button on the Password Recovery screen (`Account/Recover.tsx`).
     - Always expose a "Switch account" ("Mudar de conta") button on the Login screen (`Account/SignIn.tsx`) so the user can enter different credentials or switch account even when the studio is locked (currently line 127 hides `other_account` whenever `locked` is true!).
  2. **Feedback #14 (Password recovery footer)**:
     - Remove `"No seu computador. Do seu jeito."` (`ui.account.footer`) from the footer of `Account/Recover.tsx`.
- ADR-0052 (local account encryption & unlock semantics stay unchanged — only the UI copy and navigation controls change; locking still wipes the in-memory data key and returns `E_LOCKED` on protected routes), SDD-012 §2.1.1.

## Scope

- Modules owned:
  - `web/resources/js/pages/Account/SignIn.tsx`
  - `web/resources/js/pages/Account/Recover.tsx`
  - `web/resources/js/pages/Account/Account.test.tsx`
  - `web/resources/js/pages/Auth/Login.tsx` (keep consistent switch-account/navigation ergonomics in network mode)
  - `web/resources/lang/en.json` and `web/resources/lang/pt-BR.json` (add `ui.account.recover.back_to_login`, `ui.account.signin.switch_account`, clean up unused locked copy keys if no longer referenced, verify with `bun run check:i18n`)
  - `docs/sdds/012-ui-architecture.md` §2.1.1
- Must not touch: `core/account/`, `service/account/`, `engine/`, `renderer/`, `desktop/`.

## Out of scope

- Changing the cryptographic lock/unlock model of ADR-0052 or allowing multi-tenant local accounts inside one encrypted workspace (the "Switch account" button toggles out of the pre-filled known username card so the user can type a username/email manually, or switch to another configured account/workspace when available).
- Adding desktop update controls to the Login screen (owned by epic 101).

## Dependencies

- Epics 57 and 84 (local account and account screens). Coordinate `SignIn.tsx` merges with epic 101.

## Technical design

1. **Clean Sign In screen (`web/resources/js/pages/Account/SignIn.tsx`)**:
   - Use the clean sign-in title and intro (`ui.account.signin.welcome` / `ui.account.signin.title` and `ui.account.signin.welcome_intro` / `ui.account.signin.intro_open`) whether `locked` is `true` or `false` — never render `locked_title`, `locked_intro`, or `locked_footer`.
   - Keep the lock/unlock submit button clear (`ui.account.signin.submit`), and omit the warning lock banner copy (`O Stuarty está bloqueado`, `Bloqueio por inatividade. Seu trabalho está salvo.`, `Bloqueado, nada do espaço fica acessível: nem agentes, CLI ou API.`).
   - Always show the "Switch account" (`ui.account.signin.switch_account` / `ui.account.signin.other_account`) button whenever `known !== null && !other` — remove the `!locked` guard at line 127! When clicked (`other === true`), show the username/email input AND a secondary "Use `<known>`" button to toggle back without reloading.

2. **Password Recovery screen (`web/resources/js/pages/Account/Recover.tsx`)**:
   - Remove `footer={t("ui.account.footer")}` from `<AuthCard>` so `"No seu computador. Do seu jeito."` / `"On your computer. Your way."` no longer renders in the form footer.
   - Add a clear "Back to Login" (`Voltar para Login` / `Back to Sign in`) button/link below the submit button (with `ArrowLeft` icon) pointing to `/account/signin`.

3. **i18n hygiene (`en.json`, `pt-BR.json`)**:
   - Add `ui.account.recover.back_to_login` (`"Back to Sign in"` / `"Voltar para Login"`) and ensure `ui.account.signin.other_account` reads clearly (`"Switch account"` / `"Mudar de conta"`).
   - Remove dead keys (`ui.account.signin.locked_title`, `ui.account.signin.locked_intro`, `ui.account.signin.locked_footer`) if nothing else references them, and verify with `bun run check:i18n`.

## Tasks

- [x] 1. Update SDD-012 §2.1.1 to document the streamlined Sign In card, always-available switch-account toggle, and Recover back-to-login button without footer tagline. Verify: `task lint:md` exits 0.
- [x] 2. Update `Account/SignIn.tsx` to remove the three locked-state copy strings and enable the "Switch account" button in both locked and unlocked states, with a toggle back to the known account. Verify: `cd web && bunx vitest run resources/js/pages/Account/Account.test.tsx`.
- [x] 3. Update `Account/Recover.tsx` to remove `footer={t("ui.account.footer")}` and add the "Back to Login" button linking to `/account/signin`. Verify: `cd web && bunx vitest run resources/js/pages/Account/Account.test.tsx`.
- [ ] 4. Update `en.json` and `pt-BR.json` and adjust any existing Playwright E2E assertions in `web/e2e/specs/` that looked for the removed locked banners. Verify: `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n && bunx vitest run` and the account Playwright spec pass.

## Test strategy

- Vitest unit tests (`Account.test.tsx`):
  - SignIn with `account.state === "locked"` renders neither `locked_title`, `locked_intro`, nor `locked_footer`.
  - SignIn with `account.state === "locked"` and a known username displays the "Switch account" button; clicking it reveals the identity input; clicking back restores the known account chip.
  - Recover renders the "Back to Login" link to `/account/signin` and does NOT render `ui.account.footer` (`"No seu computador. Do seu jeito."`).
- Playwright E2E: lock studio -> verify clean Sign In screen -> click "Switch account" -> navigate to `/account/recover` -> verify no footer tagline -> click "Back to Login" -> return to `/account/signin`.

## Acceptance criteria

1. On `/account/signin` (both locked and unlocked), the texts `"O Stuarty está bloqueado"`, `"Bloqueio por inatividade. Seu trabalho está salvo."`, and `"Bloqueado, nada do espaço fica acessível: nem agentes, CLI ou API."` (and their English counterparts) never appear.
2. On `/account/signin` (including when locked), a "Switch account" / "Mudar de conta" button is visible and reveals the account identity field when clicked.
3. On `/account/recover`, `"No seu computador. Do seu jeito."` / `"On your computer. Your way."` does not appear, and a visible "Back to Login" / "Voltar para Login" button navigates back to `/account/signin`.
4. `task lint`, `task test:web`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n`, and the account Playwright spec pass.

## Risks and mitigation

- **Existing E2E tests asserting the old locked banner copy.** Mitigation: search `web/e2e/` for `locked_title`, `locked_intro`, `locked_footer`, and `Stuarty is locked` and update them to assert the actual security behavior (`E_LOCKED` / `/account/signin` redirect) rather than the removed copy.
- **STOP** if the author wants true multi-account switching across multiple separate local encrypted workspaces (which would contradict ADR-0052 item 1 "One local account per installation"); this epic covers the UI switch-account flow on Sign In without breaking ADR-0052.

## Size

S

## Hand-off for review

The commits of `epic/103-login-and-recovery-polish`, Vitest and Playwright outputs, and before/after screenshots of `/account/signin` and `/account/recover` in `pt-BR`.
