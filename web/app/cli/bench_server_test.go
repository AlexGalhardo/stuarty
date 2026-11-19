package cli

import (
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// readBudget is NFR-024: a read command through a running server, from process
// start to its output, answers in under 300 ms (SDD-003 section 9 check 16).
const readBudget = 300 * time.Millisecond

// BenchmarkCLIReadViaServer starts `stuarty serve` and times `project:list`
// as a process: it finds the owner through runtime.json, checks /up, makes the
// handshake and makes the call. The median must stay under 300 ms; the run is
// only meaningful on the reference machine of SDD-008.
func BenchmarkCLIReadViaServer(b *testing.B) {
	bin := filepath.Join(b.TempDir(), "stuarty")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	build := exec.CommandContext(b.Context(), "go", "build", "-o", bin, "../../cmd/app")
	if out, err := build.CombinedOutput(); err != nil {
		b.Fatalf("go build: %v\n%s", err, out)
	}

	home := filepath.Join(b.TempDir(), "ws")
	env := append(os.Environ(), workspace.EnvHome+"="+home, workspace.EnvURL+"=", "APP_KEY=", "APP_ENV=",
		"STUARTY_MIN_FREE_BYTES=1")

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}

	_, port, _ := net.SplitHostPort(l.Addr().String())
	_ = l.Close()

	ctx, cancel := context.WithCancel(b.Context())

	var out bytes.Buffer

	srv := exec.CommandContext(ctx, bin, "serve")
	srv.Env = slices.Concat(env, []string{"HOST=127.0.0.1", "PORT=" + port})
	srv.Stdout, srv.Stderr = &out, &out

	if err := srv.Start(); err != nil {
		cancel()
		b.Fatal(err)
	}

	done := make(chan struct{})

	go func() { _ = srv.Wait(); close(done) }()

	b.Cleanup(func() {
		cancel()
		<-done
	})

	deadline := time.Now().Add(30 * time.Second)

	for {
		if rt, rerr := workspace.Dir(home).ReadRuntime(); rerr == nil && rt.PID == srv.Process.Pid {
			break
		}

		select {
		case <-done:
			b.Fatalf("serve exited\n%s", out.String())
		case <-time.After(100 * time.Millisecond):
		}

		if time.Now().After(deadline) {
			b.Fatalf("serve did not announce itself\n%s", out.String())
		}
	}

	read := func() time.Duration {
		cmd := exec.CommandContext(b.Context(), bin, "project:list", "--json")
		cmd.Env = env

		start := time.Now()
		if o, rerr := cmd.CombinedOutput(); rerr != nil {
			b.Fatalf("project:list: %v\n%s", rerr, o)
		}

		return time.Since(start)
	}

	read() // the first run warms the disk cache of the binary

	samples := make([]time.Duration, 0, b.N)

	for b.Loop() {
		samples = append(samples, read())
	}

	slices.Sort(samples)

	median := samples[len(samples)/2]
	b.ReportMetric(float64(median.Milliseconds()), "median-ms")

	if median > readBudget {
		b.Errorf("the median read through the server is %v, at most %v (NFR-024)", median, readBudget)
	}
}
