package remote

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

type seen struct {
	method, path, rawQuery, auth, client, contentType string
	body                                              string
}

// server answers every request with status and body and records the request.
func server(t *testing.T, status int, body string, got *seen) *Client {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*got = seen{
			method: r.Method, path: r.URL.EscapedPath(), rawQuery: r.URL.RawQuery, auth: r.Header.Get("Authorization"),
			client: r.Header.Get(ClientHeader), contentType: r.Header.Get("Content-Type"), body: string(raw),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return New(srv.URL+"/", "tok", srv.Client())
}

func asCLI(label string) context.Context {
	return service.WithCaller(context.Background(), service.Caller{Actor: "local:" + label, Scopes: service.AllScopes})
}

// VV-302: the server refuses a member its route does not have, so a text write sends the members of the
// route it takes: the kind only to the route that makes a text, the base revision only to the one that saves.
func TestTextWriteSendsOnlyTheMembersOfItsRoute(t *testing.T) {
	t.Parallel()

	base := 3

	for _, tc := range []struct{ item, method, path, body string }{
		{"", "POST", "/api/v1/projects/prj_a/texts", `{"title":"T","text_kind":"post","text":"a","dry_run":true}`},
		{"pit_b", "PUT", "/api/v1/projects/prj_a/texts/pit_b", `{"title":"T","text":"a","base_revision":3,"dry_run":true}`},
	} {
		var got seen

		c := server(t, 200, `{}`, &got)

		req := service.TextWriteRequest{ProjectID: "prj_a", ItemID: tc.item, Title: "T", TextKind: "post", Text: "a"}
		req.BaseRevision, req.DryRun = &base, true

		if _, err := c.TextWrite(asCLI("cli"), req); err != nil {
			t.Fatal(err)
		}

		if got.method != tc.method || got.path != tc.path || got.body != tc.body {
			t.Errorf("item %q: got %s %s %s, want %s %s %s",
				tc.item, got.method, got.path, got.body, tc.method, tc.path, tc.body)
		}
	}
}

func TestRequestsMatchTheRoutes(t *testing.T) {
	t.Parallel()

	ctx := asCLI("claude-code")
	op := command.Operation{Op: "track.add", Args: json.RawMessage(`{"kind":"visual"}`)}

	for _, tc := range []struct {
		name                 string
		call                 func(c *Client) error
		method, path, query  string
		body                 string
		reply                string
		wantContentTypeEmpty bool
	}{
		{"list", func(c *Client) error {
			_, err := c.ProjectList(ctx, service.ProjectListRequest{State: "all", Cursor: "c1", Limit: 5})
			return err
		}, "GET", "/api/v1/projects", "cursor=c1&limit=5&state=all", "", `{"projects":[]}`, true},
		{"create", func(c *Client) error {
			_, err := c.ProjectCreate(ctx, service.ProjectCreateRequest{Name: "A", Preset: "9:16", FPS: "30", DryRun: true})
			return err
		}, "POST", "/api/v1/projects", "", `{"name":"A","preset":"9:16","fps":"30","dry_run":true}`, `{"project_id":"prj_a"}`, false},
		{"get", func(c *Client) error {
			_, err := c.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: "prj_a", Detail: "full"})
			return err
		}, "GET", "/api/v1/projects/prj_a", "detail=full", "", `{"project_id":"prj_a"}`, true},
		{"trash", func(c *Client) error {
			_, err := c.ProjectTrash(ctx, service.ProjectTrashRequest{ProjectID: "prj_a", DryRun: true})
			return err
		}, "POST", "/api/v1/projects/prj_a/trash", "", `{"dry_run":true}`, `{"state":"trashed"}`, false},
		{"restore", func(c *Client) error {
			_, err := c.ProjectRestore(ctx, service.ProjectTrashRequest{ProjectID: "prj_a"})
			return err
		}, "POST", "/api/v1/projects/prj_a/restore", "", `{}`, `{"state":"active"}`, false},
		{
			"edit", func(c *Client) error {
				_, err := c.Edit(ctx, service.EditRequest{
					ProjectID: "prj_a", BaseRevision: 0, Operations: []command.Operation{op}, IdempotencyKey: "key-12345", DryRun: true,
				})
				return err
			}, "POST", "/api/v1/projects/prj_a/edit", "",
			`{"base_revision":0,"operations":[{"op":"track.add","args":{"kind":"visual"}}],"idempotency_key":"key-12345","dry_run":true}`,
			`{"revision":1}`, false,
		},
		{"help list", func(c *Client) error {
			_, err := c.CommandHelp(ctx, service.CommandHelpRequest{})
			return err
		}, "GET", "/api/v1/commands", "", "", `{"commands":[]}`, true},
		{"help domain", func(c *Client) error {
			_, err := c.CommandHelp(ctx, service.CommandHelpRequest{Domain: "image"})
			return err
		}, "GET", "/api/v1/commands", "domain=image", "", `{"commands":[]}`, true},
		{"help one", func(c *Client) error {
			_, err := c.CommandHelp(ctx, service.CommandHelpRequest{Name: "item.move"})
			return err
		}, "GET", "/api/v1/commands/item.move", "", "", `{"text":"x"}`, true},
		{"history", func(c *Client) error {
			_, err := c.HistoryList(ctx, service.HistoryListRequest{ProjectID: "prj_a", Actor: "local:ui"})
			return err
		}, "GET", "/api/v1/projects/prj_a/history", "actor=local%3Aui", "", `{"entries":[]}`, true},
		{"undo", func(c *Client) error {
			_, err := c.Undo(ctx, service.UndoRequest{ProjectID: "prj_a"})
			return err
		}, "POST", "/api/v1/projects/prj_a/undo", "", `{}`, `{"kind":"undo"}`, false},
		{"redo", func(c *Client) error {
			_, err := c.Redo(ctx, service.UndoRequest{ProjectID: "prj_a", DryRun: true})
			return err
		}, "POST", "/api/v1/projects/prj_a/redo", "", `{"dry_run":true}`, `{"kind":"redo"}`, false},
		{"lint", func(c *Client) error {
			_, err := c.Lint(ctx, service.LintRequest{ProjectID: "prj_a", Severity: "error", Limit: 25})
			return err
		}, "GET", "/api/v1/projects/prj_a/lint", "limit=25&severity=error", "", `{"errors":0}`, true},
		{"media import", func(c *Client) error {
			_, err := c.MediaImport(ctx, service.MediaImportRequest{
				ProjectID: "prj_a", Files: []service.ImportFile{{Path: "/m/a.mp4"}}, WaitMS: 100,
			})
			return err
		}, "POST", "/api/v1/projects/prj_a/media", "", `{"files":[{"path":"/m/a.mp4"}],"wait_ms":100}`, `{"assets":[]}`, false},
		{"media list", func(c *Client) error {
			_, err := c.MediaList(ctx, service.MediaListRequest{ProjectID: "prj_a"})
			return err
		}, "GET", "/api/v1/projects/prj_a/media", "", "", `{"assets":[]}`, true},
		{"media inspect", func(c *Client) error {
			_, err := c.MediaInspect(ctx, service.MediaInspectRequest{
				ProjectID: "prj_a", AssetID: "ast_1", Sections: []string{"probe", "silences"}, MinSilence: "0.5s",
			})
			return err
		}, "GET", "/api/v1/projects/prj_a/media/ast_1", "min_silence=0.5s&sections=probe%2Csilences", "", `{}`, true},
		{"job start", func(c *Client) error {
			_, err := c.JobStart(ctx, service.JobStartRequest{ProjectID: "prj_a", Kind: "export", Args: []byte(`{"start":"0s"}`)})
			return err
		}, "POST", "/api/v1/projects/prj_a/jobs", "", `{"kind":"export","args":{"start":"0s"}}`, `{"job_id":"job_1"}`, false},
		{"job get", func(c *Client) error {
			_, err := c.JobGet(ctx, service.JobGetRequest{JobID: "job_1", WaitMS: 500})
			return err
		}, "GET", "/api/v1/jobs/job_1", "wait_ms=500", "", `{"job_id":"job_1"}`, true},
		{"job list", func(c *Client) error {
			_, err := c.JobList(ctx, service.JobListRequest{ProjectID: "prj_a", State: "all"})
			return err
		}, "GET", "/api/v1/projects/prj_a/jobs", "state=all", "", `{"jobs":[]}`, true},
		{
			"otio import", func(c *Client) error {
				_, err := c.ProjectOtioImport(ctx, service.ProjectOtioImportRequest{
					ProjectID: "prj_a", TimelineID: "prj_b", BaseRevision: 3, Path: "/m/a.otio", AllowPartial: true,
				})
				return err
			}, "POST", "/api/v1/projects/prj_a/otio", "timeline_id=prj_b",
			`{"base_revision":3,"path":"/m/a.otio","allow_partial":true}`, `{"revision":4}`, false,
		},
		{"escapes the id", func(c *Client) error {
			_, err := c.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: "a/b c"})
			return err
		}, "GET", "/api/v1/projects/a%2Fb%20c", "", "", `{}`, true},
	} {
		var got seen

		c := server(t, 200, tc.reply, &got)

		if err := tc.call(c); err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}

		if got.method != tc.method || got.path != tc.path || got.rawQuery != tc.query || got.body != tc.body {
			t.Errorf("%s: got %+v, want %s %s ?%s %s", tc.name, got, tc.method, tc.path, tc.query, tc.body)
		}

		if got.auth != "Bearer tok" || got.client != "claude-code" {
			t.Errorf("%s: credentials %q, client %q", tc.name, got.auth, got.client)
		}

		if (got.contentType == "") != tc.wantContentTypeEmpty {
			t.Errorf("%s: content type %q", tc.name, got.contentType)
		}
	}
}

