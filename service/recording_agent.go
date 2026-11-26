package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/capturehost"
	"github.com/AlexGalhardo/stuarty/service/recordings"
)

// The agent flow of a recording (SDD-025 section 19): an agent prepares a recording, marks takes and proposes
// cuts; only a person starts a capture and only a person decides a cut. Nothing here reaches the helper: the
// operations of this file never use s.capture.

const codeLimitExceeded = "E_LIMIT_EXCEEDED"

// RecordingPrepareRequest is the plan of a recording (SDD-025 section 19.2): what a person will record, with the
// helper not started. FPS 0 is 30.
type RecordingPrepareRequest struct {
	ProjectID  string          `json:"project_id"`
	Source     RecordingSource `json:"source"`
	FPS        int             `json:"fps,omitempty"`
	Microphone string          `json:"microphone,omitempty"`
	// SystemAudio, Camera and Events are asked for as RecordingStartRequest asks for them.
	SystemAudio bool   `json:"system_audio,omitempty"`
	Camera      string `json:"camera,omitempty"`
	Events      bool   `json:"events,omitempty"`
	// ScriptID is a written script of the project (a text item of kind script) the person will read.
	ScriptID string `json:"script_id,omitempty"`
	// Notes are for the person who presses record; at most 2,000 characters.
	Notes string `json:"notes,omitempty" untrusted:"true"`
}

// RecordingPlan is the stored plan of a prepared recording. SourceID is the display or window it records, for the
// dialog to show the title the helper gives it: the plan keeps no title of its own.
type RecordingPlan struct {
	SourceID    string `json:"source_id"`
	FPS         int    `json:"fps"`
	Microphone  string `json:"microphone,omitempty"`
	SystemAudio bool   `json:"system_audio,omitempty"`
	Camera      string `json:"camera,omitempty"`
	Events      bool   `json:"events,omitempty"`
	ScriptID    string `json:"script_id,omitempty"`
	Notes       string `json:"notes,omitempty"        untrusted:"true"`
}

// RecordingMarkRequest adds a mark to a take (SDD-025 section 19.2). EndMS 0 is a point.
type RecordingMarkRequest struct {
	ID    string `json:"recording_id"`
	Kind  string `json:"kind"`
	AtMS  int64  `json:"at_ms"`
	EndMS int64  `json:"end_ms,omitempty"`
	Note  string `json:"note,omitempty"   untrusted:"true"`
}

// RecordingMark is a mark as a surface sees it.
type RecordingMark struct {
	N         int       `json:"n"`
	Kind      string    `json:"kind"`
	AtMS      int64     `json:"at_ms"`
	EndMS     int64     `json:"end_ms,omitempty"`
	Note      string    `json:"note,omitempty"   untrusted:"true"`
	Actor     string    `json:"actor"`
	CreatedAt time.Time `json:"created_at"`
}

// RecordingCutRange is a stretch of a take to cut, in active milliseconds, with the reason given.
type RecordingCutRange struct {
	FromMS   int64  `json:"from_ms"`
	ToMS     int64  `json:"to_ms"`
	Reason   string `json:"reason"             untrusted:"true"`
	Decision string `json:"decision,omitempty"`
}

// RecordingProposeCutsRequest proposes cuts of a ready recording. It changes no timeline.
type RecordingProposeCutsRequest struct {
	ID     string              `json:"recording_id"`
	Ranges []RecordingCutRange `json:"ranges"`
}

// RecordingProposal is a proposal of cuts as a surface sees it.
type RecordingProposal struct {
	ID        string              `json:"proposal_id"`
	State     string              `json:"state"`
	Actor     string              `json:"actor"`
	CreatedAt time.Time           `json:"created_at"`
	Ranges    []RecordingCutRange `json:"ranges"`
	DecidedBy string              `json:"decided_by,omitempty"`
	DecidedAt *time.Time          `json:"decided_at,omitempty"`
	// Revision is the revision of the timeline the accepted ranges produced.
	Revision int64 `json:"revision,omitempty"`
}

