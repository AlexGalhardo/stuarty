package providers_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
)

// Acceptance criterion 6 of epic 09: no test of the layer can reach the network. TestMain refuses every
// dial that is not to loopback; a request to a public address (a documentation address, TEST-NET-3, which
// nothing answers) fails at the recorder, before any socket is made.
func TestNoTestOfTheLayerCanReachTheNetwork(t *testing.T) {
	t.Parallel()

	h := connectionstest.New(t, "openai", "https://203.0.113.7/v1", "sk-test-0123456789")

	h.Do(t, func(ctx context.Context, c connections.Credential) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Connection.Endpoint+"/models", http.NoBody)
		if err != nil {
			t.Fatal(err)
		}

		res, err := c.Client.Do(req)
		if err == nil {
			_ = res.Body.Close()

			t.Fatal("a request to a public address was made: no test may reach the network")
		}

		if !strings.Contains(err.Error(), "no test of the provider layer may reach the network") {
			t.Fatalf("the request failed for another reason than the guard: %v", err)
		}
	})
}
