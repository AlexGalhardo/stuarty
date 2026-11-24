package command

import (
	"cmp"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "audio.apply_voice_preset",
		Summary: "Replaces a chain with a studio voice preset.",
		Description: "One action for a clean, even voice: studio (default), broadcast or room_rescue. The chain of " +
			"an item (item_id) or a track (track_id) becomes the preset: noise reduction, equaliser, de-esser, " +
			"compressor and limiter, and the export brings the mix to the loudness_target. Change any processor " +
			"afterwards with audio.processor_set.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "preset": "studio"}`,
		Errors:  processorErrors,
		Scope:   ScopeEdit,
	}, applyVoicePreset))
}

type voicePresetArgs struct {
	ItemID  string `json:"item_id,omitempty"  desc:"A clip or audio item; give this or track_id." pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	TrackID string `json:"track_id,omitempty" desc:"A track; give this or item_id."               pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Preset  string `json:"preset,omitempty"   desc:"studio (default), broadcast or room_rescue."                                                         enum:"studio,broadcast,room_rescue"`
}

func applyVoicePreset(c *Context, a *voicePresetArgs) error {
	ref, err := c.processorChain(processorTarget{a.ItemID, a.TrackID})
	if err != nil {
		return err
	}
	name := cmp.Or(a.Preset, timeline.VoicePresets[0])
	chain := timeline.VoicePresetChain(name)
	if chain == nil {
		return invalidArg("args.preset", "args.preset %q is not a voice preset.", SafeKey(name)).
			WithHint("Use %s.", strings.Join(timeline.VoicePresets, ", "))
	}
	ref.put(chain)
	c.SetEffective("preset", name)
	c.SetEffective("processors", len(chain))

	return nil
}
