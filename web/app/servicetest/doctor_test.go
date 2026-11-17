package servicetest

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/engine/transcribe"
	"github.com/AlexGalhardo/vukem-video/service"
)

// canary is a recognisable value for every secret variable.
const canary = "CANARY-7f3a9c-do-not-print"

func doctorEnv(t *testing.T, mode string) *env {
	t.Helper()

	return newEnvWith(t, func(e *env, o *service.Options) {
		o.Workspace = string(e.dir)
		o.Version = "0.0.0-test"
		o.Doctor = service.DoctorOptions{
			Mode: mode,
			Environ: func() []string {
				return []string{
					"VUKEM_VIDEO_TRANSCRIBE_KEY=" + canary + "-transcribe",
					"VUKEM_VIDEO_ADMIN_PASSWORD=" + canary + "-password",
					"APP_KEY=" + canary + "-appkey",
					"OTEL_EXPORTER_OTLP_HEADERS=authorization=" + canary + "-otlp",
					"VUKEM_VIDEO_MAX_EXPORTS=3",
					"VUKEM_VIDEO_HOME=" + string(e.dir),
					"PATH=/usr/bin",
				}
			},
		}
	})
}

func operator() service.Caller {
	return service.Caller{Actor: "local:cli", Scopes: service.AllScopes, Operator: true}
}

func unzipAll(t *testing.T, path string) map[string]string {
	t.Helper()

	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = zr.Close() }()

	out := map[string]string{}

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}

		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		out[f.Name] = string(b)
	}

	return out
}

// T-04: a canary in every secret variable, in the token file and in a log line
// appears in no part of the report and in no file of the unpacked bundle.
func TestDoctorBundleNoSecrets(t *testing.T) {
	e := doctorEnv(t, service.ModeLocal)

	token, err := e.dir.EnsureToken()
	if err != nil {
		t.Fatal(err)
	}

	logs := filepath.Join(string(e.dir), "logs")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		t.Fatal(err)
	}

	line := "level=INFO msg=boot key=" + canary + "-transcribe token=" + token + " user=" + canary + "-appkey\n"
	if err := os.WriteFile(filepath.Join(logs, "vukem-video.log"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}

	project := e.newProject("bundle me")

	ctx := service.WithCaller(context.Background(), operator())

	res, err := e.svc.Doctor(ctx, service.DoctorRequest{Full: true, Bundle: true, BundleProject: project})
	if err != nil {
		t.Fatal(err)
	}

	whole, jerr := json.Marshal(res)
	if jerr != nil {
		t.Fatal(jerr)
	}

	if strings.Contains(string(whole)+res.Text, canary) || strings.Contains(string(whole), token) {
		t.Fatalf("the report holds a secret: %s", res.Text)
	}

	if !strings.Contains(res.Text, "VUKEM_VIDEO_TRANSCRIBE_KEY=set") ||
		!strings.Contains(res.Text, "VUKEM_VIDEO_MAX_EXPORTS=3") {
		t.Errorf("the environment line = %s", res.Text)
	}

	if !strings.HasPrefix(res.Bundle, "exports/diagnostics/doctor-") || !strings.HasSuffix(res.Bundle, ".zip") {
		t.Fatalf("bundle = %q", res.Bundle)
	}

	files := unzipAll(t, filepath.Join(string(e.dir), filepath.FromSlash(res.Bundle)))

	for _, want := range []string{"doctor.json", "doctor.txt", "versions.txt", "environment.txt", "logs/vukem-video.log", "project.json"} {
		if _, ok := files[want]; !ok {
			t.Errorf("the bundle lacks %s: %v", want, keys(files))
		}
	}

	for name, body := range files {
		if strings.Contains(body, canary) || strings.Contains(body, token) {
			t.Errorf("%s holds a secret", name)
		}

		if name != "project.json" && strings.Contains(body, string(e.dir)) {
			t.Errorf("%s holds a server path", name)
		}
	}

	if !strings.Contains(files["logs/vukem-video.log"], "[redacted]") {
		t.Error("the log was not scrubbed")
	}

	// Without the explicit flag no document is in the bundle.
	again, err := e.svc.Doctor(ctx, service.DoctorRequest{Bundle: true})
	if err != nil {
		t.Fatal(err)
	}

	if _, has := unzipAll(t, filepath.Join(string(e.dir), filepath.FromSlash(again.Bundle)))["project.json"]; has {
		t.Error("a project document was bundled without the flag")
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}

// A network caller without admin gets versions and capability facts, no path,
// no workspace and no environment, and cannot ask for a bundle.
func TestDoctorHidesPathsForNonAdmin(t *testing.T) {
	e := doctorEnv(t, service.ModeNetwork)

	reader := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:r", OwnerUserID: "alice", Scopes: []string{service.ScopeRead},
	})

	res, err := e.svc.Doctor(reader, service.DoctorRequest{Full: true})
	if err != nil {
		t.Fatal(err)
	}

	whole, jerr := json.Marshal(res)
	if jerr != nil {
		t.Fatal(jerr)
	}

	if strings.Contains(string(whole), string(e.dir)) || res.Workspace != "" {
		t.Errorf("the report holds a path: %s", whole)
	}

	for _, c := range res.Checks {
		if c.Name == "environment" {
			t.Error("a non-admin caller sees the environment")
		}
	}

	if _, err := e.svc.Doctor(reader, service.DoctorRequest{Bundle: true}); code(err) != service.CodeForbidden {
		t.Errorf("a bundle for a non-admin = %v", err)
	}

	if _, err := e.svc.Doctor(reader, service.DoctorRequest{Verify: true}); code(err) != service.CodeForbidden {
		t.Errorf("verify for a non-admin = %v", err)
	}

	admin := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:a", OwnerUserID: "alice", Scopes: service.AllScopes,
	})

	got, err := e.svc.Doctor(admin, service.DoctorRequest{Full: true})
	if err != nil {
		t.Fatal(err)
	}

	hasEnv := false

	for _, c := range got.Checks {
		hasEnv = hasEnv || c.Name == "environment"
	}

	if !hasEnv {
		t.Error("an admin does not see the environment")
	}
}

