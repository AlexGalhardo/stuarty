package main

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
)

var parentEnv = []string{
	"PATH=/bin", "HOME=/u", "SystemRoot=C:/Windows", "TEMP=/t", "TMP=/t", "STUARTY_MASTER_KEY_FILE=/k",
	"STUARTY_FFMPEG=/ff", "STUARTY_HOME=/parent-home",
	"ANTHROPIC_API_KEY=sk-ant-secret", "OPENAI_API_KEY=sk-secret", "GITHUB_TOKEN=tok", "AWS_SECRET=s3",
	"STUARTY_MASTER_KEY=raw", "STUARTY_HARNESS_PASSWORD=pw", "STUARTY_KEY=k", "MY_API_KEY=x", "Anthropic_Api_Key=mixed-case", "noequalsign",
}

func TestChildEnvKeepsOnlyTheAllowList(t *testing.T) {
	got := childEnv(parentEnv, "/run-home")

	for _, want := range []string{
		"PATH=/bin", "HOME=/u", "SystemRoot=C:/Windows", "TEMP=/t", "TMP=/t",
		"STUARTY_MASTER_KEY_FILE=/k", "STUARTY_FFMPEG=/ff", "STUARTY_HOME=/run-home",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("%q missing from %v", want, got)
		}
	}

	if slices.Contains(got, "STUARTY_HOME=/parent-home") {
		t.Errorf("the parent home leaked: %v", got)
	}

	for _, kv := range got {
		name, _, _ := strings.Cut(kv, "=")
		up := strings.ToUpper(name)

		if strings.HasSuffix(up, "_API_KEY") || strings.HasSuffix(up, "_TOKEN") ||
			strings.HasSuffix(up, "_SECRET") || up == "STUARTY_MASTER_KEY" ||
			up == "STUARTY_HARNESS_PASSWORD" || up == "STUARTY_KEY" {
			t.Errorf("a secret reached the child: %s", name)
		}
	}
}

// TestMain makes the test binary the child of TestExecRunnerNeverPassesAProviderKey when it is started
// with the single argument print-env: it prints the environment it was given and exits.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "print-env" {
		for _, kv := range os.Environ() {
			_, _ = os.Stdout.WriteString(kv + "\n")
		}

		os.Exit(0)
	}

	os.Exit(m.Run())
}

func TestExecRunnerNeverPassesAProviderKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-secret")
	t.Setenv("OPENAI_API_KEY", "sk-secret")
	t.Setenv("DEPLOY_TOKEN", "tok")
	t.Setenv("CLIENT_SECRET", "sec")
	t.Setenv("STUARTY_HARNESS_PASSWORD", "pw")
	t.Setenv("STUARTY_FFMPEG", "/ff")

	run := execRunner(os.Args[0])(t.TempDir(), t.TempDir())

	out, code, err := run(context.Background(), []string{"print-env"}, "")
	if err != nil || code != 0 {
		t.Fatalf("the helper failed: %v %d: %s", err, code, out)
	}

	for _, leaked := range []string{"sk-ant-secret", "sk-secret", "DEPLOY_TOKEN", "CLIENT_SECRET", "STUARTY_HARNESS_PASSWORD"} {
		if strings.Contains(out, leaked) {
			t.Errorf("%s reached the child:\n%s", leaked, out)
		}
	}

	if !strings.Contains(out, "STUARTY_FFMPEG=/ff") || !strings.Contains(out, "STUARTY_HOME=") {
		t.Errorf("an allowed variable did not reach the child:\n%s", out)
	}
}

func TestFirstFlowPasswordComesFromTheEnvironment(t *testing.T) {
	args := []string{
		"--runs", "1", "--scenario", "first-flow", "--flow-base", "http://127.0.0.1:1",
		"--flow-home", "h", "--flow-social", "http://127.0.0.1:2",
	}
	base := map[string]string{"STUARTY_HARNESS": "1", "ANTHROPIC_API_KEY": "k"}

	var stderr strings.Builder

	_, code := parseOptions(args, func(k string) string { return base[k] }, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "STUARTY_HARNESS_PASSWORD") {
		t.Errorf("no password: code %d: %s", code, stderr.String())
	}

	base["STUARTY_HARNESS_PASSWORD"] = "pw-from-env"

	o, code := parseOptions(args, func(k string) string { return base[k] }, &stderr)
	if code != 0 || o.flowPassword != "pw-from-env" {
		t.Errorf("code %d, password %q: %s", code, o.flowPassword, stderr.String())
	}

	withFlag := append(slices.Clone(args), "--flow-account-password", "x")

	_, code = parseOptions(withFlag, func(k string) string { return base[k] }, &stderr)
	if code != 2 {
		t.Errorf("the removed flag is still accepted: code %d", code)
	}
}
