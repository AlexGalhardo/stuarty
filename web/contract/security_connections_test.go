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

			// The MCP tool only reads: creating, testing and removing are for a person (SDD-014 section
			// 10). On the other surfaces the answers of a create, refused and accepted, and of a removal
			// must not carry the secret either: a validation error never echoes the request.
			if name != "mcp" {
				create := func(endpoint string) json.RawMessage {
					raw, err := json.Marshal(service.ConnectionCreateRequest{
						Kind: "test", Label: "Echo", Endpoint: endpoint, Accepts: []string{"text"}, Secret: connectionCanary,
					})
					if err != nil {
						t.Fatal(err)
					}

					return raw
				}

				steps = append(
					steps,
					step{Call: "ConnectionTest", Args: json.RawMessage(`{"id":"` + created.ID + `"}`)},
					step{Call: "ConnectionCreate", Args: create("http://example.com/plain")},
					step{Call: "ConnectionCreate", Args: create(echo.URL)},
					step{Call: "ConnectionRemove", Args: json.RawMessage(`{"id":"` + created.ID + `"}`)},
				)
			}

			var seen strings.Builder

			creates := 0

			for _, s := range steps {
				result, text, fail := d.do(t, s)

				seen.Write(result)
				seen.WriteString(text)

				if fail != nil {
					raw, _ := json.Marshal(fail)
					seen.Write(raw)
				}

				// The first create is refused (plain http to a public host), the second accepted, and
				// the removal goes through: each answer above was a real one.
				switch {
				case s.Call == "ConnectionCreate" && creates == 0:
					creates++

					if fail == nil || fail.Code != "E_VALIDATION" {
						t.Errorf("a create with a plain http endpoint: %+v, want E_VALIDATION", fail)
					}
				case s.Call == "ConnectionCreate", s.Call == "ConnectionRemove":
					if fail != nil {
						t.Errorf("%s: %+v", s.Call, fail)
					}
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
