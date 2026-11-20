# Stuarty

The team protocol, the layout of the repository and the rules of the project are in `AGENTS.md` (this folder). Read it first, then `.agents/README.md`.

@AGENTS.md

- The product is **Stuarty**: create, edit, manage and publish your content in one place. It was called Vukem Video (ADR-0044).
- The work: the epics of `docs/plans/` (index and order in `docs/plans/00-roadmap.md`; one epic is one unit of work for one senior; format and completion protocol in `.agents/rules/plans.md`). No phases and no milestones (ADR-0045).
- The team: seven agents in `.claude/agents/` (`senior-a` to `senior-e`, `intern`, `specialist`); the main session is the orchestrator. The specialist reviews with no prior context: only the epic, its acceptance criteria and the diff.
- One UI: the desktop app (Wails v3) is a window over the same web UI the binary serves; write a page once in `web/resources/` (ADR-0041).
- Local work files: `development/` (git-ignored, in Portuguese; layout and use fixed by `.agents/rules/development-folder.md`). The current state of the work: `development/STATE.md`, read first and rewritten after every event. Questions for the author: `development/QUESTIONS.md`. Decisions of the author: `development/brainstorming.md` (`D-NNN`). Research: `development/search/`. State of the code at the turn to Stuarty: `development/context/estado-atual.md`.
- Feedback: `development/feedbacks/vukem-video.md` (product issues `VV-NNN`) and `development/feedbacks/vukem.md` (framework frictions `VF-NNN`, always recorded when found: ADR-0047).
- Names and ids everywhere: `.agents/rules/naming.md`. Requirements, decisions and specifications: `docs/` (index in `docs/README.md`). Versioned documents are in English.
- Git: the release train (ADR-0048). The open release branch `release/vX.Y.Z` is the integration line, linear, with its pull request against `main`; one branch per epic in its own worktree outside the repository folder. The orchestrator integrates each approved epic as linear commits (`bash .agents/skills/release-pr/scripts/release-train.sh integrate <branch>`), and at about 90 commits cuts the release, rebase-merges it into `main` and opens the next one, on its own. No direct push to `main`, no force push, no tag pushed while CI is off.
- The loop: a senior does one task of its epic, runs the command that verifies it, fixes until it passes, then takes the next task; a finished epic goes to the orchestrator, then to the specialist, then to the open release pull request. When every epic is done, the five seniors test the whole product in rounds until a round finds no problem (ADR-0045).
