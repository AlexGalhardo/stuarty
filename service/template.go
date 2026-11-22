package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const (
	defaultTemplateLimit = 20
	maxTemplateLimit     = 100
	// maxTemplateText is the 500 characters that template.set_info allows in the
	// description of a template (core/command/template.go).
	maxTemplateText = 500
)

// templateRefused is the refusal of a key limited to one project: a template
// belongs to no project (SDD-007 section 12, ADR-0022).
func templateRefused(c Caller) error {
	if c.ProjectID == "" {
		return nil
	}

	return newError(CodeForbidden, "the key is limited to one project and cannot use templates.").
		withHint("use a key without a project limit")
}

// TemplateSave copies a project into the template library (SDD-005 section 15).
// The media of the slots become placeholders; no log row is written, and saving
// again makes another template.
func (s *svc) TemplateSave(ctx context.Context, req TemplateSaveRequest) (*TemplateResult, error) {
	c, err := s.authorizeProject(ctx, "TemplateSave", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &req.ProjectID, req.TimelineID); err != nil {
		return nil, err
	}

	if err := templateRefused(c); err != nil {
		return nil, err
	}

	if n := utf8.RuneCountInString(req.Name); n < 1 || n > maxProjectName {
		return nil, newError(
			command.CodeValidation,
			"the name has %d characters; it must have 1 to %d.",
			n,
			maxProjectName,
		).
			withPath("name")
	}

	if utf8.RuneCountInString(req.Description) > maxTemplateText {
		return nil, newError(command.CodeValidation, "the description is longer than %d characters.", maxTemplateText).
			withPath("description")
	}

	project, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	doc, derr := timeline.Decode(project.Document)
	if derr != nil {
		return nil, internalError()
	}

	tdoc, verr := templateDocument(doc, req.Name, req.Description)
	if verr != nil {
		return nil, verr
	}

	canon := timeline.Canonical(tdoc)
	slots := slotInfos(tdoc)

	slotsJSON, merr := json.Marshal(slots)
	if merr != nil {
		return nil, internalError()
	}

	now := s.nowMs()
	source := project.ID
	row := TemplateRow{
		ID: s.newIDs("tpl"), Name: req.Name, Description: req.Description, SchemaVersion: tdoc.SchemaVersion,
		Document: canon, DocSHA256: sha256Hex(canon), Slots: slotsJSON, Summary: summarize(tdoc),
		SourceProjectID: &source, CreatedMs: now, CreatedBy: c.Actor,
	}

	if c.OwnerUserID != "" {
		owner := c.OwnerUserID
		row.OwnerUserID = &owner
	}

	res := &TemplateResult{Template: templateSummary(&row, tdoc)}
	res.UntrustedNotice = UntrustedNotice
	res.Text = "template " + row.ID + " saved: " + templateLine(&res.Template)

	if req.DryRun {
		res.Text = "template " + row.ID + " would be saved: " + templateLine(&res.Template)

		return res, nil
	}

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.Templates().Create(ctx, row); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "template.save", project.ID, row.ID)
	})
	if err != nil {
		return nil, asError(err)
	}

	return res, nil
}

// templateDocument turns a project document into a template document: the
// assets are gone, every media slot holds the placeholder, every slot is
// unfilled, and a media item that is not a slot is refused because a template
// carries no media (ADR-0022).
func templateDocument(doc *timeline.Document, name, description string) (*timeline.Document, *Error) {
	t := timeline.Clone(doc)
	t.Name = name
	t.Assets = map[string]timeline.Asset{}

	if t.Template == nil {
		t.Template = &timeline.TemplateInfo{}
	}

	t.Template.Description = description

	for _, id := range sortedKeys(t.Items) {
		it := t.Items[id]
		media := it.Type == timeline.ItemClip || it.Type == timeline.ItemAudio

		if it.Slot != nil {
			it.Slot.Filled = false
		}

		if media {
			if it.Slot == nil && it.Asset != timeline.PlaceholderAsset {
				return nil, newError(
					command.CodeSlot,
					"item %s uses media and is not a slot; a template carries no media.",
					id,
				).
					withHint("mark it with slot.mark or delete it before saving").
					withDetail("item", id)
			}

			it.Asset = timeline.PlaceholderAsset
		}

		t.Items[id] = it
	}

	if vs := timeline.Validate(t); len(vs) > 0 {
		return nil, newError(command.CodeValidation, "the template document is not valid: %s", violationLine(vs[0])).
			withHint("fix the project and save again")
	}

	return t, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	return keys
}

