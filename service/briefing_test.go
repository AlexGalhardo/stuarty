package service

import (
	"context"
	"testing"
)

// timelineMapOnly is a store whose only answer is that an id is the timeline of no project: every other call
// panics on the nil interfaces it embeds.
type timelineMapOnly struct{ Store }

func (timelineMapOnly) Productions() ProductionRepo { return noTimelines{} }

type noTimelines struct{ ProductionRepo }

func (noTimelines) ProjectOfTimeline(context.Context, string) (string, error) { return "", ErrNotFound }

// TestBriefingRefusesBeforeTouchingTheStore: the scope and the answers are checked before any read or write,
// so a store that panics on a call proves it was not reached. The one read allowed is the project of the
// timeline an id names, which a key limited to one project needs before it is refused (VV-250).
func TestBriefingRefusesBeforeTouchingTheStore(t *testing.T) {
	t.Parallel()

	s := &svc{store: timelineMapOnly{}}
	valid := map[string]string{
		"audience": "makers", "goal": "sell", "platform": "tiktok", "format": "vertical-9x16", "duration": "30",
	}

	read := WithCaller(context.Background(), Caller{Actor: "key:k", Scopes: []string{ScopeRead}})
	if _, err := s.BriefingAnswer(
		read,
		BriefingAnswerRequest{ProjectID: "prj_a", Answers: valid},
	); codeOf(
		err,
	) != CodeForbidden {
		t.Errorf("a read key answered: %v", err)
	}

	if _, err := s.BriefingQuestions(
		context.Background(),
		BriefingQuestionsRequest{},
	); codeOf(
		err,
	) != CodeUnauthorized {
		t.Errorf("no credentials: %v", err)
	}

	edit := WithCaller(context.Background(), Caller{Actor: "key:k", Scopes: []string{ScopeEdit}})
	if _, err := s.BriefingQuestions(edit, BriefingQuestionsRequest{}); codeOf(err) != CodeForbidden {
		t.Errorf("questions need the read scope: %v", err)
	}

	for name, answers := range map[string]map[string]string{
		"nothing":        {},
		"an unknown id":  {"colour": "red"},
		"a bad choice":   {"audience": "makers", "goal": "sell", "platform": "myspace", "format": "vertical-9x16", "duration": "30"},
		"a bad duration": {"audience": "makers", "goal": "sell", "platform": "tiktok", "format": "vertical-9x16", "duration": "0"},
	} {
		_, err := s.BriefingAnswer(edit, BriefingAnswerRequest{ProjectID: "prj_a", Answers: answers})
		if codeOf(err) != "E_VALIDATION" {
			t.Errorf("%s: %v", name, err)
		}
	}

	limited := WithCaller(context.Background(), Caller{Actor: "key:k", Scopes: AllScopes, ProjectID: "prj_other"})
	if _, err := s.BriefingAnswer(
		limited,
		BriefingAnswerRequest{ProjectID: "prj_a", Answers: valid},
	); codeOf(
		err,
	) != CodeForbidden {
		t.Errorf("a key of another project answered: %v", err)
	}
}
