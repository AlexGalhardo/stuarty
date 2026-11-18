package plan

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func TestPlannerMatchesFrameWith(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("..", "timeline", "testdata", "valid", "launch-teaser.json"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := timeline.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	pl := NewPlanner(doc, Options{})

	for _, f := range []int{0, 1, 17, 60, 119, 120, 500, 100000} {
		if got, want := pl.Frame(f), FrameWith(doc, f, Options{}); !reflect.DeepEqual(got, want) {
			t.Errorf("frame %d differs:\n got %+v\nwant %+v", f, got, want)
		}
	}
}
