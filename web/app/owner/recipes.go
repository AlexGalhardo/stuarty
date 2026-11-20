package owner

import (
	"context"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/resources/templates"
)

// seedTemplates fills the library of a new workspace: the starter templates (ADR-0022 item 8), then the templates
// of the recipes. A workspace that holds one is left as it is.
func (o *Owner) seedTemplates(ctx context.Context, st service.Store) error {
	if _, err := service.SeedStarterTemplates(ctx, st, o.now().UnixMilli()); err != nil {
		return err
	}

	return o.seedRecipeTemplates(ctx, st)
}

// seedRecipeTemplates installs the templates of the recipes for a commercial and a long video (SDD-018 section
// 17), in every mode. In network mode an image belongs to one user (SDD-021), and an image template of the
// product is the one exception: it has no owner, every user reads it and makes a document from it, and none
// changes it (service.SharedImageTemplate). A workspace that holds one is left as it is.
func (o *Owner) seedRecipeTemplates(ctx context.Context, st service.Store) error {
	now := o.now().UnixMilli()

	timelines, err := templates.Timelines()
	if err != nil {
		return err
	}

	if _, err := service.SeedBuiltInTemplates(ctx, st, now, builtIn(timelines)); err != nil {
		return err
	}

	images, err := templates.Images()
	if err != nil {
		return err
	}

	_, err = service.SeedBuiltInImageTemplates(ctx, st, now, builtIn(images))

	return err
}

func builtIn(in []templates.Template) []service.BuiltInTemplate {
	out := make([]service.BuiltInTemplate, 0, len(in))

	for _, t := range in {
		out = append(out, service.BuiltInTemplate{ID: t.ID, Document: t.Document})
	}

	return out
}
