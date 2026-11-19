package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Every success case of settings.set_fps leaves a document that passes Validate.
func TestSetFPSGoldensValidate(t *testing.T) {
	dirs, err := filepath.Glob("testdata/settings.set_fps/ok_*")
	if err != nil || len(dirs) < 14 {
		t.Fatalf("cases %v %v", dirs, err)
	}
	for _, d := range dirs {
		b, err := os.ReadFile(filepath.Join(d, "after.json"))
		if err != nil {
			t.Fatal(err)
		}
		if vs := timeline.Check(b); len(vs) != 0 {
			t.Errorf("%s: %v", d, vs)
		}
	}
}

// 30 to 24 and back keeps every item inside the 108,000 frames; the rounding is
// lossy but never escapes the limit.
func TestSetFPSRoundTripStaysInsideTheLimit(t *testing.T) {
	doc := base().text(itemA, vidTr, 100000, 7999).text(itemB, vidTr, 107999, 1).d
	reg := Default()
	for _, fps := range []string{"24", "30", "120", "30"} {
		res, err := reg.Apply(doc, op2("settings.set_fps", `{"fps": "`+fps+`"}`), Env{})
		if err != nil {
			// 120 fps would pass the limit; that is the documented refusal.
			if fps != "120" {
				t.Fatalf("%s: %v", fps, err)
			}
			continue
		}
		doc = res.Doc
		for id := range doc.Items {
			if it := doc.Items[id]; it.Start+it.Duration > timeline.MaxFrames {
				t.Errorf("%s: %s ends at %d", fps, id, it.Start+it.Duration)
			}
		}
	}
}

func TestParseFPSForms(t *testing.T) {
	for _, bad := range []any{
		nil, 30.0, "", "abc", "30/x", "30/1001/2", "1/1001", "0", "30000/1000",
		map[string]any{"num": "30", "den": "1"},
	} {
		if _, err := parseFPS(bad); err == nil || err.Code != CodeValidation {
			t.Errorf("%v accepted", bad)
		}
	}
	if f, err := parseFPS("30000/1001"); err != nil || f.Num != 30000 || f.Den != 1001 {
		t.Errorf("%v %v", f, err)
	}
	if fpsString(timeline.Rational{Num: 30, Den: 1}) != "30" {
		t.Error("fpsString")
	}
}
