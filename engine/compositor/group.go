package compositor

import (
	"fmt"
	"image"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// MaxSurface is the largest side of the surface of a group, in pixels: the largest frame a renderer draws
// (SDD-013 section 7.2).
const MaxSurface = 4096

// MaxGroupDepth is the deepest nesting of groups an executor draws (SDD-001 section 3.5.1).
const MaxGroupDepth = 5

// GroupGeometry is the scale at which the nested frame of a group is drawn and the size of its surface
// (SDD-013 section 3.12 steps 1 and 2): the factor from a nested pixel to an output pixel, lowered so that no
// side passes MaxSurface, and the surface it makes. layerW is the drawn width of the group layer in project
// pixels; the surface is the size the group would be drawn at, so that it is copied and not resampled.
func GroupGeometry(g *plan.GroupSpec, layerW, outScale float64) (k float64, w, h int) {
	if g == nil || g.Width < 1 || g.Height < 1 || !(layerW > 0) || !(outScale > 0) {
		return 0, 0, 0
	}

	k = outScale * layerW / float64(g.Width)
	k = min(k, float64(MaxSurface)/float64(g.Width), float64(MaxSurface)/float64(g.Height))

	return k, max(round(float64(g.Width)*k), 1), max(round(float64(g.Height)*k), 1)
}

// groupSurface draws the nested frame of a group layer on a surface of its own: clear where nothing is drawn
// and premultiplied, ready to be drawn as the pixels of a media layer (SDD-013 section 3.12).
func (c *Compositor) groupSurface(l *plan.Layer, outScale float64) (Image, error) {
	g := l.Group
	if g == nil {
		return Image{}, fmt.Errorf("%w: a group layer has no frame", ErrSource)
	}

	k, w, h := GroupGeometry(g, l.W, outScale)
	if w < 1 {
		return Image{}, nil
	}

	if c.groupDepth >= MaxGroupDepth {
		return Image{}, fmt.Errorf("%w: groups nested more than %d deep", ErrSource, MaxGroupDepth)
	}

	c.groupDepth++
	defer func() { c.groupDepth-- }()

	if c.Scoped != nil {
		defer c.Scoped(k)()
	}

	surface := image.NewRGBA(image.Rect(0, 0, w, h))
	nested := plan.FramePlan{
		Width: g.Width, Height: g.Height, Background: "#000000", Transparent: true, Layers: g.Layers,
	}

	if err := c.Compose(surface, nested, k); err != nil {
		return Image{}, err
	}

	return Image{Pix: surface.Pix, W: w, H: h}, nil
}