// Only the person at the machine runs the sweep: an agent that asks gets E_FORBIDDEN.
func TestDoctorSweepIsForTheOperator(t *testing.T) {
	e := doctorEnv(t, service.ModeLocal)

	if _, err := e.svc.Doctor(e.as(agent), service.DoctorRequest{GC: true}); code(err) != service.CodeForbidden {
		t.Errorf("an agent asking for the sweep = %v", err)
	}

	res, err := e.svc.Doctor(service.WithCaller(context.Background(), operator()), service.DoctorRequest{GC: true})
	if err != nil || res.GC == nil {
		t.Errorf("the operator's sweep = %+v, %v", res, err)
	}
}

// --verify compares an installed helper with what was recorded: a modified file is a failed check.
func TestDoctorVerifyMismatch(t *testing.T) {
	ts := newToolsServer(t)
	dir := filepath.Join(t.TempDir(), "tools")
	m := newMediaEnv(t, mediaCfg{tools: ts.tools(dir)})
	person := service.WithCaller(context.Background(), service.Caller{
		Actor: "local:ui", Scopes: service.AllScopes, Browser: true,
	})

	if _, err := m.svc.ToolsInstall(person, service.ToolsInstallRequest{Name: "whisper", Model: "tiny"}); err != nil {
		t.Fatal(err)
	}

	// A failed install leaves one digest and would read as a doctor bug; say why.
	if st := m.waitInstall(person); st.State != "idle" {
		t.Fatalf("install ended %q: %v", st.State, st.Error)
	}

	digests := func() (ok, bad int) {
		res, err := m.svc.Doctor(person, service.DoctorRequest{Verify: true})
		if err != nil {
			t.Fatal(err)
		}

		for _, c := range res.Checks {
			if c.Name != "transcribe.digest" {
				continue
			}

			if c.OK {
				ok++
			} else {
				bad++
			}
		}

		return ok, bad
	}

	if ok, bad := digests(); ok != 2 || bad != 0 {
		t.Fatalf("clean install: %d ok, %d failed", ok, bad)
	}

	helper, _ := service.VerifiedTool(dir, "whisper", transcribe.KindHelper, "")
	if err := os.WriteFile(helper, []byte("tampered"), 0o700); err != nil {
		t.Fatal(err)
	}

	if ok, bad := digests(); ok != 1 || bad != 1 {
		t.Errorf("tampered helper: %d ok, %d failed; want 1 and 1", ok, bad)
	}
}

// A key limited to one project exists in local mode: it gets no workspace path,
// no path in a check, no environment, and cannot verify or bundle.
func TestDoctorRefusesAProjectLimitedKeyTheMachineDetails(t *testing.T) {
	e := doctorEnv(t, service.ModeLocal)
	project := e.newProject("p")

	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:limited", Scopes: service.AllScopes, ProjectID: project,
	})

	res, err := e.svc.Doctor(limited, service.DoctorRequest{Full: true})
	if err != nil {
		t.Fatal(err)
	}

	whole, jerr := json.Marshal(res)
	if jerr != nil {
		t.Fatal(jerr)
	}

	if res.Workspace != "" || strings.Contains(string(whole), string(e.dir)) ||
		strings.Contains(string(whole), "environment") {
		t.Errorf("a limited key sees machine details: %s", whole)
	}

	for _, req := range []service.DoctorRequest{{Verify: true}, {Bundle: true}, {GC: true}} {
		if _, err := e.svc.Doctor(limited, req); code(err) != service.CodeForbidden {
			t.Errorf("%+v by a limited key = %v, want E_FORBIDDEN", req, err)
		}
	}

	// The local token of an agent surface is not a person either.
	if _, err := e.svc.Doctor(e.as(agent), service.DoctorRequest{Verify: true}); code(err) != service.CodeForbidden {
		t.Errorf("verify by the local token = %v", err)
	}

	// The operator sees plain values for the allowlist and "set" for the rest.
	op, err := e.svc.Doctor(service.WithCaller(context.Background(), operator()), service.DoctorRequest{Full: true})
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range op.Checks {
		if c.Name == "environment" && (!strings.Contains(c.Detail, "VUKEM_VIDEO_MAX_EXPORTS=3") ||
			!strings.Contains(c.Detail, "VUKEM_VIDEO_HOME=set") || strings.Contains(c.Detail, string(e.dir))) {
			t.Errorf("environment = %s", c.Detail)
		}
	}
}