func TestNoCallerMeansNoClientHeader(t *testing.T) {
	t.Parallel()

	var got seen

	c := server(t, 200, `{"projects":[]}`, &got)

	if _, err := c.ProjectList(context.Background(), service.ProjectListRequest{}); err != nil {
		t.Fatal(err)
	}

	if got.client != "" {
		t.Fatalf("client header = %q", got.client)
	}
}

func TestResultsDecode(t *testing.T) {
	t.Parallel()

	var got seen

	c := server(t, 201, `{"project_id":"prj_a","name":"A","revision":0,"created":true,"text":"created prj_a"}`, &got)

	res, err := c.ProjectCreate(asCLI("cli"), service.ProjectCreateRequest{Name: "A"})
	if err != nil || res.ProjectID != "prj_a" || !res.Created || res.Text != "created prj_a" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestErrorEnvelopeBecomesAServiceError(t *testing.T) {
	t.Parallel()

	var got seen

	c := server(t, 409, `{"type":"urn:stuarty:error:E_REVISION_CONFLICT","title":"E_REVISION_CONFLICT","status":409,
		"code":"E_REVISION_CONFLICT","message":"behind","hint":"read again","retryable":true,"op_index":2,"path":"operations[2]",
		"details":{"current_revision":5}}`, &got)

	_, err := c.Edit(asCLI("cli"), service.EditRequest{ProjectID: "prj_a"})

	var se *service.Error
	if !errors.As(err, &se) {
		t.Fatalf("err = %#v, want a *service.Error", err)
	}

	sameText := se.Code == command.CodeRevisionConflict && se.Message == "behind" && se.Hint == "read again"
	sameWhere := se.OpIndex != nil && *se.OpIndex == 2 && se.Path == "operations[2]"

	if !sameText || !se.Retryable || !sameWhere || se.Details["current_revision"] != float64(5) {
		t.Fatalf("err = %#v", err)
	}
}

func TestAnswersWithoutTheEnvelopeAreInternalErrors(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"html":       {502, "<html>bad gateway</html>"},
		"empty code": {500, `{"message":"x"}`},
	} {
		var got seen

		c := server(t, tc.status, tc.body, &got)
		_, err := c.ProjectList(asCLI("cli"), service.ProjectListRequest{})

		var se *service.Error
		if !errors.As(err, &se) || se.Code != command.CodeInternal || strings.Contains(se.Message, "gateway") {
			t.Errorf("%s: err = %#v", name, err)
		}
	}

	var got seen

	c := server(t, 200, "not json", &got)

	if _, err := c.ProjectList(asCLI("cli"), service.ProjectListRequest{}); err == nil {
		t.Error("a 200 with a body that is not JSON must fail")
	}
}

