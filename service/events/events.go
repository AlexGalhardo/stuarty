// Package events builds the payloads of the event stream (SDD-006 section 3) and
// decides what a connecting client is owed (section 4.3). It is pure: no hub, no
// HTTP, no store. The adapter in web/app/events carries the frames.
package events

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
)

// Limits of SDD-006 sections 4.3 and 9.
const (
	MaxEventBytes      = 64 << 10
	MaxReplayRevisions = 1000
	MaxReplayBytes     = 16 << 20
	ReplayPage         = 100
	MaxChanged         = 500
	MaxDescription     = 500
)

// Names of the events.
const (
	BatchCommitted = "batch.committed"
	ProjectUpdated = "project.updated"
	NestedUpdated  = "nested.updated"
	AssetUpdated   = "asset.updated"
	JobUpdated     = "job.updated"
	ResyncRequired = "resync.required"
	// ImageUpdated is the event of the stream of an image document (SDD-006 section 12).
	ImageUpdated = "image.updated"
)

// Reasons of resync.required.
const (
	ReasonGapTooLarge = "gap_too_large"
	ReasonBeforeFloor = "before_floor"
	ReasonClientAhead = "client_ahead"
)

// Frame is one event as the wire carries it: the adapter writes it with the
// encoder of the transport. ID is set for batch.committed only.
type Frame struct {
	ID    string
	Event string
	Data  string
}

// Time is the timestamp format of the stream: RFC 3339 UTC with milliseconds.
func Time(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

type batchPayload struct {
	ProjectID      string          `json:"project_id"`
	Revision       int64           `json:"revision"`
	BaseRevision   int64           `json:"base_revision"`
	Kind           string          `json:"kind"`
	Actor          string          `json:"actor"`
	At             string          `json:"at"`
	TargetRevision *int64          `json:"target_revision,omitempty"`
	SkippedCount   *int            `json:"skipped_count,omitempty"`
	Description    string          `json:"description"`
	Diff           json.RawMessage `json:"diff,omitempty"`
	DiffTruncated  bool            `json:"diff_truncated,omitempty"`
	Changed        []changed       `json:"changed,omitempty"`
	ChangedMore    int             `json:"changed_more,omitempty"`
}

type changed struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Describe is the description of a batch for the stream: the text of the diff
// cut to 500 characters, ending with "..." when cut.
func Describe(doc *timeline.Document, diff json.RawMessage) string {
	var changes []command.Change
	if len(diff) > 0 {
		_ = json.Unmarshal(diff, &changes)
	}

	text := command.DescribeDiff(doc, changes)
	if utf8.RuneCountInString(text) <= MaxDescription {
		return text
	}

	r := []rune(text)

	return string(r[:MaxDescription-3]) + "..."
}

// BatchFrame is batch.committed for a row of the log. Live and replayed events
// are both made here, so a client cannot tell them apart and the 64 KiB rule
// holds for both (section 9.3).
func BatchFrame(row *service.OperationRow, description string) (Frame, error) {
	p := batchPayload{
		ProjectID: row.ProjectID, Revision: row.Revision, BaseRevision: row.BaseRevision, Kind: string(row.Kind),
		Actor: row.Actor, At: Time(time.UnixMilli(row.AtMs)), TargetRevision: row.TargetRevision,
		Description: description, Diff: row.Diff,
	}

	if len(p.Diff) == 0 {
		p.Diff = json.RawMessage("[]")
	}

	if row.Kind == service.OpUndo || row.Kind == service.OpRedo {
		var skipped []json.RawMessage
		if len(row.Skipped) > 0 {
			_ = json.Unmarshal(row.Skipped, &skipped)
		}

		n := len(skipped)
		p.SkippedCount = &n
	}

	data, err := json.Marshal(p)
	if err != nil {
		return Frame{}, err
	}

	if len(data) > MaxEventBytes {
		var changes []changed

		_ = json.Unmarshal(row.Diff, &changes)

		p.Diff, p.DiffTruncated = nil, true

		if len(changes) > MaxChanged {
			p.ChangedMore = len(changes) - MaxChanged
			changes = changes[:MaxChanged]
		}

		p.Changed = changes

		if data, err = json.Marshal(p); err != nil {
			return Frame{}, err
		}
	}

	return Frame{ID: itoa(row.Revision), Event: BatchCommitted, Data: string(data)}, nil
}

// ProjectFrame is project.updated.
func ProjectFrame(ev *service.Event) Frame {
	return marshal(ProjectUpdated, struct {
		ProjectID string `json:"project_id"`
		State     string `json:"state"`
		At        string `json:"at"`
	}{ev.ProjectID, ev.State, Time(ev.At)})
}

type wireError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Hint      string         `json:"hint,omitempty"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details"`
}

