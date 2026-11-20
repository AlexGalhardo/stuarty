package command

import (
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "shape.add",
		Summary: "Adds a rectangle, rounded rectangle or ellipse to a visual track.",
		Description: "Adds a shape item with the given size in pixels before scale. Colours are hex or a template " +
			"colour name like \"$accent\"; radius is for rounded_rect only. x and y default to the frame centre.",
		Example: `{"track_id": "trk_f6g7h8j9k0", "start": "0s", "duration": "3s", "shape": "rect", ` +
			`"width": 400, "height": 200, "fill": "#ff0000"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeIDExists, CodeLimit, CodeOverlap,
		},
		Scope: ScopeEdit,
	}, applyShapeAdd))

	Register(Define(Spec{
		Name:    "shape.set",
		Summary: "Changes the shape, size, colours, stroke or radius of a shape item.",
		Description: "Sets the given fields; a value equal to its default clears the stored field. Changing the shape " +
			"to rect or ellipse removes a stored radius. Position and transform are set with item.set_transform.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "shape": "rounded_rect", "radius": 24, "width": 600}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch},
		Scope:   ScopeEdit,
	}, applyShapeSet))
}

type shapeAddArgs struct {
	TrackID     string   `json:"track_id,omitempty"     desc:"A visual track; omitted, the highest visual track free over the item (an empty track you made for video counts; pass track_id to keep it clear), or a new one on top. effective.track_id names it." pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Start       Time     `json:"start"`
	Duration    Time     `json:"duration"               desc:"A length of at least 1 frame."`
	Shape       string   `json:"shape"                                                                                                                                                                                                                                                                                   enum:"rect,rounded_rect,ellipse"`
	Width       float64  `json:"width"                  desc:"Pixels before scale, above 0 and at most 32768."`
	Height      float64  `json:"height"                 desc:"Pixels before scale, above 0 and at most 32768."`
	Fill        string   `json:"fill,omitempty"         desc:"A colour; default #FFFFFF."`
	StrokeColor string   `json:"stroke_color,omitempty" desc:"A colour; default #000000."`
	StrokeWidth *float64 `json:"stroke_width,omitempty" desc:"Pixels, 0 or more."`
	Radius      *float64 `json:"radius,omitempty"       desc:"Pixels, 0 or more; rounded_rect only."`
	X           *float64 `json:"x,omitempty"`
	Y           *float64 `json:"y,omitempty"`
	ID          string   `json:"id,omitempty"                                                                                                                                                                                                                     pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

// shapeColour checks a colour argument and returns its stored form: a hex colour in
// upper case, a "$name" reference as written.
func (c *Context) shapeColour(path, v string) (timeline.Color, *Error) {
	if err := c.checkColour(path, v); err != nil {
		return "", err
	}
	if strings.HasPrefix(v, "#") {
		v = strings.ToUpper(v)
	}
	return timeline.Color(v), nil
}

// maxShapeSize is the largest width or height of a shape, in pixels before scale: a
// wider shape cannot be composed and its export fails (VV-088).
const maxShapeSize = 32768

// positive checks a size that must be above 0 once rounded to 3 decimals and at most maxShapeSize.
func positive(path string, v float64) *Error {
	if err := finite(path, &v); err != nil {
		return err
	}
	if round3(v) <= 0 {
		return invalidArg(path, "%s must be greater than 0, got %s.", path, trimFloat(v)).
			WithHint("Use a value of at least 1.")
	}
	if v > maxShapeSize {
		return invalidArg(path, "%s must be at most %d, got %s.", path, maxShapeSize, trimFloat(v)).
			WithHint("Use a value up to 32768 pixels, and scale the item for a larger look.")
	}
	return nil
}

// notNegative checks an optional measure that must be 0 or more.
func notNegative(path string, v *float64) *Error {
	if err := finite(path, v); err != nil {
		return err
	}
	if v != nil && *v < 0 {
		return invalidArg(path, "%s must be 0 or more, got %s.", path, trimFloat(*v)).
			WithHint("Use 0 or a positive number.")
	}
	return nil
}

func radiusShape(shape string, radius *float64) *Error {
	if radius == nil || shape == "rounded_rect" {
		return nil
	}
	return invalidArg("args.radius", "args.radius is only valid for shape \"rounded_rect\", got %q.", shape).
		WithHint("Remove radius, or use shape \"rounded_rect\".")
}

func applyShapeAdd(c *Context, a *shapeAddArgs) error {
	for _, f := range []struct{ path, prefix, id string }{
		{"args.track_id", timeline.PrefixTrack, a.TrackID}, {"args.id", timeline.PrefixItem, a.ID},
	} {
		if err := idForm(f.path, f.prefix, f.id); err != nil {
			return err
		}
	}
	if err := radiusShape(a.Shape, a.Radius); err != nil {
		return err
	}
	for _, f := range []struct {
		path string
		v    float64
	}{{"args.width", a.Width}, {"args.height", a.Height}} {
		if err := positive(f.path, f.v); err != nil {
			return err
		}
	}
	for _, f := range []struct {
		path string
		v    *float64
	}{{"args.stroke_width", a.StrokeWidth}, {"args.radius", a.Radius}} {
		if err := notNegative(f.path, f.v); err != nil {
			return err
		}
	}
	for _, f := range []struct {
		path string
		v    *float64
	}{{"args.x", a.X}, {"args.y", a.Y}} {
		if err := finite(f.path, f.v); err != nil {
			return err
		}
	}
	fill, err := c.optionalColour("args.fill", a.Fill)
	if err != nil {
		return err
	}
	stroke, err := c.optionalColour("args.stroke_color", a.StrokeColor)
	if err != nil {
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
		return Errorf(CodeTypeMismatch, "Track %s is an audio track; shapes need a visual track.", a.TrackID).
			WithHint("Use a visual track.").WithPath("args.track_id")
	}
	id, err := c.newID("items", timeline.PrefixItem, a.ID)
	if err != nil {
		return err
	}
	if err := c.limit("items", len(c.Doc.Items), timeline.MaxItems); err != nil {
		return err
	}
	if err := c.checkEnd("args.duration", start, duration); err != nil {
		return err
	}
	if err := c.checkOverlap(id, a.TrackID, start, duration); err != nil {
		return err
	}
	it := timeline.NewItem(id, timeline.ItemShape, a.TrackID, start, duration)
	it.Shape, it.Width, it.Height = a.Shape, round3(a.Width), round3(a.Height)
	if fill != "" {
		it.Fill = fill
	}
	if stroke != "" {
		it.StrokeColor = stroke
	}
	it.StrokeWidth, it.Radius = roundedOrZero(a.StrokeWidth), roundedOrZero(a.Radius)
	it.X, it.Y = roundedPtr(a.X), roundedPtr(a.Y)
	c.Doc.Items[id] = it
	c.SetEffective("id", id)
	c.SetEffectiveTime("start", start, string(a.Start))
	c.SetEffectiveTime("duration", duration, string(a.Duration))
	return nil
}

func (c *Context) optionalColour(path, v string) (timeline.Color, *Error) {
	if v == "" {
		return "", nil
	}
	return c.shapeColour(path, v)
}

func roundedOrZero(v *float64) float64 {
	if v == nil {
		return 0
	}
	return round3(*v)
}

func roundedPtr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	r := round3(*v)
	return &r
}

type shapeSetArgs struct {
	ItemID      string   `json:"item_id"                desc:"A shape item."                                   pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Shape       string   `json:"shape,omitempty"                                                                                                                      enum:"rect,rounded_rect,ellipse"`
	Width       *float64 `json:"width,omitempty"        desc:"Pixels before scale, above 0 and at most 32768."`
	Height      *float64 `json:"height,omitempty"       desc:"Pixels before scale, above 0 and at most 32768."`
	Fill        string   `json:"fill,omitempty"         desc:"A colour."`
	StrokeColor string   `json:"stroke_color,omitempty" desc:"A colour."`
	StrokeWidth *float64 `json:"stroke_width,omitempty" desc:"Pixels, 0 or more."`
	Radius      *float64 `json:"radius,omitempty"       desc:"Pixels, 0 or more; rounded_rect only."`
}

