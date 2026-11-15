package servicetest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/service"
)

// requireLimit is the P4-SRV-003 contract of SDD-007 section 9.1: a limit error carries
// details.limit, details.value and details.ceiling, as numbers once serialised.
func requireLimit(t *testing.T, what string, err error, wantCode, wantLimit string) *service.Error {
	t.Helper()

	se := serviceErr(err)
	if se == nil || se.Code != wantCode {
		t.Fatalf("%s = %v, want %s", what, err, wantCode)
	}

	raw, merr := json.Marshal(se.Details)
	if merr != nil {
		t.Fatal(merr)
	}

	var d struct {
		Limit   string   `json:"limit"`
		Value   *float64 `json:"value"`
		Ceiling *float64 `json:"ceiling"`
	}

	if json.Unmarshal(raw, &d) != nil || d.Limit != wantLimit || d.Value == nil || d.Ceiling == nil {
		t.Errorf("%s: details = %s, want limit %q with a numeric value and ceiling", what, raw, wantLimit)
	}

	return se
}

// P4-SRV-003: the disk guard of every write operation, including the BETA ones.
func TestLimitDiskLowCarriesValueAndCeiling(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter(), beta: true})
	project, track := m.withTrack()
	clip := m.write("a.mp4", clipBytes)
	bundle := m.exportBundle(project, nil).Result.Export.Path
	m.free.Store(1 << 20)

	_, err := m.addText(ui, project, track, m.head(project).Revision, "0s", "x")
	requireLimit(t, "edit", err, "E_LIMIT_EXCEEDED", "free_bytes")

	_, err = m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
		ProjectID: project, Files: []service.ImportFile{{Path: clip}},
	})
	requireLimit(t, "media_import", err, "E_LIMIT_EXCEEDED", "free_bytes")

	_, err = m.svc.JobStart(m.as(ui), service.JobStartRequest{ProjectID: project, Kind: service.JobExport})
	requireLimit(t, "job_start", err, "E_LIMIT_EXCEEDED", "free_bytes")

	data, rerr := os.ReadFile(bundle)
	if rerr != nil {
		t.Fatal(rerr)
	}

	outside := filepath.Join(m.files, "b.vvbundle")
	if werr := os.WriteFile(outside, data, 0o600); werr != nil {
		t.Fatal(werr)
	}

	_, err = m.importBundle(outside, true)
	requireLimit(t, "bundle_import", err, "E_LIMIT_EXCEEDED", "free_bytes")
}

func TestLimitBatchCarriesValueAndCeiling(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, _ := m.withTrack()
	rev := m.head(project).Revision

	ops := make([]command.Operation, 201)
	for i := range ops {
		ops[i] = op("track.add", `{"kind":"visual"}`)
	}

	_, err := m.svc.Edit(m.as(ui), service.EditRequest{ProjectID: project, BaseRevision: rev, Operations: ops})
	requireLimit(t, "201 operations", err, service.CodePayloadTooLarge, "batch_operations")

	big := op("text.add", fmt.Sprintf(`{"text":%q}`, strings.Repeat("x", 513<<10)))

	_, err = m.svc.Edit(m.as(ui), service.EditRequest{
		ProjectID: project, BaseRevision: rev, Operations: []command.Operation{big},
	})
	requireLimit(t, "513 KiB", err, service.CodePayloadTooLarge, "batch_bytes")
}

func TestLimitExportQueuesCarryValueAndCeiling(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter(), noQueue: true, limits: service.Limits{MaxExports: 1}})
	project := m.exportable()

	start := func(actor string) error {
		_, err := m.svc.JobStart(m.as(actor), service.JobStartRequest{ProjectID: project, Kind: "export"})

		return err
	}

	if err := start("local:a"); err != nil {
		t.Fatal(err)
	}

	se := requireLimit(t, "second export", start("local:b"), service.CodeQueueFull, "exports")
	if se.Details["value"] != 1 || se.Details["ceiling"] != 1 {
		t.Errorf("details = %v, want value 1 and ceiling 1", se.Details)
	}

	per := newMediaEnv(t, mediaCfg{exporter: goodExporter(), noQueue: true, limits: service.Limits{MaxExports: 10}})
	project = per.exportable()

	for range 3 { // one running and two queued
		if _, err := per.svc.JobStart(
			per.as("local:a"),
			service.JobStartRequest{ProjectID: project, Kind: "export"},
		); err != nil {
			t.Fatal(err)
		}
	}

	_, err := per.svc.JobStart(per.as("local:a"), service.JobStartRequest{ProjectID: project, Kind: "export"})

	se = requireLimit(t, "fourth export of one actor", err, service.CodeQueueFull, "exports_per_key")
	if se.Details["ceiling"] != 3 {
		t.Errorf("details = %v, want ceiling 3", se.Details)
	}
}