// SR1-01 (T-29): a logged-in member of network mode holds a browser session but is not the person
// running the machine: no paths, no environment, no verify, no bundle (SDD-007 sections 5.2 and 7.3).
func TestDoctorRefusesANetworkMemberTheMachineDetails(t *testing.T) {
	e := doctorEnv(t, service.ModeNetwork)

	member := service.WithCaller(context.Background(), service.Caller{
		Actor:       "user:usr_member",
		Scopes:      service.RoleScopes(service.RoleMember),
		OwnerUserID: "usr_member",
		Browser:     true,
	})

	res, err := e.svc.Doctor(member, service.DoctorRequest{Full: true})
	if err != nil {
		t.Fatal(err)
	}

	whole, jerr := json.Marshal(res)
	if jerr != nil {
		t.Fatal(jerr)
	}

	if res.Workspace != "" || strings.Contains(string(whole), string(e.dir)) ||
		strings.Contains(string(whole), "environment") {
		t.Errorf("a member sees machine details: %s", whole)
	}

	for _, req := range []service.DoctorRequest{{Verify: true}, {Bundle: true}} {
		if _, err := e.svc.Doctor(member, req); code(err) != service.CodeForbidden {
			t.Errorf("%+v by a member = %v, want E_FORBIDDEN", req, err)
		}
	}
}

// limitsMechanism is how the report names the limits of the platform: prlimit(1) on Unix, a Job Object on
// Windows (VV-110).
func limitsMechanism() string {
	if runtime.GOOS == "windows" {
		return "available: a Job Object"
	}

	return "available: prlimit"
}

// SDD-007 sections 6.5 and 6.7: doctor says whether the limits and the sandbox are
// available, and never prints the path of the sandbox.
func TestDoctorReportsTheChildPolicy(t *testing.T) {
	cases := map[string]struct {
		policy             service.ChildPolicy
		wantLimits, wantSB string
	}{
		"both": {
			service.ChildPolicy{
				Limits:  true,
				Sandbox: "/usr/bin/bwrap",
			},
			limitsMechanism(),
			"available: every FFmpeg run",
		},
		"neither": {
			service.ChildPolicy{LimitsWhy: "resource limits are not available on this operating system"},
			"unavailable: resource limits", "unavailable: not requested",
		},
	}

	for name, tc := range cases {
		e := newEnvWith(t, func(e *env, o *service.Options) {
			o.Workspace = string(e.dir)
			o.Doctor = service.DoctorOptions{Mode: service.ModeLocal, Child: tc.policy}
		})

		res, err := e.svc.Doctor(e.as(ui), service.DoctorRequest{Full: true})
		if err != nil {
			t.Fatal(err)
		}

		got := map[string]string{}

		for _, c := range res.Checks {
			got[c.Name] = c.Detail
		}

		if !strings.HasPrefix(got["child.limits"], tc.wantLimits) ||
			!strings.HasPrefix(got["child.sandbox"], tc.wantSB) {
			t.Errorf("%s: checks = %q", name, got)
		}

		if strings.Contains(res.Text, "/usr/bin/bwrap") {
			t.Errorf("%s: the report prints the sandbox path", name)
		}
	}
}

// VV-110: a missing transcription helper is optional, and a report without checks says how to get them.
func TestDoctorMarksTheHelperOptionalAndSaysWhenNoCheckRan(t *testing.T) {
	e := newEnvWith(t, func(e *env, o *service.Options) {
		o.Workspace = string(e.dir)
		o.Doctor = service.DoctorOptions{Mode: service.ModeLocal}
	})

	short, err := e.svc.Doctor(e.as(ui), service.DoctorRequest{})
	if err != nil || !strings.Contains(short.Text, "--full") {
		t.Fatalf("short report = %+v, %v", short, err)
	}

	full, err := e.svc.Doctor(e.as(ui), service.DoctorRequest{Full: true})
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range full.Checks {
		if strings.HasPrefix(c.Name, "transcribe.") && c.Name != "transcribe.provider" && !c.OK && !c.Optional {
			t.Errorf("%s is missing and not optional", c.Name)
		}
	}
}

// VV-129: in local mode the editor session of a launch code (not the desktop shell) sees the workspace folder.
func TestDoctorShowsTheWorkspaceToALocalEditorSession(t *testing.T) {
	e := doctorEnv(t, service.ModeLocal)

	res, err := e.svc.Doctor(e.as(ui), service.DoctorRequest{})
	if err != nil || res.Workspace != string(e.dir) {
		t.Fatalf("workspace = %q, %v; want %q", res.Workspace, err, e.dir)
	}
}
