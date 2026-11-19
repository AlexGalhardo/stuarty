package servicetest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/pathguard"
)

// leakMarkers are fragments that only a path, a SQL statement, a stack trace or
// the stderr of a child could put into an error.
var leakMarkers = []string{
	"SELECT ", "INSERT ", "sqlite", "sql:", "goroutine ", ".go:", "panic", "ffmpeg version", "Invalid data found",
	"moov atom", `\\Users\\`, "/home/", "/etc/", "stderr",
}

func assertNoLeak(t *testing.T, label string, v any, extra ...string) {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}

	text := string(b)

	for _, m := range slices.Concat(leakMarkers, extra) {
		if m != "" && strings.Contains(text, m) {
			t.Errorf("%s: the error holds %q: %s", label, m, text)
		}
	}
}

// T-29: an engine failure that carries stderr, a path, SQL or a stack trace
// reaches the caller as a code, a fixed message and a fixed reason, never as
// the text. An unknown error is E_INTERNAL without its text.
func TestSecurity_T29_ErrorsLeakNothing(t *testing.T) {
	stderr := "ffmpeg version n7.1 Invalid data found when processing input C:\\Users\\alice\\clip-" +
		runtimeCanary(t, "stderr") + ".mp4: moov atom not found"
	fixed := []string{
		ingest.ReasonDecodeFailed, ingest.ReasonAudioDecodeFailed, ingest.ReasonVerificationFailed,
		ingest.ReasonTruncated, ingest.ReasonTimeout, "interrupted",
	}

	failures := map[string]error{
		"typed with stderr": &ffmpeg.Error{
			Code: ingest.CodeIngestFailed, Reason: ingest.ReasonDecodeFailed,
			Message: "The video could not be decoded.", Err: errors.New(stderr),
		},
		"sql": errors.New("sql: SELECT * FROM assets WHERE hash = '" + stderr + "': sqlite: database is locked"),
		"path": &os.PathError{
			Op:   "open",
			Path: filepath.Join(t.TempDir(), "secret", "original-"+runtimeCanary(t, "p")),
			Err:  os.ErrPermission,
		},
		"stack": errors.New(
			"panic: boom\n\ngoroutine 1 [running]:\nmain.main()\n\t/home/runner/work/app/main.go:12 +0x1d",
		),
	}

	for name, cause := range failures {
		m := newMediaEnv(t, mediaCfg{
			noQueue: true,
			ingest:  func(context.Context, ingest.Input) (*ingest.Result, error) { return nil, cause },
		})
		project := m.newProject("p")
		a := m.importOne(project, m.write("c.mp4", clipBytes)).Assets[0]

		_ = m.runner.RunJob(context.Background(), a.JobID)

		j := m.job(a.JobID)
		if j.State != service.JobFailed || j.Error == nil {
			t.Fatalf("%s: job = %+v", name, j)
		}

		assertNoLeak(t, name+": job", j, stderr, "alice")

		list, err := m.svc.MediaList(m.as(ui), service.MediaListRequest{ProjectID: project})
		if err != nil {
			t.Fatal(err)
		}

		assertNoLeak(t, name+": media list", list, stderr, "alice")

		switch name {
		case "typed with stderr":
			reason, _ := j.Error.Details["reason"].(string)
			if j.Error.Code != service.CodeIngestFailed || !slices.Contains(fixed, reason) {
				t.Errorf(
					"%s: %s with reason %q; want E_INGEST_FAILED and a reason of the fixed set",
					name,
					j.Error.Code,
					reason,
				)
			}
		default:
			if j.Error.Code != command.CodeInternal {
				t.Errorf("%s: code %s, want E_INTERNAL", name, j.Error.Code)
			}
		}
	}

	// Errors of plain calls: none quotes a path or SQL of the server.
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)
	project := m.newProject("p")

	var calls []error

	_, err := m.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: "prj_9999999999"})
	calls = append(calls, err)

	_, err = m.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: "x'; DROP TABLE projects; --"})
	calls = append(calls, err)

	_, err = m.svc.JobGet(ctx, service.JobGetRequest{JobID: "job_9999999999"})
	calls = append(calls, err)

	_, err = m.svc.OpenFile(ctx, service.FileRef{Kind: "proxy", ProjectID: project, ID: "ast_9999999999"})
	calls = append(calls, err)

	_, err = m.svc.MediaImport(ctx, service.MediaImportRequest{
		ProjectID: project, Files: []service.ImportFile{{Path: filepath.Join(m.files, "nope", "missing.mp4")}},
	})
	calls = append(calls, err)

	_, err = m.svc.MediaImport(ctx, service.MediaImportRequest{
		ProjectID: project, Files: []service.ImportFile{{Path: "http://169.254.169.254/latest/meta-data"}},
	})
	calls = append(calls, err)

	for i, e := range calls {
		if e == nil {
			t.Errorf("call %d did not fail", i)

			continue
		}

		assertNoLeak(t, "call "+string(rune('a'+i)), e, m.ws, m.files)
	}
}

