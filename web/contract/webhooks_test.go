package contract

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// Epic 59 (SDD-022): the webhook operations answer the same on every surface, none of the reads carries the signing
// secret, and the MCP tool shows the host of an address and never its path or its query, where a receiver keeps its
// token.
func TestWebhookSurfacesAgreeAndKeepTheSecretOnce(t *testing.T) {
	master := bytes.Repeat([]byte{0x5D}, 32)

	var hits atomic.Int32

	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)

		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()

	const pathToken = "PATH-TOKEN-CANARY-59"

	address := receiver.URL + "/" + pathToken + "?key=" + pathToken

	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "mcp": newMCPWith, "cli": newCLIWith,
	} {
		t.Run(name, func(t *testing.T) {
			d := newDriver(t, testenv.Options{MasterKey: master})

			created, err := d.service().WebhookCreate(callerCtx(), service.WebhookCreateRequest{
				Name: "n8n", URL: address, Events: []string{"export.ready", "job.failed"},
			})
			if err != nil {
				t.Fatal(err)
			}

			id := `{"id":"` + created.ID + `"}`
			steps := []step{
				{Call: "WebhookList", Args: json.RawMessage(`{}`)},
			}

			// The tool only reads and lists; the other surfaces carry every operation.
			if name != "mcp" {
				steps = append(
					steps,
					step{Call: "WebhookEventList", Args: json.RawMessage(`{}`)},
					step{Call: "WebhookGet", Args: json.RawMessage(id)},
					step{Call: "WebhookGet", Args: json.RawMessage(`{"id":"whk_0000000000"}`)},
					step{
						Call: "WebhookUpdate",
						Args: json.RawMessage(`{"id":"` + created.ID + `","name":"zapier","enabled":false}`),
					},
					step{
						Call: "WebhookUpdate",
						Args: json.RawMessage(`{"id":"` + created.ID + `","url":"https://10.0.0.5/h"}`),
					},
					step{Call: "WebhookTest", Args: json.RawMessage(id)},
					step{Call: "WebhookDeliveryList", Args: json.RawMessage(id)},
					step{Call: "WebhookRemove", Args: json.RawMessage(id)},
				)
			}

			var seen strings.Builder

			refused := 0

			for _, s := range steps {
				result, text, fail := d.do(t, s)

				seen.Write(result)
				seen.WriteString(text)

				if fail != nil {
					raw, _ := json.Marshal(fail)
					seen.Write(raw)
				}

				switch {
				case s.Call == "WebhookGet" && strings.Contains(string(s.Args), "whk_0000000000"):
					if fail == nil || fail.Code != service.CodeWebhookNotFound {
						t.Errorf("an unknown webhook: %+v, want E_WEBHOOK_NOT_FOUND", fail)
					}
				case s.Call == "WebhookUpdate" && strings.Contains(string(s.Args), "10.0.0.5"):
					refused++

					if fail == nil || fail.Code != "E_VALIDATION" || fail.Path != "url" {
						t.Errorf("an update to a private address: %+v, want E_VALIDATION on url", fail)
					}
				case fail != nil:
					t.Errorf("%s: %+v", s.Call, fail)
				}
			}

			if strings.Contains(seen.String(), created.Secret) {
				t.Error("an answer carries the signing secret")
			}

			if name == "mcp" {
				if strings.Contains(seen.String(), pathToken) {
					t.Error("the MCP tool shows the path or the query of the address")
				}

				if !strings.Contains(seen.String(), "127.0.0.1") {
					t.Error("the MCP tool does not show the host")
				}
			} else if refused != 1 || hits.Load() == 0 {
				t.Errorf("refused %d, the test send reached the receiver %d times", refused, hits.Load())
			}

			if !strings.Contains(seen.String(), created.ID) {
				t.Error("the answers do not even name the webhook: the test saw nothing")
			}
		})
	}
}
