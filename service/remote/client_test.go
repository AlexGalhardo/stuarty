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

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
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

	c := server(t, 409, `{"type":"urn:vukem-video:error:E_REVISION_CONFLICT","title":"E_REVISION_CONFLICT","status":409,
		"code":"E_REVISION_CONFLICT","message":"behind","hint":"read again","retryable":true,"op_index":2,"path":"operations[2]",
		"details":{"current_revision":5}}`, &got)

	_, err := c.Edit(asCLI("cli"), service.EditRequest{ProjectID: "prj_a"})

	var se *service.Error
	if !errors.As(err, &se) || se.Code != command.CodeRevisionConflict || se.Message != "behind" || se.Hint != "read again" ||
		!se.Retryable || se.OpIndex == nil ||
		*se.OpIndex != 2 ||
		se.Path != "operations[2]" ||
		se.Details["current_revision"] != float64(5) {
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

func TestBadBaseURLIsAnInternalError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	c := New("http://bad host", "tok", srv.Client())

	if _, err := c.ProjectList(asCLI("cli"), service.ProjectListRequest{}); err == nil {
		t.Fatal("a malformed URL must fail")
	}
}
