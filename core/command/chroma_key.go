package command

import (
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The values a chroma key starts with when the clip has none and the command does not give them.
const (
	defaultChromaSimilarity = 0.3
	defaultChromaSmoothness = 0.1
	defaultChromaSpill      = 0.0
)

func init() {
	Register(Define(Spec{
		Name:    "clip.set_chroma_key",
		Summary: "Keys out one colour of a clip, such as a green screen.",
		Description: "Makes the pixels near a colour transparent. similarity (0 to 1, default 0.3) is how far from the " +
			"colour a pixel is still removed, smoothness (0 to 1, default 0.1) the soft edge beyond it and spill " +
			"(0 to 1, default 0) how much of the colour is taken out of the pixels that stay. On a clip that is " +
			"already keyed, values not given keep their current value. color null removes the key. Values are " +
			"rounded to 3 decimals.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "color": "#00FF00", "similarity": 0.35, "smoothness": 0.1, "spill": 0.2}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch},
		Scope:   ScopeEdit,
	}, applyClipSetChromaKey))
}

type clipChromaKeyArgs struct {
	ItemID     string   `json:"item_id"              desc:"A clip."                                                   pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Color      any      `json:"color"                desc:"The colour to remove, #RRGGBB, or null to remove the key."                                                        schema:"chroma_color"`
	Similarity *float64 `json:"similarity,omitempty" desc:"0 to 1; default 0.3, or the current value."                                                                                             min:"0" max:"1"`
	Smoothness *float64 `json:"smoothness,omitempty" desc:"0 to 1; default 0.1, or the current value."                                                                                             min:"0" max:"1"`
	Spill      *float64 `json:"spill,omitempty"      desc:"0 to 1; default 0, or the current value."                                                                                               min:"0" max:"1"`
}

func applyClipSetChromaKey(c *Context, a *clipChromaKeyArgs) error {
	it, err := c.clipItem(a.ItemID, "chroma key")
	if err != nil {
		return err
	}
	if a.Color == nil {
		if a.Similarity != nil || a.Smoothness != nil || a.Spill != nil {
			return invalidArg(
				"args.color",
				"args.similarity, smoothness or spill is given with color null, which removes the key.",
			).
				WithHint("Omit them, or send a colour.")
		}
		it.ChromaKey = nil
		c.Doc.Items[a.ItemID] = it
		c.SetEffective("chroma_key", nil)
		return nil
	}
	if e := c.checkColour("args.color", a.Color); e != nil {
		return e
	}
	colour, _ := a.Color.(string) // checkColour accepted it, so it is a string
	key := timeline.ChromaKey{
		Color:      timeline.Color(colour),
		Similarity: defaultChromaSimilarity, Smoothness: defaultChromaSmoothness, Spill: defaultChromaSpill,
	}
	if cur := it.ChromaKey; cur != nil {
		key.Similarity, key.Smoothness, key.Spill, key.Extra = cur.Similarity, cur.Smoothness, cur.Spill, cur.Extra
	}
	for _, f := range []struct {
		name string
		v    *float64
		to   *float64
	}{
		{"similarity", a.Similarity, &key.Similarity},
		{"smoothness", a.Smoothness, &key.Smoothness},
		{"spill", a.Spill, &key.Spill},
	} {
		if f.v == nil {
			continue
		}
		if *f.v < 0 || *f.v > 1 {
			return invalidArg("args."+f.name, "The value of args.%s is %v, outside the accepted range.", f.name, *f.v).
				WithHint("Use a value between 0 and 1.")
		}
		*f.to = round3(*f.v)
	}
	it.ChromaKey = &key
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("chroma_key", map[string]any{
		"color": string(key.Color), "similarity": key.Similarity, "smoothness": key.Smoothness, "spill": key.Spill,
	})
	return nil
}
