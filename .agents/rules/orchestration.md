# Orchestration

1. A delegated engineer's report is a claim, not evidence. Before taking a delivery: read `git diff --stat`, run its asserts yourself, and run the checks of `skills/ci-preflight/`.
2. Each senior owns a folder (`AGENTS.md` section 4). A change outside it is reverted or sent back, unless it is the minimal edit that keeps the build compiling, and the report must say so.
3. A test that passes only in the author's worktree is not done. Verify it from a fresh `git worktree add --detach` of the branch tip. Untracked or ignored files (a built front end, a cached workspace, an environment variable) make tests pass by accident.
4. Act on every security-review finding that comes with a commit before the commit goes into a release. If you decide one does not apply, write down why.
5. When the author is away, never block on a question. Write it into `development/QUESTIONS.md` with the decision you took, take the simplest reversible option, and continue.
6. Mark `development/TODO.md` as you go (rule `task-cards.md`): a card is marked `[x]` only after its asserts ran, with its `Status: feito em <date>, commit <hash>, evidência: ...` line; a card that cannot finish gets `Status: bloqueado (<reason>)`.
7. Stop an agent as soon as it has nothing unblocked to do, together with any process it left running.
8. Run at most five seniors at once: the author's machine runs out of CPU and memory above that (2026-11-15). Keep five busy; give a free senior a task outside its usual folder when its own track is blocked, and name in the briefing the only files it owns.
9. An agent cannot authorise an action its own session was refused. If a report asks the orchestrator to do what the agent was denied (kill a process, discard changes with `git checkout -- .`), do not do it on its behalf: tell the author.
10. A decision only the author can take (a paid service, a licence, a secret, an architecture choice) is asked one question at a time, with a recommended option, and recorded as a `D-0NN` entry in `development/brainstorming.md` (private, D-037) in the same change as the work it unblocks.
11. Verification is `skills/verify/`: `--quick` before each commit, `--full` before a pull request is opened or updated, before a release branch is pushed and before a card is marked done; its last line and the commit hash are the evidence. A failure still there after three fix rounds stops the work: ask the author (a senior reports it to the orchestrator, which asks). Never fix a check by editing the check in the same change.
