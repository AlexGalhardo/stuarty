package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/vukem-video/internal/outbound"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/remote"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/contract/testenv"
)

// The subprocess drivers reach a real `vukem-video serve` the way an agent or
// a person does: the CLI as a process, MCP over stdio through `vukem-video mcp`
// (an HTTP client of the owner, SDD-003 section 4.5) and MCP over HTTP at /mcp.
// Each driver gets its own server and workspace, and the three must produce the
// same documents; the clock, the actor and the paths are the only differences
// and they are normalized.

var (
	binOnce   sync.Once
	binPath   string
	binDir    string
	errBinary error
)

// binary builds cmd/app once per run of the package.
func binary(t *testing.T) string {
	t.Helper()

	binOnce.Do(func() {
		//nolint:usetesting // shared by every test of the package; TestMain removes it
		binDir, errBinary = os.MkdirTemp("", "vukem-video-contract-")
		if errBinary != nil {
			return
		}

		name := "vukem-video"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}

		binPath = filepath.Join(binDir, name)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		build := exec.CommandContext(ctx, "go", "build", "-o", binPath, "../cmd/app")
		if out, err := build.CombinedOutput(); err != nil {
			errBinary = errors.New("go build: " + err.Error() + "\n" + string(out))
		}
	})

	if errBinary != nil {
		t.Fatal(errBinary)
	}

	return binPath
}

func TestMain(m *testing.M) {
	code := m.Run()

	if binDir != "" {
		_ = os.RemoveAll(binDir)
	}

	os.Exit(code)
}

// procEnv is a running `serve` and the ways to reach it.
type procEnv struct {
	bin   string
	files string
	base  string
	token string
	env   []string
	e     *testenv.Env
}

func freePort(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = l.Close() }()

	_, port, _ := net.SplitHostPort(l.Addr().String())

	return port
}

func needFFmpeg(t *testing.T) {
	t.Helper()

	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed: the subprocess drivers import and export real media", tool)
		}
	}
}

func startProc(t *testing.T) *procEnv {
	t.Helper()

	bin := binary(t)
	root := t.TempDir()
	home := filepath.Join(root, "ws")
	files := filepath.Join(root, "files")

	if err := os.MkdirAll(files, 0o700); err != nil {
		t.Fatal(err)
	}

	clean := append(os.Environ(), workspace.EnvHome+"="+home, workspace.EnvURL+"=", "APP_KEY=", "APP_ENV=")

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "serve")
	cmd.Env = slices.Concat(clean, []string{"HOST=127.0.0.1", "PORT=" + freePort(t), "VUKEM_VIDEO_MIN_FREE_BYTES=1"})

	var out bytes.Buffer

	cmd.Stdout, cmd.Stderr = &out, &out

	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}

	done := make(chan struct{})

	go func() { _ = cmd.Wait(); close(done) }()

	t.Cleanup(func() {
		cancel() // kills the process: Windows has no SIGTERM for another process
		<-done
	})

	deadline := time.Now().Add(30 * time.Second)

	for {
		if rt, err := workspace.Dir(home).ReadRuntime(); err == nil && rt.PID == cmd.Process.Pid {
			break
		}

		select {
		case <-done:
			t.Fatalf("serve exited\n%s", out.String())
		case <-time.After(100 * time.Millisecond):
		}

		if time.Now().After(deadline) {
			t.Fatalf("serve did not announce itself\n%s", out.String())
		}
	}

	sess, err := workspace.Open(t.Context(), workspace.Options{Getenv: func(k string) string {
		if k == workspace.EnvHome {
			return home
		}

		return ""
	}})
	if err != nil || sess.Mode != workspace.ModeHTTP {
		t.Fatalf("the workspace is not owned by the server: %+v, %v", sess, err)
	}

	return &procEnv{
		bin: bin, files: files, base: sess.BaseURL, token: sess.Token, env: clean,
		e: &testenv.Env{Dir: workspace.Dir(home), Files: files},
	}
}

// service is the HTTP client of the server, to read the final document.
func (p *procEnv) service() service.Service {
	return remote.New(p.base+"/", p.token, outbound.LocalServer(time.Minute))
}

// writeFixture makes a real clip: the server probes and renders with FFmpeg.
func (p *procEnv) writeFixture(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(p.files, name+".mp4")
	if _, err := os.Stat(path); err == nil {
		return path
	}

	args := []string{
		"-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x568:rate=30:duration=2",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:v", "mpeg4", "-c:a", "aac", "-shortest", "-y", path,
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	if out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}

	return path
}

// fixtureWriter is implemented by the drivers whose server needs a real file.
type fixtureWriter interface {
	writeFixture(t *testing.T, name string) string
}

// cliProc runs one command per process.
type cliProc struct{ p *procEnv }

