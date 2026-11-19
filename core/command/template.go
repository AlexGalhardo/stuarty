package command

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "template.set_info",
		Summary: "Sets the description and the named colours of the template.",
		Description: "Sets the description and adds, replaces or (with null) removes named colours. A colour that a " +
			"style, a shape or the background still refers to as \"$name\" cannot be removed. A document that has " +
			"no template gets one.",
		Example: `{"description": "Product teaser", "colors": {"brand": "#ff3366", "old": null}}`,
		Errors:  []string{CodeValidation, CodeInUse},
		Scope:   ScopeEdit,
	}, applyTemplateSetInfo))
}

type templateInfoArgs struct {
	Description *string        `json:"description,omitempty" desc:"0 to 500 characters."                                 max:"500"`
	Colors      map[string]any `json:"colors,omitempty"      desc:"Name to #RRGGBB or #RRGGBBAA; null removes the name."           schema:"template_colors"`
}

// colourUses lists where the document refers to "$name", as "id (field)", sorted.
func colourUses(d *timeline.Document, name string) (users []string, uses []string) {
	ref := "$" + name
	seen := map[string]bool{}
	add := func(id, field string) {
		uses = append(uses, fmt.Sprintf("%s (%s)", id, field))
		if !seen[id] {
			seen[id] = true
			users = append(users, id)
		}
	}
	styleUses := func(id, prefix string, s *timeline.CaptionStyle) {
		m := timeline.StyleToMap(s)
		for _, k := range slices.Sorted(maps.Keys(m)) {
			if v, ok := m[k].(string); ok && v == ref {
				add(id, prefix+"."+k)
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(d.Items)) {
		it := d.Items[id]
		styleUses(id, "style", it.Style)
		if string(it.Fill) == ref {
			add(id, "fill")
		}
		if string(it.StrokeColor) == ref {
			add(id, "stroke_color")
		}
		if it.Shadow != nil && string(it.Shadow.Color) == ref {
			add(id, "shadow.color")
		}
		if it.Outline != nil && string(it.Outline.Color) == ref {
			add(id, "outline.color")
		}
	}
	for _, id := range slices.Sorted(maps.Keys(d.Tracks)) {
		styleUses(id, "caption_style", d.Tracks[id].CaptionStyle)
	}
	if string(d.Settings.Background) == ref {
		add("settings", "background")
	}
	slices.Sort(users)
	return users, uses
}

func applyTemplateSetInfo(c *Context, a *templateInfoArgs) error {
	if err := atLeastOne([]string{"description", "colors"}, a.Description != nil, a.Colors != nil); err != nil {
		return err
	}
	colours := map[string]timeline.Color{}
	var removals []string
	for _, name := range slices.Sorted(maps.Keys(a.Colors)) {
		if !slotName.MatchString(name) {
			return Errorf(
				CodeValidation,
				"args.colors %q is not a valid name; it must match [a-z][a-z0-9_]{0,39}.",
				name,
			).
				WithHint("Use lower case letters, digits and underscores, for example \"brand\".").
				WithPath("args.colors")
		}
		v := a.Colors[name]
		if v == nil {
			removals = append(removals, name)
			continue
		}
		s, ok := v.(string)
		if !ok || !isHex(s) {
			return Errorf(CodeValidation, "args.colors.%s must be #RRGGBB or #RRGGBBAA, got %s.", name, colourWord(v)).
				WithHint(`Send "#RRGGBB" or "#RRGGBBAA".`).WithPath("args.colors." + name)
		}
		colours[name] = timeline.Color(strings.ToUpper(s))
	}

	tpl := timeline.TemplateInfo{Colors: map[string]timeline.Color{}}
	if t := c.Doc.Template; t != nil {
		tpl = timeline.TemplateInfo{
			Description: t.Description, Colors: maps.Clone(t.Colors), Instance: t.Instance, Extra: t.Extra,
		}
		if tpl.Colors == nil {
			tpl.Colors = map[string]timeline.Color{}
		}
	}
	for _, name := range removals {
		if _, has := tpl.Colors[name]; !has {
			continue
		}
		if users, uses := colourUses(c.Doc, name); len(users) > 0 {
			return Errorf(CodeInUse, "Colour %q is used by %s.", name, strings.Join(uses, " and ")).
				WithHint("Change those fields to a fixed colour with text.set and track.set_caption_style, then remove %q.",
					name).
				WithDetail("used_by", users).WithPath("args.colors." + name)
		}
		delete(tpl.Colors, name)
	}
	if a.Description != nil {
		tpl.Description = *a.Description
	}
	maps.Copy(tpl.Colors, colours)

	changed := c.Doc.Template != nil || a.Description != nil || len(colours) > 0
	if changed {
		c.Doc.Template = &tpl
	}
	stored := map[string]any{"description": tpl.Description, "colors": tpl.Colors}
	c.SetEffective("template", stored)
	return nil
}

func colourWord(v any) string {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	return jsonKind(v)
}
