package image

import (
	"github.com/AlexGalhardo/stuarty/core/command"
)

func init() {
	register(
		command.DefineIn(command.Spec{
			Name:    "layer.transform",
			Summary: "Sets the position, size or rotation of a layer.",
			Description: "x and y are the top left corner in pixels of the parent (the canvas, or the origin of the group); " +
				"w and h the size; rotation whole degrees clockwise around the centre, -360 to 360. A group takes only x " +
				"and y, which move all its children. Changing w of a text layer measures its height again unless h is given.",
			Example: `{"layer_id": "lyr_a1b2c3d4e5", "x": 80, "y": 440, "w": 800, "rotation": -5}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeInvalidState,
			},
			Scope: command.ScopeEdit,
		}, applyTransform),
		command.DefineIn(command.Spec{
			Name:    "layer.set",
			Summary: "Sets the name, opacity, visibility, lock or slot of a layer.",
			Description: "opacity is a whole percent. A locked layer accepts this command, to unlock it. slot names a " +
				"template slot (a text or an image layer; the name is unique in the document) and an empty slot removes it.",
			Example: `{"layer_id": "lyr_a1b2c3d4e5", "opacity": 80, "name": "Headline"}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeIDExists,
			},
			Scope: command.ScopeEdit,
		}, applySet),
		command.DefineIn(command.Spec{
			Name:    "layer.crop",
			Summary: "Sets the part of the source an image layer shows.",
			Description: "x, y, w and h are pixels of the source and must lie inside it. The box of the layer keeps its " +
				"size, so use layer.fit afterwards to keep the aspect. reset true shows the whole source again.",
			Example: `{"layer_id": "lyr_a1b2c3d4e5", "x": 200, "y": 0, "w": 900, "h": 900}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeOutOfSource,
				command.CodeInvalidState,
			},
			Scope: command.ScopeEdit,
		}, applyCrop),
		command.DefineIn(command.Spec{
			Name:    "layer.set_text",
			Summary: "Changes the text or the style of a text layer.",
			Description: "style is a partial text style merged key by key; a null value removes a key and reset_style empties " +
				"the stored style first. The height of the box is measured again.",
			Example: `{"layer_id": "lyr_a1b2c3d4e5", "text": "Hello world", "style": {"color": "#FFE600"}}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeInvalidState,
				command.CodeFontUnknown,
			},
			Scope: command.ScopeEdit,
		}, applySetText),
		command.DefineIn(command.Spec{
			Name:    "layer.set_shape",
			Summary: "Changes the shape, colours, stroke or radius of a shape layer, or the colour of a fill.",
			Description: "A fill layer takes only fill. A radius is valid with rounded_rect only, and changing the shape to " +
				"another one removes a stored radius. An empty stroke_color removes the stroke colour.",
			Example: `{"layer_id": "lyr_a1b2c3d4e5", "shape": "rounded_rect", "radius": 24, "fill": "#E63946"}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeInvalidState,
			},
			Scope: command.ScopeEdit,
		}, applySetShape),
	)
}

type transformArgs struct {
	LayerID  string `json:"layer_id"           pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	X        *int   `json:"x,omitempty"                                                               min:"-16384" max:"16384"`
	Y        *int   `json:"y,omitempty"                                                               min:"-16384" max:"16384"`
	W        *int   `json:"w,omitempty"                                                               min:"1"      max:"16384"`
	H        *int   `json:"h,omitempty"                                                               min:"1"      max:"16384"`
	Rotation *int   `json:"rotation,omitempty"                                                        min:"-360"   max:"360"`
}

func applyTransform(c *command.Context, d *Document, a *transformArgs) error {
	if err := atLeastOne([]string{"x", "y", "w", "h", "rotation"},
		a.X != nil, a.Y != nil, a.W != nil, a.H != nil, a.Rotation != nil); err != nil {
		return err
	}

	l, err := target(d, a.LayerID, "args.layer_id")
	if err != nil {
		return err
	}

	if l.Kind == KindGroup && (a.W != nil || a.H != nil || a.Rotation != nil) {
		return command.Errorf(command.CodeTypeMismatch, "Layer %s is a group; it takes x and y only.", l.ID).
			WithHint("Move the group with x and y, or transform its children.").WithPath("args.layer_id")
	}

	l.X, l.Y = orElse(a.X, l.X), orElse(a.Y, l.Y)

	if l.Kind != KindGroup {
		l.W, l.H = orElse(a.W, l.W), orElse(a.H, l.H)
		l.Rotation = orElse(a.Rotation, l.Rotation)

		if l.Kind == KindText && a.W != nil && a.H == nil {
			remeasure(envOf(c), d, l)
		}
	}

	c.SetEffective("layer_id", l.ID)
	c.SetEffective("box", boxOf(l))
	c.SetEffective("rotation", l.Rotation)

	return nil
}

type setArgs struct {
	LayerID string  `json:"layer_id"          pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Name    *string `json:"name,omitempty"                                                           max:"120"`
	Opacity *int    `json:"opacity,omitempty"                                                        max:"100" desc:"A whole percent, 0 to 100."                        min:"0"`
	Visible *bool   `json:"visible,omitempty"`
	Locked  *bool   `json:"locked,omitempty"`
	Slot    *string `json:"slot,omitempty"                                                                     desc:"A template slot name; an empty string removes it."`
}

func applySet(c *command.Context, d *Document, a *setArgs) error {
	if err := atLeastOne([]string{"name", "opacity", "visible", "locked", "slot"},
		a.Name != nil, a.Opacity != nil, a.Visible != nil, a.Locked != nil, a.Slot != nil); err != nil {
		return err
	}

	l, err := reference(d, a.LayerID, "args.layer_id")
	if err != nil {
		return err
	}

	if a.Slot != nil && *a.Slot != "" {
		if l.Kind != KindText && l.Kind != KindImage {
			return command.Errorf(command.CodeTypeMismatch, "Layer %s is a %s; only a text or an image layer can be a slot.", l.ID, l.Kind).
				WithHint("Put the slot on a text or an image layer.").
				WithPath("args.slot")
		}

		if err := slotName(d, *a.Slot, l.ID, "args.slot"); err != nil {
			return err
		}
	}

	if a.Name != nil {
		l.Name = *a.Name
	}

	if a.Opacity != nil {
		l.Opacity = *a.Opacity
	}

	if a.Visible != nil {
		l.Visible = *a.Visible
	}

	if a.Locked != nil {
		l.Locked = *a.Locked
	}

	if a.Slot != nil {
		l.Slot = *a.Slot
	}

	c.SetEffective("layer_id", l.ID)

	return nil
}

type cropArgs struct {
	LayerID string `json:"layer_id"        pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	X       *int   `json:"x,omitempty"                                                            desc:"Pixels of the source."                           min:"0" max:"65536"`
	Y       *int   `json:"y,omitempty"                                                            desc:"Pixels of the source."                           min:"0" max:"65536"`
	W       *int   `json:"w,omitempty"                                                            desc:"Pixels of the source."                           min:"1" max:"65536"`
	H       *int   `json:"h,omitempty"                                                            desc:"Pixels of the source."                           min:"1" max:"65536"`
	Reset   bool   `json:"reset,omitempty"                                                        desc:"Show the whole source again; no other argument."`
}

func applyCrop(c *command.Context, d *Document, a *cropArgs) error {
	given := a.X != nil || a.Y != nil || a.W != nil || a.H != nil

	switch {
	case a.Reset && given:
		return invalid(
			"args.reset",
			"Give reset, or x, y, w and h, not both.",
		).WithHint("Remove the rectangle, or remove reset.")
	case !a.Reset && (a.X == nil || a.Y == nil || a.W == nil || a.H == nil):
		return invalid("args", "Give all of x, y, w and h, or reset.").
			WithHint("Add the missing members of the rectangle of the source.")
	}

	l, err := target(d, a.LayerID, "args.layer_id")
	if err != nil {
		return err
	}

	if l.Kind != KindImage {
		return command.Errorf(command.CodeTypeMismatch, "Layer %s is a %s; layer.crop changes image layers.", l.ID, l.Kind).
			WithHint("Use an image layer.").
			WithPath("args.layer_id")
	}

	src, ok := d.Assets[l.Asset]
	if !ok {
		return command.Errorf(command.CodeInvalidState, "Layer %s has no image yet.", l.ID).
			WithHint("Fill the slot with an image first.").WithPath("args.layer_id")
	}

	if a.Reset {
		l.Crop = nil
	} else {
		if *a.X+*a.W > src.Width || *a.Y+*a.H > src.Height {
			return command.Errorf(
				command.CodeOutOfSource, "The crop %d,%d %dx%d is outside the source, which is %dx%d.",
				*a.X, *a.Y, *a.W, *a.H, src.Width, src.Height,
			).
				WithHint("Use a rectangle inside 0,0 %dx%d.", src.Width, src.Height).WithPath("args").
				WithDetail("source", map[string]int{"w": src.Width, "h": src.Height})
		}

		l.Crop = &Crop{X: *a.X, Y: *a.Y, W: *a.W, H: *a.H}
	}

	c.SetEffective("layer_id", l.ID)
	c.SetEffective("crop", cropOf(l, src))

	return nil
}

// cropOf is the rectangle of the source an image layer shows: its crop, or the whole source.
func cropOf(l *Layer, src AssetRef) Crop {
	if l.Crop != nil {
		return *l.Crop
	}

	return Crop{W: src.Width, H: src.Height}
}

type setTextArgs struct {
	LayerID    string         `json:"layer_id"              pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Text       *string        `json:"text,omitempty"                                                               max:"2000"`
	Style      map[string]any `json:"style,omitempty"                                                                         desc:"A partial TextStyle, merged; null removes a key." schema:"text_style_merge"`
	ResetStyle bool           `json:"reset_style,omitempty"                                                                   desc:"Empty the stored style before applying style."`
}

func applySetText(c *command.Context, d *Document, a *setTextArgs) error {
	if err := atLeastOne(
		[]string{"text", "style", "reset_style"},
		a.Text != nil,
		a.Style != nil,
		a.ResetStyle,
	); err != nil {
		return err
	}

	if err := command.ValidateStyle(a.Style); err != nil {
		return err
	}

	l, err := target(d, a.LayerID, "args.layer_id")
	if err != nil {
		return err
	}

	if l.Kind != KindText {
		return command.Errorf(command.CodeTypeMismatch, "Layer %s is a %s; layer.set_text changes text layers.", l.ID, l.Kind).
			WithHint("Use layer.set_shape for shapes.").
			WithPath("args.layer_id")
	}

	current := l.Style
	if a.ResetStyle {
		current = nil
	}

	style, err := command.MergeStyle(current, a.Style)
	if err != nil {
		return err
	}

	if err := command.CheckStyleFont(style); err != nil {
		return err
	}

	if style != nil && style.MaxWidth != nil {
		return invalid("args.style.max_width", "max_width is not a key of a text layer: its width is w.").
			WithHint("Remove max_width and set w with layer.transform.")
	}

	if a.Text != nil {
		l.Text = *a.Text
	}

	l.Style = style
	remeasure(envOf(c), d, l)

	c.SetEffective("layer_id", l.ID)
	c.SetEffective("box", boxOf(l))
	c.SetEffective("style", styleOrEmpty(l))

	return nil
}

type setShapeArgs struct {
	LayerID     string  `json:"layer_id"               pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Shape       string  `json:"shape,omitempty"                                                               enum:"rect,rounded_rect,ellipse"`
	Fill        string  `json:"fill,omitempty"`
	StrokeColor *string `json:"stroke_color,omitempty"                                                                                         desc:"A colour; an empty string removes it."`
	StrokeWidth *int    `json:"stroke_width,omitempty"                                                                                                                                      min:"0" max:"1000"`
	Radius      *int    `json:"radius,omitempty"                                                                                                                                            min:"0" max:"16384"`
}

func applySetShape(c *command.Context, d *Document, a *setShapeArgs) error {
	if err := atLeastOne([]string{"shape", "fill", "stroke_color", "stroke_width", "radius"},
		a.Shape != "", a.Fill != "", a.StrokeColor != nil, a.StrokeWidth != nil, a.Radius != nil); err != nil {
		return err
	}

	l, err := target(d, a.LayerID, "args.layer_id")
	if err != nil {
		return err
	}

	switch {
	case l.Kind != KindShape && l.Kind != KindFill:
		return command.Errorf(command.CodeTypeMismatch, "Layer %s is a %s; layer.set_shape changes shapes and fills.", l.ID, l.Kind).
			WithHint("Use layer.set_text for text.").
			WithPath("args.layer_id")
	case l.Kind == KindFill && (a.Shape != "" || a.StrokeColor != nil || a.StrokeWidth != nil || a.Radius != nil):
		return command.Errorf(command.CodeTypeMismatch, "Layer %s is a fill; it takes fill only.", l.ID).
			WithHint("Add a shape layer for a shape with a stroke or rounded corners.").WithPath("args.layer_id")
	}

	if a.Fill != "" {
		fill, cerr := colour("args.fill", a.Fill)
		if cerr != nil {
			return cerr
		}

		l.Fill = fill
	}

	if l.Kind == KindShape {
		if err := setShapeFields(l, a); err != nil {
			return err
		}
	}

	c.SetEffective("layer_id", l.ID)

	return nil
}

func setShapeFields(l *Layer, a *setShapeArgs) *command.Error {
	if a.Shape != "" {
		l.Shape = a.Shape
		if a.Shape != ShapeRoundedRect {
			l.Radius = 0
		}
	}

	if a.Radius != nil {
		if l.Shape != ShapeRoundedRect {
			return invalid("args.radius", "args.radius is only valid for shape %q, got %q.", ShapeRoundedRect, l.Shape).
				WithHint("Set shape rounded_rect together with radius.")
		}

		l.Radius = *a.Radius
	}

	if a.StrokeColor != nil {
		l.StrokeColor = ""

		if *a.StrokeColor != "" {
			stroke, err := colour("args.stroke_color", *a.StrokeColor)
			if err != nil {
				return err
			}

			l.StrokeColor = stroke
		}
	}

	if a.StrokeWidth != nil {
		l.StrokeWidth = *a.StrokeWidth
	}

	return nil
}

// styleOrEmpty is the stored style of a text layer as a JSON object.
func styleOrEmpty(l *Layer) map[string]any {
	if l.Style == nil {
		return map[string]any{}
	}

	return object(l.Style)
}
