package servicetest

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/ingest"
	"github.com/AlexGalhardo/vukem-video/engine/ingest/mediagen"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/templates"
)

func (m *mediaEnv) seedStarters() int {
	m.t.Helper()

	n, err := service.SeedStarterTemplates(context.Background(), m.store, m.clock.now().UnixMilli())
	if err != nil {
		m.t.Fatal(err)
	}

	return n
}

func (m *mediaEnv) templateList(ctx context.Context) []service.TemplateSummary {
	m.t.Helper()

	res, err := m.svc.TemplateList(ctx, service.TemplateListRequest{Limit: 100})
	if err != nil {
		m.t.Fatal(err)
	}

	return res.Templates
}

// fillOf fills every slot of a template: the asset for media, a text for text,
// words for a caption.
func fillOf(t *testing.T, tpl service.TemplateSummary, asset string) map[string]service.SlotValue {
	t.Helper()

	fill := map[string]service.SlotValue{}

	for _, sl := range tpl.Slots {
		switch sl.Kind {
		case "media":
			fill[sl.Name] = service.SlotValue{AssetID: asset}
		case "text":
			text := "Filled " + sl.Name
			fill[sl.Name] = service.SlotValue{Text: &text}
		case "caption":
			var v service.SlotValue
			if err := json.Unmarshal([]byte(
				`{"words":[{"text":"Hello","start":"0s","duration":"1s"},{"text":"there","start":"1s","duration":"1s"}]}`,
			), &v); err != nil {
				t.Fatal(err)
			}

			fill[sl.Name] = v
		default:
			t.Fatalf("slot %s has the kind %q", sl.Name, sl.Kind)
		}
	}

	return fill
}

// longIngest is the fake ingest with a 30 second asset, long enough for every
// media slot of a starter template.
func longIngest(workspace func() string) service.Ingester {
	return func(_ context.Context, in ingest.Input) (*ingest.Result, error) {
		res, err := fakeResult(workspace(), in)
		if err != nil {
			return nil, err
		}

		res.Facts.FrameCount, res.Facts.SampleCount, res.Summary.DurationMS = 900, 1440000, 30000

		return res, nil
	}
}

// ADR-0022 item 8: each starter validates, lists its slots as text, and
// instantiated with media of the right length it has no unfilled slot.
func TestStarterTemplates(t *testing.T) {
	var m *mediaEnv

	m = newMediaEnv(t, mediaCfg{ingest: longIngest(func() string { return m.ws })})
	m.seedStarters()

	list := m.templateList(m.as(ui))
	if len(list) != 3 {
		t.Fatalf("the library lists %d templates, want 3", len(list))
	}

	library := m.newProject("library")
	asset := m.importOne(library, m.write("a.mp4", clipBytes)).Assets[0].AssetID

	starters, err := templates.Starters()
	if err != nil {
		t.Fatal(err)
	}

	for _, s := range starters {
		doc, derr := timeline.Decode(s.Document)
		if derr != nil || len(timeline.Validate(doc)) != 0 {
			t.Fatalf("%s does not validate: %v", s.ID, derr)
		}
	}

	for _, tpl := range list {
		t.Run(tpl.Name, func(t *testing.T) {
			if len(tpl.Slots) == 0 || tpl.Description == "" {
				t.Fatalf("the summary has no slots or description: %+v", tpl)
			}

			got, err := m.svc.TemplateList(m.as(ui), service.TemplateListRequest{Limit: 100})
			if err != nil {
				t.Fatal(err)
			}

			for _, sl := range tpl.Slots {
				if !strings.Contains(got.Text, sl.Name+":"+sl.Kind) {
					t.Errorf("the listing text does not name the slot %s:%s", sl.Name, sl.Kind)
				}
			}

			open, err := m.svc.TemplateInstantiate(m.as(ui), service.TemplateInstantiateRequest{
				TemplateID: tpl.TemplateID, Name: "Open " + tpl.Name,
			})
			if err != nil || len(open.Unfilled) == 0 {
				t.Fatalf("instantiated with no fill = %+v, %v", open, err)
			}

			res, err := m.svc.TemplateInstantiate(m.as(ui), service.TemplateInstantiateRequest{
				TemplateID: tpl.TemplateID, Name: "Filled " + tpl.Name, Fill: fillOf(t, tpl, asset),
			})
			if err != nil {
				t.Fatal(err)
			}

			if len(res.Unfilled) != 0 {
				t.Errorf("still unfilled: %v", res.Unfilled)
			}

			lint, err := m.svc.Lint(m.as(ui), service.LintRequest{ProjectID: res.ProjectID})
			if err != nil {
				t.Fatal(err)
			}

			for _, f := range lint.Findings {
				if f.Code == "L_SLOT_UNFILLED" {
					t.Errorf("L_SLOT_UNFILLED after the fill: %+v", f)
				}
			}

			if lint.Errors != 0 {
				t.Errorf("lint errors = %d: %+v", lint.Errors, lint.Findings)
			}
		})
	}
}

