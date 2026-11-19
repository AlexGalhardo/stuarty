package command

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "audio.duck",
		Summary: "Lowers a music item while speech sounds, with a ramp down before and a ramp up after.",
		Description: "Writes volume keyframes on the music item: its volume drops by gain_db over attack before each " +
			"stretch of speech and returns over release after it. Speech stretches closer than the ramps are held " +
			"low across the gap. The speech comes from the analysis of the speech items and is supplied by the " +
			"service in resolved; the command never starts a job. The item must have no volume keyframes yet; " +
			"keyframe.clear on volume undoes the ducking.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "speech_item_ids": ["itm_m1n2p3q4r5"], "gain_db": -12}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeLimit,
		},
		Scope: ScopeEdit,
		Phase: Beta,
	}, applyDuck))
}

// SpeechRanges is the speech of one item on the time axis of its asset, in frames,
// half-open, sorted and not overlapping.
type SpeechRanges struct {
	ItemID string         `json:"item_id" desc:"One of the speech items."                                             pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Ranges []SilenceRange `json:"ranges"  desc:"Sorted, non-overlapping ranges of speech in asset frames, half-open."`
}

// ResolvedSpeech is the analysis result the service puts in `resolved` (A.9).
type ResolvedSpeech struct {
	Speech []SpeechRanges `json:"speech" desc:"One entry per speech item, in any order."`
}

type duckArgs struct {
	ItemID        string          `json:"item_id"            desc:"The music: an audio item with no volume keyframes."                          pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	SpeechItemIDs []string        `json:"speech_item_ids"    desc:"Clips or audio items that carry the speech."                                 pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$" min:"1"   max:"20"`
	GainDB        *float64        `json:"gain_db,omitempty"  desc:"Change of volume while speech sounds, -40 to -1; default -12."                                                                      min:"-40" max:"-1"`
	Attack        Time            `json:"attack,omitempty"   desc:"A length, may be 0; the ramp down before speech. Default \"0.2s\"."`
	Release       Time            `json:"release,omitempty"  desc:"A length, may be 0; the ramp up after speech. Default \"0.5s\"."`
	Resolved      *ResolvedSpeech `json:"resolved,omitempty" desc:"Filled by the service from the speech of the items; never sent by a caller."`
}

func checkSpeech(ids []string, r *ResolvedSpeech) *Error {
	if r == nil {
		return Errorf(CodeValidation, "args.resolved is missing: the speech of the items is not known.").
			WithHint("The service fills it from the speech found when a file is imported: call media_inspect on the speech assets and retry once they are ready (an asset without audio has no speech); the cause is the state of the assets, not the arguments.").
			WithPath("args.resolved")
	}
	seen := map[string]bool{}
	for i, s := range r.Speech {
		path := fmt.Sprintf("args.resolved.speech[%d]", i)
		if !slices.Contains(ids, s.ItemID) || seen[s.ItemID] {
			return Errorf(CodeValidation, "%s is for %q, which is not a speech item given once.", path, s.ItemID).
				WithHint("Send one entry for each id of speech_item_ids.").WithPath(path)
		}
		seen[s.ItemID] = true
		if err := checkSilences(&ResolvedSilences{Silences: s.Ranges}); err != nil {
			return err.WithPath(path + ".ranges")
		}
	}
	for i, id := range ids {
		if !seen[id] {
			return Errorf(CodeValidation, "args.resolved.speech has no entry for args.speech_item_ids[%d], %s.", i, id).
				WithHint("Send one entry for each id of speech_item_ids.").WithPath("args.resolved.speech")
		}
	}
	return nil
}

// speechOnMusic maps the speech ranges (asset frames) of a speech item to frames of
// the music item, half-open: a frame that holds any speech is inside.
func speechOnMusic(m, p *timeline.Item, rs []SilenceRange) []SilenceRange {
	var out []SilenceRange
	num, den := p.Speed.Num, p.Speed.Den
	for _, r := range rs {
		s, e := max(r.Start, p.SourceIn), r.End
		if e <= s {
			continue
		}
		lo := p.Start + (s-p.SourceIn)*den/num
		hi := min(p.Start+ceilDiv((e-p.SourceIn)*den, num), itemEnd(p))
		lo, hi = max(lo, m.Start)-m.Start, min(hi, itemEnd(m))-m.Start
		if hi > lo {
			out = append(out, SilenceRange{lo, hi})
		}
	}
	return out
}

