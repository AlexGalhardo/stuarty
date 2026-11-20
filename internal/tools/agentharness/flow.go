package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	corepub "github.com/AlexGalhardo/stuarty/core/publish"
)

// The first complete flow as a task (epic 38, ADR-0031, NFR-066): the agent gets the brief and the
// built-in skills, the services behind Stuarty are fakes (an AI provider and a social network on
// loopback), and the run succeeds only when a person-approved, published post exists and the exported
// file that went out passes its checks. The harness plays the person: an agent cannot approve, so
// after the agent stops, the harness approves what waits in the queue and lets the post go out.
//
// Every run of a series uses the same server and workspace, so a run is judged only by what it added:
// the publications that were not there when it started, and the uploads the network received since.

// firstFlowPrompt is all the agent is told: the brief, no command names. The time is far enough ahead
// that the post is still on time when the harness, after the agent stopped, approves it.
const firstFlowPrompt = "Make a short vertical video for YouTube Shorts from the file clip.mp4 in the working " +
	"directory, about baking a first sourdough loaf: write the script, cut the footage to 4 seconds, " +
	"put captions from the speech on it, make a thumbnail, export it for YouTube Shorts, and " +
	"schedule it for fifteen minutes from now to the connected account. You are not allowed to " +
	"approve anything: stop when the post is scheduled and waiting for approval."

// Person is what a person does that an agent cannot: decide in the approval queue.
type Person interface {
	// ApprovePending approves every request that waits and says how many there were.
	ApprovePending(ctx context.Context) (int, error)
}

// Network is the fake social network, read as the world outside Stuarty sees it.
type Network interface {
	// Uploads lists what the network received, in order.
	Uploads(ctx context.Context) ([]upload, error)
}

// upload is one post the fake network received (internal/tools/firstflowfake).
type upload struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// flowEnv is the world a first-flow run happens in.
type flowEnv struct {
	person  Person
	network Network
	// wait is how long the post may take to go out after the approval, or after its time when that is
	// later; poll is the step.
	wait, poll time.Duration
	sleep      func(time.Duration)
	now        func() time.Time
}

// flowBefore is what the workspace and the network held when a run started.
type flowBefore struct {
	pubs    map[string]bool
	uploads int
}

// flowPublication is the part of a publication of publication:list the check reads.
type flowPublication struct {
	ID      string `json:"id"`
	Export  string `json:"export"`
	Targets []struct {
		State       corepub.TargetState `json:"state"`
		ScheduledAt *time.Time          `json:"scheduled_at"`
		ApprovalID  string              `json:"approval_id"`
	} `json:"targets"`
}

// maxFlowWait bounds the wait for a post whose time the agent set far ahead.
const maxFlowWait = 30 * time.Minute

var (
	reBytes      = regexp.MustCompile(`"bytes":(\d+)`)
	errNoService = errors.New("the run has no fake services")
)

func (env *flowEnv) clock() time.Time {
	if env.now != nil {
		return env.now()
	}

	return time.Now()
}

// publications reads the calendar of the workspace.
func publications(ctx context.Context, run Runner) ([]flowPublication, error) {
	out, code, err := run(ctx, []string{"publication:list", "--limit", "500", "--json"}, "")
	if err != nil || code != 0 {
		return nil, statusErr("publication:list", code, err)
	}

	var list struct {
		Publications []flowPublication `json:"publications"`
	}

	start := strings.Index(out, "{")
	if start < 0 {
		return nil, errors.New("publication:list answered no JSON")
	}

	if err := json.Unmarshal([]byte(out[start:]), &list); err != nil {
		return nil, fmt.Errorf("publication:list answered no JSON: %w", err)
	}

	return list.Publications, nil
}

// beforeFlow records what the workspace and the network hold before the agent starts, or says why it
// could not. It runs before the first turn, so a run whose services are missing costs nothing.
func beforeFlow(ctx context.Context, env *flowEnv, run Runner) (flowBefore, string) {
	if env == nil || env.person == nil || env.network == nil {
		return flowBefore{}, errNoService.Error()
	}

	sent, err := env.network.Uploads(ctx)
	if err != nil {
		return flowBefore{}, "read the network: " + err.Error()
	}

	pubs, err := publications(ctx, run)
	if err != nil {
		return flowBefore{}, err.Error()
	}

	b := flowBefore{pubs: map[string]bool{}, uploads: len(sent)}
	for _, p := range pubs {
		b.pubs[p.ID] = true
	}

	return b, ""
}

// fresh is the publications of the run: those that were not there when it started.
func fresh(all []flowPublication, before flowBefore) []flowPublication {
	var out []flowPublication

	for _, p := range all {
		if !before.pubs[p.ID] {
			out = append(out, p)
		}
	}

	return out
}

