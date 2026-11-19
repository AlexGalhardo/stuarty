package provider

import (
	"math"
	"strings"
	"testing"
)

func TestNormalizeText(t *testing.T) {
	t.Parallel()

	got := NormalizeText(TextRequest{Model: "  m1 ", Prompt: "hi"})
	if got.Model != "m1" || got.MaxTokens != DefaultMaxTokens {
		t.Fatalf("got %+v", got)
	}

	if got := NormalizeText(TextRequest{MaxTokens: 5}); got.MaxTokens != 5 {
		t.Fatalf("an explicit max_tokens must stay, got %d", got.MaxTokens)
	}
}

func TestValidateText(t *testing.T) {
	t.Parallel()

	ok := TextRequest{Model: "gpt-x.1", Prompt: "hello", MaxTokens: 100}

	cases := []struct {
		name   string
		mutate func(r *TextRequest)
		limits Limits
		path   string
	}{
		{"valid", func(*TextRequest) {}, Limits{}, ""},
		{"valid with limits", func(*TextRequest) {}, Limits{ContextTokens: 1000, OutputTokens: 200}, ""},
		{"empty model", func(r *TextRequest) { r.Model = "" }, Limits{}, "model"},
		{"model with a space", func(r *TextRequest) { r.Model = "a b" }, Limits{}, "model"},
		{"model with a newline", func(r *TextRequest) { r.Model = "a\nb" }, Limits{}, "model"},
		{"model starts with a dash", func(r *TextRequest) { r.Model = "-a" }, Limits{}, "model"},
		{"model too long", func(r *TextRequest) { r.Model = strings.Repeat("a", 201) }, Limits{}, "model"},
		{"model of 200", func(r *TextRequest) { r.Model = strings.Repeat("a", 200) }, Limits{}, ""},
		{"empty prompt", func(r *TextRequest) { r.Prompt = "" }, Limits{}, "prompt"},
		{"prompt at the limit", func(r *TextRequest) { r.Prompt = strings.Repeat("é", MaxPromptChars) }, Limits{}, ""},
		{
			"prompt over the limit",
			func(r *TextRequest) { r.Prompt = strings.Repeat("é", MaxPromptChars+1) },
			Limits{},
			"prompt",
		},
		{"prompt with a NUL", func(r *TextRequest) { r.Prompt = "a\x00b" }, Limits{}, "prompt"},
		{"prompt not utf-8", func(r *TextRequest) { r.Prompt = "a\xffb" }, Limits{}, "prompt"},
		{
			"system over the limit",
			func(r *TextRequest) { r.System = strings.Repeat("a", MaxSystemChars+1) },
			Limits{},
			"system",
		},
		{"system with a NUL", func(r *TextRequest) { r.System = "\x00" }, Limits{}, "system"},
		{"max_tokens zero", func(r *TextRequest) { r.MaxTokens = 0 }, Limits{}, "max_tokens"},
		{"max_tokens negative", func(r *TextRequest) { r.MaxTokens = -1 }, Limits{}, "max_tokens"},
		{"max_tokens at the limit", func(r *TextRequest) { r.MaxTokens = MaxOutputTokens }, Limits{}, ""},
		{
			"max_tokens over the limit",
			func(r *TextRequest) { r.MaxTokens = MaxOutputTokens + 1 },
			Limits{},
			"max_tokens",
		},
		{"max_tokens over the model", func(*TextRequest) {}, Limits{OutputTokens: 99}, "max_tokens"},
		{"max_tokens at the model", func(*TextRequest) {}, Limits{OutputTokens: 100}, ""},
		{"temperature 0", func(r *TextRequest) { r.Temperature = new(0.0) }, Limits{}, ""},
		{"temperature 2", func(r *TextRequest) { r.Temperature = new(2.0) }, Limits{}, ""},
		{"temperature over", func(r *TextRequest) { r.Temperature = new(2.01) }, Limits{}, "temperature"},
		{"temperature negative", func(r *TextRequest) { r.Temperature = new(-0.1) }, Limits{}, "temperature"},
		{"temperature NaN", func(r *TextRequest) { r.Temperature = new(math.NaN()) }, Limits{}, "temperature"},
		{"context too small", func(*TextRequest) {}, Limits{ContextTokens: 101}, "prompt"},
		{"context just fits", func(*TextRequest) {}, Limits{ContextTokens: 102}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := ok
			tc.mutate(&r)

			err := ValidateText(r, tc.limits, 0)

			switch {
			case tc.path == "" && err != nil:
				t.Fatalf("want valid, got %v", err)
			case tc.path != "" && (err == nil || err.Path != tc.path):
				t.Fatalf("want an error on %q, got %v", tc.path, err)
			}
		})
	}
}

