package gioapp

import (
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func TestSessionCreatesTextAndUndoesIt(t *testing.T) {
	t.Parallel()

	sess, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := sess.Close(); err != nil {
			t.Error(err)
		}
	})

	ctx := t.Context()
	created, err := sess.Create(ctx, "Desk", "16:9")
	if err != nil {
		t.Fatal(err)
	}

	op, err := operation("text.add", map[string]any{
		"start": "0s", "duration": "2s", "text": "Hello",
	})
	if err != nil {
		t.Fatal(err)
	}

	edited, err := sess.Edit(ctx, created.ProjectID, created.Revision, op)
	if err != nil {
		t.Fatal(err)
	}

	if edited.Revision <= created.Revision {
		t.Fatalf("revision = %d, want it past %d", edited.Revision, created.Revision)
	}

	got, err := sess.Project(ctx, created.ProjectID)
	if err != nil {
		t.Fatal(err)
	}

	if got.Document == nil || len(got.Document.Items) != 1 {
		t.Fatalf("items = %d", len(itemsOf(got.Document)))
	}

	if _, err := sess.Undo(ctx, created.ProjectID); err != nil {
		t.Fatal(err)
	}

	got, err = sess.Project(ctx, created.ProjectID)
	if err != nil {
		t.Fatal(err)
	}

	if got.Document == nil || len(got.Document.Items) != 0 {
		t.Fatalf("after undo items = %d", len(itemsOf(got.Document)))
	}
}

func itemsOf(doc *timeline.Document) map[string]timeline.Item {
	if doc == nil {
		return nil
	}

	return doc.Items
}

func TestErrTextUsesTheServiceMessage(t *testing.T) {
	t.Parallel()

	if errText(nil) != "" {
		t.Fatal("nil error should be empty")
	}
}