// RecordingProposalDecideRequest is the decision of a person: Accept[i] is the decision on range i, one for each.
type RecordingProposalDecideRequest struct {
	ID         string `json:"recording_id"`
	ProposalID string `json:"proposal_id"`
	Accept     []bool `json:"accept"`
	TimelineID string `json:"timeline_id,omitempty"`
}

// RecordingProposalDecideResult is the recording after the decision and, when ranges were accepted, the edit that
// cut them: one batch, so one undo restores the timeline.
type RecordingProposalDecideResult struct {
	Recording Recording   `json:"recording"`
	Edit      *EditResult `json:"edit,omitempty"`
	Text      string      `json:"text"`
}

func validationErrorOf(fields []recording.FieldError) *Error {
	f := fields[0]

	return newError(command.CodeValidation, "%s is not valid (%s).", f.Field, f.Reason).withPath(f.Field)
}

func recordingLimitError(err error) (*Error, bool) {
	var le *recording.LimitError
	if !errors.As(err, &le) {
		return nil, false
	}

	return newError(codeLimitExceeded, "the limit of %d (%s) is reached.", le.Ceiling, le.Limit).
		withDetail("limit", le.Limit).withDetail("ceiling", le.Ceiling), true
}

func (s *svc) auditAs(ctx context.Context, c Caller, action string, r recordings.Row) {
	s.auditRecording(ctx, c, action, r)
}

// RecordingPrepare stores what a person will record. It validates the plan as RecordingStart does and starts
// nothing: no capturer is looked at, so no sequence of calls can reach the helper (ADR-0051, T-165).
func (s *svc) RecordingPrepare(ctx context.Context, req RecordingPrepareRequest) (*Recording, error) {
	c, err := s.authorize(ctx, "RecordingPrepare", "")
	if err != nil {
		return nil, err
	}

	spec := recording.StartSpec{
		ProjectID: req.ProjectID, SourceKind: req.Source.Kind, SourceID: req.Source.ID, FPS: req.FPS,
		Microphone: req.Microphone, SystemAudio: req.SystemAudio, Camera: req.Camera, Events: req.Events,
	}

	notes := recording.CleanNote(req.Notes, recording.MaxPlanNotesRunes)
	if fields := recording.ValidatePrepare(spec, req.ScriptID, req.Notes); len(fields) > 0 {
		return nil, validationErrorOf(fields)
	}

	if _, err := s.authorizeProject(ctx, "RecordingPrepare", req.ProjectID); err != nil {
		return nil, err
	}

	if gerr := s.recordingProject(ctx, c, req.ProjectID, false); gerr != nil {
		return nil, gerr
	}

	if req.ScriptID != "" {
		it, ierr := s.store.Productions().Item(ctx, req.ScriptID)
		if ierr != nil || it.ProjectID != req.ProjectID || it.Kind != "text" || it.TextKind != "script" ||
			it.TrashedMs != nil {
			return nil, newError(command.CodeNotFound, "the project has no script %s.", req.ScriptID).
				withPath("script_id").withHint("use the id of a script of the project from project_item")
		}
	}

	plan, jerr := json.Marshal(recording.Plan{
		SourceKind: req.Source.Kind, SourceID: req.Source.ID, FPS: recording.FPSOrDefault(req.FPS),
		Microphone: req.Microphone, SystemAudio: req.SystemAudio, Camera: req.Camera, Events: req.Events,
		ScriptID: req.ScriptID, Notes: notes,
	})
	if jerr != nil {
		return nil, internalError()
	}

	var asked []recording.Track
	for _, k := range recording.TracksAsked(spec) {
		asked = append(asked, recording.Track{Kind: k, File: k.File()})
	}

	askedJSON, jerr := json.Marshal(asked)
	if jerr != nil {
		return nil, internalError()
	}

	// The name of the source is not kept: it would be the text of the caller, shown to the person as the title of
	// what they are asked to record, and kept as the title the person saw. The dialog shows the title the helper
	// gives the source id, and a person's start checks the source is still there (SDD-025 section 19.2).
	id, now := s.newIDs("rec"), s.nowMs()
	row := recordings.Row{
		ID: id, ProjectID: req.ProjectID, State: string(recording.Prepared), SourceKind: req.Source.Kind,
		FPS: recording.FPSOrDefault(req.FPS), Actor: c.Actor, CreatedMs: now, Tracks: string(askedJSON),
		Plan: string(plan),
	}

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		rows, err := tx.Recordings().ListByProject(ctx, req.ProjectID)
		if err != nil {
			return err
		}

		n := 0

		for i := range rows {
			if recording.IsPrepared(recording.State(rows[i].State), rows[i].Plan) {
				n++
			}
		}

		if n >= recording.MaxPreparedPerProject {
			return newError(codeLimitExceeded, "the project already has %d prepared recordings.", n).
				withDetail("limit", recording.LimitPrepared).withDetail("ceiling", recording.MaxPreparedPerProject).
				withHint("cancel one with recording:cancel, or ask a person to record one")
		}

		if err := tx.Recordings().Insert(ctx, row); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "recording.prepare", req.ProjectID, id)
	})
	if werr != nil {
		return nil, asError(werr)
	}

	return s.liveView(row), nil
}

