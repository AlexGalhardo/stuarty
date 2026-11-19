package command

import (
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "text.add",
		Summary: "Adds a text item to a visual track.",
		Description: "Adds a text item. The style is stored exactly as given, including keys equal to the default. x and y " +
			"are the centre of the item in pixels; without them the item is centred.",
		Example: `{"track_id": "trk_f6g7h8j9k0", "start": "0s", "duration": "2s", "text": "Hello", "style": {"size": 96}, "id": "itm_s6t7v8w9x0"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeIDExists, CodeLimit,
			CodeFontUnknown, CodeOverlap,
		},
		Scope: ScopeEdit,
		Phase: Prototype,
	}, applyTextAdd))

	Register(Define(Spec{
		Name:    "text.set",
		Summary: "Changes the content or the style of a text item.",
		Description: "Replaces the text, merges a partial style into the stored one (null removes a key), or empties the " +
			"style first with reset_style. At least one of text, style and reset_style is required.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "text": "Hello world", "style": {"color": "#FFE600"}}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch, CodeFontUnknown},
		Scope:   ScopeEdit,
		Phase:   Prototype,
	}, applyTextSet))
}

type textAddArgs struct {
	TrackID  string         `json:"track_id,omitempty" desc:"A visual track; omitted, the highest visual track free over the item (an empty track you made for video counts; pass track_id to keep it clear), or a new one on top. effective.track_id names it." pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Start    Time           `json:"start"`
	Duration Time           `json:"duration"           desc:"A length of at least 1 frame."`
	Text     string         `json:"text"               desc:"The text; line breaks are allowed."                                                                                                                                                                                                                        min:"1" max:"2000"`
	Style    map[string]any `json:"style,omitempty"    desc:"A partial TextStyle."                                                                                                                                                                                                                                                         schema:"text_style"`
	X        *float64       `json:"x,omitempty"`
	Y        *float64       `json:"y,omitempty"`
	ID       string         `json:"id,omitempty"                                                                                                                                                                                                                 pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

func applyTextAdd(c *Context, a *textAddArgs) error {
	for _, f := range []struct{ path, prefix, id string }{
		{"args.track_id", timeline.PrefixTrack, a.TrackID}, {"args.id", timeline.PrefixItem, a.ID},
	} {
		if err := idForm(f.path, f.prefix, f.id); err != nil {
			return err
		}
	}
	if err := c.validateStyle(a.Style, false, false); err != nil {
		return err
	}
	start, err := c.ParseTime("args.start", string(a.Start))
	if err != nil {
		return err
	}
	duration, err := c.length("args.duration", string(a.Duration), false)
	if err != nil {
		return err
	}
	var auto bool
	if a.TrackID, auto, err = c.visualTrackFor(a.TrackID, start, duration); err != nil {
		return err
	}
	if auto {
		c.SetEffective("track_id", a.TrackID)
	}
	t, err := c.track(a.TrackID, "args.track_id")
	if err != nil {
		return err
	}
	if t.Kind != timeline.TrackVisual {
		return Errorf(CodeTypeMismatch, "Track %s is an audio track; text needs a visual track.", a.TrackID).
			WithHint("Use a visual track.").WithPath("args.track_id")
	}
	id, err := c.newID("items", timeline.PrefixItem, a.ID)
	if err != nil {
		return err
	}
	if err := c.limit("items", len(c.Doc.Items), timeline.MaxItems); err != nil {
		return err
	}
	style, err := mergeStyle(nil, a.Style, false)
	if err != nil {
		return err
	}
	if err := c.checkFont(style); err != nil {
		return err
	}
	if err := c.checkEnd("args.duration", start, duration); err != nil {
		return err
	}
	if err := c.checkOverlap(id, a.TrackID, start, duration); err != nil {
		return err
	}
	it := timeline.NewItem(id, timeline.ItemText, a.TrackID, start, duration)
	it.Text, it.Style, it.X, it.Y = a.Text, style, a.X, a.Y
	c.Doc.Items[id] = it
	c.SetEffective("id", id)
	c.SetEffectiveTime("start", start, string(a.Start))
	c.SetEffectiveTime("duration", duration, string(a.Duration))
	c.SetEffective("style", styleOrEmpty(style))
	return nil
}

func styleOrEmpty(s *timeline.CaptionStyle) map[string]any {
	if m := timeline.StyleToMap(s); m != nil {
		return m
	}
	return map[string]any{}
}

type textSetArgs struct {
	ItemID     string         `json:"item_id"               desc:"A text item."                                     pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Text       *string        `json:"text,omitempty"                                                                                                                       min:"1" max:"2000"`
	Style      map[string]any `json:"style,omitempty"       desc:"A partial TextStyle, merged; null removes a key."                                                                           schema:"text_style_merge"`
	ResetStyle bool           `json:"reset_style,omitempty" desc:"Empty the stored style before applying style."`
}

func applyTextSet(c *Context, a *textSetArgs) error {
	if err := atLeastOne(
		[]string{"text", "style", "reset_style"},
		a.Text != nil,
		a.Style != nil,
		a.ResetStyle,
	); err != nil {
		return err
	}
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	if err := c.validateStyle(a.Style, false, true); err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if it.Type != timeline.ItemText {
		return Errorf(CodeTypeMismatch, "Item %s is a %s; text.set changes text items.", a.ItemID, it.Type).
			WithHint("Use caption.set_words or caption.edit_word for captions.").WithPath("args.item_id")
	}
	current := it.Style
	if a.ResetStyle {
		current = nil
	}
	style, err := mergeStyle(current, a.Style, false)
	if err != nil {
		return err
	}
	if err := c.checkFont(style); err != nil {
		return err
	}
	if a.Text != nil {
		it.Text = *a.Text
	}
	it.Style = style
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("text", it.Text)
	c.SetEffective("style", styleOrEmpty(style))
	return nil
}