func TestAServerThatDoesNotAnswerIsRetryable(t *testing.T) {
	t.Parallel()

	var got seen

	c := server(t, 200, "{}", &got)
	c.base = "http://127.0.0.1:1"

	_, err := c.ProjectList(asCLI("cli"), service.ProjectListRequest{})

	var se *service.Error
	if !errors.As(err, &se) || se.Code != service.CodeWorkspaceLocked || !se.Retryable {
		t.Fatalf("err = %#v", err)
	}

	ctx, cancel := context.WithCancel(asCLI("cli"))
	cancel()

	if _, err := c.ProjectList(ctx, service.ProjectListRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled context must surface as such: %v", err)
	}
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

// A write that timed out after it was sent may have been applied: no blind
// retry. A read is safe to repeat.
func TestATimedOutWriteIsNotRetryable(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "tok", doerFunc(func(r *http.Request) (*http.Response, error) {
		ctx, cancel := context.WithTimeout(r.Context(), 50*time.Millisecond)
		defer cancel()

		return srv.Client().Do(r.WithContext(ctx))
	}))

	_, err := c.JobStart(asCLI("cli"), service.JobStartRequest{ProjectID: "prj_a", Kind: "export"})

	var se *service.Error
	if !errors.As(err, &se) || se.Code != command.CodeInternal || se.Retryable || se.Details["outcome"] != "unknown" {
		t.Fatalf("a timed-out write = %#v", err)
	}

	_, err = c.ProjectList(asCLI("cli"), service.ProjectListRequest{})
	if !errors.As(err, &se) || se.Code != service.CodeWorkspaceLocked || !se.Retryable {
		t.Fatalf("a timed-out read = %#v", err)
	}
}

