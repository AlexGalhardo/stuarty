package mcp

import "testing"

// The tests below belong to tools whose Service methods do not exist yet (see
// Pending). Each names its task; replace the skip with the test when the tool
// is added to handlers().

func TestSecurity_T07_TranscriptDelimitedAndTruncated(t *testing.T) {
	t.Skip("needs media_inspect and its transcript text: task P1-SRV-004 (SDD-007 section 8.2)")
}

func TestMediaImportWaitsAndReportsFailures(t *testing.T) {
	t.Skip("needs media_import: task P1-SRV-004")
}

func TestJobGetWaitMsReturnsWhileAnExportRuns(t *testing.T) {
	t.Skip("needs job_start and job_get over a running export: tasks P1-SRV-004 and P1-RND-004")
}

func TestViewReturnsAnImageOnlyFromView(t *testing.T) {
	t.Skip("needs view: task P1-RND-005")
}

func TestStdioOwnerIsReachedByTheCLIAndOpen(t *testing.T) {
	t.Skip("needs the owner startup (listener, worker, runtime.json) of the server: P1-SRV-005 follow-up")
}
