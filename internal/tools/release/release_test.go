package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const base = `# Changelog

Intro.

## [Unreleased]

## [0.1.0] - 2026-11-05

### Added

- Studio.

[Unreleased]: https://github.com/AlexGalhardo/vukem-studio/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/AlexGalhardo/vukem-studio/releases/tag/v0.1.0
`

func TestInsertReleaseKeepsOlderLinks(t *testing.T) {
	t.Parallel()

	section := "## [0.2.0] - 2026-11-20\n\n### Added\n\n- **core:** Add timeval\n"

	got, err := insertRelease(base, section, "v0.2.0", "v0.1.0")
	if err != nil {
		t.Fatal(err)
	}

	want := `# Changelog

Intro.

## [Unreleased]

## [0.2.0] - 2026-11-20

### Added

- **core:** Add timeval

## [0.1.0] - 2026-11-05

### Added

- Studio.

[Unreleased]: https://github.com/AlexGalhardo/stuarty/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/AlexGalhardo/stuarty/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/AlexGalhardo/vukem-studio/releases/tag/v0.1.0
`
	if got != want {
		t.Errorf("insertRelease() =\n%s\nwant\n%s", got, want)
	}
}

func TestInsertReleaseRefusals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, changelog, section, version, wantErr string
	}{
		{"existing version", base, "## [0.1.0] - 2026-11-20\n\n### Added\n\n- x\n", "v0.1.0", "already has a section"},
		{"no unreleased heading", "# Changelog\n", "## [0.2.0] - d\n", "v0.2.0", "Unreleased"},
		{"wrong section heading", base, "## [0.3.0] - d\n", "v0.2.0", "does not start"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := insertRelease(tt.changelog, tt.section, tt.version, "v0.1.0")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("insertRelease() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestReleaseNotes(t *testing.T) {
	t.Parallel()

	if got, want := releaseNotes(base, "v0.1.0"), "### Added\n\n- Studio.\n"; got != want {
		t.Errorf("releaseNotes() = %q, want %q", got, want)
	}

	if got := releaseNotes(base, "v9.9.9"); got != "" {
		t.Errorf("releaseNotes() of an absent version = %q, want empty", got)
	}

	empty := "## [Unreleased]\n\n## [0.2.0] - d\n\n## [0.1.0] - d\n\n- x\n"
	if got := releaseNotes(empty, "v0.2.0"); got != "" {
		t.Errorf("releaseNotes() of an empty section = %q, want empty", got)
	}
}

func TestVersionRules(t *testing.T) {
	t.Parallel()

	for _, v := range []string{"v0.2.0", "v1.0.0", "v0.3.0-rc.1"} {
		if !validVersion(v) {
			t.Errorf("validVersion(%q) = false", v)
		}
	}

	for _, v := range []string{"0.2.0", "v1", "v1.2", "v01.2.3", "main", ""} {
		if validVersion(v) {
			t.Errorf("validVersion(%q) = true", v)
		}
	}

	for _, c := range []struct {
		v, prev string
		want    bool
	}{
		{"v0.2.0", "v0.1.0", true},
		{"v0.10.0", "v0.9.0", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.2.0", false},
		{"v1.0.0", "v0.99.9", true},
		{"v0.2.0", "", true},
	} {
		if got := newerThan(c.v, c.prev); got != c.want {
			t.Errorf("newerThan(%q, %q) = %v, want %v", c.v, c.prev, got, c.want)
		}
	}
}

// fake answers commands from a table keyed by "name arg arg" prefixes and
// records every command it was given.
type fake struct {
	answers map[string]string
	errs    map[string]error
	calls   []string
}

func (f *fake) run(_, name string, args ...string) (string, error) {
	line := strings.TrimSpace(name + " " + strings.Join(args, " "))
	f.calls = append(f.calls, line)

	for prefix, err := range f.errs {
		if strings.HasPrefix(line, prefix) {
			return "", err
		}
	}

	for prefix, out := range f.answers {
		if strings.HasPrefix(line, prefix) {
			return out, nil
		}
	}

	return "", nil
}

func (f *fake) ran(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}

	return false
}

const cliffSection = "## [0.2.0] - 2026-11-20\n\n### Added\n\n- **core:** Add timeval\n"

func newTool(t *testing.T, f *fake) *tool {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte(base), 0o600); err != nil {
		t.Fatal(err)
	}

	return &tool{dir: dir, run: f.run, out: &bytes.Buffer{}}
}

func happy() *fake {
	return &fake{answers: map[string]string{
		"git rev-parse --abbrev-ref HEAD": "release/v0.2.0\n",
		"git rev-parse HEAD":              "aaaaaaaaaaaaaaaa\n",
		"git rev-parse origin/main":       "aaaaaaaaaaaaaaaa\n",
		"git rev-parse origin/release/":   "aaaaaaaaaaaaaaaa\n",
		"git describe":                    "v0.1.0\n",
		"gh run list":                     `[{"name":"CI","status":"completed","conclusion":"success"}]`,
		"gh issue list":                   "[]",
		"git cliff":                       cliffSection,
	}}
}

func TestPrepareWritesAndCommits(t *testing.T) {
	t.Parallel()

	f := happy()
	tl := newTool(t, f)

	if err := tl.prepare("v0.2.0"); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(tl.changelogPath())
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(got), "## [0.2.0] - 2026-11-20") ||
		!strings.Contains(string(got), "vukem-studio/releases/tag/v0.1.0") {
		t.Errorf("CHANGELOG.md =\n%s", got)
	}

	if !f.ran("git add CHANGELOG.md") || !f.ran("git commit -m chore(release): v0.2.0") {
		t.Errorf("calls = %v", f.calls)
	}

	if !f.ran("git cliff --config .config/cliff.toml v0.1.0..HEAD --tag v0.2.0 --strip all") {
		t.Errorf("cliff was not asked for the range after the previous tag: %v", f.calls)
	}
}