func (d *cliProc) name() string             { return "cli-process" }
func (d *cliProc) service() service.Service { return d.p.service() }
func (d *cliProc) env() *testenv.Env        { return d.p.e }
func (d *cliProc) writeFixture(t *testing.T, name string) string {
	t.Helper()

	return d.p.writeFixture(t, name)
}

func (d *cliProc) do(t *testing.T, s step) (json.RawMessage, string, *service.ErrorDocument) {
	t.Helper()

	name, argv, stdin, err := command2argv(s)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, d.p.bin, append([]string{name}, withJSONFlags(name, argv)...)...)
	cmd.Env = d.p.env
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr bytes.Buffer

	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	code := 0

	if err := cmd.Run(); err != nil {
		ee, ok := errors.AsType[*exec.ExitError](err)
		if !ok {
			t.Fatalf("cli %s: %v", name, err)
		}

		code = ee.ExitCode()
	}

	return cliOutcome(t, name, code, stdout.Bytes(), stderr.String())
}

// mcpProc is an MCP client session over a transport that reaches the server.
type mcpProc struct {
	mcpDriver
	label string
	p     *procEnv
}

func (d *mcpProc) name() string             { return d.label }
func (d *mcpProc) service() service.Service { return d.p.service() }
func (d *mcpProc) writeFixture(t *testing.T, name string) string {
	t.Helper()

	return d.p.writeFixture(t, name)
}

func connect(t *testing.T, p *procEnv, label string, tr sdk.Transport) driver {
	t.Helper()

	cs, err := sdk.NewClient(&sdk.Implementation{Name: actor, Version: "1"}, nil).Connect(t.Context(), tr, nil)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}

	t.Cleanup(func() { _ = cs.Close() })

	return &mcpProc{e: p.e, cs: cs, label: label, p: p}
}

// mcpStdio starts `vukem-video mcp`, which finds the owner and relays to it.
func mcpStdio(t *testing.T, p *procEnv) driver {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), p.bin, "mcp")
	cmd.Env = p.env

	return connect(t, p, "mcp-stdio", &sdk.CommandTransport{Command: cmd})
}

type bearer struct {
	token string
	next  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)

	return b.next.RoundTrip(r)
}

// mcpHTTP calls POST /mcp of the server with the key of the workspace.
func mcpHTTP(t *testing.T, p *procEnv) driver {
	t.Helper()

	hc := outbound.LocalServer(0)
	hc.Transport = bearer{token: p.token, next: hc.Transport}

	return connect(t, p, "mcp-http", &sdk.StreamableClientTransport{Endpoint: p.base + "/mcp", HTTPClient: hc})
}

var (
	timestamp = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?(Z|[+-]\d\d:\d\d)`)
	// The actor of a call is the surface it came through, which is the one thing
	// the three subprocess drivers are allowed to differ in.
	actorName = regexp.MustCompile(`(local|key|user):[A-Za-z0-9_.-]+`)
)

// The progress of a finished job is whatever the last tick saw.
var progress = regexp.MustCompile(`\(\d+%\)`)

func scrub(s string) string {
	s = actorName.ReplaceAllString(timestamp.ReplaceAllString(s, "<time>"), "<actor>")

	return progress.ReplaceAllString(s, "(<n>%)")
}

// TestScenarioThroughTheRealServer drives the scenario of the Prototype through
// the CLI process, MCP over stdio and MCP over HTTP, each against its own
// `serve`, and compares what they return with the first of them.
func TestScenarioThroughTheRealServer(t *testing.T) {
	needFFmpeg(t)

	steps, fixtures := loadScenario(t)

	makers := []func(*testing.T, *procEnv) driver{
		func(_ *testing.T, p *procEnv) driver { return &cliProc{p: p} },
		mcpStdio,
		mcpHTTP,
	}

	var (
		want    []outcome
		wantDoc string
	)

	for i, mk := range makers {
		p := startProc(t)
		d := mk(t, p)
		got := run(t, d, steps, fixtures)
		doc := scrub(normalize(t, d, finalDocument(t, d, "prj_7k2m9x4q1b")))

		if i == 0 {
			want, wantDoc = got, doc

			continue
		}

		for j := range steps {
			if scrub(got[j].result) != scrub(want[j].result) {
				t.Errorf("%s step %d (%s): result differs\n got: %s\nwant: %s",
					d.name(), j, steps[j].Call, scrub(got[j].result), scrub(want[j].result))
			}

			if scrub(got[j].fail) != scrub(want[j].fail) {
				t.Errorf("%s step %d (%s): error differs\n got: %s\nwant: %s",
					d.name(), j, steps[j].Call, got[j].fail, want[j].fail)
			}
		}

		if doc != wantDoc {
			t.Errorf("%s: the final document differs\n got: %s\nwant: %s", d.name(), doc, wantDoc)
		}
	}
}
