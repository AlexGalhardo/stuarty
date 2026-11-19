package timeline

import (
	"bytes"
	"math"
	"slices"
	"testing"
)

// The documents of testdata/old/v0.29.0 were written by release 0.29.0 (they are the project files of the
// golden cases of engine/export at that tag, copied byte for byte). Epic 31 adds the easing "bezier" and
// its curve to the schema; a document of the release before it must open and save unchanged, with the same
// sampled values, so that it exports the same frames (SDD-001 section 3.8).

const oldDir = "testdata/old/v0.29.0/*.json"

func TestOldDocumentsOpenAndSaveUnchanged(t *testing.T) {
	fixtures := files(t, oldDir)
	if len(fixtures) < 15 {
		t.Fatalf("%d documents of release 0.29.0, want the committed set", len(fixtures))
	}

	for _, path := range fixtures {
		raw := bytes.TrimSpace(readFile(t, path))

		doc, err := Decode(raw)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}

		// The ids of the golden builders (itm_i000000001) are not Crockford identifiers; that is
		// the only rule these project files never met.
		for _, v := range Validate(doc) {
			if v.Code != VID {
				t.Errorf("%s: %v", path, v)
			}
		}

		if got := Canonical(doc); !bytes.Equal(got, raw) {
			t.Errorf("%s: saving the document changed its bytes", path)
		}

		for id, it := range doc.Items {
			for prop, keys := range it.Keyframes {
				for i, k := range keys {
					if k.Bezier != nil || k.Easing == EasingBezier {
						t.Errorf("%s: %s %s[%d] has a curve that release 0.29.0 could not write", path, id, prop, i)
					}
				}
			}
		}
	}
}

// legacyEase is e(u) of SDD-004 section 3.1 as release 0.29.0 had it.
func legacyEase(kind string, u float64) float64 {
	switch kind {
	case "ease_in":
		return u * u * u
	case "ease_out":
		return 1 - (1-u)*(1-u)*(1-u)
	case "ease_in_out":
		if u < 0.5 {
			return 4 * u * u * u
		}

		return 1 - (-2*u+2)*(-2*u+2)*(-2*u+2)/2
	case "hold":
		return 0
	}

	return u
}

// legacyValue is the interpolation of keys at frame r as release 0.29.0 had it.
func legacyValue(keys []Keyframe, r float64) float64 {
	last := len(keys) - 1

	switch {
	case r <= float64(keys[0].T):
		return keys[0].Value
	case r >= float64(keys[last].T):
		return keys[last].Value
	}

	i := 0
	for i+1 < len(keys) && float64(keys[i+1].T) <= r {
		i++
	}

	u := (r - float64(keys[i].T)) / float64(keys[i+1].T-keys[i].T)

	return keys[i].Value + (keys[i+1].Value-keys[i].Value)*legacyEase(keys[i].Easing, u)
}

func TestOldDocumentsSampleTheSameValuesAsRelease029(t *testing.T) {
	animated := 0

	for _, path := range files(t, oldDir) {
		doc, err := Decode(bytes.TrimSpace(readFile(t, path)))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}

		for id, it := range doc.Items {
			props := make([]string, 0, len(it.Keyframes))
			for prop := range it.Keyframes {
				props = append(props, prop)
			}

			slices.Sort(props)

			for _, prop := range props {
				keys := it.Keyframes[prop]
				animated++

				for r := 0; r <= it.Duration; r++ {
					for _, f := range []float64{float64(r), float64(r) + 0.25, float64(r) + 0.5} {
						if got, want := Interpolate(keys, f), legacyValue(keys, f); got != want {
							t.Fatalf("%s: %s %s at %v: %v, release 0.29.0 gave %v", path, id, prop, f, got, want)
						}
					}

					if got, want := ValueAt(
						keys,
						r,
						0,
					), math.Round(
						legacyValue(keys, float64(r))*1000,
					)/1000; got != want {
						t.Fatalf("%s: %s %s ValueAt(%d) = %v, release 0.29.0 gave %v", path, id, prop, r, got, want)
					}
				}
			}
		}
	}

	if animated < 10 {
		t.Errorf("only %d animated properties in the old documents", animated)
	}
}
