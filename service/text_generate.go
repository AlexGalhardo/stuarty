package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
	"unicode"
	"unicode/utf8"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// JobTextGenerate is the job kind of a text generation (SDD-016 section 6).
const JobTextGenerate = "text.generate"

// Limits of the summary a person reads before approving a generation (SDD-016 section 6).
const (
	maxExcerpt = 200
	// textTimeout is how long one generation may take.
	textTimeout = 120 * time.Second
)

// TextArgs are the arguments of a text.generate job: the connection, and the request of
// SDD-016 section 5.
type TextArgs struct {
	ConnectionID string `json:"connection_id"`
	provider.TextRequest
}

// TextResult is the result of a succeeded text.generate job (SDD-016 section 6).
type TextResult struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	// Text is what the model wrote: untrusted text, delimited wherever a text view shows it to an agent.
	Text         string     `json:"text"          untrusted:"true"`
	FinishReason string     `json:"finish_reason"`
	InputTokens  int        `json:"input_tokens"`
	OutputTokens int        `json:"output_tokens"`
	Cost         core.Money `json:"cost"`
	// ItemID and Revision are set by a text.draft job: the text and the revision the draft was written as.
	ItemID   string `json:"item_id,omitempty"`
	Revision int    `json:"revision,omitempty"`
}

// textInput is the input column of a text.generate job: what was asked, and the price the person saw,
// so what is spent is judged with it (SDD-016 section 6).
type textInput struct {
	ConnectionID string               `json:"connection_id"`
	Provider     string               `json:"provider"`
	Request      provider.TextRequest `json:"request"`
	Price        provider.Price       `json:"price"`
	PriceSource  string               `json:"price_source"`
	Estimate     core.Money           `json:"estimate"`
}

// textGenerateKind is the approvable kind that runs a text generation through the provider of a
// connection (SDD-015 section 5, SDD-016 section 6).
func (s *svc) textGenerateKind() ApprovableKind {
	return ApprovableKind{
		Name: JobTextGenerate, Scope: ScopeEdit, Prepare: s.prepareText, Run: s.runText,
	}
}

func decodeTextArgs(raw json.RawMessage) (TextArgs, *Error) {
	var a TextArgs

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if len(bytes.TrimSpace(raw)) == 0 || dec.Decode(&a) != nil || dec.More() {
		return a, newError(
			command.CodeValidation,
			"args must be one JSON object with connection_id, model, prompt and optionally system, max_tokens and temperature.",
		).
			withPath("args")
	}

	return a, nil
}

// prepareText validates the request, finds the price and computes the estimate. It makes no outbound
// call: the limits and a published price are read from the catalogue copy in memory.
func (s *svc) prepareText(ctx context.Context, req JobStartRequest) (Prepared, error) {
	args, aerr := decodeTextArgs(req.Args)
	if aerr != nil {
		return Prepared{}, aerr
	}

	conn, gen, err := s.textProvider(ctx, args.ConnectionID)
	if err != nil {
		return Prepared{}, err
	}

	r := provider.NormalizeText(args.TextRequest)

	limits := provider.Limits{}
	if m, ok := s.providers.Peek(conn.ID, r.Model); ok {
		limits = provider.Limits{ContextTokens: m.ContextTokens, OutputTokens: m.OutputTokens}
	}

	if ferr := provider.ValidateText(r, limits, gen.Framing()); ferr != nil {
		return Prepared{}, newError(command.CodeValidation, "%s", ferr.Message).withPath("args." + ferr.Path)
	}

	price, source, perr := s.providers.PriceOf(conn.Kind, conn.ID, r.Model)
	if perr != nil {
		return Prepared{}, providerError(perr)
	}

	// A price per image or per million characters is the price of another capability: for text it is none.
	if source == provider.SourceNone || price.Unit != provider.UnitTokens {
		return Prepared{}, newError(CodePriceUnknown, "no price is known for the model %s of %s.", r.Model, conn.Kind).
			withDetail("provider", conn.Kind).withDetail("model", r.Model).
			withHint("read the models of the connection with provider:models to see if the provider publishes a price, " +
				"or ask a person to set one with provider:price or on the Connections page")
	}

	cost := provider.EstimateText(r, price, gen.Framing())
	estimate := core.Money{Micros: cost.Micros, Currency: cost.Currency}

	input, merr := json.Marshal(textInput{
		ConnectionID: conn.ID, Provider: conn.Kind, Request: r, Price: price, PriceSource: source, Estimate: estimate,
	})
	if merr != nil {
		return Prepared{}, merr
	}

	return Prepared{
		Summary: textSummary(gen.Descriptor().Name, &r), ConnectionID: conn.ID, Estimate: estimate, Input: input,
	}, nil
}

