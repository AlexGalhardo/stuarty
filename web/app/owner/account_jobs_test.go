package owner

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/approval"
	"github.com/AlexGalhardo/stuarty/service/publish/destinations"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

const (
	canaryPrompt   = "CANARY-PROMPT-the-brief-sent-to-a-model"
	canaryAnswer   = "CANARY-ANSWER-what-the-model-wrote"
	canaryExcerpt  = "CANARY-EXCERPT-of-the-prompt"
	canaryMarkdown = "CANARY-MARKDOWN-body-of-the-post"
	canaryHTML     = "CANARY-HTML-body-of-the-post"
)

// The prompt of a generation, what a model wrote, the excerpt an approval quotes and the body of a published post
// are the creator's words too, in columns of their own.
func TestSecurity_T109_TheWordsInJobsApprovalsAndPublishedPostsAreSealed(t *testing.T) {
	getenv := accountEnv(t)
	home := getenv(workspace.EnvHome)
	ctx := context.Background()

	o, _, project := existingWorkspace(t, getenv)

	defer func() { _ = o.Close() }()

	jobs := o.Store.Jobs()
	input := json.RawMessage(`{"request":"` + canaryPrompt + `"}`)
	result := json.RawMessage(`{"text":"` + canaryAnswer + `"}`)

	if err := jobs.CreateLimited(ctx, service.JobRow{
		ID: "job_words", Kind: service.JobTextDraft, ProjectID: &project, Actor: "local:ui", Input: input,
		CreatedMs: 1, UpdatedMs: 1,
	}, nil); err != nil {
		t.Fatal(err)
	}

	if ok, err := jobs.Claim(ctx, "job_words", 2); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}

	if ok, err := jobs.Finish(ctx, "job_words", service.JobSucceeded, result, nil, 3); err != nil || !ok {
		t.Fatalf("finish: %v %v", ok, err)
	}

	options := json.RawMessage(`{"preset":"youtube"}`)
	if err := jobs.CreateLimited(ctx, service.JobRow{
		ID: "job_clear", Kind: service.JobExport, ProjectID: &project, Actor: "local:ui", Input: options,
		CreatedMs: 1, UpdatedMs: 1,
	}, nil); err != nil {
		t.Fatal(err)
	}

	if err := o.Store.Approvals().InsertRequest(ctx, &approval.Row{
		ID: "apr_words", Actor: "local:ui", Kind: "text.generate", Summary: "Starts with: " + canaryExcerpt,
		State: "pending", CreatedMs: 1, ExpiresMs: 4102444800000,
	}); err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

	if err := o.Store.Blog().PutPublished(ctx, destinations.PublishedPost{
		Slug: "the-post", PostItemID: "pit_post", ProjectID: "prj_post", ConnectionID: "con_blog", Status: "published",
		Markdown: canaryMarkdown, HTML: canaryHTML, PublishedAt: at, UpdatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}

	checkpoint(t, o)

	if raw := scan(t, home, "db.sqlite"); !bytes.Contains(raw, []byte(canaryPrompt)) ||
		!bytes.Contains(raw, []byte(canaryMarkdown)) {
		t.Fatal("the control does not see the words in the database")
	}

	if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
		t.Fatal(err)
	}

	raw := scan(t, home, "db.sqlite", "db.sqlite-wal")
	for _, w := range []string{canaryPrompt, canaryAnswer, canaryExcerpt, canaryMarkdown, canaryHTML} {
		if bytes.Contains(raw, []byte(w)) {
			t.Errorf("%q is in clear in the database", w)
		}
	}

	// While locked a job row still reads, for the recovery at start-up, and holds none of the words.
	locked, err := jobs.Get(ctx, "job_words")
	if err != nil {
		t.Fatalf("a sealed job while locked: %v", err)
	}

	if !locked.InputLocked || len(locked.Input) != 0 || len(locked.Result) != 0 {
		t.Fatalf("a sealed job while locked holds its words: %+v", locked)
	}

	if row, err := jobs.Get(ctx, "job_clear"); err != nil || row.InputLocked || !bytes.Equal(row.Input, options) {
		t.Fatalf("an export keeps its input in clear: %+v %v", row, err)
	}

	if err := o.Runner.Recover(ctx); err != nil {
		t.Fatalf("the recovery of the jobs while locked: %v", err)
	}

	if err := o.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	open, err := jobs.Get(ctx, "job_words")
	if err != nil || open.InputLocked || !bytes.Equal(open.Input, input) || !bytes.Equal(open.Result, result) {
		t.Fatalf("the job after the sign-in: %+v %v", open, err)
	}

	req, err := o.Store.Approvals().GetRequest(ctx, "apr_words")
	if err != nil || !strings.Contains(req.Summary, canaryExcerpt) {
		t.Fatalf("the approval after the sign-in: %+v %v", req, err)
	}

	post, err := o.Store.Blog().Published(ctx, "the-post")
	if err != nil || post.Markdown != canaryMarkdown || post.HTML != canaryHTML {
		t.Fatalf("the post after the sign-in: %+v %v", post, err)
	}

	// A job whose input is sealed cannot run while locked: it fails with E_LOCKED and is not left queued.
	if err := jobs.CreateLimited(ctx, service.JobRow{
		ID: "job_later", Kind: service.JobTextDraft, ProjectID: &project, Actor: "local:ui", Input: input,
		CreatedMs: 4, UpdatedMs: 4,
	}, nil); err != nil {
		t.Fatal(err)
	}

	o.Account.Lock()

	if err := o.Runner.RunJob(ctx, "job_later"); err != nil {
		t.Fatalf("RunJob while locked: %v", err)
	}

	failed, err := jobs.Get(ctx, "job_later")
	if err != nil || failed.State != service.JobFailed || !strings.Contains(string(failed.Error), service.CodeLocked) {
		t.Fatalf("the job while locked: state %s error %s (%v)", failed.State, failed.Error, err)
	}
}