func TestPrepareMaintenanceEntryWhenNothingUserFacing(t *testing.T) {
	t.Parallel()

	f := happy()
	f.answers["git cliff"] = "## [0.2.0] - 2026-11-20\n"
	tl := newTool(t, f)

	if err := tl.prepare("v0.2.0"); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(tl.changelogPath())
	if !strings.Contains(string(got), "Maintenance release (no user-facing changes).") {
		t.Errorf("no maintenance entry:\n%s", got)
	}
}

func TestPrepareRefusals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(f *fake)
		wantErr string
	}{
		{
			"on main",
			func(f *fake) { f.answers["git rev-parse --abbrev-ref HEAD"] = "main\n" },
			"refusing to run on main",
		},
		{
			"another branch",
			func(f *fake) { f.answers["git rev-parse --abbrev-ref HEAD"] = "feat/x\n" },
			"want release/v0.2.0",
		},
		{"dirty tree", func(f *fake) { f.answers["git status"] = " M a.go\n" }, "not clean"},
		{
			"branch not pushed as it is",
			func(f *fake) { f.answers["git rev-parse origin/release/"] = "bbbbbbbbbbbbbbbb\n" },
			"push the branch",
		},
		{
			"branch not on origin",
			func(f *fake) {
				f.errs = map[string]error{"git rev-parse origin/release/": errors.New("unknown revision")}
			},
			"not on origin",
		},
		{
			"behind origin/main",
			func(f *fake) {
				f.errs = map[string]error{"git merge-base --is-ancestor": errors.New("exit status 1")}
			},
			"does not contain origin/main",
		},
		{"tag exists locally", func(f *fake) { f.answers["git tag --list"] = "v0.2.0\n" }, "already exists locally"},
		{
			"tag exists on origin",
			func(f *fake) { f.answers["git ls-remote"] = "abc\trefs/tags/v0.2.0\n" },
			"already exists on origin",
		},
		{"red run", func(f *fake) {
			f.answers["gh run list"] = `[{"name":"CI","status":"completed","conclusion":"failure"}]`
		}, "not all green"},
		{"run still going", func(f *fake) {
			f.answers["gh run list"] = `[{"name":"CI","status":"in_progress","conclusion":""}]`
		}, "not all green"},
		{"no run", func(f *fake) { f.answers["gh run list"] = "[]" }, "no workflow run"},
		{"ci-failure issue", func(f *fake) { f.answers["gh issue list"] = `[{"number":7}]` }, "ci-failure"},
		{"older than the previous tag", func(f *fake) { f.answers["git describe"] = "v0.3.0\n" }, "not newer"},
		{"git cliff fails", func(f *fake) { f.errs = map[string]error{"git cliff": errors.New("boom")} }, "git cliff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := happy()
			tt.mutate(f)
			tl := newTool(t, f)

			err := tl.prepare("v0.2.0")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("prepare() error = %v, want %q", err, tt.wantErr)
			}

			if f.ran("git commit") {
				t.Error("a refused prepare committed")
			}

			got, _ := os.ReadFile(tl.changelogPath())
			if string(got) != base {
				t.Error("a refused prepare changed CHANGELOG.md")
			}
		})
	}
}