// textProvider finds the connection and the text generator of its kind, and checks that the connection
// accepts text.
func (s *svc) textProvider(
	ctx context.Context,
	id string,
) (conn connections.Connection, gen providers.TextGenerator, err error) {
	if id == "" {
		// The job_start description has no room for the arguments (NFR-020), so the first refusal names them all.
		return conn, nil, newError(command.CodeValidation, "connection_id is required.").withPath("args.connection_id").
			withHint("args are connection_id, model, prompt and optionally system, max_tokens and temperature; " +
				"provider:list names the connections")
	}

	conn, cerr := s.conns.Get(ctx, id)
	if cerr != nil {
		return conn, nil, connectionError(cerr)
	}

	gen, ok := s.providers.Text(conn.Kind)
	if !ok || !slices.Contains(conn.Accepts, connections.AcceptText) {
		return conn, nil, newError(command.CodeValidation, "the connection is not a connection to a provider of text.").
			withPath("args.connection_id").
			withHint("list the providers with provider:list and connect one with connection:add")
	}

	return conn, gen, nil
}

// textSummary is what a person reads before approving (SDD-016 section 6): the provider, the model, the
// sizes and an excerpt of the prompt cleaned of every character that could hide something. It is made of
// text the product wrote and of the excerpt, and fits the 500 characters of SDD-015 section 3.
func textSummary(providerName string, r *provider.TextRequest) string {
	head := fmt.Sprintf("Generate text with %s, model %s: a prompt of %d characters, up to %d tokens back.",
		providerName, r.Model, utf8.RuneCountInString(r.Prompt), r.MaxTokens)

	room := min(maxExcerpt, core.MaxSummary-utf8.RuneCountInString(head)-len(" Starts with: “”…"))
	if room < 10 {
		return head
	}

	return head + " Starts with: “" + excerpt(r.Prompt, room) + "”…"
}

// excerpt is the first n characters of s with every control, invisible and separator character removed
// and white space collapsed to one space.
func excerpt(s string, n int) string {
	out := make([]rune, 0, n)
	space := false

	for _, r := range s {
		if len(out) >= n {
			break
		}

		switch {
		case unicode.IsSpace(r):
			space = len(out) > 0

			continue
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), untrusted.Removed(r), r == utf8.RuneError:
			continue
		}

		if space {
			out = append(out, ' ')
			space = false
		}

		out = append(out, r)
	}

	return string(out[:min(len(out), n)])
}

