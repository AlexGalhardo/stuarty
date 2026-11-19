package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/core/writing"
	"github.com/AlexGalhardo/stuarty/service/textview"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// The operations on the texts of a production (SDD-019). The rules are in core/writing; this file reads and writes
// the rows and turns a broken rule into an error.

// bodyViewBudget is how many characters of a body a text view carries (SDD-003 section 3.10).
const bodyViewBudget = textview.CapFull

// maxRevisionPage is the most revisions a page of TextRevisions holds.
const maxRevisionPage = 100

func writingViolation(v *writing.RuleError) *Error {
	e := newError(v.Code, "%s.", v.Message)
	e.Path, e.Hint = v.Path, v.Hint

	return e
}

func revisionNotFound(itemID string, n int) *Error {
	return newError(command.CodeNotFound, "there is no revision %d of the text %s.", n, itemID).
		withPath("revision").withHint("list the revisions with TextRevisions")
}

func textItemView(r *ItemRow, revision int, by string) TextItem {
	return TextItem{
		ID: r.ID, ProjectID: r.ProjectID, Title: r.Title, TextKind: r.TextKind, State: stateString(r.State),
		Words: writing.CountWords(r.Body), Revision: revision, UpdatedBy: by, UpdatedAt: msTime(r.UpdatedMs),
	}
}

func revisionView(r *TextRevisionRow) TextRevision {
	v := TextRevision{
		Revision: r.Revision, Actor: r.Actor, Source: r.Source, RevertedTo: r.RevertedTo, Provider: r.Provider,
		Model: r.Model, JobID: r.JobID, CreatedAt: msTime(r.CreatedMs),
	}

	if r.CostMicros != nil {
		v.Cost = &core.Money{Micros: *r.CostMicros, Currency: r.Currency}
	}

	return v
}

// textOf is the text item the project holds under id, or the error that says why there is none.
func textOf(rows []ItemRow, id string) (*ItemRow, error) {
	it := findItem(rows, id)
	if it == nil {
		return nil, itemNotFound(id)
	}

	return checkIsText(it)
}

func checkIsText(it *ItemRow) (*ItemRow, error) {
	if it.Kind != string(project.Text) {
		return nil, newError(command.CodeInvalidState, "the item %s is a %s, not a text.", it.ID, it.Kind).
			withPath("item_id").withHint("list the texts of the project with TextList")
	}

	return it, nil
}

// readText loads, for a read, the project the caller may see and one of its texts.
func (s *svc) readText(ctx context.Context, c Caller, projectID, itemID string) (ProductionHead, *ItemRow, error) {
	repo := s.store.Productions()

	head, err := s.loadProduction(ctx, repo, c, projectID)
	if err != nil {
		return ProductionHead{}, nil, err
	}

	it, err := repo.Item(ctx, itemID)
	if err != nil || it.ProjectID != head.ID {
		if err == nil || errors.Is(err, ErrNotFound) {
			return ProductionHead{}, nil, itemNotFound(itemID)
		}

		return ProductionHead{}, nil, asError(err)
	}

	if _, err := checkIsText(&it); err != nil {
		return ProductionHead{}, nil, err
	}

	return head, &it, nil
}

func (s *svc) latestRevision(ctx context.Context, repo ProductionRepo, itemID string) (TextRevisionRow, error) {
	rev, err := repo.TextLatest(ctx, itemID)
	if err != nil {
		return TextRevisionRow{}, asError(err)
	}

	return rev, nil
}

