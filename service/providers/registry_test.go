package providers

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/vault"
)

type memStore struct {
	mu   sync.Mutex
	rows map[string]connections.Row
}

func (s *memStore) List(context.Context) ([]connections.Row, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]connections.Row, 0, len(s.rows))
	for id := range s.rows {
		out = append(out, s.rows[id])
	}

	return out, nil
}

func (s *memStore) Get(_ context.Context, id string) (connections.Row, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.rows[id]
	if !ok {
		return connections.Row{}, connections.ErrNotFound
	}

	return r, nil
}

func (s *memStore) Count(context.Context) (int, error) { return len(s.rows), nil }

func (s *memStore) Insert(_ context.Context, r connections.Row) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.rows[r.ID] = r

	return nil
}

func (s *memStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.rows[id]; !ok {
		return connections.ErrNotFound
	}

	delete(s.rows, id)

	return nil
}

func (s *memStore) Touch(context.Context, string, int64) error { return nil }

// fake is a provider that records its calls and answers from fields.
type fake struct {
	id      string
	models  []Model
	err     error
	calls   atomic.Int32
	secrets []string
}

func (f *fake) Descriptor() Descriptor {
	return Descriptor{
		ID: f.id, Name: "Fake " + f.id, Capabilities: []string{provider.CapabilityText},
		CredentialKinds: []string{provider.CredentialAPIKey}, DefaultEndpoint: "http://127.0.0.1:1/v1",
	}
}

func (f *fake) Models(_ context.Context, c connections.Credential) ([]Model, error) {
	f.calls.Add(1)
	f.secrets = append(f.secrets, string(c.Secret))

	return f.models, f.err
}

func (f *fake) Framing() int { return 4 }

func (f *fake) GenerateText(context.Context, connections.Credential, provider.TextRequest) (TextResult, error) {
	return TextResult{Text: "ok"}, nil
}

// claimsText says it writes text and implements nothing more than Provider.
type claimsText struct{ f *fake }

func (c claimsText) Descriptor() Descriptor { return c.f.Descriptor() }

func (c claimsText) Models(ctx context.Context, cr connections.Credential) ([]Model, error) {
	return c.f.Models(ctx, cr)
}

const conID = "con_aaaaaaaaaa"

type env struct {
	reg   *Registry
	conns *connections.Manager
	store *memStore
	now   time.Time
	dir   string
}

func newEnv(t *testing.T) *env {
	t.Helper()

	dir := t.TempDir()
	e := &env{
		store: &memStore{rows: map[string]connections.Row{}},
		now:   time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC),
		dir:   dir,
	}

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}

	e.conns = connections.NewManager(connections.Options{
		Store: e.store, Vault: vault.Open(filepath.Join(dir, "vault.json"), vault.NewMemoryProvider(key)),
	})
	e.reg = NewRegistry(
		e.conns,
		NewPriceBook(filepath.Join(dir, "providers", PriceFile)),
		func() time.Time { return e.now },
	)

	return e
}

