package image

import (
	"github.com/AlexGalhardo/stuarty/core/command"
)

func init() {
	register(
		command.DefineIn(command.Spec{
			Name:    "layer.add_image",
			Summary: "Adds an image layer from a file of the workspace.",
			Description: "asset is the content hash of an image file already imported (media:import gives it). Without " +
				"x, y, w and h the layer is the fit (contain by default, or cover) of the whole source on the canvas, " +
				"centred. effective.layer_id names the layer.",
			Example: `{"asset": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", "fit": "cover", "name": "Photo"}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeIDExists, command.CodeLimit,
				command.CodeTypeMismatch, command.CodeInvalidState,
			},
			Scope: command.ScopeEdit,
		}, applyAddImage),
		command.DefineIn(command.Spec{
			Name:    "layer.add_text",
			Summary: "Adds a text layer.",
			Description: "The text wraps at w (default 86% of the canvas) and its box is measured, so h is the height of " +
				"the text. x defaults to centre the box on the canvas and y to centre it vertically. style is a partial " +
				"text style (font_family, font_weight, size, color, stroke_color, stroke_width, shadow_*, background_*, " +
				"align, line_height, uppercase); max_width is not a text-layer key, w is.",
			Example: `{"text": "HOW IT WORKS", "style": {"font_family": "Anton", "size": 120, "color": "#FFE600"}, "slot": "title"}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeIDExists, command.CodeLimit,
				command.CodeTypeMismatch, command.CodeInvalidState, command.CodeFontUnknown,
			},
			Scope: command.ScopeEdit,
		}, applyAddText),
		command.DefineIn(command.Spec{
			Name:    "layer.add_shape",
			Summary: "Adds a rectangle, a rounded rectangle or an ellipse.",
			Description: "w and h are pixels. x and y default to centre the shape on the canvas, fill to #FFFFFF. radius " +
				"is for rounded_rect only.",
			Example: `{"shape": "rounded_rect", "w": 600, "h": 200, "radius": 24, "fill": "#E63946"}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeIDExists, command.CodeLimit,
				command.CodeTypeMismatch, command.CodeInvalidState,
			},
			Scope: command.ScopeEdit,
		}, applyAddShape),
		command.DefineIn(command.Spec{
			Name:        "layer.add_fill",
			Summary:     "Adds a solid colour over the whole canvas.",
			Description: "A fill is a rectangle the size of the canvas that can be moved and resized like any layer.",
			Example:     `{"color": "#101820", "name": "Backdrop", "index": 0}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeIDExists, command.CodeLimit,
				command.CodeTypeMismatch, command.CodeInvalidState,
			},
			Scope: command.ScopeEdit,
		}, applyAddFill),
	)
}

// base is the part of a layer every kind has.
func base(id, kind, name, def string) Layer {
	if name == "" {
		name = def
	}

	return Layer{ID: id, Kind: kind, Name: name, Visible: true, Opacity: 100}
}

type addImageArgs struct {
	Asset  string `json:"asset"            desc:"The sha256 content hash of an image in the workspace."              pattern:"^sha256:[0-9a-f]{64}$"`
	Name   string `json:"name,omitempty"                                                                                                                                    max:"120"`
	X      *int   `json:"x,omitempty"                                                                                                                                       max:"16384" min:"-16384"`
	Y      *int   `json:"y,omitempty"                                                                                                                                       max:"16384" min:"-16384"`
	W      *int   `json:"w,omitempty"                                                                                                                                       max:"16384" min:"1"`
	H      *int   `json:"h,omitempty"                                                                                                                                       max:"16384" min:"1"`
	Fit    string `json:"fit,omitempty"    desc:"contain (default) or cover, when no box is given."                                                                                                  enum:"cover,contain"`
	Slot   string `json:"slot,omitempty"   desc:"Make the layer an image slot of a template."`
	ID     string `json:"id,omitempty"                                                                               pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Parent string `json:"parent,omitempty" desc:"A group; absent means the top level."                               pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Index  *int   `json:"index,omitempty"  desc:"Position among the siblings, 0 is the bottom; absent means on top."                                                        max:"500"   min:"0"`
}