func TestPatchReleaseRefusesABreakingCommit(t *testing.T) {
	t.Parallel()

	if !isPatchRelease("v0.2.1") || isPatchRelease("v0.2.0") || isPatchRelease("v1.0.0") {
		t.Fatal("isPatchRelease misreads the patch number")
	}

	f := happy()
	f.answers["git describe"] = "v0.2.0\n"
	f.answers["git rev-parse --abbrev-ref HEAD"] = "release/v0.2.1\n"
	f.answers["git log"] = "fix(core): keep the order\nfeat(cli)!: rename a flag\n"
	tl := newTool(t, f)

	err := tl.prepare("v0.2.1")
	if err == nil || !strings.Contains(err.Error(), "patch release") ||
		!strings.Contains(err.Error(), "rename a flag") {
		t.Fatalf("prepare() error = %v", err)
	}

	if f.ran("git commit") {
		t.Error("a refused prepare committed")
	}

	// The same commits are fine in a minor.
	f.answers["git rev-parse --abbrev-ref HEAD"] = "release/v0.2.0\n"
	f.answers["git describe"] = "v0.1.0\n"
	if err := newTool(t, f).prepare("v0.2.0"); err != nil {
		t.Fatalf("a minor with a breaking commit: %v", err)
	}
}

func TestPrepareRefusesAVersionAlreadyInTheChangelog(t *testing.T) {
	t.Parallel()

	f := happy()
	f.answers["git rev-parse --abbrev-ref HEAD"] = "release/v0.1.0\n"
	f.answers["git describe"] = "v0.0.9\n"
	f.answers["git cliff"] = "## [0.1.0] - 2026-11-20\n\n### Added\n\n- x\n"
	tl := newTool(t, f)

	if err := tl.prepare("v0.1.0"); err == nil || !strings.Contains(err.Error(), "already has a section") {
		t.Fatalf("prepare() error = %v", err)
	}
}

func TestSkipRemoteChecks(t *testing.T) {
	t.Parallel()

	f := happy()
	f.answers["git rev-parse origin/main"] = "bbbbbbbbbbbbbbbb\n"
	f.answers["gh run list"] = "[]"
	tl := newTool(t, f)
	tl.skipRemote = true

	if err := tl.prepare("v0.2.0"); err != nil {
		t.Fatal(err)
	}

	if f.ran("gh ") || f.ran("git fetch") || f.ran("git ls-remote") {
		t.Errorf("a remote command ran: %v", f.calls)
	}
}

func tagTool(t *testing.T, f *fake, changelog string) *tool {
	t.Helper()

	tl := newTool(t, f)
	if err := os.WriteFile(tl.changelogPath(), []byte(changelog), 0o600); err != nil {
		t.Fatal(err)
	}

	return tl
}

const released = "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - 2026-11-20\n\n### Added\n\n- x\n\n[0.2.0]: https://example.test\n"

func TestTagCreatesAndPushes(t *testing.T) {
	t.Parallel()

	f := happy()
	f.answers["git log -1"] = "chore(release): v0.2.0 (#50)\n"
	tl := tagTool(t, f, released)

	if err := tl.tag("v0.2.0"); err != nil {
		t.Fatal(err)
	}

	if !f.ran("git tag -a v0.2.0 -m v0.2.0") || !f.ran("git push origin v0.2.0") {
		t.Errorf("calls = %v", f.calls)
	}
}

