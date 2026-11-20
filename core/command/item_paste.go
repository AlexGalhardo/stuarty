package command

import (
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "item.paste_attributes",
		Summary: "Copies the look of one item to other items.",
		Description: "Copies attribute groups from one item to 1 to 20 others: color, filter, blur, framing, " +
			"transform (x, y, scale, rotation, opacity), blend, mask, flip, chroma_key, effects, audio (volume, " +
			"denoise, voice). A group the target type does not have is skipped and listed in effective.skipped. " +
			"Keyframes are not copied; a target whose animated property would be overwritten is refused.",
		Example: `{"from_item_id": "itm_m1n2p3q4r5", "item_ids": ["itm_s6t7v8w9x0"], "attributes": ["color", "filter"]}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState,
		},
		Scope: ScopeEdit,
	}, applyItemPasteAttributes))
}

type itemPasteArgs struct {
	FromItemID string   `json:"from_item_id" desc:"The item to copy from."                                                                                                           pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	ItemIDs    []string `json:"item_ids"     desc:"The items to copy to, 1 to 20."                                                                                                   pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$" minitems:"1" maxitems:"20"`
	Attributes []string `json:"attributes"   desc:"Groups to copy: color, filter, blur, framing, transform, blend, mask, flip, chroma_key, effects, audio. At least one, each once."                                                        minitems:"1" maxitems:"11"`
}

// pasteGroup is one group of attributes: the item types that have it, the properties that
// have keyframes and cannot be overwritten, and the copy.
type pasteGroup struct {
	name      string
	types     []timeline.ItemType
	animated  []string
	copyField func(dst, src *timeline.Item)
}

var visualTypes = []timeline.ItemType{
	timeline.ItemClip, timeline.ItemText, timeline.ItemCaption, timeline.ItemShape,
}

// withComposition is the visual types and the composition: the attributes every visual field of a composition
// shares with them (SDD-001 section 3.5.1); chroma key, colour, framing and audio are not among them.
var withComposition = append(slices.Clone(visualTypes), timeline.ItemComposition)

var pasteGroups = []pasteGroup{
	{"color", []timeline.ItemType{timeline.ItemClip}, timeline.ColourProps, func(d, s *timeline.Item) {
		d.Brightness, d.Contrast, d.Saturation = s.Brightness, s.Contrast, s.Saturation
		d.Temperature, d.Tint, d.Hue = s.Temperature, s.Tint, s.Hue
		d.Lut = s.Lut
		d.Hsl, d.Wheels, d.Curves = s.Hsl, s.Wheels, s.Curves
	}},
	{"filter", withComposition, nil, func(d, s *timeline.Item) { d.Filter, d.FilterAmount = s.Filter, s.FilterAmount }},
	{"blur", withComposition, nil, func(d, s *timeline.Item) { d.Blur = s.Blur }},
	{
		"framing",
		[]timeline.ItemType{timeline.ItemClip},
		timeline.CropProps,
		func(d, s *timeline.Item) { d.Fit, d.Crop = s.Fit, s.Crop },
	},
	{"transform", withComposition, []string{"x", "y", "scale", "rotation", "opacity"}, func(d, s *timeline.Item) {
		d.X, d.Y, d.Scale, d.Rotation, d.Opacity = s.X, s.Y, s.Scale, s.Rotation, s.Opacity
	}},
	{"blend", withComposition, nil, func(d, s *timeline.Item) { d.Blend = s.Blend }},
	{"mask", withComposition, timeline.MaskProps, func(d, s *timeline.Item) { d.Mask = s.Mask }},
	{"flip", withComposition, nil, func(d, s *timeline.Item) { d.FlipX, d.FlipY = s.FlipX, s.FlipY }},
	{"chroma_key", visualTypes, nil, func(d, s *timeline.Item) { d.ChromaKey = s.ChromaKey }},
	{"effects", withComposition, nil, func(d, s *timeline.Item) { d.Effects = s.Effects }},
	{
		"audio",
		[]timeline.ItemType{timeline.ItemClip, timeline.ItemAudio},
		[]string{"volume"},
		func(d, s *timeline.Item) {
			d.Volume, d.Denoise, d.Voice = s.Volume, s.Denoise, s.Voice
			d.Processors = timeline.CloneProcessors(s.Processors)
		},
	},
}

func pasteGroupNames() []string {
	names := make([]string, len(pasteGroups))
	for i, g := range pasteGroups {
		names[i] = g.name
	}
	return names
}

func pasteGroupByName(name string) *pasteGroup {
	for i := range pasteGroups {
		if pasteGroups[i].name == name {
			return &pasteGroups[i]
		}
	}
	return nil
}

