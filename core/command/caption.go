package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const maxWordsPerDocument = timeline.MaxCaptionWords

func init() {
	Register(Define(Spec{
		Name:    "caption.add",
		Summary: "Adds a caption item with timed words.",
		Description: "Adds a caption. Give either words (times relative to the caption, as lengths) or text with a duration: " +
			"the text is split on whitespace and the words share the duration in proportion to their character counts. " +
			"x and y are used when the caption style position of the track is custom.",
		Example: `{"track_id": "trk_a1b2c3d4e5", "start": "12f", "text": "Meet the new editor.", "duration": "48f", ` +
			`"linked_item": "itm_m1n2p3q4r5", "id": "itm_s6t7v8w9x0"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeIDExists,
			CodeLimit, CodeOverlap,
		},
		Scope: ScopeEdit,
	}, applyCaptionAdd))
}

type captionAddArgs struct {
	TrackID    string    `json:"track_id,omitempty"    desc:"A visual track; omitted, the highest visual track free over the item (an empty track you made for video counts; pass track_id to keep it clear), or a new one on top. effective.track_id names it." pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Start      Time      `json:"start"`
	Words      []WordArg `json:"words,omitempty"       desc:"Words with times relative to the caption, as lengths."`
	Text       *string   `json:"text,omitempty"        desc:"Text to split on whitespace; needs duration."`
	Duration   Time      `json:"duration,omitempty"    desc:"A length; with text only."`
	LinkedItem string    `json:"linked_item,omitempty" desc:"The clip whose speech this transcribes; it need not exist."`
	X          *float64  `json:"x,omitempty"`
	Y          *float64  `json:"y,omitempty"`
	ID         string    `json:"id,omitempty"                                                                                                                                                                                                                    pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
}

// captionForm checks the words-or-text rules of caption.add and returns the words and
// the duration of the caption; for the text form the duration is the one given, and
// the words are made once it is known to be long enough.
func (c *Context) captionForm(a *captionAddArgs) (words []timeline.Word, fields []string, duration int, err *Error) {
	if e := requireOne([]string{"words", "text"}, a.Words != nil, a.Text != nil); e != nil {
		return nil, nil, 0, e
	}
	if a.Words != nil {
		if a.Duration != "" {
			return nil, nil, 0, Errorf(
				CodeValidation,
				"args.duration cannot be used with words: the caption lasts until the end of the last word.",
			).
				WithHint("Remove duration, or send text instead of words.").
				WithPath("args.duration")
		}
		words, err = c.parseWords("args.words", a.Words)
		if err != nil {
			return nil, nil, 0, err
		}
		last := words[len(words)-1]
		return words, nil, last.Start + last.Duration, nil
	}
	if a.Duration == "" {
		return nil, nil, 0, Errorf(CodeValidation, "args.duration is required with text.").
			WithHint(`Send a duration, for example "48f".`).WithPath("args.duration")
	}
	if duration, err = c.length("args.duration", string(a.Duration), false); err != nil {
		return nil, nil, 0, err
	}
	if fields, err = textFields(*a.Text); err != nil {
		return nil, nil, 0, err
	}
	return nil, fields, duration, nil
}

// textFields splits a caption text on whitespace and checks each word (1 to 80 characters).
func textFields(text string) ([]string, *Error) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil, Errorf(CodeValidation, "args.text has no word.").
			WithHint(`Send the text to show, for example "Meet the new editor."`).WithPath("args.text")
	}
	for i, f := range fields {
		if chars(f) > 80 {
			p := fmt.Sprintf("args.text word %d", i)
			return nil, Errorf(CodeValidation, "%s has %d characters; a word has 1 to 80.", p, chars(f)).
				WithHint("Split long words.").WithPath("args.text")
		}
	}
	return fields, nil
}

func applyCaptionAdd(c *Context, a *captionAddArgs) error {
	for _, f := range []struct{ path, prefix, id string }{
		{"args.track_id", timeline.PrefixTrack, a.TrackID},
		{"args.id", timeline.PrefixItem, a.ID},
		{"args.linked_item", timeline.PrefixItem, a.LinkedItem},
	} {
		if err := idForm(f.path, f.prefix, f.id); err != nil {
			return err
		}
	}
	start, err := c.ParseTime("args.start", string(a.Start))
	if err != nil {
		return err
	}
	words, fields, duration, err := c.captionForm(a)
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
		return Errorf(CodeTypeMismatch, "Track %s is an audio track; captions need a visual track.", a.TrackID).
			WithHint("Use a visual track.").WithPath("args.track_id")
	}
	if fields != nil {
		if duration < len(fields) {
			return Errorf(
				CodeInvalidState,
				"Duration %d frames is too short for %d words; each word needs 1 frame.",
				duration,
				len(fields),
			).
				WithHint("Use duration \"%df\" or longer.", len(fields)).
				WithDetail("min_duration", len(fields)).
				WithPath("args.duration")
		}
		words = wordsFromText(fields, duration)
	}
	id, err := c.newID("items", timeline.PrefixItem, a.ID)
	if err != nil {
		return err
	}
	if err := c.limit("items", len(c.Doc.Items), timeline.MaxItems); err != nil {
		return err
	}
	if err := c.checkWordLimits("", len(words)); err != nil {
		return err
	}
	if err := c.checkEnd("args.start", start, duration); err != nil {
		return err
	}
	if err := c.checkOverlap(id, a.TrackID, start, duration); err != nil {
		return err
	}
	it := timeline.NewItem(id, timeline.ItemCaption, a.TrackID, start, duration)
	it.Words, it.LinkedItem, it.X, it.Y = words, a.LinkedItem, a.X, a.Y
	c.Doc.Items[id] = it
	c.SetEffective("id", id)
	c.SetEffectiveTime("start", start, string(a.Start))
	c.SetEffectiveTime("duration", duration, string(a.Duration))
	c.SetEffective("word_count", len(words))
	return nil
}