func TestValidateTextCountsTheFraming(t *testing.T) {
	t.Parallel()

	r := TextRequest{Model: "m", Prompt: "abc", MaxTokens: 100}
	// "abc" is 1 input token; with 10 framing tokens the request needs 111.
	if err := ValidateText(r, Limits{ContextTokens: 111}, 10); err != nil {
		t.Fatalf("want valid, got %v", err)
	}

	if err := ValidateText(r, Limits{ContextTokens: 110}, 10); err == nil {
		t.Fatal("want the framing to count against the context")
	}
}

func TestEstimateInput(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		system, prompt string
		want           int
	}{
		{"", "", 0},
		{"", "a", 1},
		{"", "abc", 1},
		{"", "abcd", 2},
		{"ab", "cd", 2},
		{"", strings.Repeat("あ", 3), 3}, // nine bytes
	} {
		if got := EstimateInput(tc.system, tc.prompt); got != tc.want {
			t.Errorf("EstimateInput(%q, %q) = %d, want %d", tc.system, tc.prompt, got, tc.want)
		}
	}
}

func TestNormalizePrice(t *testing.T) {
	t.Parallel()

	got, err := NormalizePrice(Price{Currency: " usd ", InputMicrosPerMTok: 1, OutputMicrosPerMTok: 2})
	if err != nil || got.Currency != "USD" {
		t.Fatalf("got %+v, %v", got, err)
	}

	for name, p := range map[string]Price{
		"empty currency":  {InputMicrosPerMTok: 1},
		"long currency":   {Currency: "USDT"},
		"digit currency":  {Currency: "U5D"},
		"negative input":  {Currency: "USD", InputMicrosPerMTok: -1},
		"negative output": {Currency: "USD", OutputMicrosPerMTok: -1},
		"huge input":      {Currency: "USD", InputMicrosPerMTok: MaxPriceMicros + 1},
		"huge output":     {Currency: "USD", OutputMicrosPerMTok: MaxPriceMicros + 1},
	} {
		if _, err := NormalizePrice(p); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}

	if _, err := NormalizePrice(Price{Currency: "USD", InputMicrosPerMTok: MaxPriceMicros}); err != nil {
		t.Errorf("the largest price must be valid: %v", err)
	}
}

func TestTextCost(t *testing.T) {
	t.Parallel()

	usd := func(in, out int64) Price {
		return Price{Currency: "USD", InputMicrosPerMTok: in, OutputMicrosPerMTok: out}
	}

	for _, tc := range []struct {
		name  string
		usage Usage
		price Price
		want  int64
	}{
		{"nothing", Usage{}, usd(3_000_000, 15_000_000), 0},
		{"free", Usage{InputTokens: 10, OutputTokens: 10}, usd(0, 0), 0},
		{"one token rounds up", Usage{InputTokens: 1}, usd(1, 0), 1},
		{"a million tokens", Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}, usd(3_000_000, 15_000_000), 18_000_000},
		{"each side rounds up", Usage{InputTokens: 1, OutputTokens: 1}, usd(1, 1), 2},
		{"exact", Usage{InputTokens: 500_000}, usd(2_000_000, 0), 1_000_000},
		{"negative tokens count as none", Usage{InputTokens: -5, OutputTokens: -5}, usd(1, 1), 0},
		{"a typical request", Usage{InputTokens: 1200, OutputTokens: 800}, usd(2_500_000, 10_000_000), 3_000 + 8_000},
	} {
		got := TextCost(tc.usage, tc.price)
		if got.Micros != tc.want || got.Currency != "USD" {
			t.Errorf("%s: got %+v, want %d USD micros", tc.name, got, tc.want)
		}
	}
}

