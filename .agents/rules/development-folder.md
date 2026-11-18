# The development folder

`development/` holds every file that exists only to develop the project locally: the state of the work, the decisions of the author, the questions waiting for the author, the feedback queues, the research and the QA material. The work itself is in the public epics of `docs/plans/`. The folder is not part of the product and is never published.

## 1. What it is and what it is not

1. `development/` is git-ignored and docker-ignored. Never commit a file from it, never reference a path inside it from product code, tests, CI, the site or `docs/`; only the agent guides (`AGENTS.md`, `CLAUDE.md`, `.agents/`, `.claude/agents/`) name it, to tell agents where the local files are.
2. Product facts belong in `docs/`. A decision of the author goes to `development/brainstorming.md` as the next `D-NNN`, and public documents cite it as "author decision D-NNN". A research note is cited as "research note `<name>`", never by path.
3. Everything in `development/` is written in Brazilian Portuguese (the author reads it). Identifiers, file names, commands, code, commit messages and quoted errors stay in English. Everything committed is in English. The earlier research in `search/editor/` stays in English because it was once versioned.
4. **It is kept in step with the project, not as an archive.** What a change makes stale is updated or deleted in the same change: a state that is no longer true, a question that was answered and became code, a script for a process that was retired, the files of a finished round that nobody will read again. The history of the project is in git; the only things kept for safety are the backups of `context/backups/`.
5. Large or binary files (builds, workspaces of test runs, logs) never go into `development/`, except the test media of `tests/assets/` and the backups of `context/backups/`. Local builds and test workspaces live in `qa-build/` at the repository root (git-ignored); test credentials stay there, are never read into a report and are never copied elsewhere.

## 2. Layout

The layout is fixed. Do not add a file or a folder unless this rule lists it; change this rule first, in the same change.

```text
development/
├── README.md          the map of this folder, in Portuguese: what each file is, who writes it, when it is read
├── STATE.md           the current state of the work: branch, pull request, what each agent is doing, epics done and blocked,
│                      what waits for the author, the commands that validate. Read first; written after every event
├── QUESTIONS.md       what the agents need from the author during the work: a confirmation or a manual action, with the options,
│                      the recommended one and what was done meanwhile. Empty when nothing waits; an answered line leaves the file
├── brainstorming.md   the decisions of the author (D-NNN) that define Stuarty, from D-047; a new decision takes the next number
├── context/           the turn to Stuarty: estado-atual.md (the code on 2026-11-18), brainstorming-rodadas.md (questions and answers),
│   │                  brainstorming-vukem-video-legado.md (decisions D-001 to D-046), changelog-docs.md (what the planning changed in the documents)
│   └── backups/       safety copies only: the bundle of the refs deleted on 2026-11-18 and patches of uncommitted work
├── feedbacks/
│   ├── vukem-video.md             the open product issues (VV-NNN), ascending by id, each with where it is to be fixed (plans.md section 5)
│   ├── vukem-video-resolvidas.md  the fixed issues, whole, for the QA campaign to check again
│   └── vukem.md                   frictions with the vukem framework and ideas to improve it (VF-NNN), recorded when found (ADR-0047)
├── search/
│   ├── README.md      the map of the research
│   ├── web/           the research of 2026-11-18 for Stuarty (index in 00-indice.md), cited as "research note `web/<name>`"
│   └── editor/        the research of 2026-11-14 to 17 for the video editor, closed, cited as "research note `<slug>`"
└── tests/
    ├── PLAN.md        the repeatable QA script of the video editor and the benchmark history
    ├── inventory/     the coverage inventory (MCP, CLI, operations, UI) and the scripts that build and fill it
    ├── assets/        the test media, with LICENSES.md (source, licence, author, technical data of each file)
    ├── bench/         the benchmark scripts and their raw results
    └── results/       R<n>/<agent>/ evidence of QA round n, one folder per agent; L<n>/<agent>/ evidence of model-ladder round n
```

## 3. How to use it

