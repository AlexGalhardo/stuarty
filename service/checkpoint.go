package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service/textview"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

const (
	maxCheckpointName  = 60
	maxCheckpointNote  = 300
	maxAutoCheckpoints = 30
	autoNamePrefix     = "auto "
	maxAutoActorRunes  = 100
)

// CheckpointCreate puts a name and a note on a revision (SDD-003 section 2.5.1).
func (s *svc) CheckpointCreate(ctx context.Context, req CheckpointCreateRequest) (*Checkpoint, error) {
	c, head, err := s.checkpointScope(ctx, "CheckpointCreate", &req.ProjectID, req.TimelineID, true)
	if err != nil {
		return nil, err
	}

	name, nerr := cleanCheckpointName(req.Name, "name")
	if nerr != nil {
		return nil, nerr
	}

	rev := req.Revision
	if rev == 0 {
		rev = head.Revision
	}

	if rev < 1 || rev > head.Revision {
		return nil, newError(command.CodeValidation, "revision %d does not exist; the timeline is at revision %d.",
			rev, head.Revision).withPath("revision").withHint("use a revision from 1 to %d", head.Revision)
	}

	row := CheckpointRow{
		ProjectID: req.ProjectID, Name: name, Note: cleanCheckpointNote(req.Note), Revision: rev, Actor: c.Actor,
		AtMs: s.nowMs(),
	}

	if err := s.store.Checkpoints().Create(ctx, row); err != nil {
		if errors.Is(err, ErrExists) {
			return nil, newError(command.CodeIDExists, "the timeline already has a checkpoint named %q.", name).
				withPath("name").withHint("choose another name, or rename or remove the existing one")
		}

		return nil, asError(err)
	}

	return checkpointOf(row), nil
}

// CheckpointList lists the checkpoints of a timeline, newest revision first.
func (s *svc) CheckpointList(ctx context.Context, req CheckpointListRequest) (*CheckpointListResult, error) {
	_, head, err := s.checkpointScope(ctx, "CheckpointList", &req.ProjectID, req.TimelineID, false)
	if err != nil {
		return nil, err
	}

	rows, err := s.store.Checkpoints().List(ctx, req.ProjectID)
	if err != nil {
		return nil, asError(err)
	}

	res := &CheckpointListResult{
		UntrustedNotice: UntrustedNotice, ProjectID: req.ProjectID, Revision: head.Revision,
		Checkpoints: make([]Checkpoint, 0, len(rows)),
	}

	lines := make([]textview.CheckpointLine, 0, len(rows))

	for i := range rows {
		cp := checkpointOf(rows[i])
		cp.UntrustedNotice = ""
		res.Checkpoints = append(res.Checkpoints, *cp)
		lines = append(lines, textview.CheckpointLine{
			Revision: cp.Revision, Name: cp.Name, Note: cp.Note, Actor: cp.Actor, At: cp.At, Auto: cp.Auto,
		})
	}

	fit := textview.FitCheckpoints(lines)
	res.Text = textview.Checkpoints(req.ProjectID, head.Revision, lines[:fit], len(lines))

	return res, nil
}

// CheckpointRename changes the name of a checkpoint, and its note when given.
func (s *svc) CheckpointRename(ctx context.Context, req CheckpointRenameRequest) (*Checkpoint, error) {
	if _, _, err := s.checkpointScope(ctx, "CheckpointRename", &req.ProjectID, req.TimelineID, true); err != nil {
		return nil, err
	}

	newName, nerr := cleanCheckpointName(req.NewName, "new_name")
	if nerr != nil {
		return nil, nerr
	}

	var note *string

	if req.Note != nil {
		n := cleanCheckpointNote(*req.Note)
		note = &n
	}

	repo := s.store.Checkpoints()

	if err := repo.Rename(ctx, req.ProjectID, req.Name, newName, note); err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			return nil, checkpointNotFound(req.Name)
		case errors.Is(err, ErrExists):
			return nil, newError(command.CodeIDExists, "the timeline already has a checkpoint named %q.", newName).
				withPath("new_name").withHint("choose another name")
		}

		return nil, asError(err)
	}

	row, err := repo.Get(ctx, req.ProjectID, newName)
	if err != nil {
		return nil, asError(err)
	}

	return checkpointOf(row), nil
}

// CheckpointRemove removes a checkpoint. No revision is removed.
func (s *svc) CheckpointRemove(ctx context.Context, req CheckpointRemoveRequest) (*Checkpoint, error) {
	if _, _, err := s.checkpointScope(ctx, "CheckpointRemove", &req.ProjectID, req.TimelineID, true); err != nil {
		return nil, err
	}

	repo := s.store.Checkpoints()

	row, err := repo.Get(ctx, req.ProjectID, req.Name)
	if err == nil {
		err = repo.Remove(ctx, req.ProjectID, req.Name)
	}

	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, checkpointNotFound(req.Name)
		}

		return nil, asError(err)
	}

	return checkpointOf(row), nil
}