// planOf reads the stored plan of a row.
func planOf(r recordings.Row) (recording.Plan, bool) {
	var p recording.Plan

	if r.Plan == "" || json.Unmarshal([]byte(r.Plan), &p) != nil {
		return p, false
	}

	return p, true
}

func decodeMarks(s string) []recording.Mark {
	var out []recording.Mark
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}

	return out
}

func decodeProposals(s string) []recording.Proposal {
	var out []recording.Proposal
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}

	return out
}

// RecordingMark adds a mark to a take: a point or a stretch tagged good, bad or mistake. It writes only the row of
// the recording.
func (s *svc) RecordingMark(ctx context.Context, req RecordingMarkRequest) (*Recording, error) {
	c, err := s.authorize(ctx, "RecordingMark", "")
	if err != nil {
		return nil, err
	}

	r, err := s.recordingFor(ctx, c, req.ID)
	if err != nil {
		return nil, err
	}

	if _, err := s.authorizeProject(ctx, "RecordingMark", r.ProjectID); err != nil {
		return nil, err
	}

	note := recording.CleanLabel(req.Note, recording.MaxNoteRunes)

	got, uerr := s.updateRecording(ctx, req.ID, func(x *recordings.Row) error {
		marks := decodeMarks(x.Marks)

		fields, verr := recording.ValidateMark(
			recording.MarkKind(req.Kind), req.AtMS, req.EndMS, recording.State(x.State), x.DurationMs, len(marks))

		switch {
		case verr != nil:
			if le, ok := recordingLimitError(verr); ok {
				return le
			}

			return newError(command.CodeInvalidState, "the recording is %s and takes no mark yet.", x.State).
				withHint("mark a recording that is running, or one that is ready")
		case len(fields) > 0:
			return validationErrorOf(fields)
		}

		marks = append(marks, recording.Mark{
			N: len(marks) + 1, Kind: recording.MarkKind(req.Kind), AtMS: req.AtMS, EndMS: req.EndMS, Note: note,
			Actor: c.Actor, CreatedMs: s.nowMs(),
		})

		b, merr := json.Marshal(marks)
		if merr != nil {
			return merr
		}

		x.Marks = string(b)

		return nil
	})
	if uerr != nil {
		return nil, asError(uerr)
	}

	s.auditAs(ctx, c, "recording.mark", got)

	return s.liveView(got), nil
}

