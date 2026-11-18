package servicetest

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/ingest"
	"github.com/AlexGalhardo/vukem-video/service"
)

func networkCaller(actor, owner string, scopes []string) context.Context {
	return service.WithCaller(context.Background(), service.Caller{Actor: actor, OwnerUserID: owner, Scopes: scopes})
}

func assertNamesNoVariable(t *testing.T, se *service.Error) {
	t.Helper()

	if _, has := se.Details["variable"]; has {
		t.Errorf("details.variable is present: %+v", se.Details)
	}

	for _, text := range []string{se.Message, se.Hint} {
		if strings.Contains(text, "VUKEM_VIDEO_") {
			t.Errorf("the text names a variable: %q", text)
		}
	}

	if se.Details["limit"] == nil || se.Details["ceiling"] == nil {
		t.Errorf("the limit and the ceiling must stay: %+v", se.Details)
	}
}

// SDD-007 section 9.1: in network mode a caller who is not an administrator
// gets the limit and its figures but not the name of the variable; the
// administrator and local mode get it (P2-SRV-006 subtask 3).
func TestLimitErrorsHideTheVariableFromNonAdmins(t *testing.T) {
	m := newMediaEnv(
		t,
		mediaCfg{limits: service.Limits{MaxImportBytes: 20, MaxExports: 1}, noQueue: true, exporter: goodExporter()},
	)

	member := networkCaller("user:alice", "alice", []string{
		service.ScopeRead, service.ScopeEdit, service.ScopeMedia, service.ScopeRender,
	})
	admin := networkCaller("user:root", "root", service.AllScopes)

	for name, who := range map[string]context.Context{"member": member, "admin": admin} {
		created, err := m.svc.ProjectCreate(who, service.ProjectCreateRequest{Name: name})
		if err != nil {
			t.Fatal(err)
		}

		project := created.ProjectID
		hidden := name == "member"

		_, err = m.svc.MediaImport(who, service.MediaImportRequest{
			ProjectID: project, Files: []service.ImportFile{{Path: m.write(name+".mp4", clipBytes)}},
		})

		se := serviceError(t, err)
		if hidden {
			assertNamesNoVariable(t, se)
		} else if se.Details["variable"] != "VUKEM_VIDEO_MAX_IMPORT_BYTES" ||
			!strings.Contains(se.Message, "(VUKEM_VIDEO_MAX_IMPORT_BYTES)") {
			t.Errorf("%s: the administrator lost the variable: %+v", name, se)
		}

		track, err := m.svc.Edit(who, service.EditRequest{
			ProjectID: project, Operations: []command.Operation{op("track.add", `{"kind":"visual","name":"V"}`)},
		})
		if err != nil {
			t.Fatal(err)
		}

		_, err = m.svc.Edit(who, service.EditRequest{
			ProjectID: project, BaseRevision: track.Revision, Operations: []command.Operation{op("text.add",
				`{"track_id":"`+track.Created[0]+`","start":"0s","duration":"1s","text":"hi"}`)},
		})
		if err != nil {
			t.Fatal(err)
		}

		var queueFull error

		for range 2 {
			_, queueFull = m.svc.JobStart(who, service.JobStartRequest{ProjectID: project, Kind: "export"})
		}

		se = serviceError(t, queueFull)
		if se.Code != service.CodeQueueFull {
			t.Fatalf("%s: code = %s, want a full queue", name, se.Code)
		}

		if hidden {
			assertNamesNoVariable(t, se)
		} else if se.Details["variable"] != "VUKEM_VIDEO_MAX_EXPORTS" {
			t.Errorf("%s: the administrator lost the variable: %+v", name, se.Details)
		}
	}
}

// SDD-011 section 2.6: the error an ingest limit leaves on the asset is the
// error of its job, with the same details and hint, redacted for the caller
// the same way (a member of a network server never sees the variable).
func TestFailedIngestLimitIsTheSameOnTheAssetAndTheJob(t *testing.T) {
	limitErr := func(context.Context, ingest.Input) (*ingest.Result, error) {
		return nil, &ffmpeg.Error{
			Code: "E_LIMIT_EXCEEDED", Reason: "import_seconds", Message: "The media is 9000 seconds long.",
			Hint: "Ask the operator to raise VUKEM_VIDEO_MAX_IMPORT_SECONDS, or cut the file first.",
			Details: map[string]any{
				"limit": "import_seconds", "value": int64(9000), "ceiling": int64(7200),
				"variable": "VUKEM_VIDEO_MAX_IMPORT_SECONDS",
			},
		}
	}

	member := networkCaller("user:alice", "alice", []string{
		service.ScopeRead, service.ScopeEdit, service.ScopeMedia, service.ScopeRender,
	})
	admin := networkCaller("user:root", "root", service.AllScopes)

	for name, who := range map[string]context.Context{"member": member, "admin": admin} {
		m := newMediaEnv(t, mediaCfg{ingest: limitErr})

		created, err := m.svc.ProjectCreate(who, service.ProjectCreateRequest{Name: name})
		if err != nil {
			t.Fatal(err)
		}

		imported, err := m.svc.MediaImport(who, service.MediaImportRequest{
			ProjectID: created.ProjectID, Files: []service.ImportFile{{Path: m.write(name+".mp4", clipBytes)}},
		})
		if err != nil {
			t.Fatal(err)
		}

		job, err := m.svc.JobGet(who, service.JobGetRequest{JobID: imported.Assets[0].JobID})
		if err != nil || job.Error == nil {
			t.Fatalf("%s: job = %+v, %v", name, job, err)
		}

		list, err := m.svc.MediaList(who, service.MediaListRequest{ProjectID: created.ProjectID})
		if err != nil || len(list.Assets) != 1 || list.Assets[0].Error == nil {
			t.Fatalf("%s: list = %+v, %v", name, list, err)
		}

		got, want := list.Assets[0].Error, job.Error
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: asset error %+v differs from job error %+v", name, got, want)
		}

		if name == "member" {
			assertNamesNoVariable(t, got)
		} else if got.Details["variable"] != "VUKEM_VIDEO_MAX_IMPORT_SECONDS" {
			t.Errorf("admin lost the variable: %+v", got)
		}
	}
}