// runText is the job once approved: one request through the connection and the real cost from the
// usage the provider reports, at the price the person saw.
func (s *svc) runText(ctx context.Context, run JobRun) (JobOutcome, error) {
	var in textInput
	if err := json.Unmarshal(run.Input, &in); err != nil {
		return JobOutcome{}, newError(command.CodeInternal, "the input of the job is not readable.")
	}

	gen, ok := s.providers.Text(in.Provider)
	if !ok {
		return JobOutcome{}, newError(command.CodeValidation, "the provider %s is no longer available.", in.Provider).
			withHint("connect a provider with connection:add")
	}

	run.Report(ctx, 0.1, "asking the provider")

	var res providers.TextResult

	err := s.conns.WithTimeout(ctx, in.ConnectionID, textTimeout, func(c connections.Credential) error {
		var gerr error

		res, gerr = gen.GenerateText(ctx, c, in.Request)
		// A server that speaks the protocol may repeat the key it was sent; the text is shown to every
		// caller, so an answer that holds the key is dropped, and the cost is still recorded (T-55).
		if gerr == nil && (c.Echoes(res.Text) || c.Echoes(res.Model)) {
			res.Text, res.Model = connections.EchoedAnswer, in.Request.Model
		}

		return gerr
	})
	if err != nil {
		return JobOutcome{}, providerError(err)
	}

	// The model an answer names is written by the server, and the result does not mark it as untrusted: one
	// that is not a model id (SDD-016 section 3) is replaced by the model that was asked for.
	if !provider.ValidModel(res.Model) {
		res.Model = in.Request.Model
	}

	cost := provider.TextCost(res.Usage, in.Price)
	spent := core.Money{Micros: cost.Micros, Currency: cost.Currency}

	raw, merr := json.Marshal(JobResult{Text: &TextResult{
		Provider: in.Provider, Model: res.Model, Text: res.Text, FinishReason: res.FinishReason,
		InputTokens: res.Usage.InputTokens, OutputTokens: res.Usage.OutputTokens, Cost: spent,
	}})
	if merr != nil {
		return JobOutcome{Cost: &spent}, merr
	}

	return JobOutcome{Result: raw, Cost: &spent}, nil
}

// providerError maps the errors of the registry, the adapters and the gate to the codes of SDD-016
// section 8 and SDD-014 section 7. The text is written here from the kind and the status: the body of a
// provider and the text of a transport error are never in it.
func providerError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return newError(CodeProvider, "the provider did not answer in time.").
			withDetail("kind", providers.KindTimeout).
			withHint("try again later; a generation of several files may be asked in smaller ones")
	}

	if pe, ok := providers.AsError(err); ok {
		e := newError(
			CodeProvider,
			"%s",
			providerSentence(pe),
		).withDetail("kind", pe.Kind).
			withHint("%s", providerHint(pe))

		if pe.Status != 0 {
			_ = e.withDetail("status", pe.Status)
		}

		if pe.Code != "" {
			_ = e.withDetail("code", pe.Code)
		}

		if pe.RetryAfter > 0 {
			_ = e.withDetail("retry_after_seconds", pe.RetryAfter)
		}

		return e
	}

	switch {
	case errors.Is(err, providers.ErrNotProvider):
		return newError(command.CodeValidation, "the connection is not a connection to an AI provider.").
			withPath("connection_id").withHint("list the providers with provider:list")
	case errors.Is(err, providers.ErrBadPriceFile), errors.Is(err, providers.ErrTooManyPrices):
		return newError(command.CodeInternal, "the table of prices cannot be used.").
			withHint("fix or remove providers/prices.json in the workspace; the file holds no secret")
	}

	return connectionError(err)
}

func providerSentence(pe *providers.Error) string {
	switch pe.Kind {
	case providers.KindAuth:
		return "the provider refused the key of the connection."
	case providers.KindRateLimited:
		return "the provider is limiting the requests."
	case providers.KindBadRequest:
		return "the provider refused the request."
	case providers.KindRefused:
		return "the provider declined the prompt."
	case providers.KindTimeout:
		return "the provider did not answer in time."
	case providers.KindUnreadable:
		return "the provider answered in a form this version does not understand."
	}

	return "the provider could not be reached or failed."
}

func providerHint(pe *providers.Error) string {
	switch pe.Kind {
	case providers.KindAuth:
		return "check the key: connect the account again with connection:add"
	case providers.KindRateLimited:
		return "wait and submit the generation again"
	case providers.KindBadRequest:
		return "check the model and the size of the request against the models of the connection (provider:models)"
	case providers.KindRefused:
		return "the provider applies its own content policy: change the prompt and submit the generation again"
	case providers.KindTimeout:
		return "try again later; a generation of several files may be asked in smaller ones"
	}

	return "try again later; the connection is tested with connection:test"
}
