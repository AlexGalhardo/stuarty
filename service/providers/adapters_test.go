package providers_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/anthropic"
	"github.com/AlexGalhardo/stuarty/service/providers/openai"
	"github.com/AlexGalhardo/stuarty/service/providers/providertest"
)

// adapters is the one list of the adapters of a build: a provider is added with one line here, the
// same line the owner has (web/app/owner).
func adapters() []providers.Provider { return []providers.Provider{openai.New(), anthropic.New()} }

func TestBothAdaptersServeThroughTheRegistry(t *testing.T) {
	t.Parallel()

	rec := providertest.Serve(t, filepath.Join("anthropic", "testdata"),
		providertest.Exchange{Method: "GET", Path: "/v1/models?limit=1000", Response: "models.json"},
		providertest.Exchange{Method: "GET", Path: "/v1/models?limit=1000", Response: "models.json"},
	)
	h := connectionstest.New(t, anthropic.ID, rec.Endpoint("/v1"), "sk-ant-test-0123456789")

	reg := providers.NewRegistry(
		h.Manager,
		providers.NewPriceBook(filepath.Join(t.TempDir(), providers.PriceFile)),
		time.Now,
	)
	for _, p := range adapters() {
		if err := reg.Register(p); err != nil {
			t.Fatal(err)
		}
	}

	var ids []string
	for _, d := range reg.Descriptors() {
		ids = append(ids, d.ID)
	}

	if !slices.Equal(ids, []string{"anthropic", "openai"}) {
		t.Fatalf("got %v", ids)
	}

	if !slices.Contains(h.Manager.Kinds(), "openai") || !slices.Contains(h.Manager.Kinds(), "anthropic") {
		t.Fatalf("each provider is a kind of connection, got %v", h.Manager.Kinds())
	}

	cat, err := reg.Models(context.Background(), connectionstest.ID, false)
	if err != nil {
		t.Fatal(err)
	}

	// The invalid id of the recorded answer is dropped; the name is one clean line.
	if len(cat.Models) != 2 || cat.Models[0].ID != "claude-haiku-x" || strings.ContainsAny(cat.Models[0].Name, "\n") {
		t.Fatalf("got %+v", cat.Models)
	}

	// The limits the catalogue gives bound a request before it is sent.
	m, ok := reg.Peek(connectionstest.ID, "claude-sonnet-x")
	if !ok || m.OutputTokens != 64000 {
		t.Fatalf("got %+v", m)
	}

	res, err := h.Manager.Test(context.Background(), connectionstest.ID)
	if err != nil || !res.OK || !strings.Contains(res.Message, "2 models") {
		t.Fatalf("a connection of a provider is tested by reading its catalogue: %+v, %v", res, err)
	}
}

func TestAddingAProviderNeedsOnePackageAndOneLine(t *testing.T) {
	t.Parallel()

	// Every folder of service/providers that is an adapter is one package, and the registry knows no
	// provider by name: adding a provider is a folder and a line in adapters() above and in the owner.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	support := []string{"wire", "providertest"}

	var dirs []string

	for _, e := range entries {
		if e.IsDir() && e.Name() != "testdata" && !slices.Contains(support, e.Name()) {
			dirs = append(dirs, e.Name())
		}
	}

	var registered []string
	for _, p := range adapters() {
		registered = append(registered, p.Descriptor().ID)
	}

	slices.Sort(dirs)
	slices.Sort(registered)

	if !slices.Equal(dirs, registered) {
		t.Fatalf("the adapter folders %v and the registered providers %v must be the same", dirs, registered)
	}

	// The registry and the shared code name no provider.
	for _, f := range []string{"registry.go", "providers.go", "prices.go", filepath.Join("wire", "wire.go")} {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}

		for _, id := range registered {
			if strings.Contains(strings.ToLower(string(raw)), "\""+id+"\"") {
				t.Errorf("%s names the provider %q", f, id)
			}
		}
	}

	var _ providers.TextGenerator = openai.New()
	var _ providers.TextGenerator = anthropic.New()
}
