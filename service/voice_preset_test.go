package service

import (
	"encoding/json"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The studio voice presets of audio.apply_voice_preset (SDD-002 C.91): each gives a valid chain that reaches the plan
// of the mix, complete and ending in the limiter at -1 dBFS, and the one without a preset name is the studio chain.
func TestVoicePresetsReachThePlan(t *testing.T) {
	t.Parallel()

	for _, name := range append([]string{""}, timeline.VoicePresets...) {
		env := measureEnv(t, nil)
		store, _ := env.s.store.(*batchStore)

		doc, err := timeline.Decode(store.projects.row.Document)
		if err != nil {
			t.Fatal(err)
		}

		args := map[string]string{"item_id": measureAudioItem}
		if name != "" {
			args["preset"] = name
		}

		raw, merr := json.Marshal(args)
		if merr != nil {
			t.Fatal(merr)
		}

		res, aerr := command.Default().
			Apply(doc, command.Operation{Op: "audio.apply_voice_preset", Args: raw}, command.Env{})
		if aerr != nil {
			t.Fatalf("%q: %v", name, aerr)
		}

		if vs := timeline.Validate(res.Doc); len(vs) > 0 {
			t.Fatalf("%q: the document is not valid: %+v", name, vs[0])
		}

		srcs := plan.Audio(res.Doc, 0, 300)
		if len(srcs) != 1 || len(srcs[0].Chain) == 0 {
			t.Fatalf("%q: the plan has %+v", name, srcs)
		}

		chain := srcs[0].Chain
		if last := chain[len(chain)-1]; last.Kind != timeline.ProcLimiter || last.Params["ceiling_db"] != -1 {
			t.Errorf("%q: the chain does not end with the limiter at -1 dBFS: %+v", name, last)
		}

		for _, p := range chain {
			if len(p.Params) != len(timeline.ProcessorParams(p.Kind)) {
				t.Errorf("%q: %s carries %d of its parameters", name, p.Kind, len(p.Params))
			}
		}

		want := name
		if want == "" {
			want = timeline.VoicePresets[0]
		}

		if len(chain) != len(timeline.VoicePresetChain(want)) {
			t.Errorf("%q: the plan has %d processors, the %s chain has %d",
				name, len(chain), want, len(timeline.VoicePresetChain(want)))
		}
	}
}