// slotInfos lists the slots of a document ordered by name.
func slotInfos(doc *timeline.Document) []SlotInfo {
	out := []SlotInfo{}

	for _, id := range sortedKeys(doc.Items) {
		sl := doc.Items[id].Slot
		if sl == nil {
			continue
		}

		info := SlotInfo{Name: sl.Name, Kind: sl.Kind, Required: sl.Required, Description: sl.Description}
		if sl.Kind == "media" {
			info.DurationPolicy = sl.DurationPolicy
		}

		out = append(out, info)
	}

	slices.SortFunc(out, func(a, b SlotInfo) int { return strings.Compare(a.Name, b.Name) })

	return out
}

// templateSummary is the listing form of a row; doc, when known, adds the colours.
func templateSummary(row *TemplateRow, doc *timeline.Document) TemplateSummary {
	sum := TemplateSummary{
		TemplateID: row.ID, Name: row.Name, Description: row.Description, Slots: []SlotInfo{},
		Width: row.Summary.Width, Height: row.Summary.Height,
		Duration: timeValue(
			timeline.Rational{Num: row.Summary.FPSNum, Den: row.Summary.FPSDen}, row.Summary.DurationFrames,
		),
	}

	if len(row.Slots) > 0 {
		_ = json.Unmarshal(row.Slots, &sum.Slots)
	}

	if doc != nil && doc.Template != nil && len(doc.Template.Colors) > 0 {
		sum.Colors = map[string]string{}
		for k, v := range doc.Template.Colors {
			sum.Colors[k] = string(v)
		}
	}

	return sum
}

func templateLine(t *TemplateSummary) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s %q %dx%d %.3gs slots:", t.TemplateID, t.Name, t.Width, t.Height, t.Duration.Seconds)

	for _, sl := range t.Slots {
		star := ""
		if sl.Required {
			star = "*"
		}

		fmt.Fprintf(&b, " %s:%s%s", sl.Name, sl.Kind, star)
	}

	return b.String()
}

// TemplateList pages the library, newest first.
func (s *svc) TemplateList(ctx context.Context, req TemplateListRequest) (*TemplateListResult, error) {
	c, err := authorize(ctx, "TemplateList", "")
	if err != nil {
		return nil, err
	}

	if err := templateRefused(c); err != nil {
		return nil, err
	}

	if err := checkLimit(req.Limit); err != nil {
		return nil, err
	}

	rows, next, lerr := s.store.Templates().
		List(ctx, clampLimit(req.Limit, defaultTemplateLimit, maxTemplateLimit), req.Cursor, c.OwnerUserID)
	if lerr != nil {
		return nil, listError(lerr)
	}

	res := &TemplateListResult{Templates: []TemplateSummary{}}
	res.UntrustedNotice = UntrustedNotice

	var text strings.Builder

	for i := range rows {
		var doc *timeline.Document

		if full, gerr := s.store.Templates().Get(ctx, rows[i].ID); gerr == nil {
			doc, _ = timeline.Decode(full.Document)
		}

		sum := templateSummary(&rows[i], doc)
		res.Templates = append(res.Templates, sum)
		text.WriteString(templateLine(&sum) + "\n")
	}

	if next != "" {
		res.NextCursor = &next
	}

	res.Text = fmt.Sprintf("templates (%d)\n%s", len(res.Templates), text.String())

	return res, nil
}

// visibleTemplate reads a template the caller may see. A template of another
// owner answers exactly like one that does not exist.
func (s *svc) visibleTemplate(ctx context.Context, c Caller, id string) (TemplateRow, error) {
	row, err := s.store.Templates().Get(ctx, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return TemplateRow{}, asError(err)
	}

	if err != nil || (!visible(c, row.OwnerUserID) && !sharedTemplate(&row)) {
		return TemplateRow{}, newError(command.CodeNotFound, "there is no template %s.", id).
			withHint("list the templates with template_list")
	}

	return row, nil
}

