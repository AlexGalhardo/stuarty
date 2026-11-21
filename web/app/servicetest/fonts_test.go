package servicetest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

func bundledFontFile(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "renderer", "fonts", name))
	if err != nil {
		t.Fatal(err)
	}

	return data
}

// brandProject is a project with a text in the family "Brand" whose regular weight is the file
// with the hash.
func (m *mediaEnv) brandProject(hash string) string {
	m.t.Helper()

	project, track := m.withTrack()

	m.mustEdit(ui, project, m.head(project).Revision, op(
		"font.add", fmt.Sprintf(`{"family":"Brand","files":[{"weight":400,"hash":"sha256:%s"}]}`, hash),
	))
	m.mustEdit(ui, project, m.head(project).Revision, op(
		"text.add", fmt.Sprintf(
			`{"track_id":%q,"start":"0s","duration":"1s","text":"Hamburgefonstiv","style":{"font_family":"Brand","font_weight":400}}`,
			track,
		),
	))

	return project
}

func (m *mediaEnv) bitmapOf(project string) (key string, png []byte) {
	m.t.Helper()

	ctx := m.as(ui)

	w, err := m.svc.PlanWindow(ctx, service.PlanWindowRequest{ProjectID: project, From: 0, Until: 30})
	if err != nil {
		m.t.Fatal(err)
	}

	if len(w.Layers) != 1 || w.Layers[0].Bitmaps == nil {
		m.t.Fatalf("window = %+v", w)
	}

	key = w.Layers[0].Bitmaps.Keys[0]

	f, err := m.svc.OpenFile(ctx, service.FileRef{Kind: "bitmap", ProjectID: project, ID: key})
	if err != nil {
		m.t.Fatal(err)
	}

	png, err = os.ReadFile(filepath.Join(m.ws, filepath.FromSlash(f.Path)))
	if err != nil {
		m.t.Fatal(err)
	}

	return key, png
}

