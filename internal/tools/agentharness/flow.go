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
)

// The first complete flow as a task (epic 38, ADR-0031, NFR-066): the agent gets the brief and the
// built-in skills, the services behind Stuarty are fakes (an AI provider and a social network on
// loopback), and the run succeeds only when a person-approved, published post exists and the exported
// file that went out passes its checks. The harness plays the person: an agent cannot approve, so
// after the agent stops, the harness approves what waits in the queue and lets the post go out.

// firstFlowPrompt is all the agent is told: the brief, no command names.
const firstFlowPrompt = "Make a short vertical video for YouTube Shorts from the file clip.mp4 in the working " +
	"directory, about baking a first sourdough loaf: write the script, cut the footage to 4 seconds, " +
	"put captions from the speech on it, make a thumbnail, export it for YouTube Shorts, and " +
	"schedule it for a few minutes from now to the connected account. You are not allowed to " +
	"approve anything: stop when the post is scheduled and waiting for approval."

// Person is what a person does that an agent cannot: decide in the approval queue.
type Person interface {
	// ApprovePending approves every request that waits and says how many there were.
	ApprovePending(ctx context.Context) (int, error)
}

// Network is the fake social network, read as the world outside Stuarty sees it.
type Network interface {
	// Uploads lists what the network received.
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
	// wait is how long the post may take to go out after the approval; poll is the step.
	wait, poll time.Duration
	sleep      func(time.Duration)
}

var (
	reExport     = regexp.MustCompile(`"export":"(job_[0-9a-z]{10})"`)
	reApproval   = regexp.MustCompile(`"approval_id":"apr_[0-9a-z]{10}"`)
	rePublished  = regexp.MustCompile(`"state":"published"`)
	reBytes      = regexp.MustCompile(`"bytes":(\d+)`)
	errNoService = errors.New("the run has no fake services")
)

// settleFlow is the success check of the first flow, run after the agent stopped:
//  1. nothing went out while the request waited (the post is never sent without approval);
//  2. a person approves, and the post goes out;
//  3. the publication shows a published target that carries an approval;
//  4. the exported file of that publication succeeded with every check ok, and it is the file the
//     network received.
func settleFlow(ctx context.Context, env *flowEnv, run Runner) (bool, string) {
	if env == nil || env.person == nil || env.network == nil {
		return false, errNoService.Error()
	}

	before, err := env.network.Uploads(ctx)
	if err != nil {
		return false, "read the network: " + err.Error()
	}

	if len(before) != 0 {
		return false, "a post went out before a person approved it"
	}

	if _, err := env.person.ApprovePending(ctx); err != nil {
		return false, "approve: " + err.Error()
	}

	var pub string

	deadline := time.Now().Add(env.wait)

	for {
		out, code, err := run(ctx, []string{"publication:list", "--json"}, "")
		if err != nil || code != 0 {
			return false, fmt.Sprintf("publication:list failed (exit %d): %v", code, err)
		}

		if rePublished.MatchString(out) {
			pub = out

			break
		}

		if time.Now().After(deadline) {
			return false, "no target was published in time"
		}

		sleep(env, env.poll)
	}

	if !reApproval.MatchString(pub) {
		return false, "the published target carries no approval"
	}

	job := reExport.FindStringSubmatch(pub)
	if job == nil {
		return false, "the publication names no exported video"
	}

	shown, code, err := run(ctx, []string{"job:show", job[1], "--json"}, "")
	if err != nil || code != 0 {
		return false, fmt.Sprintf("job:show failed (exit %d): %v", code, err)
	}

	if ok, why := exportPassed(shown); !ok {
		return false, why
	}

	sent, err := env.network.Uploads(ctx)
	if err != nil || len(sent) != 1 {
		return false, fmt.Sprintf("want one upload at the network, got %d (%v)", len(sent), err)
	}

	m := reBytes.FindStringSubmatch(shown)
	if len(m) != 2 || strconv.FormatInt(sent[0].Bytes, 10) != m[1] {
		return false, "the file that went out is not the exported video"
	}

	return true, ""
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
