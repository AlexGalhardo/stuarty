package servicetest

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
	"github.com/AlexGalhardo/stuarty/web/resources/templates"
)

// SDD-007 T-89: in network mode the image templates that ship with the product are the one image that is not a
// user's. Every write path refuses to change one, and the exception is exactly a template, with no owner, made
// by the starter actor: no other row is shown to a user who does not own it, by the list or by a read.
func TestSecurity_T89_SharedImageTemplateIsReadOnlyAndShowsNothingElse(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	images, err := templates.Images()
	if err != nil || len(images) == 0 {
		t.Fatalf("the templates: %d %v", len(images), err)
	}

	set := make([]service.BuiltInTemplate, 0, len(images))
	sharedIDs := make([]string, 0, len(images))

	for _, im := range images {
		set = append(set, service.BuiltInTemplate{ID: im.ID, Document: im.Document})
		sharedIDs = append(sharedIDs, im.ID)
	}

	if _, serr := service.SeedBuiltInImageTemplates(t.Context(), e.store, 1, set); serr != nil {
		t.Fatal(serr)
	}

	member := func(user string) context.Context {
		return service.WithCaller(t.Context(), service.Caller{
			Actor: "user:" + user, Scopes: service.RoleScopes(service.RoleMember), OwnerUserID: user, Browser: true,
		})
	}
	alice, bob := member("alice"), member("bob")
	shared := images[0].ID

	before, err := e.store.Images().Get(t.Context(), shared)
	if err != nil {
		t.Fatal(err)
	}

	// Alice's own documents, one of them a template of her own.
	own, err := e.svc.ImageCreate(alice, service.ImageCreateRequest{Name: "Mine", Preset: "square_post"})
	if err != nil {
		t.Fatal(err)
	}

	ownTemplate, err := e.svc.ImageCreate(
		alice,
		service.ImageCreateRequest{Name: "My cover", Preset: "square_post", Template: true},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Another user reads the shared template to make a document from it: with no picture, the template itself
	// says what is missing, so it was read (a template the caller may not see is E_NOT_FOUND).
	if _, cerr := e.svc.ImageCreate(bob, service.ImageCreateRequest{TemplateID: shared}); code(cerr) != "E_SLOT" {
		t.Fatalf("a user cannot make a document from a shared template: %v", cerr)
	}

	// Rows that are almost the exception: an ownerless template of the local workspace, an ownerless document of
	// the starter actor, and a starter template that has an owner. None of them is shared.
	near := map[string]func(*service.ImageRow){
		"img_n0000000a1": func(r *service.ImageRow) { r.CreatedBy = "local:user" },
		"img_n0000000a2": func(r *service.ImageRow) { r.Template = false },
		"img_n0000000a3": func(r *service.ImageRow) { owner := "alice"; r.OwnerUserID = &owner },
	}

	if werr := e.store.Write(t.Context(), func(ctx context.Context, tx service.Tx) error {
		for id, change := range near {
			row := before
			row.ID = id
			change(&row)

			if cerr := tx.Images().Create(ctx, row); cerr != nil {
				return cerr
			}
		}

		return nil
	}); werr != nil {
		t.Fatal(werr)
	}

	hidden := []string{own.ImageID, ownTemplate.ImageID}
	for id := range near {
		hidden = append(hidden, id)
	}

	// The list of another user holds the shared templates and nothing else.
	for _, filter := range []string{"", "only", "none"} {
		list, lerr := e.svc.ImageList(bob, service.ImageListRequest{Limit: 100, Templates: filter})
		if lerr != nil {
			t.Fatal(lerr)
		}

		for _, h := range list.Images {
			if slices.Contains(hidden, h.ImageID) || !slices.Contains(sharedIDs, h.ImageID) {
				t.Errorf("templates=%q: bob lists %s, which is not his", filter, h.ImageID)
			}
		}

		want := len(images)
		if filter == "none" {
			want = 0
		}

		if len(list.Images) != want {
			t.Errorf("templates=%q: bob lists %d images, want %d", filter, len(list.Images), want)
		}
	}

	// A read of any of them is as unknown as an image that does not exist.
	for _, id := range hidden {
		if _, derr := e.svc.ImageDescribe(bob, service.ImageDescribeRequest{ImageID: id}); code(derr) != "E_NOT_FOUND" {
			t.Errorf("bob reads %s: %v", id, derr)
		}

		if _, herr := e.svc.ImageHistory(bob, service.ImageHistoryRequest{ImageID: id}); code(herr) != "E_NOT_FOUND" {
			t.Errorf("bob reads the history of %s: %v", id, herr)
		}

		if _, cerr := e.svc.ImageCreate(bob, service.ImageCreateRequest{TemplateID: id}); code(cerr) != "E_NOT_FOUND" {
			t.Errorf("bob makes a document from %s: %v", id, cerr)
		}
	}

	// Every path that writes a document refuses the shared template, whoever asks.
	move := []command.Operation{{Op: "layer.transform", Args: json.RawMessage(`{"layer_id":"lyr_0000000001","x":3}`)}}
	hash := "sha256:" + strings.Repeat("0", 64)

	for _, who := range []context.Context{alice, bob} {
		// The image.edit job starts in a project of the caller.
		prj, perr := testproject.CreateWithTimeline(who, e.svc, "Covers")
		if perr != nil {
			t.Fatal(perr)
		}

		writes := map[string]func() error{
			"edit": func() error {
				_, werr := e.svc.ImageEdit(who, service.ImageEditRequest{
					ImageID: shared, BaseRevision: 1, Operations: move,
				})

				return werr
			},
			"edit, dry run": func() error {
				_, werr := e.svc.ImageEdit(who, service.ImageEditRequest{
					ImageID: shared, BaseRevision: 1, Operations: move, DryRun: true,
				})

				return werr
			},
			"undo": func() error {
				_, werr := e.svc.ImageUndo(who, service.ImageStepRequest{ImageID: shared})

				return werr
			},
			"redo": func() error {
				_, werr := e.svc.ImageRedo(who, service.ImageStepRequest{ImageID: shared})

				return werr
			},
			"import into": func() error {
				_, werr := e.svc.ImageImport(who, service.ImageImportRequest{
					ImageID: shared, Hash: hash, BaseRevision: 1,
				})

				return werr
			},
			"model edit": func() error {
				_, werr := e.svc.ImageModelEdit(who, service.ImageModelEditRequest{
					ProjectID: prj.ProjectID, ConnectionID: "con_0000000001", ImageID: shared,
					Operation: "remove_background", Model: "m", LayerID: "lyr_0000000001",
				})

				return werr
			},
		}

		for name, write := range writes {
			if got := code(write()); got != "E_FORBIDDEN" {
				t.Errorf("%s of the shared template: %q, want E_FORBIDDEN", name, got)
			}
		}
	}

	after, err := e.store.Images().Get(t.Context(), shared)
	if err != nil {
		t.Fatal(err)
	}

	if after.Revision != before.Revision || after.DocSHA256 != before.DocSHA256 || after.Template != before.Template ||
		after.OwnerUserID != nil || after.CreatedBy != service.StarterActor {
		t.Fatalf("the shared template changed: %+v, was %+v", after.ImageHead, before.ImageHead)
	}

	ops, err := e.store.Images().OpsSince(t.Context(), shared, 0)
	if err != nil || len(ops) != 1 {
		t.Fatalf("the log of the shared template has %d batches (%v), want the one of the seed", len(ops), err)
	}

	// The owner still reads her own documents, so the refusals above are not a store that refuses everything.
	if _, derr := e.svc.ImageDescribe(alice, service.ImageDescribeRequest{ImageID: ownTemplate.ImageID}); derr != nil {
		t.Fatalf("alice reads her own document: %v", derr)
	}
}
