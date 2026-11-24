package service

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/engine/capturehost"
	"github.com/AlexGalhardo/stuarty/service/recordings"
)

// decodeTracks reads the JSON list of a row; a list that does not decode is empty.
func decodeTracks(s string) []recording.Track {
	var out []recording.Track
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}

	return out
}

func decodePauses(s string) []recording.Pause {
	var out []recording.Pause
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}

	return out
}

// settledTracks is what a row keeps of the tracks when a recording ends (SDD-025 section 16.8). With the helper's
// finished message (fin) it is what the helper reported, if that holds together (core/recording validates it).
// Without one (a failed helper, or the recovery of start-up) it is the tracks the request asked for whose file is
// there, with the size of the file: the recording keeps what was written.
func settledTracks(row recordings.Row, folder string, fin *capturehost.Finished) (tracks, pauses string, events int64) {
	var list []recording.Track

	var gaps []recording.Pause

	if fin != nil {
		for _, t := range fin.Tracks {
			k := recording.TrackKind(t.Kind)
			list = append(list, recording.Track{
				Kind: k, File: k.File(), StartMS: t.StartMS, DurationMS: t.DurationMS, Bytes: t.Bytes, Lost: t.Lost,
			})
		}

		if recording.ValidateTracks(list) != nil {
			list = nil
		}

		for _, p := range fin.Pauses {
			gaps = append(gaps, recording.Pause{FromMS: p.FromMS, ToMS: p.ToMS})
		}

		if recording.ValidatePauses(gaps) != nil {
			gaps = nil
		}

		events = max(fin.PointerEvents, 0)
	}

	if list == nil {
		for _, t := range decodeTracks(row.Tracks) {
			if st, err := os.Stat(filepath.Join(folder, t.File)); err == nil && st.Mode().IsRegular() {
				t.Bytes = st.Size()
				list = append(list, t)
			}
		}
	}

	if fin == nil && events == 0 {
		events = countEvents(filepath.Join(folder, recording.EventsFile))
	}

	return encodeList(nonNil(list)), encodeList(nonNilPauses(gaps)), events
}

// encodeList is the JSON of a list of plain values, which cannot fail to encode; an empty list is the answer if
// it somehow does.
func encodeList(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}

	return string(b)
}

func nonNil(t []recording.Track) []recording.Track {
	if t == nil {
		return []recording.Track{}
	}

	return t
}

func nonNilPauses(p []recording.Pause) []recording.Pause {
	if p == nil {
		return []recording.Pause{}
	}

	return p
}

// countEvents is the events a file holds, read through the reader that validates them; 0 for no file.
func countEvents(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}

	defer func() { _ = f.Close() }()

	res, err := recording.ReadEvents(f, recording.EventsQuery{Limit: 1})
	if err != nil {
		return 0
	}

	return int64(res.Total)
}

// tracksBytes is the size of every track.
func tracksBytes(tracks string) int64 {
	var n int64

	for _, t := range decodeTracks(tracks) {
		n += t.Bytes
	}

	return n
}
