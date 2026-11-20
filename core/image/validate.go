package image

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Violation is a way in which a document is not valid.
type Violation struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Layer   string `json:"layer,omitempty"`
	Message string `json:"message"`
}

var (
	hexColour   = regexp.MustCompile(`^#([0-9A-F]{6}|[0-9A-F]{8})$`)
	slotPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	hashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// SlotPattern is the form of a slot name.
const SlotPattern = `^[a-z][a-z0-9_]{0,39}$`

// ValidColour reports whether s is a stored colour: upper case #RRGGBB or #RRGGBBAA.
func ValidColour(s string) bool { return hexColour.MatchString(s) }

// Validate lists every way in which the document breaks the invariants of SDD-021 section 2; an empty list is
// a valid document. The batch runs it on every result.
func Validate(d *Document) []Violation {
	v := &validator{d: d, ids: map[string]bool{}, slots: map[string]bool{}}
	v.document()

	v.layers(d.Layers, "layers", "", 0)

	v.maskSources()

	if n := Count(d); n > MaxLayers {
		v.add("limit", "layers", "", "the document has %d layers; the limit is %d", n, MaxLayers)
	}

	for h, a := range d.Assets {
		sized := a.Kind == "" || a.Kind == AssetImage

		if !hashPattern.MatchString(h) || a.Hash != h || (!sized && !a.isLUT()) ||
			(sized && (a.Width < 1 || a.Height < 1)) || (a.isLUT() && (a.Width != 0 || a.Height != 0)) {
			v.add("asset", "assets."+h, "", "the asset entry %q is not valid", h)
		}
	}

	if len(d.Assets) > MaxAssets {
		v.add("limit", "assets", "", "the document uses %d files; the limit is %d", len(d.Assets), MaxAssets)
	}

	images, luts := usedAssets(d)

	for h := range images {
		if a, ok := d.Assets[h]; !ok {
			v.add("asset", "assets", "", "an image layer uses %s, which is not in assets", h)
		} else if a.isLUT() {
			v.add("asset", "assets."+h, "", "an image layer uses %s, which is a lookup table", h)
		}
	}

	for h := range luts {
		if a, ok := d.Assets[h]; !ok {
			v.add("asset", "assets", "", "a layer uses the lookup table %s, which is not in assets", h)
		} else if !a.isLUT() {
			v.add("asset", "assets."+h, "", "a layer uses %s as a lookup table, and it is an image", h)
		}
	}

	for h, a := range d.Assets {
		if (a.isLUT() && !luts[h]) || (!a.isLUT() && !images[h]) {
			v.add("asset", "assets."+h, "", "assets lists %s, which no layer uses", h)
		}
	}

	return v.vs
}

type validator struct {
	d     *Document
	vs    []Violation
	ids   map[string]bool
	slots map[string]bool
}

func (v *validator) add(code, path, layer, format string, a ...any) {
	v.vs = append(v.vs, Violation{Code: code, Path: path, Layer: layer, Message: fmt.Sprintf(format, a...)})
}

func (v *validator) document() {
	d := v.d

	switch {
	case d.SchemaVersion != SchemaVersion:
		v.add(
			"schema",
			"schema_version",
			"",
			"schema_version is %d; this build reads %d",
			d.SchemaVersion,
			SchemaVersion,
		)
	case !timeline.ValidID(PrefixImage, d.ID):
		v.add("id", "id", "", "the document id %q is not an img_ identifier", d.ID)
	case d.Width < 1 || d.Width > MaxCanvas || d.Height < 1 || d.Height > MaxCanvas:
		v.add("canvas", "width", "", "the canvas is %dx%d; each side must be 1 to %d", d.Width, d.Height, MaxCanvas)
	case d.Width*d.Height > MaxCanvasPixels:
		v.add("canvas", "width", "", "the canvas has %d pixels; the limit is %d", d.Width*d.Height, MaxCanvasPixels)
	case d.Background != Transparent && !ValidColour(d.Background):
		v.add("colour", "background", "", "the background %q is not a colour or transparent", d.Background)
	case len([]rune(d.Name)) > MaxName:
		v.add("name", "name", "", "the name is longer than %d characters", MaxName)
	}
}

func (v *validator) layers(ls []Layer, path, parent string, depth int) {
	if depth >= MaxDepth && len(ls) > 0 {
		v.add("depth", path, parent, "groups are nested deeper than %d levels", MaxDepth)

		return
	}

	prev := ""

	for i := range ls {
		l := &ls[i]
		p := fmt.Sprintf("%s[%d]", path, i)

		if !timeline.ValidID(PrefixLayer, l.ID) {
			v.add("id", p+".id", l.ID, "the layer id %q is not a lyr_ identifier", l.ID)
		} else if v.ids[l.ID] {
			v.add("id", p+".id", l.ID, "the layer id %s is used twice", l.ID)
		}

		v.ids[l.ID] = true

		if !timeline.ValidOrder(l.Order) || l.Order <= prev {
			v.add("order", p+".order", l.ID, "the order key %q does not sort after the layer before it", l.Order)
		}

		prev = l.Order

		v.layer(l, p)
		v.layers(l.Children, p+".children", l.ID, depth+1)
	}
}

func (v *validator) layer(l *Layer, p string) {
	if len([]rune(l.Name)) > MaxName {
		v.add("name", p+".name", l.ID, "the name is longer than %d characters", MaxName)
	}

	if l.Opacity < 0 || l.Opacity > 100 {
		v.add("opacity", p+".opacity", l.ID, "opacity is %d; it must be 0 to 100", l.Opacity)
	}

	if l.Rotation < -360 || l.Rotation > 360 {
		v.add("rotation", p+".rotation", l.ID, "rotation is %d; it must be -360 to 360", l.Rotation)
	}

	if abs(l.X) > MaxCoord || abs(l.Y) > MaxCoord {
		v.add("position", p, l.ID, "the position %d,%d is outside -%d to %d", l.X, l.Y, MaxCoord, MaxCoord)
	}

	if l.Slot != "" {
		v.slot(l, p)
	}

	v.look(l, p)

	if l.Kind != KindGroup {
		if l.W < 1 || l.H < 1 || l.W > MaxBoxSide || l.H > MaxBoxSide {
			v.add("size", p, l.ID, "the size %dx%d must be 1 to %d on each side", l.W, l.H, MaxBoxSide)
		}

		if len(l.Children) > 0 {
			v.add("kind", p+".children", l.ID, "only a group has children")
		}
	}

	switch l.Kind {
	case KindImage:
		v.image(l, p)
	case KindText:
		v.text(l, p)
	case KindShape:
		v.shape(l, p)
	case KindFill:
		if !ValidColour(l.Fill) {
			v.add("colour", p+".fill", l.ID, "the fill %q is not a colour", l.Fill)
		}
	case KindGroup:
		if l.W != 0 || l.H != 0 || l.Rotation != 0 {
			v.add("kind", p, l.ID, "a group has no size and no rotation, only an offset")
		}
	default:
		v.add("kind", p+".kind", l.ID, "the kind %q is not image, text, shape, fill or group", l.Kind)
	}
}

func (v *validator) slot(l *Layer, p string) {
	switch {
	case l.Kind != KindText && l.Kind != KindImage:
		v.add("slot", p+".slot", l.ID, "only a text or an image layer can be a slot")
	case !slotPattern.MatchString(l.Slot):
		v.add("slot", p+".slot", l.ID, "the slot name %q does not match %s", l.Slot, SlotPattern)
	case v.slots[l.Slot]:
		v.add("slot", p+".slot", l.ID, "the slot name %q is used twice", l.Slot)
	}

	v.slots[l.Slot] = true
}

func (v *validator) image(l *Layer, p string) {
	if l.Asset == "" {
		return // an image slot of a template that has no file yet
	}

	a, ok := v.d.Assets[l.Asset]
	if !ok {
		return // reported once for the document
	}

	if c := l.Crop; c != nil {
		if c.W < 1 || c.H < 1 || c.X < 0 || c.Y < 0 || c.X+c.W > a.Width || c.Y+c.H > a.Height {
			v.add("crop", p+".crop", l.ID, "the crop %d,%d %dx%d is not inside the source %dx%d",
				c.X, c.Y, c.W, c.H, a.Width, a.Height)
		}
	}
}

func (v *validator) text(l *Layer, p string) {
	if len([]rune(l.Text)) > MaxText {
		v.add("text", p+".text", l.ID, "the text is longer than %d characters", MaxText)
	}

	if l.Style == nil {
		return
	}

	for _, sv := range timeline.StyleViolations(l.Style) {
		v.add("style", p+".style", l.ID, "%s", strings.TrimSpace(sv.Message))
	}

	if l.Style.MaxWidth != nil {
		v.add("style", p+".style.max_width", l.ID, "max_width is not stored for a text layer: its width is w")
	}
}

func (v *validator) shape(l *Layer, p string) {
	if !slices.Contains([]string{ShapeRect, ShapeRoundedRect, ShapeEllipse}, l.Shape) {
		v.add("shape", p+".shape", l.ID, "the shape %q is not rect, rounded_rect or ellipse", l.Shape)
	}

	if !ValidColour(l.Fill) {
		v.add("colour", p+".fill", l.ID, "the fill %q is not a colour", l.Fill)
	}

	if l.StrokeColor != "" && !ValidColour(l.StrokeColor) {
		v.add("colour", p+".stroke_color", l.ID, "the stroke colour %q is not a colour", l.StrokeColor)
	}

	if l.StrokeWidth < 0 || l.Radius < 0 {
		v.add("shape", p, l.ID, "stroke_width and radius cannot be negative")
	}

	if l.Radius != 0 && l.Shape != ShapeRoundedRect {
		v.add("shape", p+".radius", l.ID, "radius is only valid for a rounded_rect")
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}

	return n
}
