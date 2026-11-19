package contract

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/openai"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// onePixelPNG is a real 1x1 PNG, base64 encoded as OpenAI answers an image.
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/q842iQAAAABJRU5ErkJggg=="

// generatedJob is the part of a generation job its surfaces are judged by.
type generatedJob struct {
	JobID      string `json:"job_id"`
	State      string `json:"state"`
	ApprovalID string `json:"approval_id"`
	Estimate   *struct {
		Micros   int64  `json:"micros"`
		Currency string `json:"currency"`
	} `json:"estimate"`
	Result *struct {
		Generated *struct {
			Provider  string `json:"provider"`
			Model     string `json:"model"`
			Requested int    `json:"requested"`
			Delivered int    `json:"delivered"`
			Items     []struct {
				AssetID string `json:"asset_id"`
				Hash    string `json:"hash"`
				Kind    string `json:"kind"`
			} `json:"items"`
			Cost struct {
				Micros int64 `json:"micros"`
			} `json:"cost"`
		} `json:"generated"`
	} `json:"result"`
}

// Acceptance criterion 1 of epic 10 on every surface (the Service, HTTP, MCP through job_start, the CLI):
// ImageGenerate and VoiceGenerate return a job that shows its estimate and waits for a person; once approved it
// ends with assets whose origin names the provider, the model, the prompt, the cost and the actor.
func TestImageGenerate_EverySurfaceStartsAJobThatStoresAssetsWithTheirOrigin(t *testing.T) {
	testGeneration(t, "ImageGenerate", "gpt-image-test", "image", 80_000, 2, func(
		project, conn string,
	) []byte {
		raw, err := json.Marshal(service.ImageGenerateRequest{
			ProjectID: project, ConnectionID: conn,
			Model: "gpt-image-test", Prompt: "A red fox in the snow", Count: 2,
		})
		if err != nil {
			t.Fatal(err)
		}

		return raw
	}, "A red fox in the snow")
}

func TestVoiceGenerate_EverySurfaceStartsAJobThatStoresTheSpeechWithItsOrigin(t *testing.T) {
	testGeneration(t, "VoiceGenerate", "tts-test", "mchar", 345, 1, func(
		project, conn string,
	) []byte {
		raw, err := json.Marshal(service.VoiceGenerateRequest{
			ProjectID: project, ConnectionID: conn,
			Model: "tts-test", Text: "Welcome to the channel.", Voice: "alloy", Format: "wav",
		})
		if err != nil {
			t.Fatal(err)
		}

		return raw
	}, "Welcome to the channel.")
}

