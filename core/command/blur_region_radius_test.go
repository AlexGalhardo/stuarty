package command

import "testing"

// The strong default: the least radius is 1.5% of the shorter side, the default 4%, and neither passes the limit of the
// blur field (SDD-002 C.90).
func TestBlurRegionRadiusRules(t *testing.T) {
	t.Parallel()

	s := base().d.Settings
	least, def := BlurRadii(s)

	if least != 17 || def != 44 {
		t.Errorf("at 1080 pixels the least and the default radius are %v and %v, want 17 and 44", least, def)
	}

	s.Width, s.Height = 3840, 2160
	if least, def = BlurRadii(s); least != 33 || def != 87 {
		t.Errorf("at 2160 pixels the least and the default radius are %v and %v, want 33 and 87", least, def)
	}

	s.Width, s.Height = 16, 16
	if least, _ = BlurRadii(s); least != 1 {
		t.Errorf("a tiny frame has a least radius of %v, want 1", least)
	}

	s.Width, s.Height = 16000, 16000
	if least, def = BlurRadii(s); least != 100 || def != 100 {
		t.Errorf("a huge frame has radii %v and %v, want both held to 100", least, def)
	}
}
