package contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/fake"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// publishFixture is what an agent needs to ask for a publication: a connected fake network, a project and a
// succeeded export. uploads counts the requests that reached the network other than the sign-in.
type publishFixture struct {
	d          driver
	connection string
	project    string
	export     string
	uploads    *atomic.Int64
}

func newPublishFixture(t *testing.T, newDriver func(*testing.T, testenv.Options) driver) publishFixture {
	t.Helper()

	uploads := new(atomic.Int64)

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
	t.Cleanup(network.Close)

	reg := social.NewRegistry()
	if err := reg.Register(fake.New(network.URL)); err != nil {
		t.Fatal(err)
	}

	d := newDriver(t, testenv.Options{MasterKey: bytes.Repeat([]byte{0x5C}, 32), Social: reg})
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

	return publishFixture{
		d: d, connection: begun.ConnectionID, project: project.ProjectID, export: export, uploads: uploads,
	}
}

// D-093: an agent prepares a publication with job_start (kind publish.upload) and reads the approval queue with
// job_get, on MCP as on HTTP and in the CLI: the same job waiting for a person, the same request, and nothing
// leaves the machine. That no member of job_get or job_start decides is the test of T-48 below.
func TestPublishAndApprovalsAreTheSameOnEverySurface(t *testing.T) {
	got := map[string][]string{}

	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "cli": newCLIWith, "mcp": newMCPWith,
	} {
		t.Run(name, func(t *testing.T) {
			fx := newPublishFixture(t, newDriver)
			d, svc := fx.d, fx.d.service()
			beforeAgent := fx.uploads.Load()
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
				ProjectID:    fx.project,
				ConnectionID: fx.connection, Export: fx.export, Title: "Launch", MadeForKids: &kids,
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
				mk("ApprovalList", service.ApprovalListRequest{State: "all", ProjectID: fx.project}),
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

			if n := fx.uploads.Load(); n != beforeAgent {
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

// D-093: reading the queue and starting a publication on MCP cannot decide. With a real request waiting for a
// person, no tool and no member of job_get or job_start approves, rejects or publishes it: the input schemas
// refuse every guess, the reads change nothing, the request stays pending, the job stays waiting_approval and
// nothing reaches the network.
func TestSecurity_T48_MCPCannotApproveRejectOrPublish(t *testing.T) {
	fx := newPublishFixture(t, newMCPWith)
	d, ok := fx.d.(*mcpDriver)
	if !ok {
		t.Fatalf("the driver is %T, want *mcpDriver", fx.d)
	}

	svc := d.service()

	call := func(name string, args map[string]any) (*sdk.CallToolResult, error) {
		return d.cs.CallTool(t.Context(), &sdk.CallToolParams{Name: name, Arguments: args})
	}

	post := map[string]any{
		"connection_id": fx.connection, "export": fx.export, "title": "Launch", "made_for_kids": false,
	}

	started, err := call("job_start", map[string]any{"project_id": fx.project, "kind": "publish.upload", "args": post})
	if err != nil || started.IsError {
		t.Fatalf("job_start publish.upload: %v %+v", err, started)
	}

	raw, err := json.Marshal(started.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}

	var job service.Job
	if err := json.Unmarshal(raw, &job); err != nil || job.State != service.JobWaitingApproval ||
		job.ApprovalID == "" {
		t.Fatalf("job_start answered %s (%v), want a job waiting for approval with an approval_id", raw, err)
	}

	before := fx.uploads.Load()

	// No tool of the list decides, and no input schema has a member that would.
	list, err := d.cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, tool := range list.Tools {
		if strings.Contains(tool.Name, "approv") || strings.Contains(tool.Name, "decide") {
			t.Errorf("the tool %s reaches the approval queue by its name", tool.Name)
		}

		schema, merr := json.Marshal(tool.InputSchema)
		if merr != nil {
			t.Fatal(merr)
		}

		var in struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if uerr := json.Unmarshal(schema, &in); uerr != nil {
			t.Fatal(uerr)
		}

		for _, member := range []string{"decision", "decide", "approve", "reject", "rule", "ceiling"} {
			if _, has := in.Properties[member]; has {
				t.Errorf("the tool %s has the member %s", tool.Name, member)
			}
		}
	}

	for _, guess := range []struct {
		name string
		args map[string]any
	}{
		{"job_get", map[string]any{"approval_id": job.ApprovalID, "decision": "approve"}},
		{"job_get", map[string]any{"approval_id": job.ApprovalID, "approve": true}},
		{"job_get", map[string]any{"approvals": "pending", "decide": "reject"}},
		{"job_get", map[string]any{"approvals": "approve"}},
		{"job_get", map[string]any{"job_id": job.JobID, "state": "approved"}},
		{"job_get", map[string]any{"job_id": job.JobID, "approval_id": job.ApprovalID}},
		{"job_start", map[string]any{
			"project_id": fx.project, "kind": "publish.upload", "args": post, "approve": true,
		}},
		{"job_start", map[string]any{
			"project_id": fx.project, "kind": "approval.decide",
			"args": map[string]any{"id": job.ApprovalID, "decision": "approve"},
		}},
		{"approval", map[string]any{"id": job.ApprovalID, "decision": "approve"}},
		{"approval_decide", map[string]any{"id": job.ApprovalID, "decision": "approve"}},
	} {
		res, cerr := call(guess.name, guess.args)
		if cerr == nil && (res == nil || !res.IsError) {
			t.Errorf("%s %v answered %+v, want a refusal", guess.name, guess.args, res)
		}
	}

	// The reads are reads: they name the request and leave it as it was.
	for _, args := range []map[string]any{
		{"approval_id": job.ApprovalID},
		{"approvals": "all"},
		{"approvals": "pending", "project_id": fx.project},
		{"job_id": job.JobID},
	} {
		res, cerr := call("job_get", args)
		if cerr != nil || res.IsError {
			t.Fatalf("job_get %v: %v %+v", args, cerr, res)
		}

		out, merr := json.Marshal(res.StructuredContent)
		if merr != nil {
			t.Fatal(merr)
		}

		if !bytes.Contains(out, []byte(job.ApprovalID)) {
			t.Errorf("job_get %v = %s, want the request %s", args, out, job.ApprovalID)
		}
	}

	req, err := svc.ApprovalGet(callerCtx(), service.ApprovalRef{ID: job.ApprovalID})
	if err != nil || req.State != "pending" || req.DecidedBy != nil {
		t.Errorf("the request = %+v, %v; want it pending and undecided", req, err)
	}

	now, err := svc.JobGet(callerCtx(), service.JobGetRequest{JobID: job.JobID})
	if err != nil || now.State != service.JobWaitingApproval {
		t.Errorf("the job = %+v, %v; want it waiting for approval", now, err)
	}

	if n := fx.uploads.Load(); n != before {
		t.Errorf("%d requests left the machine with no person", n-before)
	}
}
