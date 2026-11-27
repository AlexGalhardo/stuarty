package store

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/service"
)

func TestProductionStandaloneIsolationAndPromote(t *testing.T) {
	s := testStore(t)

	reg := service.ProductionHead{
		ID:        "prj_regular001",
		Name:      "Regular Production",
		State:     service.ProjectActive,
		Kind:      project.KindVideo,
		CreatedMs: 100,
		UpdatedMs: 100,
		CreatedBy: "local:ui",
	}
	if err := s.Productions().Create(ctx, reg); err != nil {
		t.Fatal(err)
	}

	st := service.ProductionHead{
		ID:         "prj_standal001",
		Name:       "Quick Edit",
		State:      service.ProjectActive,
		Kind:       project.KindVideo,
		Standalone: true,
		CreatedMs:  200,
		UpdatedMs:  200,
		CreatedBy:  "local:ui",
	}
	if err := s.Productions().Create(ctx, st); err != nil {
		t.Fatal(err)
	}

	// Default list and count exclude standalone sessions.
	regulars, _, err := s.Productions().List(ctx, service.ProductionQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(regulars) != 1 || regulars[0].ID != reg.ID || regulars[0].Standalone {
		t.Fatalf("default list = %+v, want only %s", regulars, reg.ID)
	}
	if n, err := s.Productions().Count(ctx, service.ProductionQuery{}); err != nil || n != 1 {
		t.Fatalf("default count = %d, %v; want 1", n, err)
	}

	// StandaloneOnly list returns only standalone sessions.
	standalones, _, err := s.Productions().List(ctx, service.ProductionQuery{StandaloneOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(standalones) != 1 || standalones[0].ID != st.ID || !standalones[0].Standalone {
		t.Fatalf("standalone list = %+v, want only %s", standalones, st.ID)
	}
	if n, err := s.Productions().Count(ctx, service.ProductionQuery{StandaloneOnly: true}); err != nil || n != 1 {
		t.Fatalf("standalone count = %d, %v; want 1", n, err)
	}

	// IncludeStandalone returns both.
	all, _, err := s.Productions().List(ctx, service.ProductionQuery{IncludeStandalone: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("include standalone list = %d, want 2", len(all))
	}

	// Promote flips standalone = 0 and seals the production context.
	promotedCtx := project.Context{
		Name:        "Promoted Project",
		Description: strings.Repeat("Promoted from standalone edit with full context. ", 2),
		Goal: project.Goal{
			Specific:   "Publish the edited video cleanly.",
			Measurable: "Complete export and upload this week.",
			Achievable: "Timeline is already edited and ready.",
			Relevant:   "Directly supports the release launch.",
			TimeBound:  "Finish before Friday evening.",
			Due:        "2026-12-20",
		},
	}
	if err := s.Productions().Promote(ctx, st.ID, promotedCtx, project.KindVideo, 300); err != nil {
		t.Fatal(err)
	}

	got, err := s.Productions().Get(ctx, st.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Standalone || got.Name != "Promoted Project" || got.Description != promotedCtx.Description {
		t.Fatalf("promoted row = %+v", got)
	}

	regulars, _, err = s.Productions().List(ctx, service.ProductionQuery{})
	if err != nil || len(regulars) != 2 {
		t.Fatalf("after promote default list = %d, %v; want 2", len(regulars), err)
	}
	standalones, _, err = s.Productions().List(ctx, service.ProductionQuery{StandaloneOnly: true})
	if err != nil || len(standalones) != 0 {
		t.Fatalf("after promote standalone list = %d, %v; want 0", len(standalones), err)
	}
}

func TestStandaloneServiceLifecycle(t *testing.T) {
	s := testStore(t)
	svc, err := service.New(service.Options{Store: s})
	if err != nil {
		t.Fatal(err)
	}

	localCtx := service.WithCaller(ctx, service.Caller{
		Actor:   "local:ui",
		Scopes:  service.AllScopes,
		Browser: true,
	})
	readOnlyCtx := service.WithCaller(ctx, service.Caller{
		Actor:  "key:key_readonly01",
		Scopes: []string{service.ScopeRead},
	})

	// Read-only caller cannot create a standalone video.
	if _, err := svc.StandaloneVideoCreate(
		readOnlyCtx,
		service.StandaloneVideoCreateRequest{Preset: "16:9"},
	); err == nil {
		t.Fatal("expected read-only caller to be forbidden from StandaloneVideoCreate")
	}

	// Create a blank standalone video session with zero pre-created projects.
	created, err := svc.StandaloneVideoCreate(localCtx, service.StandaloneVideoCreateRequest{
		Name:   "Quick demo clip",
		Preset: "16:9",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Created || !created.Standalone || created.Width != 1920 || created.Height != 1080 ||
		created.TrackID == "" {
		t.Fatalf("unexpected created standalone result: %+v", created)
	}

	// ProjectList stays empty; StandaloneVideoList shows it.
	projList, err := svc.ProjectList(localCtx, service.ProjectListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(projList.Projects) != 0 {
		t.Fatalf("ProjectList should exclude standalone sessions, got %+v", projList.Projects)
	}

	stList, err := svc.StandaloneVideoList(localCtx, service.StandaloneVideoListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(stList.Projects) != 1 || stList.Projects[0].ProjectID != created.ProjectID ||
		!stList.Projects[0].Standalone {
		t.Fatalf("StandaloneVideoList = %+v", stList.Projects)
	}

	// ProjectGet reports Standalone: true.
	got, err := svc.ProjectGet(localCtx, service.ProjectGetRequest{ProjectID: created.ProjectID})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Standalone || got.TimelineID != created.TimelineID {
		t.Fatalf("ProjectGet = %+v", got)
	}

	// Empty upload body is rejected cleanly before writing any row.
	if _, err := svc.StandaloneVideoUpload(localCtx, service.StandaloneVideoUploadRequest{
		FileName: "empty.mp4",
		Body:     bytes.NewReader(nil),
		Size:     0,
	}); err == nil {
		t.Fatal("expected error for empty upload")
	}

	// Promote converts the standalone session into a full project.
	promoted, err := svc.StandalonePromote(localCtx, service.StandalonePromoteRequest{
		ProjectID:   created.ProjectID,
		Name:        "Launch Video",
		Description: strings.Repeat("A full production description for the standalone video edit. ", 2),
		Goal: service.ProjectGoal{
			Specific:   "Publish a concise product launch short video.",
			Measurable: "Reach five thousand views in the first week.",
			Achievable: "All footage is already edited in the standalone timeline.",
			Relevant:   "Drives creator onboarding for the new release.",
			TimeBound:  "Export and schedule within three days.",
			Due:        "2026-12-20",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !promoted.Updated || !promoted.Complete || promoted.Name != "Launch Video" {
		t.Fatalf("promoted = %+v", promoted)
	}

	// Now it appears in ProjectList and disappears from StandaloneVideoList.
	projList, err = svc.ProjectList(localCtx, service.ProjectListRequest{})
	if err != nil || len(projList.Projects) != 1 || projList.Projects[0].ProjectID != created.ProjectID {
		t.Fatalf("after promote ProjectList = %+v, err=%v", projList, err)
	}
	stList, err = svc.StandaloneVideoList(localCtx, service.StandaloneVideoListRequest{})
	if err != nil || len(stList.Projects) != 0 {
		t.Fatalf("after promote StandaloneVideoList = %+v, err=%v", stList, err)
	}

	// Promoting again fails with E_INVALID_STATE.
	if _, err := svc.StandalonePromote(localCtx, service.StandalonePromoteRequest{
		ProjectID:   created.ProjectID,
		Name:        "Launch Video",
		Description: strings.Repeat("A full production description for the standalone video edit. ", 2),
		Goal: service.ProjectGoal{
			Specific:   "Publish a concise product launch short video.",
			Measurable: "Reach five thousand views in the first week.",
			Achievable: "All footage is already edited in the standalone timeline.",
			Relevant:   "Drives creator onboarding for the new release.",
			TimeBound:  "Export and schedule within three days.",
			Due:        "2026-12-20",
		},
	}); err == nil {
		t.Fatal("expected E_INVALID_STATE on second promote")
	}
}
