package library

import (
	"context"
	"errors"
	"reflect"
	"testing"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

func TestPromptWriteAndRender(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	ctx := context.Background()

	_, e, err := m.SavePrompt(ctx, person, PromptInput{
		Name: "video-intro", Description: "An intro", Defaults: map[string]string{"tone": "warm"},
		Body: "Write a {{tone}} intro about {{topic}}.\n",
	})
	if err != nil || e.Name != "video-intro" || e.Actor != "local:ui" {
		t.Fatalf("%v %+v", err, e)
	}

	out, p, err := m.Render(ctx, "video-intro", map[string]string{"topic": "cats"})
	if err != nil || out != "Write a warm intro about cats.\n" ||
		!reflect.DeepEqual(core.Variables(p.Body), []string{"tone", "topic"}) {
		t.Fatalf("%v %q", err, out)
	}

	out, _, err = m.Render(ctx, "video-intro", map[string]string{"topic": "{{tone}}", "tone": "cold"})
	if err != nil || out != "Write a cold intro about {{tone}}.\n" {
		t.Fatalf("a value is never read for variables again: %v %q", err, out)
	}
}

func TestPromptRenderNamesTheProblemVariables(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	ctx := context.Background()

	if _, _, err := m.SavePrompt(
		ctx,
		person,
		PromptInput{Name: "p", Description: "d", Body: "{{a}} {{b}}"},
	); err != nil {
		t.Fatal(err)
	}

	_, _, err := m.Render(ctx, "p", map[string]string{"a": "1", "zzz": "2"})

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("%v", err)
	}

	if !reflect.DeepEqual(ve.Details["missing"], []string{"b"}) ||
		!reflect.DeepEqual(ve.Details["unknown"], []string{"zzz"}) {
		t.Fatalf("%+v", ve.Details)
	}

	if _, _, err := m.Render(ctx, "missing", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
}

func TestPromptRewriteKeepsTheCreationTimeAndLimitsHold(t *testing.T) {
	t.Parallel()

	m, _, _ := newManager(t)
	ctx := context.Background()

	first, _, err := m.SavePrompt(ctx, person, PromptInput{Name: "p", Description: "d", Body: "one {{x}}"})
	if err != nil {
		t.Fatal(err)
	}

	second, _, err := m.SavePrompt(ctx, agent, PromptInput{Name: "p", Description: "d2", Body: "two {{x}}"})
	if err != nil || second.Created != first.Created || second.Actor != agent.Actor {
		t.Fatalf("%v %+v", err, second)
	}

	for name, in := range map[string]PromptInput{
		"name":           {Name: "A", Description: "d", Body: "x"},
		"no body":        {Name: "p2", Description: "d"},
		"default":        {Name: "p2", Description: "d", Body: "x", Defaults: map[string]string{"q": "1"}},
		"no description": {Name: "p2", Body: "x"},
	} {
		var ve *ValidationError
		if _, _, err := m.SavePrompt(ctx, person, in); !errors.As(err, &ve) {
			t.Errorf("%s: %v", name, err)
		}
	}

	limited := Caller{Actor: "key:key_0123456789", ProjectID: "prj_0123456789"}
	if _, _, err := m.SavePrompt(
		ctx,
		limited,
		PromptInput{Name: "p3", Description: "d", Body: "x"},
	); !errors.Is(
		err,
		ErrForbidden,
	) {
		t.Fatalf("%v", err)
	}
}
