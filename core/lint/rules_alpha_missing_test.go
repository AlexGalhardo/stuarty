package lint

import "testing"

// L_ALPHA_MISSING: a clip reads a file that declares alpha and whose sampled pictures are all opaque. It needs the
// fact from the store (Env.AlphaOpaque), runs in Full only, and stays quiet without the fact or for an asset that
// does not declare alpha.
func TestAlphaMissing(t *testing.T) {
	t.Parallel()

	opaque := func(opaque, known bool) Env {
		return Env{AlphaOpaque: func(string) (bool, bool) { return opaque, known }}
	}

	for name, c := range map[string]struct {
		alpha bool
		env   Env
		mode  Mode
		want  int
	}{
		"declares alpha, all opaque":       {true, opaque(true, true), Full, 1},
		"declares alpha, has transparency": {true, opaque(false, true), Full, 0},
		"the fact is not stored":           {true, opaque(true, false), Full, 0},
		"no alpha declared":                {false, opaque(true, true), Full, 0},
		"no input at all":                  {true, Env{}, Full, 0},
		"a fast run reads no artifact":     {true, opaque(true, true), Fast, 0},
	} {
		d := fine()
		a := d.Assets[astVideo]
		a.HasAlpha = c.alpha
		d.Assets[astVideo] = a

		got := has(Lint(d, c.env, c.mode), CodeAlphaMissing)
		if len(got) != c.want {
			t.Errorf("%s: %d findings, want %d", name, len(got), c.want)

			continue
		}

		if c.want == 1 {
			f := got[0]
			if f.Severity != SeverityWarning || f.Item != "itm_a" || f.Message == "" || f.Hint == "" ||
				Blocking(CodeAlphaMissing) {
				t.Errorf("%s: %+v", name, f)
			}
		}
	}
}

// A disabled clip is ignored, as every rule ignores it.
func TestAlphaMissingIgnoresADisabledClip(t *testing.T) {
	t.Parallel()

	d := fine()
	a := d.Assets[astVideo]
	a.HasAlpha = true
	d.Assets[astVideo] = a

	it := d.Items["itm_a"]
	it.Disabled = true
	d.Items["itm_a"] = it

	env := Env{AlphaOpaque: func(string) (bool, bool) { return true, true }}
	if got := has(Lint(d, env, Full), CodeAlphaMissing); len(got) != 0 {
		t.Errorf("a disabled clip: %+v", got)
	}
}
