package servicetest

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// The revert and the checkpoints of an image over the SQLite store: the automatic checkpoint of an agent, the
// revert to it by name and its undo, and the names that another user cannot reach (SDD-021 section 4).
func TestImageRevertOverTheRealStore(t *testing.T) {
	e := newEnvWith(t, func(e *env, o *service.Options) { o.Workspace = string(e.dir) })
	ctx, person := e.as(agent), e.as(ui)

	created, err := e.svc.ImageCreate(person, service.ImageCreateRequest{Preset: "video_thumbnail", Name: "Thumb"})
	if err != nil {
		t.Fatal(err)
	}

	id := created.ImageID

	for rev, ops := range [][]command.Operation{
		{op("layer.add_text", `{"text": "Hello", "id": "lyr_bbbbbbbbbb"}`)},
		{op("layer.set", `{"layer_id": "lyr_bbbbbbbbbb", "opacity": 50}`)},
		{op("layer.remove", `{"layer_id": "lyr_bbbbbbbbbb"}`)},
	} {
		actor := ctx
		if rev == 0 {
			actor = person
		}

		if _, err := e.svc.ImageEdit(actor, service.ImageEditRequest{
			ImageID: id, BaseRevision: int64(rev + 1), Operations: ops,
		}); err != nil {
			t.Fatal(err)
		}
	}

	list, err := e.svc.ImageCheckpointList(person, service.ImageCheckpointListRequest{ImageID: id})
	if err != nil || len(list.Checkpoints) != 1 || !list.Checkpoints[0].Auto || list.Checkpoints[0].Revision != 2 ||
		!strings.HasPrefix(list.Checkpoints[0].Name, "auto "+agent+" ") {
		t.Fatalf("list = %v %+v", err, list)
	}

	name := list.Checkpoints[0].Name

	res, err := e.svc.ImageRevert(person, service.ImageRevertRequest{ImageID: id, Checkpoint: name})
	if err != nil || !res.Applied || res.Target != 2 || res.Revision != 5 {
		t.Fatalf("revert = %v %+v", err, res)
	}

	desc, _ := e.svc.ImageDescribe(person, service.ImageDescribeRequest{ImageID: id})
	if len(desc.Layers) != 1 || desc.Layers[0].Opacity != 100 {
		t.Fatalf("the document after the revert: %+v", desc.Layers)
	}

	if undone, err := e.svc.ImageUndo(person, service.ImageStepRequest{ImageID: id}); err != nil || !undone.Applied {
		t.Fatalf("undo of the revert: %v %+v", err, undone)
	}

	cp, err := e.svc.ImageCheckpointCreate(person, service.ImageCheckpointCreateRequest{ImageID: id, Name: "mine"})
	if err != nil || cp.Revision != 6 {
		t.Fatalf("create = %v %+v", err, cp)
	}

	// Another user reaches none of it, and the answer is that of an image that does not exist.
	alice, bob := network("user:alice", "alice"), network("user:bob", "bob")

	own, err := e.svc.ImageCreate(bob, service.ImageCreateRequest{Preset: "square_post"})
	if err != nil {
		t.Fatal(err)
	}

	calls := map[string]func() error{
		"ImageRevert": func() error {
			_, err := e.svc.ImageRevert(alice, service.ImageRevertRequest{ImageID: own.ImageID, Revision: 1})
			return err
		},
		"ImageCheckpointCreate": func() error {
			_, err := e.svc.ImageCheckpointCreate(
				alice,
				service.ImageCheckpointCreateRequest{ImageID: own.ImageID, Name: "x"},
			)
			return err
		},
		"ImageCheckpointList": func() error {
			_, err := e.svc.ImageCheckpointList(alice, service.ImageCheckpointListRequest{ImageID: own.ImageID})
			return err
		},
		"ImageCheckpointRename": func() error {
			_, err := e.svc.ImageCheckpointRename(
				alice,
				service.ImageCheckpointRenameRequest{ImageID: own.ImageID, Name: "x", NewName: "y"},
			)
			return err
		},
		"ImageCheckpointRemove": func() error {
			_, err := e.svc.ImageCheckpointRemove(
				alice,
				service.ImageCheckpointRemoveRequest{ImageID: own.ImageID, Name: "x"},
			)
			return err
		},
	}

	for name, call := range calls {
		if err := call(); code(err) != command.CodeNotFound {
			t.Errorf("%s on another user's image = %v, want E_NOT_FOUND", name, err)
		}
	}
}