func applyShapeSet(c *Context, a *shapeSetArgs) error {
	if err := atLeastOne(
		[]string{"shape", "width", "height", "fill", "stroke_color", "stroke_width", "radius"},
		a.Shape != "", a.Width != nil, a.Height != nil, a.Fill != "", a.StrokeColor != "",
		a.StrokeWidth != nil, a.Radius != nil,
	); err != nil {
		return err
	}
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	for _, f := range []struct {
		path string
		v    *float64
	}{{"args.width", a.Width}, {"args.height", a.Height}} {
		if f.v != nil {
			if err := positive(f.path, *f.v); err != nil {
				return err
			}
		}
	}
	for _, f := range []struct {
		path string
		v    *float64
	}{{"args.stroke_width", a.StrokeWidth}, {"args.radius", a.Radius}} {
		if err := notNegative(f.path, f.v); err != nil {
			return err
		}
	}
	fill, err := c.optionalColour("args.fill", a.Fill)
	if err != nil {
		return err
	}
	stroke, err := c.optionalColour("args.stroke_color", a.StrokeColor)
	if err != nil {
		return err
	}
	if a.Shape != "" {
		if err := radiusShape(a.Shape, a.Radius); err != nil {
			return err
		}
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if it.Type != timeline.ItemShape {
		return Errorf(CodeTypeMismatch, "Item %s is %s; shape.set changes shapes.", a.ItemID, article(it.Type)).
			WithHint("Use the command for that item type.").WithPath("args.item_id")
	}
	if a.Shape == "" {
		if err := radiusShape(it.Shape, a.Radius); err != nil {
			return err
		}
	}
	if a.Shape != "" {
		it.Shape = a.Shape
		if a.Shape != "rounded_rect" {
			it.Radius = 0
		}
		c.SetEffective("shape", a.Shape)
	}
	for _, f := range []struct {
		name string
		v    *float64
		to   *float64
	}{
		{"width", a.Width, &it.Width},
		{"height", a.Height, &it.Height},
		{"stroke_width", a.StrokeWidth, &it.StrokeWidth},
		{"radius", a.Radius, &it.Radius},
	} {
		if f.v != nil {
			*f.to = round3(*f.v)
			c.SetEffective(f.name, *f.to)
		}
	}
	if fill != "" {
		it.Fill = fill
		c.SetEffective("fill", fill)
	}
	if stroke != "" {
		it.StrokeColor = stroke
		c.SetEffective("stroke_color", stroke)
	}
	c.Doc.Items[a.ItemID] = it
	return nil
}
