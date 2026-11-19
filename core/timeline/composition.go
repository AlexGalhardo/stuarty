package timeline

// CompositionFeature names the first thing of a composition item that makes it draw as one unit instead of as
// its timeline would on its own (SDD-004 §3.16): a property of the item that changes the picture of the nested
// frame, or one that animates. It is "" for an item that is only a place for its timeline, which is what the
// planner flattens and what composition.expand can undo. It reads the stored item, never a frame.
func CompositionFeature(it *Item) string {
	switch {
	case it.Opacity != 1:
		return "opacity"
	case it.Scale != 1:
		return "scale"
	case it.Rotation != 0:
		return "rotation"
	case it.X != nil || it.Y != nil:
		return "position"
	case len(it.Keyframes) > 0:
		return "keyframes"
	case it.TransitionIn != nil || it.TransitionOut != nil:
		return "transition"
	case it.Mask != nil:
		return "mask"
	case len(it.Effects) > 0:
		return "effects"
	case it.Shadow != nil:
		return "shadow"
	case it.Outline != nil:
		return "outline"
	case it.Blur != 0:
		return "blur"
	case it.Filter != "" && it.Filter != "none":
		return "filter"
	case it.Blend != "" && it.Blend != "normal":
		return "blend"
	case it.FlipX || it.FlipY:
		return "flip"
	}

	return ""
}

// CompositionAudioFeature names the first thing of a composition item that changes the sound of its timeline:
// its volume, its fades or its mute.
func CompositionAudioFeature(it *Item) string {
	switch {
	case it.Volume != 1:
		return "volume"
	case it.FadeIn != 0 || it.FadeOut != 0:
		return "fade"
	case it.AudioMuted:
		return "audio_muted"
	}

	return ""
}

// SameFrame says two documents have the frame size and the frame rate, so a timeline nested in the other lands
// on its pixels and its frames one to one.
func SameFrame(a, b *Settings) bool {
	return a.Width == b.Width && a.Height == b.Height && a.FPS == b.FPS
}
