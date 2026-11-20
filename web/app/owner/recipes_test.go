package owner

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
	"github.com/AlexGalhardo/stuarty/web/app/store"
	"github.com/AlexGalhardo/stuarty/web/resources/templates"
)

// SDD-018 section 17 and VV-291: the recipes install their timeline templates and their image templates in
// every mode; in network mode the image templates have no owner (service.SharedImageTemplate).
func TestEveryModeInstallsTheRecipeImageTemplates(t *testing.T) {
	t.Parallel()

	timelines, err := templates.Timelines()
	if err != nil {
		t.Fatal(err)
	}

	images, err := templates.Images()
	if err != nil {
		t.Fatal(err)
	}

	for name, network := range map[string]*httpapi.Network{"local": nil, "network": {}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			dir := workspace.Dir(t.TempDir())

			st, err := store.Open(ctx, dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = st.Close() })

			o := &Owner{Dir: dir, now: time.Now, network: network}
			if err := o.seedRecipeTemplates(ctx, st); err != nil {
				t.Fatal(err)
			}

			for _, want := range timelines {
				if _, err := st.Templates().Get(ctx, want.ID); err != nil {
					t.Errorf("%s mode: no timeline template %s: %v", name, want.Name, err)
				}
			}

			for _, want := range images {
				row, gerr := st.Images().Get(ctx, want.ID)
				if gerr != nil {
					t.Errorf("%s mode: no image template %s: %v", name, want.Name, gerr)

					continue
				}

				if !service.SharedImageTemplate(row.ImageHead) {
					t.Errorf("%s mode: the image template %s is not shared: %+v", name, want.Name, row.ImageHead)
				}
			}
		})
	}
}

// SDD-018 section 17: a fresh workspace lists the templates of the recipes, the timeline ones in the library of
// templates and the covers in the library of images, and starting it again adds nothing and changes nothing.
func TestANewWorkspaceHoldsTheRecipeTemplates(t *testing.T) {
	getenv := envOf(
		map[string]string{workspace.EnvHome: filepath.Join(t.TempDir(), "ws"), "STUARTY_MIN_FREE_BYTES": "1"},
	)
	ctx := service.WithCaller(context.Background(), service.Caller{Actor: "local:cli", Scopes: service.AllScopes})

	timelines, err := templates.Timelines()
	if err != nil {
		t.Fatal(err)
	}

	images, err := templates.Images()
	if err != nil {
		t.Fatal(err)
	}

	for start := 1; start <= 2; start++ {
		o, err := Open(ctx, Config{Getenv: getenv, Inline: true})
		if err != nil {
			t.Fatalf("start %d: %v", start, err)
		}

		tpls, lerr := o.Service.TemplateList(ctx, service.TemplateListRequest{Limit: 100})
		if lerr != nil {
			t.Fatal(lerr)
		}

		have := map[string]bool{}
		for _, s := range tpls.Templates {
			have[s.TemplateID] = true
		}

		for _, want := range timelines {
			if !have[want.ID] {
				t.Errorf("start %d: the library of templates has no %s (%s)", start, want.Name, want.ID)
			}
		}

		imgs, ierr := o.Service.ImageList(ctx, service.ImageListRequest{Limit: 100, Templates: "only"})
		if ierr != nil {
			t.Fatal(ierr)
		}

		var ids []string
		for _, s := range imgs.Images {
			ids = append(ids, s.ImageID)
		}

		for _, want := range images {
			if !slices.Contains(ids, want.ID) {
				t.Errorf("start %d: the library of images has no %s (%s): %v", start, want.Name, want.ID, ids)
			}
		}

		if len(ids) != len(images) {
			t.Errorf("start %d: %d image templates, want %d", start, len(ids), len(images))
		}

		// The template is usable: asked with the texts and no picture, it says the picture is what is missing.
		title, sub := "A title", "A line"

		_, merr := o.Service.ImageCreate(ctx, service.ImageCreateRequest{
			TemplateID: images[0].ID,
			Values:     map[string]coreimage.SlotValue{"title": {Text: &title}, "subtitle": {Text: &sub}},
		})

		var se *service.Error
		if !errors.As(merr, &se) || se.Code != "E_SLOT" || !strings.Contains(se.Message, "picture") {
			t.Errorf("start %d: a cover with no picture = %v, want E_SLOT naming the picture", start, merr)
		}

		if err := o.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