func testGeneration(
	t *testing.T,
	call, model, unit string,
	wantEstimate int64,
	want int,
	request func(project, conn string) []byte,
	prompt string,
) {
	t.Helper()

	master := bytes.Repeat([]byte{0x5C}, 32)

	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "mcp": newMCPWith, "cli": newCLIWith,
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32

			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer "+providerKey {
					http.Error(w, "bad key", http.StatusUnauthorized)

					return
				}

				switch r.URL.Path {
				case "/v1/images/generations":
					calls.Add(1)

					var body struct {
						N int `json:"n"`
					}

					_ = json.NewDecoder(r.Body).Decode(&body)

					base, derr := base64.StdEncoding.DecodeString(onePixelPNG)
					if derr != nil {
						http.Error(w, derr.Error(), http.StatusInternalServerError)

						return
					}

					data := make([]map[string]string, 0, body.N)
					for i := range body.N {
						// Two images are two files: a byte after the end of the PNG tells them apart.
						data = append(data, map[string]string{
							"b64_json": base64.StdEncoding.EncodeToString(append(slices.Clone(base), byte(i))),
						})
					}

					_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
				case "/v1/audio/speech":
					calls.Add(1)

					// The format the person asked for is what comes back: wav is the one every surface can ask for
					// (the format enum of job_start is the one of the export, SDD-016 section 10).
					_, _ = w.Write([]byte("RIFF\x24\x00\x00\x00WAVEfmt fake wav frames"))
				default:
					http.NotFound(w, r)
				}
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

			micros := int64(40_000)
			if unit == "mchar" {
				micros = 15_000_000
			}

			if _, err := svc.ProviderPriceSet(person, service.ProviderPriceSetRequest{
				Provider: "openai", Model: model, Currency: "USD", Unit: unit, UnitMicros: &micros,
			}); err != nil {
				t.Fatal(err)
			}

			project, err := svc.ProjectCreate(callerCtx(), service.ProjectCreateRequest{Name: "generated"})
			if err != nil {
				t.Fatal(err)
			}

			raw, text, fail := d.do(t, step{Call: call, Args: request(project.ProjectID, conn.ID)})
			if fail != nil {
				t.Fatalf("%s %s: %+v", d.name(), call, fail)
			}

			var job generatedJob
			if err := json.Unmarshal(raw, &job); err != nil {
				t.Fatalf("%s: %v in %s", d.name(), err, raw)
			}

			// The estimate is there before anything is asked of the provider.
			if job.State != service.JobWaitingApproval || job.ApprovalID == "" || job.Estimate == nil ||
				job.Estimate.Micros != wantEstimate || job.Estimate.Currency != "USD" {
				t.Fatalf(
					"%s: job = %s, want waiting_approval with an estimate of %d USD micros",
					d.name(),
					raw,
					wantEstimate,
				)
			}

			if d.name() != "http" && !strings.Contains(text, "estimated cost") {
				t.Errorf("%s: the text does not show the estimate: %q", d.name(), text)
			}

			if calls.Load() != 0 {
				t.Fatalf("%s: the provider was called before a person approved", d.name())
			}

			if _, err := svc.ApprovalDecide(person, service.ApprovalDecideRequest{
				ID: job.ApprovalID, Decision: "approve",
			}); err != nil {
				t.Fatal(err)
			}

			done := waitForGenerated(t, d, job.JobID)
			if done.State != service.JobSucceeded || done.Result == nil || done.Result.Generated == nil {
				t.Fatalf("%s: job after approval = %+v", d.name(), done)
			}

			res := done.Result.Generated
			if res.Provider != "openai" || res.Model != model || res.Delivered != want || len(res.Items) != want ||
				res.Cost.Micros != wantEstimate {
				t.Fatalf("%s: result = %+v", d.name(), res)
			}

			// Each file is an asset of the library with its origin: provider, model, prompt, cost and actor.
			for _, it := range res.Items {
				a, aerr := svc.AssetGet(callerCtx(), service.AssetRef{Hash: it.Hash})
				if aerr != nil {
					t.Fatal(aerr)
				}

				o := a.Origin
				if o.Kind != "generated" || o.Provider != "openai" || o.Model != model || o.Prompt != prompt ||
					o.CostMicros == nil || *o.CostMicros <= 0 || a.CreatedBy == "" {
					t.Errorf("%s: origin = %+v, created by %q", d.name(), o, a.CreatedBy)
				}
			}

			if int(calls.Load()) != 1 {
				t.Errorf("%s: provider calls = %d, want 1", d.name(), calls.Load())
			}
		})
	}
}

func waitForGenerated(t *testing.T, d driver, id string) generatedJob {
	t.Helper()

	args := json.RawMessage(`{"job_id":"` + id + `","wait_ms":25000}`)

	var job generatedJob

	for range 4 {
		raw, _, fail := d.do(t, step{Call: "JobGet", Args: args})
		if fail != nil {
			t.Fatalf("%s JobGet: %+v", d.name(), fail)
		}

		job = generatedJob{}
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

// VoiceList answers the same on every surface it has (the Service, HTTP and the CLI; MCP has no tool for it), and
// no surface returns the key.
func TestVoiceList_EverySurfaceNamesTheVoicesAndNoneReturnsTheKey(t *testing.T) {
	master := bytes.Repeat([]byte{0x6D}, 32)

	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "cli": newCLIWith,
	} {
		t.Run(name, func(t *testing.T) {
			d := newDriver(t, testenv.Options{MasterKey: master, Providers: []providers.Provider{openai.New()}})

			conn, err := d.service().ConnectionCreate(callerCtx(), service.ConnectionCreateRequest{
				Kind: "openai", Label: "OpenAI", Endpoint: "https://api.openai.com/v1", Accepts: []string{"text"},
				Secret: providerKey,
			})
			if err != nil {
				t.Fatal(err)
			}

			raw, text, fail := d.do(
				t,
				step{Call: "VoiceList", Args: json.RawMessage(`{"connection_id":"` + conn.ID + `"}`)},
			)
			if fail != nil {
				t.Fatalf("%s VoiceList: %+v", d.name(), fail)
			}

			var res service.VoiceListResult
			if err := json.Unmarshal(raw, &res); err != nil || res.Provider != "openai" || len(res.Voices) < 10 {
				t.Fatalf("%s: got %s (%v)", d.name(), raw, err)
			}

			for _, out := range []string{string(raw), text} {
				if strings.Contains(out, providerKey) {
					t.Errorf("%s: the key is in the answer", d.name())
				}
			}

			ids := map[string]bool{}
			for _, v := range res.Voices {
				ids[v.ID] = true
			}

			if !ids["alloy"] || !ids["nova"] {
				t.Errorf("%s: voices = %v", d.name(), ids)
			}
		})
	}
}
