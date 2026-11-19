package servicetest

import (
	"errors"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// TestImageOverTheRealStore runs the image operations of SDD-021 on the SQLite store: the batch with its key and
// its conflict, the undo and the redo of an actor, the export and the append-only log.
func TestImageOverTheRealStore(t *testing.T) {
	e := newEnvWith(t, func(e *env, o *service.Options) { o.Workspace = string(e.dir) })
	ctx := e.as(agent)

	created, err := e.svc.ImageCreate(ctx, service.ImageCreateRequest{Preset: "video_thumbnail", Name: "Thumb"})
	if err != nil {
		t.Fatal(err)
	}

	id := created.ImageID
	add := []command.Operation{
		op("layer.add_fill", `{"color": "#101820", "id": "lyr_aaaaaaaaaa"}`),
		op("layer.add_text", `{"text": "Hello", "id": "lyr_bbbbbbbbbb"}`),
	}

	res, err := e.svc.ImageEdit(
		ctx,
		service.ImageEditRequest{ImageID: id, BaseRevision: 1, Operations: add, IdempotencyKey: "image-key-1"},
	)
	if err != nil || res.Revision != 2 {
		t.Fatalf("%v %+v", err, res)
	}

	replay, err := e.svc.ImageEdit(
		ctx,
		service.ImageEditRequest{ImageID: id, BaseRevision: 1, Operations: add, IdempotencyKey: "image-key-1"},
	)
	if err != nil || !replay.Replayed {
		t.Fatalf("%v %+v", err, replay)
	}

	if _, err := e.svc.ImageEdit(
		e.as(ui),
		service.ImageEditRequest{ImageID: id, BaseRevision: 2, Operations: []command.Operation{
			op("layer.transform", `{"layer_id": "lyr_bbbbbbbbbb", "x": 10}`),
		}},
	); err != nil {
		t.Fatal(err)
	}

	_, err = e.svc.ImageEdit(
		ctx,
		service.ImageEditRequest{ImageID: id, BaseRevision: 2, Operations: []command.Operation{
			op("layer.set", `{"layer_id": "lyr_bbbbbbbbbb", "name": "x"}`),
		}},
	)

	var se *service.Error
	if !errors.As(err, &se) || se.Code != command.CodeRevisionConflict {
		t.Fatalf("want a conflict: %v", err)
	}

	undone, err := e.svc.ImageUndo(ctx, service.ImageStepRequest{ImageID: id})
	if err != nil || !undone.Applied || undone.Step != 2 {
		t.Fatalf("%v %+v", err, undone)
	}

	if _, err := e.svc.ImageRedo(ctx, service.ImageStepRequest{ImageID: id}); err != nil {
		t.Fatal(err)
	}

	desc, err := e.svc.ImageDescribe(ctx, service.ImageDescribeRequest{ImageID: id})
	if err != nil || desc.Revision != 5 || len(desc.Layers) != 2 {
		t.Fatalf("%v %+v", err, desc)
	}

	out, err := e.svc.ImageExport(ctx, service.ImageExportRequest{ImageID: id, Format: "jpeg"})
	if err != nil || out.Bytes == 0 {
		t.Fatalf("%v %+v", err, out)
	}

	list, err := e.svc.ImageList(ctx, service.ImageListRequest{})
	if err != nil || len(list.Images) != 1 || list.Images[0].Layers != 2 {
		t.Fatalf("%v %+v", err, list)
	}

	// the log is append-only: a row cannot be rewritten (SDD-005 section 5.12)
	if _, err := e.store.DB().
		ExecContext(ctx, `UPDATE image_operations SET actor = 'x' WHERE image_id = ?`, id); err == nil {
		t.Error("the log of an image was rewritten")
	}
}
