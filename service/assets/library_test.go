package assets_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/service/assets"
)

// memStore is a Store in memory: Search matches every term of the expression as a prefix of a word of the
// name, tags, description and origin text, as the full-text index does.
type memStore struct {
	rows  map[string]assets.Row
	names map[string]string
}

func newStore() *memStore {
	return &memStore{rows: map[string]assets.Row{}, names: map[string]string{}}
}

// rowKey is the key of a row: the bare hash for the local owner (the empty one), so a test reads it as st.rows[hash].
func rowKey(owner, hash string) string {
	if owner == "" {
		return hash
	}

	return owner + "|" + hash
}

func (m *memStore) Get(_ context.Context, owner, hash string) (assets.Row, error) {
	if r, ok := m.rows[rowKey(owner, hash)]; ok {
		return r, nil
	}

	return assets.Row{}, assets.ErrNotFound
}

func (m *memStore) GetMany(_ context.Context, owner string, hashes []string) (map[string]assets.Row, error) {
	out := map[string]assets.Row{}

	for _, h := range hashes {
		if r, ok := m.rows[rowKey(owner, h)]; ok {
			out[h] = r
		}
	}

	return out, nil
}

func (m *memStore) Put(_ context.Context, r assets.Row) error {
	if old, ok := m.rows[rowKey(r.Owner, r.Hash)]; ok {
		r.CreatedBy, r.CreatedMs = old.CreatedBy, old.CreatedMs
	}

	m.rows[rowKey(r.Owner, r.Hash)] = r

	return nil
}

func (m *memStore) Reindex(context.Context, string, string) error { return nil }

func (m *memStore) Search(_ context.Context, owner, match string) (map[string]bool, error) {
	out := map[string]bool{}

	for key := range m.rows {
		r := m.rows[key]
		if r.Owner != owner {
			continue
		}

		hash := r.Hash
		text := strings.ToLower(strings.Join([]string{
			m.names[hash], strings.Join(r.Tags, " "), r.Description, r.Origin.Source, r.Origin.Provider,
			r.Origin.Model, r.Origin.Prompt,
		}, " "))
		words := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		all := true

		for term := range strings.FieldsSeq(match) {
			prefix := strings.Trim(term, `"*`)
			if !slices.ContainsFunc(words, func(w string) bool { return strings.HasPrefix(w, prefix) }) {
				all = false
			}
		}

		if all {
			out[hash] = true
		}
	}

	return out, nil
}

func held(hash, name, kind, project string, timelines ...string) assets.Held {
	h := assets.Held{Hash: hash, Name: name, MediaKind: kind, CreatedMs: 1000}
	for i, tl := range timelines {
		h.Holdings = append(
			h.Holdings,
			asset.Holding{ProjectID: project, TimelineID: tl, AssetID: "ast_" + string(rune('a'+i))},
		)
	}

	return h
}

func hashesOf(p assets.Page) []string {
	var out []string
	for i := range p.Assets {
		out = append(out, p.Assets[i].Hash)
	}

	return out
}

