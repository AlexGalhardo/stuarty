package servicetest

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/service"
)

func briefingAnswers() map[string]string {
	return map[string]string{
		"audience": "indie developers", "goal": "sign up for the newsletter",
		"platform": "youtube-shorts", "format": "vertical-9x16", "duration": "30",
		"references": "# not a heading\nsecond line",
	}
}

// TestBriefingAnswersBecomeTheBrief: the answers are stored as the brief of the project (RF-301, RF-302), the
// questions read them back, and an agent stores the same structure.
func TestBriefingAnswersBecomeTheBrief(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := e.newProject("Launch")
	p := e.prod()
	ctx := e.as(agent)

	q, err := e.svc.BriefingQuestions(ctx, service.BriefingQuestionsRequest{ProjectID: id})
	if err != nil || len(q.Questions) != 10 || q.HasBrief || q.Guided || q.Questions[0].ID != "audience" {
		t.Fatalf("questions of a project with no brief: %+v %v", q, err)
	}

	res, err := e.svc.BriefingAnswer(ctx, service.BriefingAnswerRequest{ProjectID: id, Answers: briefingAnswers()})
	if err != nil || res.ItemID == "" || !strings.HasPrefix(res.Text, "# Brief\n\n## Audience\n\nindie developers") {
		t.Fatalf("answer: %+v %v", res, err)
	}

	got, err := p.BriefGet(ctx, service.BriefGetRequest{ProjectID: id})
	if err != nil || got.Text != res.Text || !e.get(id, "", "").Production.HasBrief {
		t.Fatalf("the brief is not the answers: %+v %v", got, err)
	}

	q, err = e.svc.BriefingQuestions(e.as(ui), service.BriefingQuestionsRequest{ProjectID: id})
	if err != nil || !q.HasBrief || !q.Guided || len(q.Answers) != 6 || q.Answers[0].Value != "indie developers" ||
		q.Answers[5].ID != "references" || q.Answers[5].Value != "# not a heading\nsecond line" {
		t.Fatalf("questions of an answered project: %+v %v", q, err)
	}

	// Answering again replaces what the briefing wrote.
	again := briefingAnswers()
	again["tone"] = "warm"

	if _, err := e.svc.BriefingAnswer(ctx, service.BriefingAnswerRequest{ProjectID: id, Answers: again}); err != nil {
		t.Fatal(err)
	}

	if proj := e.get(id, "", ""); len(proj.Items) != 2 {
		t.Fatalf("a second answer made another item: %+v", proj.Items)
	}
}

func TestBriefingRefusesWhatIsNotAnAnswer(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := e.newProject("Launch")
	ctx := e.as(agent)

	bad := briefingAnswers()
	bad["duration"] = "ten"

	if _, err := e.svc.BriefingAnswer(
		ctx,
		service.BriefingAnswerRequest{ProjectID: id, Answers: bad},
	); code(
		err,
	) != "E_VALIDATION" {
		t.Fatalf("a bad duration: %v", err)
	}

	if got, _ := e.prod().BriefGet(ctx, service.BriefGetRequest{ProjectID: id}); got.Text != "" {
		t.Fatalf("a refused answer wrote the brief: %q", got.Text)
	}

	// A dry run writes nothing.
	if _, err := e.svc.BriefingAnswer(
		ctx, service.BriefingAnswerRequest{ProjectID: id, Answers: briefingAnswers(), DryRun: true},
	); err != nil {
		t.Fatal(err)
	}

	if got, _ := e.prod().BriefGet(ctx, service.BriefGetRequest{ProjectID: id}); got.Text != "" {
		t.Fatalf("a dry run wrote the brief: %q", got.Text)
	}

	// A project that does not exist.
	if _, err := e.svc.BriefingAnswer(
		ctx, service.BriefingAnswerRequest{ProjectID: "prj_0000000009", Answers: briefingAnswers()},
	); code(err) != "E_NOT_FOUND" {
		t.Fatalf("an unknown project: %v", err)
	}
}

// TestBriefingNeverDestroysABriefWrittenByHand: a brief a person wrote is replaced only with replace.
func TestBriefingNeverDestroysABriefWrittenByHand(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := e.newProject("Launch")
	ctx := e.as(ui)

	if _, err := e.prod().BriefSet(ctx, service.BriefSetRequest{ProjectID: id, Text: "My own plan."}); err != nil {
		t.Fatal(err)
	}

	req := service.BriefingAnswerRequest{ProjectID: id, Answers: briefingAnswers()}
	if _, err := e.svc.BriefingAnswer(ctx, req); code(err) != "E_INVALID_STATE" {
		t.Fatalf("a brief written by hand was replaced: %v", err)
	}

	q, err := e.svc.BriefingQuestions(ctx, service.BriefingQuestionsRequest{ProjectID: id})
	if err != nil || !q.HasBrief || q.Guided || len(q.Answers) != 0 || !strings.Contains(q.Text, "My own plan.") {
		t.Fatalf("%+v %v", q, err)
	}

	if got, _ := e.prod().BriefGet(ctx, service.BriefGetRequest{ProjectID: id}); got.Text != "My own plan." {
		t.Fatalf("the brief changed: %q", got.Text)
	}

	req.Replace = true
	if _, err := e.svc.BriefingAnswer(ctx, req); err != nil {
		t.Fatal(err)
	}

	if got, _ := e.prod().
		BriefGet(ctx, service.BriefGetRequest{ProjectID: id}); !strings.HasPrefix(
		got.Text,
		project.BriefTitle,
	) {
		t.Fatalf("replace did not write the briefing: %q", got.Text)
	}
}

