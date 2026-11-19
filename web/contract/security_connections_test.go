package contract

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

const connectionCanary = "sk-live-CANARY-T43-0f1e2d3c4b5a"

// SDD-007 T-43, SDD-014 section 10 (RF-137): no operation returns a credential, on any surface. A
// destination that echoes the credential it was sent is the worst case: what ConnectionTest returns
// is redacted, and neither the secret nor the master key appears in an answer of the Service, the
// HTTP API, the MCP tool or the CLI, successes and failures alike.
func TestSecurity_T43_NoSurfaceReturnsASecret(t *testing.T) {
	master := bytes.Repeat([]byte{0x5C}, 32)

	forbidden := []string{
		connectionCanary,
		base64.StdEncoding.EncodeToString([]byte(connectionCanary)),
		base64.StdEncoding.EncodeToString(master),
		base64.RawStdEncoding.EncodeToString(master),
		hex.EncodeToString(master),
	}

	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The credential comes back in the body, as a careless API would do.
		fmt.Fprintf(w, "you sent %s", r.Header.Get("Authorization"))
	}))
	defer echo.Close()

	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "mcp": newMCPWith, "cli": newCLIWith,
	} {
		t.Run(name, func(t *testing.T) {
			d := newDriver(t, testenv.Options{MasterKey: master})

			created, err := d.service().ConnectionCreate(callerCtx(), service.ConnectionCreateRequest{
				Kind: "test", Label: "Echo", Endpoint: echo.URL, Accepts: []string{"text"}, Secret: connectionCanary,
			})
			if err != nil {
				t.Fatal(err)
			}

			steps := []step{
				{Call: "ConnectionList", Args: json.RawMessage(`{}`)},
				{Call: "ConnectionGet", Args: json.RawMessage(`{"id":"` + created.ID + `"}`)},
				{Call: "ConnectionGet", Args: json.RawMessage(`{"id":"con_0000000000"}`)},
			}

			// The MCP tool only reads: testing is for a person (SDD-014 section 10).
			if name != "mcp" {
				steps = append(
					steps,
					step{Call: "ConnectionTest", Args: json.RawMessage(`{"id":"` + created.ID + `"}`)},
				)
			}

			var seen strings.Builder

			for _, s := range steps {
				result, text, fail := d.do(t, s)

				seen.Write(result)
				seen.WriteString(text)

				if fail != nil {
					raw, _ := json.Marshal(fail)
					seen.Write(raw)
				}
			}

			for _, secret := range forbidden {
				if strings.Contains(seen.String(), secret) {
					t.Errorf("an answer carries %q", secret)
				}
			}

			if name != "mcp" && !strings.Contains(seen.String(), "[redacted]") {
				t.Error("the echoed credential was not redacted: the test did not reach the destination")
			}

			if !strings.Contains(seen.String(), created.ID) {
				t.Error("the answers do not even name the connection: the test saw nothing")
			}
		})
	}
}
