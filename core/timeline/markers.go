package timeline

import "fmt"

// MaxMarkers is the most markers one item carries.
const MaxMarkers = 200

// Marker labels a frame of an item, for example a beat of a music item (SDD-001 §3.4).
type Marker struct {
	Frame int    `json:"frame"           min:"0"`
	Label string `json:"label,omitempty"         maxlen:"60"`
	Extra Extra  `json:"-"`
}

// docMarkers checks the markers of the document, the ones a person puts on the timeline:
// frames from 0 of the timeline, strictly increasing, at most MaxMarkers.
func (v *validator) docMarkers(d *Document) {
	if len(d.Markers) > MaxMarkers {
		v.add(VMarkers, "markers", d.ID, "%d markers exceed the limit of %d", len(d.Markers), MaxMarkers)
	}
	prev := -1
	for i, m := range d.Markers {
		mp := fmt.Sprintf("markers[%d]", i)
		if runes(m.Label) > 60 {
			v.add(VShape, mp+".label", d.ID, "label has more than 60 characters")
		}
		switch {
		case m.Frame < 0 || m.Frame >= MaxFrames:
			v.add(VMarkers, mp+".frame", d.ID, "marker frame %d is outside 0 to %d", m.Frame, MaxFrames-1)
		case m.Frame <= prev:
			v.add(VMarkers, mp+".frame", d.ID, "marker frames must be strictly increasing: %d after %d", m.Frame, prev)
		}
		prev = m.Frame
	}
}

// markers checks the markers of an item: inside the item, strictly increasing.
func (v *validator) markers(path, key string, it Item) {
	if len(it.Markers) > MaxMarkers {
		v.add(VMarkers, path+".markers", key, "%d markers exceed the limit of %d", len(it.Markers), MaxMarkers)
	}
	prev := -1
	for i, m := range it.Markers {
		mp := fmt.Sprintf("%s.markers[%d]", path, i)
		if runes(m.Label) > 60 {
			v.add(VShape, mp+".label", key, "label has more than 60 characters")
		}
		switch {
		case m.Frame < 0 || m.Frame >= it.Duration:
			v.add(VMarkers, mp+".frame", key, "marker frame %d is outside 0 to %d", m.Frame, it.Duration-1)
		case m.Frame <= prev:
			v.add(VMarkers, mp+".frame", key, "marker frames must be strictly increasing: %d after %d", m.Frame, prev)
		}
		prev = m.Frame
	}
}
