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
