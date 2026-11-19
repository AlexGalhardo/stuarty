package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// SDD-007 section 9.1: the message carries both figures and the variable, and
// the details name the limit.
func TestLimitMessage(t *testing.T) {
	s := &svc{limits: Limits{}.withDefaults()}

	size := int64(123) << 30 / 10

	e := s.checkImportSize(size)
	if e == nil {
		t.Fatal("a 12.3 GiB file was accepted")
	}

	const want = "The file is 12.3 GiB; the import limit is 8 GiB (STUARTY_MAX_IMPORT_BYTES)."
	if e.Code != "E_LIMIT_EXCEEDED" || e.Message != want || e.Details["limit"] != "import_bytes" ||
		e.Details["variable"] != "STUARTY_MAX_IMPORT_BYTES" || e.Details["ceiling"] != int64(8<<30) {
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

func TestForCallerRemovesEveryVariableName(t *testing.T) {
	e := newError(
		"E_LIMIT_EXCEEDED",
		"The audio is 9 seconds; the limit is 5 seconds (STUARTY_MAX_TRANSCRIBE_SECONDS).",
	).
		withHint("transcribe a shorter file; the person running the server can raise "+
			"STUARTY_MAX_TRANSCRIBES or STUARTY_MAX_TRANSCRIBES_PER_KEY").
		withDetail("variable", "STUARTY_MAX_TRANSCRIBE_SECONDS").
		withDetail("limit", "transcribe_seconds")

	member := Caller{Actor: "user:a", OwnerUserID: "a", Scopes: []string{ScopeRead}}

	got := forCaller(member, e)
	if got.Message != "The audio is 9 seconds; the limit is 5 seconds." || got.Hint != "transcribe a shorter file" ||
		got.Details["variable"] != nil || got.Details["limit"] != "transcribe_seconds" {
		t.Errorf("member sees %+v", got)
	}

	if e.Details["variable"] == nil || e.Message == got.Message {
		t.Error("the shared error was modified")
	}

	for name, c := range map[string]Caller{
		"admin": {Actor: "user:r", OwnerUserID: "r", Scopes: AllScopes},
		"local": {Actor: "local:ui", Scopes: []string{ScopeRead}},
	} {
		if forCaller(c, e) != e {
			t.Errorf("%s lost the variable", name)
		}
	}
}

// An event reaches every subscriber of the project, so a failed job's event
// carries no variable name, while JobGet keeps it for an administrator.
func TestEventErrorCarriesNoVariable(t *testing.T) {
	stored := `{"code":"E_LIMIT_EXCEEDED","message":"The file is 9 GiB; the import limit is 8 GiB (STUARTY_MAX_IMPORT_BYTES).",` +
		`"hint":"import a smaller file; the person running the server can raise STUARTY_MAX_IMPORT_BYTES",` +
		`"details":{"limit":"import_bytes","variable":"STUARTY_MAX_IMPORT_BYTES"}}`

	var got []Event

	s := &svc{publish: func(e Event) { got = append(got, e) }, now: time.Now}
	project := "prj_0000000001"

	s.emitJob(context.Background(), &JobRow{
		ID: "job_0000000001", Kind: JobExport, ProjectID: &project, State: "failed", Error: json.RawMessage(stored),
	})

	if len(got) != 1 || got[0].Error == nil {
		t.Fatalf("events = %+v", got)
	}

	e := got[0].Error
	if _, has := e.Details["variable"]; has || strings.Contains(e.Message+e.Hint, "STUARTY_") ||
		e.Details["limit"] != "import_bytes" {
		t.Errorf("the event error = %+v", e)
	}

	admin := Caller{Actor: "user:r", OwnerUserID: "r", Scopes: AllScopes}
	if full := forCaller(admin, parseError(json.RawMessage(stored))); full.Details["variable"] == nil {
		t.Error("an administrator lost the variable in JobGet")
	}
}