// The size watchdog ends an export mid-run: the engine reports reason size_limit and the service
// names the limit and the ceiling. The engine does not say how large the file had become.
func TestLimitExportEndedByTheWatchdogNamesTheLimit(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{
		exporter: exportFn(func(context.Context, service.ExportInput) (*service.ExportOutput, error) {
			return nil, &ffmpeg.Error{
				Code:    "E_EXPORT_FAILED",
				Reason:  "size_limit",
				Message: "The exported file passed the size limit.",
			}
		}),
		limits: service.Limits{MaxExportBytes: 123456},
	})

	job, err := m.startExport(m.exportable())
	if err != nil {
		t.Fatal(err)
	}

	d := job.Error.Details
	if job.State != service.JobFailed || job.Error.Code != "E_EXPORT_FAILED" || d["reason"] != "size_limit" ||
		d["limit"] != "export_bytes" || d["ceiling"] != 123456.0 {
		t.Errorf("job = %+v, details %v", job, d)
	}
}

// SDD-007 section 9.1: with the ingests running and the queue behind them full, the next file of
// a MediaImport is E_QUEUE_FULL for that file, naming the limit, the jobs and the ceiling.
func TestLimitIngestQueueCarriesValueAndCeiling(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, limits: service.Limits{MaxIngests: 1, MaxQueuedIngests: 1}})
	project := m.newProject("p")

	var files []service.ImportFile
	for i := range 3 {
		files = append(
			files,
			service.ImportFile{Path: m.write(fmt.Sprintf("clip%d.mp4", i), clipBytes+strings.Repeat("x", i+1))},
		)
	}

	res, err := m.svc.MediaImport(m.as(ui), service.MediaImportRequest{ProjectID: project, Files: files})
	if err != nil {
		t.Fatal(err)
	}

	if res.Failed != 1 || res.Assets[0].Error != nil || res.Assets[1].Error != nil {
		t.Fatalf("assets = %+v, want only the third refused", res.Assets)
	}

	requireLimit(t, "third file", res.Assets[2].Error, service.CodeQueueFull, "ingests")
	if res.Assets[2].Error.Details["ceiling"] != 2 {
		t.Errorf("details = %v, want ceiling 2", res.Assets[2].Error.Details)
	}
}

func TestLimitViewWidthCarriesValueAndCeiling(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true})
	project := m.exportable()

	_, err := m.svc.ViewFrame(m.as(ui), service.ViewFrameRequest{ProjectID: project, At: "0s", Width: 100000})
	requireLimit(t, "width", err, "E_LIMIT_EXCEEDED", "width")
}

// The size and operation ceilings of an OpenTimelineIO import (SDD-001 section 9).
func TestLimitOtioImportCarriesValueAndCeiling(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	target := m.newProject("target")

	huge := `{"OTIO_SCHEMA":"Timeline.1","name":"x","metadata":"` + strings.Repeat("a", 17<<20) + `"}`
	_, err := m.otioImport(target, m.head(target).Revision, m.write("huge.otio", huge))
	requireLimit(t, "17 MiB file", err, service.CodePayloadTooLarge, "otio_bytes")

	tracks := make([]any, 201)
	for i := range tracks {
		tracks[i] = map[string]any{"OTIO_SCHEMA": "Track.1", "name": "t", "kind": "Video", "children": []any{}}
	}

	root := map[string]any{
		"OTIO_SCHEMA": "Timeline.1", "name": "many",
		"tracks": map[string]any{"OTIO_SCHEMA": "Stack.1", "children": tracks},
	}

	data, merr := json.Marshal(root)
	if merr != nil {
		t.Fatal(merr)
	}

	_, err = m.otioImport(target, m.head(target).Revision, m.write("many.otio", string(data)))
	requireLimit(t, "201 tracks", err, service.CodePayloadTooLarge, "batch_operations")
}

// A bundle over a cap says which cap, what it saw and the ceiling.
func TestLimitBundleCarriesValueAndCeiling(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	project, _ := src.bundleProject()

	raw, err := os.ReadFile(src.exportBundle(project, nil).Result.Export.Path)
	if err != nil {
		t.Fatal(err)
	}

	patchDeclaredSize(t, raw, "manifest.json", 8<<20)

	dst := newMediaEnv(t, mediaCfg{})

	path := filepath.Join(dst.files, "lying.vvbundle")
	if werr := os.WriteFile(path, raw, 0o600); werr != nil {
		t.Fatal(werr)
	}

	_, err = dst.importBundle(path, false)
	requireLimit(t, "manifest over its cap", err, "E_LIMIT_EXCEEDED", "bundle_entry_bytes")
}