// A new workspace lists exactly 3 templates and starting it again lists 3.
func TestStarterIdempotent(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})

	if n := m.templateList(m.as(ui)); len(n) != 0 {
		t.Fatalf("a workspace that was not seeded lists %d templates", len(n))
	}

	if n := m.seedStarters(); n != 3 {
		t.Errorf("first start added %d, want 3", n)
	}

	before := m.templateList(m.as(ui))

	for range 2 {
		if n := m.seedStarters(); n != 0 {
			t.Errorf("a later start added %d templates", n)
		}
	}

	after := m.templateList(m.as(ui))
	if len(after) != 3 || !slices.EqualFunc(before, after, func(a, b service.TemplateSummary) bool {
		return a.TemplateID == b.TemplateID
	}) {
		t.Errorf("the library after later starts = %+v", after)
	}

	// A template the person saved is not touched by the seeding.
	saved := m.saveTemplate(m.templateProject())
	m.seedStarters()

	got := m.templateList(m.as(ui))
	listed := slices.ContainsFunc(got, func(s service.TemplateSummary) bool {
		return s.TemplateID == saved.Template.TemplateID
	})

	if len(got) != 4 || !listed {
		t.Errorf("the library = %+v", got)
	}
}

// In network mode the starters are everyone's; a template with no owner that is
// not a starter belongs to the local workspace and no user sees it.
func TestStartersAreSharedAndNothingElseWithoutAnOwner(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	m.seedStarters()

	local := m.saveTemplate(m.templateProject()).Template.TemplateID
	alice := service.WithCaller(context.Background(), service.Caller{
		Actor: "user:alice", OwnerUserID: "alice", Scopes: service.AllScopes,
	})

	got := m.templateList(alice)
	if len(got) != 3 {
		t.Fatalf("alice lists %d templates, want the 3 starters: %+v", len(got), got)
	}

	for _, tpl := range got {
		if tpl.TemplateID == local {
			t.Errorf("alice lists the local template %s", local)
		}
	}

	if _, err := m.svc.TemplateInstantiate(alice, service.TemplateInstantiateRequest{
		TemplateID: got[0].TemplateID, Name: "Mine",
	}); err != nil {
		t.Errorf("alice instantiates a starter: %v", err)
	}

	if _, err := m.svc.TemplateInstantiate(alice, service.TemplateInstantiateRequest{
		TemplateID: local, Name: "Not mine",
	}); code(err) != "E_NOT_FOUND" {
		t.Errorf("alice instantiates a local template = %v, want E_NOT_FOUND", err)
	}
}

// ADR-0022 item 8: each starter filled with real media exports through the
// real engine and the file passes verification.
func TestStarterTemplatesExportAndVerify(t *testing.T) {
	if raceEnabled {
		t.Skip("three real renders of 6 to 15 seconds take minutes under the race detector; the engine tests run them")
	}

	m := newMediaEnv(t, mediaCfg{realIngest: true, realExport: true})
	inst := pinnedFFmpeg(t)
	m.seedStarters()

	gen := mediagen.Gen{
		Dir: m.files,
		Run: func(ctx context.Context, dir string, args ...string) (string, error) {
			res, err := inst.FFmpeg.Run(ctx, dir, args...)

			return res.Stderr, err
		},
	}

	clip, err := gen.Clip(context.Background(), "source.mp4", 360, 640, 30, 16)
	if err != nil {
		t.Fatal(err)
	}

	library := m.newProject("library")
	res := m.importOne(library, clip)

	if res.Failed != 0 || res.Assets[0].State != "ready" {
		t.Fatalf("import = %+v", res)
	}

	asset := res.Assets[0].AssetID

	for _, tpl := range m.templateList(m.as(ui)) {
		t.Run(tpl.Name, func(t *testing.T) {
			made, err := m.svc.TemplateInstantiate(m.as(ui), service.TemplateInstantiateRequest{
				TemplateID: tpl.TemplateID, Name: "Export " + tpl.Name, Fill: fillOf(t, tpl, asset),
			})
			if err != nil || len(made.Unfilled) != 0 {
				t.Fatalf("instantiate = %+v, %v", made, err)
			}

			job, err := m.startExport(made.ProjectID)
			if err != nil {
				t.Fatal(err)
			}

			done := m.job(job.JobID)
			if done.State != service.JobSucceeded || done.Result == nil || done.Result.Export == nil {
				t.Fatalf("export = %+v", done)
			}

			e := done.Result.Export
			if e.Bytes == 0 || len(e.Checks) == 0 {
				t.Fatalf("export result = %+v", e)
			}

			for _, c := range e.Checks {
				if !c.OK {
					t.Errorf("verification %s failed: %+v", c.Name, c)
				}
			}

			if _, err := os.Stat(e.Path); err != nil {
				t.Errorf("the exported file is missing: %v", err)
			}
		})
	}
}
