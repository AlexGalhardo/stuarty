package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	bundleDir      = "exports/diagnostics"
	bundleLogFiles = 5
	bundleLogBytes = 256 << 10
)

// writeBundle writes the diagnostic bundle inside the workspace and returns its
// slash path relative to it (RF-207). The file is written under a temporary
// name and renamed, mode 0600. It holds the report without paths, the versions,
// the FFmpeg configure line, the tail of the recent logs and, only when asked
// for, the document of one project. Every entry is scrubbed of the value of
// every secret variable and of the local token before it is written.
func (s *svc) writeBundle(ctx context.Context, c Caller, res *DoctorResult, project string) (string, error) {
	if s.workspace == "" {
		return "", newError("E_INTERNAL", "the diagnostic bundle needs a workspace.")
	}

	secrets := s.secretValues()
	if tok := s.localToken(); tok != "" {
		secrets = append(secrets, tok)
	}

	files := map[string][]byte{}

	report := *res
	report.Workspace, report.GC, report.Bundle = "", nil, ""
	report.Checks = s.doctorChecks(ctx, false, false)
	report.Text = doctorText(&report)

	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", internalError()
	}

	files["doctor.json"] = body
	files["doctor.txt"] = []byte(report.Text + "\n")
	files["versions.txt"] = []byte(s.versionsText())
	files["environment.txt"] = []byte(s.environmentNames())

	for name, data := range s.recentLogs() {
		files["logs/"+name] = data
	}

	if project != "" {
		row, lerr := s.loadProject(ctx, c, project)
		if lerr != nil {
			return "", lerr
		}

		files["project.json"] = row.Document
	}

	rel := bundleDir + "/doctor-" + s.bundleStamp() + ".zip"
	abs := filepath.Join(s.workspace, filepath.FromSlash(rel))

	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return "", internalError()
	}

	if err := writeZip(abs, files, secrets); err != nil {
		return "", internalError()
	}

	return rel, nil
}

func (s *svc) localToken() string {
	b, err := os.ReadFile(filepath.Join(s.workspace, "token"))
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(b))
}

func (s *svc) versionsText() string {
	var b strings.Builder

	fmt.Fprintf(&b, "stuarty %s\n", s.version)

	if r := s.media.Runner; r != nil && r.In != nil {
		fmt.Fprintf(&b, "ffmpeg %s\nffprobe %s\nffmpeg configuration: %s\n",
			r.In.Info.Version, r.In.ProbeInfo.Version, r.In.Info.ConfigLine)
	} else {
		b.WriteString("ffmpeg: not found\n")
	}

	return b.String()
}

// environmentNames lists the names of the STUARTY_ variables that are set,
// never a value: a bundle is meant to be sent to someone else.
func (s *svc) environmentNames() string {
	var names []string

	for _, kv := range s.environ() {
		name, value, ok := strings.Cut(kv, "=")
		if ok && value != "" && strings.HasPrefix(name, "STUARTY_") {
			names = append(names, name)
		}
	}

	sort.Strings(names)

	return strings.Join(names, "\n") + "\n"
}

// recentLogs reads the end of the newest log files under logs/.
func (s *svc) recentLogs() map[string][]byte {
	out := map[string][]byte{}
	dir := filepath.Join(s.workspace, "logs")

	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}

	type logFile struct {
		name string
		mod  int64
	}

	var logs []logFile

	for _, e := range entries {
		if info, ierr := e.Info(); ierr == nil && info.Mode().IsRegular() {
			logs = append(logs, logFile{e.Name(), info.ModTime().UnixNano()})
		}
	}

	sort.Slice(logs, func(i, j int) bool { return logs[i].mod > logs[j].mod })

	for i := 0; i < len(logs) && i < bundleLogFiles; i++ {
		if data := tailOf(filepath.Join(dir, logs[i].name), bundleLogBytes); data != nil {
			out[logs[i].name] = data
		}
	}

	return out
}

func tailOf(path string, n int64) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}

	defer func() { _ = f.Close() }()

	if info, serr := f.Stat(); serr == nil && info.Size() > n {
		if _, err := f.Seek(info.Size()-n, io.SeekStart); err != nil {
			return nil
		}
	}

	data, err := io.ReadAll(io.LimitReader(f, n))
	if err != nil {
		return nil
	}

	return data
}

// scrub replaces every secret in data.
func scrub(data []byte, secrets []string) []byte {
	for _, s := range secrets {
		if s != "" {
			data = bytes.ReplaceAll(data, []byte(s), []byte("[redacted]"))
		}
	}

	return data
}

func writeZip(abs string, files map[string][]byte, secrets []string) (err error) {
	tmp := abs + ".part"

	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = out.Close()
			_ = os.Remove(tmp)
		}
	}()

	zw := zip.NewWriter(out)

	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}

	sort.Strings(names)

	for _, n := range names {
		w, werr := zw.Create(n)
		if werr != nil {
			return werr
		}

		if _, werr = w.Write(scrub(files[n], secrets)); werr != nil {
			return werr
		}
	}

	if cerr := zw.Close(); cerr != nil {
		return cerr
	}

	if cerr := out.Close(); cerr != nil {
		return cerr
	}

	return os.Rename(tmp, abs)
}
