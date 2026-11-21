package servicetest

import (
	"encoding/json"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/resources/templates"
)

// VV-291: in network mode an image template of the product has no owner, so every user lists it and makes a
// document from it, no user changes it, and one user's own documents stay private.
func TestSharedImageTemplatesInNetworkMode(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	images, err := templates.Images()
	if err != nil || len(images) == 0 {
		t.Fatalf("the templates: %d %v", len(images), err)
	}

	set := make([]service.BuiltInTemplate, 0, len(images))
	for _, im := range images {
		set = append(set, service.BuiltInTemplate{ID: im.ID, Document: im.Document})
	}

	if n, err := service.SeedBuiltInImageTemplates(t.Context(), e.store, 1, set); err != nil || n != len(set) {
		t.Fatalf("seed: %d %v", n, err)
	}

	member := func(user string) service.Caller {
		return service.Caller{Actor: "user:" + user, Scopes: service.RoleScopes(service.RoleMember), OwnerUserID: user}
	}
	alice, bob := service.WithCaller(t.Context(), member("alice")), service.WithCaller(t.Context(), member("bob"))

	list, err := e.svc.ImageList(bob, service.ImageListRequest{Limit: 100, Templates: "only"})
	if err != nil || len(list.Images) != len(images) {
		t.Fatalf("a user lists %d image templates, want %d: %v", len(list.Images), len(images), err)
	}

	none, err := e.svc.ImageList(bob, service.ImageListRequest{Limit: 100, Templates: "none"})
	if err != nil || len(none.Images) != 0 {
		t.Fatalf("the templates are not documents: %+v %v", none, err)
	}

	if _, err := e.svc.ImageDescribe(alice, service.ImageDescribeRequest{ImageID: images[0].ID}); err != nil {
		t.Fatalf("a user reads a shared template: %v", err)
	}

	// With no picture the template itself names what is missing, so it was read: one the caller may not see is
	// E_NOT_FOUND, and an assertion on anything but E_SLOT would also pass for a refusal of another kind.
	_, err = e.svc.ImageCreate(bob, service.ImageCreateRequest{TemplateID: images[0].ID})
	if code(err) != "E_SLOT" {
		t.Fatalf("a user cannot make a document from a shared template: %v", err)
	}

	if _, err := e.svc.ImageEdit(alice, service.ImageEditRequest{
		ImageID:      images[0].ID,
		BaseRevision: 1,
		Operations: []command.Operation{
			{Op: "layer.transform", Args: json.RawMessage(`{"layer_id":"lyr_0000000001","x":3}`)},
		},
	}); code(err) != "E_FORBIDDEN" {
		t.Fatalf("a user changed a shared template: %v", err)
	}
}

// VV-300: a template of the product that a person edited in local mode is no longer the one that ships, so serving
// the same workspace in network mode does not publish the edited version.
func TestEditedImageTemplateIsNotSharedInNetworkMode(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	images, err := templates.Images()
	if err != nil || len(images) == 0 {
		t.Fatalf("the templates: %d %v", len(images), err)
	}

	set := make([]service.BuiltInTemplate, 0, len(images))
	for _, im := range images {
		set = append(set, service.BuiltInTemplate{ID: im.ID, Document: im.Document})
	}

	if _, err := service.SeedBuiltInImageTemplates(t.Context(), e.store, 1, set); err != nil {
		t.Fatal(err)
	}

	local := service.WithCaller(t.Context(), service.Caller{
		Actor: "local:me", Scopes: service.RoleScopes(service.RoleAdmin),
	})
	if _, err := e.svc.ImageEdit(local, service.ImageEditRequest{
		ImageID:      images[0].ID,
		BaseRevision: 1,
		Operations: []command.Operation{
			{Op: "layer.transform", Args: json.RawMessage(`{"layer_id":"lyr_0000000001","x":3}`)},
		},
	}); err != nil {
		t.Fatalf("the local edit of a template: %v", err)
	}

	bob := service.WithCaller(t.Context(), service.Caller{
		Actor: "user:bob", Scopes: service.RoleScopes(service.RoleMember), OwnerUserID: "bob",
	})

	_, err = e.svc.ImageDescribe(bob, service.ImageDescribeRequest{ImageID: images[0].ID})
	if code(err) != "E_NOT_FOUND" {
		t.Fatalf("a user read the edited template: %v", err)
	}

	// Every other way to reach it: the list, a copy and its history, and after the edit was undone, since an undo is
	// one more revision of a document a person changed.
	assertHidden := func(when string) {
		t.Helper()

		list, err := e.svc.ImageList(bob, service.ImageListRequest{Limit: 100, Templates: "only"})
		if err != nil {
			t.Fatal(err)
		}

		for i := range list.Images {
			if list.Images[i].ImageID == images[0].ID {
				t.Errorf("%s: a user lists the edited template", when)
			}
		}

		if len(list.Images) != len(images)-1 {
			t.Errorf("%s: a user lists %d templates, want the %d not edited", when, len(list.Images), len(images)-1)
		}

		_, err = e.svc.ImageCreate(bob, service.ImageCreateRequest{TemplateID: images[0].ID})
		if code(err) != "E_NOT_FOUND" {
			t.Errorf("%s: a user made a document from the edited template: %v", when, err)
		}

		_, err = e.svc.ImageHistory(bob, service.ImageHistoryRequest{ImageID: images[0].ID})
		if code(err) != "E_NOT_FOUND" {
			t.Errorf("%s: a user read the history of the edited template: %v", when, err)
		}
	}

	assertHidden("after the edit")

	if _, err := e.svc.ImageUndo(local, service.ImageStepRequest{ImageID: images[0].ID}); err != nil {
		t.Fatalf("the local undo: %v", err)
	}

	assertHidden("after the undo")

	if len(images) < 2 {
		return
	}

	if _, err := e.svc.ImageDescribe(bob, service.ImageDescribeRequest{ImageID: images[1].ID}); err != nil {
		t.Fatalf("an untouched template stopped being shared: %v", err)
	}
}
