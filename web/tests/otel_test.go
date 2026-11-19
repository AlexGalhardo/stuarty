package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/web/app/owner"
	"github.com/AlexGalhardo/stuarty/web/bootstrap"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/vukemtest"
)

// collector is an OTLP/HTTP endpoint that counts what reaches it.
func collector(t *testing.T) (url string, hits *atomic.Int64) {
	t.Helper()

	hits = &atomic.Int64{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	return srv.URL, hits
}

// ADR-0026 item 4: with the endpoint set, the provider of drivers/otel sends the
// spans of a request to that endpoint when the application stops.
func TestOTelExportsToTheConfiguredEndpoint(t *testing.T) {
	t.Parallel()

	url, hits := collector(t)

	app := vukemtest.New(t, bootstrap.New, vukemtest.Options{
		Env:        map[string]string{owner.EnvOTLPEndpoint: url},
		AppOptions: []vukem.Option{vukem.WithProviders(owner.OTelProvider("test"))},
	})

	app.Client(t).Get("/up").AssertOK()

	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	if hits.Load() == 0 {
		t.Error("the collector received nothing")
	}
}

// With no endpoint the provider builds no exporter, so nothing is dialled even
// when the provider is registered.
func TestOTelWithoutEndpointSendsNothing(t *testing.T) {
	t.Parallel()

	_, hits := collector(t)

	app := vukemtest.New(t, bootstrap.New, vukemtest.Options{
		AppOptions: []vukem.Option{vukem.WithProviders(owner.OTelProvider("test"))},
	})

	app.Client(t).Get("/up").AssertOK()

	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	if hits.Load() != 0 {
		t.Errorf("%d requests reached the collector", hits.Load())
	}
}