// SDD-007 section 5.4 item 3: font.add of a file the font store lacks is E_NOT_FOUND, and the same
// operation succeeds once the file is stored.
func TestFontAddRefusesAHashTheStoreLacks(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	hash := strings.Repeat("ab", 32)
	args := fmt.Sprintf(`{"family":"Brand","files":[{"weight":400,"hash":"sha256:%s"}]}`, hash)

	_, err := m.edit(ui, project, 0, op("font.add", args))

	se := serviceErr(err)
	if se == nil || se.Code != command.CodeNotFound || se.OpIndex == nil || *se.OpIndex != 0 {
		t.Fatalf("error = %+v", err)
	}

	if m.head(project).Revision != 0 {
		t.Error("a refused font.add changed the project")
	}

	stored, err := enginestore.NewFonts(m.ws).Put(bundledFontFile(t, "Anton-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}

	m.mustEdit(ui, project, 0, op("font.add", strings.ReplaceAll(args, hash, stored)))
}

// RF-053: two projects that name a family alike and ship different files never share a bitmap, a
// document with a user font renders (no E_FONT_UNKNOWN) and the bundled families are untouched.
func TestUserFontBitmapsAreNotSharedBetweenProjects(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true})
	fonts := enginestore.NewFonts(m.ws)

	anton, err := fonts.Put(bundledFontFile(t, "Anton-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}

	bebas, err := fonts.Put(bundledFontFile(t, "BebasNeue-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}

	a, b := m.brandProject(anton), m.brandProject(bebas)

	keyA, pngA := m.bitmapOf(a)
	keyB, pngB := m.bitmapOf(b)

	if keyA == keyB || bytes.Equal(pngA, pngB) {
		t.Fatalf("two files under one family name share a bitmap: keys %s and %s", keyA, keyB)
	}

	// The same file under another project name is the same bitmap: the key follows the files.
	keyC, _ := m.bitmapOf(m.brandProject(anton))
	if keyC != keyA {
		t.Errorf("the same font file gave another key: %s and %s", keyC, keyA)
	}

	// A project with the same text in a bundled family keeps the key of the plan.
	plain := m.exportable()
	keyPlain, _ := m.bitmapOf(plain)

	if keyPlain == keyA || len(keyPlain) != 64 {
		t.Errorf("bundled key = %s", keyPlain)
	}
}

// The export renders a document that uses a user font through the workspace font store.
func TestExportRendersAUserFont(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true, realExport: true})

	anton, err := enginestore.NewFonts(m.ws).Put(bundledFontFile(t, "Anton-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}

	project := m.brandProject(anton)

	job, err := m.startExport(project)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.runner.RunJob(context.Background(), job.JobID); err != nil {
		t.Fatal(err)
	}

	if done := m.job(job.JobID); done.State != service.JobSucceeded {
		t.Fatalf("job = %+v", done)
	}
}

// A bundle carries the user fonts of the document; the import validates them and stores them in the
// font store of the new workspace, not in the media store.
func TestBundleCarriesUserFontsIntoTheFontStore(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})

	anton, err := enginestore.NewFonts(src.ws).Put(bundledFontFile(t, "Anton-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}

	project := src.brandProject(anton)

	job := src.exportBundle(project, nil)
	if job.State != service.JobSucceeded {
		t.Fatalf("export = %+v", job)
	}

	_, entries := readZip(t, job.Result.Export.Path)
	if _, ok := entries["font."+anton]; !ok {
		t.Fatalf("the bundle has no font entry: %v", entries)
	}

	dst := newMediaEnv(t, mediaCfg{realExport: true})

	res, err := dst.importBundle(job.Result.Export.Path, false)
	if err != nil {
		t.Fatal(err)
	}

	if !res.Created || !bytes.Equal(dst.document(project), src.document(project)) {
		t.Fatalf("result = %+v", res)
	}

	if !enginestore.NewFonts(dst.ws).Has(anton) {
		t.Error("the font is not in the font store of the new workspace")
	}

	if dst.mediaHas("sha256:" + anton) {
		t.Error("the font was put in the media store")
	}

	// The imported project renders: its plan draws the text with the imported file.
	if key, png := dst.bitmapOf(project); len(key) != 64 || !bytes.HasPrefix(png, []byte("\x89PNG")) {
		t.Errorf("bitmap = %s, %d bytes", key, len(png))
	}
}

func memberCtx(user string) context.Context {
	return service.WithCaller(context.Background(), service.Caller{
		Actor: "user:" + user, Scopes: service.RoleScopes(service.RoleMember), OwnerUserID: user,
	})
}

// SDD-007 T-21: in network mode the font store is shared, so a user cannot name the font another
// user brought; the refusal is the one a hash nobody stored gets, and names no hash.
func TestSecurity_T21_FontAddCannotNameAnotherUsersFont(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})

	anton, err := enginestore.NewFonts(src.ws).Put(bundledFontFile(t, "Anton-Regular.ttf"))
	if err != nil {
		t.Fatal(err)
	}

	bundle := src.exportBundle(src.brandProject(anton), nil).Result.Export.Path

	net := newMediaEnv(t, mediaCfg{})
	alice, bob := memberCtx("alice"), memberCtx("bob")

	// Alice brings the font with a bundle, which links it to her project.
	if _, err := net.svc.(bundleImporter).ProjectBundleImport(
		alice, service.ProjectBundleImportRequest{Path: bundle, AsNew: true},
	); err != nil {
		t.Fatal(err)
	}

	add := op("font.add", fmt.Sprintf(`{"family":"Mine","files":[{"weight":400,"hash":"sha256:%s"}]}`, anton))

	newProject := func(ctx context.Context) string {
		res, err := testproject.CreateWithTimeline(ctx, net.svc, "p")
		if err != nil {
			t.Fatal(err)
		}

		return res.ProjectID
	}

	if _, err := net.svc.Edit(alice, service.EditRequest{
		ProjectID: newProject(alice), BaseRevision: 0, Operations: []command.Operation{add},
	}); err != nil {
		t.Fatalf("the owner of the font: %v", err)
	}

	_, errBob := net.svc.Edit(bob, service.EditRequest{
		ProjectID: newProject(bob), BaseRevision: 0, Operations: []command.Operation{add},
	})

	unknown := newMediaEnv(t, mediaCfg{})
	unknownProject := unknown.newProject("p")
	_, errNever := unknown.svc.Edit(unknown.as(ui), service.EditRequest{
		ProjectID: unknownProject, BaseRevision: 0, Operations: []command.Operation{add},
	})

	if !isCode(errBob, command.CodeNotFound) || !isCode(errNever, command.CodeNotFound) {
		t.Fatalf("bob: %v; never stored: %v; want both E_NOT_FOUND", errBob, errNever)
	}

	if errBob.Error() != errNever.Error() {
		t.Errorf("the refusals differ, which tells whether a font exists:\n%v\n%v", errBob, errNever)
	}

	var e *service.Error
	if errors.As(errBob, &e) && (e.Details["hash"] != nil || strings.Contains(e.Message+e.Hint, anton)) {
		t.Errorf("the refusal names the hash: %+v", e)
	}
}

// countStore counts the documents a write transaction reads.
type countStore struct {
	service.Store
	gets *atomic.Int64
}

func (s countStore) WriteProject(
	ctx context.Context, id string, fn func(context.Context, service.ProjectTx) error,
) error {
	return s.Store.WriteProject(ctx, id, func(ctx context.Context, tx service.ProjectTx) error {
		return fn(ctx, countTx{ProjectTx: tx, gets: s.gets})
	})
}

type countTx struct {
	service.ProjectTx
	gets *atomic.Int64
}

func (t countTx) Projects() service.ProjectRepo { return countRepo{t.ProjectTx.Projects(), t.gets} }

type countRepo struct {
	service.ProjectRepo
	gets *atomic.Int64
}

func (r countRepo) Get(ctx context.Context, id string) (service.ProjectRow, error) {
	r.gets.Add(1)

	return r.ProjectRepo.Get(ctx, id)
}

// The ownership check of font.add reads the caller's documents once for the whole batch: many
// entries do not multiply the work (SDD-007 section 9: bounded work per request).
func TestSecurity_T21_FontAddOwnershipScanIsBoundedPerBatch(t *testing.T) {
	var gets atomic.Int64

	m := newMediaEnv(t, mediaCfg{wrapStore: func(s service.Store) service.Store {
		return countStore{Store: s, gets: &gets}
	}})

	if _, err := enginestore.NewFonts(m.ws).Put(bundledFontFile(t, "Anton-Regular.ttf")); err != nil {
		t.Fatal(err)
	}

	bob := memberCtx("bob")
	create := func() string {
		res, err := testproject.CreateWithTimeline(bob, m.svc, "p")
		if err != nil {
			t.Fatal(err)
		}

		return res.ProjectID
	}

	project := create()

	for range 3 {
		create()
	}

	batch := func(n int) int64 {
		ops := make([]command.Operation, n)
		for i := range ops {
			ops[i] = op("font.add", fmt.Sprintf(
				`{"family":"F%d","files":[{"weight":400,"hash":"sha256:%064x"}]}`, i, i+1))
		}

		before := gets.Load()

		if _, err := m.svc.Edit(
			bob,
			service.EditRequest{ProjectID: project, Operations: ops},
		); !isCode(
			err,
			command.CodeNotFound,
		) {
			t.Fatalf("batch of %d: %v, want E_NOT_FOUND", n, err)
		}

		return gets.Load() - before
	}

	one, many := batch(1), batch(100)
	if many != one || one > 4+1 {
		t.Errorf(
			"document reads: %d for one entry and %d for 100; want the same and at most the caller's 4 projects",
			one,
			many,
		)
	}
}
