# The development folder

`development/` holds every file that exists only to develop the project locally: the decisions of the author, the research, the state of the planning, the feedback logs, the QA campaigns and their tools. The work itself is in the public epics of `docs/plans/`. The folder is not part of the product and is never published.

1. `development/` is git-ignored and docker-ignored. Never commit a file from it, never reference a path inside it from product code, tests, CI, the site or `docs/`; only the agent guides (`AGENTS.md`, `CLAUDE.md`, `.agents/`, `.claude/agents/`) name it, to tell agents where the local files are. Product facts belong in `docs/`; a decision of the author found while working goes to `development/brainstorming.md` as the next `D-NNN`, and public documents cite it as "author decision D-NNN".
2. Everything in `development/` is written in Brazilian Portuguese (the author reads it). Identifiers, file names, commands, code, commit messages and quoted errors stay in English. Everything committed (`.agents/`, `.claude/`, `AGENTS.md`, `CLAUDE.md`, `docs/`, the product) is in English.
3. The layout is fixed. Do not add a file or folder at the top of `development/` or inside a subfolder unless this rule lists it; change this rule first, in the same change.

```text
development/
├── README.md          the map of this folder, in Portuguese (what each file is, who writes it)
├── PROMPT.md          the state of the planning session of 2026-11-18 and the history before it; the work-session prompt is prompt-workers.md at the repository root (git-ignored)
├── QUESTIONS.md       questions for the author asked while they were away, each with the decision taken
├── brainstorming.md   the decisions of the author (D-NNN) that define Stuarty, from D-047; a new decision takes the next number at the end
├── context/           the planning of the turn to Stuarty: estado-atual.md (the code on 2026-11-18), brainstorming-rodadas.md (questions and answers),
│                      brainstorming-vukem-video-legado.md (decisions D-001 to D-046), changelog-docs.md (what the documentation refresh changed), backups
├── feedbacks/
│   ├── vukem-video.md issues found by using and testing the product (VV-NNN), sorted by id, ascending (plans.md section 5)
│   └── vukem.md       frictions with the vukem framework and ideas to improve it (VF-NNN), always recorded when found (ADR-0047)
├── search/            the research notes: web/ (the research of 2026-11-18, index in web/00-indice.md) and the earlier notes (<slug>.md, notes/, projects/)
├── tests/
│   ├── PLAN.md        the repeatable real-use QA campaign and the benchmark history
│   ├── inventory/     the coverage inventory (MCP, CLI, operations, UI) and the scripts that build and fill it
│   ├── assets/        the test media, with LICENSES.md (source, licence, author, technical data of each file)
│   ├── bench/         the benchmark scripts and their raw results
│   └── results/       R<n>/<agent>/ evidence of QA round n, one folder per agent; L<n>/<agent>/ evidence of model-ladder round n
├── scripts/           local tools
└── .cache/            tool caches (never read, never edited by hand)
```

4. Naming inside `development/`: the canonical files at the top of a folder are UPPERCASE (`README.md`, `PROMPT.md`, `QUESTIONS.md`, `PLAN.md`, `LICENSES.md`); everything else is kebab-case, lowercase, ASCII (`vukem-video.md`, `estado-atual.md`). QA rounds are `R1`, `R2`, ... and model-ladder rounds `L1`, `L2`, ...; dates are `AAAA-MM-DD`.
5. Ids are global, never reused and always ascending in their file: `D-NNN` (brainstorming.md), `VV-NNN` (feedbacks/vukem-video.md), `VF-NNN` (feedbacks/vukem.md), `TV-<AREA>-NNN` and `B-NN` (tests/PLAN.md). A new entry takes the largest id of its file plus one.
6. Every file starts with a level-one title, then two to five lines that say what the file is, who writes it and how it is kept. The sections that follow are numbered (`## 1. ...`) when the file has more than two.
7. Who writes what: the orchestrator writes `brainstorming.md`, the state of the session and the status of the feedback ids (and marks epics done in `docs/plans/00-roadmap.md`); any agent may add a `VV` or a `VF` entry or a question, following the format; QA agents write only under `tests/results/R<n>/<their name>/` or `tests/results/L<n>/<their name>/`.
8. Large or binary files (media, builds, workspaces of test runs) never go into `development/` except `tests/assets/` and the backups of `context/`. Local builds and test workspaces live in `qa-build/` at the repository root (git-ignored); test credentials stay there and are never copied into a report.
