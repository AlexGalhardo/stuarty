package owner

import (
	"path/filepath"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/anthropic"
	"github.com/AlexGalhardo/stuarty/service/providers/openai"
)

// adapters is the one list of the AI providers of this build (SDD-016 section 2): a provider is a package
// under service/providers and one line here.
func adapters() []providers.Provider {
	return []providers.Provider{
		openai.New(),
		anthropic.New(),
	}
}

// destinations builds the registry of connections and, over it, the registry of the AI providers.
func (o *Owner) destinations(
	getenv func(string) string, st service.Store, now func() time.Time,
) (*connections.Manager, *providers.Registry, error) {
	conns, err := o.connections(getenv, st, now)
	if err != nil {
		return nil, nil, err
	}

	provs, err := o.aiProviders(conns, now)
	if err != nil {
		return nil, nil, err
	}

	return conns, provs, nil
}

// aiProviders builds the registry of the AI providers over the connections and the price table of the
// workspace, with every adapter registered as the connector of its kind.
func (o *Owner) aiProviders(conns *connections.Manager, now func() time.Time) (*providers.Registry, error) {
	reg := providers.NewRegistry(
		conns,
		providers.NewPriceBook(filepath.Join(string(o.Dir), "providers", providers.PriceFile)),
		now,
	)

	for _, p := range adapters() {
		if err := reg.Register(p); err != nil {
			return nil, err
		}
	}

	return reg, nil
}