// RecordingProposeCuts adds a proposal of cuts to a ready recording. It writes only the row of the recording: no
// document is read for writing and no revision moves (SDD-025 section 19.2, T-166).
func (s *svc) RecordingProposeCuts(ctx context.Context, req RecordingProposeCutsRequest) (*Recording, error) {
	c, err := s.authorize(ctx, "RecordingProposeCuts", "")
	if err != nil {
		return nil, err
	}

	r, err := s.recordingFor(ctx, c, req.ID)
	if err != nil {
		return nil, err
	}

	if _, err := s.authorizeProject(ctx, "RecordingProposeCuts", r.ProjectID); err != nil {
		return nil, err
	}

	ranges := make([]recording.CutRange, len(req.Ranges))
	for i, x := range req.Ranges {
		ranges[i] = recording.CutRange{
			FromMS: x.FromMS, ToMS: x.ToMS, Reason: x.Reason,
		}
	}

	got, uerr := s.updateRecording(ctx, req.ID, func(x *recordings.Row) error {
		have := decodeProposals(x.Proposals)

		if cerr := recording.CheckNewProposal(have, recording.State(x.State)); cerr != nil {
			if le, ok := recordingLimitError(cerr); ok {
				return le.withHint("a person decides the pending proposals first")
			}

			return newError(command.CodeInvalidState, "the recording is %s and takes no proposal yet.", x.State).
				withHint("propose cuts of a recording that is ready")
		}

		lo, hi := recording.SharedSpan(decodeTracks(x.Tracks), x.DurationMs)
		if fields := recording.ValidateRanges(ranges, lo, hi); len(fields) > 0 {
			return validationErrorOf(fields).
				withHint("send sorted ranges that do not overlap, each at least %d ms, inside %d to %d ms, with a reason",
					recording.MinRangeMS, lo, hi)
		}

		for i := range ranges {
			ranges[i].Reason = recording.CleanLabel(ranges[i].Reason, recording.MaxNoteRunes)
		}

		have = append(have, recording.Proposal{
			ID: s.newIDs("prp"), Actor: c.Actor, CreatedMs: s.nowMs(), State: recording.ProposalPending, Ranges: ranges,
		})

		b, merr := json.Marshal(have)
		if merr != nil {
			return merr
		}

		x.Proposals = string(b)

		return nil
	})
	if uerr != nil {
		return nil, asError(uerr)
	}

	s.auditAs(ctx, c, "recording.propose", got)

	return s.liveView(got), nil
}

