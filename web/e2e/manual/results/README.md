# Results of the screen reader passes

Each run of `../screen-reader-checklist.md` with one reader leaves one file here, so a reader can be run again without touching the others.

## Naming

`screen-reader-<YYYY-MM-DD>-<reader>.md`, where `<reader>` is one of `nvda-firefox`, `nvda-chrome`, `narrator-edge`, `voiceover-safari`. For example `screen-reader-2026-12-01-nvda-firefox.md`. Copy `TEMPLATE.md`, fill the header and the table.

## Rules

- The table has one row for each id of the checklist (`SR-01` to `SR-16`), in order, and no other row.
- A finished file has `pass`, `n/a` or `fail #<number>` in every Result cell, never `todo`. The number is the card opened for the failure (see "Filing a failure" in the checklist). A `fail` without a number is incomplete.
- `TEMPLATE.md` keeps `todo` in every cell.
- `n/a` needs a reason in Notes (for example the marker command is not in the build, or the row belongs to the desktop shell).

`go test ./web/e2e/manual/results/...` checks the file names, the table of the template and of every result file, and that the ids are the ones of the checklist.