// settleFlow is the success check of the first flow, run after the agent stopped:
//  1. nothing went out while the request waited (the post is never sent without approval);
//  2. a person approves, and the post goes out at its time;
//  3. a publication of the run shows a published target that carries an approval;
//  4. the exported file of that publication succeeded with every check ok, and it is the one file the
//     network received during the run.
func settleFlow(ctx context.Context, env *flowEnv, run Runner, before flowBefore) (bool, string) {
	if env == nil || env.person == nil || env.network == nil {
		return false, errNoService.Error()
	}

	sent, err := env.network.Uploads(ctx)
	if err != nil {
		return false, "read the network: " + err.Error()
	}

	if len(sent) != before.uploads {
		return false, "a post went out before a person approved it"
	}

	all, err := publications(ctx, run)
	if err != nil {
		return false, err.Error()
	}

	mine := fresh(all, before)
	if len(mine) == 0 {
		return false, "the run created no publication"
	}

	deadline := env.clock().Add(env.wait)

	for _, p := range mine {
		for _, t := range p.Targets {
			if t.State == corepub.TargetPublishing || t.State == corepub.TargetPublished {
				return false, "a post went out before a person approved it"
			}

			if t.ScheduledAt != nil && t.ScheduledAt.Add(env.wait).After(deadline) {
				deadline = t.ScheduledAt.Add(env.wait)
			}
		}
	}

	if limit := env.clock().Add(maxFlowWait); deadline.After(limit) {
		deadline = limit
	}

	if _, err := env.person.ApprovePending(ctx); err != nil {
		return false, "approve: " + err.Error()
	}

	pub, why := waitPublished(ctx, env, run, before, deadline)
	if pub == nil {
		return false, why
	}

	shown, code, err := run(ctx, []string{"job:show", pub.Export, "--json"}, "")
	if err != nil || code != 0 {
		return false, fmt.Sprintf("job:show failed (exit %d): %v", code, err)
	}

	if ok, why := exportPassed(shown); !ok {
		return false, why
	}

	sent, err = env.network.Uploads(ctx)
	if err != nil || len(sent) != before.uploads+1 {
		return false, fmt.Sprintf("want one upload at the network, got %d (%v)", len(sent)-before.uploads, err)
	}

	m := reBytes.FindStringSubmatch(shown)
	if len(m) != 2 || strconv.FormatInt(sent[len(sent)-1].Bytes, 10) != m[1] {
		return false, "the file that went out is not the exported video"
	}

	return true, ""
}

// waitPublished polls the calendar until a publication of the run has a published target, and says why
// when none will: every target of the run ended another way, or the deadline passed.
func waitPublished(
	ctx context.Context, env *flowEnv, run Runner, before flowBefore, deadline time.Time,
) (*flowPublication, string) {
	for {
		all, err := publications(ctx, run)
		if err != nil {
			return nil, err.Error()
		}

		open, ended := false, ""

		for _, p := range fresh(all, before) {
			for _, t := range p.Targets {
				switch {
				case t.State == corepub.TargetPublished && t.ApprovalID == "":
					return nil, "the published target carries no approval"
				case t.State == corepub.TargetPublished && p.Export == "":
					return nil, "the publication names no exported video"
				case t.State == corepub.TargetPublished:
					return &p, ""
				case t.State.Final() || t.State == corepub.TargetMissed || t.State == corepub.TargetFailed:
					ended = string(t.State)
				default:
					open = true
				}
			}
		}

		if !open && ended != "" {
			return nil, "the post was not published: its target is " + ended
		}

		if env.clock().After(deadline) {
			return nil, "no target was published in time"
		}

		sleep(env, env.poll)
	}
}

// exportPassed reads the answer of job:show for an export: succeeded, and no failed check.
func exportPassed(shown string) (bool, string) {
	var job struct {
		State  string `json:"state"`
		Result struct {
			Export struct {
				Checks []struct {
					Name string `json:"name"`
					OK   bool   `json:"ok"`
				} `json:"checks"`
			} `json:"export"`
		} `json:"result"`
	}

	start := strings.Index(shown, "{")
	if start < 0 {
		return false, "job:show answered no JSON"
	}

	if err := json.Unmarshal([]byte(shown[start:]), &job); err != nil {
		return false, "job:show answered no JSON"
	}

	if job.State != "succeeded" {
		return false, "the export did not succeed: " + job.State
	}

	if len(job.Result.Export.Checks) == 0 {
		return false, "the export reports no checks"
	}

	for _, c := range job.Result.Export.Checks {
		if !c.OK {
			return false, "the export failed its check " + c.Name
		}
	}

	return true, ""
}

// builtinSkills is the text of every built-in skill of dir, in name order: what the agent of the first
// flow knows besides the discovery skill and `help`.
func builtinSkills(dir string) (string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*", "SKILL.md"))
	if err != nil || len(files) == 0 {
		return "", fmt.Errorf("no built-in skills in %s", dir)
	}

	var b strings.Builder

	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("read a skill: %w", err)
		}

		b.WriteString("\n\n---\n\n")
		b.Write(raw)
	}

	return b.String(), nil
}

func sleep(env *flowEnv, d time.Duration) {
	if env.sleep != nil {
		env.sleep(d)

		return
	}

	time.Sleep(d)
}
