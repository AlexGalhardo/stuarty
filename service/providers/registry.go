package providers

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections"
)

// CatalogueTTL is how long the catalogue of a connection is kept in memory (SDD-016 section 3).
const CatalogueTTL = 10 * time.Minute

// ErrNotProvider is a connection whose kind is not the id of a registered provider.
var ErrNotProvider = errors.New("providers: the connection is not a connection to an AI provider")

// Registry finds the adapter of a connection, reads and keeps the catalogues and resolves prices
// (SDD-016 sections 2 to 4). It is built at start-up from the adapters.
type Registry struct {
	conns  *connections.Manager
	prices *PriceBook
	now    func() time.Time
	ttl    time.Duration

	mu        sync.RWMutex
	providers map[string]Provider
	cache     map[string]catalogue
}

type catalogue struct {
	at     time.Time
	models []Model
}

// NewRegistry returns a registry over the connections and the price table. now defaults to time.Now.
func NewRegistry(conns *connections.Manager, prices *PriceBook, now func() time.Time) *Registry {
	if now == nil {
		now = time.Now
	}

	return &Registry{
		conns: conns, prices: prices, now: now, ttl: CatalogueTTL,
		providers: map[string]Provider{}, cache: map[string]catalogue{},
	}
}

// Register adds a provider and makes it the connector of its kind in the connections manager, so a
// connection of that kind can be created, listed and tested (SDD-016 section 2). It fails for an
// id that is not valid or already taken, and for a provider that has no capability.
func (r *Registry) Register(p Provider) error {
	d := p.Descriptor()

	switch {
	case !provider.ValidProviderID(d.ID):
		return fmt.Errorf("providers: %q is not a valid provider id", d.ID)
	case len(d.Capabilities) == 0:
		return fmt.Errorf("providers: %s has no capability", d.ID)
	case slices.Contains(d.Capabilities, provider.CapabilityText) && !isTextGenerator(p):
		return fmt.Errorf("providers: %s says it writes text and does not implement TextGenerator", d.ID)
	case slices.Contains(d.Capabilities, provider.CapabilityImage) && !isImageGenerator(p):
		return fmt.Errorf("providers: %s says it makes images and does not implement ImageGenerator", d.ID)
	case slices.Contains(d.Capabilities, provider.CapabilityVoice) && !isVoiceGenerator(p):
		return fmt.Errorf("providers: %s says it makes speech and does not implement VoiceGenerator", d.ID)
	case slices.Contains(d.Capabilities, provider.CapabilityVideo) && !isVideoGenerator(p):
		return fmt.Errorf("providers: %s says it makes video and does not implement VideoGenerator", d.ID)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, taken := r.providers[d.ID]; taken {
		return fmt.Errorf("providers: %s is registered twice", d.ID)
	}

	r.providers[d.ID] = p
	r.conns.Register(d.ID, connector{p: p})

	return nil
}

func isTextGenerator(p Provider) bool {
	_, ok := p.(TextGenerator)

	return ok
}

func isImageGenerator(p Provider) bool {
	_, ok := p.(ImageGenerator)

	return ok
}

func isVoiceGenerator(p Provider) bool {
	_, ok := p.(VoiceGenerator)

	return ok
}

func isVideoGenerator(p Provider) bool {
	_, ok := p.(VideoGenerator)

	return ok
}

// Video returns the video generator whose id is kind, when its descriptor says it makes video.
func (r *Registry) Video(kind string) (VideoGenerator, bool) {
	p, ok := r.Provider(kind)
	if !ok || !slices.Contains(p.Descriptor().Capabilities, provider.CapabilityVideo) {
		return nil, false
	}

	g, ok := p.(VideoGenerator)

	return g, ok
}

// Image returns the image generator whose id is kind, when its descriptor says it makes images.
func (r *Registry) Image(kind string) (ImageGenerator, bool) {
	p, ok := r.Provider(kind)
	if !ok || !slices.Contains(p.Descriptor().Capabilities, provider.CapabilityImage) {
		return nil, false
	}

	g, ok := p.(ImageGenerator)

	return g, ok
}

// Voice returns the voice generator whose id is kind, when its descriptor says it makes speech.
func (r *Registry) Voice(kind string) (VoiceGenerator, bool) {
	p, ok := r.Provider(kind)
	if !ok || !slices.Contains(p.Descriptor().Capabilities, provider.CapabilityVoice) {
		return nil, false
	}

	g, ok := p.(VoiceGenerator)

	return g, ok
}

// Descriptors lists the registered providers, sorted by id.
func (r *Registry) Descriptors() []Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Descriptor, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, p.Descriptor())
	}

	slices.SortFunc(out, func(a, b Descriptor) int { return compare(a.ID, b.ID) })

	return out
}

func compare(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}

	return 0
}

// Provider returns the provider whose id is kind.
func (r *Registry) Provider(kind string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[kind]

	return p, ok
}

// Text returns the text generator whose id is kind.
func (r *Registry) Text(kind string) (TextGenerator, bool) {
	p, ok := r.Provider(kind)
	if !ok {
		return nil, false
	}

	g, ok := p.(TextGenerator)

	return g, ok
}

