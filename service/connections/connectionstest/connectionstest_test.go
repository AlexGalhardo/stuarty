package connectionstest

import (
	"errors"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/connections"
)

func TestStoreListsInOrderAndRemovesOnlyTheSelectedConnection(t *testing.T) {
	t.Parallel()

	h := New(t, "fake", "https://example.com", "test-value")
	ctx := t.Context()
	other := connections.Row{ID: "con_aaaaaaaaaa", Kind: "fake"}
	if err := h.Store.Insert(ctx, other); err != nil {
		t.Fatal(err)
	}

	rows, err := h.Store.List(ctx)
	if err != nil || len(rows) != 2 || rows[0].ID != other.ID || rows[1].ID != ID {
		t.Fatalf("List = %v, %v", rows, err)
	}

	if err := h.Store.Delete(ctx, other.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := h.Store.Get(ctx, other.ID); !errors.Is(err, connections.ErrNotFound) {
		t.Fatalf("Get after Delete = %v", err)
	}

	if err := h.Store.Delete(ctx, other.ID); !errors.Is(err, connections.ErrNotFound) {
		t.Fatalf("second Delete = %v", err)
	}

	if n, err := h.Store.Count(ctx); n != 1 || err != nil {
		t.Fatalf("Count = %d, %v", n, err)
	}
}