// duckKeyframes builds the volume keyframes: the original volume at frame 0, and
// for each stretch of speech a ramp down ending where speech starts, the low volume
// held to its end, and a ramp up. A ramp is at least one frame long.
func duckKeyframes(ranges []SilenceRange, duration, attack, release int, base, low float64) []timeline.Keyframe {
	release = max(release, 1)
	var merged []SilenceRange
	for _, r := range ranges {
		last := len(merged) - 1
		if last >= 0 && max(r.Start-attack, 0) <= min(merged[last].End+release, duration) {
			merged[last].End = max(merged[last].End, r.End)
			continue
		}
		merged = append(merged, r)
	}

	var keys []timeline.Keyframe
	put := func(t int, v float64) {
		if n := len(keys); n > 0 && keys[n-1].T == t {
			keys[n-1].Value = v
			return
		}
		keys = append(keys, timeline.NewKeyframe(t, v))
	}
	put(0, base)
	for _, r := range merged {
		down := max(r.Start-attack, 0)
		put(down, base)
		put(max(r.Start, down+1), low)
		if r.End >= duration {
			continue
		}
		put(r.End, low)
		put(min(r.End+release, duration), base)
	}
	return keys
}

// speechItem is one speech source of audio.duck. The path stays args.speech_item_ids.
func (c *Context) speechItem(id string) (timeline.Item, *Error) {
	p, err := c.item(id)
	if err != nil {
		return p, err.WithPath("args.speech_item_ids")
	}
	if p.Type != timeline.ItemClip && p.Type != timeline.ItemAudio {
		return p, Errorf(CodeTypeMismatch, "Item %s is %s item and carries no speech.", p.ID, article(p.Type)).
			WithHint("Choose clips or audio items.").WithPath("args.speech_item_ids")
	}
	if asset, ok := c.Doc.Assets[p.Asset]; !ok || !assetHasAudio(asset) {
		return p, Errorf(CodeTypeMismatch, "Item %s has no asset with audio.", p.ID).
			WithHint("Choose items placed from an asset with audio.").WithPath("args.speech_item_ids")
	}
	if err := rejectRemap(&p, "ducking under its speech"); err != nil {
		return p, err.WithPath("args.speech_item_ids")
	}
	return p, nil
}

func applyDuck(c *Context, a *duckArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	for i, id := range a.SpeechItemIDs {
		path := fmt.Sprintf("args.speech_item_ids[%d]", i)
		if err := idForm(path, timeline.PrefixItem, id); err != nil {
			return err
		}
		if id == a.ItemID || slices.Index(a.SpeechItemIDs, id) != i {
			return Errorf(CodeValidation, "%s is %s, which is the music or repeated.", path, id).
				WithHint("List each speech item once, apart from the music.").WithPath(path)
		}
	}
	gain := -12.0
	if a.GainDB != nil {
		gain = *a.GainDB
	}
	attack, err := c.length("args.attack", cmp.Or(string(a.Attack), "0.2s"), true)
	if err != nil {
		return err
	}
	release, err := c.length("args.release", cmp.Or(string(a.Release), "0.5s"), true)
	if err != nil {
		return err
	}

	m, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if err := c.audioSource(&m, "only music can be ducked."); err != nil {
		return err
	}
	if n := len(m.Keyframes["volume"]); n > 0 {
		return Errorf(CodeInvalidState, "Item %s already has %d volume keyframes.", a.ItemID, n).
			WithHint("Run keyframe.clear on volume first, then duck again.").WithPath("args.item_id")
	}

	// Name a missing or wrong item before the internal resolved field. The service
	// leaves resolved empty in that case and expects this answer (VV-207).
	speechItems := make(map[string]timeline.Item, len(a.SpeechItemIDs))
	for _, id := range a.SpeechItemIDs {
		p, err := c.speechItem(id)
		if err != nil {
			return err
		}
		speechItems[id] = p
	}
	if err := checkSpeech(a.SpeechItemIDs, a.Resolved); err != nil {
		return err
	}

	var ranges []SilenceRange
	for _, s := range a.Resolved.Speech {
		p := speechItems[s.ItemID]
		ranges = append(ranges, speechOnMusic(&m, &p, s.Ranges)...)
	}
	slices.SortFunc(ranges, func(x, y SilenceRange) int { return cmp.Or(x.Start-y.Start, x.End-y.End) })

	low := round3(m.Volume * math.Pow(10, gain/20))
	keys := duckKeyframes(ranges, m.Duration, attack, release, m.Volume, low)
	if len(ranges) > 0 {
		if len(keys) > maxKeyframes {
			return Errorf(CodeLimit, "Ducking %s would need %d volume keyframes; the limit is %d.",
				a.ItemID, len(keys), maxKeyframes).
				WithHint("Duck a shorter item, split it, or use longer ramps so that nearby speech merges.")
		}
		ks := cloneKeyframes(m.Keyframes)
		ks["volume"] = keys
		m.Keyframes = ks
		c.Doc.Items[a.ItemID] = m
	}
	c.SetEffective("gain_db", gain)
	c.SetEffectiveTime("attack", attack, string(a.Attack))
	c.SetEffectiveTime("release", release, string(a.Release))
	c.SetEffective("ducked", len(ranges) > 0)
	c.SetEffective("low_volume", low)
	c.SetEffective("keyframes", len(keys))
	return nil
}
