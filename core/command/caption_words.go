package command

import (
	"fmt"
	"slices"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "caption.set_words",
		Summary: "Replaces the words of a caption.",
		Description: "Replaces all words, given as words (1 to 60, times relative to the caption, as lengths) or as " +
			"text, which is split on whitespace and spread over the caption's current duration like caption.add " +
			"(duration may change it). With words the caption lasts until the end of the last word unless duration " +
			"is given. Line breaks are cleared; keyframes are cut when the caption gets shorter.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "words": [{"text": "Hello", "start": "0f", "duration": "12f"}, ` +
			`{"text": "world", "start": "12f", "duration": "12f"}]}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeLimit, CodeOverlap,
		},
		Scope: ScopeEdit,
		Phase: Alpha,
	}, applyCaptionSetWords))

	Register(Define(Spec{
		Name:    "caption.edit_word",
		Summary: "Changes the text of one word of a caption, or removes the word.",
		Description: "Replaces the text of the word at index; timing is untouched. An empty text removes the word and " +
			"leaves a gap. The only word of a caption cannot be removed.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "index": 3, "text": "editor"}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch, CodeInvalidState},
		Scope:   ScopeEdit,
		Phase:   Alpha,
	}, applyCaptionEditWord))

	Register(Define(Spec{
		Name:    "caption.set_breaks",
		Summary: "Sets the forced line breaks of a caption.",
		Description: "Each break is the index of the word that starts a new line, from 1 to the word count minus 1, " +
			"strictly increasing. An empty array clears the breaks.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "breaks": [2]}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch},
		Scope:   ScopeEdit,
		Phase:   Alpha,
	}, applyCaptionSetBreaks))

	Register(Define(Spec{
		Name:    "caption.set_style",
		Summary: "Sets, changes or clears the style override of one caption.",
		Description: "The given keys are merged into the override the caption already has, so a later call that " +
			"gives only a colour keeps the position set before; a null value removes one key. With replace true the " +
			"object replaces the whole override. A null or {} style clears the override. highlight is none or " +
			"word. The caption style of the track is not touched.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "style": {"size": 80, "highlight": "word"}}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch, CodeFontUnknown},
		Scope:   ScopeEdit,
		Phase:   Alpha,
	}, applyCaptionSetStyle))
}

// caption returns the caption item id names. A text item gets the message and the
// hint of the command, any other type a plain one.
func (c *Context) caption(id, textMsg, textHint string) (timeline.Item, *Error) {
	it, err := c.item(id)
	if err != nil {
		return it, err
	}
	if it.Type == timeline.ItemCaption {
		return it, nil
	}
	if it.Type == timeline.ItemText {
		return it, Errorf(CodeTypeMismatch, "Item %s is a text item; %s", id, textMsg).
			WithHint("%s", textHint).WithPath("args.item_id")
	}
	return it, Errorf(CodeTypeMismatch, "Item %s is %s item, not a caption.", id, article(it.Type)).
		WithHint("Choose a caption item.").WithPath("args.item_id")
}

type captionSetWordsArgs struct {
	ItemID   string    `json:"item_id"            desc:"A caption."                                                                                                          pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Words    []WordArg `json:"words,omitempty"    desc:"1 to 60 words with times relative to the caption, as lengths. Or give text."`
	Text     *string   `json:"text,omitempty"     desc:"Text split on whitespace and spread over the duration; instead of words."`
	Duration Time      `json:"duration,omitempty" desc:"A length. With words: at least the end of the last word, default that end. With text: default the current duration."`
}

func applyCaptionSetWords(c *Context, a *captionSetWordsArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	if e := requireOne([]string{"words", "text"}, a.Words != nil, a.Text != nil); e != nil {
		return e
	}
	var words []timeline.Word
	var err *Error
	if a.Text == nil {
		if words, err = c.parseWords("args.words", a.Words); err != nil {
			return err
		}
	}
	given := 0
	if a.Duration != "" {
		if given, err = c.length("args.duration", string(a.Duration), false); err != nil {
			return err
		}
	}
	it, err := c.caption(a.ItemID, "use text.set.", "Use text.set for text items.")
	if err != nil {
		return err
	}
	var duration int
	if a.Text != nil {
		fields, ferr := textFields(*a.Text)
		if ferr != nil {
			return ferr
		}
		duration = it.Duration
		if a.Duration != "" {
			duration = given
		}
		if duration < len(fields) {
			return Errorf(CodeInvalidState, "Duration %d frames is too short for %d words; each word needs 1 frame.",
				duration, len(fields)).
				WithHint("Use duration \"%df\" or longer.", len(fields)).
				WithDetail("min_duration", len(fields)).WithPath("args.duration")
		}
		words = wordsFromText(fields, duration)
	} else {
		last := words[len(words)-1]
		duration = last.Start + last.Duration
	}
	if a.Text == nil && a.Duration != "" {
		if given < duration {
			return Errorf(
				CodeInvalidState,
				"args.duration %d frames is before the last word ends at %d.",
				given,
				duration,
			).
				WithHint("Use duration \"%df\" or longer, or omit it.", duration).
				WithPath("args.duration")
		}
		duration = given
	}
	if err := c.checkWordLimits(a.ItemID, len(words)); err != nil {
		return err
	}
	if duration > it.Duration {
		if err := c.checkOverlap(a.ItemID, it.Track, it.Start, duration); err != nil {
			return err.WithHint("Use words that end by \"%df\", or move the next caption.", it.Duration)
		}
		if err := c.checkEnd("args.words", it.Start, duration); err != nil {
			return err
		}
	}
	it.Words, it.Breaks = words, nil
	if duration < it.Duration {
		kCut(&it, duration)
	}
	it.Duration = duration
	if err := checkFit(&it, "The new words"); err != nil {
		return err
	}
	c.Doc.Items[a.ItemID] = it
	c.SetEffectiveTime("duration", duration, string(a.Duration))
	c.SetEffective("word_count", len(words))
	return nil
}

