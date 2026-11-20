// Package provider holds the pure rules of the AI provider layer (SDD-016): what a text request may
// ask for, how a price is written, and how the cost of a request is estimated before it runs and
// counted after it ends. It does no I/O, reads no clock and knows no provider; a price is data the
// caller supplies, never a constant here (ADR-0036 item 5).
package provider

import (
	"fmt"
	"math"
	"math/bits"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The capabilities a provider may offer (ADR-0036 item 1). Only text has adapters in epic 09.
const (
	CapabilityText  = "text"
	CapabilityImage = "image"
	CapabilityVoice = "voice"
	CapabilityVideo = "video"
)

// The credential kinds of a provider (ADR-0036 item 2): a key the person pastes, or a sign-in in the
// browser (epic 12, SDD-016 section 12) whose tokens the vault keeps.
const (
	CredentialAPIKey = "api_key"
	CredentialOAuth  = "oauth"
)

// Limits of SDD-016 sections 3 to 5.
const (
	MaxModelID       = 200
	MaxPromptChars   = 100_000
	MaxSystemChars   = 20_000
	MaxOutputTokens  = 32_768
	DefaultMaxTokens = 1024
	MaxTemperature   = 2.0
	MaxPrices        = 500
	// MaxPriceMicros is the largest price of a million tokens: a million units of the currency.
	MaxPriceMicros = 1_000_000 * 1_000_000
	// BytesPerToken is how many bytes of text the estimate counts as one input token. Three is
	// pessimistic for English (about four characters per token) and about right for CJK scripts.
	BytesPerToken = 3
	// microsPerMTok is the divisor of a price: it is stated for one million tokens.
	microsPerMTok = 1_000_000
)

// Where the price of a model came from (SDD-016 section 4.2).
const (
	SourceUser      = "user"
	SourceCatalogue = "catalogue"
	SourceNone      = "none"
)

// FieldError is a validation failure; Path names the member of the request.
type FieldError struct {
	Path    string
	Message string
}

func (e *FieldError) Error() string { return e.Path + ": " + e.Message }

func bad(path, format string, args ...any) *FieldError {
	return &FieldError{Path: path, Message: fmt.Sprintf(format, args...)}
}

var (
	modelRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,199}$`)
	providerRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)
	currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)
)

// ValidModel reports whether s is a model id: 1 to 200 characters of letters, digits and . _ : / @ + -,
// the first a letter or a digit. An id from a provider that does not match is dropped from the
// catalogue, so an id never carries text.
func ValidModel(s string) bool { return modelRe.MatchString(s) }

// ValidProviderID reports whether s can be a provider id, and so a connection kind.
func ValidProviderID(s string) bool { return providerRe.MatchString(s) }

// TextRequest is one request for text (SDD-016 section 5).
type TextRequest struct {
	Model       string   `json:"model"`
	System      string   `json:"system,omitempty"`
	Prompt      string   `json:"prompt"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
}

// Limits are what the catalogue says of one model; a zero is "not known".
type Limits struct {
	ContextTokens int
	OutputTokens  int
}

// TokenEstimate is a count of tokens before a request runs.
type TokenEstimate struct {
	Input  int
	Output int
}

// Price is the cost of one million input tokens and of one million output tokens, in millionths of the
// currency unit.
type Price struct {
	Currency            string `json:"currency"`
	InputMicrosPerMTok  int64  `json:"input_micros_per_mtok"`
	OutputMicrosPerMTok int64  `json:"output_micros_per_mtok"`
}

// Usage is what a provider says it counted for a finished request.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Cost is an amount of money in millionths of the currency unit.
type Cost struct {
	Micros   int64  `json:"micros"`
	Currency string `json:"currency"`
}

// NormalizeText applies the default of max_tokens and trims the model id.
func NormalizeText(r TextRequest) TextRequest {
	r.Model = strings.TrimSpace(r.Model)

	if r.MaxTokens == 0 {
		r.MaxTokens = DefaultMaxTokens
	}

	return r
}

// ValidateText checks a request, in stored form (NormalizeText), against the rules of SDD-016
// section 5 and the limits the catalogue gives for the model. framing is the number of tokens the
// adapter adds to the input for the framing of its messages.
func ValidateText(r TextRequest, l Limits, framing int) *FieldError {
	if !ValidModel(r.Model) {
		return bad("model", "must be 1 to %d characters of letters, digits and . _ : / @ + -", MaxModelID)
	}

	if f := checkText("prompt", r.Prompt, 1, MaxPromptChars); f != nil {
		return f
	}

	if f := checkText("system", r.System, 0, MaxSystemChars); f != nil {
		return f
	}

	if r.MaxTokens < 1 || r.MaxTokens > MaxOutputTokens {
		return bad("max_tokens", "must be 1 to %d", MaxOutputTokens)
	}

	if l.OutputTokens > 0 && r.MaxTokens > l.OutputTokens {
		return bad("max_tokens", "the model produces at most %d tokens", l.OutputTokens)
	}

	if t := r.Temperature; t != nil && (math.IsNaN(*t) || *t < 0 || *t > MaxTemperature) {
		return bad("temperature", "must be 0 to %g", MaxTemperature)
	}

	if l.ContextTokens > 0 {
		if in := EstimateInput(r.System, r.Prompt) + framing; in+r.MaxTokens > l.ContextTokens {
			return bad(
				"prompt",
				"the prompt and the largest answer do not fit the %d tokens of the model",
				l.ContextTokens,
			)
		}
	}

	return nil
}

