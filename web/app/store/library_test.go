package store

import (
	"errors"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/assets"
)

func searchOf(t *testing.T, s *Store, match string) []string {
	t.Helper()

	got, err := s.Library().Search(ctx, match)
	if err != nil {
		t.Fatalf("Search(%s): %v", match, err)
	}

	var out []string
	for h := range got {
		out = append(out, h)
	}

	slices.Sort(out)

	return out
}

// The library side of an asset round-trips, is found by every part of the text it indexes, and goes with the
// asset: the search row of a virtual table has no foreign key, so Delete removes it by hand.
func TestLibraryRepo(t *testing.T) {
	s := testStore(t)

	for _, h := range []string{"sha256:aa", "sha256:bb"} {
		if _, _, err := s.Assets().Upsert(ctx, service.AssetRow{
			Hash: h, SizeBytes: 1, OriginalName: "clip-" + h[7:] + ".mp4", IngestState: service.IngestReady,
			CreatedMs: 1, UpdatedMs: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := s.Library().Get(ctx, "sha256:aa"); !errors.Is(err, assets.ErrNotFound) {
		t.Fatalf("Get with no row = %v", err)
	}

	cost := int64(2500)
	row := assets.Row{
		Hash: "sha256:aa", CreatedBy: "agent", CreatedMs: 5, UpdatedMs: 6, Description: "a harbour at night",
		Reusable: true, Tags: []string{"harbour", "night"},
		Origin: asset.Origin{
			Kind: asset.Generated, Provider: "acme", Model: "vid-2", Prompt: "boats under the moon", CostMicros: &cost,
			RightsNote: "mine", Source: "https://example.test/x",
		},
	}

	if err := s.Library().Put(ctx, row); err != nil {
		t.Fatal(err)
	}

	got, err := s.Library().Get(ctx, "sha256:aa")
	if err != nil {
		t.Fatal(err)
	}

	if got.Origin.Kind != asset.Generated || got.Origin.Model != "vid-2" || got.Origin.CostMicros == nil ||
		*got.Origin.CostMicros != cost || got.Origin.RightsNote != "mine" || !got.Reusable ||
		!slices.Equal(got.Tags, row.Tags) || got.CreatedBy != "agent" || got.Description != row.Description {
		t.Errorf("Get = %+v", got)
	}

	many, err := s.Library().GetMany(ctx, []string{"sha256:aa", "sha256:bb", "sha256:none"})
	if err != nil || len(many) != 1 || !slices.Equal(many["sha256:aa"].Tags, row.Tags) {
		t.Errorf("GetMany = %+v, %v", many, err)
	}

	// An update keeps who created it and when, and replaces the tags.
	row.CreatedBy, row.CreatedMs, row.Tags = "someone else", 99, []string{"harbour"}
	if err := s.Library().Put(ctx, row); err != nil {
		t.Fatal(err)
	}

	got, _ = s.Library().Get(ctx, "sha256:aa")
	if got.CreatedBy != "agent" || got.CreatedMs != 5 || !slices.Equal(got.Tags, []string{"harbour"}) {
		t.Errorf("after the update: %+v", got)
	}

	for match, want := range map[string][]string{
		`"clip"*`:          {"sha256:aa", "sha256:bb"}, // the name of the file, both
		`"harb"*`:          {"sha256:aa"},              // a tag and the description
		`"moon"*`:          {"sha256:aa"},              // the prompt
		`"acme"*`:          {"sha256:aa"},              // the provider
		`"example"*`:       {"sha256:aa"},              // the source
		`"night"*`:         {"sha256:aa"},              // the description, though the tag is gone
		`"boats"* "moon"*`: {"sha256:aa"},
		`"nothing"*`:       nil,
	} {
		if got := searchOf(t, s, match); !slices.Equal(got, want) {
			t.Errorf("Search(%s) = %v, want %v", match, got, want)
		}
	}

	// A bad expression is an error, never a wrong answer: the query builder of core/asset never makes one.
	if _, err := s.Library().Search(ctx, `"unterminated`); err == nil {
		t.Error("a malformed expression was accepted")
	}

	if err := s.Assets().Delete(ctx, "sha256:aa"); err != nil {
		t.Fatal(err)
	}

	if got := searchOf(t, s, `"harb"*`); got != nil {
		t.Errorf("the search row outlived the asset: %v", got)
	}

	if _, err := s.Library().Get(ctx, "sha256:aa"); !errors.Is(err, assets.ErrNotFound) {
		t.Errorf("the library row outlived the asset: %v", err)
	}
}
