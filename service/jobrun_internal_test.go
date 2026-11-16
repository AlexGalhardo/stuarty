package service

import "testing"

func TestQueueKindFor(t *testing.T) {
	cases := map[string]string{
		JobIngest:     QueueKindIngest,
		JobTranscribe: QueueKindIngest,
		JobExport:     QueueKindExport,
		"frame":       QueueKindFrame,
		"unknown":     QueueKindFrame,
	}

	for kind, want := range cases {
		if got := queueKindFor(kind); got != want {
			t.Errorf("queueKindFor(%q) = %q, want %q", kind, got, want)
		}
	}
}
