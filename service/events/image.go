package events

import (
	"encoding/json"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
)

// The frames of the image document stream (SDD-006 section 12). They are made like the frames of a project
// stream: live and replayed events come from the same function, so a client cannot tell them apart and the 64 KiB
// rule holds for both.

type imagePayload struct {
	ImageID       string          `json:"image_id"`
	Revision      int64           `json:"revision"`
	BaseRevision  int64           `json:"base_revision"`
	Kind          string          `json:"kind"`
	Actor         string          `json:"actor"`
	At            string          `json:"at"`
	Diff          json.RawMessage `json:"diff,omitempty"`
	DiffTruncated bool            `json:"diff_truncated,omitempty"`
	Changed       []changed       `json:"changed,omitempty"`
	ChangedMore   int             `json:"changed_more,omitempty"`
}

// ImageRowFrame is image.updated for a row of the log of an image: the replay.
func ImageRowFrame(row *service.ImageOpRow) (Frame, error) {
	return imageFrame(&imagePayload{
		ImageID: row.ImageID, Revision: row.Revision, BaseRevision: row.BaseRevision, Kind: string(row.Kind),
		Actor: row.Actor, At: Time(time.UnixMilli(row.AtMs)), Diff: row.Diff,
	})
}

// ImageEventFrame is image.updated for the event the Service publishes after a commit: the live event.
func ImageEventFrame(ev *service.Event) (Frame, error) {
	return imageFrame(&imagePayload{
		ImageID: ev.ImageID, Revision: ev.Revision, BaseRevision: ev.BaseRevision, Kind: ev.Kind, Actor: ev.Actor,
		At: Time(ev.At), Diff: ev.Diff,
	})
}

func imageFrame(p *imagePayload) (Frame, error) {
	if len(p.Diff) == 0 {
		p.Diff = json.RawMessage("[]")
	}

	data, err := json.Marshal(p)
	if err != nil {
		return Frame{}, err
	}

	if len(data) > MaxEventBytes {
		var changes []changed

		_ = json.Unmarshal(p.Diff, &changes)

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

	return Frame{ID: itoa(p.Revision), Event: ImageUpdated, Data: string(data)}, nil
}

// ImageResyncFrame is resync.required on the stream of an image: it names the image, not a project. Like the
// project one it has no id, so the client never advances its Last-Event-ID over a range it missed.
func ImageResyncFrame(imageID string, revision int64, reason string) Frame {
	return marshal(ResyncRequired, struct {
		ImageID  string `json:"image_id"`
		Revision int64  `json:"revision"`
		Reason   string `json:"reason"`
	}{imageID, revision, reason})
}
