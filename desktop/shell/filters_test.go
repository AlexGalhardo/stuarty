package shell

import (
	"strings"
	"testing"
)

func TestMediaFilters(t *testing.T) {
	f := MediaFilters()
	if len(f) < 2 || f[0].Pattern == "*.*" {
		t.Fatalf("the first filter must be the media one, got %+v", f)
	}
	for _, want := range []string{"*.mp4", "*.MP4", "*.wav", "*.png"} {
		if !strings.Contains(f[0].Pattern, want) {
			t.Errorf("first filter misses %s", want)
		}
	}
	if f[len(f)-1].Pattern != "*.*" {
		t.Errorf("the last filter must keep any file reachable")
	}
}
