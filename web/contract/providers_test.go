package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/openai"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

const providerKey = "sk-live-CANARY-E09-7c1d2e3f4a5b"

// textJob is the part of a job a generation is judged by, as every surface answers it.
type textJob struct {
	JobID      string `json:"job_id"`
	State      string `json:"state"`
	ApprovalID string `json:"approval_id"`
	Estimate   *struct {
		Micros   int64  `json:"micros"`
		Currency string `json:"currency"`
	} `json:"estimate"`
	Result *struct {
		Text *struct {
			Text         string `json:"text"`
			InputTokens  int    `json:"input_tokens"`
			OutputTokens int    `json:"output_tokens"`
			Cost         struct {
				Micros int64 `json:"micros"`
			} `json:"cost"`
		} `json:"text"`
	} `json:"result"`
}

// Acceptance criterion 1 of epic 09: with a provider connected by API key, TextGenerate returns a job on
// every surface (the Service, HTTP, MCP through job_start and the CLI); it waits for a person with its
// estimate, and once a person approves it the job completes and stores the text and the real cost.
func TestTextGenerate_EverySurfaceStartsAJobThatCompletesAndStoresTheText(t *testing.T) {
	master := bytes.Repeat([]byte{0x3A}, 32)

	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "mcp": newMCPWith, "cli": newCLIWith,
	} {
		t.Run(name, func(t *testing.T) {
			var chats atomic.Int32

			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" {
					http.NotFound(w, r)

					return
				}

				chats.Add(1)

				if got := r.Header.Get("Authorization"); got != "Bearer "+providerKey {
					http.Error(w, "bad key", http.StatusUnauthorized)

					return
				}

				fmt.Fprint(
					w,
					`{"model":"gpt-test","choices":[{"message":{"content":"Sourdough, from starter to slice."},`+
						`"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":20}}`,
				)
			}))
			defer fake.Close()

			d := newDriver(t, testenv.Options{MasterKey: master, Providers: []providers.Provider{openai.New()}})
			svc := d.service()
			person := service.WithCaller(
				callerCtx(), service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true},
			)

			conn, err := svc.ConnectionCreate(callerCtx(), service.ConnectionCreateRequest{
				Kind:     "openai",
				Label:    "OpenAI",
				Endpoint: fake.URL + "/v1",
				Accepts:  []string{"text"},
				Secret:   providerKey,
			})
			if err != nil {
				t.Fatal(err)
			}

			one, two := int64(1_000_000), int64(2_000_000)
			if _, err := svc.ProviderPriceSet(person, service.ProviderPriceSetRequest{
				Provider:            "openai",
				Model:               "gpt-test",
				Currency:            "USD",
				InputMicrosPerMTok:  &one,
				OutputMicrosPerMTok: &two,
			}); err != nil {
				t.Fatal(err)
			}

			project, err := svc.ProjectCreate(callerCtx(), service.ProjectCreateRequest{Name: "text"})
			if err != nil {
				t.Fatal(err)
			}

			args, err := json.Marshal(service.TextGenerateRequest{
				ProjectID:    project.ProjectID,
				ConnectionID: conn.ID,
				Model:        "gpt-test",
				Prompt:       "Caption a video about sourdough.",
				MaxTokens:    100,
			})
			if err != nil {
				t.Fatal(err)
			}

			raw, text, fail := d.do(t, step{Call: "TextGenerate", Args: args})
			if fail != nil {
				t.Fatalf("%s TextGenerate: %+v", d.name(), fail)
			}

			var job textJob
			if err := json.Unmarshal(raw, &job); err != nil {
				t.Fatalf("%s: %v in %s", d.name(), err, raw)
			}

			// The estimate is there before anything runs: 27 tokens in at 1 and 100 out at 2 per million.
			if job.State != service.JobWaitingApproval || job.ApprovalID == "" || job.Estimate == nil ||
				job.Estimate.Micros != 227 || job.Estimate.Currency != "USD" {
				t.Fatalf("%s: job = %s, want waiting_approval with an estimate of 227 USD micros", d.name(), raw)
			}

			if !strings.Contains(text, "estimated cost 0.000227 USD") && d.name() != "http" {
				t.Errorf("%s: the text does not show the estimate: %q", d.name(), text)
			}

			if chats.Load() != 0 {
				t.Fatalf("%s: the provider was called before a person approved", d.name())
			}

			if _, err := svc.ApprovalDecide(person, service.ApprovalDecideRequest{
				ID: job.ApprovalID, Decision: "approve",
			}); err != nil {
				t.Fatal(err)
			}

			done := waitForJob(t, d, job.JobID)

			// 10 tokens in at 1 and 20 out at 2 per million: 50 millionths.
			if done.State != service.JobSucceeded || done.Result == nil || done.Result.Text == nil ||
				done.Result.Text.Text != "Sourdough, from starter to slice." || done.Result.Text.Cost.Micros != 50 ||
				done.Result.Text.InputTokens != 10 || done.Result.Text.OutputTokens != 20 {
				t.Fatalf("%s: job after approval = %+v", d.name(), done)
			}

			if chats.Load() != 1 {
				t.Errorf("%s: provider calls = %d, want 1", d.name(), chats.Load())
			}
		})
	}
}