// T-29, network mode: outside the import roots a missing path, an existing
// file, a directory and a path inside the workspace all give the same answer,
// so a caller learns nothing about the machine.
func TestSecurity_T29_NoExistenceOracleOutsideRoots(t *testing.T) {
	resolved := func(p string) string {
		r, err := filepath.EvalSymlinks(p)
		if err != nil {
			t.Fatal(err)
		}

		return r
	}

	root := resolved(t.TempDir())
	outside := resolved(t.TempDir())

	if err := os.WriteFile(filepath.Join(outside, "exists.mp4"), []byte(clipBytes), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "inside.mp4"), []byte(clipBytes), 0o600); err != nil {
		t.Fatal(err)
	}

	m := newMediaEnv(t, mediaCfg{guard: &pathguard.Config{Roots: []string{root}, Network: true}})
	// A member: an administrator is shown the import roots on purpose (VV-062, SDD-007 section 7.3).
	alice := service.WithCaller(context.Background(), service.Caller{
		Actor: "user:alice", Scopes: service.RoleScopes(service.RoleMember), OwnerUserID: "alice",
	})

	project, err := m.svc.ProjectCreate(alice, service.ProjectCreateRequest{Name: "p"})
	if err != nil {
		t.Fatal(err)
	}

	answer := func(path string) string {
		_, err := m.svc.MediaImport(alice, service.MediaImportRequest{
			ProjectID: project.ProjectID, Files: []service.ImportFile{{Path: path}},
		})
		if err == nil {
			t.Fatalf("%s was accepted", path)
		}

		b, jerr := json.Marshal(err)
		if jerr != nil {
			t.Fatal(jerr)
		}

		return err.Error() + "|" + string(b)
	}

	want := answer(filepath.Join(outside, "exists.mp4"))

	for name, path := range map[string]string{
		"outside, missing":  filepath.Join(outside, "missing.mp4"),
		"outside, a folder": outside,
		"root, missing":     filepath.Join(root, "missing.mp4"),
		"root, a folder":    root,
		"the workspace":     filepath.Join(m.ws, "token"),
		"parent of root":    filepath.Dir(root),
	} {
		if got := answer(path); got != want {
			t.Errorf("%s answers differently:\n got %s\nwant %s", name, got, want)
		}
	}

	// The JSON form escapes backslashes, so a Windows path must be searched in both forms.
	for _, p := range []string{outside, root, m.ws} {
		escaped, jerr := json.Marshal(p)
		if jerr != nil {
			t.Fatal(jerr)
		}

		if strings.Contains(want, p) || strings.Contains(want, strings.Trim(string(escaped), `"`)) {
			t.Errorf("the refusal names the path %s: %s", p, want)
		}
	}

	// The same call for a file inside the root works: the guard is a door, not a wall.
	if _, err := m.svc.MediaImport(alice, service.MediaImportRequest{
		ProjectID: project.ProjectID, Files: []service.ImportFile{{Path: filepath.Join(root, "inside.mp4")}},
	}); err != nil {
		t.Errorf("a file inside the root: %v", err)
	}
}
