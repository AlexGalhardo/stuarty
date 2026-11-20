package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Spec is one line of the performance table: a requirement, the benchmark that measures it, the
// metric to read and the limit (SDD-008 section 7).
type Spec struct {
	NFR       string
	Package   string
	Bench     string
	Metric    string  // a unit of the benchmark output: ns/op or one reported with b.ReportMetric
	Scale     float64 // the measured value is divided by Scale (1e6 turns ns/op into ms)
	Limit     float64
	Unit      string
	Lower     bool // lower is better: the limit is a ceiling, otherwise a floor
	Benchtime string
}

// Specs is the table of SDD-008 section 7 that the Prototype measures.
var Specs = []Spec{
	{"NFR-001", "./web/app/servicetest", "BenchmarkCommit200Items", "p95-ms", 1, 50, "ms", true, "200x"},
	{"NFR-005", "./engine/ingest", "BenchmarkIngest60s", "s_ingest", 1, 30, "s", true, "1x"},
	{"NFR-006", "./engine/export", "BenchmarkExport60s", "s_export", 1, 60, "s", true, "1x"},
	{"NFR-006", "./engine/export", "BenchmarkExportCommonCase", "x_realtime", 1, 1.5, "x real time", false, "1x"},
	{"NFR-007", "./engine/export", "BenchmarkExport60s", "peak_MB", 1, 500, "MB", true, "1x"},
	{"NFR-023", "./engine/frame", "BenchmarkViewFrame", "ns/op", 1e6, 1000, "ms", true, "5x"},
	{"NFR-023", "./engine/frame", "BenchmarkViewSheet9", "ns/op", 1e6, 2000, "ms", true, "5x"},
	{"NFR-024", "./web/app/cli", "BenchmarkCLIReadViaServer", "median-ms", 1, 300, "ms", true, "9x"},
	{"NFR-061", "./engine/export", "BenchmarkImageReference", "compose_ms", 1, 100, "ms", true, "1x"},
	{"NFR-061", "./engine/export", "BenchmarkImageReference", "s_export", 1, 2, "s", true, "1x"},
}

// Result is a benchmark line: the metrics it printed, by unit.
type Result map[string]float64

// Env is what the benchmark output says about the machine.
type Env struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	CPU    string `json:"cpu"`
}

var (
	benchLine = regexp.MustCompile(`^(Benchmark\w+?)(?:-\d+)?\s+(\d+)\s+(.*)$`)
	metricRe  = regexp.MustCompile(`(\d[0-9.eE+-]*)\s+(\S+)`)
)

// ParseBench reads the output of go test -bench: the metrics of each benchmark (the last line of a
// benchmark wins) and the environment lines.
func ParseBench(out string) (map[string]Result, Env) {
	res := map[string]Result{}
	var env Env
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "goos: "):
			env.GOOS = strings.TrimPrefix(line, "goos: ")
		case strings.HasPrefix(line, "goarch: "):
			env.GOARCH = strings.TrimPrefix(line, "goarch: ")
		case strings.HasPrefix(line, "cpu: "):
			env.CPU = strings.TrimPrefix(line, "cpu: ")
		}
		m := benchLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		r := Result{}
		for _, mm := range metricRe.FindAllStringSubmatch(m[3], -1) {
			if v, err := strconv.ParseFloat(mm[1], 64); err == nil {
				r[mm[2]] = v
			}
		}
		if len(r) > 0 {
			res[m[1]] = r
		}
	}
	return res, env
}

// Row is one measured line of the table.
type Row struct {
	Spec     *Spec
	Measured float64
	Missing  bool
}

// Pass says whether the measured value is on the right side of the limit.
func (r Row) Pass() bool {
	if r.Missing {
		return false
	}
	if r.Spec.Lower {
		return r.Measured <= r.Spec.Limit
	}
	return r.Measured >= r.Spec.Limit
}

