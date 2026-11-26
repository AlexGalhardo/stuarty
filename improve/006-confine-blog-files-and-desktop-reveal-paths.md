# Plan 006: Confine `blog_files` destination and desktop `showInFolder` paths against host escape

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `improve/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a62856c36..HEAD -- service/publish/destinations/files.go service/publish/destinations/destinations.go desktop/shell/app/bridge.go desktop/shell/reveal.go docs/sdds/020-publishing.md`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `a62856c36`, 2026-11-25

## Why this matters

Two filesystem boundaries allow arbitrary host-path targeting and existence probing:

1. **`blog_files` destination (`service/publish/destinations/files.go`)**: accepts ANY absolute directory on the host without resolving symlinks, without excluding `<workspace>` (or its ancestors/subdirectories), and without restricting network mode. Because `writeTree` replaces `<folder>/<slug>` using `os.RemoveAll(old)`, pointing `folder` at `<workspace>` (or a symlink resolving to it) lets a slug matching `store`, `library`, `fonts`, or `exports` destructively delete workspace data. In addition, `Send` persists raw absolute server paths in `Result.URL`, leaking server filesystem layout to non-admin project members in network mode (violating SDD-007 T-29).
2. **Desktop shell `showInFolder` (`desktop/shell/app/bridge.go`)**: `OpRevealAsset` enforces `shell.Revealer{Root: *mediaRoot}` because the webview page is untrusted, yet `OpShowInFolder` right above it validates `req.Path` only with `shell.CheckRevealPath`, which allows any existing file anywhere on the user's machine and leaks distinct existence/type errors to the webview.

## Current state

- `service/publish/destinations/files.go:45-54, 85-96, 141-169`:

```go
	if !filepath.IsAbs(c.Folder) || strings.ContainsRune(c.Folder, 0) {
		return c, errors.New("the folder is an absolute path")
	}
...
func (f *files) ValidateConfig(secret []byte, _ string) error {
	c, err := parseFilesConfig(secret)
	if err != nil {
		return err
	}

	if st, serr := os.Stat(c.Folder); serr != nil || !st.IsDir() {
		return errors.New("the folder does not exist")
	}

	return nil
}
...
	return Result{RemoteID: p.Slug, URL: target, State: p.Status}, nil
```

- `desktop/shell/app/bridge.go:154-196`:

```go
func (b *bridge) showInFolder(win *application.WebviewWindow, req shell.BridgeRequest) {
	msg := ""
	if err := shell.CheckRevealPath(req.Path); err != nil {
		msg = err.Error()
	} else {
		name, args := shell.RevealCommand(runtime.GOOS, req.Path)
		cmd := exec.CommandContext(context.Background(), name, args...)
		...
	}
	win.ExecJS(shell.ResolveScript(req.ID, nil, msg))
}

func (b *bridge) revealAsset(win *application.WebviewWindow, req shell.BridgeRequest) {
	root := b.mediaRoot.Load()
	...
	r := shell.Revealer{GOOS: runtime.GOOS, Root: *root, ...}
	if err := r.Reveal(req.Path); err != nil {
		msg = err.Error()
	}
	win.ExecJS(shell.ResolveScript(req.ID, nil, msg))
}
```

- Exemplar patterns:
  - `service/asset_reveal.go:40` (local-mode-only gating & never leaking host paths in network mode).
  - `desktop/shell/reveal.go:76-105` (`CheckRevealUnder` symlink-safe root confinement).

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Destinations tests | `go test -race -count=1 ./service/publish/destinations/...` | `ok`, exit 0 |
| Desktop shell tests | `go test -race -count=1 ./desktop/shell/...` | `ok`, exit 0 |
| Desktop app module tests | `cd desktop/shell/app && go test -tags purego -count=1 ./...` | `ok`, exit 0 |
| Security suite | `go test -count=1 -run TestSecurity_ ./...` | `ok`, exit 0 |
| Go lint & vet | `task lint:go && task vet` | exit 0 |

## Scope

**In scope** (the only files you should modify):

- `service/publish/destinations/destinations.go`
- `service/publish/destinations/files.go`
- `service/publish/destinations/files_test.go`
- `service/publish_ops.go` (only if `destinations.Config` needs `Workspace` and `NetworkMode` passed when constructing `destinations`)
- `desktop/shell/reveal.go`
- `desktop/shell/reveal_test.go`
- `desktop/shell/app/bridge.go`
- `desktop/shell/app/main.go` (to pass `<workspace>/exports` root to `bridge`, alongside `mediaRoot`)
- `docs/sdds/020-publishing.md` (document `blog_files` workspace exclusion and network-mode rule)

**Out of scope** (do NOT touch):

- WordPress, Ghost, Webhook, or social connectors under `service/publish/destinations/` and `service/social/`.
- Frontend export dialog (`web/resources/js/editor/export/`).

## Git workflow

- Branch: `fix/security-path-confinement` cut from `release/v0.60.0`.
- Commit with Conventional Commits + `Security:` footer, e.g.:
  `fix(security): confine blog_files destinations and desktop showInFolder to safe roots`
  `Security: prevents workspace overwrite/deletion via blog_files and arbitrary host file reveal via desktop bridge`
- Do NOT push or merge to `main`.

## Steps

### Step 1: Harden `blog_files` destination against workspace targets, symlink escape, destructive overwrite, and network path leaks