func TestBriefingScopes(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := e.newProject("Launch")

	read := service.WithCaller(context.Background(), service.Caller{Actor: agent, Scopes: []string{service.ScopeRead}})
	if _, err := e.svc.BriefingQuestions(read, service.BriefingQuestionsRequest{ProjectID: id}); err != nil {
		t.Fatalf("a read key asks the questions: %v", err)
	}

	if _, err := e.svc.BriefingAnswer(
		read,
		service.BriefingAnswerRequest{ProjectID: id, Answers: briefingAnswers()},
	); code(
		err,
	) != "E_FORBIDDEN" {
		t.Fatalf("a read key answered: %v", err)
	}

	// The edit scope is enough to answer (SDD-003 section 2.3): the check of the brief there is reads nothing
	// through a read operation.
	edit := service.WithCaller(context.Background(), service.Caller{Actor: agent, Scopes: []string{service.ScopeEdit}})
	if _, err := e.svc.BriefingAnswer(
		edit, service.BriefingAnswerRequest{ProjectID: id, Answers: briefingAnswers()},
	); err != nil {
		t.Fatalf("an edit key could not answer: %v", err)
	}
}

// A brief a person wrote with the headings of the briefing is still theirs: answering does not replace it.
func TestBriefingKeepsAHandWrittenBriefWithTheSameHeadings(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := e.newProject("Launch")
	ctx := e.as(ui)
	mine := "# Brief\n\n## Goal\n\n" + strings.Repeat("A long plan I wrote myself. ", 40) + "\n"

	if _, err := e.prod().BriefSet(ctx, service.BriefSetRequest{ProjectID: id, Text: mine}); err != nil {
		t.Fatal(err)
	}

	q, err := e.svc.BriefingQuestions(ctx, service.BriefingQuestionsRequest{ProjectID: id})
	if err != nil || !q.HasBrief || q.Guided || len(q.Answers) != 0 {
		t.Fatalf("a brief written by hand reads as guided: %+v %v", q, err)
	}

	req := service.BriefingAnswerRequest{ProjectID: id, Answers: briefingAnswers()}
	if _, err := e.svc.BriefingAnswer(e.as(agent), req); code(err) != "E_INVALID_STATE" {
		t.Fatalf("a brief written by hand was replaced: %v", err)
	}

	if got, _ := e.prod().BriefGet(ctx, service.BriefGetRequest{ProjectID: id}); got.Text != mine {
		t.Fatalf("the brief changed: %q", got.Text)
	}
}

// VV-289: the briefing asks and validates the questions of the kind of production the caller names (SDD-018
// section 17); with no kind it is the short video, and an unknown kind is E_VALIDATION on the path kind.
func TestBriefingServesTheKindOfProduction(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	id := e.newProject("Ad")
	ctx := e.as(agent)

	q, err := e.svc.BriefingQuestions(ctx, service.BriefingQuestionsRequest{ProjectID: id, Kind: "commercial"})
	if err != nil || len(q.Questions) == 0 || q.Questions[0].ID != "product" {
		t.Fatalf("commercial questions: %+v %v", q, err)
	}

	if _, err := e.svc.BriefingAnswer(
		ctx,
		service.BriefingAnswerRequest{ProjectID: id, Answers: briefingAnswers()},
	); err != nil {
		t.Fatalf("the default kind is short: %v", err)
	}

	ad := map[string]string{
		"product": "a pen", "audience": "writers", "problem": "smudges", "offer": "10% off", "call_to_action": "buy",
		"platform": "tiktok", "format": "vertical-9x16", "duration": "30",
	}

	if _, err := e.svc.BriefingAnswer(ctx, service.BriefingAnswerRequest{ProjectID: id, Answers: ad}); err == nil {
		t.Fatal("commercial answers were taken for a short")
	}

	res, err := e.svc.BriefingAnswer(ctx, service.BriefingAnswerRequest{ProjectID: id, Kind: "commercial", Answers: ad})
	if err != nil || !strings.Contains(res.Text, "## Product\n\na pen") {
		t.Fatalf("commercial answer: %+v %v", res, err)
	}

	q, err = e.svc.BriefingQuestions(ctx, service.BriefingQuestionsRequest{ProjectID: id, Kind: "commercial"})
	if err != nil || !q.Guided || len(q.Answers) != len(ad) {
		t.Fatalf("the commercial brief is not read back: %+v %v", q, err)
	}

	for _, bad := range []service.BriefingQuestionsRequest{{Kind: "podcast"}} {
		if _, err := e.svc.BriefingQuestions(ctx, bad); err == nil {
			t.Error("an unknown kind was served")
		}
	}

	if _, err := e.svc.BriefingAnswer(
		ctx,
		service.BriefingAnswerRequest{ProjectID: id, Kind: "podcast", Answers: ad},
	); err == nil {
		t.Error("an unknown kind was answered")
	}
}