// RecordingProposalDecide is the decision of a person on a proposal: one yes or no for every range. The ranges
// accepted become one batch of item.split and item.delete on the timeline where the recording was placed, so one
// undo restores it. The check of a person is the first thing done (T-166).
func (s *svc) RecordingProposalDecide(
	ctx context.Context, req RecordingProposalDecideRequest,
) (*RecordingProposalDecideResult, error) {
	c, err := s.person(ctx, "RecordingProposalDecide")
	if err != nil {
		return nil, err
	}

	s.decideMu.Lock()
	defer s.decideMu.Unlock()

	r, err := s.recordingFor(ctx, c, req.ID)
	if err != nil {
		return nil, err
	}

	if _, err := s.authorizeProject(ctx, "RecordingProposalDecide", r.ProjectID); err != nil {
		return nil, err
	}

	var prop *recording.Proposal

	proposals := decodeProposals(r.Proposals)
	if i := slices.IndexFunc(proposals, func(p recording.Proposal) bool { return p.ID == req.ProposalID }); i >= 0 {
		prop = &proposals[i]
	}

	if prop == nil {
		return nil, newError(command.CodeNotFound, "the recording has no proposal %s.", req.ProposalID).
			withPath("proposal_id").withHint("read the proposals of the recording with recording:show")
	}

	if prop.State != recording.ProposalPending {
		return nil, newError(command.CodeInvalidState, "the proposal is already %s.", prop.State)
	}

	if len(req.Accept) != len(prop.Ranges) {
		return nil, newError(command.CodeValidation, "send one decision for each of the %d ranges, not %d.",
			len(prop.Ranges), len(req.Accept)).withPath("accept")
	}

	decided, accepted, derr := recording.Decide(*prop, req.Accept, c.Actor, s.nowMs())
	if derr != nil {
		return nil, newError(command.CodeValidation, "the decisions do not fit the proposal.").withPath("accept")
	}

	var edit *EditResult

	if len(accepted) > 0 {
		if edit, err = s.applyCuts(ctx, r, req.TimelineID, accepted); err != nil {
			return nil, err
		}

		decided.Revision = edit.Revision
	}

	got, uerr := s.updateRecording(ctx, req.ID, func(x *recordings.Row) error {
		list := decodeProposals(x.Proposals)

		i := slices.IndexFunc(list, func(p recording.Proposal) bool { return p.ID == req.ProposalID })
		if i < 0 || list[i].State != recording.ProposalPending {
			return newError(command.CodeInvalidState, "the proposal is no longer pending.")
		}

		list[i] = decided

		b, merr := json.Marshal(list)
		if merr != nil {
			return merr
		}

		x.Proposals = string(b)

		return nil
	})
	if uerr != nil {
		return nil, asError(uerr)
	}

	s.auditAs(ctx, c, "recording.decide", got)

	out := &RecordingProposalDecideResult{Recording: *s.liveView(got), Edit: edit}

	out.Text = "No range was accepted: nothing was changed."
	if edit != nil {
		out.Text = fmt.Sprintf("%d of %d ranges were cut in one batch (revision %d); one undo restores them.",
			len(accepted), len(decided.Ranges), edit.Revision)
	}

	return out, nil
}

// applyCuts builds the batch of the accepted ranges (SDD-025 section 19.5) and sends it through Edit with the
// revision just read, so an edit made meanwhile is a conflict and nothing is applied.
func (s *svc) applyCuts(
	ctx context.Context, r recordings.Row, timelineID string, accepted []recording.CutRange,
) (*EditResult, error) {
	if !recording.Placeable(recording.State(r.State), recording.Review(r.Review)) {
		return nil, newError(command.CodeInvalidState, "the recording is not in the project yet.").
			withHint("a person adds the recording to the project and places it on a timeline first")
	}

	pg, err := s.ProjectGet(ctx, ProjectGetRequest{ProjectID: r.ProjectID, TimelineID: timelineID, Detail: "document"})
	if err != nil {
		return nil, err
	}

	if pg.Document == nil {
		return nil, internalError()
	}

	doc := pg.Document

	rtracks := decodeTracks(r.Tracks)
	if len(rtracks) == 0 {
		rtracks = []recording.Track{{Kind: recording.TrackScreen, AssetHash: r.AssetHash}}
	}

	kindOf := map[string]recording.TrackKind{}

	for _, t := range rtracks {
		hash := t.AssetHash
		if hash == "" && t.Kind == recording.TrackScreen {
			hash = r.AssetHash
		}

		for id := range doc.Assets {
			if hash != "" && doc.Assets[id].Hash == hash {
				kindOf[id] = t.Kind
			}
		}
	}

	var clips []recording.Clip

	for itemID := range doc.Items {
		it := doc.Items[itemID]
		kind, ok := kindOf[it.Asset]
		if !ok || (it.Type != timeline.ItemClip && it.Type != timeline.ItemAudio) {
			continue
		}

		slow := it.Freeze || it.Reverse || len(it.SpeedCurve) > 0 || (it.Speed.Num != 0 && it.Speed.Num != it.Speed.Den)
		clips = append(clips, recording.Clip{
			ID: it.ID, Track: it.Track, Kind: kind, Start: it.Start, Duration: it.Duration, SourceIn: it.SourceIn,
			Slow: slow,
		})
	}

	if len(clips) == 0 {
		return nil, newError(command.CodeInvalidState, "the recording is not on this timeline.").
			withHint("place the recording on the timeline first with recording:place")
	}

	// A stable order, so the same proposal makes the same batch.
	slices.SortFunc(clips, func(a, b recording.Clip) int { return strings.Compare(a.ID, b.ID) })

	steps, perr := recording.PlanCuts(
		accepted, clips, rtracks, max(doc.Settings.FPS.Num, 1), max(doc.Settings.FPS.Den, 1),
		func() string { return s.newIDs("itm") })

	switch {
	case errors.Is(perr, recording.ErrSlowClip):
		return nil, newError(command.CodeInvalidState, "a clip of the recording plays at another speed.").
			withHint("cut that clip by hand: its frames are not those of the take")
	case errors.Is(perr, recording.ErrNothingToCut):
		return nil, newError(command.CodeInvalidState, "no accepted range falls on a clip of this timeline.").
			withHint("the clips may have been trimmed away already")
	case perr != nil:
		return nil, internalError()
	}

	ops := make([]command.Operation, 0, len(steps))

	for _, st := range steps {
		var args []byte

		var merr error

		switch st.Op {
		case recording.StepSplit:
			args, merr = json.Marshal(struct {
				ItemID string `json:"item_id"`
				At     string `json:"at"`
				NewID  string `json:"new_id"`
			}{st.ItemID, strconv.Itoa(st.At) + "f", st.NewID})
		case recording.StepDelete:
			args, merr = json.Marshal(struct {
				ItemIDs []string `json:"item_ids"`
				Ripple  string   `json:"ripple"`
			}{[]string{st.ItemID}, "track"})
		}

		if merr != nil {
			return nil, internalError()
		}

		ops = append(ops, command.Operation{Op: string(st.Op), Args: args})
	}

	// The steps are frames of the document of pg.Revision: an edit made since, even by the same person, could move
	// them onto other frames, so it is a conflict and not merged (SDD-025 section 19.5).
	return s.Edit(ctx, EditRequest{
		ProjectID: r.ProjectID, TimelineID: timelineID, BaseRevision: pg.Revision, Operations: ops, exactBase: true,
	})
}