func applyAddImage(c *command.Context, d *Document, a *addImageArgs) error {
	env := envOf(c)

	if !hashPattern.MatchString(a.Asset) {
		return invalid("args.asset", "args.asset %q is not a content hash.", a.Asset).
			WithHint("A content hash is sha256: and 64 lower case hexadecimal digits.")
	}

	info, ok := lookup(env, a.Asset)
	if !ok {
		return command.Errorf(command.CodeNotFound, "The file %s is not a ready image of the workspace.", a.Asset).
			WithHint("Import the image with media:import and use the hash it returns.").
			WithDetail("kind", "asset").WithDetail("id", a.Asset).WithPath("args.asset")
	}

	if _, known := d.Assets[a.Asset]; !known {
		if err := limit("images", len(d.Assets), MaxAssets); err != nil {
			return err
		}
	}

	id, err := newLayerID(c, d, a.ID)
	if err != nil {
		return err
	}

	l := base(id, KindImage, a.Name, info.Name)
	l.Asset = a.Asset

	if a.Slot != "" {
		if err := slotName(d, a.Slot, id, "args.slot"); err != nil {
			return err
		}

		l.Slot = a.Slot
	}

	box := fitBox(info.Width, info.Height, 0, 0, d.Width, d.Height, a.Fit != "cover")
	l.X, l.Y, l.W, l.H = box.x, box.y, box.w, box.h

	if a.X != nil || a.Y != nil || a.W != nil || a.H != nil {
		l.X, l.Y = orElse(a.X, 0), orElse(a.Y, 0)
		l.W, l.H = orElse(a.W, info.Width), orElse(a.H, info.Height)
	}

	if d.Assets == nil {
		d.Assets = map[string]AssetRef{}
	}

	d.Assets[a.Asset] = AssetRef{Hash: a.Asset, Name: info.Name, Width: info.Width, Height: info.Height}

	if err := place(d, l, a.Parent, a.Index); err != nil {
		return err
	}

	c.SetEffective("layer_id", id)
	c.SetEffective("box", boxOf(&l))

	return nil
}

func lookup(env *Env, hash string) (AssetInfo, bool) {
	if env.Asset == nil {
		return AssetInfo{}, false
	}

	info, ok := env.Asset(hash)
	if !ok || info.Kind == AssetLUT || info.Width < 1 || info.Height < 1 {
		return AssetInfo{}, false
	}

	if info.Name == "" {
		info.Name = "Image"
	}

	return info, true
}

func orElse(p *int, def int) int {
	if p == nil {
		return def
	}

	return *p
}

type addTextArgs struct {
	Text   string         `json:"text"             desc:"Up to 2000 characters; an empty text is an empty slot."             max:"2000"`
	Name   string         `json:"name,omitempty"                                                                             max:"120"`
	X      *int           `json:"x,omitempty"                                                                                max:"16384" min:"-16384"`
	Y      *int           `json:"y,omitempty"                                                                                max:"16384" min:"-16384"`
	W      *int           `json:"w,omitempty"      desc:"The width the text wraps at; default 86% of the canvas."            max:"16384" min:"1"`
	Style  map[string]any `json:"style,omitempty"  desc:"A partial TextStyle."                                                                        schema:"text_style_merge"`
	Slot   string         `json:"slot,omitempty"   desc:"Make the layer a text slot of a template."`
	ID     string         `json:"id,omitempty"                                                                                                                                  pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Parent string         `json:"parent,omitempty" desc:"A group; absent means the top level."                                                                                  pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Index  *int           `json:"index,omitempty"  desc:"Position among the siblings, 0 is the bottom; absent means on top." max:"500"   min:"0"`
}

func applyAddText(c *command.Context, d *Document, a *addTextArgs) error {
	if err := command.ValidateStyle(a.Style); err != nil {
		return err
	}

	style, err := command.MergeStyle(nil, a.Style)
	if err != nil {
		return err
	}

	if err := command.CheckStyleFont(style); err != nil {
		return err
	}

	if style != nil && style.MaxWidth != nil {
		return invalid("args.style.max_width", "max_width is not a key of a text layer: its width is w.").
			WithHint("Remove max_width and set w.")
	}

	id, err := newLayerID(c, d, a.ID)
	if err != nil {
		return err
	}

	l := base(id, KindText, a.Name, "Text")
	l.Text, l.Style = a.Text, style
	l.W = orElse(a.W, roundDiv(86*d.Width, 100))

	if a.Slot != "" {
		if err := slotName(d, a.Slot, id, "args.slot"); err != nil {
			return err
		}

		l.Slot = a.Slot
	}

	remeasure(envOf(c), d, &l)

	l.X = orElse(a.X, floorDiv(d.Width-l.W, 2))
	l.Y = orElse(a.Y, floorDiv(d.Height-l.H, 2))

	if err := place(d, l, a.Parent, a.Index); err != nil {
		return err
	}

	c.SetEffective("layer_id", id)
	c.SetEffective("box", boxOf(&l))

	return nil
}

