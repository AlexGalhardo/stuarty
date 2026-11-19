package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
	"github.com/AlexGalhardo/stuarty/service/textview"
)

const (
	defaultProjectLimit = 20
	maxProjectLimit     = 100
	maxProjectName      = 120
)

// presets are the frame sizes of RF-002.
var presets = map[string][2]int{
	"9:16": {1080, 1920}, "1:1": {1080, 1080}, "16:9": {1920, 1080}, "4:5": {1080, 1350},
}

var fpsPattern = regexp.MustCompile(`^(\d{1,6})(?:/(1|1001))?$`)

// parseFPS reads "30", "24" or "30000/1001" (SDD-001 section 3.1: the
// denominator is 1 or 1001).
func parseFPS(s string) (timeline.Rational, bool) {
	m := fpsPattern.FindStringSubmatch(s)
	if m == nil {
		return timeline.Rational{}, false
	}

	num, _ := strconv.Atoi(m[1])
	den := 1

	if m[2] != "" {
		den, _ = strconv.Atoi(m[2])
	}

	f := timeval.FPS{Num: int64(num), Den: int64(den)}
	if !f.Valid() || num > 120*den {
		return timeline.Rational{}, false
	}

	return timeline.Rational{Num: num, Den: den}, true
}

// visible reports whether the caller may see a project: in network mode only
// the owner (SDD-007 section 4.1); in local mode every project.
func visible(c Caller, owner *string) bool {
	return c.OwnerUserID == "" || (owner != nil && *owner == c.OwnerUserID)
}

func (s *svc) ProjectCreate(ctx context.Context, req ProjectCreateRequest) (*ProjectCreateResult, error) {
	c, err := authorize(ctx, "ProjectCreate", "")
	if err != nil {
		return nil, err
	}

	// A key limited to one project cannot create another (SDD-007 section 12, item 9).
	if c.ProjectID != "" {
		return nil, newError(CodeForbidden, "the key is limited to one project and cannot create projects.").
			withHint("use a key without a project limit")
	}

	// In network mode the server picks the identifier: a client-chosen one
	// would answer E_ID_EXISTS for another user's project (an existence oracle).
	if c.OwnerUserID != "" && req.ID != "" {
		return nil, newError(command.CodeValidation, "the id of a project is chosen by the server.").
			withPath("id").withHint("leave id out")
	}

	doc, verr := s.newDocument(req)
	if verr != nil {
		return nil, verr
	}

	res := &ProjectCreateResult{
		UntrustedNotice: UntrustedNotice,
		ProjectID:       doc.ID, Name: doc.Name, Width: doc.Settings.Width, Height: doc.Settings.Height,
		FPS: fpsString(doc.Settings.FPS.Num, doc.Settings.FPS.Den),
		Text: textview.Created(doc.ID, doc.Name, doc.Settings.Width, doc.Settings.Height,
			timeval.FPS{Num: int64(doc.Settings.FPS.Num), Den: int64(doc.Settings.FPS.Den)}),
	}

	if req.DryRun {
		return res, nil
	}

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

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.Projects().Create(ctx, row); err != nil {
			return err
		}

		return tx.Audit().Append(ctx, AuditRow{AtMs: now, Actor: c.Actor, Action: "project.create", ProjectID: &doc.ID})
	})
	if err != nil {
		if errors.Is(err, ErrExists) {
			return nil, newError(command.CodeIDExists, "a project with the identifier %s already exists.", doc.ID).
				withHint("choose another id or leave it out").withPath("id")
		}

		return nil, asError(err)
	}

	res.Created = true

	return res, nil
}