func TestBadBaseURLIsAnInternalError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	c := New("http://bad host", "tok", srv.Client())

	if _, err := c.ProjectList(asCLI("cli"), service.ProjectListRequest{}); err == nil {
		t.Fatal("a malformed URL must fail")
	}
}

func TestOpenFileIsNotAvailableOverHTTP(t *testing.T) {
	t.Parallel()

	var got seen

	c := server(t, 200, `{}`, &got)

	_, err := c.OpenFile(context.Background(), service.FileRef{Kind: "proxy", ID: "ast_1"})

	var se *service.Error
	if !errors.As(err, &se) || se.Code != command.CodeInternal || got.method != "" {
		t.Fatalf("err = %v, request %+v; want E_INTERNAL and no request", err, got)
	}
}

// VV-049: the server answers an oversized header or body itself, without the envelope.
func TestOversizedRequestsAreE_PAYLOAD_TOO_LARGE(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusRequestEntityTooLarge, http.StatusRequestHeaderFieldsTooLarge} {
		var got seen

		c := server(t, status, "Request Header Fields Too Large", &got)
		_, err := c.ProjectList(asCLI("cli"), service.ProjectListRequest{})

		var se *service.Error
		if !errors.As(err, &se) || se.Code != service.CodePayloadTooLarge {
			t.Errorf("status %d: err = %#v", status, err)
		}
	}
}

func TestDecodeErrorGivesTheHintInToolNames(t *testing.T) {
	t.Parallel()

	raw := `{"code":"E_NOT_FOUND","message":"no such project.","hint":"list the projects with GET /api/v1/projects"}`

	var se *service.Error
	if !errors.As(decodeError(http.StatusNotFound, []byte(raw)), &se) {
		t.Fatal("not a service error")
	}

	if se.Hint != "list the projects with project_list" {
		t.Errorf("hint = %q", se.Hint)
	}
}

// VV-200: the text of a job relayed from an HTTP server names the tool, not the HTTP route.
func TestJobTextGivesTheNextCallInToolNames(t *testing.T) {
	t.Parallel()

	var got seen

	body := `{"job_id":"job_a","kind":"export","state":"running",` +
		`"text":"job_a export running\nstill running: call GET /api/v1/jobs/{id} again"}`
	c := server(t, 202, body, &got)

	res, err := c.JobStart(asCLI("mcp"), service.JobStartRequest{ProjectID: "prj_a", Kind: "export"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(res.Text, "call job_get again") || strings.Contains(res.Text, "/api/v1") {
		t.Errorf("text = %q", res.Text)
	}
}
