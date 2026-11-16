package timeline

// TrackVisible says whether the picture of the items of a track is drawn: the track is not
// hidden and, when any visual track is solo, it is one of them. A solo audio track does not
// hide any picture.
func (d *Document) TrackVisible(id string) bool {
	t, ok := d.Tracks[id]
	if !ok || t.Hidden {
		return false
	}

	for _, o := range d.Tracks {
		if o.Solo && o.Kind == TrackVisual {
			return t.Solo
		}
	}

	return true
}

// TrackSounds says whether the sound of the items of a track is mixed: the track is not
// muted and, when any track is solo, it is one of them.
func (d *Document) TrackSounds(id string) bool {
	t, ok := d.Tracks[id]
	if !ok || t.Muted {
		return false
	}

	for _, o := range d.Tracks {
		if o.Solo {
			return t.Solo
		}
	}

	return true
}

// TrackGain is the volume of a track, a factor on the gain of its items: 1 when none is set.
func (d *Document) TrackGain(id string) float64 {
	if v := d.Tracks[id].Volume; v != nil {
		return *v
	}

	return 1
}
