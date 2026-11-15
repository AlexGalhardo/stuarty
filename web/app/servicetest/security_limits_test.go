package servicetest

import (
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
)

// T-18, SDD-007 section 15 check 17: with VUKEM_VIDEO_MAX_EXPORTS=1 the second
// concurrent export is E_QUEUE_FULL with details.running naming the first, and
// the system keeps working: once the first is cancelled a new export starts.
func TestSecurity_T18_ExportConcurrencyCeilings(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter(), noQueue: true, limits: service.Limits{MaxExports: 1}})
	project := m.exportable()

	first, err := m.svc.JobStart(m.as("local:a"), service.JobStartRequest{ProjectID: project, Kind: "export"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.svc.JobStart(m.as("local:b"), service.JobStartRequest{ProjectID: project, Kind: "export"})

	se := serviceError(t, err)
	if se.Code != service.CodeQueueFull || se.HTTPStatus() != 429 || !se.Retryable {
		t.Fatalf("the second export = %+v, want a retryable 429 E_QUEUE_FULL", se)
	}

	running, _ := se.Details["running"].([]string)
	if len(running) != 1 || running[0] != first.JobID {
		t.Errorf("details.running = %v, want [%s]", se.Details["running"], first.JobID)
	}

	if _, err := m.svc.JobCancel(m.as("local:a"), service.JobCancelRequest{JobID: first.JobID}); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	if _, err := m.svc.JobStart(
		m.as("local:b"),
		service.JobStartRequest{ProjectID: project, Kind: "export"},
	); err != nil {
		t.Errorf("an export after the first was cancelled: %v", err)
	}
}
