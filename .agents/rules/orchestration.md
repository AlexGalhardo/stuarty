# Orchestration

1. A delegated engineer's report is a claim, not evidence. Before taking a delivery: read `git diff --stat`, run its asserts yourself, and run the checks of `skills/ci-preflight/`.
2. Each senior owns a folder (`AGENTS.md` section 4). A change outside it is reverted or sent back, unless it is the minimal edit that keeps the build compiling, and the report must say so.
3. A test that passes only in the author's worktree is not done. Verify it from a fresh `git worktree add --detach` of the branch tip. Untracked or ignored files (a built front end, a cached workspace, an environment variable) make tests pass by accident.
4. Act on every security-review finding that comes with a commit before the commit goes into a release. If you decide one does not apply, write down why.
5. When the author is away, never block on a question. Write it into `questions-to-dev.md` (excluded from git through `.git/info/exclude`) with the decision you took, take the simplest reversible option, and continue.
6. Mark the ROADMAP as you go: the task line, every subtask and assert, and an `Audit <date>:` line with the evidence; or a `Status <date>:` line with what is done and what is open.
7. Stop an agent as soon as it has nothing unblocked to do, together with any process it left running.
