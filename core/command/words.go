package command

import (
	"fmt"
	"slices"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// WordArg is a caption word as a caller writes it: times are lengths relative to the
// caption (SDD-002 R16).
type WordArg struct {
	Text     string `json:"text"`
	Start    Time   `json:"start"`
	Duration Time   `json:"duration"`
}

const maxWordsPerCaption = 60

// parseWords checks and converts word arguments (R16): times first (E_INVALID_TIME),
// then text, order and overlap (E_VALIDATION). The count limit is the caller's.
func (c *Context) parseWords(path string, args []WordArg) ([]timeline.Word, *Error) {
	if len(args) == 0 {
		return nil, Errorf(CodeValidation, "%s must have at least 1 word, got 0.", path).
			WithHint("Send at least one word, for example {\"text\": \"Hello\", \"start\": \"0f\", \"duration\": \"9f\"}.").
			WithPath(path)
	}
	words := make([]timeline.Word, len(args))
	for i, a := range args {
		wp := fmt.Sprintf("%s[%d]", path, i)
		start, err := c.length(wp+".start", string(a.Start), true)
		if err != nil {
			return nil, err
		}
		dur, err := c.length(wp+".duration", string(a.Duration), false)
		if err != nil {
			return nil, err
		}
		words[i] = timeline.Word{Text: a.Text, Start: start, Duration: dur}
	}
	end := 0
	for i, w := range words {
		wp := fmt.Sprintf("%s[%d]", path, i)
		switch {
		case chars(w.Text) < 1 || chars(w.Text) > 80:
			return nil, Errorf(CodeValidation, "%s.text must have 1 to 80 characters, got %d.", wp, chars(w.Text)).
				WithHint("Send one word of at most 80 characters.").WithPath(wp)
		case strings.IndexFunc(w.Text, isSpace) >= 0:
			return nil, Errorf(CodeValidation, "%s.text %q contains whitespace.", wp, w.Text).
				WithHint("Send one word per entry.").WithPath(wp)
		case w.Start < end:
			return nil, Errorf(
				CodeValidation,
				"%s overlaps words[%d]: starts at %d but the previous word ends at %d.",
				wp,
				i-1,
				w.Start,
				end,
			).
				WithHint("Start each word at or after the end of the previous one.").
				WithPath(wp)
		}
		end = w.Start + w.Duration
	}
	return words, nil
}

func isSpace(r rune) bool { return strings.TrimSpace(string(r)) == "" }

// wordsFromText splits text on whitespace and shares duration among the words in
// proportion to their character counts (decision 19): each word gets 1 frame, then
// the rest by largest remainder, lower index first on ties. The caller has checked
// that duration is at least the number of words.
func wordsFromText(fields []string, duration int) []timeline.Word {
	n := len(fields)
	total := 0
	for _, f := range fields {
		total += chars(f)
	}
	free := duration - n
	shares := make([]int, n)
	rems := make([]int, n)
	given := 0
	for i, f := range fields {
		shares[i] = free * chars(f) / total
		rems[i] = free * chars(f) % total
		given += shares[i]
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return rems[b] - rems[a] })
	for _, i := range order[:free-given] {
		shares[i]++
	}
	words := make([]timeline.Word, n)
	at := 0
	for i, f := range fields {
		words[i] = timeline.Word{Text: f, Start: at, Duration: 1 + shares[i]}
		at += 1 + shares[i]
	}
	return words
}

// trimWordsFront is the word rule of item.trim for the start edge with d frames lost
// (d < 0: gained): each start moves by -d, words that end at or before 0 are removed,
// a word that crosses 0 starts at 0 and keeps its end. breaks follow.
func trimWordsFront(it *timeline.Item, d int) {
	var kept []timeline.Word
	removed := 0
	for _, w := range it.Words {
		end := w.Start - d + w.Duration
		if end <= 0 {
			removed++
			continue
		}
		w.Start -= d
		if w.Start < 0 {
			w.Duration += w.Start
			w.Start = 0
		}
		kept = append(kept, w)
	}
	it.Words = kept
	var breaks []int
	for _, b := range it.Breaks {
		if b -= removed; b >= 1 && b < len(kept) {
			breaks = append(breaks, b)
		}
	}
	it.Breaks = breaks
}

// trimWordsBack is the word rule of item.trim for the end edge cut to duration:
// words starting at or after it are removed, a word that crosses it is clamped.
func trimWordsBack(it *timeline.Item, duration int) {
	var kept []timeline.Word
	for _, w := range it.Words {
		if w.Start >= duration {
			continue
		}
		if w.Start+w.Duration > duration {
			w.Duration = duration - w.Start
		}
		kept = append(kept, w)
	}
	it.Words = kept
	var breaks []int
	for _, b := range it.Breaks {
		if b >= 1 && b < len(kept) {
			breaks = append(breaks, b)
		}
	}
	it.Breaks = breaks
}

// splitWords divides the words of an item at relative frame tc (item.split): a word
// goes to the left part when it starts before tc, and is clamped there; the right
// part gets the others shifted by -tc. breaks are divided with them.
func splitWords(words []timeline.Word, breaks []int, tc int) (left, right []timeline.Word, lb, rb []int) {
	for _, w := range words {
		if w.Start < tc {
			if w.Start+w.Duration > tc {
				w.Duration = tc - w.Start
			}
			left = append(left, w)
			continue
		}
		w.Start -= tc
		right = append(right, w)
	}
	for _, b := range breaks {
		switch {
		case b >= 1 && b < len(left):
			lb = append(lb, b)
		case b > len(left):
			rb = append(rb, b-len(left))
		}
	}
	return left, right, lb, rb
}

// wordTotal counts the caption words of the document, optionally not counting one item.
func (c *Context) wordTotal(except string) int {
	n := 0
	for id := range c.Doc.Items {
		if id != except {
			n += len(c.Doc.Items[id].Words)
		}
	}
	return n
}

// checkWordLimits is the E_LIMIT of R16: a caption has at most 60 words and the
// document at most 5,000 once n words replace those of the item except (or are added,
// when except is empty).
func (c *Context) checkWordLimits(except string, n int) *Error {
	if n > maxWordsPerCaption {
		return Errorf(CodeLimit, "The caption would have %d words; the limit is %d.", n, maxWordsPerCaption).
			WithHint("Split the text into several captions.")
	}
	if total := c.wordTotal(except) + n; total > maxWordsPerDocument {
		return Errorf(
			CodeLimit,
			"The document would have %d caption words; the limit is %d.",
			total,
			maxWordsPerDocument,
		).
			WithHint("Remove captions you no longer need.")
	}
	return nil
}
