package image

import (
	"slices"
	"strconv"

	"github.com/AlexGalhardo/stuarty/core/command"
)

func init() {
	register(
		command.DefineIn(command.Spec{
			Name:    "layer.align",
			Summary: "Aligns layers to the canvas or to another layer.",
			Description: "to is canvas or a layer id. horizontal puts the left edge, the centre or the right edge of each " +
				"layer on that of the target; vertical the top, the middle or the bottom. The box of a group is the union " +
				"of its children; rotation is ignored. At least one axis.",
			Example: `{"layer_ids": ["lyr_a1b2c3d4e5"], "to": "canvas", "horizontal": "center", "vertical": "middle"}`,
			Errors:  []string{command.CodeValidation, command.CodeNotFound, command.CodeInvalidState},
			Scope:   command.ScopeEdit,
		}, applyAlign),
		command.DefineIn(command.Spec{
			Name:    "layer.fit",
			Summary: "Scales a layer to cover or to be contained in the canvas or another layer.",
			Description: "The aspect of an image layer is that of its crop (or of the source), of a shape or a fill that of " +
				"its box. The result is centred on the target. A text layer and a group are sized with layer.transform.",
			Example: `{"layer_id": "lyr_a1b2c3d4e5", "mode": "cover"}`,
			Errors: []string{
				command.CodeValidation, command.CodeNotFound, command.CodeTypeMismatch, command.CodeInvalidState,
			},
			Scope: command.ScopeEdit,
		}, applyFit),
	)
}

// extent is the box a layer covers in the coordinates of its parent: its own, or for a group the union of its
// children moved by its offset. ok is false for a group with nothing in it.
func extent(l *Layer) (box, bool) {
	if l.Kind != KindGroup {
		return box{l.X, l.Y, l.W, l.H}, true
	}

	var (
		out   box
		found bool
	)

	for i := range l.Children {
		b, ok := extent(&l.Children[i])
		if !ok {
			continue
		}

		if !found {
			out, found = b, true

			continue
		}

		x0, y0 := min(out.x, b.x), min(out.y, b.y)
		x1, y1 := max(out.x+out.w, b.x+b.w), max(out.y+out.h, b.y+b.h)
		out = box{x0, y0, x1 - x0, y1 - y0}
	}

	if !found {
		return box{}, false
	}

	out.x += l.X
	out.y += l.Y

	return out, true
}

// absolute is the box of a layer on the canvas.
func absolute(d *Document, l *Layer) (box, bool) {
	b, ok := extent(l)
	if !ok {
		return box{}, false
	}

	for _, g := range ancestors(d, l.ID) {
		b.x += g.X
		b.y += g.Y
	}

	return b, true
}

// targetBox is the box of "canvas" or of a layer, on the canvas.
func targetBox(d *Document, to, path string) (box, *command.Error) {
	if to == "" || to == "canvas" {
		return box{0, 0, d.Width, d.Height}, nil
	}

	t, err := reference(d, to, path)
	if err != nil {
		return box{}, err
	}

	b, ok := absolute(d, t)
	if !ok {
		return box{}, command.Errorf(command.CodeInvalidState, "Layer %s has nothing in it, so it has no box.", to).
			WithHint("Add a layer to the group, or align to the canvas.").WithPath(path)
	}

	return b, nil
}