func TestTextCostNeverOverflowsIntoASmallNumber(t *testing.T) {
	t.Parallel()

	p := Price{Currency: "USD", InputMicrosPerMTok: MaxPriceMicros, OutputMicrosPerMTok: MaxPriceMicros}

	got := TextCost(Usage{InputTokens: math.MaxInt32, OutputTokens: math.MaxInt32}, p)
	if got.Micros <= 0 {
		t.Fatalf("an enormous count must be an enormous cost, got %d", got.Micros)
	}

	if got := TextCost(Usage{InputTokens: math.MaxInt}, p); got.Micros <= 0 {
		t.Fatalf("got %d", got.Micros)
	}

	// 2^62 tokens at 10^12 per million does not fit 64 bits after the division.
	if got := mulDivCeil(1<<62, MaxPriceMicros); got != math.MaxInt64 {
		t.Fatalf("a result that does not fit saturates, got %d", got)
	}

	if got := TextCost(
		Usage{InputTokens: math.MaxInt},
		Price{Currency: "USD", InputMicrosPerMTok: 1, OutputMicrosPerMTok: 1},
	); got.Micros <= 0 {
		t.Fatalf("got %d", got.Micros)
	}
}

func TestEstimateText(t *testing.T) {
	t.Parallel()

	r := TextRequest{Model: "m", System: "sss", Prompt: "pppppp", MaxTokens: 1000}
	p := Price{Currency: "EUR", InputMicrosPerMTok: 1_000_000, OutputMicrosPerMTok: 2_000_000}

	// 9 bytes are 3 input tokens, plus 7 of framing: 10 in and 1000 out.
	got := EstimateText(r, p, 7)
	if want := int64(10 + 2000); got.Micros != want || got.Currency != "EUR" {
		t.Fatalf("got %+v, want %d EUR micros", got, want)
	}

	if tk := EstimateTokens(r, 7); tk.Input != 10 || tk.Output != 1000 {
		t.Fatalf("got %+v", tk)
	}

	// The estimate is an upper bound of a request that ends early.
	if spent := TextCost(Usage{InputTokens: 3, OutputTokens: 10}, p); spent.Micros > got.Micros {
		t.Fatalf("the estimate %d must bound the cost %d", got.Micros, spent.Micros)
	}
}

func TestNormalizeEntry(t *testing.T) {
	t.Parallel()

	good := PriceEntry{Provider: " openai ", Model: "m-1", Currency: "usd", InputMicrosPerMTok: 5}

	got, err := NormalizeEntry(good)
	if err != nil || got.Provider != "openai" || got.Currency != "USD" {
		t.Fatalf("got %+v, %v", got, err)
	}

	for name, e := range map[string]PriceEntry{
		"no provider":  {Model: "m", Price: Price{Currency: "USD"}},
		"upper case":   {Provider: "OpenAI", Model: "m", Price: Price{Currency: "USD"}},
		"bad model":    {Provider: "openai", Model: "a b", Price: Price{Currency: "USD"}},
		"bad currency": {Provider: "openai", Model: "m", Price: Price{Currency: "X"}},
	} {
		if _, err := NormalizeEntry(e); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestResolvePrice(t *testing.T) {
	t.Parallel()

	user := &Price{Currency: "USD", InputMicrosPerMTok: 1}
	cat := &Price{Currency: "USD", InputMicrosPerMTok: 2}

	for _, tc := range []struct {
		name      string
		user, cat *Price
		want      string
		micros    int64
	}{
		{"the table wins", user, cat, SourceUser, 1},
		{"the catalogue", nil, cat, SourceCatalogue, 2},
		{"none", nil, nil, SourceNone, 0},
	} {
		p, src := ResolvePrice(tc.user, tc.cat)
		if src != tc.want || p.InputMicrosPerMTok != tc.micros {
			t.Errorf("%s: got %+v from %s", tc.name, p, src)
		}
	}
}

func TestValidProviderID(t *testing.T) {
	t.Parallel()

	for s, want := range map[string]bool{
		"openai": true, "anthropic": true, "open-router": true, "a": false, "": false,
		"OpenAI": false, "1abc": false, "a_b": false, strings.Repeat("a", 33): false, strings.Repeat("a", 32): true,
	} {
		if got := ValidProviderID(s); got != want {
			t.Errorf("ValidProviderID(%q) = %v, want %v", s, got, want)
		}
	}
}
