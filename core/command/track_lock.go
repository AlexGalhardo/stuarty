package command

import (
	"reflect"
	"slices"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// lockedSnapshot is the items on locked tracks as they were before an operation.
type lockedSnapshot struct {
	tracks map[string]bool
	items  map[string]timeline.Item
}

// lockedItems copies the items on locked tracks, or returns nil when no track is locked,
// so a document without locks pays nothing.
func lockedItems(doc *timeline.Document) *lockedSnapshot {
	var snap *lockedSnapshot

	for id, t := range doc.Tracks {
		if !t.Locked {
			continue
		}
		if snap == nil {
			snap = &lockedSnapshot{tracks: map[string]bool{}, items: map[string]timeline.Item{}}
		}
		snap.tracks[id] = true
	}
	if snap == nil {
		return nil
	}

	// A deep copy: an operation may edit the maps and slices of an item in place.
	frozen := timeline.Clone(doc)
	for id := range frozen.Items {
		if it := frozen.Items[id]; snap.tracks[it.Track] {
			snap.items[id] = it
		}
	}

	return snap
}

// checkLocks refuses an operation that added, removed, moved or changed an item of a track
// that was locked when it started. Tracks themselves can still be updated, so a track can be
// unlocked.
func checkLocks(snap *lockedSnapshot, doc *timeline.Document) *Error {
	if snap == nil {
		return nil
	}

	touched := map[string]bool{}

	for id := range snap.items {
		old := snap.items[id]
		if now, ok := doc.Items[id]; !ok || !reflect.DeepEqual(old, now) {
			touched[old.Track] = true
		}
	}
	for id := range doc.Items {
		if _, was := snap.items[id]; !was && snap.tracks[doc.Items[id].Track] {
			touched[doc.Items[id].Track] = true
		}
	}
	if len(touched) == 0 {
		return nil
	}

	ids := make([]string, 0, len(touched))
	for id := range touched {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	id := ids[0]

	return Errorf(CodeInvalidState, "Track %s is locked; the operation would change its items.", id).
		WithHint("Unlock the track with track.update locked false, then repeat the edit.").
		WithDetail("track_ids", ids).WithPath("args")
}
