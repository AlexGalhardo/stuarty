package contract

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
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

// Acceptance criterion 4 of epic 30: the two operations of image.edit are reachable on HTTP, the CLI and MCP (job_start
// with the kind image.edit), and every surface answers the same job: waiting for a person, with the estimate shown
// first, and no picture or prompt sent before that.
func TestImageModelEdit_EverySurfaceStartsTheSameJobAndSendsNothing(t *testing.T) {
	master := bytes.Repeat([]byte{0x7E}, 32)

	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "mcp": newMCPWith, "cli": newCLIWith,
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32

			fake := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
			defer fake.Close()

			d := newDriver(t, testenv.Options{MasterKey: master, Providers: []providers.Provider{openai.New()}})
			svc := d.service()
			person := service.WithCaller(
				callerCtx(), service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true},
			)

			conn, err := svc.ConnectionCreate(callerCtx(), service.ConnectionCreateRequest{
				Kind: "openai", Label: "OpenAI", Endpoint: fake.URL + "/v1", Accepts: []string{"text", "image"},
				Secret: providerKey,
			})
			if err != nil {
				t.Fatal(err)
			}

			micros := int64(40_000)
			if _, err := svc.ProviderPriceSet(person, service.ProviderPriceSetRequest{
				Provider: "openai", Model: "gpt-image-test", Currency: "USD", Unit: "image", UnitMicros: &micros,
			}); err != nil {
				t.Fatal(err)
			}

			project, err := svc.ProjectCreate(callerCtx(), service.ProjectCreateRequest{Name: "edited"})
			if err != nil {
				t.Fatal(err)
			}

			image, err := svc.ImageCreate(
				callerCtx(),
				service.ImageCreateRequest{Preset: "video_thumbnail", Name: "Thumb"},
			)
			if err != nil {
				t.Fatal(err)
			}

			start := func(args map[string]any) (json.RawMessage, string, *service.ErrorDocument) {
				args["project_id"], args["image_id"] = project.ProjectID, image.ImageID
				args["connection_id"], args["model"] = conn.ID, "gpt-image-test"

				raw, err := json.Marshal(args)
				if err != nil {
					t.Fatal(err)
				}

				return d.do(t, step{Call: "ImageModelEdit", Args: raw})
			}

			// Generating into the document: the estimate is on the job, and a person must approve.
			raw, text, fail := start(map[string]any{"operation": "generate", "prompt": "A red fox in the snow"})
			if fail != nil {
				t.Fatalf("%s ImageModelEdit: %+v", d.name(), fail)
			}

			var job generatedJob
			if err := json.Unmarshal(raw, &job); err != nil {
				t.Fatalf("%s: %v in %s", d.name(), err, raw)
			}

			if job.State != service.JobWaitingApproval || job.ApprovalID == "" || job.Estimate == nil ||
				job.Estimate.Micros != 40_000 || job.Estimate.Currency != "USD" {
				t.Fatalf("%s: job = %s, want waiting_approval with an estimate of 40000 USD micros", d.name(), raw)
			}

			if d.name() != "http" && !strings.Contains(text, "estimated cost") {
				t.Errorf("%s: the text does not show the estimate: %q", d.name(), text)
			}

			req, err := svc.ApprovalGet(person, service.ApprovalRef{ID: job.ApprovalID})
			if err != nil || !strings.Contains(req.Summary, "Only the text below is sent") ||
				!strings.Contains(req.Summary, "A red fox in the snow") {
				t.Fatalf("%s: the approval request = %+v, %v", d.name(), req, err)
			}

			// A removal with no layer is the same refusal everywhere.
			_, _, fail = start(map[string]any{"operation": "remove_background"})
			if fail == nil || fail.Code != "E_VALIDATION" || fail.Path != "args.layer_id" {
				t.Fatalf("%s: remove_background with no layer: %+v", d.name(), fail)
			}

			// Nothing reached the provider: not the prompt, not a picture.
			if calls.Load() != 0 {
				t.Fatalf("%s: the provider was called %d times before a person approved", d.name(), calls.Load())
			}

			for _, out := range []string{string(raw), text} {
				if strings.Contains(out, providerKey) ||
					strings.Contains(out, base64.StdEncoding.EncodeToString([]byte(providerKey))) {
					t.Errorf("%s: the key is in the answer", d.name())
				}
			}
		})
	}
}
