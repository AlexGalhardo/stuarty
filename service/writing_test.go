package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/core/writing"
)

// The checks that need no store: a request that breaks a rule is refused before anything is read, and the two
// checks of SDD-019 sections 4 and 5.2 read the text of the request alone.

func writer() context.Context {
	return WithCaller(context.Background(), Caller{Actor: "local:cli", Scopes: AllScopes})
}

func readerOnly() context.Context {
	return WithCaller(context.Background(), Caller{Actor: "key:k", Scopes: []string{ScopeRead}})
}

func wantCode(t *testing.T, err error, code, path string) {
	t.Helper()

	e, ok := errors.AsType[*Error](err)
	if !ok || e == nil {
		t.Fatalf("want %s at %q, got %v", code, path, err)
	}

	if e.Code != code || e.Path != path {
		t.Fatalf("want %s at %q, got %s at %q: %s", code, path, e.Code, e.Path, e.Message)
	}
}

func TestTextWriteRefusesWhatBreaksARuleBeforeTouchingTheStore(t *testing.T) {
	t.Parallel()

	s := &svc{}
	big := strings.Repeat("a", project.MaxText+1)

	for name, tc := range map[string]struct {
		req        TextWriteRequest
		code, path string
	}{
		"a text over 256 KiB":      {TextWriteRequest{ProjectID: "prj_x", ItemID: "pit_x", Text: big}, "E_LIMIT_EXCEEDED", "text"},
		"a text that is not UTF-8": {TextWriteRequest{ProjectID: "prj_x", ItemID: "pit_x", Text: "\xff"}, "E_VALIDATION", "text"},
		"a new text with no title": {TextWriteRequest{ProjectID: "prj_x", Text: "a"}, "E_VALIDATION", "title"},
		"a new text of a kind that does not exist": {
			TextWriteRequest{ProjectID: "prj_x", Title: "t", TextKind: "poem"}, "E_VALIDATION", "text_kind",
		},
		"a rename to a title with a control character": {
			TextWriteRequest{ProjectID: "prj_x", ItemID: "pit_x", Title: "a\x07", Text: "a"}, "E_VALIDATION", "title",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := s.TextWrite(writer(), tc.req)
			wantCode(t, err, tc.code, tc.path)
		})
	}

	if _, err := s.TextWrite(readerOnly(), TextWriteRequest{ProjectID: "prj_x", Title: "t"}); err == nil {
		t.Fatal("a caller with only the read scope wrote a text")
	} else {
		wantCode(t, err, CodeForbidden, "")
	}
}

