# Vukem Video

The team protocol, the layout of the repository and the rules of the project are in `AGENTS.md` (this folder). Read it first, then `.agents/README.md`.

@AGENTS.md

- Local work files: `development/` (git-ignored, in Portuguese; layout fixed by `.agents/rules/development-folder.md`).
- New work: `development/TODO.md`, epics and task cards (format and completion protocol in `.agents/rules/task-cards.md`). Fixes: the issue queue of `development/feedbacks/vukem-video.md`, worked by one dedicated feedback agent in parallel (same rule, section 5).
- Prompt that starts a work session: `development/PROMPT.md`. Its last section is the current state of the work: update it at the end of a session, and read it first after a context compaction.
- Feedback: `development/feedbacks/vukem-video.md` (product issues `VV-NNN`) and `development/feedbacks/vukem.md` (framework frictions `VF-NNN`).
- Real-use QA campaigns and benchmark history: `development/tests/PLAN.md`.
- Names and ids everywhere: `.agents/rules/naming.md`. Requirements, decisions and specifications: `docs/` (index in `docs/README.md`).
- A clone of the vukem framework may be kept outside this repository for reading and fixes.
- `C:/dev/vukem-video/` is the git repository; worktrees for parallel agents live in `C:/dev/vukem-video-worktrees/`.