func wireOf(e *service.Error) *wireError {
	if e == nil {
		return nil
	}

	d := e.Details
	if d == nil {
		d = map[string]any{}
	}

	return &wireError{Code: e.Code, Message: e.Message, Hint: e.Hint, Retryable: e.Retryable, Details: d}
}

// AssetFrame is asset.updated.
func AssetFrame(ev *service.Event) Frame {
	return marshal(AssetUpdated, struct {
		ProjectID string     `json:"project_id"`
		AssetID   string     `json:"asset_id"`
		State     string     `json:"state"`
		Progress  float64    `json:"progress"`
		Message   string     `json:"message"`
		Error     *wireError `json:"error"`
		UpdatedAt string     `json:"updated_at"`
	}{ev.ProjectID, ev.AssetID, ev.State, ev.Progress, ev.Message, wireOf(ev.Error), Time(ev.At)})
}

// JobFrame is job.updated. An ingest job belongs to the file, so its
// project_id is null.
func JobFrame(ev *service.Event) Frame {
	var project *string
	if ev.Kind != service.JobIngest {
		project = &ev.ProjectID
	}

	return marshal(JobUpdated, struct {
		ProjectID *string            `json:"project_id"`
		JobID     string             `json:"job_id"`
		Kind      string             `json:"kind"`
		State     string             `json:"state"`
		Progress  float64            `json:"progress"`
		Message   string             `json:"message"`
		Error     *wireError         `json:"error"`
		Result    *service.JobResult `json:"result"`
		UpdatedAt string             `json:"updated_at"`
	}{project, ev.JobID, ev.Kind, ev.State, ev.Progress, ev.Message, wireOf(ev.Error), ev.Result, Time(ev.At)})
}

// ResyncFrame is resync.required: it has no id, so the client never advances
// its Last-Event-ID over a range it missed.
func ResyncFrame(project string, revision int64, reason string) Frame {
	return marshal(ResyncRequired, struct {
		ProjectID string `json:"project_id"`
		Revision  int64  `json:"revision"`
		Reason    string `json:"reason"`
	}{project, revision, reason})
}

func marshal(event string, v any) Frame {
	data, err := json.Marshal(v)
	if err != nil {
		data = []byte("{}") // the payloads are plain values: this cannot happen
	}

	return Frame{Event: event, Data: string(data)}
}

// Action is what a connecting client is owed.
type Action int

// The actions of section 4.3.
const (
	Live    Action = iota // no replay: live events and the snapshot only
	Nothing               // the client is at the head
	Replay                // revisions after the client's, from the log
	Resync                // resync.required, with a reason
)

// Decide is the table of section 4.3. since is nil when the client gave no
// revision; head and floor come from the project.
func Decide(since *int64, head, floor int64) (Action, string) {
	switch {
	case since == nil:
		return Live, ""
	case *since > head:
		return Resync, ReasonClientAhead
	case *since == head:
		return Nothing, ""
	case *since < floor:
		return Resync, ReasonBeforeFloor
	case head-*since > MaxReplayRevisions:
		return Resync, ReasonGapTooLarge
	}

	return Replay, ""
}

// Replayable reports whether a row can be replayed: a migration or an import
// is never replayed (it sets the floor).
func Replayable(row *service.OperationRow) bool {
	k := strings.ToLower(string(row.Kind))

	return k != "migrate" && k != "import"
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// NestedFrame is nested.updated: the timeline ev.TimelineID, which the timeline ev.ProjectID holds as a
// composition, changed at revision ev.Revision (SDD-006 section 3.3.1).
func NestedFrame(ev *service.Event) Frame {
	return marshal(NestedUpdated, struct {
		ProjectID  string `json:"project_id"`
		TimelineID string `json:"timeline_id"`
		Revision   int64  `json:"revision"`
		At         string `json:"at"`
	}{ev.ProjectID, ev.TimelineID, ev.Revision, Time(ev.At)})
}
