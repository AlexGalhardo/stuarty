package social

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/AlexGalhardo/stuarty/service/connections"
)

// ErrUnknownNetwork is a network no connector was registered for.
var ErrUnknownNetwork = errors.New("social: no connector for that network")

// Registry maps a network name to its connector. A network is added by its package and one Register
// line (acceptance criterion 5 of epic 18).
type Registry struct {
	mu   sync.RWMutex
	by   map[string]Connector
	bind *connections.Manager
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{by: map[string]Connector{}} }

// Register adds a connector under the network it describes. A network registered twice is an error.
func (r *Registry) Register(c Connector) error {
	d := c.Describe()

	if d.Network == "" {
		return errors.New("social: a connector must name its network")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, dup := r.by[d.Network]; dup {
		return fmt.Errorf("social: the network %q is registered twice", d.Network)
	}

	r.by[d.Network] = c

	if r.bind != nil {
		r.bind.Register(d.Network, c)
	}

	return nil
}

// Bind registers the kind of every connector, now and later, with the connection manager, so a
// connection of the network can be created, listed and tested like any other.
func (r *Registry) Bind(m *connections.Manager) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.bind = m

	for name, c := range r.by {
		m.Register(name, c)
	}
}

// Get returns the connector of a network.
func (r *Registry) Get(network string) (Connector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.by[network]
	if !ok {
		return nil, ErrUnknownNetwork
	}

	return c, nil
}

// Networks lists the names of the networks, sorted.
func (r *Registry) Networks() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return slices.Sorted(maps.Keys(r.by))
}

// List describes every network, sorted by name.
func (r *Registry) List() []Description {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Description, 0, len(r.by))

	for _, n := range slices.Sorted(maps.Keys(r.by)) {
		out = append(out, r.by[n].Describe())
	}

	return out
}
