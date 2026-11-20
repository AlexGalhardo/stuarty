package contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/fake"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// D-093: an agent prepares a publication with job_start (kind publish.upload) and reads the approval queue with
// job_get, on MCP as on HTTP and in the CLI: the same job waiting for a person, the same request, and nothing
// leaves the machine. The refusals of the decision are in TestSecurity_T48 and T49; the last part of this test is
// the one that matters for MCP: no member of job_get or job_start decides.
func TestPublishAndApprovalsAreTheSameOnEverySurface(t *testing.T) {
	master := bytes.Repeat([]byte{0x5C}, 32)
	got := map[string][]string{}

	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "cli": newCLIWith, "mcp": newMCPWith,
	} {
		t.Run(name, func(t *testing.T) {
			var uploads atomic.Int64

			network := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/token":
					_, _ = w.Write([]byte(`{"access_token":"a","refresh_token":"r","expires_in":3600}`))
				case "/account":
					_, _ = w.Write([]byte(`{"id":"UC-1","title":"Channel"}`))
				default:
					uploads.Add(1)
					_, _ = w.Write([]byte("pong"))
				}
			}))
			defer network.Close()

			reg := social.NewRegistry()
			if err := reg.Register(fake.New(network.URL)); err != nil {
				t.Fatal(err)
			}

			d := newDriver(t, testenv.Options{MasterKey: master, Social: reg})
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

			project, err := svc.ProjectCreate(callerCtx(), service.ProjectCreateRequest{Name: "launch"})
			if err != nil {
				t.Fatal(err)
			}

			export := "job_0000000902"
			seedPublishedExport(t, d.env(), project.ProjectID, export)

			beforeAgent := uploads.Load()
			kids := false

			mk := func(call string, v any) step {
				raw, merr := json.Marshal(v)
				if merr != nil {
					t.Fatal(merr)
				}

				return step{Call: call, Args: raw}
			}

			var results []string

			now, _, fail := d.do(t, mk("PublishNow", service.PublishNowRequest{
				ProjectID:    project.ProjectID,
				ConnectionID: begun.ConnectionID, Export: export, Title: "Launch", MadeForKids: &kids,
			}))
			if fail != nil {
				t.Fatalf("%s PublishNow: %+v", d.name(), fail)
			}

			results = append(results, normalize(t, d, now))

			var job service.Job
			if jerr := json.Unmarshal(now, &job); jerr != nil || job.State != service.JobWaitingApproval ||
				job.ApprovalID == "" {
				t.Fatalf(
					"%s: the job = %s (%v), want one waiting for approval with an approval_id",
					d.name(),
					now,
					jerr,
				)
			}

			for _, s := range []step{
				mk("ApprovalList", service.ApprovalListRequest{}),
				mk("ApprovalList", service.ApprovalListRequest{State: "all", ProjectID: project.ProjectID}),
				mk("ApprovalGet", service.ApprovalRef{ID: job.ApprovalID}),
			} {
				raw, _, afail := d.do(t, s)
				if afail != nil {
					t.Fatalf("%s %s: %+v", d.name(), s.Call, afail)
				}

				results = append(results, normalize(t, d, raw))
			}

			// The publication state is the job's: waiting for a person.
			status, _, fail := d.do(t, mk("PublishStatus", service.PublishStatusRequest{JobID: job.JobID}))
			if fail != nil {
				t.Fatalf("%s PublishStatus: %+v", d.name(), fail)
			}

			if !bytes.Contains(status, []byte("waiting_approval")) {
				t.Errorf("%s PublishStatus = %s, want waiting_approval", d.name(), status)
			}

			if n := uploads.Load(); n != beforeAgent {
				t.Errorf("%s: %d requests left the machine before a person approved", d.name(), n-beforeAgent)
			}

			pending, err := svc.ApprovalList(callerCtx(), service.ApprovalListRequest{})
			if err != nil || len(pending.Requests) != 1 || pending.Requests[0].State != "pending" {
				t.Fatalf("%s: pending = %+v, %v", d.name(), pending, err)
			}

			got[name] = results
		})
	}

	base := got["in-process"]
	for name, other := range got {
		if len(other) != len(base) {
			t.Fatalf("%s answered %d calls, in-process %d", name, len(other), len(base))
		}

		for i := range base {
			if base[i] != other[i] {
				t.Errorf("call %d differs between in-process and %s:\n%s\n%s", i, name, base[i], other[i])
			}
		}
	}
}

// D-093: reading the queue and starting a publication on MCP cannot decide. No member of job_get or job_start
// approves, rejects or publishes: the input schema refuses it, the request stays pending and the job stays
// waiting_approval.
func TestSecurity_T50_MCPCannotApproveRejectOrPublish(t *testing.T) {
	d := newMCP(t).(*mcpDriver)
	svc := d.service()

	project, err := svc.ProjectCreate(callerCtx(), service.ProjectCreateRequest{Name: "launch"})
	if err != nil {
		t.Fatal(err)
	}

	for _, guess := range []struct {
		name string
		args map[string]any
	}{
		{"job_get", map[string]any{"approval_id": "apr_0000000001", "decision": "approve"}},
		{"job_get", map[string]any{"approvals": "pending", "decide": "reject"}},
		{"job_get", map[string]any{"approvals": "pending", "approve": true}},
		{"job_start", map[string]any{
			"project_id": project.ProjectID, "kind": "publish.upload", "approve": true,
		}},
		{"job_start", map[string]any{
			"project_id": project.ProjectID, "kind": "publish.upload", "args": map[string]any{"decision": "approve"},
		}},
		{"job_get", map[string]any{"approvals": "approve"}},
		{"approval", map[string]any{"id": "apr_0000000001", "decision": "approve"}},
	} {
		res, cerr := d.cs.CallTool(t.Context(), &sdk.CallToolParams{Name: guess.name, Arguments: guess.args})
		if cerr == nil && (res == nil || !res.IsError) {
			t.Errorf("%s %v answered %+v, want a refusal", guess.name, guess.args, res)
		}
	}

	// A read through job_get is a read: the queue stays as it was.
	for _, args := range []map[string]any{{"approvals": "all"}, {"approvals": "pending", "project_id": project.ProjectID}} {
		res, cerr := d.cs.CallTool(t.Context(), &sdk.CallToolParams{Name: "job_get", Arguments: args})
		if cerr != nil || res.IsError {
			t.Fatalf("job_get %v: %v %+v", args, cerr, res)
		}
	}

	rows, err := svc.ApprovalList(callerCtx(), service.ApprovalListRequest{State: "all"})
	if err != nil || len(rows.Requests) != 0 {
		t.Errorf("the queue = %+v, %v; want it untouched", rows, err)
	}
}
