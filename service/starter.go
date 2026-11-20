package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/templates"
)

// StarterActor is the creator of the starter templates. A template with no owner
// and this creator is shared: in network mode every user sees it (and none can
// change it), while a template with no owner and another creator belongs to the
// local workspace and stays hidden from users.
const StarterActor = "system:starter"

// sharedTemplate reports whether a template is one of the starters.
func sharedTemplate(row *TemplateRow) bool {
	return row.OwnerUserID == nil && row.CreatedBy == StarterActor
}

// SeedStarterTemplates copies the starter templates into the template library
// (ADR-0022 item 8). Each has a fixed identifier, so a workspace that already
// holds it is left as it is: a later start adds nothing, and a template that a
// migration brought to a newer schema is not overwritten with the embedded one.
// It returns how many templates it added.
func SeedStarterTemplates(ctx context.Context, st Store, nowMs int64) (int, error) {
	starters, err := templates.Starters()
	if err != nil {
		return 0, err
	}

	added := 0

	for _, s := range starters {
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

// seedTimelineTemplate adds one template with a fixed identifier to the library, as a shared one. It reports
// whether it added the row: a workspace that holds the identifier is left as it is.
func seedTimelineTemplate(ctx context.Context, st Store, nowMs int64, id string, document []byte) (bool, error) {
	if _, gerr := st.Templates().Get(ctx, id); gerr == nil {
		return false, nil
	} else if !errors.Is(gerr, ErrNotFound) {
		return false, fmt.Errorf("service: built-in template %s: %w", id, gerr)
	}

	doc, derr := timeline.Decode(document)
	if derr != nil {
		return false, fmt.Errorf("service: built-in template %s: %w", id, derr)
	}

	slots, merr := json.Marshal(slotInfos(doc))
	if merr != nil {
		return false, merr
	}

	row := TemplateRow{
		ID: id, Name: doc.Name, Description: doc.Template.Description, SchemaVersion: doc.SchemaVersion,
		Document: document, DocSHA256: sha256Hex(document), Slots: slots, Summary: summarize(doc),
		CreatedMs: nowMs, CreatedBy: StarterActor,
	}

	err := st.Write(ctx, func(ctx context.Context, tx Tx) error { return tx.Templates().Create(ctx, row) })

	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrExists):
		return false, nil
	default:
		return false, fmt.Errorf("service: built-in template %s: %w", id, err)
	}
}