func checkText(path, s string, minChars, maxChars int) *FieldError {
	n := utf8.RuneCountInString(s)

	switch {
	case !utf8.ValidString(s):
		return bad(path, "must be valid UTF-8")
	case n < minChars || n > maxChars:
		return bad(path, "must be %d to %d characters", minChars, maxChars)
	case strings.ContainsRune(s, 0):
		return bad(path, "must not contain a NUL character")
	}

	return nil
}

// EstimateInput is the number of input tokens the estimate counts for a system text and a prompt:
// one per BytesPerToken bytes, rounded up, with a minimum of one for a request that has text.
func EstimateInput(system, prompt string) int {
	n := len(system) + len(prompt)

	return (n + BytesPerToken - 1) / BytesPerToken
}

// EstimateTokens is the upper bound of the tokens of a request: the input count plus framing, and
// the largest output the request allows.
func EstimateTokens(r TextRequest, framing int) TokenEstimate {
	return TokenEstimate{Input: EstimateInput(r.System, r.Prompt) + framing, Output: r.MaxTokens}
}

// NormalizePrice checks a price as a person wrote it. Both amounts are non-negative and at most
// MaxPriceMicros, and the currency is three upper-case letters.
func NormalizePrice(p Price) (Price, *FieldError) {
	p.Currency = strings.ToUpper(strings.TrimSpace(p.Currency))

	switch {
	case !currencyRe.MatchString(p.Currency):
		return p, bad("currency", "must be three letters, such as USD")
	case p.InputMicrosPerMTok < 0 || p.InputMicrosPerMTok > MaxPriceMicros:
		return p, bad("input_micros_per_mtok", "must be 0 to %d", int64(MaxPriceMicros))
	case p.OutputMicrosPerMTok < 0 || p.OutputMicrosPerMTok > MaxPriceMicros:
		return p, bad("output_micros_per_mtok", "must be 0 to %d", int64(MaxPriceMicros))
	}

	return p, nil
}

// TextCost is the cost of a number of tokens at a price: each side rounded up, so a cost is never
// rounded down to nothing. The product is computed in 128 bits, so a huge count cannot overflow into
// a small number; a cost that does not fit in an int64 is the largest int64, which no ceiling passes.
func TextCost(u Usage, p Price) Cost {
	in := mulDivCeil(int64(max(u.InputTokens, 0)), p.InputMicrosPerMTok)
	out := mulDivCeil(int64(max(u.OutputTokens, 0)), p.OutputMicrosPerMTok)

	total := in + out
	if total < in {
		total = math.MaxInt64
	}

	return Cost{Micros: total, Currency: p.Currency}
}

// EstimateText is TextCost of the tokens EstimateTokens counts.
func EstimateText(r TextRequest, p Price, framing int) Cost {
	t := EstimateTokens(r, framing)

	return TextCost(Usage{InputTokens: t.Input, OutputTokens: t.Output}, p)
}

// mulDivCeil is ceil(a*b / microsPerMTok), saturating at MaxInt64.
func mulDivCeil(a, b int64) int64 {
	if a <= 0 || b <= 0 {
		return 0
	}

	hi, lo := bits.Mul64(uint64(a), uint64(b))

	lo, carry := bits.Add64(lo, microsPerMTok-1, 0)
	hi += carry

	if hi >= microsPerMTok {
		return math.MaxInt64
	}

	q, _ := bits.Div64(hi, lo, microsPerMTok)

	if q > math.MaxInt64 {
		return math.MaxInt64
	}

	return int64(q)
}

// PriceEntry is one line of the price table the person edits (SDD-016 section 4.3).
type PriceEntry struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Price
}

// NormalizeEntry checks an entry as a person wrote it.
func NormalizeEntry(e PriceEntry) (PriceEntry, *FieldError) {
	e.Provider = strings.TrimSpace(e.Provider)
	e.Model = strings.TrimSpace(e.Model)

	switch {
	case !ValidProviderID(e.Provider):
		return e, bad("provider", "must be the id of a provider")
	case !ValidModel(e.Model):
		return e, bad("model", "must be 1 to %d characters of letters, digits and . _ : / @ + -", MaxModelID)
	}

	p, ferr := NormalizePrice(e.Price)
	e.Price = p

	return e, ferr
}

// ResolvePrice picks the price of a model (SDD-016 section 4.2): the table of the person first, then the
// catalogue of the provider, else none. The second result says where it came from.
func ResolvePrice(user, catalogue *Price) (Price, string) {
	switch {
	case user != nil:
		return *user, SourceUser
	case catalogue != nil:
		return *catalogue, SourceCatalogue
	}

	return Price{}, SourceNone
}
