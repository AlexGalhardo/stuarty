package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/providers"
)

// ProviderListRequest has no member.
type ProviderListRequest struct{}

// ProviderView is a provider and the connections the person made to it (SDD-016 section 7).
type ProviderView struct {
	providers.Descriptor
	// Connections are the ids of the connections whose kind is this provider.
	Connections []string `json:"connections"`
}

// ProviderListResult is every registered provider and the price table the person edits.
type ProviderListResult struct {
	Providers []ProviderView        `json:"providers"`
	Prices    []provider.PriceEntry `json:"prices"`
}

// ProviderModelsRequest reads the catalogue of one connection. Refresh asks for a new read.
type ProviderModelsRequest struct {
	ConnectionID string `json:"connection_id"`
	Refresh      bool   `json:"refresh,omitempty"`
}

// ModelView is a model of the catalogue with the price that would be used and where it comes from.
type ModelView struct {
	ID            string `json:"id"`
	Name          string `json:"name"           untrusted:"true"`
	ContextTokens int    `json:"context_tokens"`
	OutputTokens  int    `json:"output_tokens"`
	// Price is nil when none is known: a generation with this model is refused until a person sets one.
	Price *provider.Price `json:"price"`
	// PriceSource is user, catalogue or none (provider.Source*).
	PriceSource string `json:"price_source"`
}

// ProviderModelsResult is the catalogue of a connection, read from its provider.
type ProviderModelsResult struct {
	ConnectionID string      `json:"connection_id"`
	Provider     string      `json:"provider"`
	Models       []ModelView `json:"models"`
	FetchedAt    time.Time   `json:"fetched_at"`
	// Cached is true when the list came from memory (it is at most ten minutes old).
	Cached bool `json:"cached"`
}

// ProviderPriceSetRequest sets the price of a model of a provider, or removes it when both amounts are
// empty. The amounts are millionths of the currency unit for a million tokens.
type ProviderPriceSetRequest struct {
	Provider            string `json:"provider"`
	Model               string `json:"model"`
	Currency            string `json:"currency,omitempty"`
	InputMicrosPerMTok  *int64 `json:"input_micros_per_mtok,omitempty"`
	OutputMicrosPerMTok *int64 `json:"output_micros_per_mtok,omitempty"`
}

// ProviderPricesResult is the price table after a change.
type ProviderPricesResult struct {
	Prices []provider.PriceEntry `json:"prices"`
}

// TextGenerateRequest starts a text generation in a project: the arguments of the job kind text.generate
// (SDD-016 section 6) and the wait and the dry run of JobStart.
type TextGenerateRequest struct {
	ProjectID    string `json:"project_id"`
	ConnectionID string `json:"connection_id"`
	provider.TextRequest
	WaitMS int  `json:"wait_ms,omitempty"`
	DryRun bool `json:"dry_run,omitempty"`
}

// ProviderList returns the providers, the connections of each and the price table.
func (s *svc) ProviderList(ctx context.Context, _ ProviderListRequest) (*ProviderListResult, error) {
	if _, err := s.connectionCaller(ctx, "ProviderList"); err != nil {
		return nil, err
	}

	conns, err := s.conns.List(ctx)
	if err != nil {
		return nil, connectionError(err)
	}

	prices, err := s.providers.Prices().List()
	if err != nil {
		return nil, providerError(err)
	}

	out := &ProviderListResult{Providers: []ProviderView{}, Prices: prices}

	for _, d := range s.providers.Descriptors() {
		v := ProviderView{Descriptor: d, Connections: []string{}}

		for i := range conns {
			if conns[i].Kind == d.ID {
				v.Connections = append(v.Connections, conns[i].ID)
			}
		}

		out.Providers = append(out.Providers, v)
	}

	return out, nil
}