1. Read `service/publish/destinations/destinations.go`, `service/publish/destinations/files.go:175-342` (`writeTree`, `within`), and where `destinations.Config` is populated in `service/` (grep `destinations.New` or `destinations.Config`).
2. Add `Workspace string` and `NetworkMode bool` to `destinations.Config` (and pass them from the caller in `service/`).
3. In `service/publish/destinations/files.go`:
   - In `ValidateConfig`, `problem`, and `Send`:
     - If `f.cfg.NetworkMode` is true, refuse `blog_files` (matching `service/asset_reveal.go:40` — no raw server filesystem publishing in network mode unless explicitly configured in a future epic).
     - Canonicalize `cfg.Folder` via `filepath.EvalSymlinks` (and `f.cfg.Workspace` when set). Refuse if canonical `cfg.Folder` equals `ws`, lies inside `ws`, or is an ancestor of `ws` (so `<folder>/<slug>` can never collide with `<workspace>` or any directory inside it).
     - In `Send`, re-resolve `cfg.Folder` with `filepath.EvalSymlinks` and write against the canonical directory so a symlink swap cannot escape between validation and write.
   - In `writeTree`: before moving an existing `target` directory to `.stuarty-old-...` and deleting it with `os.RemoveAll`, verify that `target` looks like a prior Stuarty post directory (contains `index.md` as a regular file, not an arbitrary directory such as `store` or `library`), refusing otherwise.
   - Do not expose raw absolute server paths in `Result.URL` when `NetworkMode` is true (keep local-mode `URL: target` or relative slug; verify existing tests in `files_test.go` and `servicetest/`).
4. Update `docs/sdds/020-publishing.md` to state the workspace exclusion and local-mode confinement of `blog_files`.

**Verify**: `go test -race -count=1 ./service/publish/destinations/... && go test -race -count=1 -run 'Blog|Destination' ./web/app/servicetest/` -> `PASS`, exit 0.

### Step 2: Confine desktop `OpShowInFolder` (`showInFolder`) to `<workspace>/exports`

1. Read `desktop/shell/reveal.go` in full and check where `b.mediaRoot` is set in `desktop/shell/app/` (grep `mediaRoot` under `desktop/`).
2. Add a sibling `CheckExportRevealUnder(exportsRoot, target string) error` in `desktop/shell/reveal.go` (reusing `CheckRevealUnder`'s symlink resolution + root containment, allowing exported media filenames rather than requiring SHA-256 asset names, and rejecting symlinks pointing outside `exportsRoot`, returning a single generic refusal message for out-of-root paths so file existence outside `exportsRoot` is never leaked).
3. Track `exportsRoot atomic.Pointer[string]` on `bridge` in `desktop/shell/app/bridge.go` (populated from `<workspace>/exports` wherever `mediaRoot` is set in `desktop/shell/app/main.go`).
4. Update `bridge.showInFolder` (`desktop/shell/app/bridge.go:154-169`) to require `exportsRoot` and validate `req.Path` with `shell.CheckExportRevealUnder(*root, req.Path)` before invoking `RevealCommand`.

**Verify**: `go test -race -count=1 ./desktop/shell/... && (cd desktop/shell/app && go test -tags purego -count=1 ./...)` -> `PASS`, exit 0.

### Step 3: Add security regression tests

1. In `service/publish/destinations/files_test.go`:
   - Test that configuring `blog_files` to `<workspace>`, `<workspace>/exports`, an ancestor of `<workspace>`, or a symlink pointing to `<workspace>` is rejected by both `ValidateConfig` and `Send`.
   - Test that `blog_files` is rejected when `NetworkMode: true`.
   - Test that `writeTree` refuses to delete an existing `<folder>/<slug>` directory that does not contain `index.md`.
2. In `desktop/shell/reveal_test.go`:
   - Test `CheckExportRevealUnder`: allows a real file inside `<exportsRoot>`, rejects a file outside `<exportsRoot>` (with identical generic error whether that external file exists or not), rejects `..` traversal, and rejects a symlink inside `<exportsRoot>` pointing outside `<exportsRoot>`.

**Verify**: `go test -count=1 -run TestSecurity_ ./... && task lint:go && task lint:md && task vet` -> all exit 0.

## Test plan

- `service/publish/destinations/files_test.go`: workspace/ancestor/symlink rejection, network-mode rejection, non-post directory overwrite protection.
- `desktop/shell/reveal_test.go`: `CheckExportRevealUnder` positive, out-of-root non-enumeration, traversal, and symlink-escape tests.
- Existing `web/app/servicetest/` blog/export tests and full `TestSecurity_` suite.

## Done criteria

- [ ] `go test -race -count=1 ./service/publish/destinations/...` exits 0
- [ ] `go test -race -count=1 ./desktop/shell/...` exits 0
- [ ] `cd desktop/shell/app && go test -tags purego -count=1 ./...` exits 0
- [ ] `go test -count=1 -run TestSecurity_ ./...` exits 0
- [ ] `task lint:go && task lint:md && task vet` exits 0
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `improve/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- Any existing test or UI flow exports a video/image outside `<workspace>/exports` and passes that external path to `showInFolder` (grep `showInFolder` across `web/resources/js/` and `service/` first — if custom user export directories exist, report what paths `showInFolder` actually receives).
- Any existing network-mode test expects `blog_files` to work in network mode (check `grep -rn 'blog_files\|KindFiles' web/app/servicetest/ service/` before gating network mode; if network-mode tests depend on `blog_files`, switch to an explicit operator-configured `STUARTY_PUBLISH_ROOTS` confinement instead of disabling it).

## Maintenance notes

- Every desktop bridge IPC operation (`desktop/shell/app/bridge.go`) must treat `req` from the webview as untrusted and enforce explicit root confinement via `desktop/shell/reveal.go`.
- Reviewer focus: verify `filepath.EvalSymlinks` handles macOS `/var` -> `/private/var` symlinks cleanly in tests (canonicalize BOTH `root` and `target` before comparing!).
