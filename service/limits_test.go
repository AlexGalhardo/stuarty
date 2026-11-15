package service

import "testing"

// SDD-007 section 9.1: the message carries both figures and the variable, and
// the details name the limit.
func TestLimitMessage(t *testing.T) {
	s := &svc{limits: Limits{}.withDefaults()}

	size := int64(123) << 30 / 10

	e := s.checkImportSize(size)
	if e == nil {
		t.Fatal("a 12.3 GiB file was accepted")
	}

	const want = "The file is 12.3 GiB; the import limit is 8 GiB (VUKEM_VIDEO_MAX_IMPORT_BYTES)."
	if e.Code != "E_LIMIT_EXCEEDED" || e.Message != want || e.Details["limit"] != "import_bytes" ||
		e.Details["variable"] != "VUKEM_VIDEO_MAX_IMPORT_BYTES" || e.Details["ceiling"] != int64(8<<30) {
		t.Errorf("error = %+v", e)
	}
}

func TestLimitsDefaults(t *testing.T) {
	l := Limits{}.withDefaults()
	if l.MaxExports != 2 || l.MaxExportsPerActor != 1 || l.MaxIngests != 2 || l.MaxTranscribes != 1 ||
		l.MaxTranscribeSeconds != 7200 || l.MaxExportBytes != 4<<30 || l.MaxImportBytes != 8<<30 {
		t.Errorf("defaults = %+v", l)
	}
}