func TestTextRevertAndRevisionsRefuseABadRevisionNumber(t *testing.T) {
	t.Parallel()

	s := &svc{}

	_, err := s.TextRevert(writer(), TextRevertRequest{ProjectID: "prj_x", ItemID: "pit_x", Revision: 0})
	wantCode(t, err, "E_VALIDATION", "revision")

	for name, req := range map[string]TextRevisionsRequest{
		"a negative limit": {ProjectID: "prj_x", ItemID: "pit_x", Limit: -1},
		"a huge limit":     {ProjectID: "prj_x", ItemID: "pit_x", Limit: 101},
		"a negative from":  {ProjectID: "prj_x", ItemID: "pit_x", From: -1},
		"to without from":  {ProjectID: "prj_x", ItemID: "pit_x", To: 2},
	} {
		if _, err := s.TextRevisions(writer(), req); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestTextListRefusesABadFilter(t *testing.T) {
	t.Parallel()

	s := &svc{}

	_, err := s.TextList(writer(), TextListRequest{TextKind: "poem"})
	wantCode(t, err, "E_VALIDATION", "text_kind")

	_, err = s.TextList(writer(), TextListRequest{State: "lost"})
	wantCode(t, err, "E_VALIDATION", "state")

	_, err = s.TextList(writer(), TextListRequest{Limit: 101})
	wantCode(t, err, "E_VALIDATION", "limit")
}

// X counts after Unicode NFC (SDD-019 section 5.1): 280 decomposed e-acute are 280 characters on X, not 560.
func TestCaptionValidateCountsXAfterNFC(t *testing.T) {
	t.Parallel()

	s := &svc{}
	text := strings.Repeat("e\U00000301", 280)

	res, err := s.CaptionValidate(readerOnly(), CaptionValidateRequest{Text: text, Network: "x"})
	if err != nil {
		t.Fatal(err)
	}

	if !res.OK || res.Variants[0].Counts.Length != 280 {
		t.Fatalf("X after NFC: %+v", res.Variants)
	}
}

func TestCaptionValidateReportsTheExactLimitBrokenForEachNetwork(t *testing.T) {
	t.Parallel()

	s := &svc{}

	for _, n := range writing.Networks() {
		over := strings.Repeat("a", n.MaxLength+1)

		res, err := s.CaptionValidate(readerOnly(), CaptionValidateRequest{Text: over, Network: n.ID})
		if err != nil {
			t.Fatalf("%s: %v", n.ID, err)
		}

		if res.OK || len(res.Variants) != 1 || len(res.Variants[0].Problems) != 1 {
			t.Fatalf("%s: %+v", n.ID, res)
		}

		p := res.Variants[0].Problems[0]
		if p.Code != writing.ProblemTooLong || p.Limit != n.MaxLength || p.Actual != n.MaxLength+1 || p.Unit != n.Unit {
			t.Errorf("%s: the problem is %+v, want too_long at %d %s", n.ID, p, n.MaxLength, n.Unit)
		}

		if !strings.Contains(res.Text, "too_long") || !strings.Contains(res.Text, n.ID) {
			t.Errorf("%s: the text view does not say it: %q", n.ID, res.Text)
		}

		ok, err := s.CaptionValidate(readerOnly(), CaptionValidateRequest{Text: over[:n.MaxLength], Network: n.ID})
		if err != nil || !ok.OK {
			t.Errorf("%s: a text at the limit is refused: %+v, %v", n.ID, ok, err)
		}
	}
}

func TestCaptionValidateReadsAWholeCaptionBody(t *testing.T) {
	t.Parallel()

	s := &svc{}
	body := "Common #tag\n\n## instagram\n" + strings.Repeat("#t ", 31) + "\n\n## x\n" + strings.Repeat("a", 281)

	res, err := s.CaptionValidate(readerOnly(), CaptionValidateRequest{Body: body})
	if err != nil {
		t.Fatal(err)
	}

	got := map[string][]string{}

	for i := range res.Variants {
		v := &res.Variants[i]
		for _, p := range v.Problems {
			got[v.Network] = append(got[v.Network], p.Code)
		}
	}

	if res.OK || len(res.Variants) != 3 || res.Variants[0].Network != "" ||
		!slices.Equal(got["instagram"], []string{writing.ProblemTooManyTags}) ||
		!slices.Equal(got["x"], []string{writing.ProblemTooLong}) || len(got[""]) != 0 {
		t.Fatalf("variants: %+v; problems: %v", res.Variants, got)
	}
}

func TestCaptionValidateRefusesAmbiguousOrIncompleteRequests(t *testing.T) {
	t.Parallel()

	s := &svc{}

	for name, tc := range map[string]struct {
		req  CaptionValidateRequest
		path string
	}{
		"nothing given":                  {CaptionValidateRequest{}, "item_id"},
		"two things given":               {CaptionValidateRequest{Body: "a", Text: "b", Network: "x"}, "item_id"},
		"text without a network":         {CaptionValidateRequest{Text: "a"}, "network"},
		"a network that is not in table": {CaptionValidateRequest{Text: "a", Network: "myspace"}, "network"},
	} {
		_, err := s.CaptionValidate(readerOnly(), tc.req)
		if e, ok := errors.AsType[*Error](err); !ok || e.Code != "E_VALIDATION" || e.Path != tc.path {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestScriptEstimateChangesWithTheWordsPerMinute(t *testing.T) {
	t.Parallel()

	s := &svc{}
	text := "# Hook\nVO: " + strings.TrimSpace(strings.Repeat("word ", 150)) + "\n"

	at := func(wpm int) *ScriptEstimateResult {
		res, err := s.ScriptEstimate(readerOnly(), ScriptEstimateRequest{Text: text, WordsPerMinute: wpm})
		if err != nil {
			t.Fatal(err)
		}

		return res
	}

	slow, def, fast := at(100), at(0), at(300)

	if slow.DurationMillis != 90000 || def.DurationMillis != 60000 || fast.DurationMillis != 30000 {
		t.Fatalf("slow %d default %d fast %d", slow.DurationMillis, def.DurationMillis, fast.DurationMillis)
	}

	if def.WordsPerMinute != 150 || def.Words != 150 || !strings.Contains(def.Text, "60.000 s") {
		t.Fatalf("%+v", def)
	}

	for name, req := range map[string]ScriptEstimateRequest{
		"a pace below the range": {Text: text, WordsPerMinute: 59},
		"a pace above the range": {Text: text, WordsPerMinute: 301},
		"neither text nor item":  {},
		"both text and item":     {Text: text, ItemID: "pit_x"},
	} {
		if _, err := s.ScriptEstimate(readerOnly(), req); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
