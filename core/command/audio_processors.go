package command

import (
	"maps"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The commands of the audio processor chain of an item or a track (SDD-002 C.78 to C.82, SDD-004 section 7.1).

var processorErrors = []string{CodeValidation, CodeNotFound, CodeTypeMismatch}

func init() {
	Register(Define(Spec{
		Name:    "audio.processor_add",
		Summary: "Add",
		Description: "Adds an audio processor to the chain of a clip or audio item (item_id) or of a track (track_id). " +
			"kind is eq, compressor, limiter, gate, deesser or noise_reduction; params are the values that differ from " +
			"the default of the kind (units and ranges in SDD-004 7.1). The chain runs in order, so index matters; a " +
			"track chain runs on the sum of the track.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "kind": "compressor", "params": {"threshold_db": -24, "ratio": 4}}`,
		Errors:  append(slices.Clone(processorErrors), CodeLimit),
		Scope:   ScopeEdit,
	}, applyProcessorAdd))
	Register(Define(Spec{
		Name:    "audio.processor_set",
		Summary: "Set",
		Description: "Changes one processor of the chain of an item or a track. index is the 0-based place in the chain. " +
			"params are merged with the current ones; enabled false keeps the processor but skips it.",
		Example: `{"track_id": "trk_m1n2p3q4r5", "index": 0, "enabled": false}`,
		Errors:  processorErrors,
		Scope:   ScopeEdit,
	}, applyProcessorSet))
	Register(Define(Spec{
		Name:    "audio.processor_move",
		Summary: "Move processor",
		Description: "Moves a processor to another place in the chain of an item or a track. index and to are 0-based " +
			"places in the chain; the order changes the sound.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "index": 0, "to": 2}`,
		Errors:  processorErrors,
		Scope:   ScopeEdit,
	}, applyProcessorMove))
	Register(Define(Spec{
		Name:    "audio.processor_remove",
		Summary: "Remove processor",
		Description: "Removes a processor from the chain of an item or a track. index is the 0-based place in the chain; " +
			"later processors move one place earlier.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "index": 1}`,
		Errors:  processorErrors,
		Scope:   ScopeEdit,
	}, applyProcessorRemove))
	Register(Define(Spec{
		Name:    "audio.apply_preset",
		Summary: "Apply preset",
		Description: "Replaces the chain of an item or a track with a preset: voice, voice_with_music, podcast or " +
			"clean_noisy_room. The chain becomes exactly the preset; change any processor afterwards with " +
			"audio.processor_set. voice_with_music has no noise reduction.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "preset": "voice"}`,
		Errors:  processorErrors,
		Scope:   ScopeEdit,
	}, applyProcessorPreset))
}

// processorTarget is what the five commands share: the chain of an item or of a track.
type processorTarget struct{ ItemID, TrackID string }

type processorAddArgs struct {
	ItemID  string             `json:"item_id,omitempty"  desc:"A clip or audio item; give this or track_id."                    pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	TrackID string             `json:"track_id,omitempty" desc:"A track; give this or item_id."                                  pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Kind    string             `json:"kind"               desc:"eq, compressor, limiter, gate, deesser or noise_reduction."                                                             enum:"eq,compressor,limiter,gate,deesser,noise_reduction"`
	Params  map[string]float64 `json:"params,omitempty"   desc:"Parameter values by name; the rest keep their default."                                                                                                                           schema:"processor_params"`
	Index   *int               `json:"index,omitempty"    desc:"Where to insert, 0 to the length of the chain; default the end."                                                                                                                                            min:"0"`
	Enabled *bool              `json:"enabled,omitempty"  desc:"false adds it switched off; default true."`
}

type processorSetArgs struct {
	ItemID  string             `json:"item_id,omitempty"  desc:"A clip or audio item; give this or track_id." pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	TrackID string             `json:"track_id,omitempty" desc:"A track; give this or item_id."               pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Index   *int               `json:"index"              desc:"The processor to change, 0-based."                                                                   min:"0"`
	Params  map[string]float64 `json:"params,omitempty"   desc:"Values to change; the others stay."                                                                          schema:"processor_params"`
	Enabled *bool              `json:"enabled,omitempty"  desc:"true switches it on, false off."`
}

type processorMoveArgs struct {
	ItemID  string `json:"item_id,omitempty"  desc:"A clip or audio item; give this or track_id." pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	TrackID string `json:"track_id,omitempty" desc:"A track; give this or item_id."               pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Index   *int   `json:"index"              desc:"The processor to move, 0-based."                                                                     min:"0"`
	To      *int   `json:"to"                 desc:"Its new place, 0 to the length minus one."                                                           min:"0"`
}

type processorRemoveArgs struct {
	ItemID  string `json:"item_id,omitempty"  desc:"A clip or audio item; give this or track_id." pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	TrackID string `json:"track_id,omitempty" desc:"A track; give this or item_id."               pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Index   *int   `json:"index"              desc:"The processor to remove, 0-based."                                                                   min:"0"`
}

type processorPresetArgs struct {
	ItemID  string `json:"item_id,omitempty"  desc:"A clip or audio item; give this or track_id."          pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	TrackID string `json:"track_id,omitempty" desc:"A track; give this or item_id."                        pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Preset  string `json:"preset"             desc:"voice, voice_with_music, podcast or clean_noisy_room."                                                        enum:"voice,voice_with_music,podcast,clean_noisy_room"`
}

// chainRef is the chain of the target of a command and the way to put it back.
type chainRef struct {
	item  bool
	chain []timeline.Processor
	store func(chain []timeline.Processor)
}

// processorChain resolves the target: exactly one of an item (a clip or audio item with audio) or a track.
func (c *Context) processorChain(t processorTarget) (*chainRef, *Error) {
	if (t.ItemID == "") == (t.TrackID == "") {
		return nil, invalidArg("args.item_id", "Give exactly one of args.item_id and args.track_id.").
			WithHint("Send item_id for a clip or audio item, or track_id for a track chain.")
	}
	if err := idForm("args.item_id", timeline.PrefixItem, t.ItemID); err != nil {
		return nil, err
	}
	if err := idForm("args.track_id", timeline.PrefixTrack, t.TrackID); err != nil {
		return nil, err
	}
	if t.TrackID != "" {
		tr, err := c.track(t.TrackID, "args.track_id")
		if err != nil {
			return nil, err
		}
		return &chainRef{chain: slices.Clone(tr.Processors), store: func(chain []timeline.Processor) {
			tr.Processors = chain
			c.Doc.Tracks[t.TrackID] = tr
		}}, nil
	}
	it, err := c.item(t.ItemID)
	if err != nil {
		return nil, err
	}
	if it.Type != timeline.ItemClip && it.Type != timeline.ItemAudio {
		return nil, Errorf(CodeTypeMismatch, "Item %s is a %s item and has no audio.", t.ItemID, it.Type).
			WithHint("Use an audio item or a clip.").WithPath("args.item_id")
	}
	if err := c.stillImageNoAudio(&it, "has no audio to process"); err != nil {
		return nil, err
	}
	return &chainRef{item: true, chain: slices.Clone(it.Processors), store: func(chain []timeline.Processor) {
		it.Processors = chain
		c.Doc.Items[t.ItemID] = it
	}}, nil
}

// put stores the chain; an empty chain is no field.
func (r *chainRef) put(chain []timeline.Processor) {
	if len(chain) == 0 {
		chain = nil
	}
	r.store(chain)
}

// checkParams checks each value of params against the kind: the name, the range and the unit.
func checkParams(kind string, params map[string]float64) *Error {
	for _, name := range slices.Sorted(maps.Keys(params)) {
		spec, ok := paramOf(kind, name)
		if !ok {
			names := make([]string, 0, 8)
			for _, s := range timeline.ProcessorParams(kind) {
				names = append(names, s.Name)
			}
			return invalidArg("args.params."+name, "args.params.%s is not a parameter of %s.", name, kind).
				WithHint("The parameters of %s are %s.", kind, strings.Join(names, ", "))
		}
		if msg := spec.Check(params[name]); msg != "" {
			return invalidArg("args.params."+name, "args.params.%s.", msg).
				WithHint("Send %s between %s and %s.", name, trimFloat(spec.Min), trimFloat(spec.Max))
		}
	}
	return nil
}

func paramOf(kind, name string) (timeline.ParamSpec, bool) {
	for _, s := range timeline.ProcessorParams(kind) {
		if s.Name == name {
			return s, true
		}
	}
	return timeline.ParamSpec{}, false
}

// checkProcessor is the whole-processor check: the noise profile of the kind and, on a track, that it has none.
func checkProcessor(p timeline.Processor, item bool) *Error {
	if msg := timeline.CheckProcessor(p); msg != "" {
		return invalidArg("args.params", "args.params: %s.", msg).
			WithHint("Send profile_start_ms and profile_end_ms at least %d ms apart, or leave both out.", timeline.MinProfileMS)
	}
	if !item && p.Kind == timeline.ProcNoiseReduction &&
		(p.Value("profile_start_ms") != 0 || p.Value("profile_end_ms") != 0) {
		return invalidArg("args.params", "args.params: a marked noise profile belongs to an item, not to a track.").
			WithHint("Use item_id to mark a range of the clip, or leave the profile out to estimate it.")
	}
	return nil
}

func indexIn(path string, index *int, n int) (int, *Error) {
	if index == nil {
		return 0, invalidArg(path, "%s is required.", path).WithHint("Send the 0-based place of the processor.")
	}
	if *index < 0 || *index >= n {
		return 0, invalidArg(path, "%s %d is outside the chain, which has %d processors.", path, *index, n).
			WithHint("Use 0 to %d.", max(n-1, 0))
	}
	return *index, nil
}

func processorEffective(c *Context, index int, p timeline.Processor) {
	c.SetEffective("index", index)
	c.SetEffective("kind", p.Kind)
	c.SetEffective("params", p.FullParams())
	c.SetEffective("enabled", !p.Disabled)
}

func applyProcessorAdd(c *Context, a *processorAddArgs) error {
	ref, err := c.processorChain(processorTarget{a.ItemID, a.TrackID})
	if err != nil {
		return err
	}
	if !timeline.ValidProcessorKind(a.Kind) {
		return invalidArg("args.kind", "args.kind %q is not a processor.", a.Kind).
			WithHint("Use %s.", strings.Join(timeline.ProcessorKinds, ", "))
	}
	if err := checkParams(a.Kind, a.Params); err != nil {
		return err
	}
	at := len(ref.chain)
	if a.Index != nil {
		if *a.Index < 0 || *a.Index > len(ref.chain) {
			return invalidArg(
				"args.index",
				"args.index %d is outside the chain, which has %d processors.",
				*a.Index,
				len(ref.chain),
			).
				WithHint("Use 0 to %d.", len(ref.chain))
		}
		at = *a.Index
	}
	p := timeline.Processor{Kind: a.Kind, Disabled: a.Enabled != nil && !*a.Enabled, Params: a.Params}.Normalized()
	if err := checkProcessor(p, ref.item); err != nil {
		return err
	}
	if len(ref.chain) >= timeline.MaxProcessors {
		return Errorf(
			CodeLimit,
			"The chain already has %d processors; the limit is %d.",
			len(ref.chain),
			timeline.MaxProcessors,
		).
			WithHint("Remove one with audio.processor_remove first.")
	}
	chain := slices.Insert(ref.chain, at, p)
	ref.put(chain)
	processorEffective(c, at, p)

	return nil
}

func applyProcessorSet(c *Context, a *processorSetArgs) error {
	ref, err := c.processorChain(processorTarget{a.ItemID, a.TrackID})
	if err != nil {
		return err
	}
	if err := atLeastOne([]string{"params", "enabled"}, len(a.Params) > 0, a.Enabled != nil); err != nil {
		return err
	}
	at, err := indexIn("args.index", a.Index, len(ref.chain))
	if err != nil {
		return err
	}
	p := ref.chain[at]
	if err := checkParams(p.Kind, a.Params); err != nil {
		return err
	}
	merged := timeline.Processor{Kind: p.Kind, Disabled: p.Disabled, Params: map[string]float64{}}
	maps.Copy(merged.Params, p.Params)
	maps.Copy(merged.Params, a.Params)
	if a.Enabled != nil {
		merged.Disabled = !*a.Enabled
	}
	merged = merged.Normalized()
	if err := checkProcessor(merged, ref.item); err != nil {
		return err
	}
	ref.chain[at] = merged
	ref.put(ref.chain)
	processorEffective(c, at, merged)

	return nil
}

func applyProcessorMove(c *Context, a *processorMoveArgs) error {
	ref, err := c.processorChain(processorTarget{a.ItemID, a.TrackID})
	if err != nil {
		return err
	}
	from, err := indexIn("args.index", a.Index, len(ref.chain))
	if err != nil {
		return err
	}
	to, err := indexIn("args.to", a.To, len(ref.chain))
	if err != nil {
		return err
	}
	p := ref.chain[from]
	chain := slices.Delete(ref.chain, from, from+1)
	chain = slices.Insert(chain, to, p)
	ref.put(chain)
	c.SetEffective("index", from)
	c.SetEffective("to", to)
	c.SetEffective("kind", p.Kind)

	return nil
}

func applyProcessorRemove(c *Context, a *processorRemoveArgs) error {
	ref, err := c.processorChain(processorTarget{a.ItemID, a.TrackID})
	if err != nil {
		return err
	}
	at, err := indexIn("args.index", a.Index, len(ref.chain))
	if err != nil {
		return err
	}
	kind := ref.chain[at].Kind
	chain := slices.Delete(ref.chain, at, at+1)
	ref.put(chain)
	c.SetEffective("index", at)
	c.SetEffective("kind", kind)
	c.SetEffective("remaining", len(chain))

	return nil
}

func applyProcessorPreset(c *Context, a *processorPresetArgs) error {
	ref, err := c.processorChain(processorTarget{a.ItemID, a.TrackID})
	if err != nil {
		return err
	}
	chain := timeline.PresetChain(a.Preset)
	if chain == nil {
		return invalidArg("args.preset", "args.preset %q is not a preset.", a.Preset).
			WithHint("Use %s.", strings.Join(timeline.Presets, ", "))
	}
	ref.put(chain)
	c.SetEffective("preset", a.Preset)
	c.SetEffective("processors", len(chain))

	return nil
}
