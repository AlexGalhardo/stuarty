package service

import (
	"context"
	"testing"
)

// TestBriefingRefusesBeforeTouchingTheStore: the scope and the answers are checked before any read or write,
// so a store that panics on a call proves it was not reached.
func TestBriefingRefusesBeforeTouchingTheStore(t *testing.T) {
	t.Parallel()

	s := &svc{}
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
