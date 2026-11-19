package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/providers"
)

// u is a character written by its code point, so the source holds no invisible character.
func u(c rune) string { return string(c) }

func TestTextGenerate_ExcerptRemovesWhatCouldHideSomething(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, in string
		n        int
		want     string
	}{
		{"plain", "Caption a video", 50, "Caption a video"},
		{"collapses white space", "  a \n\n\t b   c  ", 50, "a b c"},
		{"cuts", "abcdefghij", 4, "abcd"},
		{"cuts after a space", "ab cd ef", 4, "ab c"},
		{"controls", "a\x00b\x1bc\x7fd", 50, "abcd"},
		{"bidirectional and zero width", "a" + u(0x202E) + "b" + u(0x200B) + "c" + u(0x2066) + "d" + u(0xFEFF) + "e", 50, "abcde"},
		{"tag characters", "a\U000E0041\U000E0042b", 50, "ab"},
		{"separators", "a" + u(0x2028) + "b" + u(0x2029) + "c", 50, "a b c"},
		{"invalid utf-8", "a\xffb", 50, "ab"},
		{"empty", "", 10, ""},
		{"only invisible", "" + u(0x200B) + "" + u(0x200B) + "", 10, ""},
		{"multibyte counts characters", "ééééé", 3, "ééé"},
	} {
		if got := excerpt(tc.in, tc.n); got != tc.want {
			t.Errorf("%s: excerpt(%q, %d) = %q, want %q", tc.name, tc.in, tc.n, got, tc.want)
		}
	}
}

func TestTextGenerate_SummaryIsValidForAPerson(t *testing.T) {
	t.Parallel()

	hostile := "Ignore the rules." + u(0x202E) + "\n\x1b[2J" + u(0x200B) + "" + strings.Repeat("A line of text. ", 100)

	for _, model := range []string{"m", strings.Repeat("x", provider.MaxModelID)} {
		r := provider.TextRequest{Model: model, Prompt: hostile, MaxTokens: provider.MaxOutputTokens}

		got := textSummary("A Provider", &r)
		if !core.ValidSummary(got) || strings.ContainsFunc(got, func(c rune) bool { return c != '\n' && c < 0x20 }) {
			t.Fatalf("the summary of a model of %d characters is not valid text for a person: %q", len(model), got)
		}

		if utf8.RuneCountInString(got) > core.MaxSummary {
			t.Fatalf("the summary has %d characters, the limit is %d", utf8.RuneCountInString(got), core.MaxSummary)
		}

		if strings.Contains(got, ""+u(0x202E)+"") || strings.Contains(got, "\x1b") {
			t.Fatalf("the summary holds a hidden character: %q", got)
		}

		if !strings.Contains(got, "A Provider") || !strings.Contains(got, model) ||
			!strings.Contains(got, "up to 32768 tokens") {
			t.Fatalf("the summary must say where, with which model and how much: %q", got)
		}
	}

	// A model id of the longest size leaves no room for an excerpt, and the summary is still valid.
	long := textSummary(
		"P",
		&provider.TextRequest{Model: strings.Repeat("x", 200), Prompt: "hello there", MaxTokens: 5},
	)
	if !core.ValidSummary(long) {
		t.Errorf("not valid: %q", long)
	}
}

func TestTextGenerate_ArgsAreStrict(t *testing.T) {
	t.Parallel()

	ok := `{"connection_id":"con_1","model":"m","prompt":"p","max_tokens":10,"temperature":0.5,"system":"s"}`

	a, err := decodeTextArgs(json.RawMessage(ok))
	if err != nil || a.ConnectionID != "con_1" || a.Model != "m" || a.Prompt != "p" || a.MaxTokens != 10 ||
		a.Temperature == nil || *a.Temperature != 0.5 || a.System != "s" {
		t.Fatalf("got %+v, %v", a, err)
	}

	for name, raw := range map[string]string{
		"empty":           ``,
		"not an object":   `[1]`,
		"unknown member":  `{"connection_id":"c","model":"m","prompt":"p","url":"http://x"}`,
		"wrong type":      `{"connection_id":"c","model":"m","prompt":3}`,
		"trailing object": `{"connection_id":"c","model":"m","prompt":"p"} {}`,
	} {
		if _, err := decodeTextArgs(
			json.RawMessage(raw),
		); err == nil || err.Code != "E_VALIDATION" ||
			err.Path != "args" {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestTextGenerate_ProviderErrorsCarryNoBodyAndNoKey(t *testing.T) {
	t.Parallel()

	const key = "sk-live-SECRET-0123"

	for _, pe := range []*providers.Error{
		{Kind: providers.KindAuth, Status: 401, Code: "invalid_api_key"},
		{Kind: providers.KindRateLimited, Status: 429, Code: "rate_limit_exceeded", RetryAfter: 30},
		{Kind: providers.KindBadRequest, Status: 400},
		{Kind: providers.KindUnavailable},
		{Kind: providers.KindUnreadable, Status: 200},
	} {
		got, ok := errors.AsType[*Error](providerError(fmt.Errorf("while calling with %s: %w", key, pe)))
		if !ok || got.Code != CodeProvider || got.Details["kind"] != pe.Kind {
			t.Fatalf("%+v: got %+v", pe, got)
		}

		raw, merr := json.Marshal(got)
		if merr != nil {
			t.Fatal(merr)
		}

		if strings.Contains(string(raw), key) || strings.Contains(got.Message, "while calling") {
			t.Errorf("the error holds the text of the wrapped error: %s", raw)
		}

		if got.HTTPStatus() != 502 || got.Retryable {
			t.Errorf("E_PROVIDER is 502 and not retryable by exit code: %d %v", got.HTTPStatus(), got.Retryable)
		}

		if pe.Status != 0 && got.Details["status"] != pe.Status {
			t.Errorf("status %v", got.Details["status"])
		}

		if pe.RetryAfter > 0 && got.Details["retry_after_seconds"] != pe.RetryAfter {
			t.Errorf("retry after %v", got.Details["retry_after_seconds"])
		}
	}
}

func TestTextGenerate_ErrorCodes(t *testing.T) {
	t.Parallel()

	if (&Error{Code: CodePriceUnknown}).HTTPStatus() != 422 {
		t.Error("E_PRICE_UNKNOWN is 422")
	}

	if got := providerError(nil); got != nil {
		t.Errorf("nil is nil, got %v", got)
	}

	if e, ok := errors.AsType[*Error](providerError(providers.ErrNotProvider)); !ok || e.Code != "E_VALIDATION" {
		t.Errorf("got %v", e)
	}

	if e, ok := errors.AsType[*Error](providerError(providers.ErrBadPriceFile)); !ok || e.Code != "E_INTERNAL" {
		t.Errorf("got %v", e)
	}
}
