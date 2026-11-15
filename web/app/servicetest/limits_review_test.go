package servicetest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
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
		if _, err := per.svc.JobStart(per.as("local:a"), service.JobStartRequest{ProjectID: project, Kind: "export"}); err != nil {
			t.Fatal(err)
		}
	}

	_, err := per.svc.JobStart(per.as("local:a"), service.JobStartRequest{ProjectID: project, Kind: "export"})

	se = requireLimit(t, "fourth export of one actor", err, service.CodeQueueFull, "exports_per_key")
	if se.Details["ceiling"] != 3 {
		t.Errorf("details = %v, want ceiling 3", se.Details)
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