// Rows picks the metric of each spec out of the parsed results.
func Rows(specs []Spec, res map[string]Result) []Row {
	rows := make([]Row, 0, len(specs))
	for i := range specs {
		s := &specs[i]
		row := Row{Spec: s, Missing: true}
		if v, ok := res[s.Bench][s.Metric]; ok {
			row.Measured, row.Missing = v/s.Scale, false
		}
		rows = append(rows, row)
	}
	return rows
}

// Report is a table with the environment it was measured on.
type Report struct {
	Date string
	Env  Env
	Rows []Row
}

// Values is the machine-readable form kept in the file: benchmark and metric to value.
type Values struct {
	Env    Env                `json:"env"`
	Values map[string]float64 `json:"values"`
}

func key(s *Spec) string { return s.Bench + " " + s.Metric }

const marker = "perfreport:"

var markerRe = regexp.MustCompile(`<!-- ` + marker + ` (\{.*\}) -->`)

func format(v float64, unit string) string {
	switch unit {
	case "MB", "ms":
		return strconv.FormatFloat(v, 'f', 1, 64)
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// Markdown writes the table of docs/perf/<date>.md.
func (r Report) Markdown() (string, error) {
	vals := Values{Env: r.Env, Values: map[string]float64{}}
	for _, row := range r.Rows {
		if !row.Missing {
			vals.Values[key(row.Spec)] = row.Measured
		}
	}
	js, err := json.Marshal(vals)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Performance %s\n\n", r.Date)
	fmt.Fprintf(&b, "Measured on %s/%s, %s. Written by `go run ./internal/tools/perfreport`; limits are those of "+
		"SDD-008 section 7 and PRD-001. A regression of more than 20%% against the last table of the same "+
		"platform fails the job.\n\n", r.Env.GOOS, r.Env.GOARCH, orUnknown(r.Env.CPU))
	b.WriteString("| Requirement | Benchmark | Measured | Limit | Result |\n| --- | --- | --- | --- | --- |\n")
	for _, row := range r.Rows {
		s := row.Spec
		limit := "at most "
		if !s.Lower {
			limit = "at least "
		}
		limit += format(s.Limit, s.Unit) + " " + s.Unit
		measured, result := "not measured", "fail"
		if !row.Missing {
			measured = format(row.Measured, s.Unit) + " " + s.Unit
			if row.Pass() {
				result = "pass"
			}
		}
		fmt.Fprintf(&b, "| %s | `%s` (%s) | %s | %s | %s |\n", s.NFR, s.Bench, s.Metric, measured, limit, result)
	}
	fmt.Fprintf(&b, "\n<!-- %s %s -->\n", marker, js)
	return b.String(), nil
}

func orUnknown(s string) string {
	if s == "" {
		return "an unknown CPU"
	}
	return s
}

// ParseReport reads the values out of a committed table.
func ParseReport(md string) (Values, error) {
	m := markerRe.FindStringSubmatch(md)
	if m == nil {
		return Values{}, errors.New("perfreport: the table has no machine-readable values")
	}
	var v Values
	if err := json.Unmarshal([]byte(m[1]), &v); err != nil {
		return Values{}, fmt.Errorf("perfreport: %w", err)
	}
	return v, nil
}

// MaxRegression is the worsening against the baseline that fails the job.
const MaxRegression = 0.20

// Regression is a row that got more than 20% worse than the baseline.
type Regression struct {
	Spec     *Spec
	Before   float64
	Now      float64
	Worsened float64 // as a fraction of the baseline
}

// Compare lists the rows that are more than MaxRegression worse than the baseline.
func Compare(base Values, rows []Row) []Regression {
	var out []Regression
	for _, row := range rows {
		before, ok := base.Values[key(row.Spec)]
		if !ok || row.Missing || before == 0 {
			continue
		}
		worse := (row.Measured - before) / before
		if !row.Spec.Lower {
			worse = (before - row.Measured) / before
		}
		if worse > MaxRegression {
			out = append(out, Regression{row.Spec, before, row.Measured, worse})
		}
	}
	sort.Slice(out, func(i, j int) bool { return key(out[i].Spec) < key(out[j].Spec) })
	return out
}