type captionEditWordArgs struct {
	ItemID string `json:"item_id" desc:"A caption."                                                      pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Index  int    `json:"index"   desc:"0-based index of the word."                                                                                             min:"0"`
	Text   string `json:"text"    desc:"The new text, 0 to 80 characters, no whitespace; empty removes."                                                        min:"0" max:"80"`
}

func applyCaptionEditWord(c *Context, a *captionEditWordArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	if strings.ContainsFunc(a.Text, isSpace) {
		return Errorf(CodeValidation, "args.text %q contains whitespace; a word has none.", a.Text).
			WithHint("Use caption.set_words to replace one word by several.").WithPath("args.text")
	}
	it, err := c.caption(a.ItemID, "use text.set.", "Use text.set for text items.")
	if err != nil {
		return err
	}
	n := len(it.Words)
	if a.Index >= n {
		return Errorf(CodeNotFound, "Caption %s has %d words; index %d does not exist.", a.ItemID, n, a.Index).
			WithHint("Use an index from 0 to %d.", n-1).WithPath("args.index").
			WithDetail("kind", "word").WithDetail("id", a.Index)
	}
	words := slices.Clone(it.Words)
	removed := a.Text == ""
	if removed {
		if n == 1 {
			return Errorf(CodeInvalidState, "Removing word 0 would leave caption %s with no words.", a.ItemID).
				WithHint("Delete the caption with item.delete instead.")
		}
		words = slices.Delete(words, a.Index, a.Index+1)
		var breaks []int
		for _, b := range it.Breaks {
			if b > a.Index {
				b--
			}
			if b >= 1 && b < len(words) && !slices.Contains(breaks, b) {
				breaks = append(breaks, b)
			}
		}
		it.Breaks = breaks
	} else {
		words[a.Index].Text = a.Text
	}
	it.Words = words
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("index", a.Index)
	c.SetEffective("removed", removed)
	c.SetEffective("word_count", len(words))
	return nil
}

type captionSetBreaksArgs struct {
	ItemID string `json:"item_id" desc:"A caption."                                                                  pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Breaks []int  `json:"breaks"  desc:"Indexes of the words that start a new line, strictly increasing; [] clears."`
}

func applyCaptionSetBreaks(c *Context, a *captionSetBreaksArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	it, err := c.caption(a.ItemID, "text breaks lines in its text.", "Put a line break in the text with text.set.")
	if err != nil {
		return err
	}
	largest := len(it.Words) - 1
	for i, b := range a.Breaks {
		path := fmt.Sprintf("args.breaks[%d]", i)
		switch {
		case b < 1 || b > largest:
			return Errorf(CodeValidation, "%s is %d but the caption has %d words; the allowed indexes are 1 to %d.",
				path, b, len(it.Words), largest).
				WithHint("Use indexes from 1 to %d.", largest).WithPath(path)
		case i > 0 && b <= a.Breaks[i-1]:
			return Errorf(CodeValidation, "%s is %d but must be greater than the previous break %d.",
				path, b, a.Breaks[i-1]).
				WithHint("Send the breaks in strictly increasing order.").WithPath(path)
		}
	}
	it.Breaks = nil
	shown := []int{}
	if len(a.Breaks) > 0 {
		it.Breaks = slices.Clone(a.Breaks)
		shown = it.Breaks
	}
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("breaks", shown)
	return nil
}

type captionSetStyleArgs struct {
	ItemID  string `json:"item_id"           desc:"A caption."                                                                                       pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Style   any    `json:"style"             desc:"A partial CaptionStyle merged into the override (null removes a key), or null or {} to clear it."                                                        schema:"caption_style_merge_or_nil"`
	Replace bool   `json:"replace,omitempty" desc:"Replace the whole override with style instead of merging."`
}

func applyCaptionSetStyle(c *Context, a *captionSetStyleArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	var given map[string]any
	switch s := a.Style.(type) {
	case nil:
	case map[string]any:
		given = s
	default:
		return Errorf(CodeValidation, "args.style must be an object or null, got %s.", kindOf(a.Style)).
			WithHint("Send a partial CaptionStyle, or null to clear the override.").WithPath("args.style")
	}
	if err := c.validateStyle(given, true, !a.Replace); err != nil {
		return err
	}
	it, err := c.caption(a.ItemID, "use text.set.", "Use text.set for text items.")
	if err != nil {
		return err
	}
	clearAll := len(given) == 0
	style, err := mergeStyle(it.Style, given, a.Replace || clearAll)
	if err != nil {
		return err
	}
	if err := c.checkFont(style); err != nil {
		return err
	}
	it.Style = style
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("style", styleOrEmpty(style))
	return nil
}
