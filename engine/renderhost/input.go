package renderhost

import (
	"encoding/json"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// RenderInputVersion is the version of the render input this host writes (SDD-000 section 12).
const RenderInputVersion = 1

// Output is the frame to write (SDD-013 section 2.1). Width and Height are whole pixels from 1 to 4096; Scale
// is the output scale of SDD-004 section 6, which multiplies every position and size of the plan. When Y1 is
// greater than Y0 the module draws only the rows [Y0, Y1) of the frame, which is how a band of the export is
// rendered by its own instance.
type Output struct {
	Width, Height int
	Scale         float64
	Y0, Y1        int
}

// RenderInput is what one render call reads: the frame to write and the frame plan to draw.
type RenderInput struct {
	Output Output
	Plan   plan.FramePlan
}

// jsonLayer is a layer of the plan with the key of its bitmap, which the host computes and the renderer does
// not (SDD-013 section 2.2, bitmap_key).
type jsonLayer struct {
	plan.Layer

	BitmapKey string `json:"bitmap_key,omitempty"`
}

type jsonPlan struct {
	Frame       int         `json:"frame"`
	Width       int         `json:"width"`
	Height      int         `json:"height"`
	Background  string      `json:"background"`
	Transparent bool        `json:"transparent,omitempty"`
	Layers      []jsonLayer `json:"layers"`
}

type jsonOutput struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Scale  float64 `json:"scale"`
	Y0     *int    `json:"y0,omitempty"`
	Y1     *int    `json:"y1,omitempty"`
}

// MarshalJSON writes the render input of SDD-013 section 2.1: the plan keeps the JSON names of core/plan.
func (in RenderInput) MarshalJSON() ([]byte, error) {
	layers := make([]jsonLayer, len(in.Plan.Layers))
	for i := range in.Plan.Layers {
		layers[i].Layer = in.Plan.Layers[i]
		if b := in.Plan.Layers[i].Bitmap; b != nil {
			layers[i].BitmapKey = b.Key()
		}
	}

	out := jsonOutput{Width: in.Output.Width, Height: in.Output.Height, Scale: in.Output.Scale}
	if in.Output.Y1 > in.Output.Y0 {
		out.Y0, out.Y1 = &in.Output.Y0, &in.Output.Y1
	}

	return json.Marshal(struct {
		Version int        `json:"version"`
		Output  jsonOutput `json:"output"`
		Plan    jsonPlan   `json:"plan"`
	}{
		Version: RenderInputVersion,
		Output:  out,
		Plan: jsonPlan{
			Frame: in.Plan.Frame, Width: in.Plan.Width, Height: in.Plan.Height,
			Background: in.Plan.Background, Transparent: in.Plan.Transparent, Layers: layers,
		},
	})
}
