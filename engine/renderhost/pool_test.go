package renderhost

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

func TestPoolGivesBackACleanInstanceAndKeepsItsMemory(t *testing.T) {
	ctx := context.Background()
	p := NewPool(Config{Uncancellable: true}, 2, 1<<30, time.Hour)

	defer p.Drain()

	h, err := p.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := h.PutTexture(ctx, "a", 64, 64, make([]byte, 64*64*4), true); err != nil {
		t.Fatal(err)
	}

	grown := h.MemoryBytes()

	p.Put(h)

	again, err := p.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if again != h {
		t.Fatal("an idle instance was not reused")
	}

	if again.MemoryBytes() < grown {
		t.Fatalf("the instance lost the memory it had grown to: %d, then %d", grown, again.MemoryBytes())
	}

	// Nothing the last user stored is left: its texture is gone.
	in := RenderInput{
		Output: Output{Width: 8, Height: 8, Scale: 1},
		Plan: plan.FramePlan{Width: 8, Height: 8, Layers: []plan.Layer{
			{ItemID: "a", Kind: plan.Media, W: 4, H: 4, CX: 4, CY: 4, Opacity: 1},
		}},
	}

	err = again.Render(ctx, in, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	if re, ok := errors.AsType[*Error](err); !ok || re.Reason != MissingTexture {
		t.Fatalf("a reused instance still held the texture of its last user: %v", err)
	}
}

func TestPoolKeepsAtMostWhatItIsGiven(t *testing.T) {
	ctx := context.Background()

	var hosts []*Host

	p := NewPool(Config{Uncancellable: true}, 2, 1<<30, time.Hour)

	defer p.Drain()

	for range 3 {
		h, err := p.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}

		hosts = append(hosts, h)
	}

	for _, h := range hosts {
		p.Put(h)
	}

	if p.Idle() != 2 {
		t.Fatalf("a pool of two holds %d", p.Idle())
	}

	if hosts[2].dead == nil {
		t.Fatal("the instance that did not fit was not closed")
	}

	// A pool that allows no memory keeps nothing.
	none := NewPool(Config{Uncancellable: true}, 4, 1, time.Hour)

	h, err := none.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}

	none.Put(h)

	if none.Idle() != 0 {
		t.Fatal("an instance over the memory bound was kept")
	}

	// A failed instance is not kept.
	h, err = p.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}

	_ = h.Close(ctx)

	before := p.Idle()
	p.Put(h)

	if p.Idle() != before {
		t.Fatal("a closed instance was kept")
	}
}

func TestPoolClosesItsInstancesWhenNoneIsTakenForItsTTL(t *testing.T) {
	ctx := context.Background()
	p := NewPool(Config{Uncancellable: true}, 2, 1<<30, 20*time.Millisecond)

	h, err := p.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}

	p.Put(h)

	deadline := time.Now().Add(10 * time.Second)
	for p.Idle() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("the idle instance was never closed")
		}

		time.Sleep(10 * time.Millisecond)
	}
}
