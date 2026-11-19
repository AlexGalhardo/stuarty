package servicetest

import (
	"context"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
)

// SDD-005 section 18 check 11: after every commit, whatever the batch, the
// summary columns equal the summary recomputed from the document and
// project_assets agrees with the document.
func TestPropertySummaryAndProjectAssetsFollowTheDocument(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	rng := rand.New(rand.NewSource(20261114))
	project, track := e.withTrack()

	commits := 0

	for step := range 120 {
		head, err := e.store.Projects().Get(context.Background(), project)
		if err != nil {
			t.Fatal(err)
		}

		doc, err := timeline.Decode(head.Document)
		if err != nil {
			t.Fatal(err)
		}

		before := head.Revision

		switch rng.Intn(10) {
		case 0:
			_, err = e.svc.Undo(e.as(ui), service.UndoRequest{ProjectID: project})
		case 1:
			_, err = e.svc.Redo(e.as(ui), service.UndoRequest{ProjectID: project})
		case 2:
			hash := fmt.Sprintf("sha256:%064x", rng.Int63())
			if _, _, uerr := e.store.Assets().Upsert(context.Background(), service.AssetRow{
				Hash: hash, Kind: "image", SizeBytes: 1, IngestState: service.IngestReady, CreatedMs: 1, UpdatedMs: 1,
			}); uerr != nil {
				t.Fatal(uerr)
			}

			ast := fmt.Sprintf(`{"asset":{"id":%q,"hash":%q,"kind":"image","name":"a%d.png","width":64,"height":64}}`,
				e.newID("ast"), hash, step)
			_, err = service.EditInternal(e.as(ui), e.svc, service.EditRequest{
				ProjectID: project, BaseRevision: head.Revision, Operations: []command.Operation{op("asset.add", ast)},
			})
		default:
			_, err = e.edit(ui, project, head.Revision, randomOp(rng, doc, track))
		}

		if err != nil && (code(err) == "" || code(err) == command.CodeInternal) {
			t.Fatalf("step %d: %v", step, err)
		}

		if e.head(project).Revision > before {
			commits++
		}

		assertDerivedState(t, e, project)
	}

	if commits < 40 {
		t.Errorf("only %d of 120 random batches committed; the generator is too weak to prove anything", commits)
	}

	all, err := e.store.Assets().ListProjectAssets(context.Background(), project)
	if err != nil || len(all) == 0 {
		t.Errorf("no project_assets row was ever written (%v): the sync was not exercised", err)
	}

	t.Logf("%d commits, %d project_assets rows", commits, len(all))
}

func randomOp(rng *rand.Rand, doc *timeline.Document, track string) command.Operation {
	items := make([]string, 0, len(doc.Items))
	for id := range doc.Items {
		items = append(items, id)
	}

	slices.Sort(items)

	pick := func() string {
		if len(items) == 0 {
			return "itm_zzzzzzzzzz"
		}

		return items[rng.Intn(len(items))]
	}

	switch rng.Intn(6) {
	case 0:
		return op("project.rename", fmt.Sprintf(`{"name":"P%d"}`, rng.Intn(1000)))
	case 1:
		return op("settings.set", fmt.Sprintf(`{"width":%d,"height":%d}`, 16+2*rng.Intn(1000), 16+2*rng.Intn(1000)))
	case 2:
		return op("item.move", fmt.Sprintf(`{"item_id":%q,"start":"%ds"}`, pick(), rng.Intn(60)))
	case 3:
		return op("item.delete", fmt.Sprintf(`{"item_ids":[%q]}`, pick()))
	default:
		return op(
			"text.add",
			fmt.Sprintf(`{"track_id":%q,"start":"%ds","duration":"1s","text":"t"}`, track, rng.Intn(60)),
		)
	}
}

func assertDerivedState(t *testing.T, e *env, project string) {
	t.Helper()

	ctx := context.Background()

	row, err := e.store.Projects().Get(ctx, project)
	if err != nil {
		t.Fatal(err)
	}

	doc, err := timeline.Decode(row.Document)
	if err != nil {
		t.Fatal(err)
	}

	want := service.DocSummary{
		Width:          doc.Settings.Width,
		Height:         doc.Settings.Height,
		FPSNum:         doc.Settings.FPS.Num,
		FPSDen:         doc.Settings.FPS.Den,
		DurationFrames: endOf(doc),
		Tracks:         len(doc.Tracks),
		Items:          len(doc.Items),
		Assets:         len(doc.Assets),
	}
	if row.Summary != want {
		t.Fatalf("r%d: summary %+v, recomputed %+v", row.Revision, row.Summary, want)
	}

	if row.Name != doc.Name {
		t.Fatalf("r%d: name column %q, document %q", row.Revision, row.Name, doc.Name)
	}

	active, err := e.store.Assets().ListProjectAssets(ctx, project, service.ProjectAssetActive)
	if err != nil {
		t.Fatal(err)
	}

	var got, wantIDs []string
	for i := range active {
		got = append(got, active[i].AssetID)
	}

	for id := range doc.Assets {
		wantIDs = append(wantIDs, id)
	}

	slices.Sort(got)
	slices.Sort(wantIDs)

	if !slices.Equal(got, wantIDs) {
		t.Fatalf(
			"r%d: active project_assets %s, document assets %s",
			row.Revision,
			strings.Join(got, ","),
			strings.Join(wantIDs, ","),
		)
	}
}
