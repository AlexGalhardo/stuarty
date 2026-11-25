package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service/recordings"
)

// resolvePointer fills resolved of recording.auto_zoom, recording.set_cursor and recording.set_click_highlight
// with the pointer events of the recording whose screen track is the clip's asset, for the source range the clip
// plays (SDD-025 section 17.2). The zoom and the highlight get the presses; the cursor gets the positions too, thinned
// by core/recording. It reads a file and starts nothing; without pointer events the batch fails with E_INVALID_STATE
// and nothing is applied.
func (s *svc) resolvePointer(
	ctx context.Context,
	tx Tx,
	projectID string,
	doc *timeline.Document,
	op *command.Operation,
	index int,
) (json.RawMessage, error) {
	a, ok := unresolvedArgs(op)
	if !ok || (op.Op == "recording.set_cursor" && a.Visible != nil && !*a.Visible) ||
		(op.Op == "recording.set_click_highlight" && a.Style == "none") {
		return nil, nil //nolint:nilnil // nothing to resolve
	}

	it, ok := doc.Items[a.ItemID]
	if !ok {
		return nil, nil //nolint:nilnil // the command answers E_NOT_FOUND
	}

	asset, ok := doc.Assets[it.Asset]
	if !ok {
		return nil, nil //nolint:nilnil // the command answers E_NOT_FOUND
	}

	fps := doc.Settings.FPS
	if fps.Num < 1 || fps.Den < 1 || it.Speed.Num < 1 || it.Speed.Den < 1 {
		return nil, nil //nolint:nilnil // the command reports it
	}

	from := int64(it.SourceIn) * 1000 * int64(fps.Den) / int64(fps.Num)
	to := int64(it.SourceIn+it.Duration*it.Speed.Num/it.Speed.Den) * 1000 * int64(fps.Den) / int64(fps.Num)

	const what = "the pointer events of the screen recording"

	hint := "use a clip of a screen recording a person made in Stuarty with pointer events (the record dialog has a switch)"

	row, found, err := screenRecordingOf(ctx, tx, projectID, asset.Hash)
	if err != nil {
		return nil, err
	}

	if !found {
		return nil, notAnalysed(index, asset.ID, what, hint)
	}

	f, oerr := os.Open(filepath.Join(s.workspace, recordingsDir, row.ID, recording.EventsFile))
	if oerr != nil {
		return nil, notAnalysed(index, asset.ID, what, hint)
	}

	defer func() { _ = f.Close() }()

	track, rerr := recording.ReadTrack(f, recording.TrackQuery{
		FromMS: from, ToMS: to, ClicksOnly: op.Op != "recording.set_cursor",
	})
	if rerr != nil {
		return nil, notAnalysed(index, asset.ID, what, hint)
	}

	out, merr := json.Marshal(track)
	if merr != nil {
		return nil, internalError()
	}

	return out, nil
}

// screenRecordingOf is the recording of the project that a person added and whose screen track became the asset with
// that hash.
func screenRecordingOf(ctx context.Context, tx Tx, projectID, hash string) (recordings.Row, bool, error) {
	rows, err := tx.Recordings().ListByProject(ctx, projectID)
	if err != nil {
		return recordings.Row{}, false, asError(err)
	}

	for _, r := range rows {
		if r.AssetHash == hash && recording.Placeable(recording.State(r.State), recording.Review(r.Review)) {
			return r, true, nil
		}
	}

	return recordings.Row{}, false, nil
}
