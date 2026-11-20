package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
)

// BuiltInTemplate is a template that ships with the product (SDD-018 section 17): a fixed identifier and the
// canonical document. The service knows nothing of where it comes from.
type BuiltInTemplate struct {
	ID       string
	Document []byte
}

// SeedBuiltInTemplates installs timeline templates that ship with the product into the template library, as the
// starter templates are (ADR-0022 item 8): shared, each with a fixed identifier, so that a workspace that holds one
// is left as it is and a template that a migration or a person changed is never overwritten. It returns how many
// it added.
func SeedBuiltInTemplates(ctx context.Context, st Store, nowMs int64, set []BuiltInTemplate) (int, error) {
	added := 0

	for _, s := range set {
		ok, err := seedTimelineTemplate(ctx, st, nowMs, s.ID, s.Document)
		if err != nil {
			return added, err
		}

		if ok {
			added++
		}
	}

	return added, nil
}

// SeedBuiltInImageTemplates installs image templates that ship with the product into the library of images, with
// the same rules as SeedBuiltInTemplates. In network mode they belong to no user and are shared and read only
// (SharedImageTemplate). It returns how many it added.
func SeedBuiltInImageTemplates(ctx context.Context, st Store, nowMs int64, set []BuiltInTemplate) (int, error) {
	added := 0

	for _, s := range set {
		ok, err := seedImageTemplate(ctx, st, nowMs, s)
		if err != nil {
			return added, err
		}

		if ok {
			added++
		}
	}

	return added, nil
}

func seedImageTemplate(ctx context.Context, st Store, nowMs int64, s BuiltInTemplate) (bool, error) {
	if _, gerr := st.Images().Get(ctx, s.ID); gerr == nil {
		return false, nil
	} else if !errors.Is(gerr, ErrNotFound) {
		return false, fmt.Errorf("service: built-in image template %s: %w", s.ID, gerr)
	}

	doc, derr := coreimage.Decode(s.Document)
	if derr != nil {
		return false, fmt.Errorf("service: built-in image template %s: %w", s.ID, derr)
	}

	if !doc.Template || doc.ID != s.ID {
		return false, fmt.Errorf("service: built-in image template %s is not a template document with that id", s.ID)
	}

	u := imageUpdate(doc, nowMs)
	row := ImageRow{
		Document: u.Document, DocSHA256: u.DocSHA256,
	}
	row.ID, row.Name, row.Template, row.Revision, row.SchemaVersion = s.ID, u.Name, true, 1, u.SchemaVersion
	row.Width, row.Height, row.Layers, row.CreatedMs, row.UpdatedMs = u.Width, u.Height, u.Layers, nowMs, nowMs
	row.CreatedBy = StarterActor

	err := st.Write(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.Images().Create(ctx, row); err != nil {
			return err
		}

		return tx.Images().AppendOp(ctx, ImageOpRow{
			ImageID: s.ID, Revision: 1, Kind: OpEdit, Actor: StarterActor, BaseRevision: 0, AtMs: nowMs,
			Ops:     marshalOr([]command.Operation{{Op: "image.create", Args: json.RawMessage(`{"template":true}`)}}),
			Inverse: json.RawMessage("[]"), Diff: json.RawMessage("[]"), DocSHA256: u.DocSHA256,
		})
	})

	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrExists):
		return false, nil
	default:
		return false, fmt.Errorf("service: built-in image template %s: %w", s.ID, err)
	}
}