type alignArgs struct {
	LayerIDs   []string `json:"layer_ids"            min:"1" max:"100" pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	To         string   `json:"to"                                                                                            desc:"canvas, or a layer id."`
	Horizontal string   `json:"horizontal,omitempty"                                                                                                        enum:"left,center,right"`
	Vertical   string   `json:"vertical,omitempty"                                                                                                          enum:"top,middle,bottom"`
}

func applyAlign(c *command.Context, d *Document, a *alignArgs) error {
	if err := atLeastOne([]string{"horizontal", "vertical"}, a.Horizontal != "", a.Vertical != ""); err != nil {
		return err
	}

	if a.To != "canvas" {
		if err := idForm("args.to", a.To); err != nil {
			return err
		}

		if slices.Contains(a.LayerIDs, a.To) {
			return invalid("args.to", "Layer %s cannot be aligned to itself.", a.To).
				WithHint("Take it out of layer_ids, or align to the canvas.")
		}
	}

	to, err := targetBox(d, a.To, "args.to")
	if err != nil {
		return err
	}

	boxes := map[string]map[string]int{}

	for i, id := range a.LayerIDs {
		path := "args.layer_ids[" + strconv.Itoa(i) + "]"

		l, err := target(d, id, path)
		if err != nil {
			return err
		}

		b, ok := absolute(d, l)
		if !ok {
			return command.Errorf(command.CodeInvalidState, "Layer %s has nothing in it, so it has no box.", id).
				WithHint("Add a layer to the group first.").WithPath(path)
		}

		l.X += deltaX(a.Horizontal, b, to)
		l.Y += deltaY(a.Vertical, b, to)

		if moved, ok := absolute(d, l); ok {
			boxes[id] = map[string]int{"x": moved.x, "y": moved.y, "w": moved.w, "h": moved.h}
		}
	}

	c.SetEffective("layer_ids", a.LayerIDs)
	c.SetEffective("boxes", boxes)

	return nil
}

func deltaX(mode string, b, to box) int {
	switch mode {
	case "left":
		return to.x - b.x
	case "center":
		return floorDiv(2*to.x+to.w-2*b.x-b.w, 2)
	case "right":
		return to.x + to.w - (b.x + b.w)
	}

	return 0
}

func deltaY(mode string, b, to box) int {
	switch mode {
	case "top":
		return to.y - b.y
	case "middle":
		return floorDiv(2*to.y+to.h-2*b.y-b.h, 2)
	case "bottom":
		return to.y + to.h - (b.y + b.h)
	}

	return 0
}

type fitArgs struct {
	LayerID string `json:"layer_id"     pattern:"^lyr_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Mode    string `json:"mode"                                                                enum:"cover,contain"`
	To      string `json:"to,omitempty"                                                                             desc:"canvas (default), or a layer id."`
}

func applyFit(c *command.Context, d *Document, a *fitArgs) error {
	l, err := target(d, a.LayerID, "args.layer_id")
	if err != nil {
		return err
	}

	if a.To != "" && a.To != "canvas" {
		if err := idForm("args.to", a.To); err != nil {
			return err
		}

		if a.To == l.ID {
			return invalid("args.to", "Layer %s cannot be fitted to itself.", l.ID).WithHint("Fit it to the canvas.")
		}
	}

	to, terr := targetBox(d, a.To, "args.to")
	if terr != nil {
		return terr
	}

	var sw, sh int

	switch l.Kind {
	case KindImage:
		src, ok := d.Assets[l.Asset]
		if !ok {
			return command.Errorf(command.CodeInvalidState, "Layer %s has no image yet.", l.ID).
				WithHint("Fill the slot with an image first.").WithPath("args.layer_id")
		}

		cr := cropOf(l, src)
		sw, sh = cr.W, cr.H
	case KindShape, KindFill:
		sw, sh = l.W, l.H
	default:
		return command.Errorf(command.CodeTypeMismatch, "Layer %s is a %s; layer.fit changes images, shapes and fills.", l.ID, l.Kind).
			WithHint("Size a text layer or a group with layer.transform.").
			WithPath("args.layer_id")
	}

	// the target is on the canvas and the layer is placed in the coordinates of its parent
	var dx, dy int
	for _, g := range ancestors(d, l.ID) {
		dx += g.X
		dy += g.Y
	}

	b := fitBox(sw, sh, to.x-dx, to.y-dy, to.w, to.h, a.Mode == "contain")
	l.X, l.Y, l.W, l.H = b.x, b.y, b.w, b.h

	c.SetEffective("layer_id", l.ID)
	c.SetEffective("box", boxOf(l))

	return nil
}
