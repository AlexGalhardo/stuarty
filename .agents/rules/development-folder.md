# The development folder

`development/` holds every file that exists only to develop the project locally (the work itself is in the public plans of `docs/plans/` since D-046): the session memory, the session prompt, the feedback logs, the QA campaigns and their tools. It is not part of the product and is never published (D-036).

1. `development/` is git-ignored and docker-ignored. Never commit a file from it, never reference a path inside it from product code, tests, CI, the site or `docs/`; only the agent guides (`AGENTS.md`, `CLAUDE.md`, `.agents/`) name it, to tell agents where the local files are, and decision D-036 records that it exists. Product facts belong in `docs/`; a decision found while working goes to `docs/brainstorming.md` as a `D-0NN`.
2. Everything in `development/` is written in Brazilian Portuguese (the author reads it). Identifiers, file names, commands, code, commit messages and quoted errors stay in English. Everything committed (this folder `.agents/`, `AGENTS.md`, `CLAUDE.md`, `docs/`, the product) is in English (D-008).
3. The layout is fixed. Do not add a file or folder at the top of `development/` or inside a subfolder unless this rule lists it; change this rule first, in the same change.

```text
development/
├── README.md          the map of this folder, in Portuguese (what each file is, who writes it)
├── PROMPT.md          the prompt the author pastes to start a work session; its last section is the current state; read it first after a context compaction
├── QUESTIONS.md       questions for the author asked while they were away, each with the decision taken
├── feedbacks/
│   ├── vukem-video.md issues found by using and testing the product (VV-NNN), sorted by id, ascending; the fix queue of the feedback agent (plans.md section 5)
│   └── vukem.md       frictions with the vukem framework (VF-NNN)
├── tests/
│   ├── PLAN.md        the repeatable real-use QA campaign and the benchmark history
│   ├── inventory/     the coverage inventory (MCP, CLI, operations, UI) and the scripts that build and fill it
│   ├── assets/        the test media, with LICENSES.md (source, licence, author, technical data of each file)
│   ├── bench/         the benchmark scripts and their raw results
│   └── results/       R<n>/<agent>/ evidence of QA round n, one folder per agent (logs, screenshots, scripts, coverage);
│                      L<n>/<agent>/ evidence of model-ladder round n (haiku, sonnet, opus), each with its feedback.md
├── brainstorming.md   the decisions of the author (D-NNN), in English as written; moved out of docs/ by D-037
├── search/            the research notes (<slug>.md, notes/), in English as written; moved out of docs/ by D-037
├── scripts/           local tools: gate.sh (release gate), traceability.py, build-todo.py
└── .cache/            tool caches (never read, never edited by hand)
```

4. Naming inside `development/`: the canonical files at the top of a folder are UPPERCASE (`README.md`, `PROMPT.md`, `QUESTIONS.md`, `PLAN.md`, `LICENSES.md`); everything else is kebab-case, lowercase, ASCII (`vukem-video.md`, `gen-matrix.py`). QA rounds are `R1`, `R2`, ... and model-ladder rounds `L1`, `L2`, ...; dates are `AAAA-MM-DD`.
5. Ids are global, never reused and always ascending in their file: `VV-NNN` (feedbacks/vukem-video.md), `VF-NNN` (feedbacks/vukem.md), `TV-<AREA>-NNN` and `B-NN` (tests/PLAN.md). A new entry takes the largest id of its file plus one. After any insertion that breaks the order, renumber and rewrite every reference in the same change.
6. Every file starts with a level-one title, then two to five lines that say what the file is, who writes it and how it is kept. The sections that follow are numbered (`## 1. ...`) when the file has more than two.
7. Who writes what: the orchestrator writes `PROMPT.md` and the feedback ids (and marks plans done in `docs/plans/`); any agent may add a `VV`/`VF` entry or a question, following the format; QA agents write only under `tests/results/R<n>/<their name>/` or `tests/results/L<n>/<their name>/`.
8. Large or binary files (media, builds, workspaces of test runs) never go into `development/` except `tests/assets/`. Local builds and test workspaces live outside the repository, in `C:/dev/vukem-video/qa-build/`; test credentials stay there and are never copied into a report.