// newDocument builds the empty document of a new project.
func (s *svc) newDocument(req ProjectCreateRequest) (*timeline.Document, *Error) {
	if n := utf8.RuneCountInString(req.Name); n < 1 || n > maxProjectName {
		return nil, newError(
			command.CodeValidation,
			"the name has %d characters; it must have 1 to %d.",
			n,
			maxProjectName,
		).
			withPath("name").
			withHint("send a name of 1 to %d characters", maxProjectName)
	}

	if msg := projectNameProblem(req.Name); msg != "" {
		return nil, newError(command.CodeValidation, "the name %s.", msg).
			withPath("name").withHint("send a name with visible text and no control or direction characters")
	}

	preset := req.Preset
	if preset == "" {
		preset = "9:16"
	}

	size, ok := presets[preset]
	if !ok {
		return nil, newError(command.CodeValidation, "the preset %q is not known.", preset).
			withPath("preset").withHint("use 9:16, 1:1, 16:9 or 4:5")
	}

	fps := timeline.Rational{Num: 30, Den: 1}

	if req.FPS != "" {
		if fps, ok = parseFPS(req.FPS); !ok {
			return nil, newError(command.CodeValidation, "the frame rate %q is not valid.", req.FPS).
				withPath("fps").withHint("use a whole number such as 30, or <n>/1001 such as 30000/1001")
		}
	}

	id := req.ID
	if id == "" {
		id = s.newIDs(timeline.PrefixProject)
	} else if !timeline.ValidID(timeline.PrefixProject, id) {
		return nil, newError(command.CodeValidation, "the id %q is not a valid project identifier.", id).
			withPath("id").withHint("use prj_ followed by 10 characters of 0-9 a-z without i, l, o, u")
	}

	doc := &timeline.Document{
		SchemaVersion: timeline.CurrentSchemaVersion, ID: id, Name: req.Name,
		Settings: timeline.DefaultSettings(),
		Assets:   map[string]timeline.Asset{}, Tracks: map[string]timeline.Track{}, Items: map[string]timeline.Item{},
	}
	doc.Settings.Width, doc.Settings.Height, doc.Settings.FPS = size[0], size[1], fps

	if vs := timeline.Validate(doc); len(vs) > 0 {
		return nil, newError(
			command.CodeValidation,
			"the project is not valid: %s.",
			vs[0].Message,
		).withPath(vs[0].Path)
	}

	return doc, nil
}

func (s *svc) ProjectList(ctx context.Context, req ProjectListRequest) (*ProjectListResult, error) {
	c, err := authorize(ctx, "ProjectList", "")
	if err != nil {
		return nil, err
	}

	if err := checkLimit(req.Limit); err != nil {
		return nil, err
	}

	q := ProjectQuery{
		After: req.Cursor, Limit: clampLimit(req.Limit, defaultProjectLimit, maxProjectLimit),
		OwnerUserID: c.OwnerUserID, ProjectID: c.ProjectID,
	}

	switch req.State {
	case "", "active":
		req.State = "active"
	case "trashed":
		q.State = ProjectTrashed
	case "all":
		q.IncludeTrashed = true
	default:
		return nil, newError(command.CodeValidation, "the state %q is not known.", req.State).
			withPath("state").withHint("use active, trashed or all")
	}

	heads, next, err := s.store.Projects().List(ctx, q)
	if err != nil {
		return nil, listError(err)
	}

	lines := func(hs []ProjectHead) []textview.ProjectLine {
		out := make([]textview.ProjectLine, 0, len(hs))

		for i := range hs {
			h := &hs[i]
			out = append(out, textview.ProjectLine{
				ID:       h.ID,
				Name:     h.Name,
				Revision: h.Revision,
				Width:    h.Summary.Width,
				Height:   h.Summary.Height,
				FPS: timeval.FPS{
					Num: int64(h.Summary.FPSNum),
					Den: int64(h.Summary.FPSDen),
				},
				DurationFrame: h.Summary.DurationFrames,
				Items:         h.Summary.Items,
				Trashed:       h.State == ProjectTrashed,
			})
		}

		return out
	}

	// The text has a character cap: when the page does not fit, ask for a
	// shorter one so that the store hands back the matching cursor.
	if n := textview.FitProjectList(lines(heads)); n < len(heads) {
		q.Limit = n
		if heads, next, err = s.store.Projects().List(ctx, q); err != nil {
			return nil, listError(err)
		}
	}

	total, err := s.store.Projects().Count(ctx, ProjectQuery{
		State: q.State, IncludeTrashed: q.IncludeTrashed, OwnerUserID: q.OwnerUserID, ProjectID: q.ProjectID,
	})
	if err != nil {
		return nil, asError(err)
	}

	res := &ProjectListResult{UntrustedNotice: UntrustedNotice, Projects: []ProjectSummary{}}

	for i := range heads {
		res.Projects = append(res.Projects, headSummary(&heads[i]))
	}

	res.Text = textview.ProjectList(lines(heads), total, req.State)

	if next != "" {
		res.NextCursor = &next
		res.Text += "\nnext_cursor: " + next
	}

	return res, nil
}