// Prices is the price table the person edits.
func (r *Registry) Prices() *PriceBook { return r.prices }

// Catalogue is the list of models of one connection, read from its provider.
type Catalogue struct {
	Connection connections.Connection
	Provider   Descriptor
	Models     []Model
	// FetchedAt is when the provider was asked; Cached is true when this answer came from memory.
	FetchedAt time.Time
	Cached    bool
}

// Models returns the catalogue of a connection. A copy younger than CatalogueTTL is served unless
// refresh is set; the connection is checked first either way, so a removed connection is never served
// from memory. The provider is asked through the gate of the connection.
func (r *Registry) Models(ctx context.Context, connectionID string, refresh bool) (*Catalogue, error) {
	conn, err := r.conns.Get(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	p, ok := r.Provider(conn.Kind)
	if !ok {
		return nil, ErrNotProvider
	}

	if !refresh {
		r.mu.RLock()
		c, hit := r.cache[connectionID]
		r.mu.RUnlock()

		if hit && r.now().Sub(c.at) < r.ttl {
			return &Catalogue{
				Connection: conn,
				Provider:   p.Descriptor(),
				Models:     slices.Clone(c.models),
				FetchedAt:  c.at,
				Cached:     true,
			}, nil
		}
	}

	var models []Model

	err = r.conns.With(ctx, connectionID, func(cred connections.Credential) error {
		var merr error

		models, merr = p.Models(ctx, cred)
		models = withoutEchoes(cred, models)

		return SignInAware(merr, cred)
	})
	if err != nil {
		return nil, err
	}

	models = finish(models)
	at := r.now()

	r.mu.Lock()
	r.cache[connectionID] = catalogue{at: at, models: models}
	r.mu.Unlock()

	return &Catalogue{Connection: conn, Provider: p.Descriptor(), Models: slices.Clone(models), FetchedAt: at}, nil
}

// Forget drops the copy of a connection, for a connection that was removed.
func (r *Registry) Forget(connectionID string) {
	r.mu.Lock()
	delete(r.cache, connectionID)
	r.mu.Unlock()
}

// Peek returns a model of the copy in memory of a connection, with no network and no check of the
// age: the estimate of a job (SDD-015 section 5: Prepare opens nothing) uses what is known.
func (r *Registry) Peek(connectionID, model string) (Model, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, m := range r.cache[connectionID].models {
		if m.ID == model {
			return m, true
		}
	}

	return Model{}, false
}

// PriceOf resolves the price of a model (SDD-016 section 4.2): the table of the person, then the
// catalogue copy in memory. The source is provider.SourceNone when neither has one. It opens nothing.
func (r *Registry) PriceOf(providerID, connectionID, model string) (provider.Price, string, error) {
	user, err := r.prices.Lookup(providerID, model)
	if err != nil {
		return provider.Price{}, provider.SourceNone, err
	}

	var cat *provider.Price
	if m, ok := r.Peek(connectionID, model); ok {
		cat = m.Price
	}

	p, src := provider.ResolvePrice(user, cat)

	return p, src, nil
}

// withoutEchoes drops the models whose id or name holds the credential: a provider, or a server that
// speaks its protocol, may repeat the key it was sent, and a catalogue is shown to every caller (T-55).
func withoutEchoes(cred connections.Credential, models []Model) []Model {
	out := models[:0:0]

	for i := range models {
		if cred.Echoes(models[i].ID) || cred.Echoes(models[i].Name) {
			continue
		}

		out = append(out, models[i])
	}

	return out
}

// connector makes a provider the connector of its kind: its test is the read of the catalogue.
type connector struct{ p Provider }

// Test reads the catalogue, a request that spends nothing. The message is written by the product from
// the outcome; no body of the provider is forwarded.
func (c connector) Test(ctx context.Context, cred connections.Credential) (connections.TestResult, error) {
	models, err := c.p.Models(ctx, cred)
	if err == nil {
		models = finish(models)

		return connections.TestResult{
			OK:     true,
			Status: 200,
			Message: connections.SafeMessage(
				cred,
				fmt.Appendf(nil, "the provider answered; %d models are available.", len(models)),
			),
		}, nil
	}

	if isGateError(err) || errors.Is(err, context.Canceled) {
		return connections.TestResult{}, err
	}

	res := connections.TestResult{Message: connections.SafeMessage(cred, []byte(failureSentence(err)))}
	if pe, ok := AsError(err); ok {
		res.Status = pe.Status
	}

	return res, nil
}

func isGateError(err error) bool {
	return errors.Is(err, outbound.ErrNoConnection) || errors.Is(err, outbound.ErrDestination) ||
		errors.Is(err, outbound.ErrNotPublic)
}

// failureSentence says what went wrong in a fixed sentence per kind of failure.
func failureSentence(err error) string {
	pe, ok := AsError(err)
	if !ok {
		return "the provider could not be reached."
	}

	switch pe.Kind {
	case KindAuth:
		return "the provider refused the key."
	case KindRateLimited:
		return "the provider is limiting the requests; try again later."
	case KindBadRequest:
		return "the provider refused the request."
	case KindUnreadable:
		return "the provider answered in a form this version does not understand."
	}

	return "the provider could not be reached."
}
