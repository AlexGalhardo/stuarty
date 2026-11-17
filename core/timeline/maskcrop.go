package timeline

// MaskProps are the mask properties that keyframe.set can animate (SDD-001 §3.8, VV-120). They
// apply to an item that has a mask; the shape and invert stay fixed.
var MaskProps = []string{"mask_x", "mask_y", "mask_width", "mask_height", "mask_feather"}

// CropProps are the crop sides of a clip that keyframe.set can animate (SDD-001 §3.8, VV-120).
var CropProps = []string{"crop_left", "crop_top", "crop_right", "crop_bottom"}

// MaskRange is the range of a mask property, and whether it is one.
func MaskRange(prop string) (lo, hi float64, ok bool) {
	switch prop {
	case "mask_x", "mask_y":
		return 0, 1, true
	case "mask_width", "mask_height":
		return 0.001, 1, true
	case "mask_feather":
		return 0, MaxMaskFeather, true
	}
	return 0, 0, false
}

// CropRange is the range of a crop property, and whether it is one.
func CropRange(prop string) (lo, hi float64, ok bool) {
	switch prop {
	case "crop_left", "crop_top", "crop_right", "crop_bottom":
		return 0, 1, true
	}
	return 0, 0, false
}

// MaskValue is the static value of a mask property of an item; 0 when it has no mask.
func (it *Item) MaskValue(prop string) float64 {
	m := it.Mask
	if m == nil {
		return 0
	}
	switch prop {
	case "mask_x":
		return m.X
	case "mask_y":
		return m.Y
	case "mask_width":
		return m.Width
	case "mask_height":
		return m.Height
	}
	return float64(m.Feather)
}

// CropValue is the static value of a crop property of an item; 0 when it has no crop.
func (it *Item) CropValue(prop string) float64 {
	c := it.Crop
	if c == nil {
		return 0
	}
	switch prop {
	case "crop_left":
		return c.Left
	case "crop_top":
		return c.Top
	case "crop_right":
		return c.Right
	}
	return c.Bottom
}

// SetMaskValue writes the static field of a mask property; an item without a mask is left as it is.
func (it *Item) SetMaskValue(prop string, v float64) {
	if it.Mask == nil {
		return
	}
	cp := *it.Mask // the pointer may be shared with another copy of the item
	m := &cp
	it.Mask = m
	switch prop {
	case "mask_x":
		m.X = v
	case "mask_y":
		m.Y = v
	case "mask_width":
		m.Width = v
	case "mask_height":
		m.Height = v
	case "mask_feather":
		m.Feather = int(v + 0.5)
	}
}

// SetCropValue writes the static field of a crop property and drops the crop when every side is 0.
func (it *Item) SetCropValue(prop string, v float64) {
	c := Crop{}
	if it.Crop != nil {
		c = *it.Crop
	}
	switch prop {
	case "crop_left":
		c.Left = v
	case "crop_top":
		c.Top = v
	case "crop_right":
		c.Right = v
	case "crop_bottom":
		c.Bottom = v
	}
	if c.Left == 0 && c.Top == 0 && c.Right == 0 && c.Bottom == 0 && len(c.Extra) == 0 {
		it.Crop = nil
		return
	}
	it.Crop = &c
}