1. **Starting a session, and after every context compaction:** read `STATE.md` first, then `QUESTIONS.md` for what the author answered. Never start an epic again from memory.
2. **The state.** After every event (an epic dispatched, handed off, reviewed, merged or blocked; an agent started or stopped) the orchestrator rewrites `STATE.md`: it describes now, not the story. The status of an epic is the Status column of `docs/plans/00-roadmap.md`; `STATE.md` only summarizes it and adds what the roadmap does not hold (who is working where, what waits for the author).
3. **A STOP condition.** `QUESTIONS.md` is where an agent asks the author for a confirmation (a decision only the author can take) or a manual action (something only the author can do on the machine or in an account: a download an agent may not make, a real sign-in, creating a developer app, approving a spend). The agent that stops adds the entry in the format of that file: the question with enough context to answer it without opening another file, the options with the recommended one first, the exact command or steps of a manual action, what it did in order not to block the rest, and what the entry blocks. It is not for a choice that is cheap to undo, nor for anything the epic, a specification, an ADR or a recorded decision already answers. The orchestrator marks the epic blocked in the roadmap and continues with work that does not depend on the answer.
4. **An answer of the author.** The orchestrator records a decision as the next `D-NNN` in `brainstorming.md`, fixes the epic and the public documents it changes, and removes the entry from `QUESTIONS.md`: the file holds only what still waits, never a history.
5. **A product problem.** Whoever finds a defect outside the scope of what it is doing adds a `VV-NNN` block at the end of `feedbacks/vukem-video.md`, in the format of that file, with the steps, what was expected, what happened, the evidence and where it should be fixed (an epic, or the queue). Only the orchestrator writes a status of fixed, after the specialist approved, and then moves the whole block to `vukem-video-resolvidas.md`.
6. **A friction with vukem.** Whoever meets it adds a `VF-NNN` to `feedbacks/vukem.md` at once, checked against the vukem source, before working around anything (ADR-0047).
7. **A briefing.** An epic never cites a path of this folder. The orchestrator copies into the briefing what the agent needs from here: the `VV` entries the epic closes and the decisions `D-NNN` that apply.
8. **The specialist** receives nothing from this folder beyond what the epic cites. A defect it finds and does not fix goes to the orchestrator, who records the `VV`.
9. **Evidence.** A QA agent writes only under `tests/results/R<n>/<its name>/`; an intern writes reports and evidence only where its briefing says. Nobody edits the results of a finished round.

## 4. Who writes what

| File | Orchestrator | Senior | Intern | Specialist |
| --- | --- | --- | --- | --- |
| `STATE.md`, `brainstorming.md`, `README.md`, `context/` | writes | reads | reads | no |
| `QUESTIONS.md` | writes and keeps | adds a question at a STOP | no | no |
| `feedbacks/vukem-video.md` | adds, sets the status, moves fixed blocks | adds a `VV` | adds a `VV` when asked to test | reports to the orchestrator |
| `feedbacks/vukem.md` | adds | adds a `VF` | adds a `VF` | reports to the orchestrator |
| `search/web/` | writes | reads | writes when asked to research | no |
| `tests/results/` | reads | writes in its own folder during the QA campaign | writes where the briefing says | no |
| `tests/PLAN.md`, `tests/inventory/`, `tests/bench/`, `tests/assets/` | writes | reads | writes when asked | no |

## 5. Naming and ids

1. The canonical files at the top of a folder are UPPERCASE (`README.md`, `STATE.md`, `QUESTIONS.md`, `PLAN.md`, `LICENSES.md`); everything else is kebab-case, lowercase, ASCII (`vukem-video.md`, `estado-atual.md`). QA rounds are `R1`, `R2`, ... and model-ladder rounds `L1`, `L2`, ...; dates are `AAAA-MM-DD`.
2. Ids are global, never reused and always ascending in their file: `D-NNN` (`brainstorming.md`), `VV-NNN` (the two files of the product queue share one sequence), `VF-NNN` (`feedbacks/vukem.md`), `TV-<AREA>-NNN` and `B-NN` (`tests/PLAN.md`). A new entry takes the largest id ever used plus one; each file states that number at its top.
3. Every file starts with a level-one title, then two to five lines that say what the file is, who writes it and how it is kept. The sections that follow are numbered (`## 1. ...`) when the file has more than two.
4. The feedback files keep the former product name in their file names; they are renamed only together with the documents that cite them.