// checkLimit refuses a limit below zero; zero is the default of the operation. An explicit limit=0 is
// refused by the adapters, which can tell it from an absent one (VV-104).
func checkLimit(n int) error {
	if n < 0 {
		return newError(command.CodeValidation, "limit is %d; it must be at least 1.", n).
			withPath("limit").withHint("omit limit for the default page size")
	}

	return nil
}

func clampLimit(n, def, maxN int) int {
	if n <= 0 {
		return def
	}

	return min(n, maxN)
}

func listError(err error) error {
	if errors.Is(err, ErrBadCursor) {
		return newError(
			command.CodeValidation,
			"the cursor is not valid.",
		).withPath("cursor").
			withHint("repeat the call without cursor")
	}

	return asError(err)
}

// cursor of a view of the document (SDD-003 section 1.5 rule 1).
type viewCursor struct {
	V int    `json:"v"`
	P string `json:"p"`
	R int64  `json:"r"`
	K string `json:"k"`
	N int    `json:"n"`
}

func encodeViewCursor(c viewCursor) string {
	b := marshalOrNull(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeViewCursor(s, project string, revision int64) (viewCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)

	var c viewCursor

	if err != nil || json.Unmarshal(raw, &c) != nil || c.V != 1 || c.P != project || c.N < 2 {
		return c, newError(command.CodeValidation, "the cursor is not valid.").withPath("cursor").
			withHint("repeat the call without cursor")
	}

	if c.R != revision {
		return c, newError(
			command.CodeValidation,
			"the project changed since the cursor was issued.",
		).withPath("cursor").
			withDetail("reason", "stale").
			withDetail("revision", revision).
			withHint("repeat the call without cursor")
	}

	return c, nil
}

func (s *svc) ProjectGet(ctx context.Context, req ProjectGetRequest) (*ProjectGetResult, error) {
	c, err := authorize(ctx, "ProjectGet", req.ProjectID)
	if err != nil {
		return nil, err
	}

	detail := req.Detail
	if detail == "" {
		detail = "concise"
	}

	if detail != "concise" && detail != "full" && detail != "document" {
		return nil, newError(command.CodeValidation, "the detail %q is not known.", detail).
			withPath("detail").withHint("use concise, full or document")
	}

	if detail == "document" && (req.Cursor != "" || req.Limit != 0) {
		return nil, newError(command.CodeValidation, "cursor and limit are not accepted with detail document.").
			withPath("cursor").withHint("leave them out")
	}

	row, err := s.store.Projects().Get(ctx, req.ProjectID)
	if err != nil || !visible(c, row.OwnerUserID) {
		return nil, projectNotFound(req.ProjectID, err)
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	end := endFrame(doc)
	words := 0

	for id := range doc.Items {
		words += len(doc.Items[id].Words)
	}

	res := &ProjectGetResult{
		UntrustedNotice: UntrustedNotice,
		ProjectID:       row.ID,
		Name:            row.Name,
		Revision:        row.Revision,
		State:           stateString(row.State),
		Width:           doc.Settings.Width,
		Height:          doc.Settings.Height,
		FPS:             fpsString(doc.Settings.FPS.Num, doc.Settings.FPS.Den),
		DurationFrames:  end,
		Duration:        timeValue(doc.Settings.FPS, end),
		Counts: ProjectCounts{
			Tracks:       len(doc.Tracks),
			Items:        len(doc.Items),
			Assets:       len(doc.Assets),
			CaptionWords: words,
		},
	}

	if detail == "document" {
		res.Document = doc
		return res, nil
	}

	opts := textview.ProjectOptions{Full: detail == "full", Revision: row.Revision, Page: 1, Limit: req.Limit}

	if req.Cursor != "" {
		cur, err := decodeViewCursor(req.Cursor, row.ID, row.Revision)
		if err != nil {
			return nil, err
		}

		opts.After, opts.Page = cur.K, cur.N
	}

	text, next := textview.Project(doc, opts)
	res.Text = text

	if next != "" {
		cur := encodeViewCursor(viewCursor{V: 1, P: row.ID, R: row.Revision, K: next, N: opts.Page + 1})
		res.NextCursor = &cur
		res.Text += "\nnext_cursor: " + cur
	}

	return res, nil
}

// projectNotFound is E_NOT_FOUND for a missing project and for one the caller
// may not see, so that a project's existence is not disclosed.
func projectNotFound(id string, err error) error {
	if err == nil || errors.Is(err, ErrNotFound) {
		return newError(command.CodeNotFound, "there is no project %s.", id).
			withHint("list the projects with project_list")
	}

	return asError(err)
}

func (s *svc) ProjectTrash(ctx context.Context, req ProjectTrashRequest) (*ProjectStateResult, error) {
	return s.setProjectState(ctx, "ProjectTrash", req, ProjectTrashed, "project.trash")
}

func (s *svc) ProjectRestore(ctx context.Context, req ProjectTrashRequest) (*ProjectStateResult, error) {
	return s.setProjectState(ctx, "ProjectRestore", req, ProjectActive, "project.restore")
}

// setProjectState trashes or restores. Both are idempotent: a project already
// in the state is reported and left alone. Trash touches the state and the
// time only, never the revision or the document (SDD-005 section 13.1).
func (s *svc) setProjectState(
	ctx context.Context,
	method string,
	req ProjectTrashRequest,
	to ProjectState,
	action string,
) (*ProjectStateResult, error) {
	c, err := authorize(ctx, method, req.ProjectID)
	if err != nil {
		return nil, err
	}

	res := &ProjectStateResult{ProjectID: req.ProjectID, State: string(to)}
	verb := map[ProjectState]string{ProjectTrashed: "trashed", ProjectActive: "restored"}[to]

	err = s.store.WriteProject(ctx, req.ProjectID, func(ctx context.Context, tx ProjectTx) error {
		row := tx.Project()
		if !visible(c, row.OwnerUserID) {
			return ErrNotFound
		}

		if stateString(row.State) == string(to) || req.DryRun {
			res.Text = req.ProjectID + " " + string(to)
			return nil
		}

		now := s.nowMs()
		if err := tx.Projects().SetState(ctx, req.ProjectID, to, c.Actor, now); err != nil {
			return err
		}

		id := req.ProjectID
		if err := appendAudit(ctx, tx, now, c.Actor, action, id, ""); err != nil {
			return err
		}

		res.Text = req.ProjectID + " " + verb

		ev := Event{
			Type:      "project.updated",
			ProjectID: id,
			Revision:  row.Revision,
			Actor:     c.Actor,
			State:     string(to),
			At:        msTime(now),
		}
		tx.AfterCommit(func() { s.emit(ev) })

		return nil
	})
	if err != nil {
		return nil, projectNotFound(req.ProjectID, err)
	}

	switch {
	case req.DryRun:
	case to == ProjectTrashed:
		s.cancelProjectJobs(ctx, req.ProjectID)
	default:
		s.reconcilePending(ctx, req.ProjectID)
	}

	return res, nil
}

// projectNameProblem is why a name that has a valid length is still refused
// (VV-049): the same rule for the desktop and the network mode.
func projectNameProblem(name string) string {
	visible := false

	for _, r := range name {
		switch {
		case unicode.IsControl(r):
			return "has a control character"
		case isBidiControl(r):
			return "has a text direction character"
		case !unicode.IsSpace(r) && !unicode.Is(unicode.Cf, r):
			visible = true
		}
	}

	if !visible {
		return "has no visible character"
	}

	return ""
}

// isBidiControl reports the characters that reorder the text around them.
func isBidiControl(r rune) bool {
	return r == 0x061C || r == 0x200E || r == 0x200F ||
		(r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}