// fillOps turns the fill of a request into one batch: the asset.add of every
// media asset, then a slot.fill per slot, in slot-name order.
func (s *svc) fillOps(ctx context.Context, c Caller, fill map[string]SlotValue) ([]command.Operation, *Error) {
	var adds, fills []command.Operation

	added := map[string]bool{}

	for _, name := range sortedKeys(fill) {
		v := fill[name]
		path := "fill." + command.SafeKey(name)

		given := 0
		if v.AssetID != "" {
			given++
		}

		if v.Text != nil {
			given++
		}

		if len(v.Words) > 0 {
			given++
		}

		if given != 1 {
			return nil, newError(
				command.CodeValidation,
				"the value of slot %q must give exactly one of asset_id, text or words.",
				command.SafeKey(name),
			).
				withPath(path)
		}

		args := map[string]any{"name": name}

		switch {
		case v.AssetID != "":
			args["asset_id"] = v.AssetID

			if v.SourceIn != "" {
				args["source_in"] = v.SourceIn
			}

			if !added[v.AssetID] {
				snap, e := s.visibleAsset(ctx, c, v.AssetID)
				if e != nil {
					return nil, e.withPath(path + ".asset_id")
				}

				raw, merr := json.Marshal(map[string]json.RawMessage{"asset": snap})
				if merr != nil {
					return nil, internalError()
				}

				adds = append(adds, command.Operation{Op: "asset.add", Args: raw})
				added[v.AssetID] = true
			}
		case v.Text != nil:
			args["text"] = *v.Text
		default:
			args["words"] = v.Words
		}

		raw, merr := json.Marshal(args)
		if merr != nil {
			return nil, internalError()
		}

		fills = append(fills, command.Operation{Op: "slot.fill", Args: raw})
	}

	return append(adds, fills...), nil
}

// visibleAsset finds the snapshot of an active asset of a project the caller
// may see. An asset that is not visible is E_NOT_FOUND, the same answer as one
// that does not exist, so no other owner's asset is disclosed.
func (s *svc) visibleAsset(ctx context.Context, c Caller, assetID string) (json.RawMessage, *Error) {
	notFound := newError(command.CodeNotFound, "there is no asset %s.", assetID).
		withHint("import the file into one of your projects and use its asset id")

	after := ""

	for {
		heads, next, err := s.store.Projects().List(ctx, ProjectQuery{
			After: after, Limit: 100, OwnerUserID: c.OwnerUserID, ProjectID: c.ProjectID,
		})
		if err != nil {
			return nil, internalError()
		}

		for i := range heads {
			pa, gerr := s.store.Assets().GetProjectAsset(ctx, heads[i].ID, assetID)
			if gerr == nil && pa.State == ProjectAssetActive && len(pa.Snapshot) > 0 {
				return pa.Snapshot, nil
			}

			if gerr != nil && !errors.Is(gerr, ErrNotFound) {
				return nil, internalError()
			}
		}

		if next == "" {
			return nil, notFound
		}

		after = next
	}
}

