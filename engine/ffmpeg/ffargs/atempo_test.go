package ffargs

import "testing"

// The export decodes a clip with a speed other than 1 through atempo (SDD-004 section 7).
func TestATempoIsInTheGrammar(t *testing.T) {
	t.Parallel()

	for _, tempo := range []float64{0.5, 1.25, 2, 1.0 / 3} {
		_, err := Cmd{
			Inputs: []Input{FileInput(run("in.wav"))},
			Outputs: []Output{{
				Format: "s16le", Pipe: true, Maps: []string{"0:a:0"},
				AF: Chain1(ATempo(tempo), ATempo(2)),
			}},
		}.Argv(testPolicy())
		if err != nil {
			t.Errorf("atempo=%v: %v", tempo, err)
		}
	}

	if _, err := (Cmd{
		Inputs:  []Input{FileInput(run("in.wav"))},
		Outputs: []Output{{Format: "s16le", Pipe: true, Maps: []string{"0:a:0"}, AF: "atempo=2;1"}},
	}).Argv(testPolicy()); err == nil {
		t.Error("a tempo outside the grammar was accepted")
	}
}