// pasteGroupsOf checks the arguments that do not need the document and returns the groups asked for.
func pasteGroupsOf(a *itemPasteArgs) ([]*pasteGroup, *Error) {
	if err := idForm("args.from_item_id", timeline.PrefixItem, a.FromItemID); err != nil {
		return nil, err
	}
	if len(a.ItemIDs) < 1 || len(a.ItemIDs) > 20 {
		return nil, Errorf(CodeValidation, "args.item_ids has %d items; paste to 1 to 20.", len(a.ItemIDs)).
			WithHint("Send between one and twenty item ids.").WithPath("args.item_ids")
	}
	if len(a.Attributes) < 1 || len(a.Attributes) > len(pasteGroups) {
		return nil, Errorf(
			CodeValidation,
			"args.attributes has %d groups; give 1 to %d.",
			len(a.Attributes),
			len(pasteGroups),
		).
			WithHint("Use groups from: %s.", strings.Join(pasteGroupNames(), ", ")).WithPath("args.attributes")
	}
	groups := make([]*pasteGroup, len(a.Attributes))
	for i, name := range a.Attributes {
		g := pasteGroupByName(name)
		if g == nil {
			return nil, Errorf(CodeValidation, "args.attributes[%d] %q is not an attribute group.", i, name).
				WithHint("Use one of: %s.", strings.Join(pasteGroupNames(), ", ")).
				WithDetail("allowed", pasteGroupNames()).WithPath("args.attributes")
		}
		if slices.Contains(groups[:i], g) {
			return nil, Errorf(CodeValidation, "args.attributes lists %s twice.", name).
				WithHint("List each group once.").WithPath("args.attributes")
		}
		groups[i] = g
	}
	for i, id := range a.ItemIDs {
		if err := idForm("args.item_ids", timeline.PrefixItem, id); err != nil {
			return nil, err
		}
		if id == a.FromItemID {
			return nil, Errorf(CodeValidation, "args.item_ids lists the source item %s.", id).
				WithHint("Copy to other items.").WithPath("args.item_ids")
		}
		if slices.Contains(a.ItemIDs[:i], id) {
			return nil, Errorf(CodeValidation, "args.item_ids lists %s twice.", id).
				WithHint("List each item once.").WithPath("args.item_ids")
		}
	}
	return groups, nil
}

// paste is one target: the item as it will be, and the groups it takes.
type paste struct {
	it     timeline.Item
	groups []*pasteGroup
}

// pasteTarget picks the groups a target takes and refuses one whose animated property a group
// would overwrite; skipped collects the groups the target type does not have.
func pasteTarget(c *Context, id string, groups []*pasteGroup, skipped *[]map[string]string) (*paste, *Error) {
	dst, err := c.item(id)
	if err != nil {
		return nil, err
	}
	p := &paste{it: dst}
	for _, g := range groups {
		if !slices.Contains(g.types, dst.Type) {
			*skipped = append(*skipped, map[string]string{"item_id": id, "attribute": g.name})
			continue
		}
		if animated := animatedOf(&dst, g.animated); len(animated) > 0 {
			return nil, Errorf(CodeInvalidState, "Property %s of %s has %d keyframes, which define its value.",
				animated[0], id, len(dst.Keyframes[animated[0]])).
				WithHint("Use keyframe.clear on the target first, or leave %s out of attributes.", g.name).
				WithDetail("properties", animated).WithPath("args.item_ids")
		}
		p.groups = append(p.groups, g)
	}
	return p, nil
}

func applyItemPasteAttributes(c *Context, a *itemPasteArgs) error {
	groups, err := pasteGroupsOf(a)
	if err != nil {
		return err
	}
	src, err := c.item(a.FromItemID)
	if err != nil {
		return err
	}
	for _, g := range groups {
		if !slices.Contains(g.types, src.Type) {
			return Errorf(
				CodeTypeMismatch,
				"Item %s is a %s item and has no %s attributes.",
				a.FromItemID,
				src.Type,
				g.name,
			).
				WithHint("Copy from an item of a type that has them: %s.", typeList(g.types)).
				WithPath("args.attributes")
		}
	}

	pastes := make([]*paste, 0, len(a.ItemIDs))
	var skipped []map[string]string
	pasted := 0
	for _, id := range a.ItemIDs {
		p, err := pasteTarget(c, id, groups, &skipped)
		if err != nil {
			return err
		}
		pasted += len(p.groups)
		pastes = append(pastes, p)
	}
	if pasted == 0 {
		return Errorf(
			CodeTypeMismatch,
			"None of the target items has the attributes %s.",
			strings.Join(a.Attributes, ", "),
		).
			WithHint("Choose targets of a type that has them, or other groups.").
			WithPath("args.item_ids")
	}

	for _, p := range pastes {
		from := timeline.CloneItem(src)
		for _, g := range p.groups {
			g.copyField(&p.it, &from)
		}
		c.Doc.Items[p.it.ID] = p.it
	}
	c.SetEffective("items", len(pastes))
	c.SetEffective("attributes", a.Attributes)
	if len(skipped) > 0 {
		c.SetEffective("skipped", skipped)
	}
	return nil
}

func animatedOf(it *timeline.Item, props []string) []string {
	var out []string
	for _, p := range props {
		if len(it.Keyframes[p]) > 0 {
			out = append(out, p)
		}
	}
	return out
}

func typeList(types []timeline.ItemType) string {
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}
	return strings.Join(names, ", ")
}
