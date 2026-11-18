# Plan 024: First export in under ten minutes, the reference scenario on three systems and the site audit

This plan answers whether a person who follows only the documentation reaches a verified exported video in under ten minutes (NFR-040), whether the CLI, MCP and browser drivers each produce a verified MP4 on three operating systems, and whether the documentation site passes every acceptance check of SDD-010. When delivered, the timed runs are recorded and the site build and its checks pass in CI.

- Owner: senior-d (site, CLI and MCP driver, release timing); senior-e (browser driver); orchestrator (timed run with a person)
- Effort: M
- Depends on: real CI runs on three systems, a clean machine that never ran the product, and the author or a person for the timed run; a signed installer is not available in the BETA (no paid certificate)
- Origin: EPIC-03 (P1-DOC-001): TASK-0017; EPIC-04 (P1-QA-001): TASK-0022; EPIC-15 (P2-REL-006): TASK-0072; EPIC-36 (P4-DOC-001): TASK-0169, TASK-0172
- Status: BLOCKED (needs a real CI run, a paid API key, a clean machine or another system, or the author)
- Written against: 8787714a

## 1. Why

NFR-040 (ten minutes from download to the first verified export) is an exit criterion of the prototype, the ALPHA and 1.0. The site workflow has not run in CI since the pull-request workflows were disabled, and the reference scenario job has not run on three systems.

## 2. Current state (drift check)

```text
cd site && npm run build && npm run reference:check
cd site && npm run check:mdx && npm run check:built && npm run test:site && npm run test:scripts
bash scripts/first-export-timing.sh
ls web/e2e/fixtures
```

Already true (from the former card statuses):

- Local site checks pass: `check:mdx` with 0 errors (31 warnings), `check:built`, `test:site`, `test:scripts`; the build produced 273 pages; a base path test was added. `check:translations` no longer exists (the site is English only), although the old assertion text still names it.
- Container timing: about 100 s from `docker pull` to a verified export (21 s without the pull), fresh container and volume (9d3eca6f + 770f84bb, release/v0.22.0); 5 documentation failures fixed; `scripts/first-export-timing.sh` exists. Later run: 101.7 s against the 600 s budget, and quick-start now reaches the export on its own (be158f46).
- The checks of SDD-010 section 14 were run and recorded (senior-d, 2026-11-16); checks 13, 14, 17 and 19 depend on a published site, CI or a person; check 18 is the author's.
- The reference scenario produces three equal documents after identifier normalisation (NFR-025). The reference job has not run on three systems since CI was paused; Windows only locally. The observer delay (NFR-002) and the timed ten minutes were not measured; the first-steps page guides the CLI scenario.
- Not done: the timed run with the signed installer; `task release:preflight` needs Docker and was not run.

## 3. Steps

### Step 1: Site build and its checks in CI

Owner: senior-d. Files: `site/`, `.github/workflows/site.yml`.

Acceptance:

- [ ] The site build and its SDD-010 section 12 checks pass in CI; the SDD-010 section 14 checks marked for the prototype hold.
- [ ] `npm run build`, `check:mdx`, `check:built`, `reference:check`, `test:site` and `test:scripts` pass (and `check:translations` only if it exists again).

Commit: `docs(site): <english summary>` per fixed page.

### Step 2: The three drivers produce a verified MP4 on three systems

Owner: senior-d (CLI and MCP), senior-e (browser). Files: `web/e2e/`, `web/e2e/fixtures/`, `.github/workflows/ci.yml`.

Acceptance:

- [ ] The three drivers each produce an MP4 that passes the verification of SDD-004 section 8 on Linux, macOS and Windows.
- [ ] The observer delay is at most 200 ms p95 over 20 edits (NFR-002), measured on an idle machine and recorded.

Commit: `test(cli): <english summary>` or none if only measurements.

### Step 3: Release preflight and the timed docker path

Owner: senior-d, orchestrator for the timed run. Files: `Taskfile.yml`, `internal/tools/release/`, `scripts/`.

Acceptance:

- [ ] `task release:preflight` exits 0 on `main` after the last ALPHA group.
- [ ] The release checklist of the ALPHA candidate records the timed run from `docker run` to the first export that passes verification, in under 10 minutes, on a machine that never ran the product, following only `self-hosting/docker` and the first-steps page.

### Step 4: Time quick-start on a clean machine

Owner: senior-d. Files: `site/`, `site/content/docs/en/`.

Time `getting-started/quick-start` on a clean machine with the signed installer and again with the container image.

Acceptance:

- [ ] A person following only `getting-started/quick-start` reaches an exported MP4 in under 10 minutes on a clean machine, timed once for the installer and once for the container, and recorded by the author in the maintainers' private release log (NFR-040).
- [ ] The whole reference scenario follows only what the first-steps page says, in under 10 minutes for one person, timed once by the orchestrator.

### Step 5: Close the site audit

Owner: senior-d. Files: `site/content/docs/en/`.

Acceptance:

- [ ] The pages that failed the audit are updated.
- [ ] `task lint` reports no findings.

## 4. Out of scope

- Buying a signing certificate.
- Release verification of the 1.0 tag (plan 031).
- Translating the site back to Portuguese.

## 5. STOP conditions

- No clean machine is available: stop; a developer machine does not count for NFR-040.
- The timed run exceeds 10 minutes: stop and report which documentation step wasted the time; fixing the page is allowed, relaxing the budget is not.
- The signed installer does not exist: stop and ask the orchestrator whether the installer timing may be skipped for this release.

## 6. Maintenance

Re-time the first-steps path at every release candidate and whenever the install steps, the image or the first-steps page change. Keep `scripts/first-export-timing.sh` in step with the docs.
