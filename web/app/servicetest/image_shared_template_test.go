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

	if _, err := e.svc.ImageCreate(
		bob,
		service.ImageCreateRequest{TemplateID: images[0].ID},
	); code(
		err,
	) == "E_NOT_FOUND" {
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
