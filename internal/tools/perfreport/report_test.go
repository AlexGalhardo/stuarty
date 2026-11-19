package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sample = `goos: windows
goarch: amd64
pkg: github.com/AlexGalhardo/stuarty/engine/export
cpu: AMD Ryzen 7 5700X3D 8-Core Processor
BenchmarkExport60s
    perf_test.go:185: h264_nvenc: 22.2 s, peak 111 MB
BenchmarkExport60s-16    	       1	22160535500 ns/op	       111.0 peak_MB	        22.16 s_export	         2.708 x_realtime
BenchmarkViewFrame-16     	       5	 170792060 ns/op
PASS
`

func TestParseBench(t *testing.T) {
	t.Parallel()
	res, env := ParseBench(sample)
	if env.GOOS != "windows" || env.GOARCH != "amd64" || !strings.HasPrefix(env.CPU, "AMD Ryzen") {
		t.Errorf("env %+v", env)
	}
	r := res["BenchmarkExport60s"]
	if r["s_export"] != 22.16 || r["peak_MB"] != 111 || r["x_realtime"] != 2.708 || r["ns/op"] != 22160535500 {
		t.Errorf("metrics %v", r)
	}
	if res["BenchmarkViewFrame"]["ns/op"] != 170792060 {
		t.Errorf("view frame %v", res["BenchmarkViewFrame"])
	}
}

func rowsOf(t *testing.T, out string, specs ...Spec) []Row {
	t.Helper()
	res, _ := ParseBench(out)
	return Rows(specs, res)
}

func TestRowsPassAndFail(t *testing.T) {
	t.Parallel()
	export60 := Spec{"NFR-006", "./x", "BenchmarkExport60s", "s_export", 1, 60, "s", true, "1x"}
	memory := Spec{"NFR-007", "./x", "BenchmarkExport60s", "peak_MB", 1, 100, "MB", true, "1x"}
	speed := Spec{"NFR-006", "./x", "BenchmarkExport60s", "x_realtime", 1, 1.5, "x real time", false, "1x"}
	frame := Spec{"NFR-023", "./x", "BenchmarkViewFrame", "ns/op", 1e6, 1000, "ms", true, "5x"}
	absent := Spec{"NFR-005", "./x", "BenchmarkIngest60s", "s_ingest", 1, 30, "s", true, "1x"}
	rows := rowsOf(t, sample, export60, memory, speed, frame, absent)
	want := []bool{true, false, true, true, false}
	for i, w := range want {
		if rows[i].Pass() != w {
			t.Errorf("row %d (%s): pass = %v, want %v", i, rows[i].Spec.Metric, rows[i].Pass(), w)
		}
	}
	if !rows[4].Missing {
		t.Error("an unmeasured benchmark is not marked missing")
	}
	if got := rows[3].Measured; got < 170 || got > 171 {
		t.Errorf("ns/op scaled to ms: %v", got)
	}
}

func TestMarkdownRoundTripAndRegression(t *testing.T) {
	t.Parallel()
	specs := []Spec{
		{"NFR-006", "./x", "BenchmarkExport60s", "s_export", 1, 60, "s", true, "1x"},
		{"NFR-006", "./x", "BenchmarkExport60s", "x_realtime", 1, 1.5, "x real time", false, "1x"},
	}
	res, env := ParseBench(sample)
	rep := Report{Date: "2026-11-14", Env: env, Rows: Rows(specs, res)}
	md, err := rep.Markdown()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "| NFR-006 | `BenchmarkExport60s` (s_export) | 22.16 s | at most 60.00 s | pass |") {
		t.Errorf("table:\n%s", md)
	}
	base, err := ParseReport(md)
	if err != nil || base.Env.GOOS != "windows" || base.Values["BenchmarkExport60s s_export"] != 22.16 {
		t.Fatalf("base %+v %v", base, err)
	}

	if regs := Compare(base, rep.Rows); len(regs) != 0 {
		t.Errorf("the same numbers regress: %v", regs)
	}
	// 15% slower and 15% less speed: within the limit of 20%.
	slower := []Row{{Spec: &specs[0], Measured: 22.16 * 1.15}, {Spec: &specs[1], Measured: 2.708 * 0.85}}
	if regs := Compare(base, slower); len(regs) != 0 {
		t.Errorf("a 15%% change failed: %v", regs)
	}
	// A deliberately slowed function: 30% slower, and 30% less speed.
	worse := []Row{{Spec: &specs[0], Measured: 22.16 * 1.3}, {Spec: &specs[1], Measured: 2.708 * 0.7}}
	regs := Compare(base, worse)
	if len(regs) != 2 || regs[0].Worsened < 0.29 || regs[0].Worsened > 0.31 {
		t.Errorf("regressions %+v", regs)
	}
	// Better is never a regression.
	if regs := Compare(base, []Row{{Spec: &specs[0], Measured: 10}, {Spec: &specs[1], Measured: 9}}); len(regs) != 0 {
		t.Errorf("an improvement failed: %v", regs)
	}
	if _, err := ParseReport("# nothing"); err == nil {
		t.Error("a table without values was accepted")
	}
}