// recordingAgentView fills what the agent flow adds to the view of a row: the plan, the marks and the proposals.
func recordingAgentView(v *Recording, r recordings.Row) {
	v.Marks, v.Proposals = []RecordingMark{}, []RecordingProposal{}

	if p, ok := planOf(r); ok && recording.IsPrepared(recording.State(r.State), r.Plan) {
		v.Plan = &RecordingPlan{
			SourceID:    p.SourceID,
			FPS:         p.FPS,
			Microphone:  p.Microphone,
			SystemAudio: p.SystemAudio,
			Camera:      p.Camera,
			Events:      p.Events,
			ScriptID:    p.ScriptID,
			Notes:       p.Notes,
		}
	}

	for _, m := range decodeMarks(r.Marks) {
		v.Marks = append(v.Marks, RecordingMark{
			N: m.N, Kind: string(m.Kind), AtMS: m.AtMS, EndMS: m.EndMS, Note: m.Note, Actor: m.Actor,
			CreatedAt: time.UnixMilli(m.CreatedMs).UTC(),
		})
	}

	for _, p := range decodeProposals(r.Proposals) {
		out := RecordingProposal{
			ID: p.ID, State: string(p.State), Actor: p.Actor, CreatedAt: time.UnixMilli(p.CreatedMs).UTC(),
			DecidedBy: p.DecidedBy, Revision: p.Revision, Ranges: make([]RecordingCutRange, 0, len(p.Ranges)),
		}

		if p.DecidedMs > 0 {
			t := time.UnixMilli(p.DecidedMs).UTC()
			out.DecidedAt = &t
		}

		for _, x := range p.Ranges {
			out.Ranges = append(out.Ranges, RecordingCutRange{
				FromMS: x.FromMS, ToMS: x.ToMS, Reason: x.Reason, Decision: string(x.Decision),
			})
		}

		v.Proposals = append(v.Proposals, out)
	}
}

