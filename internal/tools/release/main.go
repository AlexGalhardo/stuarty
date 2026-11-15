// Command release prepares and tags a release of Vukem Video (SDD-009
// sections 7 and 8, ADR-0028).
//
//	release prepare vX.Y.Z   on branch release/vX.Y.Z, pushed and green: write the changelog section, commit it
//	release tag vX.Y.Z       on main, after the release pull request merged: tag and push the tag
//	release notes vX.Y.Z     print the notes release.yml will publish
//	release dispatch vX.Y.Z  start release.yml by hand (recovery)
//
// It never runs on main when it writes (prepare refuses), never reuses a
// version and never publishes empty notes. It does not create the GitHub
// release: release.yml does, from the changelog section, when the tag is
// pushed. Standard library only; git, git-cliff and gh are external commands.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runner runs an external command in dir and returns its standard output.
type runner func(dir, name string, args ...string) (string, error)

// tool carries what a command needs, so tests can replace the commands.
type tool struct {
	dir string
	run runner
	out io.Writer
	// skipRemote skips the checks that need the network and the state of
	// origin: HEAD equal to origin/main, green runs, no ci-failure issue,
	// no remote tag. It is for a dry run in a clone or a fork.
	skipRemote bool
}

func main() {
	if err := cli(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

const usage = "usage: release [-skip-remote-checks] prepare|tag|notes|dispatch vX.Y.Z"

func cli(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	skip := fs.Bool("skip-remote-checks", false, "skip the checks that need origin and the network (dry runs only)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 2 {
		return errors.New(usage)
	}

	t := &tool{dir: ".", run: execRunner, out: out, skipRemote: *skip}
	cmd, version := fs.Arg(0), fs.Arg(1)

	if !validVersion(version) {
		return fmt.Errorf("%q is not a version tag (vX.Y.Z)", version)
	}

	switch cmd {
	case "prepare":
		return t.prepare(version)
	case "tag":
		return t.tag(version)
	case "notes":
		return t.notes(version)
	case "dispatch":
		_, err := t.run(t.dir, "gh", "workflow", "run", "release.yml", "-f", "version="+version)
		return err
	default:
		return errors.New(usage)
	}
}

func execRunner(dir, name string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer

	// Fixed binaries and a version argument checked by validVersion.
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
}

// git runs git and returns its trimmed output.
func (t *tool) git(args ...string) (string, error) {
	s, err := t.run(t.dir, "git", args...)
	return strings.TrimSpace(s), err
}

// checkClean refuses a tree with changes to tracked files.
func (t *tool) checkClean() error {
	dirty, err := t.git("status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return err
	}

	if dirty != "" {
		return fmt.Errorf("the working tree is not clean:\n%s", dirty)
	}

	return nil
}

// checkVersionFree refuses a version whose tag exists here or on origin.
func (t *tool) checkVersionFree(version string) error {
	if s, _ := t.git("tag", "--list", version); s != "" {
		return fmt.Errorf("the tag %s already exists locally", version)
	}

	if t.skipRemote {
		return nil
	}

	s, err := t.git("ls-remote", "--tags", "origin", "refs/tags/"+version)
	if err != nil {
		return err
	}

	if s != "" {
		return fmt.Errorf("the tag %s already exists on origin", version)
	}

	return nil
}

// checkAtOriginMain refuses a HEAD that is not the commit origin/main points to.
func (t *tool) checkAtOriginMain() error {
	if t.skipRemote {
		return nil
	}

	if _, err := t.git("fetch", "origin", "--tags"); err != nil {
		return err
	}

	head, err := t.git("rev-parse", "HEAD")
	if err != nil {
		return err
	}

	main, err := t.git("rev-parse", "origin/main")
	if err != nil {
		return err
	}

	if head != main {
		return fmt.Errorf("HEAD %s is not origin/main %s: update main first", short(head), short(main))
	}

	return nil
}

// checkReleaseBranch refuses a release branch that is not pushed as it is, or
// that does not contain origin/main. The release pull request carries every
// commit of the release (D-031), so prepare runs on its tip, not on main.
func (t *tool) checkReleaseBranch(version string) error {
	if t.skipRemote {
		return nil
	}

	if _, err := t.git("fetch", "origin", "--tags"); err != nil {
		return err
	}

	head, err := t.git("rev-parse", "HEAD")
	if err != nil {
		return err
	}

	remote, err := t.git("rev-parse", "origin/release/"+version)
	if err != nil {
		return fmt.Errorf("release/%s is not on origin: push it first: %w", version, err)
	}

	if head != remote {
		return fmt.Errorf("HEAD %s is not origin/release/%s %s: push the branch and wait for its checks",
			short(head), version, short(remote))
	}

	if _, err := t.git("merge-base", "--is-ancestor", "origin/main", "HEAD"); err != nil {
		return fmt.Errorf("release/%s does not contain origin/main: rebase it on main first", version)
	}

	return nil
}

func short(sha string) string {
	if len(sha) > 9 {
		return sha[:9]
	}

	return sha
}

// checkGreen refuses a tip of branch with a workflow run that is not a success
// and refuses one with no run at all (SDD-009 section 7 step 2c), and refuses
// an open ci-failure issue (step 2d).
func (t *tool) checkGreen(branch string) error {
	if t.skipRemote {
		return nil
	}

	sha, err := t.git("rev-parse", "HEAD")
	if err != nil {
		return err
	}

	raw, err := t.run(t.dir, "gh", "run", "list", "--branch", branch, "--commit", sha, "--limit", "50",
		"--json", "name,status,conclusion")
	if err != nil {
		return err
	}

	var runs []struct {
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	}
	if err := json.Unmarshal([]byte(raw), &runs); err != nil {
		return fmt.Errorf("gh run list: %w", err)
	}

	if len(runs) == 0 {
		return fmt.Errorf("no workflow run found for %s", short(sha))
	}

	var bad []string

	for _, r := range runs {
		if r.Status != "completed" || (r.Conclusion != "success" && r.Conclusion != "skipped") {
			bad = append(bad, fmt.Sprintf("%s (%s %s)", r.Name, r.Status, r.Conclusion))
		}
	}

	if len(bad) > 0 {
		return fmt.Errorf("the runs of %s are not all green: %s", short(sha), strings.Join(bad, ", "))
	}

	raw, err = t.run(t.dir, "gh", "issue", "list", "--label", "ci-failure", "--state", "open", "--json", "number")
	if err != nil {
		return err
	}

	var issues []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal([]byte(raw), &issues); err != nil {
		return fmt.Errorf("gh issue list: %w", err)
	}

	if len(issues) > 0 {
		return fmt.Errorf("%d open issue(s) labelled ci-failure: triage them first", len(issues))
	}

	return nil
}

func (t *tool) changelogPath() string { return filepath.Join(t.dir, "CHANGELOG.md") }

// prepare writes the changelog section of version and commits it. It runs on
// the branch release/<version> and refuses main (ADR-0028: no direct commit
// to main).
func (t *tool) prepare(version string) error {
	branch, err := t.git("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}

	switch {
	case branch == "main":
		return errors.New(
			"refusing to run on main: create release/" + version + " first (git switch -c release/" + version + ")",
		)
	case branch != "release/"+version:
		return fmt.Errorf("the branch is %q, want release/%s", branch, version)
	}

	if err := t.checkClean(); err != nil {
		return err
	}

	if err := t.checkReleaseBranch(version); err != nil {
		return err
	}

	if err := t.checkGreen("release/" + version); err != nil {
		return err
	}

	if err := t.checkVersionFree(version); err != nil {
		return err
	}

	previous, _ := t.git("describe", "--tags", "--abbrev=0", "--match", "v[0-9]*", "HEAD")
	if !newerThan(version, previous) {
		return fmt.Errorf("%s is not newer than the previous tag %s", version, previous)
	}

	rangeArg := "HEAD"
	if previous != "" {
		rangeArg = previous + "..HEAD"
	}

	needsMajor := previous != "" && stable(previous) && !isMajorBump(version, previous)
	if isPatchRelease(version) || needsMajor {
		log, lerr := t.git("log", "--format=%B", rangeArg)
		if lerr != nil {
			return lerr
		}

		if breaking := breakingSubjects(log); len(breaking) > 0 {
			if needsMajor {
				return fmt.Errorf(
					"%s is not a major bump over %s but %s carries a breaking change: %q; after 1.0 cut the next major",
					version, previous, rangeArg, breaking[0],
				)
			}

			return fmt.Errorf(
				"%s is a patch release but %s carries a breaking change: %q; cut a minor instead",
				version, rangeArg, breaking[0],
			)
		}
	}

	section, err := t.run(
		t.dir,
		"git",
		"cliff",
		"--config",
		".config/cliff.toml",
		rangeArg,
		"--tag",
		version,
		"--strip",
		"all",
	)
	if err != nil {
		return fmt.Errorf("git cliff: %w", err)
	}

	// SDD-009 section 5.1 rule 6: a release body is never empty.
	if !strings.Contains(section, "### ") {
		section = strings.TrimSpace(section) + "\n\n### Changed\n\n- Maintenance release (no user-facing changes).\n"
	}

	current, err := os.ReadFile(t.changelogPath())
	if err != nil {
		return err
	}

	updated, err := insertRelease(string(current), section, version, previous)
	if err != nil {
		return err
	}

	if releaseNotes(updated, version) == "" {
		return fmt.Errorf("refusing to continue: the notes of %s would be empty", version)
	}

	if err := os.WriteFile(t.changelogPath(), []byte(updated), 0o600); err != nil {
		return err
	}

	if _, err := t.git("add", "CHANGELOG.md"); err != nil {
		return err
	}

	if _, err := t.git("commit", "-m", "chore(release): "+version); err != nil {
		return err
	}

	fmt.Fprintf(
		t.out,
		"CHANGELOG.md updated for %s (previous %s) and committed on %s\n",
		version,
		orNone(previous),
		branch,
	)

	return nil
}

// tag creates the annotated tag on the merged release commit and pushes it.
func (t *tool) tag(version string) error {
	if err := t.checkClean(); err != nil {
		return err
	}

	if err := t.checkAtOriginMain(); err != nil {
		return err
	}

	subject, err := t.git("log", "-1", "--format=%s")
	if err != nil {
		return err
	}

	if want := "chore(release): " + version; !strings.HasPrefix(subject, want) {
		return fmt.Errorf("HEAD is %q, want a subject starting with %q", subject, want)
	}

	if err := t.checkVersionFree(version); err != nil {
		return err
	}

	current, err := os.ReadFile(t.changelogPath())
	if err != nil {
		return err
	}

	if releaseNotes(string(current), version) == "" {
		return fmt.Errorf("CHANGELOG.md has no entries for %s: refusing to tag", version)
	}

	if _, err := t.git("tag", "-a", version, "-m", version); err != nil {
		return err
	}

	if t.skipRemote {
		fmt.Fprintf(t.out, "tagged %s (dry run: not pushed)\n", version)
		return nil
	}

	if _, err := t.git("push", "origin", version); err != nil {
		return err
	}

	fmt.Fprintf(t.out, "tagged and pushed %s; release.yml publishes it\n", version)

	return nil
}

// notes prints the body release.yml will publish; it fails when that is empty.
func (t *tool) notes(version string) error {
	current, err := os.ReadFile(t.changelogPath())
	if err != nil {
		return err
	}

	n := releaseNotes(string(current), version)
	if n == "" {
		return fmt.Errorf("CHANGELOG.md has no entries for %s", version)
	}

	fmt.Fprint(t.out, n)

	return nil
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}

	return s
}
