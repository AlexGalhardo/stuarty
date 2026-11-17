package service

import "testing"

func TestFinishedJobDoesNotKeepItsProgressMessage(t *testing.T) {
	t.Parallel()

	s := &svc{}
	row := JobRow{ID: "job_1", Kind: JobExport, State: JobSucceeded, Progress: 1, Message: "Rendering (97%)"}

	if got := s.jobView(Caller{Actor: "local:ui", Scopes: AllScopes}, row, false); got.Message != "" {
		t.Errorf("message = %q, want none for a succeeded job", got.Message)
	}

	row.State = JobRunning
	if got := s.jobView(Caller{Actor: "local:ui", Scopes: AllScopes}, row, false); got.Message != "Rendering (97%)" {
		t.Errorf("a running job lost its message: %q", got.Message)
	}
}
