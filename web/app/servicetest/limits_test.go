package servicetest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// SDD-007 section 15 check 17 (T-17): below the free-space floor the writes
// are refused with disk_low and the reads keep working.
func TestSecurity_T17_DiskLowRefusesWrites(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter()})
	project, track := m.withTrack()
	if _, err := m.addText(ui, project, track, 1, "0s", "hello"); err != nil {
		t.Fatal(err)
	}

	clip := m.write("a.mp4", clipBytes)

	m.free.Store(1 << 20) // 1 MiB free, under the default floor of 2 GiB

	diskLow := func(name string, err error) {
		t.Helper()

		se := serviceErr(err)
		if se == nil || se.Code != "E_LIMIT_EXCEEDED" || se.Details["reason"] != "disk_low" {
			t.Errorf("%s = %v, want E_LIMIT_EXCEEDED disk_low", name, err)
		}
	}

	_, err := m.addText(ui, project, track, m.head(project).Revision, "0s", "x")
	diskLow("edit", err)

	_, err = m.svc.MediaImport(m.as(ui), service.MediaImportRequest{
		ProjectID: project, Files: []service.ImportFile{{Path: clip}},
	})
	diskLow("media_import", err)

	_, err = m.svc.JobStart(m.as(ui), service.JobStartRequest{ProjectID: project, Kind: service.JobExport})
	diskLow("job_start", err)

	if _, err := m.svc.ProjectGet(m.as(ui), service.ProjectGetRequest{ProjectID: project}); err != nil {
		t.Errorf("project_get on a full disk: %v", err)
	}

	if _, err := m.svc.Lint(m.as(ui), service.LintRequest{ProjectID: project}); err != nil {
		t.Errorf("lint on a full disk: %v", err)
	}

	m.free.Store(1 << 40)

	if _, err := m.addText(ui, project, track, m.head(project).Revision, "5s", "x"); err != nil {
		t.Errorf("edit after the space came back: %v", err)
	}
}

// SDD-007 section 15 check 18 (T-18): a batch of 201 operations and one of
// 513 KiB are refused whole.
func TestSecurity_T18_BatchLimits(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, _ := m.withTrack()
	rev := m.head(project).Revision

	ops := make([]command.Operation, 201)
	for i := range ops {
		ops[i] = op("track.add", `{"kind":"visual"}`)
	}

	_, err := m.svc.Edit(m.as(ui), service.EditRequest{ProjectID: project, BaseRevision: rev, Operations: ops})
	if code(err) != service.CodePayloadTooLarge {
		t.Errorf("201 operations = %v, want E_PAYLOAD_TOO_LARGE", err)
	}

	big := op("text.add", fmt.Sprintf(`{"text":%q}`, strings.Repeat("x", 513<<10)))

	_, err = m.svc.Edit(m.as(ui), service.EditRequest{
		ProjectID: project, BaseRevision: rev, Operations: []command.Operation{big},
	})
	if code(err) != service.CodePayloadTooLarge {
		t.Errorf("513 KiB = %v, want E_PAYLOAD_TOO_LARGE", err)
	}

	if m.head(project).Revision != rev {
		t.Error("a refused batch moved the project")
	}
}

// An export larger than the ceiling ends failed with E_EXPORT_FAILED and
// details.reason size_limit (SDD-007 section 9.1), and leaves no file.
func TestExportOverTheByteCeilingFails(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter(), limits: service.Limits{MaxExportBytes: 1}})

	job, err := m.startExport(m.exportable())
	if err != nil {
		t.Fatal(err)
	}

	if job.State != service.JobFailed || job.Error == nil || job.Error.Code != "E_EXPORT_FAILED" ||
		job.Error.Details["reason"] != "size_limit" || job.Error.Details["limit"] != "export_bytes" ||
		job.Error.Details["ceiling"] != 1.0 || job.Error.Details["value"] == nil || job.Result != nil {
		t.Errorf("job = %+v", job)
	}
}

// The ceiling reaches the exporter before the first byte is written, so the
// encoder stops at it instead of filling the disk (SDD-007 section 9.1).
func TestExportInputCarriesTheByteCeiling(t *testing.T) {
	got := make(chan int64, 1)

	m := newMediaEnv(t, mediaCfg{
		exporter: exportFn(func(_ context.Context, in service.ExportInput) (*service.ExportOutput, error) {
			got <- in.MaxBytes

			return nil, errors.New("stop here")
		}),
		limits: service.Limits{MaxExportBytes: 123456},
	})

	if _, err := m.startExport(m.exportable()); err != nil {
		t.Fatal(err)
	}

	if ceiling := <-got; ceiling != 123456 {
		t.Errorf("ExportInput.MaxBytes = %d, want 123456", ceiling)
	}
}