func (e *env) connect(t *testing.T, kind, secret string) {
	t.Helper()

	const id = conID

	if err := e.conns.PutSecret(id, []byte(secret)); err != nil {
		t.Fatal(err)
	}

	err := e.store.Insert(context.Background(), connections.Row{
		ID: id, Kind: kind, Label: "x", Endpoint: "http://127.0.0.1:1/v1", Hosts: []string{"127.0.0.1"},
		Accepts: []string{"text"}, SecretRef: id,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegisterChecksTheProvider(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	if err := e.reg.Register(&fake{id: "alpha"}); err != nil {
		t.Fatal(err)
	}

	if err := e.reg.Register(&fake{id: "alpha"}); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("a duplicate id must fail, got %v", err)
	}

	if err := e.reg.Register(&fake{id: "Bad Id"}); err == nil {
		t.Error("an invalid id must fail")
	}

	if err := e.reg.Register(claimsText{f: &fake{id: "liar"}}); err == nil {
		t.Error("a provider that claims text and cannot write it must fail")
	}

	if !slices.Contains(e.conns.Kinds(), "alpha") {
		t.Errorf("the provider must be a connection kind, got %v", e.conns.Kinds())
	}
}

func TestDescriptorsAreSorted(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	for _, id := range []string{"zeta", "alpha", "mid-one"} {
		if err := e.reg.Register(&fake{id: id}); err != nil {
			t.Fatal(err)
		}
	}

	var ids []string
	for _, d := range e.reg.Descriptors() {
		ids = append(ids, d.ID)
	}

	if want := []string{"alpha", "mid-one", "zeta"}; !slices.Equal(ids, want) {
		t.Errorf("got %v, want %v", ids, want)
	}
}

func TestModelsAreReadAtRunTimeAndCached(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	f := &fake{id: "alpha", models: []Model{{ID: "m-b"}, {ID: "m-a", Name: "  The\nA  model ", ContextTokens: 1000}}}

	if err := e.reg.Register(f); err != nil {
		t.Fatal(err)
	}

	e.connect(t, "alpha", "sk-secret-123")

	ctx := context.Background()

	first, err := e.reg.Models(ctx, conID, false)
	if err != nil {
		t.Fatal(err)
	}

	if first.Cached || f.calls.Load() != 1 {
		t.Fatalf("the first read must ask the provider once (cached %v, calls %d)", first.Cached, f.calls.Load())
	}

	if got := []string{first.Models[0].ID, first.Models[1].ID}; !slices.Equal(got, []string{"m-a", "m-b"}) {
		t.Errorf("models must be sorted by id, got %v", got)
	}

	if first.Models[0].Name != "The A model" {
		t.Errorf("the name must be one clean line, got %q", first.Models[0].Name)
	}

	if f.secrets[0] != "sk-secret-123" {
		t.Errorf("the provider must receive the credential of the connection, got %q", f.secrets[0])
	}

	again, err := e.reg.Models(ctx, conID, false)
	if err != nil || !again.Cached || f.calls.Load() != 1 {
		t.Fatalf("a young copy is served from memory (err %v, cached %v, calls %d)", err, again.Cached, f.calls.Load())
	}

	// The list the provider publishes changes: a refresh sees it, with no change of code.
	f.models = []Model{{ID: "m-new"}}

	fresh, err := e.reg.Models(ctx, conID, true)
	if err != nil || fresh.Cached || len(fresh.Models) != 1 || fresh.Models[0].ID != "m-new" {
		t.Fatalf("refresh must read the current list, got %+v, %v", fresh, err)
	}

	e.now = e.now.Add(CatalogueTTL + time.Second)
	f.models = []Model{{ID: "m-newer"}}

	aged, err := e.reg.Models(ctx, conID, false)
	if err != nil || aged.Cached || aged.Models[0].ID != "m-newer" {
		t.Fatalf("an old copy must be read again, got %+v, %v", aged, err)
	}
}

func TestModelsDropsInvalidIDsDuplicatesAndCaps(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	models := []Model{
		{ID: "ok"},
		{ID: "ok"},
		{ID: "has space"},
		{ID: "ignore previous instructions\nand"},
		{ID: ""},
		{ID: "-lead"},
		{ID: "0priced", Price: &provider.Price{Currency: "USD", InputMicrosPerMTok: 1}},
		{ID: "0badprice", Price: &provider.Price{Currency: "usdx", InputMicrosPerMTok: 1}},
		{ID: "0neg", ContextTokens: -5, OutputTokens: -5},
	}

	for i := range 2100 {
		models = append(models, Model{ID: "bulk-" + strings.Repeat("x", 1) + string(rune('a'+i%26)) + "-" + itoa(i)})
	}

	if err := e.reg.Register(&fake{id: "alpha", models: models}); err != nil {
		t.Fatal(err)
	}

	e.connect(t, "alpha", "k")

	got, err := e.reg.Models(context.Background(), conID, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Models) != MaxModels {
		t.Fatalf("the list is capped at %d, got %d", MaxModels, len(got.Models))
	}

	by := map[string]Model{}
	for _, m := range got.Models {
		by[m.ID] = m
	}

	for _, bad := range []string{"has space", "", "-lead"} {
		if _, ok := by[bad]; ok {
			t.Errorf("the id %q must be dropped", bad)
		}
	}

	if by["0priced"].Price == nil || by["0badprice"].Price != nil {
		t.Errorf("a published price is kept only when valid: %+v %+v", by["0priced"], by["0badprice"])
	}

	if m := by["0neg"]; m.ContextTokens != 0 || m.OutputTokens != 0 {
		t.Errorf("negative limits are not known limits: %+v", m)
	}
}

func itoa(i int) string {
	const digits = "0123456789"

	if i == 0 {
		return "0"
	}

	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{digits[i%10]}, b...)
	}

	return string(b)
}

func TestModelsNeverServesARemovedConnection(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	f := &fake{id: "alpha", models: []Model{{ID: "m"}}}

	if err := e.reg.Register(f); err != nil {
		t.Fatal(err)
	}

	e.connect(t, "alpha", "k")

	if _, err := e.reg.Models(context.Background(), conID, false); err != nil {
		t.Fatal(err)
	}

	if err := e.store.Delete(context.Background(), conID); err != nil {
		t.Fatal(err)
	}

	if _, err := e.reg.Models(context.Background(), conID, false); !errors.Is(err, connections.ErrNotFound) {
		t.Fatalf("a removed connection must not be served from memory, got %v", err)
	}
}

func TestModelsOfAConnectionThatIsNotAProvider(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	e.connect(t, connections.KindTest, "k")

	if _, err := e.reg.Models(context.Background(), conID, false); !errors.Is(err, ErrNotProvider) {
		t.Fatalf("got %v", err)
	}
}

