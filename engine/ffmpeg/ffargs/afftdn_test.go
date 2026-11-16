package ffargs

import "testing"

// The export decodes a source with denoise through afftdn (SDD-004 section 7, SDD-007 FF-10).
func TestAFFTDNIsInTheGrammar(t *testing.T) {
	t.Parallel()

	argv := func(af Filter) error {
		_, err := Cmd{
			Inputs:  []Input{FileInput(run("in.wav"))},
			Outputs: []Output{{Format: "s16le", Pipe: true, Maps: []string{"0:a:0"}, AF: Chain1(af, ATempo(1.5))}},
		}.Argv(testPolicy())
		return err
	}
	for _, nr := range []float64{0.01, 12, 30, 97} {
		if err := argv(AFFTDN(nr, -45)); err != nil {
			t.Errorf("nr %v: %v", nr, err)
		}
	}
	for _, nf := range []float64{-80, -45, -20} {
		if err := argv(AFFTDN(12, nf)); err != nil {
			t.Errorf("nf %v: %v", nf, err)
		}
	}
	for _, bad := range []Filter{AFFTDN(97.5, -45), AFFTDN(12, -10), AFFTDN(12, -81), AFFTDN(0, -45), "afftdn=nr=12:bn=x"} {
		if argv(bad) == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}
