package command

import (
	"github.com/AlexGalhardo/stuarty/core/subtitle"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// maxSubtitleChars bounds the text of caption.import_subtitles.
const maxSubtitleChars = 2_000_000

func init() {
	Register(Define(Spec{
		Name:    "caption.import_subtitles",
		Summary: "Creates one caption per cue of an SRT or WebVTT text.",
		Description: "Reads the subtitle text (the service or the caller reads the file) and adds one caption for each cue " +
			"on a visual track: the cue time is the caption time, rounded to frames, and its words share the duration " +
			"in proportion to their character counts. Markup tags and line breaks are dropped. start moves every cue " +
			"later by that length. The cues must not overlap each other or the items of the track.",
		Example: `{"track_id": "trk_a1b2c3d4e5", "text": "1\n00:00:01,000 --> 00:00:02,500\nHello there\n", "format": "srt"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeIDExists,
			CodeLimit, CodeOverlap,
		},
		Scope: ScopeEdit,
	}, applyCaptionImportSubtitles))
}

type captionImportArgs struct {
	TrackID string   `json:"track_id"          desc:"A visual track that receives the captions."                                          pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Text    string   `json:"text"              desc:"The SRT or WebVTT text itself, not a file path: read the file and pass its content."                                                        maxlen:"2000000"`
	Format  string   `json:"format,omitempty"  desc:"srt or vtt; default: vtt when the text starts with WEBVTT."                                                                                                  enum:"srt,vtt"`
	Start   Time     `json:"start,omitempty"   desc:"A length added to every cue; default 0."`
	NewIDs  []string `json:"new_ids,omitempty" desc:"Ids for the created captions, in cue order."                                         pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

// msToFrames rounds a time in milliseconds to the nearest frame of the project.
func msToFrames(ms int, fps timeline.Rational) int {
	den := int64(1000) * int64(fps.Den)
	return int((int64(ms)*int64(fps.Num) + den/2) / den)
}

func applyCaptionImportSubtitles(c *Context, a *captionImportArgs) error {
	if err := idForm("args.track_id", timeline.PrefixTrack, a.TrackID); err != nil {
		return err
	}
	if err := validateNewIDs(a.NewIDs); err != nil {
		return err
	}
	offset := 0
	if a.Start != "" {
		var err *Error
		if offset, err = c.length("args.start", string(a.Start), true); err != nil {
			return err
		}
	}
	if chars(a.Text) > maxSubtitleChars {
		return invalidArg(
			"args.text",
			"args.text has %d characters; the limit is %d.",
			chars(a.Text),
			maxSubtitleChars,
		).
			WithHint("Import the subtitles in parts.")
	}
	format := a.Format
	if format == "" {
		format = subtitle.Detect(a.Text)
	}
	cues, perr := subtitle.Parse(format, a.Text)
	if perr != nil {
		return invalidArg("args.text", "args.text is not valid %s: %v.", format, perr).
			WithHint("Send SubRip or WebVTT text with a time line (00:00:01,000 --> 00:00:02,500) for each cue.")
	}
	if len(cues) == 0 {
		return invalidArg("args.text", "args.text has no cue with text.").
			WithHint("Send SubRip or WebVTT text with at least one cue.")
	}
	track, err := c.track(a.TrackID, "args.track_id")
	if err != nil {
		return err
	}
	if track.Kind != timeline.TrackVisual {
		return Errorf(CodeTypeMismatch, "Track %s is an audio track; captions need a visual track.", a.TrackID).
			WithHint("Use a visual track.").WithPath("args.track_id")
	}
	if total := len(c.Doc.Items) + len(cues); total > timeline.MaxItems {
		return Errorf(CodeLimit, "The document would have %d items; the limit is %d.", total, timeline.MaxItems).
			WithHint("Delete unused items first.")
	}
	ids, unused, err := c.takeNewIDs(a.NewIDs, len(cues))
	if err != nil {
		return err
	}
	fps := c.Doc.Settings.FPS
	for i, cue := range cues {
		start := offset + msToFrames(cue.StartMs, fps)
		duration := max(1, msToFrames(cue.EndMs, fps)+offset-start)
		fields, ferr := textFields(cue.Text)
		if ferr != nil {
			return ferr.WithHint("Cue %d: %s", i+1, "shorten the words of the cue.")
		}
		if duration < len(fields) {
			return Errorf(CodeInvalidState, "Cue %d lasts %d frames but has %d words; each word needs 1 frame.",
				i+1, duration, len(fields)).
				WithHint("Lengthen the cue in the file or use a higher frame rate.").WithPath("args.text")
		}
		words := wordsFromText(fields, duration)
		if err := c.checkWordLimits("", len(words)); err != nil {
			return err
		}
		if err := c.checkEnd("args.text", start, duration); err != nil {
			return err
		}
		if err := c.checkOverlap(ids[i], a.TrackID, start, duration); err != nil {
			return err.WithHint("Cue %d overlaps another item of the track; pick an empty track or another start.", i+1)
		}
		it := timeline.NewItem(ids[i], timeline.ItemCaption, a.TrackID, start, duration)
		it.Words = words
		c.Doc.Items[ids[i]] = it
	}
	c.SetEffective("created", ids)
	c.SetEffective("cue_count", len(cues))
	c.SetEffective("format", format)
	c.SetEffective("new_ids", ids)
	c.SetEffective("unused_ids", unused)
	return nil
}