func TestTagRefusals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		changelog string
		mutate    func(f *fake)
		wantErr   string
	}{
		{
			"wrong subject",
			released,
			func(f *fake) { f.answers["git log -1"] = "feat(core): something\n" },
			"want a subject",
		},
		{"not origin/main", released, func(f *fake) {
			f.answers["git rev-parse origin/main"] = "bbbbbbbbbbbbbbbb\n"
		}, "not origin/main"},
		{"dirty tree", released, func(f *fake) { f.answers["git status"] = " M a\n" }, "not clean"},
		{"tag exists", released, func(f *fake) { f.answers["git tag --list"] = "v0.2.0\n" }, "already exists"},
		{"empty notes", "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] - d\n", nil, "no entries"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := happy()
			f.answers["git log -1"] = "chore(release): v0.2.0 (#50)\n"

			if tt.mutate != nil {
				tt.mutate(f)
			}

			tl := tagTool(t, f, tt.changelog)

			err := tl.tag("v0.2.0")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("tag() error = %v, want %q", err, tt.wantErr)
			}

			if f.ran("git tag -a") || f.ran("git push") {
				t.Errorf("a refused tag created or pushed: %v", f.calls)
			}
		})
	}
}

func TestNotes(t *testing.T) {
	t.Parallel()

	tl := tagTool(t, happy(), released)
	if err := tl.notes("v0.2.0"); err != nil {
		t.Fatal(err)
	}

	if got := tl.out.(*bytes.Buffer).String(); got != "### Added\n\n- x\n" {
		t.Errorf("notes = %q", got)
	}

	if err := tl.notes("v0.9.0"); err == nil {
		t.Error("notes of an absent version succeeded")
	}
}

func TestCLIRejectsBadInput(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{}, {"prepare"}, {"prepare", "0.2.0"}, {"frobnicate", "v0.2.0"}} {
		if err := cli(args, &bytes.Buffer{}); err == nil {
			t.Errorf("cli(%v) = nil", args)
		}
	}
}

// afterOne makes happy() answer as a repository whose last tag is v1.0.0 and
// whose release branch is release/<version>.
func afterOne(version, log string) *fake {
	f := happy()
	f.answers["git describe"] = "v1.0.0\n"
	f.answers["git rev-parse --abbrev-ref HEAD"] = "release/" + version + "\n"
	f.answers["git log"] = log
	f.answers["git cliff"] = strings.ReplaceAll(cliffSection, "0.2.0", strings.TrimPrefix(version, "v"))

	return f
}

func TestAfterOneABreakingCommitNeedsAMajor(t *testing.T) {
	t.Parallel()

	logs := map[string]string{
		"bang":   "fix(core): keep the order\nfeat(cli)!: rename a flag\n",
		"footer": "feat(cli): rename a flag\n\nBREAKING CHANGE: the flag is now --out\n",
		"hyphen": "feat(cli): rename a flag\n\nBREAKING-CHANGE: the flag is now --out\n",
	}

	for name, log := range logs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for _, v := range []string{"v1.1.0", "v1.0.1"} {
				f := afterOne(v, log)

				err := newTool(t, f).prepare(v)
				if err == nil || !strings.Contains(err.Error(), "major") {
					t.Fatalf("prepare(%s) error = %v, want a major-bump refusal", v, err)
				}

				if f.ran("git commit") {
					t.Error("a refused prepare committed")
				}
			}

			f := afterOne("v2.0.0", log)
			if err := newTool(t, f).prepare("v2.0.0"); err != nil {
				t.Fatalf("prepare(v2.0.0) with a breaking commit: %v", err)
			}

			if !f.ran("git commit") {
				t.Error("an accepted prepare did not commit")
			}
		})
	}
}

func TestAfterOneWithoutABreakingCommitAMinorIsFine(t *testing.T) {
	t.Parallel()

	f := afterOne("v1.1.0", "feat(core): add a command\nfix(cli): trim the output\n")
	if err := newTool(t, f).prepare("v1.1.0"); err != nil {
		t.Fatalf("prepare(v1.1.0): %v", err)
	}
}

func TestOneZeroIsCutFromTheZeroSeriesWithABreakingCommit(t *testing.T) {
	t.Parallel()

	f := afterOne("v1.0.0", "feat(core)!: freeze the surface\n")
	f.answers["git describe"] = "v0.9.0\n"

	if err := newTool(t, f).prepare("v1.0.0"); err != nil {
		t.Fatalf("prepare(v1.0.0) from v0.9.0: %v", err)
	}
}

func TestBeforeOneAMinorMayBreak(t *testing.T) {
	t.Parallel()

	f := happy()
	f.answers["git log"] = "feat(cli)!: rename a flag\n"

	if err := newTool(t, f).prepare("v0.2.0"); err != nil {
		t.Fatalf("a v0 minor with a breaking commit: %v", err)
	}
}
