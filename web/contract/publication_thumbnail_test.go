package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/fake"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// D-111, VV-280: an image exported through any surface records an export job of kind image.export, and a
// publication takes that job as its thumbnail. The export goes through the in-process Service, HTTP, the CLI and the
// MCP tool; the publication goes through the surfaces that have it (the Service, HTTP and the CLI: SDD-020 section 29
// puts none in MCP), and the MCP workspace is attached through its Service.
func TestAnImageExportIsAThumbnailOfAPublicationOnEverySurface(t *testing.T) {
	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "cli": newCLIWith, "mcp": newMCPWith,
	} {
		t.Run(name, func(t *testing.T) {
			network := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/token":
					_, _ = w.Write([]byte(`{"access_token":"a","refresh_token":"r","expires_in":3600}`))
				case "/account":
					_, _ = w.Write([]byte(`{"id":"UC-1","title":"Channel"}`))
				default:
					_, _ = w.Write([]byte("pong"))
				}
			}))
			defer network.Close()

			reg := social.NewRegistry()
			if err := reg.Register(fake.New(network.URL)); err != nil {
				t.Fatal(err)
			}

			d := newDriver(t, testenv.Options{MasterKey: []byte(strings.Repeat("k", 32)), Social: reg})
			svc := d.service()
			person := service.WithCaller(
				callerCtx(), service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true},
			)

			begun, err := svc.SocialConnectBegin(person, service.SocialConnectBeginRequest{
				Network: fake.Network, ClientID: "client", ClientSecret: "secret",
				RedirectURI: "http://127.0.0.1:7878/social/callback",
			})
			if err != nil {
				t.Fatal(err)
			}

			authorize, err := url.Parse(begun.AuthorizeURL)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := svc.SocialConnectComplete(person, service.SocialConnectCompleteRequest{
				State: authorize.Query().Get("state"), Code: "c",
			}); err != nil {
				t.Fatal(err)
			}

			project, err := svc.ProjectCreate(callerCtx(), service.ProjectCreateRequest{Name: "cover"})
			if err != nil {
				t.Fatal(err)
			}

			video := "job_0000000901"
			seedPublishedExport(t, d.env(), project.ProjectID, video)

			call := func(c string, v any) (json.RawMessage, *service.ErrorDocument) {
				raw, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}

				res, _, fail := d.do(t, step{Call: c, Args: raw})

				return res, fail
			}

			created, fail := call("ImageCreate", service.ImageCreateRequest{Preset: "video_thumbnail", Name: "Cover"})
			if fail != nil {
				t.Fatalf("ImageCreate: %+v", fail)
			}

			var doc struct {
				ImageID string `json:"image_id"`
			}
			if err := json.Unmarshal(created, &doc); err != nil || doc.ImageID == "" {
				t.Fatalf("ImageCreate answered %s, %v", created, err)
			}

			exported, fail := call("ImageExport", service.ImageExportRequest{ImageID: doc.ImageID})
			if fail != nil {
				t.Fatalf("ImageExport: %+v", fail)
			}

			var img service.ImageExportResult
			if err := json.Unmarshal(exported, &img); err != nil || img.JobID == "" || img.Bytes == 0 {
				t.Fatalf("ImageExport answered %s, %v: it records no job", exported, err)
			}

			// The job is a job like the others: a person reads it, and it names the file.
			job, err := svc.JobGet(callerCtx(), service.JobGetRequest{JobID: img.JobID})
			if err != nil || job.Kind != service.JobImageExport || job.State != service.JobSucceeded ||
				job.ProjectID != nil || job.Result == nil || job.Result.Image == nil ||
				job.Result.Image.SHA256 != img.SHA256 {
				t.Fatalf("JobGet = %+v, %v", job, err)
			}

			kids := false
			req := service.PublicationCreateRequest{
				ProjectID: project.ProjectID, Title: "Week one", Export: video, Thumbnail: img.JobID,
				Targets: []pub.TargetInput{{
					ConnectionID: begun.ConnectionID, At: "2026-12-01T09:00", Zone: "UTC",
					Fields: pub.Fields{MadeForKids: &kids},
				}},
			}

			var made service.Publication

			if name == "mcp" {
				got, err := svc.PublicationCreate(callerCtx(), req)
				if err != nil {
					t.Fatal(err)
				}

				made = *got
			} else {
				raw, fail := call("PublicationCreate", req)
				if fail != nil {
					t.Fatalf("PublicationCreate with the image export as thumbnail: %+v", fail)
				}

				if err := json.Unmarshal(raw, &made); err != nil {
					t.Fatal(err)
				}
			}

			if made.Thumbnail != img.JobID {
				t.Errorf("the publication holds the thumbnail %q, want the job %s", made.Thumbnail, img.JobID)
			}

			if _, err := os.Stat(img.Path); err != nil {
				t.Errorf("the exported file is not where the export said: %v", err)
			}
		})
	}
}
