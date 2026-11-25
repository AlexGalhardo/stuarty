package command

import (
	"slices"

	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "recording.apply_layout",
		Summary: "Frames a screen and a webcam clip in a layout.",
		Description: "Places a screen clip and, for every layout but screen_only, a webcam clip by a named layout made for " +
			"the project format (16:9, 1:1, 4:5 or 9:16): screen_only, pip (the camera over a corner of the screen, a " +
			"circle by default), split (side by side in 16:9, one above the other in the others) or camera_focus (the " +
			"camera fills the frame, the screen is a small inset). padding is a fraction of the shorter side of the " +
			"frame, 0 to 0.25, left empty around everything. The result is ordinary fit, position, scale, crop and " +
			"mask values, so every one stays editable, and no clip leaves the frame. The clip in front is put on the " +
			"upper track. A clip whose x, y, scale, rotation or crop has keyframes cannot be laid out: apply the layout before " +
			"recording.auto_zoom.",
		Example: `{"layout": "pip", "screen_item_id": "itm_m1n2p3q4r5", "camera_item_id": "itm_s6t7v8w9x0", "corner": "bottom_left"}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState},
		Scope:   ScopeEdit,
	}, applyApplyLayout))
}

type applyLayoutArgs struct {
	Layout       string   `json:"layout"                   desc:"screen_only, pip, split or camera_focus."                                                      enum:"screen_only,pip,split,camera_focus"`
	ScreenItemID string   `json:"screen_item_id"           desc:"The clip of the screen take."                                                                                                                     pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	CameraItemID string   `json:"camera_item_id,omitempty" desc:"The clip of the webcam take; required by every layout but screen_only, which refuses it."                                                         pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Corner       string   `json:"corner,omitempty"         desc:"For pip and camera_focus. Default bottom_right."                                               enum:"top_left,top_right,bottom_left,bottom_right"`
	CameraShape  string   `json:"camera_shape,omitempty"   desc:"For pip. circle (default) or rect."                                                            enum:"circle,rect"`
	Padding      *float64 `json:"padding,omitempty"        desc:"Fraction of the shorter side of the frame left empty around everything, 0 to 0.25. Default 0."                                                                                                           min:"0" max:"0.25"`
}

// layoutClip is a clip of a layout with the size of its asset.
type layoutClip struct {
	it   timeline.Item
	size recording.Size
}

// layoutKeyframed are the properties a layout writes; a keyframe defines the value of one, so none may have any.
var layoutKeyframed = []string{"x", "y", "scale", "rotation", "crop_left", "crop_top", "crop_right", "crop_bottom"}

func (a *applyLayoutArgs) check() *Error {
	if err := idForm("args.screen_item_id", timeline.PrefixItem, a.ScreenItemID); err != nil {
		return err
	}

	if err := idForm("args.camera_item_id", timeline.PrefixItem, a.CameraItemID); err != nil {
		return err
	}

	switch recording.Layout(a.Layout) {
	case recording.LayoutScreenOnly:
		if a.CameraItemID != "" {
			return invalidArg("args.camera_item_id", "args.camera_item_id: the layout screen_only has no camera.").
				WithHint("Remove camera_item_id, or use pip, split or camera_focus.")
		}
	case recording.LayoutPip, recording.LayoutSplit, recording.LayoutCameraFocus:
		if a.CameraItemID == "" {
			return invalidArg(
				"args.camera_item_id",
				"args.camera_item_id: the layout %s needs the webcam clip.",
				a.Layout,
			).
				WithHint("Send camera_item_id, or use the layout screen_only.")
		}

		if a.CameraItemID == a.ScreenItemID {
			return invalidArg("args.camera_item_id", "args.camera_item_id is the screen clip %s.", a.ScreenItemID).
				WithHint("Send two different clips.")
		}
	default:
		return invalidArg("args.layout", "args.layout %q is not a layout.", a.Layout).
			WithHint("Use screen_only, pip, split or camera_focus.")
	}

	if a.Corner != "" && a.Layout != string(recording.LayoutPip) && a.Layout != string(recording.LayoutCameraFocus) {
		return invalidArg("args.corner", "args.corner applies to pip and camera_focus, not %s.", a.Layout).
			WithHint("Remove corner.")
	}

	if a.CameraShape != "" && a.Layout != string(recording.LayoutPip) {
		return invalidArg("args.camera_shape", "args.camera_shape applies to pip, not %s.", a.Layout).
			WithHint("Remove camera_shape; the other layouts show the camera as a rectangle.")
	}

	if a.Padding != nil && (*a.Padding < 0 || *a.Padding > recording.MaxLayoutPadding) {
		return invalidArg("args.padding", "args.padding is %s, outside 0 to %s.", trimFloat(*a.Padding),
			trimFloat(recording.MaxLayoutPadding)).WithHint("Use a fraction of the shorter side, 0 to 0.25.")
	}

	return nil
}

// layoutClipOf finds a clip of a layout and the size of its asset.
func (c *Context) layoutClipOf(id, path string) (layoutClip, *Error) {
	it, err := c.clipItem(id, "a layout")
	if err != nil {
		return layoutClip{}, err
	}

	asset, ok := c.Doc.Assets[it.Asset]
	if !ok || asset.Width <= 0 || asset.Height <= 0 {
		return layoutClip{}, Errorf(
			CodeInvalidState,
			"Clip %s has no asset with a size, so it cannot be laid out.",
			id,
		).
			WithHint("Use a clip of a video or an image.").
			WithPath(path)
	}

	var animated []string

	for _, p := range layoutKeyframed {
		if len(it.Keyframes[p]) > 0 {
			animated = append(animated, p)
		}
	}

	if len(animated) > 0 {
		return layoutClip{}, Errorf(CodeInvalidState, "Property %s of %s has %d keyframes, which define its value.",
			animated[0], id, len(it.Keyframes[animated[0]])).
			WithHint("Apply the layout before recording.auto_zoom, or remove the keyframes with keyframe.clear.").
			WithDetail("properties", animated).WithPath(path)
	}

	return layoutClip{it: it, size: recording.Size{W: float64(asset.Width), H: float64(asset.Height)}}, nil
}

// putClip writes a placed clip into its item: ordinary values, so the clip stays editable.
func putClip(it *timeline.Item, p *recording.Placed, circle bool) {
	x, y := p.CX, p.CY
	it.X, it.Y, it.Scale, it.Rotation, it.Fit = &x, &y, p.Scale, 0, "contain"

	for _, prop := range timeline.MaskProps {
		if len(it.Keyframes[prop]) > 0 {
			dropKeyframes(it, prop, 0)
		}
	}

	it.Crop = nil
	if !p.Crop.IsZero() {
		it.Crop = &timeline.Crop{Left: p.Crop.Left, Top: p.Crop.Top, Right: p.Crop.Right, Bottom: p.Crop.Bottom}
	}

	it.Mask = nil
	if circle {
		it.Mask = &timeline.Mask{Shape: "ellipse", X: 0, Y: 0, Width: 1, Height: 1}
	}
}

func rectMap(r recording.Rect) map[string]any {
	return map[string]any{"x": r.X, "y": r.Y, "width": r.W, "height": r.H}
}

func applyApplyLayout(c *Context, a *applyLayoutArgs) error {
	if err := a.check(); err != nil {
		return err
	}

	screen, err := c.layoutClipOf(a.ScreenItemID, "args.screen_item_id")
	if err != nil {
		return err
	}

	camera := layoutClip{}

	if a.CameraItemID != "" {
		if camera, err = c.layoutClipOf(a.CameraItemID, "args.camera_item_id"); err != nil {
			return err
		}
	}

	padding := 0.0
	if a.Padding != nil {
		padding = *a.Padding
	}

	plan, perr := recording.PlanLayout(recording.LayoutRequest{
		Layout: recording.Layout(
			a.Layout,
		),
		Corner:    recording.Corner(a.Corner),
		Shape:     recording.CameraShape(a.CameraShape),
		Padding:   padding,
		FrameW:    c.Doc.Settings.Width,
		FrameH:    c.Doc.Settings.Height,
		Screen:    screen.size,
		Camera:    camera.size,
		HasCamera: a.CameraItemID != "",
	})
	if perr != nil {
		return invalidArg("args", "The layout cannot be made: %s", perr.Error())
	}

	putClip(&screen.it, &plan.Screen, false)
	c.Doc.Items[screen.it.ID] = screen.it
	c.SetEffective("layout", a.Layout)
	c.SetEffective("format", string(plan.Format))
	c.SetEffective("screen", rectMap(plan.Screen.Shown))

	if plan.Camera == nil {
		return nil
	}

	putClip(&camera.it, plan.Camera, plan.Circle)
	c.Doc.Items[camera.it.ID] = camera.it
	c.SetEffective("camera", rectMap(plan.Camera.Shown))

	if plan.Overlap {
		front, back := screen.it.Track, camera.it.Track
		if plan.CameraInFront {
			front, back = back, front
		}

		return c.bringAbove(front, back)
	}

	return nil
}

// bringAbove puts the track front directly above the track back when it is below it; clips that share a track
// are not reordered.
func (c *Context) bringAbove(front, back string) error {
	if front == back {
		return nil
	}

	order := c.tracksByOrder("")
	if slices.Index(order, front) < slices.Index(order, back) {
		return nil
	}

	ord, err := c.placeOrder(front, "", back, "")
	if err != nil {
		return err
	}

	tr := c.Doc.Tracks[front]
	tr.Order = ord
	c.Doc.Tracks[front] = tr

	return nil
}