// agentText is what the text of a recording says about its plan, marks and pending proposals, so an agent that
// reads it with job_get sees them without another call (SDD-025 section 19.2).
func agentText(v *Recording) string {
	var parts []string

	if v.Plan != nil {
		parts = append(parts, "It is prepared: a person presses record.")
	}

	if n := len(v.Marks); n > 0 {
		parts = append(parts, strconv.Itoa(n)+" marks.")
	}

	for i := range v.Proposals {
		p := &v.Proposals[i]
		if p.State == string(recording.ProposalPending) {
			parts = append(parts, "Proposal "+p.ID+" of "+strconv.Itoa(len(p.Ranges))+
				" cuts waits for a person to decide.")
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return " " + strings.Join(parts, " ")
}

// preparedFor loads the recording a person starts by id and fills the request from its plan. Every other member
// of the request must be empty: the plan is the request, and two sources of one would be ambiguous.
func (s *svc) preparedFor(ctx context.Context, c Caller, req *RecordingStartRequest) (*recordings.Row, error) {
	if req.ProjectID != "" || req.Source != (RecordingSource{}) || req.FPS != 0 || req.Microphone != "" ||
		req.SystemAudio || req.Camera != "" || req.Events {
		return nil, newError(command.CodeValidation, "a recording that was prepared is started by its id alone.").
			withPath("recording_id").withHint("send only recording_id, or the project and the source without it")
	}

	r, err := s.recordingFor(ctx, c, req.RecordingID)
	if err != nil {
		return nil, err
	}

	p, ok := planOf(r)
	if !ok || !recording.IsPrepared(recording.State(r.State), r.Plan) {
		return nil, newError(command.CodeInvalidState, "the recording is not prepared: it is %s.", r.State).
			withHint("a recording is prepared with recording:prepare")
	}

	// The person records what the helper lists under the id of the plan now, or nothing: an id that is gone is not
	// guessed at.
	if err := s.planSourceListed(ctx, p); err != nil {
		return nil, err
	}

	req.ProjectID = r.ProjectID
	req.Source = RecordingSource{Kind: p.SourceKind, ID: p.SourceID, Name: r.SourceLabel}
	req.FPS, req.Microphone, req.SystemAudio, req.Camera, req.Events = p.FPS, p.Microphone, p.SystemAudio, p.Camera,
		p.Events

	return &r, nil
}

// planSourceListed checks that the display or window of a plan is one the helper lists now.
func (s *svc) planSourceListed(ctx context.Context, p recording.Plan) error {
	src, err := s.capture.Sources(ctx)
	if err != nil {
		return captureError(err)
	}

	list := src.Displays
	if p.SourceKind == recording.SourceWindow {
		list = src.Windows
	}

	if !slices.ContainsFunc(list, func(x capturehost.Source) bool { return x.ID == p.SourceID }) {
		return newError(command.CodeInvalidState, "the %s of the plan is not there any more.", p.SourceKind).
			withPath("recording_id").
			withHint("pick what to record in the dialog, or prepare the recording again")
	}

	return nil
}

// claimOrInsert makes the row of a recording that is starting: a new one, or the prepared one, whose plan leaves
// the row in the same transaction. From then on it is a recording a helper is starting, and a second start of the
// same plan finds nothing prepared.
func claimOrInsert(ctx context.Context, tx Tx, prepared *recordings.Row, row recordings.Row) error {
	if prepared == nil {
		return tx.Recordings().Insert(ctx, row)
	}

	cur, err := tx.Recordings().Find(ctx, prepared.ID)
	if err != nil || !recording.IsPrepared(recording.State(cur.State), cur.Plan) {
		return newError(command.CodeInvalidState, "the recording is not prepared any more.")
	}

	cur.Plan = ""

	return tx.Recordings().Update(ctx, cur)
}
