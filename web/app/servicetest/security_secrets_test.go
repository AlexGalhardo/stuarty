package servicetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
)

// runtimeCanary is a secret made at run time, so no literal that looks like a
// credential is in the repository.
func runtimeCanary(t *testing.T, label string) string {
	t.Helper()

	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}

	return "canary-" + label + "-" + hex.EncodeToString(b)
}

// T-04, the part below HTTP: every secret variable of SDD-007 section 11 and
// the local token carry a canary; the errors of failing calls, the report and
// every file of the diagnostic bundle hold none of them.
func TestSecurity_T04_BundleAndEnvironmentHoldNoSecret(t *testing.T) {
	t.Parallel()

	names := []string{
		"APP_KEY", "VUKEM_VIDEO_KEY", "VUKEM_VIDEO_TRANSCRIBE_KEY", "VUKEM_VIDEO_ADMIN_PASSWORD",
		"OTEL_EXPORTER_OTLP_HEADERS", "VUKEM_VIDEO_TRANSCRIBE_TOKEN",
		"VUKEM_VIDEO_FUTURE_SECRET", // a secret nobody listed yet: the name pattern must catch it
	}
	canaries := map[string]string{}
	environ := []string{"PATH=/usr/bin", "VUKEM_VIDEO_MAX_EXPORTS=3"}

	for _, n := range names {
		canaries[n] = runtimeCanary(t, strings.ToLower(n[len(n)-6:]))
		environ = append(environ, n+"="+canaries[n])
	}

	e := newEnvWith(t, func(e *env, o *service.Options) {
		o.Workspace = string(e.dir)
		o.Version = "0.0.0-test"
		o.Doctor = service.DoctorOptions{Mode: service.ModeNetwork, Environ: func() []string { return environ }}
	})

	token, err := e.dir.EnsureToken()
	if err != nil {
		t.Fatal(err)
	}

	canaries["local token"] = token

	logs := filepath.Join(string(e.dir), "logs")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		t.Fatal(err)
	}

	var line strings.Builder

	line.WriteString("level=INFO msg=boot")

	for n, c := range canaries {
		line.WriteString(" " + strings.ToLower(n[len(n)-3:]) + "=" + c)
	}

	if err := os.WriteFile(filepath.Join(logs, "vukem-video.log"), []byte(line.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	project := e.newProject("p")
	ctx := e.as(ui)

	var observed []string

	note := func(err error) {
		observed = append(observed, err.Error())

		if b, jerr := json.Marshal(err); jerr == nil {
			observed = append(observed, string(b))
		}
	}

	_, perr := e.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: "prj_9999999999"})
	note(perr)

	_, eerr := e.edit(ui, project, 0, op("no.such.op", `{}`))
	note(eerr)

	_, merr := e.svc.MediaImport(
		ctx,
		service.MediaImportRequest{ProjectID: project, Files: []service.ImportFile{{Path: "../missing.mp4"}}},
	)
	note(merr)

	_, jerr := e.svc.JobGet(ctx, service.JobGetRequest{JobID: "job_9999999999"})
	note(jerr)

	_, nerr := e.svc.ProjectList(context.Background(), service.ProjectListRequest{})
	note(nerr)

	admin := service.WithCaller(context.Background(), service.Caller{
		Actor: "local:cli", Scopes: service.AllScopes, Operator: true,
	})

	res, derr := e.svc.Doctor(admin, service.DoctorRequest{Full: true, Bundle: true, BundleProject: project})
	if derr != nil {
		t.Fatal(derr)
	}

	whole, merr2 := json.Marshal(res)
	if merr2 != nil {
		t.Fatal(merr2)
	}

	observed = append(observed, string(whole), res.Text)

	files := unzipAll(t, filepath.Join(string(e.dir), filepath.FromSlash(res.Bundle)))
	for name, body := range files {
		observed = append(observed, "["+name+"]"+body)
	}

	all := strings.Join(observed, "\n")

	for n, c := range canaries {
		if strings.Contains(all, c) {
			t.Errorf("the value of %s appears in an error, the report or the bundle", n)
		}
	}
}
