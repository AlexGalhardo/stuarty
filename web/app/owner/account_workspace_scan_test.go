package owner

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/assets"
	"github.com/AlexGalhardo/stuarty/service/testproject"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// The words of the second stage and of the library, in the places a person puts them (ADR-0053).
const (
	canaryCaption    = "CANARY-CAPTION-on-a-video"
	canaryLayer      = "CANARY-TEXTLAYER-on-an-image"
	canaryPromptBody = "CANARY-PROMPT-body-of-a-prompt"
	canarySkill      = "CANARY-SKILL-body-of-a-skill"
	canaryLater      = "CANARY-CAPTION-written-after-the-account"
	canaryLaterRef   = "CANARY-SKILLFILE-a-file-of-a-skill"
	canaryAssetDesc  = "CANARY-DESCRIPTION-of-an-asset"
	canaryRights     = "CANARY-RIGHTSNOTE-of-an-asset"
	canaryBrand      = "canary_brand_colour_q7"
)

var workspaceCanaries = []string{
	canaryCaption, canaryLayer, canaryPromptBody, canarySkill, canaryNote, canaryLaterRef,
	canaryAssetDesc, canaryRights, canaryBrand,
}

// everyByteOfTheWorkspace is every file under the workspace: the database, its log and its shared memory, library/,
// backups/, exports/, the vault, the media: what a copy of the folder carries.
func everyByteOfTheWorkspace(t *testing.T, home string) map[string][]byte {
	t.Helper()

	out := map[string][]byte{}

	err := filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		raw, rerr := os.ReadFile(p)
		if rerr != nil {
			// A file another process holds (the shared memory of the database) is not a file to scan.
			return nil //nolint:nilerr // see above
		}

		out[p] = raw

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func foundIn(files map[string][]byte, words ...string) map[string]string {
	hit := map[string]string{}

	for name, raw := range files {
		for _, w := range words {
			if bytes.Contains(raw, []byte(w)) {
				hit[w] = name
			}
		}
	}

	return hit
}

// With an account, no byte of the workspace holds a caption, a text layer, a template, an edit, a brand kit, the
// description or the rights note of an asset, a memory note, a prompt or a skill in clear text: not db.sqlite, its
// log, library/, backups/ or exports/ (SDD-007 T-136).
func TestSecurity_T136_NoByteOfTheWorkspaceHoldsTheWordsOfThePerson(t *testing.T) {
	getenv := accountEnv(t)
	home := getenv(workspace.EnvHome)
	ctx := context.Background()

	o, as, _ := existingWorkspace(t, getenv)

	// A video with a caption, a template made from it, an image with a text layer, a prompt and a skill.
	video, serr := testproject.CreateWithTimeline(as, o.Service, "Clip")
	if serr != nil {
		t.Fatal(serr)
	}

	edit, serr := o.Service.Edit(as, service.EditRequest{
		ProjectID: video.ProjectID, BaseRevision: 0,
		Operations: []command.Operation{
			op("track.add", `{"kind":"visual","name":"Base","id":"trk_f6g7h8j9k0"}`),
			op("text.add", `{"track_id":"trk_f6g7h8j9k0","start":"1s","duration":"1s","text":"`+canaryCaption+
				`","id":"itm_b2c3d4e5f6"}`),
		},
	})
	if serr != nil {
		t.Fatal(serr)
	}

	if _, serr := o.Service.TemplateSave(as, service.TemplateSaveRequest{
		ProjectID: video.ProjectID, Name: "Clip template", Description: "a template made from a clip",
	}); serr != nil {
		t.Fatal(serr)
	}

	img, serr := o.Service.ImageCreate(as, service.ImageCreateRequest{Width: 200, Height: 100, Background: "#102030"})
	if serr != nil {
		t.Fatal(serr)
	}

	if _, serr := o.Service.ImageEdit(as, service.ImageEditRequest{
		ImageID: img.ImageID, BaseRevision: img.Revision,
		Operations: []command.Operation{op("layer.add_text", `{"text":"`+canaryLayer+`","id":"lyr_bbbbbbbbbb"}`)},
	}); serr != nil {
		t.Fatal(serr)
	}

	if _, serr := o.Service.PromptWrite(as, service.PromptWriteRequest{
		Name: "intro", Description: "an intro prompt", Body: canaryPromptBody + " about {{topic}}",
	}); serr != nil {
		t.Fatal(serr)
	}

	if _, serr := o.Service.SkillWrite(as, service.SkillWriteRequest{
		Name:        "hook-writer",
		Description: "Write the opening of a short video.",
		Body:        "# Hooks\n\n" + canarySkill + "\n",
		Files:       []service.SkillFileInput{{Path: "references/patterns.md", Text: canaryLaterRef + "\n"}},
	}); serr != nil {
		t.Fatal(serr)
	}

	// A brand kit, and the description and the rights note of a file of the asset library. The file is put through
	// the repository the service writes with: importing media needs FFmpeg, and the sealing is in the repository.
	if _, serr := o.Service.BrandKitSet(as, service.BrandKitSetRequest{
		Colors: map[string]string{canaryBrand: "#112233"},
	}); serr != nil {
		t.Fatal(serr)
	}

	if _, _, err := o.Store.Assets().Upsert(ctx, service.AssetRow{
		Hash: "sha256:" + strings.Repeat("ab", 32), SizeBytes: 1, OriginalName: "clip.mp4",
		IngestState: service.IngestReady, CreatedMs: 1, UpdatedMs: 1,
	}); err != nil {
		t.Fatal(err)
	}

	if err := o.Store.AssetLibrary().Put(ctx, assets.Row{
		Hash: "sha256:" + strings.Repeat("ab", 32), CreatedBy: "local:ui", CreatedMs: 1, UpdatedMs: 1,
		Description: canaryAssetDesc, Tags: []string{"harbour"},
		Origin: asset.Origin{Kind: asset.Imported, RightsNote: canaryRights},
	}); err != nil {
		t.Fatal(err)
	}

	// The control: with no account the scan finds every word, so it can see what it looks for.
	checkpoint(t, o)

	control := foundIn(everyByteOfTheWorkspace(t, home), workspaceCanaries...)
	for _, w := range workspaceCanaries {
		if control[w] == "" {
			t.Fatalf("the control does not find %q in the workspace: the scan is blind", w)
		}
	}

	if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
		t.Fatal(err)
	}

	checkpoint(t, o)

	if hit := foundIn(everyByteOfTheWorkspace(t, home), workspaceCanaries...); len(hit) != 0 {
		t.Fatalf("after the creation of the account these words are in clear: %v", hit)
	}

	// What is written afterwards is sealed as it is written.
	if err := o.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	if _, serr := o.Service.Edit(as, service.EditRequest{
		ProjectID:    video.ProjectID,
		BaseRevision: edit.Revision,
		Operations: []command.Operation{
			op("text.add", `{"track_id":"trk_f6g7h8j9k0","start":"3s","duration":"1s","text":"`+
				canaryLater+`","id":"itm_c3d4e5f6a7"}`),
		},
	}); serr != nil {
		t.Fatal(serr)
	}

	checkpoint(t, o)

	if hit := foundIn(everyByteOfTheWorkspace(t, home), append(workspaceCanaries, canaryLater)...); len(hit) != 0 {
		t.Fatalf("after a write with an account these words are in clear: %v", hit)
	}

	// And everything still reads back through the service.
	got, err := o.Service.MemoryGet(as, service.LibraryRef{Name: "note"})
	if err != nil || got.Body == "" {
		t.Fatalf("the note: %+v %v", got, err)
	}

	if _, err := o.Service.ImageDescribe(as, service.ImageDescribeRequest{ImageID: img.ImageID}); err != nil {
		t.Fatalf("the image: %v", err)
	}

	info, err := o.Store.AssetLibrary().Get(ctx, "", "sha256:"+strings.Repeat("ab", 32))
	if err != nil || info.Description != canaryAssetDesc || info.Origin.RightsNote != canaryRights {
		t.Fatalf("the notes of the asset: %+v %v", info, err)
	}

	if kit, err := o.Service.BrandKitGet(as, service.BrandKitGetRequest{}); err != nil || kit.Colors[canaryBrand] == "" {
		t.Fatalf("the brand kit: %+v %v", kit, err)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
}
