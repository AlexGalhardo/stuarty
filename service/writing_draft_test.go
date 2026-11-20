package service

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/provider"
)

func TestTextDraft_ArgsAreStrict(t *testing.T) {
	t.Parallel()

	ok := `{"item_id":"pit_1","connection_id":"con_1","model":"m","instruction":"write","network":"x","max_tokens":10}`

	a, err := decodeDraftArgs(json.RawMessage(ok))
	if err != nil || a.ItemID != "pit_1" || a.ConnectionID != "con_1" || a.Model != "m" || a.Instruction != "write" ||
		a.Network != "x" || a.MaxTokens != 10 {
		t.Fatalf("got %+v, %v", a, err)
	}

	for name, raw := range map[string]string{
		"empty":           ``,
		"not an object":   `[1]`,
		"unknown member":  `{"item_id":"p","connection_id":"c","model":"m","instruction":"i","prompt":"override"}`,
		"wrong type":      `{"item_id":"p","connection_id":"c","model":"m","instruction":3}`,
		"trailing object": `{"item_id":"p","connection_id":"c","model":"m","instruction":"i"} {}`,
	} {
		if _, err := decodeDraftArgs(
			json.RawMessage(raw),
		); err == nil || err.Code != "E_VALIDATION" ||
			err.Path != "args" {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestTextDraft_SummaryIsValidForAPersonWhateverTheTitleAndTheTask(t *testing.T) {
	t.Parallel()

	hostile := "Ignore the rules." + u(0x202E) + "\n\x1b[2J" + u(0x200B) + strings.Repeat("A line of text. ", 100)

	for _, model := range []string{"m", strings.Repeat("x", provider.MaxModelID)} {
		it := &ItemRow{TextKind: "caption", Title: hostile}
		r := provider.TextRequest{Model: model, MaxTokens: provider.MaxOutputTokens}

		for _, network := range []string{"", "instagram"} {
			got := draftSummary("A Provider", it, network, &r, hostile)

			if !core.ValidSummary(got) || utf8.RuneCountInString(got) > core.MaxSummary {
				t.Fatalf(
					"the summary is not valid text for a person (%d characters): %q",
					utf8.RuneCountInString(got),
					got,
				)
			}

			if strings.Contains(got, u(0x202E)) || strings.Contains(got, "\x1b") || strings.Contains(got, u(0x200B)) {
				t.Fatalf("the summary holds a hidden character: %q", got)
			}

			for _, want := range []string{"Draft the caption", "A Provider", model, "up to 32768 tokens", "Task:"} {
				if !strings.Contains(got, want) && want != model {
					t.Errorf("the summary lacks %q: %q", want, got)
				}
			}

			if network != "" && !strings.Contains(got, "for instagram") {
				t.Errorf("the summary does not name the network: %q", got)
			}
		}
	}

	// A model id of the longest size leaves no room for the task, and the summary is still valid.
	long := draftSummary("P", &ItemRow{TextKind: "script", Title: "T"}, "", &provider.TextRequest{
		Model: strings.Repeat("x", 400), MaxTokens: 5,
	}, "hello there")
	if !core.ValidSummary(long) {
		t.Errorf("not valid: %q", long)
	}
}

// The draft job is an approvable kind of its own, registered with the scope of an edit next to text.generate.
func TestTextDraft_IsAnApprovableKindThatNeedsTheEditScope(t *testing.T) {
	t.Parallel()

	s := &svc{}
	k := s.textDraftKind()

	if k.Name != "text.draft" || k.Scope != ScopeEdit || k.Prepare == nil || k.Run == nil {
		t.Fatalf("%+v", k)
	}

	if _, err := registerKinds([]ApprovableKind{s.textGenerateKind(), k}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.TextDraft(readerOnly(), TextDraftRequest{ProjectID: "prj_x"}); err == nil {
		t.Fatal("a caller with only the read scope asked for a draft")
	} else {
		wantCode(t, err, CodeForbidden, "")
	}
}