// TextList lists the texts of a project, or of every project the caller may see.
func (s *svc) TextList(ctx context.Context, req TextListRequest) (*TextListResult, error) {
	c, err := authorize(ctx, "TextList", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if req.TextKind != "" {
		if _, v := writing.CheckKind(req.TextKind); v != nil {
			return nil, writingViolation(v)
		}
	}

	state := ProjectState(req.State)
	if state != "" && state != ProjectActive && state != ProjectTrashed {
		return nil, newError(command.CodeValidation, "the state %q is not known.", req.State).
			withPath("state").withHint("use active or trashed")
	}

	if req.Limit < 0 || req.Limit > 100 {
		return nil, newError(command.CodeValidation, "limit is %d; it must be 1 to 100.", req.Limit).
			withPath("limit").withHint("omit limit for 50")
	}

	q := TextQuery{
		ProjectID: req.ProjectID, OwnerUserID: c.OwnerUserID, TextKind: req.TextKind, State: state, After: req.Cursor,
		Limit: req.Limit,
	}

	if req.ProjectID != "" {
		head, lerr := s.loadProduction(ctx, s.store.Productions(), c, req.ProjectID)
		if lerr != nil {
			return nil, lerr
		}

		q.ProjectID = head.ID
	} else if c.ProjectID != "" {
		q.ProjectID = c.ProjectID
	}

	heads, next, err := s.store.Productions().TextItems(ctx, q)
	if err != nil {
		if errors.Is(err, ErrBadCursor) {
			return nil, newError(command.CodeValidation, "the cursor is not one this server made.").
				withPath("cursor").withHint("start again without a cursor")
		}

		return nil, asError(err)
	}

	res := &TextListResult{UntrustedNotice: UntrustedNotice, Texts: make([]TextItem, 0, len(heads)), NextCursor: next}

	var b strings.Builder

	b.WriteString(textview.Note)

	for i := range heads {
		h := &heads[i]
		v := textItemView(&h.Item, h.Revision, h.RevisionBy)
		v.ProjectName = h.ProjectName
		res.Texts = append(res.Texts, v)

		fmt.Fprintf(&b, "\n%s %s %s rev %d, %d words, in %s %s", v.ID, v.TextKind, textview.Quote(v.Title, 60),
			v.Revision, v.Words, v.ProjectID, textview.Quote(v.ProjectName, 40))
	}

	switch {
	case len(heads) == 0:
		b.WriteString("\nno texts")
	case next != "":
		b.WriteString("\nnext_cursor: " + next)
	}

	res.Text = b.String()

	return res, nil
}

// TextGet reads a text with its body, or one of its revisions.
func (s *svc) TextGet(ctx context.Context, req TextGetRequest) (*TextGetResult, error) {
	c, err := authorize(ctx, "TextGet", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if req.Revision != 0 {
		if v := writing.CheckRevision(req.Revision); v != nil {
			return nil, writingViolation(v)
		}
	}

	head, it, err := s.readText(ctx, c, req.ProjectID, req.ItemID)
	if err != nil {
		return nil, err
	}

	repo := s.store.Productions()

	latest, err := s.latestRevision(ctx, repo, it.ID)
	if err != nil {
		return nil, err
	}

	rev := latest

	if req.Revision != 0 && req.Revision != latest.Revision {
		got, rerr := repo.TextRevision(ctx, it.ID, req.Revision)
		if rerr != nil {
			if errors.Is(rerr, ErrNotFound) {
				return nil, revisionNotFound(it.ID, req.Revision)
			}

			return nil, asError(rerr)
		}

		rev = got
	}

	item := textItemView(it, latest.Revision, latest.Actor)
	res := &TextGetResult{
		UntrustedNotice: UntrustedNotice, ProjectID: head.ID, Item: item, Revision: revisionView(&rev), Body: rev.Body,
	}
	res.Text = textview.Note + "\n" + fmt.Sprintf(
		"text %s %s %s revision %d of %d by %s, %d words\n",
		it.ID,
		it.TextKind,
		textview.Quote(it.Title, 60),
		rev.Revision,
		latest.Revision,
		rev.Actor,
		writing.CountWords(rev.Body),
	) +
		untrusted.Block(
			untrusted.SourceProjectText,
			untrusted.Attrs{"project": head.ID, "item": it.ID, "revision": strconv.Itoa(rev.Revision)},
			rev.Body,
			bodyViewBudget,
		)

	return res, nil
}

// TextWrite makes a text or saves a new revision of one.
func (s *svc) TextWrite(ctx context.Context, req TextWriteRequest) (*TextWriteResult, error) {
	c, err := authorize(ctx, "TextWrite", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if v := project.CheckText(req.Text); v != nil {
		return nil, violation(v)
	}

	action := "text.write"

	var kind writing.Kind

	if req.ItemID == "" {
		action = "text.create"

		if v := project.CheckTitle(req.Title); v != nil {
			return nil, violation(v)
		}

		var kerr *writing.RuleError
		if kind, kerr = writing.CheckKind(req.TextKind); kerr != nil {
			return nil, writingViolation(kerr)
		}
	} else if req.Title != "" {
		if v := project.CheckTitle(req.Title); v != nil {
			return nil, violation(v)
		}
	}

	var out writeOutcome

	head, _, err := s.changeProduction(ctx, c, action, req.ProjectID, req.DryRun,
		func(ctx context.Context, tx Tx, head *ProductionHead, rows []ItemRow) (*productionChange, error) {
			if req.ItemID == "" {
				return s.planTextCreate(&out, c, head, rows, &req, kind)
			}

			return s.planTextWrite(ctx, tx, &out, c, rows, &req)
		})
	if err != nil {
		return nil, err
	}

	return s.writeResult(&head, &out), nil
}

// writeOutcome is what the plan of a text write decided, for the answer.
type writeOutcome struct {
	item    ItemRow
	rev     TextRevisionRow
	created bool
	changed bool
}

func (s *svc) planTextCreate(
	out *writeOutcome, c Caller, head *ProductionHead, rows []ItemRow, req *TextWriteRequest, kind writing.Kind,
) (*productionChange, error) {
	if v := project.CheckRoom(coreItems(rows), project.Text); v != nil {
		return nil, violation(v)
	}

	now := s.nowMs()
	it := s.newItem(head.ID, string(project.Text), req.Title, len(rows), c.Actor, now)
	it.Body, it.TextKind = req.Text, string(kind)

	ids := append(idsOf(rows), it.ID)
	rows = append(rows, it)
	renumber(rows, ids)

	out.item, out.created, out.changed = it, true, true
	out.rev = TextRevisionRow{
		ItemID:    it.ID,
		Revision:  1,
		Body:      it.Body,
		Actor:     c.Actor,
		Source:    string(writing.SourceCreated),
		CreatedMs: now,
	}

	return &productionChange{rows: rows, itemID: it.ID, write: func(ctx context.Context, tx Tx) error {
		if err := tx.Productions().CreateItem(ctx, it); err != nil {
			return err
		}

		return tx.Productions().SetPositions(ctx, head.ID, ids)
	}}, nil
}

func (s *svc) planTextWrite(
	ctx context.Context, tx Tx, out *writeOutcome, c Caller, rows []ItemRow, req *TextWriteRequest,
) (*productionChange, error) {
	it, err := textOf(rows, req.ItemID)
	if err != nil {
		return nil, err
	}

	if it.State == ProjectTrashed {
		return nil, textInTrash(it.ID)
	}

	repo := tx.Productions()

	latest, err := s.latestRevision(ctx, repo, it.ID)
	if err != nil {
		return nil, err
	}

	if v := writing.CheckBase(latest.Revision, req.BaseRevision); v != nil {
		return nil, writingViolation(v).withDetail("current_revision", latest.Revision)
	}

	retitle := req.Title != "" && req.Title != it.Title
	rewrite := !writing.Unchanged(latest.Body, req.Text)

	out.item, out.rev = *it, latest
	ch := &productionChange{rows: rows, itemID: it.ID}

	if !retitle && !rewrite {
		return ch, nil
	}

	now := s.nowMs()
	next := TextRevisionRow{
		ItemID: it.ID, Revision: writing.NextRevision(latest.Revision), Body: req.Text, Actor: c.Actor,
		Source: string(writing.SourceWrite), CreatedMs: now,
	}

	if retitle {
		it.Title = req.Title
	}

	if rewrite {
		it.Body = req.Text
		out.rev = next
	}

	it.UpdatedMs = now
	out.item, out.changed = *it, true

	ch.write = func(ctx context.Context, tx Tx) error {
		if rewrite {
			if err := tx.Productions().AddTextRevision(ctx, next, writing.PruneBefore(next.Revision)); err != nil {
				return err
			}
		}

		return tx.Productions().UpdateItem(ctx, *it)
	}

	return ch, nil
}

func textInTrash(id string) *Error {
	return newError(command.CodeInvalidState, "the text %s is in the trash.", id).
		withPath("item_id").withHint("restore it with project_item restore before writing")
}

func (s *svc) writeResult(head *ProductionHead, out *writeOutcome) *TextWriteResult {
	item := textItemView(&out.item, out.rev.Revision, out.rev.Actor)
	res := &TextWriteResult{
		UntrustedNotice: UntrustedNotice, ProjectID: head.ID, Item: item, Revision: revisionView(&out.rev),
		Created: out.created, Changed: out.changed,
	}

	verb := "saved as revision"
	if !out.changed {
		verb = "unchanged, still at revision"
	} else if out.created {
		verb = "made at revision"
	}

	res.Text = textview.Note + "\n" + fmt.Sprintf("text %s %s %s %s %d, %d words", out.item.ID, out.item.TextKind,
		textview.Quote(out.item.Title, 60), verb, out.rev.Revision, item.Words)

	return res
}

// TextRevert brings an older revision back as a new one; the history is never rewritten.
func (s *svc) TextRevert(ctx context.Context, req TextRevertRequest) (*TextWriteResult, error) {
	c, err := authorize(ctx, "TextRevert", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if v := writing.CheckRevision(req.Revision); v != nil {
		return nil, writingViolation(v)
	}

	var out writeOutcome

	head, _, err := s.changeProduction(ctx, c, "text.revert", req.ProjectID, req.DryRun,
		func(ctx context.Context, tx Tx, _ *ProductionHead, rows []ItemRow) (*productionChange, error) {
			it, terr := textOf(rows, req.ItemID)
			if terr != nil {
				return nil, terr
			}

			if it.State == ProjectTrashed {
				return nil, textInTrash(it.ID)
			}

			repo := tx.Productions()

			latest, lerr := s.latestRevision(ctx, repo, it.ID)
			if lerr != nil {
				return nil, lerr
			}

			target, rerr := repo.TextRevision(ctx, it.ID, req.Revision)
			if rerr != nil {
				if errors.Is(rerr, ErrNotFound) {
					return nil, revisionNotFound(it.ID, req.Revision)
				}

				return nil, asError(rerr)
			}

			out.item, out.rev = *it, latest
			ch := &productionChange{rows: rows, itemID: it.ID}

			if writing.Unchanged(latest.Body, target.Body) {
				return ch, nil
			}

			now := s.nowMs()
			from := req.Revision
			next := TextRevisionRow{
				ItemID: it.ID, Revision: writing.NextRevision(latest.Revision), Body: target.Body, Actor: c.Actor,
				Source: string(writing.SourceRevert), RevertedTo: &from, CreatedMs: now,
			}

			it.Body, it.UpdatedMs = target.Body, now
			out.item, out.rev, out.changed = *it, next, true

			ch.write = func(ctx context.Context, tx Tx) error {
				return tx.Productions().AddTextRevision(ctx, next, writing.PruneBefore(next.Revision))
			}

			return ch, nil
		})
	if err != nil {
		return nil, err
	}

	res := s.writeResult(&head, &out)

	if out.changed {
		res.Text = strings.Replace(res.Text, "saved as revision", "reverted to revision "+strconv.Itoa(req.Revision)+
			" as revision", 1)
	}

	return res, nil
}

// TextRevisions lists the revisions of a text, newest first, and the difference between two when asked.
func (s *svc) TextRevisions(ctx context.Context, req TextRevisionsRequest) (*TextRevisionsResult, error) {
	c, err := authorize(ctx, "TextRevisions", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if req.Limit < 0 || req.Limit > maxRevisionPage {
		return nil, newError(command.CodeValidation, "limit is %d; it must be 1 to %d.", req.Limit, maxRevisionPage).
			withPath("limit").withHint("omit limit for 50")
	}

	if req.Before < 0 {
		return nil, newError(command.CodeValidation, "before is %d; it must be 0 or more.", req.Before).
			withPath("before")
	}

	for path, n := range map[string]int{"from": req.From, "to": req.To} {
		if n != 0 {
			if v := writing.CheckRevision(n); v != nil {
				return nil, writingViolation(v).withPath(path)
			}
		}
	}

	if req.To != 0 && req.From == 0 {
		return nil, newError(command.CodeValidation, "to needs from.").withPath("from").
			withHint("pass from and to, or from alone to compare with the latest revision")
	}

	head, it, err := s.readText(ctx, c, req.ProjectID, req.ItemID)
	if err != nil {
		return nil, err
	}

	repo := s.store.Productions()
	limit := cmpOr(req.Limit, 50)

	rows, err := repo.TextRevisions(ctx, it.ID, req.Before, limit+1)
	if err != nil {
		return nil, asError(err)
	}

	res := &TextRevisionsResult{
		UntrustedNotice: UntrustedNotice, ProjectID: head.ID, ItemID: it.ID, Revisions: []TextRevision{},
	}

	if len(rows) > limit {
		rows = rows[:limit]
		res.NextBefore = rows[len(rows)-1].Revision
	}

	var b strings.Builder

	fmt.Fprintf(&b, "%s\nrevisions of %s %s %s", textview.Note, it.ID, it.TextKind, textview.Quote(it.Title, 60))

	for i := range rows {
		res.Revisions = append(res.Revisions, revisionView(&rows[i]))
		b.WriteString("\n" + revisionLine(&rows[i]))
	}

	if res.NextBefore != 0 {
		fmt.Fprintf(&b, "\nnext_before: %d", res.NextBefore)
	}

	if req.From != 0 {
		if err := s.addDiff(ctx, repo, it.ID, &req, res, &b); err != nil {
			return nil, err
		}
	}

	res.Text = b.String()

	return res, nil
}

func cmpOr(n, def int) int {
	if n == 0 {
		return def
	}

	return n
}

func revisionLine(r *TextRevisionRow) string {
	line := fmt.Sprintf(
		"r%d %s by %s at %s",
		r.Revision,
		r.Source,
		r.Actor,
		msTime(r.CreatedMs).Format("2006-01-02T15:04:05Z"),
	)

	if r.RevertedTo != nil {
		line += " (back to r" + strconv.Itoa(*r.RevertedTo) + ")"
	}

	if r.Provider != "" {
		line += " " + r.Provider + " " + textview.Quote(r.Model, 60)
	}

	if r.CostMicros != nil {
		line += " cost " + money(*r.CostMicros, r.Currency)
	}

	return line
}

func (s *svc) addDiff(
	ctx context.Context, repo ProductionRepo, itemID string, req *TextRevisionsRequest, res *TextRevisionsResult,
	b *strings.Builder,
) error {
	to := req.To

	if to == 0 {
		latest, err := s.latestRevision(ctx, repo, itemID)
		if err != nil {
			return err
		}

		to = latest.Revision
	}

	read := func(n int) (TextRevisionRow, error) {
		rev, err := repo.TextRevision(ctx, itemID, n)
		if errors.Is(err, ErrNotFound) {
			return rev, revisionNotFound(itemID, n)
		}

		return rev, asError(err)
	}

	a, err := read(req.From)
	if err != nil {
		return err
	}

	z, err := read(to)
	if err != nil {
		return err
	}

	d := writing.Diff(a.Body, z.Body)
	res.Diff, res.DiffFrom, res.DiffTo = &d, req.From, to

	fmt.Fprintf(b, "\ndiff r%d to r%d, by paragraph:", req.From, to)

	for _, h := range d.Hunks {
		if h.Op == writing.OpEqual {
			continue
		}

		sign := "+"
		if h.Op == writing.OpRemoved {
			sign = "-"
		}

		b.WriteString("\n" + sign + " " + untrusted.Inline(untrusted.SourceProjectText,
			untrusted.Attrs{"item": itemID}, h.Text, 200))
	}

	return nil
}

// textSource finds the body a check reads: a stored text, or the text of the request.
func (s *svc) textSource(
	ctx context.Context, c Caller, projectID, itemID string, want writing.Kind,
) (string, error) {
	_, it, err := s.readText(ctx, c, projectID, itemID)
	if err != nil {
		return "", err
	}

	if it.TextKind != string(want) {
		return "", newError(command.CodeValidation, "the text %s is a %s, not a %s.", it.ID, it.TextKind, want).
			withPath("item_id").withHint("list the texts with TextList and pick a %s", want)
	}

	return it.Body, nil
}

// CaptionValidate measures a caption against the limits of its networks.
func (s *svc) CaptionValidate(ctx context.Context, req CaptionValidateRequest) (*CaptionValidateResult, error) {
	c, err := authorize(ctx, "CaptionValidate", req.ProjectID)
	if err != nil {
		return nil, err
	}

	given := 0

	for _, set := range []bool{req.ItemID != "", req.Body != "", req.Text != ""} {
		if set {
			given++
		}
	}

	if given != 1 {
		return nil, newError(command.CodeValidation, "give one of item_id, body and text.").withPath("item_id").
			withHint("item_id checks a stored caption, body a whole caption, text one variant with its network")
	}

	if req.Network != "" {
		if _, ok := writing.Lookup(req.Network); !ok {
			return nil, newError(command.CodeValidation, "%q is not a network of the table of limits.", req.Network).
				withPath("network").withHint("use one of %s", strings.Join(writing.NetworkIDs(), ", "))
		}
	}

	var variants []writing.Variant

	switch {
	case req.Text != "":
		if req.Network == "" {
			return nil, newError(command.CodeValidation, "text needs the network it is checked against.").
				withPath("network").withHint("use one of %s", strings.Join(writing.NetworkIDs(), ", "))
		}

		variants = []writing.Variant{{Network: req.Network, Text: req.Text}}
	case req.Body != "":
		variants = writing.ParseCaption(req.Body)
	default:
		body, serr := s.textSource(ctx, c, req.ProjectID, req.ItemID, writing.KindCaption)
		if serr != nil {
			return nil, serr
		}

		variants = writing.ParseCaption(body)
	}

	if req.Network != "" && req.Text == "" {
		variants = slices.DeleteFunc(variants, func(v writing.Variant) bool { return v.Network != req.Network })
		if len(variants) == 0 {
			return nil, newError(command.CodeNotFound, "the caption has no variant for %s.", req.Network).
				withPath("network").withHint("start a variant with a line ## %s", req.Network)
		}
	}

	res := &CaptionValidateResult{
		UntrustedNotice: UntrustedNotice,
		Variants:        make([]CaptionVariantReport, 0, len(variants)),
		OK:              true,
	}

	var b strings.Builder

	b.WriteString(textview.Note)

	for i := range variants {
		r := writing.ValidateCaption(variants[i].Text, variants[i].Network)
		res.Variants = append(
			res.Variants,
			CaptionVariantReport{Report: r, Words: writing.CountWords(variants[i].Text)},
		)
		res.OK = res.OK && r.OK

		b.WriteString("\n" + variantLine(&r))
	}

	res.Text = b.String()

	return res, nil
}

func variantLine(r *writing.Report) string {
	name := r.Network
	if name == "" {
		name = "common"
	}

	line := fmt.Sprintf("%s: %d", name, r.Counts.Length)
	if r.MaxLength > 0 {
		line += fmt.Sprintf("/%d", r.MaxLength)
	}

	line += fmt.Sprintf(
		" %s, %d hashtags, %d mentions, %d links",
		r.Unit,
		r.Counts.Hashtags,
		r.Counts.Mentions,
		r.Counts.Links,
	)

	if r.OK {
		return line + ", ok"
	}

	for _, p := range r.Problems {
		line += "; " + p.Code + ": " + p.Message
	}

	return line
}

// ScriptEstimate estimates how long a script takes when read aloud.
func (s *svc) ScriptEstimate(ctx context.Context, req ScriptEstimateRequest) (*ScriptEstimateResult, error) {
	c, err := authorize(ctx, "ScriptEstimate", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if (req.ItemID == "") == (req.Text == "") {
		return nil, newError(command.CodeValidation, "give one of item_id and text.").withPath("item_id").
			withHint("item_id estimates a stored script, text one that is not saved")
	}

	body := req.Text

	if req.ItemID != "" {
		if body, err = s.textSource(ctx, c, req.ProjectID, req.ItemID, writing.KindScript); err != nil {
			return nil, err
		}
	}

	est, v := writing.EstimateDuration(writing.ParseScript(body), req.WordsPerMinute)
	if v != nil {
		return nil, writingViolation(v)
	}

	res := &ScriptEstimateResult{UntrustedNotice: UntrustedNotice, Estimate: est}

	var b strings.Builder

	fmt.Fprintf(
		&b,
		"%s\nscript: %d words of narration, %s at %d words per minute, %d words on screen, %d scenes",
		textview.Note,
		est.Words,
		durationText(est.DurationMillis),
		est.WordsPerMinute,
		est.OnScreenWords,
		len(est.Scenes),
	)

	if est.Plain {
		b.WriteString(" (plain mode: no VO: marker, every paragraph is narration)")
	}

	for i := range est.Scenes {
		sc := &est.Scenes[i]
		fmt.Fprintf(
			&b,
			"\n%d. %s %d words, %s",
			i+1,
			textview.Quote(sc.Title, 60),
			sc.Words,
			durationText(sc.DurationMillis),
		)
	}

	res.Text = b.String()

	return res, nil
}

func durationText(ms int64) string {
	return fmt.Sprintf("%d.%03d s", ms/1000, ms%1000)
}

// textDetail is the part of a text line of the description of a project: its kind, its words and its revision.
func (s *svc) textDetail(ctx context.Context, r *ItemRow) string {
	detail := "text_kind=" + r.TextKind + " words=" + strconv.Itoa(writing.CountWords(r.Body))

	if revs, err := s.store.Productions().TextRevisions(ctx, r.ID, 0, 1); err == nil && len(revs) > 0 {
		detail += " rev=" + strconv.Itoa(revs[0].Revision)
	}

	return detail
}
