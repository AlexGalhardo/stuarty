package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/AlexGalhardo/stuarty/core/provider"
)

// PriceFile is the name of the price table inside the providers folder of the workspace.
const PriceFile = "prices.json"

const (
	priceFormat  = "stuarty-prices"
	priceVersion = 1
	// maxPriceFile bounds what is read of the file.
	maxPriceFile = 1 << 20
)

// ErrTooManyPrices is a table that would have more than provider.MaxPrices entries.
var ErrTooManyPrices = errors.New("providers: the price table is full")

// ErrBadPriceFile is a price table that does not parse or has an unknown version.
var ErrBadPriceFile = errors.New("providers: the price table is not a valid file")

type priceFileJSON struct {
	Format  string                `json:"format"`
	Version int                   `json:"version"`
	Prices  []provider.PriceEntry `json:"prices"`
}

// PriceBook is the table of prices the person edits for the models whose provider publishes none
// (SDD-016 section 4.3): one JSON file, written through a temporary file and a rename. It holds no
// secret. One mutex serialises the operations of a process.
type PriceBook struct {
	path string
	mu   sync.Mutex
	// mem is the table of a book with no file (an empty path): a service with no workspace keeps it in memory.
	mem []provider.PriceEntry
}

// NewPriceBook returns a price table kept at path. Nothing is read or created until the first call. An
// empty path is a table kept in memory only.
func NewPriceBook(path string) *PriceBook { return &PriceBook{path: path} }

// List returns every entry, sorted by provider and model. A missing file is an empty table.
func (b *PriceBook) List() ([]provider.PriceEntry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.read()
}

// Lookup returns the price of a model of a provider, or nil when the table has none.
func (b *PriceBook) Lookup(providerID, model string) (*provider.Price, error) {
	entries, err := b.List()
	if err != nil {
		return nil, err
	}

	for i := range entries {
		if entries[i].Provider == providerID && entries[i].Model == model {
			p := entries[i].Price

			return &p, nil
		}
	}

	return nil, nil //nolint:nilnil // no price is an answer, not a failure
}

// Set adds or replaces the entry of a provider and a model, and returns the table.
func (b *PriceBook) Set(e provider.PriceEntry) ([]provider.PriceEntry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entries, err := b.read()
	if err != nil {
		return nil, err
	}

	if i := slices.IndexFunc(entries, same(e)); i >= 0 {
		entries[i] = e
	} else {
		if len(entries) >= provider.MaxPrices {
			return nil, ErrTooManyPrices
		}

		entries = append(entries, e)
	}

	sortEntries(entries)

	return entries, b.write(entries)
}

// Remove deletes the entry of a provider and a model, if there is one, and returns the table.
func (b *PriceBook) Remove(providerID, model string) ([]provider.PriceEntry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entries, err := b.read()
	if err != nil {
		return nil, err
	}

	e := provider.PriceEntry{Provider: providerID, Model: model}

	i := slices.IndexFunc(entries, same(e))
	if i < 0 {
		return entries, nil
	}

	entries = slices.Delete(entries, i, i+1)

	return entries, b.write(entries)
}

func same(e provider.PriceEntry) func(provider.PriceEntry) bool {
	return func(o provider.PriceEntry) bool { return o.Provider == e.Provider && o.Model == e.Model }
}

func sortEntries(entries []provider.PriceEntry) {
	slices.SortFunc(entries, func(a, b provider.PriceEntry) int {
		if c := strings.Compare(a.Provider, b.Provider); c != 0 {
			return c
		}

		return strings.Compare(a.Model, b.Model)
	})
}

func (b *PriceBook) read() ([]provider.PriceEntry, error) {
	if b.path == "" {
		return append([]provider.PriceEntry{}, b.mem...), nil
	}

	f, err := os.Open(b.path)
	if errors.Is(err, fs.ErrNotExist) {
		return []provider.PriceEntry{}, nil
	}

	if err != nil {
		return nil, err
	}

	defer func() { _ = f.Close() }()

	var doc priceFileJSON

	dec := json.NewDecoder(io.LimitReader(f, maxPriceFile))
	if err := dec.Decode(&doc); err != nil || doc.Format != priceFormat || doc.Version != priceVersion {
		return nil, ErrBadPriceFile
	}

	// Every entry is checked again: the file is a person's to edit by hand, and what is read is
	// what the estimate trusts.
	out := make([]provider.PriceEntry, 0, len(doc.Prices))
	seen := make(map[[2]string]bool, len(doc.Prices))

	for i := range doc.Prices {
		e, ferr := provider.NormalizeEntry(doc.Prices[i])
		if ferr != nil {
			return nil, fmt.Errorf("%w: entry %d: %s", ErrBadPriceFile, i, ferr.Path)
		}

		// One price per provider and model (SDD-016 section 4.3): with two, which one an estimate used would
		// depend on the order of a sort, not on the person.
		key := [2]string{e.Provider, e.Model}
		if seen[key] {
			return nil, fmt.Errorf("%w: entry %d: a second price for %s %s", ErrBadPriceFile, i, e.Provider, e.Model)
		}

		seen[key] = true
		out = append(out, e)
	}

	if len(out) > provider.MaxPrices {
		return nil, ErrBadPriceFile
	}

	sortEntries(out)

	return out, nil
}

func (b *PriceBook) write(entries []provider.PriceEntry) error {
	if b.path == "" {
		b.mem = slices.Clone(entries)

		return nil
	}

	raw, err := json.MarshalIndent(priceFileJSON{Format: priceFormat, Version: priceVersion, Prices: entries}, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(b.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".prices-*.tmp")
	if err != nil {
		return err
	}

	name := tmp.Name()

	if err := writeSynced(tmp, raw); err != nil {
		_ = os.Remove(name)

		return err
	}

	if err := os.Rename(name, b.path); err != nil {
		_ = os.Remove(name)

		return err
	}

	return nil
}

// writeSynced writes raw to f with mode 0600, flushes it and closes it.
func writeSynced(f *os.File, raw []byte) error {
	if err := f.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		_ = f.Close()

		return err
	}

	if _, err := f.Write(raw); err != nil {
		_ = f.Close()

		return err
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()

		return err
	}

	return f.Close()
}