// TemplateInstantiate creates a project from a template: revision 0, then one
// batch of fills as revision 1 when values were given (SDD-005 section 15).
// Required slots left empty are reported, not refused (RF-051).
func (s *svc) TemplateInstantiate(
	ctx context.Context,
	req TemplateInstantiateRequest,
) (*TemplateInstantiateResult, error) {
	c, err := authorize(ctx, "TemplateInstantiate", "")
	if err != nil {
		return nil, err
	}

	if err := templateRefused(c); err != nil {
		return nil, err
	}

	if c.OwnerUserID != "" && req.ID != "" {
		return nil, newError(command.CodeValidation, "the id of a project is chosen by the server.").
			withPath("id").withHint("leave id out")
	}

	if n := utf8.RuneCountInString(req.Name); n < 1 || n > maxProjectName {
		return nil, newError(
			command.CodeValidation,
			"the name has %d characters; it must have 1 to %d.",
			n,
			maxProjectName,
		).
			withPath("name")
	}

	tpl, gerr := s.visibleTemplate(ctx, c, req.TemplateID)
	if gerr != nil {
		return nil, gerr
	}

	doc, derr := timeline.Decode(tpl.Document)
	if derr != nil {
		return nil, internalError()
	}

	doc.ID = req.ID
	if doc.ID == "" {
		doc.ID = s.newIDs(timeline.PrefixProject)
	} else if !timeline.ValidID(timeline.PrefixProject, doc.ID) {
		return nil, newError(command.CodeValidation, "the id %q is not a valid project identifier.", doc.ID).
			withPath("id")
	}

	doc.Name = req.Name

	// An instance is no template draft: its open slots are lint errors.
	if doc.Template == nil {
		doc.Template = &timeline.TemplateInfo{}
	}

	doc.Template.Instance = true

	if berr := s.applyBrandKit(ctx, c, doc, &req); berr != nil {
		return nil, berr
	}

	// The fonts of a template are named in the new project, so they follow the rule of font.add.
	if _, bad := s.unusableFont(ctx, s.store, doc, ""); bad {
		return nil, fontNotUsable("fonts")
	}

	ops, oerr := s.fillOps(ctx, c, req.Fill)
	if oerr != nil {
		return nil, oerr
	}

	// The batch is tried on a copy first: a bad fill must not leave a project behind.
	final := doc

	if len(ops) > 0 {
		out, aerr := s.reg.ApplyBatch(timeline.Clone(doc), ops, command.Env{NewIDs: s.newIDs})
		if aerr != nil {
			return nil, fromCommand(aerr)
		}

		final = out.Doc
	}

	res := &TemplateInstantiateResult{ProjectID: doc.ID, Unfilled: unfilled(final)}
	res.Findings = lint.Lint(final, s.lintEnv, lint.Fast)

	if req.DryRun {
		res.Text = instantiateText(tpl.ID, res, true)

		return res, nil
	}

	if err := s.createFromTemplate(ctx, c, doc, tpl.ID); err != nil {
		return nil, err
	}

	if len(ops) > 0 {
		er, eerr := s.edit(ctx, EditRequest{ProjectID: doc.ID, BaseRevision: 0, Operations: ops}, true)
		if eerr != nil {
			s.discard(ctx, c, doc.ID)

			return nil, eerr
		}

		res.Revision = er.Revision
	}

	res.Text = instantiateText(tpl.ID, res, false)

	return res, nil
}

func instantiateText(template string, r *TemplateInstantiateResult, dryRun bool) string {
	text := fmt.Sprintf("project %s made from %s at r%d", r.ProjectID, template, r.Revision)
	if dryRun {
		text = fmt.Sprintf("project %s would be made from %s", r.ProjectID, template)
	}

	if len(r.Unfilled) > 0 {
		text += "; unfilled: " + strings.Join(r.Unfilled, ", ")
	}

	return text
}

// unfilled lists the required slots that hold no value, by name.
func unfilled(doc *timeline.Document) []string {
	out := []string{}

	for _, id := range sortedKeys(doc.Items) {
		if sl := doc.Items[id].Slot; sl != nil && sl.Required && !sl.Filled {
			out = append(out, sl.Name)
		}
	}

	slices.Sort(out)

	return out
}

func (s *svc) createFromTemplate(ctx context.Context, c Caller, doc *timeline.Document, templateID string) error {
	canon := timeline.Canonical(doc)
	now := s.nowMs()
	row := ProjectRow{
		ID: doc.ID, Name: doc.Name, State: ProjectActive, SchemaVersion: doc.SchemaVersion,
		Summary: summarize(doc), CreatedMs: now, UpdatedMs: now, CreatedBy: c.Actor,
		Document: canon, DocSHA256: sha256Hex(canon),
	}

	if c.OwnerUserID != "" {
		owner := c.OwnerUserID
		row.OwnerUserID = &owner
	}

	err := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		if err := s.putProduction(ctx, tx, &row); err != nil {
			return err
		}

		if err := appendAudit(ctx, tx, now, c.Actor, "project.create", doc.ID, ""); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "template.instantiate", doc.ID, templateID)
	})
	if errors.Is(err, ErrExists) {
		return newError(command.CodeIDExists, "a project with the identifier %s already exists.", doc.ID).
			withPath("id").withHint("choose another id or leave it out")
	}

	return asError(err)
}

// discard puts a project whose first batch failed unexpectedly into the trash.
// Nothing is deleted: only an emptying of the trash removes data.
func (s *svc) discard(ctx context.Context, c Caller, id string) {
	_ = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()
		if err := tx.Productions().SetState(ctx, id, ProjectTrashed, c.Actor, now); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "project.trash", id, "")
	})
}