// checkpointScope is what every checkpoint operation starts with: the caller's scope, the timeline, and the
// head of the project it may see. A write is refused on a project in the trash.
func (s *svc) checkpointScope(
	ctx context.Context,
	method string,
	projectID *string,
	timelineID string,
	write bool,
) (Caller, ProjectHead, error) {
	c, err := s.authorizeProject(ctx, method, *projectID)
	if err != nil {
		return Caller{}, ProjectHead{}, err
	}

	if err := s.useTimeline(ctx, c, projectID, timelineID); err != nil {
		return Caller{}, ProjectHead{}, err
	}

	head, err := s.store.Projects().Head(ctx, *projectID)
	if err != nil || !visible(c, head.OwnerUserID) {
		return Caller{}, ProjectHead{}, projectNotFound(*projectID, err)
	}

	if write && head.State == ProjectTrashed {
		return Caller{}, ProjectHead{}, newError(command.CodeInvalidState, "the project %s is in the trash.", head.ID).
			withHint("%s", hintRestore)
	}

	return c, head, nil
}

func checkpointNotFound(name string) *Error {
	return newError(command.CodeNotFound, "the timeline has no checkpoint named %q.", name).
		withDetail("checkpoint", name).withHint("list the checkpoints to see the names")
}

func checkpointOf(r CheckpointRow) *Checkpoint {
	return &Checkpoint{
		UntrustedNotice: UntrustedNotice, Name: r.Name, Note: r.Note, Revision: r.Revision,
		Actor: r.Actor, At: msTime(r.AtMs), Auto: r.Auto,
	}
}

// cleanCheckpointName cleans a name of control and bidirectional characters and refuses one that is empty,
// longer than 60 characters or holds a slash (the name is a path segment of the HTTP API).
func cleanCheckpointName(raw, path string) (string, *Error) {
	name := strings.TrimSpace(untrusted.Clean(raw))

	switch {
	case name == "":
		return "", newError(command.CodeValidation, "the checkpoint needs a name.").withPath(path).
			withHint("send a name of 1 to %d characters", maxCheckpointName)
	case utf8.RuneCountInString(name) > maxCheckpointName:
		return "", newError(command.CodeValidation, "the name is longer than %d characters.", maxCheckpointName).
			withPath(path).withHint("use a shorter name; the note takes the details")
	case strings.Contains(name, "/"):
		return "", newError(command.CodeValidation, "the name cannot contain a slash.").withPath(path).
			withHint("write the name without /")
	}

	return name, nil
}

func cleanCheckpointNote(raw string) string {
	note, _ := untrusted.Cut(strings.TrimSpace(untrusted.Clean(raw)), maxCheckpointNote)

	return note
}

// isAgentActor is the actors that count as an agent for the automatic checkpoint: every one except the browser
// UI, the command line a person types and a signed-in user (SDD-003 section 2.5.1).
func isAgentActor(actor string) bool {
	switch {
	case actor == "local:ui", actor == "local:cli":
		return false
	case strings.HasPrefix(actor, "user:"), strings.HasPrefix(actor, importedActorPrefix):
		return false
	}

	return strings.HasPrefix(actor, "key:") || strings.HasPrefix(actor, "local:")
}

// autoCheckpointName is the name that makes the checkpoint once a day per actor: the same actor on the same UTC
// day gets the same name, so the second edit finds the first one's checkpoint.
func autoCheckpointName(actor string, at time.Time) string {
	kept, _ := untrusted.Cut(untrusted.Clean(actor), maxAutoActorRunes)

	return autoNamePrefix + kept + " " + at.UTC().Format("2006-01-02")
}

// autoCheckpoint makes the checkpoint before an agent's first edit of the day, at the revision the edit starts
// from, inside the transaction of the edit (SDD-005 section 5.14). It keeps at most maxAutoCheckpoints automatic
// checkpoints; a checkpoint a person or an agent named is never removed here.
func (s *svc) autoCheckpoint(ctx context.Context, tx ProjectTx, actor string, revision int64) error {
	if !isAgentActor(actor) || revision < 1 {
		return nil
	}

	now := s.now()
	row := CheckpointRow{
		ProjectID: tx.Project().ID, Name: autoCheckpointName(actor, now), Revision: revision, Actor: actor, Auto: true,
		Note: "Made before the first edit of the day by " + actor + ".", AtMs: now.UnixMilli(),
	}

	if err := tx.Checkpoints().Create(ctx, row); err != nil {
		if errors.Is(err, ErrExists) {
			return nil
		}

		return err
	}

	return tx.Checkpoints().TrimAuto(ctx, row.ProjectID, maxAutoCheckpoints)
}
