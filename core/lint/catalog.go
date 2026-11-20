package lint

// Rule describes one rule of SDD-003 §3.6, implemented or not. The table is
// what the lint documentation prints, so the text and the code cannot drift.
type Rule struct {
	Code     string
	Severity string
	Run      string // "doc", "item", "full" or a list such as "doc, full"
	Triggers string
	Hint     string
	// Implemented is false for a rule that Lint does not run yet.
	Implemented bool
}

// Catalog lists every rule of SDD-003 §3.6 in the order of the table there.
func Catalog() []Rule {
	return []Rule{
		{
			CodeEmptyTimeline,
			SeverityError,
			"doc",
			"the document has no item",
			"import media and place it with clip.add",
			true,
		},
		{
			CodeGap,
			SeverityWarning,
			"doc",
			"3 or more frames inside [0, end) with no visible clip or shape while a visible text or caption shows",
			"fill the range, or close it with timeline.close_gap",
			true,
		},
		{
			CodeBlackFrames,
			SeverityWarning,
			"doc",
			"1 or more frames inside [0, end) with no visible item of any kind",
			"fill the range with a clip or a shape, close it with timeline.close_gap, or shorten the timeline",
			true,
		},
		{
			CodeMediaMissing,
			SeverityError,
			"doc, full",
			"an item's asset is not in the project (the file check of the store is the service's)",
			"import the file again, then replace the item's asset",
			true,
		},
		{
			CodeMediaUnready,
			SeverityError,
			"doc",
			"the normalized media of the asset is not available",
			"wait for the asset to be ready and run lint again",
			true,
		},
		{
			CodeSourceRange,
			SeverityError,
			"full",
			"a clip reads past the end of the media actually in the store",
			"re-import the file",
			true,
		},
		{
			CodeOddDimensions,
			SeverityError,
			"doc",
			"width or height is odd or outside 16 to 4096",
			"use settings.set with even values",
			true,
		},
		{
			CodeSilentExport,
			SeverityWarning,
			"doc",
			"no source would be audible",
			"add an audio item or unmute a clip",
			true,
		},
		{
			CodeSliver,
			SeverityWarning,
			"doc",
			"a clip, audio or caption item lasts 1 or 2 frames",
			"delete it or trim the neighbour",
			true,
		},
		{
			CodeOccluded,
			SeverityWarning,
			"doc",
			"a clip or shape is hidden for its whole range by one item above it",
			"move or delete one of the two",
			true,
		},
		{
			CodeAudioOverlap,
			SeverityWarning,
			"doc",
			"two audible clips on different tracks overlap in time",
			"mute one with item.set_audio audio_muted or move one",
			true,
		},
		{
			CodeSafeArea,
			SeverityWarning,
			"item",
			"the box of a text or caption item reaches outside the safe rectangle of SDD-004 §4.1",
			"move the item inside the safe area or use caption position \"middle\"",
			true,
		},
		{
			CodeOffFrame,
			SeverityWarning,
			"item",
			"a visible item's box lies partly or wholly outside the frame",
			"reposition it with item.set_transform",
			true,
		},
		{
			CodeTransitionEmpty,
			SeverityWarning,
			"item",
			"a transition has no other visible visual item beneath it",
			"place a clip under it or remove the transition",
			true,
		},
		{
			CodeCaptionDesync,
			SeverityWarning,
			"item, full",
			"a caption extends beyond its linked clip, or starts away from the transcript",
			"run caption.retime_from_clip",
			true,
		},
		{
			CodeSlotUnfilled,
			SeverityError,
			"doc",
			"a required slot of a non-template document is not filled (an optional slot left on its placeholder is quiet)",
			"fill it with slot.fill",
			true,
		},
		{
			CodeAudioClip,
			SeverityWarning,
			"full",
			"the estimated mix peak exceeds 0 dBFS",
			"lower volume with item.set_audio",
			true,
		},
		{
			CodeLoudness,
			SeverityWarning,
			"full",
			"the estimated loudness differs from the target by more than 6 LU",
			"adjust volumes or accept the normalization",
			true,
		},
		{
			CodeDurationLimit,
			SeverityWarning,
			"doc",
			"the timeline is longer than 180 s",
			"trim the timeline or accept it",
			true,
		},
		{
			CodeAlphaMissing,
			SeverityWarning,
			"full",
			"a clip reads a file that declares an alpha channel but whose sampled pictures are all opaque",
			"render it with alpha",
			true,
		},
		{
			CodeAudioLimiterNotLast,
			SeverityWarning,
			"doc",
			"a limiter in a processor chain is followed by another enabled processor",
			"move it last with audio.processor_move",
			true,
		},
		{
			CodeAudioNoiseReductionRepeats,
			SeverityWarning,
			"doc",
			"a processor chain has more than one enabled noise reduction",
			"remove one",
			true,
		},
	}
}