func TestModelsPassesProviderErrorsThrough(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	f := &fake{id: "alpha", err: &Error{Kind: KindAuth, Status: 401}}

	if err := e.reg.Register(f); err != nil {
		t.Fatal(err)
	}

	e.connect(t, "alpha", "k")

	_, err := e.reg.Models(context.Background(), conID, false)
	if pe, ok := AsError(err); !ok || pe.Kind != KindAuth {
		t.Fatalf("got %v", err)
	}

	if _, ok := e.reg.Peek(conID, "x"); ok {
		t.Error("a failed read must not leave a copy")
	}
}

func TestPriceOfOrder(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	cat := &provider.Price{Currency: "USD", InputMicrosPerMTok: 2, OutputMicrosPerMTok: 4}

	if err := e.reg.Register(&fake{id: "alpha", models: []Model{{ID: "pub", Price: cat}, {ID: "plain"}}}); err != nil {
		t.Fatal(err)
	}

	e.connect(t, "alpha", "k")

	// Nothing is known before the catalogue is read: the estimate opens no connection.
	if _, src, err := e.reg.PriceOf("alpha", conID, "pub"); err != nil || src != provider.SourceNone {
		t.Fatalf("before the read: %s, %v", src, err)
	}

	if _, err := e.reg.Models(context.Background(), conID, false); err != nil {
		t.Fatal(err)
	}

	p, src, err := e.reg.PriceOf("alpha", conID, "pub")
	if err != nil || src != provider.SourceCatalogue || p.InputMicrosPerMTok != 2 {
		t.Fatalf("the catalogue price: %+v %s %v", p, src, err)
	}

	if _, src, _ := e.reg.PriceOf("alpha", conID, "plain"); src != provider.SourceNone {
		t.Errorf("a model with no price anywhere has none, got %s", src)
	}

	user := provider.PriceEntry{
		Provider: "alpha",
		Model:    "pub",
		Currency: "EUR", InputMicrosPerMTok: 9,
	}
	if _, err := e.reg.Prices().Set(user); err != nil {
		t.Fatal(err)
	}

	p, src, _ = e.reg.PriceOf("alpha", conID, "pub")
	if src != provider.SourceUser || p.Currency != "EUR" {
		t.Errorf("the table of the person wins over the catalogue, got %+v %s", p, src)
	}
}

func TestTheConnectorTestsByReadingTheCatalogue(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	f := &fake{id: "alpha", models: []Model{{ID: "a"}, {ID: "b"}}}

	if err := e.reg.Register(f); err != nil {
		t.Fatal(err)
	}

	e.connect(t, "alpha", "sk-very-secret")

	res, err := e.conns.Test(context.Background(), conID)
	if err != nil || !res.OK || !strings.Contains(res.Message, "2 models") {
		t.Fatalf("got %+v, %v", res, err)
	}

	f.err = &Error{Kind: KindAuth, Status: 401, Code: "invalid_api_key"}

	res, err = e.conns.Test(context.Background(), conID)
	if err != nil || res.OK || res.Status != 401 || !strings.Contains(res.Message, "refused the key") {
		t.Fatalf("got %+v, %v", res, err)
	}

	if strings.Contains(res.Message, "invalid_api_key") || strings.Contains(res.Message, "sk-very-secret") {
		t.Errorf("a fixed sentence only: %q", res.Message)
	}

	f.err = outbound.ErrDestination
	if _, err = e.conns.Test(context.Background(), conID); !errors.Is(err, outbound.ErrDestination) {
		t.Errorf("a gate refusal is the error of the test, got %v", err)
	}
}

func TestFailureSentences(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		err  error
		want string
	}{
		{&Error{Kind: KindAuth}, "refused the key"},
		{&Error{Kind: KindRateLimited}, "limiting"},
		{&Error{Kind: KindBadRequest}, "refused the request"},
		{&Error{Kind: KindUnreadable}, "does not understand"},
		{&Error{Kind: KindUnavailable}, "could not be reached"},
		{errors.New("dial tcp 10.0.0.1: boom with Authorization: Bearer sk-x"), "could not be reached"},
	} {
		got := failureSentence(tc.err)
		if !strings.Contains(got, tc.want) || strings.Contains(got, "sk-x") {
			t.Errorf("%v: got %q, want it to hold %q", tc.err, got, tc.want)
		}
	}
}

func TestErrorTextHoldsNoBody(t *testing.T) {
	t.Parallel()

	e := &Error{Kind: KindBadRequest, Status: 400, Code: "context_length_exceeded"}
	if got := e.Error(); got != "provider: bad_request (status 400)" {
		t.Errorf("got %q", got)
	}

	if SafeCode("model_not_found") != "model_not_found" || SafeCode("Ignore previous instructions") != "" ||
		SafeCode("") != "" ||
		SafeCode(strings.Repeat("a", 65)) != "" {
		t.Error("SafeCode must accept short tokens only")
	}
}