func fixture(t *testing.T) (*assets.Library, *memStore, []assets.Held) {
	t.Helper()

	st := newStore()
	lib := &assets.Library{Store: st}
	files := []assets.Held{
		held("sha256:a", "logo.png", "image", "prj_1", "prj_1"),
		held("sha256:b", "reaction.mov", "video", "prj_1", "prj_1", "prj_1b"),
		held("sha256:c", "sting.wav", "audio", "prj_2", "prj_2"),
		held("sha256:d", "grade.cube", "lut", "prj_2", "prj_2"),
	}

	for i := range files {
		st.names[files[i].Hash] = files[i].Name
	}

	ctx := context.Background()

	if _, err := lib.Tag(ctx, &files[0], []string{"Brand", "logo"}, nil, "local:ui", 2000, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := lib.SetInfo(
		ctx,
		&files[0],
		assets.InfoPatch{Reusable: new(true)},
		"local:ui",
		2000,
		nil,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := lib.Tag(ctx, &files[1], []string{"logo", "funny"}, nil, "local:ui", 2000, nil); err != nil {
		t.Fatal(err)
	}

	st.rows["sha256:c"] = assets.Row{
		Hash:      "sha256:c",
		Origin:    asset.Origin{Kind: asset.FetchedByAgent, Source: "https://example.test/sting", RightsNote: "CC0"},
		Reusable:  true,
		Tags:      []string{"sound"},
		CreatedBy: "agent",
	}

	return lib, st, files
}

func TestListFiltersAgreeWithTheEntries(t *testing.T) {
	t.Parallel()

	lib, _, files := fixture(t)
	ctx := context.Background()

	cases := []struct {
		name string
		q    assets.Query
		want []string
	}{
		{"everything, by name", assets.Query{}, []string{"sha256:d", "sha256:a", "sha256:b", "sha256:c"}},
		{"tag", assets.Query{Filter: asset.Filter{Tag: "LOGO"}}, []string{"sha256:a", "sha256:b"}},
		{
			"reusable and tag",
			assets.Query{Filter: asset.Filter{Tag: "logo", Reusable: new(true)}},
			[]string{"sha256:a"},
		},
		{"not reusable", assets.Query{Filter: asset.Filter{Reusable: new(false)}}, []string{"sha256:d", "sha256:b"}},
		{
			"visual section",
			assets.Query{Filter: asset.Filter{Section: asset.SectionVisual}},
			[]string{"sha256:d", "sha256:a"},
		},
		{
			"video section",
			assets.Query{Filter: asset.Filter{Section: asset.SectionVideo}},
			[]string{"sha256:b"},
		},
		{
			"audio section",
			assets.Query{Filter: asset.Filter{Section: asset.SectionAudio}},
			[]string{"sha256:c"},
		},
		{"origin", assets.Query{Filter: asset.Filter{Origin: asset.FetchedByAgent}}, []string{"sha256:c"}},
		{
			"imported means no row too",
			assets.Query{Filter: asset.Filter{Origin: asset.Imported}},
			[]string{"sha256:d", "sha256:a", "sha256:b"},
		},
		{"project", assets.Query{Filter: asset.Filter{Project: "prj_1b"}}, []string{"sha256:b"}},
		{"text on a tag", assets.Query{Text: "funny"}, []string{"sha256:b"}},
		{"text on a prefix of the name", assets.Query{Text: "react"}, []string{"sha256:b"}},
		{"text on the origin", assets.Query{Text: "example.test"}, []string{"sha256:c"}},
		{
			"text and a filter",
			assets.Query{Text: "logo", Filter: asset.Filter{Reusable: new(true)}},
			[]string{"sha256:a"},
		},
		{"text that is only punctuation matches nothing", assets.Query{Text: `"*-`}, nil},
	}

	for _, c := range cases {
		page, err := lib.List(ctx, files, &c.q)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if got := hashesOf(page); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}

	if _, err := lib.List(ctx, files, &assets.Query{Filter: asset.Filter{Origin: "stolen"}}); err == nil {
		t.Error("an unknown origin was accepted")
	} else if bad, ok := errors.AsType[*asset.RuleError](err); !ok || bad.Reason != "origin_kind_unknown" {
		t.Errorf("error = %v", err)
	}
}

func TestListPages(t *testing.T) {
	t.Parallel()

	lib, _, files := fixture(t)
	ctx := context.Background()

	first, err := lib.List(ctx, files, &assets.Query{Limit: 3})
	if err != nil || len(first.Assets) != 3 || first.Next != 3 || first.Total != 4 {
		t.Fatalf("first page = %+v, %v", first, err)
	}

	rest, err := lib.List(ctx, files, &assets.Query{Offset: first.Next, Limit: 3})
	if err != nil || len(rest.Assets) != 1 || rest.Next != -1 {
		t.Fatalf("second page = %+v, %v", rest, err)
	}

	if got := append(
		hashesOf(first),
		hashesOf(rest)...); !slices.Equal(
		got,
		[]string{"sha256:d", "sha256:a", "sha256:b", "sha256:c"},
	) {
		t.Errorf("the pages hold %v", got)
	}
}

func TestEntryCarriesOriginTagsAndUses(t *testing.T) {
	t.Parallel()

	lib, _, files := fixture(t)

	e, err := lib.Get(context.Background(), &files[1], map[string]string{"prj_1": "Teaser"})
	if err != nil {
		t.Fatal(err)
	}

	if e.Origin.Kind != asset.Imported || !slices.Equal(e.Tags, []string{"funny", "logo"}) || e.Projects != 1 {
		t.Errorf("entry = %+v", e)
	}

	if len(e.Uses) != 1 || e.Uses[0].Name != "Teaser" || len(e.Uses[0].Timelines) != 2 {
		t.Errorf("a project that holds the file in two timelines is one use: %+v", e.Uses)
	}

	bare, err := lib.Get(context.Background(), &files[3], nil)
	if err != nil || bare.Origin.Kind != asset.Imported || bare.Tags == nil || bare.Reusable {
		t.Errorf("a file with no row reads as imported, with an empty tag list: %+v, %v", bare, err)
	}
}

func TestSetInfoKeepsTheOriginAndChecksTheRightsNote(t *testing.T) {
	t.Parallel()

	lib, st, files := fixture(t)
	ctx := context.Background()
	empty, note, desc := "", "licensed to me by the author", "the sting of the intro"

	if _, err := lib.SetInfo(ctx, &files[2], assets.InfoPatch{RightsNote: &empty}, "local:ui", 3000, nil); err == nil {
		t.Fatal("the rights note of a fetched file was cleared")
	} else if bad, ok := errors.AsType[*asset.RuleError](err); !ok || bad.Reason != "rights_note_required" {
		t.Fatalf("error = %v", err)
	}

	e, err := lib.SetInfo(
		ctx,
		&files[2],
		assets.InfoPatch{RightsNote: &note, Description: &desc, Reusable: new(false)},
		"local:ui",
		3000,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if e.Origin.RightsNote != note || e.Description != desc || e.Reusable || e.Origin.Kind != asset.FetchedByAgent ||
		e.Origin.Source == "" {
		t.Errorf("entry = %+v", e)
	}

	if st.rows["sha256:c"].CreatedBy != "agent" {
		t.Errorf("the creator changed: %q", st.rows["sha256:c"].CreatedBy)
	}

	long := strings.Repeat("x", asset.MaxDescription+1)
	if _, err := lib.SetInfo(ctx, &files[0], assets.InfoPatch{Description: &long}, "local:ui", 3000, nil); err == nil {
		t.Error("a description over the cap was accepted")
	}
}

func TestTagRules(t *testing.T) {
	t.Parallel()

	lib, _, files := fixture(t)
	ctx := context.Background()

	e, err := lib.Tag(ctx, &files[0], []string{"New Tag"}, []string{"brand"}, "x", 3000, nil)
	if err != nil || !slices.Equal(e.Tags, []string{"logo", "new-tag"}) {
		t.Fatalf("tags = %v, %v", e.Tags, err)
	}

	if _, err := lib.Tag(ctx, &files[0], []string{"no way!"}, nil, "x", 3000, nil); err == nil {
		t.Error("an invalid tag was accepted")
	}
}

func TestCheckImportRefusesAFetchedFileWithoutRightsNote(t *testing.T) {
	t.Parallel()

	_, _, err := assets.CheckImport(
		&asset.Origin{Kind: asset.FetchedByAgent, Source: "https://example.test/a.mp4"},
		nil,
	)

	bad, ok := errors.AsType[*asset.RuleError](err)
	if !ok || bad.Reason != "rights_note_required" || bad.Path != "origin.rights_note" {
		t.Fatalf("error = %v", err)
	}

	o, in, err := assets.CheckImport(
		&asset.Origin{Kind: asset.FetchedByAgent, Source: " https://example.test/a.mp4 ", RightsNote: "CC0"},
		&assets.ImportInfo{Tags: []string{"B-Roll", "b-roll"}, Reusable: new(true)},
	)
	if err != nil || o.Source != "https://example.test/a.mp4" || !slices.Equal(in.Tags, []string{"b-roll"}) {
		t.Fatalf("CheckImport = %+v, %+v, %v", o, in, err)
	}

	if o, _, err := assets.CheckImport(nil, nil); err != nil || o.Kind != asset.Imported {
		t.Errorf("no block is an import: %+v, %v", o, err)
	}

	if _, _, err := assets.CheckImport(nil, &assets.ImportInfo{Tags: []string{"bad tag!"}}); err == nil {
		t.Error("a bad tag in the info was accepted")
	}
}

func TestRecordImportKeepsTheFirstOrigin(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := newStore()

	first := asset.Origin{Kind: asset.Generated, Provider: "p", Model: "m", Prompt: "a cat"}
	if err := assets.RecordImport(
		ctx,
		st,
		"",
		"sha256:z",
		first,
		assets.ImportInfo{Tags: []string{"cat"}},
		"agent",
		10,
	); err != nil {
		t.Fatal(err)
	}

	later := asset.Origin{Kind: asset.FetchedByAgent, Source: "https://x", RightsNote: "n"}
	if err := assets.RecordImport(
		ctx,
		st,
		"",
		"sha256:z",
		later,
		assets.ImportInfo{Tags: []string{"pet"}, Reusable: new(true), Description: "a cat"},
		"ui",
		20,
	); err != nil {
		t.Fatal(err)
	}

	r := st.rows["sha256:z"]
	if r.Origin.Kind != asset.Generated || r.Origin.Provider != "p" || r.CreatedBy != "agent" || r.CreatedMs != 10 {
		t.Errorf("the origin of the first import was replaced: %+v", r)
	}

	if !slices.Equal(r.Tags, []string{"cat", "pet"}) || !r.Reusable || r.Description != "a cat" || r.UpdatedMs != 20 {
		t.Errorf("the later import did not add its info: %+v", r)
	}
}

// Two owners of the same hash keep separate rows, tags and search text; a row written for one is absent for the other.
func TestLibraryRowsBelongToTheirOwner(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := newStore()
	file := held("sha256:s", "shared.png", "image", "prj_1", "prj_1")
	st.names[file.Hash] = file.Name

	for owner, note := range map[string]string{"alice": "alice note", "bob": "bob note"} {
		lib := &assets.Library{Store: st, Owner: owner}

		if _, err := lib.SetInfo(ctx, &file, assets.InfoPatch{RightsNote: &note}, "user:"+owner, 5, nil); err != nil {
			t.Fatal(err)
		}

		if _, err := lib.Tag(ctx, &file, []string{owner + "tag"}, nil, "user:"+owner, 5, nil); err != nil {
			t.Fatal(err)
		}
	}

	for owner, other := range map[string]string{"alice": "bob", "bob": "alice"} {
		lib := &assets.Library{Store: st, Owner: owner}

		got, err := lib.Get(ctx, &file, nil)
		if err != nil {
			t.Fatal(err)
		}

		if got.Origin.RightsNote != owner+" note" || !slices.Equal(got.Tags, []string{owner + "tag"}) {
			t.Errorf("%s reads %+v", owner, got)
		}

		page, err := lib.List(ctx, []assets.Held{file}, &assets.Query{Text: other + "tag"})
		if err != nil || len(page.Assets) != 0 {
			t.Errorf("%s finds the text of %s: %+v, %v", owner, other, page, err)
		}
	}

	if got, err := (&assets.Library{Store: st, Owner: "carol"}).Get(ctx, &file, nil); err != nil ||
		got.Origin.RightsNote != "" || len(got.Tags) != 0 {
		t.Errorf("a third owner reads a record that is not hers: %+v, %v", got, err)
	}
}

// core/asset imports only the standard library, so the library composes a tag typed with a decomposed accent
// before core normalizes it: an import, a tag change and a filter all store and match the composed form.
func TestLibraryComposesDecomposedTags(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := newStore()
	file := held("sha256:n", "menu.png", "image", "prj_1", "prj_1")
	decomposed := "Cafe" + string(rune(0x0301))

	_, info, err := assets.CheckImport(nil, &assets.ImportInfo{Tags: []string{decomposed}})
	if err != nil || !slices.Equal(info.Tags, []string{"café"}) {
		t.Fatalf("CheckImport = %+v, %v", info, err)
	}

	if err := assets.RecordImport(
		ctx, st, "", file.Hash, asset.Origin{Kind: asset.Imported},
		assets.ImportInfo{Tags: []string{"The" + string(rune(0x0300))}}, "user:a", 1,
	); err != nil {
		t.Fatal(err)
	}

	lib := &assets.Library{Store: st}

	got, err := lib.Tag(ctx, &file, []string{decomposed}, nil, "user:a", 2, nil)
	if err != nil || !slices.Equal(got.Tags, []string{"café", "thè"}) {
		t.Fatalf("Tag = %+v, %v", got.Tags, err)
	}

	page, err := lib.List(ctx, []assets.Held{file}, &assets.Query{Filter: asset.Filter{Tag: decomposed}})
	if err != nil || !slices.Equal(hashesOf(page), []string{file.Hash}) {
		t.Errorf("filter by the decomposed tag = %v, %v", hashesOf(page), err)
	}

	if got, err := lib.Tag(ctx, &file, nil, []string{decomposed}, "user:a", 3, nil); err != nil ||
		!slices.Equal(got.Tags, []string{"thè"}) {
		t.Errorf("removing the decomposed tag = %+v, %v", got.Tags, err)
	}
}

func withSize(h assets.Held, state string, n *int64) assets.Held {
	h.State, h.Bytes = state, n

	return h
}

func TestAssetLibraryTotalsCountEachFileOnceAndNameTheUnsized(t *testing.T) {
	t.Parallel()

	lib, _, files := fixture(t)
	files[0] = withSize(files[0], assets.StateReady, new(int64(4<<30))) // logo.png, over 4 GiB
	files[1] = withSize(files[1], assets.StateReady, new(int64(1_500)))
	files[2] = withSize(files[2], assets.StateImporting, nil)
	files[3] = withSize(files[3], assets.StateMissing, nil)

	// The same file held twice, as two names of a project would: it weighs once.
	dup := files[1]
	dup.Name = "reaction copy.mov"

	page, err := lib.List(context.Background(), append(slices.Clone(files), dup), &assets.Query{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}

	if want := (asset.Total{Bytes: 4<<30 + 1_500, Files: 4, Unsized: 2}); page.Size != want {
		t.Errorf("Size = %+v, want %+v", page.Size, want)
	}

	if len(page.Assets) != 2 {
		t.Errorf("the page holds %d entries, want 2 (the total is not the page)", len(page.Assets))
	}

	for _, a := range page.Assets {
		switch a.Hash {
		case "sha256:a":
			if a.SizeText != "4.3 GB" || a.State != assets.StateReady {
				t.Errorf("logo entry = %q %q", a.SizeText, a.State)
			}
		case "sha256:b":
			if a.SizeText != "1.5 KB" {
				t.Errorf("reaction size text = %q", a.SizeText)
			}
		}
	}
}

func TestAssetLibraryTheTotalFollowsTheFilters(t *testing.T) {
	t.Parallel()

	lib, _, files := fixture(t)
	files[0] = withSize(files[0], assets.StateReady, new(int64(10)))
	files[1] = withSize(files[1], assets.StateReady, new(int64(1_000)))
	files[2] = withSize(files[2], assets.StateReady, new(int64(100)))
	files[3] = withSize(files[3], assets.StateReady, new(int64(1)))

	page, err := lib.List(context.Background(), files, &assets.Query{Filter: asset.Filter{Section: "visual"}})
	if err != nil {
		t.Fatal(err)
	}

	if want := (asset.Total{Bytes: 11, Files: 2}); page.Size != want {
		t.Errorf("visual section Size = %+v, want %+v", page.Size, want)
	}
}

func TestAssetLibraryAnEmptyLibraryWeighsNothingAndIsNotAnError(t *testing.T) {
	t.Parallel()

	lib := &assets.Library{Store: newStore()}

	page, err := lib.List(context.Background(), nil, &assets.Query{})
	if err != nil || page.Total != 0 || page.Size != (asset.Total{}) || len(page.Assets) != 0 {
		t.Errorf("page = %+v, %v", page, err)
	}
}

func TestAssetLibraryAnEntryWithNoStateIsReadyAndAUnknownSizeHasNoText(t *testing.T) {
	t.Parallel()

	lib, _, files := fixture(t)
	files[0].Bytes = nil

	page, err := lib.List(context.Background(), files[:1], &assets.Query{})
	if err != nil || len(page.Assets) != 1 {
		t.Fatalf("page = %+v, %v", page, err)
	}

	if a := page.Assets[0]; a.State != assets.StateReady || a.SizeText != "" || a.Bytes != nil {
		t.Errorf(
			"entry = state %q size %q bytes %v; an unknown size is null and has no text, not 0 B",
			a.State, a.SizeText, a.Bytes,
		)
	}
}

func TestListCountsTheTagsInUseByTheOtherFilters(t *testing.T) {
	t.Parallel()

	lib, _, files := fixture(t)
	ctx := context.Background()

	all, err := lib.List(ctx, files, &assets.Query{})
	if err != nil {
		t.Fatal(err)
	}

	want := []asset.TagCount{
		{Tag: "logo", Count: 2},
		{Tag: "brand", Count: 1},
		{Tag: "funny", Count: 1},
		{Tag: "sound", Count: 1},
	}
	if !slices.Equal(all.TagsInUse, want) {
		t.Fatalf("tags in use = %v, want %v", all.TagsInUse, want)
	}

	// The tag filter narrows the files, not the picker; a section does narrow it.
	byTag, err := lib.List(ctx, files, &assets.Query{Filter: asset.Filter{Tag: "brand"}})
	if err != nil {
		t.Fatal(err)
	}

	if byTag.Total != 1 || !slices.Equal(byTag.TagsInUse, want) {
		t.Fatalf("tag filter: total %d, tags %v", byTag.Total, byTag.TagsInUse)
	}

	audio, err := lib.List(ctx, files, &assets.Query{Filter: asset.Filter{Section: asset.SectionAudio}})
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(audio.TagsInUse, []asset.TagCount{{Tag: "sound", Count: 1}}) {
		t.Fatalf("audio tags = %v", audio.TagsInUse)
	}
}

func TestSoundKindIsSetFilteredAndOnlyForAudio(t *testing.T) {
	t.Parallel()

	lib, _, files := fixture(t)
	ctx := context.Background()

	voice := "voice"

	got, err := lib.SetInfo(ctx, &files[2], assets.InfoPatch{SoundKind: &voice}, "local:ui", 3000, nil)
	if err != nil || got.SoundKind != "voice" {
		t.Fatalf("SetInfo = %+v, %v", got, err)
	}

	page, err := lib.List(ctx, files, &assets.Query{Filter: asset.Filter{SoundKind: "voice"}})
	if err != nil || !slices.Equal(hashesOf(page), []string{"sha256:c"}) {
		t.Fatalf("filter voice = %v, %v", hashesOf(page), err)
	}

	page, err = lib.List(ctx, files, &assets.Query{Filter: asset.Filter{SoundKind: "music"}})
	if err != nil || len(page.Assets) != 0 {
		t.Fatalf("filter music = %v, %v", hashesOf(page), err)
	}

	if _, err := lib.List(ctx, files, &assets.Query{Filter: asset.Filter{SoundKind: "sfx"}}); err == nil {
		t.Error("an unknown sound kind filter must be refused")
	}

	for name, in := range map[string]struct {
		h    *assets.Held
		kind string
		why  string
	}{
		"image":   {&files[0], "music", "sound_kind_not_allowed"},
		"unknown": {&files[2], "sfx", "sound_kind_unknown"},
	} {
		_, err := lib.SetInfo(ctx, in.h, assets.InfoPatch{SoundKind: &in.kind}, "local:ui", 3000, nil)

		var bad *asset.RuleError
		if !errors.As(err, &bad) || bad.Reason != in.why || bad.Path != "sound_kind" {
			t.Errorf("%s: err = %v, want %s", name, err, in.why)
		}
	}

	none := ""

	cleared, err := lib.SetInfo(ctx, &files[2], assets.InfoPatch{SoundKind: &none}, "local:ui", 4000, nil)
	if err != nil || cleared.SoundKind != "" {
		t.Fatalf("clear = %+v, %v", cleared, err)
	}
}

func TestRecordImportSoundKindExplicitReplacesAndDefaultOnlyFills(t *testing.T) {
	t.Parallel()

	_, st, _ := fixture(t)
	ctx := context.Background()
	origin := asset.Origin{Kind: asset.Imported}

	if err := assets.RecordImport(
		ctx,
		st,
		"",
		"sha256:new",
		origin,
		assets.ImportInfo{DefaultSoundKind: "voice"},
		"a",
		1,
	); err != nil {
		t.Fatal(err)
	}

	if got := st.rows["sha256:new"].SoundKind; got != "voice" {
		t.Fatalf("a default fills an empty kind: %q", got)
	}

	if err := assets.RecordImport(
		ctx,
		st,
		"",
		"sha256:new",
		origin,
		assets.ImportInfo{SoundKind: "music"},
		"a",
		2,
	); err != nil {
		t.Fatal(err)
	}

	if got := st.rows["sha256:new"].SoundKind; got != "music" {
		t.Fatalf("an explicit kind replaces: %q", got)
	}

	if err := assets.RecordImport(
		ctx,
		st,
		"",
		"sha256:new",
		origin,
		assets.ImportInfo{DefaultSoundKind: "voice"},
		"a",
		3,
	); err != nil {
		t.Fatal(err)
	}

	if got := st.rows["sha256:new"].SoundKind; got != "music" {
		t.Fatalf("a default never replaces: %q", got)
	}

	if _, _, err := assets.CheckImport(nil, &assets.ImportInfo{SoundKind: "sfx"}); err == nil {
		t.Error("an unknown sound kind on import must be refused")
	}
}