type addShapeArgs struct {
	Shape       string `json:"shape"                  enum:"rect,rounded_rect,ellipse"`
	W           int    `json:"w"                                                       desc:"Pixels, 1 to 16384."                                                min:"1"      max:"16384"`
	H           int    `json:"h"                                                       desc:"Pixels, 1 to 16384."                                                min:"1"      max:"16384"`
	X           *int   `json:"x,omitempty"                                                                                                                       min:"-16384" max:"16384"`
	Y           *int   `json:"y,omitempty"                                                                                                                       min:"-16384" max:"16384"`
	Fill        string `json:"fill,omitempty"                                          desc:"A colour; default #FFFFFF."`
	StrokeColor string `json:"stroke_color,omitempty"                                  desc:"A colour; default none."`
	StrokeWidth *int   `json:"stroke_width,omitempty"                                  desc:"Pixels, 0 or more."                                                 min:"0"      max:"1000"`
	Radius      *int   `json:"radius,omitempty"                                        desc:"Pixels; rounded_rect only."                                         min:"0"      max:"16384"`
	Name        string `json:"name,omitempty"                                                                                                                                 max:"120"`
	ID          string `json:"id,omitempty"                                                                                                                                               pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Parent      string `json:"parent,omitempty"                                        desc:"A group; absent means the top level."                                                        pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Index       *int   `json:"index,omitempty"                                         desc:"Position among the siblings, 0 is the bottom; absent means on top." min:"0"      max:"500"`
}

func applyAddShape(c *command.Context, d *Document, a *addShapeArgs) error {
	if a.Radius != nil && a.Shape != ShapeRoundedRect {
		return invalid("args.radius", "args.radius is only valid for shape %q, got %q.", ShapeRoundedRect, a.Shape).
			WithHint("Remove radius, or use shape rounded_rect.")
	}

	fill, err := colourOr("args.fill", a.Fill, DefaultBackground)
	if err != nil {
		return err
	}

	var stroke string
	if a.StrokeColor != "" {
		if stroke, err = colour("args.stroke_color", a.StrokeColor); err != nil {
			return err
		}
	}

	id, err := newLayerID(c, d, a.ID)
	if err != nil {
		return err
	}

	l := base(id, KindShape, a.Name, "Shape")
	l.Shape, l.W, l.H, l.Fill, l.StrokeColor = a.Shape, a.W, a.H, fill, stroke
	l.StrokeWidth, l.Radius = orElse(a.StrokeWidth, 0), orElse(a.Radius, 0)
	l.X = orElse(a.X, floorDiv(d.Width-a.W, 2))
	l.Y = orElse(a.Y, floorDiv(d.Height-a.H, 2))

	if err := place(d, l, a.Parent, a.Index); err != nil {
		return err
	}

	c.SetEffective("layer_id", id)
	c.SetEffective("box", boxOf(&l))

	return nil
}

func colourOr(path, v, def string) (string, *command.Error) {
	if v == "" {
		return def, nil
	}

	return colour(path, v)
}

type addFillArgs struct {
	Color  string `json:"color"`
	Name   string `json:"name,omitempty"   max:"120"`
	ID     string `json:"id,omitempty"               pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Parent string `json:"parent,omitempty"           pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$" desc:"A group; absent means the top level."`
	Index  *int   `json:"index,omitempty"  max:"500"                                                        desc:"Position among the siblings, 0 is the bottom; absent means on top." min:"0"`
}

func applyAddFill(c *command.Context, d *Document, a *addFillArgs) error {
	fill, err := colour("args.color", a.Color)
	if err != nil {
		return err
	}

	id, err := newLayerID(c, d, a.ID)
	if err != nil {
		return err
	}

	l := base(id, KindFill, a.Name, "Fill")
	l.Fill, l.W, l.H = fill, d.Width, d.Height

	if err := place(d, l, a.Parent, a.Index); err != nil {
		return err
	}

	c.SetEffective("layer_id", id)
	c.SetEffective("box", boxOf(&l))

	return nil
}