// ProviderModels reads the models of a connection from its provider.
func (s *svc) ProviderModels(ctx context.Context, req ProviderModelsRequest) (*ProviderModelsResult, error) {
	if _, err := s.connectionCaller(ctx, "ProviderModels"); err != nil {
		return nil, err
	}

	cat, err := s.providers.Models(ctx, req.ConnectionID, req.Refresh)
	if err != nil {
		return nil, providerError(err)
	}

	out := &ProviderModelsResult{
		ConnectionID: cat.Connection.ID, Provider: cat.Provider.ID, Models: make([]ModelView, 0, len(cat.Models)),
		FetchedAt: cat.FetchedAt.UTC(), Cached: cat.Cached,
	}

	// The table is read once for the whole list: a catalogue has up to 2,000 models, and a lookup per model
	// would read the file as many times.
	entries, perr := s.providers.Prices().List()
	if perr != nil {
		return nil, providerError(perr)
	}

	table := make(map[string]provider.Price, len(entries))

	for i := range entries {
		if entries[i].Provider == cat.Provider.ID {
			table[entries[i].Model] = entries[i].Price
		}
	}

	for i := range cat.Models {
		m := &cat.Models[i]

		var user *provider.Price
		if p, ok := table[m.ID]; ok {
			user = &p
		}

		price, source := provider.ResolvePrice(user, m.Price)
		v := ModelView{
			ID: m.ID, Name: m.Name, ContextTokens: m.ContextTokens, OutputTokens: m.OutputTokens, PriceSource: source,
		}

		if source != provider.SourceNone {
			v.Price = &price
		}

		out.Models = append(out.Models, v)
	}

	return out, nil
}

// ProviderPriceSet sets or removes one entry of the price table. A lower price lowers every estimate, so
// only a person in the app does it (SDD-016 section 4.3).
func (s *svc) ProviderPriceSet(ctx context.Context, req ProviderPriceSetRequest) (*ProviderPricesResult, error) {
	c, err := s.person(ctx, "ProviderPriceSet")
	if err != nil {
		return nil, err
	}

	if _, ok := s.providers.Provider(req.Provider); !ok {
		return nil, newError(command.CodeValidation, "no provider has the id %s.", req.Provider).
			withPath("provider").withHint("list the providers with provider:list")
	}

	remove := req.InputMicrosPerMTok == nil && req.OutputMicrosPerMTok == nil
	if !remove && (req.InputMicrosPerMTok == nil || req.OutputMicrosPerMTok == nil) {
		return nil, newError(command.CodeValidation, "give both amounts, or none to remove the price.").
			withPath("input_micros_per_mtok").
			withHint("a price with only one side would estimate the other at zero")
	}

	entry := provider.PriceEntry{Provider: req.Provider, Model: req.Model}
	if !remove {
		entry.Currency, entry.InputMicrosPerMTok, entry.OutputMicrosPerMTok = req.Currency, *req.InputMicrosPerMTok, *req.OutputMicrosPerMTok
	}

	var prices []provider.PriceEntry

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		var perr error

		if remove {
			prices, perr = s.providers.Prices().Remove(req.Provider, req.Model)
		} else {
			norm, ferr := provider.NormalizeEntry(entry)
			if ferr != nil {
				return newError(command.CodeValidation, "%s", ferr.Message).withPath(ferr.Path)
			}

			entry = norm
			prices, perr = s.providers.Prices().Set(norm)
		}

		if perr != nil {
			return providerError(perr)
		}

		raw, merr := json.Marshal(map[string]any{
			"model": req.Model, "currency": entry.Currency, "input_micros_per_mtok": req.InputMicrosPerMTok,
			"output_micros_per_mtok": req.OutputMicrosPerMTok,
		})
		if merr != nil {
			return merr
		}

		subject := req.Provider

		return tx.Audit().Append(ctx, AuditRow{
			AtMs: s.nowMs(), Actor: c.Actor, Action: "provider.price.set", SubjectID: &subject, Details: raw,
		})
	})
	if werr != nil {
		return nil, asError(werr)
	}

	return &ProviderPricesResult{Prices: prices}, nil
}

// TextGenerate starts a text.generate job in a project and returns it. The estimate is on the job from the
// start; the job waits for a person, or for a rule, as every costly generation does (SDD-015 section 5).
func (s *svc) TextGenerate(ctx context.Context, req TextGenerateRequest) (*Job, error) {
	args, err := json.Marshal(TextArgs{ConnectionID: req.ConnectionID, TextRequest: req.TextRequest})
	if err != nil {
		return nil, newError(command.CodeInternal, "the request cannot be encoded.")
	}

	return s.JobStart(ctx, JobStartRequest{
		ProjectID: req.ProjectID, Kind: JobTextGenerate, Args: args, WaitMS: req.WaitMS,
		DryRun: req.DryRun,
	})
}