func fake(out string) Runner {
	return func(context.Context, string, string, string) (string, error) { return out, nil }
}

func TestRunWritesThenComparesAgainstTheLastTable(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "perf")
	day := func(d string) func() time.Time {
		return func() time.Time {
			tm, _ := time.Parse("2006-01-02", d)
			return tm
		}
	}
	var out, errb bytes.Buffer
	if code := run(
		context.Background(),
		[]string{"-dir", dir, "-write", "-only", "NFR-006"},
		&out,
		&errb,
		fake(sample),
		day("2026-11-14"),
	); code != 0 {
		t.Fatalf("write: %d %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-11-14-windows.md")); err != nil {
		t.Fatal(err)
	}

	errb.Reset()
	if code := run(
		context.Background(),
		[]string{"-dir", dir, "-compare", "-only", "NFR-006"},
		&out,
		&errb,
		fake(sample),
		day("2026-11-15"),
	); code != 0 {
		t.Fatalf("same numbers: %d %s", code, errb.String())
	}

	slow := strings.ReplaceAll(sample, "22.16 s_export", "40.00 s_export")
	errb.Reset()
	if code := run(
		context.Background(),
		[]string{"-dir", dir, "-compare", "-only", "NFR-006"},
		&out,
		&errb,
		fake(slow),
		day("2026-11-15"),
	); code != 1 ||
		!strings.Contains(errb.String(), "worse than 2026-11-14-windows.md") {
		t.Fatalf("a slowed run passed: %d %s", code, errb.String())
	}

	// Another platform has no baseline: nothing to compare, no failure.
	other := strings.ReplaceAll(strings.ReplaceAll(slow, "windows", "linux"), "amd64", "arm64")
	errb.Reset()
	if code := run(
		context.Background(),
		[]string{"-dir", dir, "-compare", "-only", "NFR-006"},
		&out,
		&errb,
		fake(other),
		day("2026-11-15"),
	); code != 0 {
		t.Fatalf("another platform: %d %s", code, errb.String())
	}

	// The absolute limits: a missing benchmark and a missed limit both fail.
	errb.Reset()
	if code := run(
		context.Background(),
		[]string{"-dir", dir, "-absolute", "-only", "NFR-007"},
		&out,
		&errb,
		fake(sample),
		day("2026-11-15"),
	); code != 0 {
		t.Fatalf("absolute within limits: %d %s", code, errb.String())
	}
	errb.Reset()
	if code := run(
		context.Background(),
		[]string{"-dir", dir, "-absolute", "-only", "NFR-006"},
		&out,
		&errb,
		fake("goos: linux\n"),
		day("2026-11-15"),
	); code != 1 {
		t.Fatalf("absolute with nothing measured: %d", code)
	}
	if code := run(
		context.Background(),
		[]string{"-only", "NFR-999"},
		&out,
		&errb,
		fake(sample),
		day("2026-11-15"),
	); code != 2 {
		t.Errorf("unknown requirement: %d", code)
	}
}