// waitForJob reads the job through the surface of the driver until it is final: each read waits on the
// server for a final state (wait_ms), so the test never sleeps.
func waitForJob(t *testing.T, d driver, id string) textJob {
	t.Helper()

	args := json.RawMessage(`{"job_id":"` + id + `","wait_ms":25000}`)

	var job textJob

	for range 4 {
		raw, _, fail := d.do(t, step{Call: "JobGet", Args: args})
		if fail != nil {
			t.Fatalf("%s JobGet: %+v", d.name(), fail)
		}

		job = textJob{}
		if err := json.Unmarshal(raw, &job); err != nil {
			t.Fatal(err)
		}

		if service.JobFinal(job.State) {
			return job
		}
	}

	t.Fatalf("%s: the job %s did not finish: %+v", d.name(), id, job)

	return job
}

// Acceptance criterion 5 of epic 09 on every surface: the models and the providers come back with no key,
// and setting a price is for a person.
func TestTextGenerate_NoSurfaceReturnsTheKeyOrLetsAnAgentSetAPrice(t *testing.T) {
	master := bytes.Repeat([]byte{0x4B}, 32)

	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "cli": newCLIWith,
	} {
		t.Run(name, func(t *testing.T) {
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// A careless provider that repeats the key in its model list.
				fmt.Fprintf(
					w,
					`{"data":[{"id":"gpt-test"},{"id":"echo-%s"}]}`,
					strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "),
				)
			}))
			defer fake.Close()

			d := newDriver(t, testenv.Options{MasterKey: master, Providers: []providers.Provider{openai.New()}})

			conn, err := d.service().ConnectionCreate(callerCtx(), service.ConnectionCreateRequest{
				Kind:     "openai",
				Label:    "OpenAI",
				Endpoint: fake.URL + "/v1",
				Accepts:  []string{"text"},
				Secret:   providerKey,
			})
			if err != nil {
				t.Fatal(err)
			}

			list, _, fail := d.do(t, step{Call: "ProviderList", Args: json.RawMessage(`{}`)})
			if fail != nil {
				t.Fatalf("ProviderList: %+v", fail)
			}

			models, _, fail := d.do(
				t,
				step{Call: "ProviderModels", Args: json.RawMessage(`{"connection_id":"` + conn.ID + `"}`)},
			)
			if fail != nil {
				t.Fatalf("ProviderModels: %+v", fail)
			}

			for what, body := range map[string]string{"ProviderList": string(list), "ProviderModels": string(models)} {
				if strings.Contains(body, providerKey) {
					t.Errorf("%s answered with the key: %s", what, body)
				}
			}

			if !strings.Contains(string(list), `"id":"openai"`) || !strings.Contains(string(list), conn.ID) {
				t.Errorf("ProviderList = %s", list)
			}

			// The local token an agent holds cannot set a price: one stable code, with its hint.
			_, _, fail = d.do(t, step{Call: "ProviderPriceSet", Args: json.RawMessage(
				`{"provider":"openai","model":"gpt-test","currency":"USD","input_micros_per_mtok":0,"output_micros_per_mtok":0}`,
			)})
			if fail == nil || fail.Code != service.CodePrivilegedSession || fail.Hint == "" {
				t.Errorf("ProviderPriceSet with the local token: %+v, want E_PRIVILEGED_SESSION with a hint", fail)
			}

			if prices, err := d.service().
				ProviderList(callerCtx(), service.ProviderListRequest{}); err != nil ||
				len(prices.Prices) != 0 {
				t.Errorf("a refused price was written: %+v, %v", prices, err)
			}
		})
	}
}
