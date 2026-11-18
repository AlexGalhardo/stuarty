# Commits

1. Conventional Commits: `type(scope): subject`. The header is at most 100 characters, the subject starts with a lower-case letter and has no final period.
2. The scope must be one of `scopes=` in `.github/workflows/pr-title.yml`. The same list is in SDD-009 §6.2 and in `.github/labels.txt` (`area:<scope>`); `internal/tools/labelsync` tests that the three agree, so a new scope goes into all three.
3. Never edit `CHANGELOG.md` by hand. The release tool writes it from the commits when releases are cut again (ADR-0045).
4. One small, self-contained commit per task of an epic, with `Refs: epic NN task N` in the body (`Refs: VV-NNN` for a fix of the feedback queue). A commit that leaves the tree red in the middle of a series must say so in the hand-off.
5. The commit type decides the section of the release notes later (`feat` Added, breaking Changed, `revert` Removed, `fix` Fixed, `Security:` footer Security), so choose it with care even while no release is cut.
6. Everything in the repository is in English.
