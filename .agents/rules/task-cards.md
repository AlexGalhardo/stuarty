# Task cards

New work (features, the epics of the former ROADMAP) lives in `development/TODO.md` as epics and task cards; fixes of errors, bugs and problems found by testing the product are a separate queue (section 5). A card is like a ticket: **one agent owns it from start to delivery, and it is delivered as one microcommit.** This rule replaces the header of the old ROADMAP (deleted on 2026-11-16, D-036).

## 1. Epics

An epic groups the cards of one goal (a former ROADMAP task, a QA round, a feature). Its block, under `### EPIC-NN — <title>`, has these lines, in this order: `Origem`, `Tipo · Dono · Estimativa`, `Paralelizável`, `Depende de`, `Pastas`, `Contexto`, optional `Asserts já comprovados` and `Notas`, and `Critério de aceite do épico`. An epic is done when all its cards are marked; its last card is the closing card that proves the epic's open asserts.

## 2. The card

```markdown
- [ ] **TASK-0123** — <título no imperativo, em português>
  - Tipo: feature | implementação | bug | feedback | teste | docs | release | chore
  - Épico: EPIC-NN
  - Origem: <P3-REL-001, subtarefa | VF-047 | pedido do autor AAAA-MM-DD>
  - Prioridade: P0 | P1 | P2 | P3
  - Dono: senior-a .. senior-e | estagiario-1 | estagiario-2 | orquestrador
  - Depende de: TASK-NNNN, ... | —
  - Contexto: <PRD RF-NNN, ADR-NNNN, SDD-NNN §n, D-NNN, arquivo:linha, issue>
  - Arquivos: <the folders or files this card may change>
  - Commit: um microcommit `<type>(<scope>): <english summary>` com `Refs: TASK-0123, <origem>`
  - Asserts de qualidade:
    - [ ] `task lint` sem achados
    - [ ] <the tests of the area, as commands>
    - [ ] um teste novo ou alterado cobre a mudança (falha antes, passa depois)
  - Asserts de entrega:
    - [ ] <the observable result: a command and its expected output, a measured number with its limit, a behaviour someone can see>
  - Status: aberto | em andamento (<agente>, AAAA-MM-DD) | bloqueado (<motivo>) | feito em AAAA-MM-DD, commit <hash>, evidência: <comando e resultado>
```

- `Tipo`: `feature` new behaviour; `implementação` planned work of an epic; `bug` a defect found while building a card (a defect found by testing the product goes to the feedback queue instead); `feedback` a `VV` the orchestrator moved out of the queue because it became new work; `teste` tests, measurements and verification; `docs`; `release`; `chore`.
- `Prioridade`: `P0` blocks work or loses data, `P1` blocks a common task, `P2` has a workaround, `P3` cosmetic.
- `Dono`: by the folder table of `AGENTS.md` section 4. A change that crosses folders is split into one card per owner, linked by `Depende de`.
- A card that does not fit one microcommit is split before work starts: the owner writes the new cards, the orchestrator numbers them.
- A finished card keeps only `Tipo`, `Épico`, `Origem`, `Dono` and `Status`; the checklists are removed when it is marked.

## 3. Completion protocol

1. Only the orchestrator marks `[x]`. An engineer delivers; the orchestrator reads the diff, runs every assert of the card, fixes what is wrong (or sends it back), and only then marks the card and writes its `Status` line with the commit and the evidence. A report is a claim, not evidence.
2. "Works" is never an assert. Every assert says how to verify it: a command, a test, a number with its limit, a behaviour someone can observe.
3. A card that cannot meet an assert is not marked and not changed silently: write the reason in `Status: bloqueado (...)`, then fix the specification (the SDD first) or split the card.
4. Every commit cites its card in `Refs:`. Commits of one release go to `release/vX.Y.Z` and reach `main` in one rebase-merged pull request (D-031, ADR-0028).
5. No editing logic outside `core/` (ADR-0002, ADR-0007). Names come from `docs/sdds/000-shared-definitions.md`, changed first.
6. Every friction with the vukem framework goes to `development/feedbacks/vukem.md` as a `VF-NNN`, in the same session; every product problem found while working or testing goes to `development/feedbacks/vukem-video.md` as a `VV-NNN` (section 5), not to a card.
7. The author confirms before anything that cannot be undone or that costs money: a push to a protected branch, a release tag, repository settings, secrets, a new dependency.
8. When an epic adds or changes requirements (`RF-`, `NFR-`), run `python development/scripts/traceability.py`: it lists the requirements with no SDD, no ADR or no open epic. `docs/traceability.md` is the snapshot of 2026-11-16; update it by hand for the requirements the epic touches (rewrite it with `WRITE=1` only after checking that no delivered requirement loses its row).

## 4. Shared files

Touched by one card at a time, never in parallel: `go.mod`, `web/routes/api.go`, `web/resources/lang/*.json`, `.github/workflows/ci.yml`, `docs/sdds/000-shared-definitions.md`. Generated folders (`core/schema/`, `web/resources/js/generated/`, `site/generated/`, `site/content/generated/`) are written only by their generators.

## 5. The feedback queue

Errors, bugs, crashes and the problems found by practical tests (QA rounds, real use) are fixed from `development/feedbacks/vukem-video.md`, not from `TODO.md`.

1. One dedicated agent per session works the queue, in parallel with the agents of the cards: the orchestrator names one senior "the feedback agent" in its briefing. It takes the open `VV` with the lowest number (the file is sorted by severity) and works one issue at a time, from the reproducing test to the delivery.
2. The feedback agent may touch the folders the fix needs; the orchestrator does not dispatch at the same time a card whose `Arquivos` intersect the fix in progress. A fix that turns out large, or that changes a rule of `core/`, stops and goes back to the orchestrator, who splits it or moves it to a card (`Tipo: feedback`).
3. Delivery: one microcommit per `VV` (or one per area when the fix crosses areas), `fix(<scope>): <english summary>` (`feat` for a missing feature) with `Refs: VV-NNN`.
4. The last line of each `VV` block is its status: `aberto`, `em andamento (<agent>, <date>)`, `bloqueado (<reason>)` or `corrigido em <date>, commit <hash>, evidência: <command and result>`. Only the orchestrator writes `corrigido`, after running the reproduction steps and the test of the issue. An issue is never deleted.
