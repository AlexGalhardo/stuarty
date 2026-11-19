package writing

import (
	"fmt"
	"strings"
)

// Words per minute of the estimate of a script (SDD-019 section 4).
const (
	DefaultWordsPerMinute = 150
	MinWordsPerMinute     = 60
	MaxWordsPerMinute     = 300
)

// Scene is a part of a script: what is said, what is shown on the screen, and the directions for the editor.
type Scene struct {
	Title     string   `json:"title"     untrusted:"true"`
	Narration []string `json:"narration" untrusted:"true"`
	OnScreen  []string `json:"on_screen" untrusted:"true"`
	Notes     []string `json:"notes"     untrusted:"true"`
}

// Script is a script read from its Markdown.
type Script struct {
	Scenes []Scene `json:"scenes"`
	// Plain is true when the script has no narration marker: every paragraph that is not a heading is narration.
	Plain bool `json:"plain"`
}

type blockKind int

const (
	blockNote blockKind = iota
	blockNarration
	blockOnScreen
)

var (
	narrationMarkers = []string{"vo:", "narration:"}
	onScreenMarkers  = []string{"on screen:", "on-screen:", "text:"}
)

// ParseScript reads the structure of a script (SDD-019 section 4): a heading of level 1 to 3 starts a scene, a
// paragraph that starts with VO: or NARRATION: is narration, one that starts with ON SCREEN:, ON-SCREEN: or TEXT: is
// on-screen text, any other paragraph is a note, and a script with no narration marker is read in plain mode.
func ParseScript(body string) Script {
	var (
		scenes  []Scene
		cur     = -1
		para    []string
		kind    blockKind
		anyNarr bool
	)

	scene := func() *Scene {
		if cur < 0 {
			scenes = append(scenes, Scene{})
			cur = 0
		}

		return &scenes[cur]
	}

	flush := func() {
		if len(para) == 0 {
			return
		}

		text := strings.TrimSpace(strings.Join(para, "\n"))
		para = nil

		if text == "" {
			return
		}

		sc := scene()

		switch kind {
		case blockNarration:
			sc.Narration = append(sc.Narration, text)
			anyNarr = true
		case blockOnScreen:
			sc.OnScreen = append(sc.OnScreen, text)
		case blockNote:
			sc.Notes = append(sc.Notes, text)
		}
	}

	for line := range strings.Lines(strings.ReplaceAll(body, "\r\n", "\n")) {
		line = strings.TrimRight(line, "\n")

		if title, ok := sceneHeading(line); ok {
			flush()

			scenes = append(scenes, Scene{Title: title})
			cur = len(scenes) - 1

			continue
		}

		if strings.TrimSpace(line) == "" {
			flush()

			continue
		}

		if k, rest, ok := marker(line); ok {
			flush()

			kind = k
			para = append(para, rest)

			continue
		}

		if len(para) == 0 {
			kind = blockNote
		}

		para = append(para, strings.TrimSpace(line))
	}

	flush()

	s := Script{Scenes: scenes, Plain: !anyNarr}
	if s.Plain {
		for i := range s.Scenes {
			s.Scenes[i].Narration, s.Scenes[i].Notes = s.Scenes[i].Notes, nil
		}
	}

	if s.Scenes == nil {
		s.Scenes = []Scene{}
	}

	return s
}

// sceneHeading reports whether line is a heading of level 1 to 3 and returns its text.
func sceneHeading(line string) (string, bool) {
	t := strings.TrimLeft(line, " ")
	if len(line)-len(t) > 3 {
		return "", false
	}

	level := 0
	for level < len(t) && t[level] == '#' {
		level++
	}

	if level < 1 || level > 3 || (len(t) > level && t[level] != ' ' && t[level] != '\t') {
		return "", false
	}

	title := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(t[level:]), "#"))

	return title, true
}

// marker reports whether line starts a narration or on-screen block, and returns the text after the marker.
func marker(line string) (blockKind, string, bool) {
	t := strings.TrimSpace(line)
	t = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(t, "- "), "* "))
	low := strings.ToLower(t)

	for _, m := range narrationMarkers {
		if strings.HasPrefix(low, m) {
			return blockNarration, strings.TrimSpace(t[len(m):]), true
		}
	}

	for _, m := range onScreenMarkers {
		if strings.HasPrefix(low, m) {
			return blockOnScreen, strings.TrimSpace(t[len(m):]), true
		}
	}

	return blockNote, "", false
}

// SceneEstimate is the estimate of one scene.
type SceneEstimate struct {
	Title          string `json:"title"           untrusted:"true"`
	Words          int    `json:"words"`
	OnScreenWords  int    `json:"on_screen_words"`
	DurationMillis int64  `json:"duration_ms"`
}

// Estimate is the estimated duration of a script when read aloud at a pace (SDD-019 section 4).
type Estimate struct {
	WordsPerMinute int             `json:"words_per_minute"`
	Words          int             `json:"words"`
	OnScreenWords  int             `json:"on_screen_words"`
	DurationMillis int64           `json:"duration_ms"`
	Plain          bool            `json:"plain"`
	Scenes         []SceneEstimate `json:"scenes"`
}

// CheckWordsPerMinute is the rule of the pace: 60 to 300; 0 means the default.
func CheckWordsPerMinute(wpm int) (int, *RuleError) {
	if wpm == 0 {
		return DefaultWordsPerMinute, nil
	}

	if wpm < MinWordsPerMinute || wpm > MaxWordsPerMinute {
		return 0, validation("words_per_minute", fmt.Sprintf("use %d to %d, or omit it for %d",
			MinWordsPerMinute, MaxWordsPerMinute, DefaultWordsPerMinute),
			"the pace %d words per minute is outside %d to %d", wpm, MinWordsPerMinute, MaxWordsPerMinute)
	}

	return wpm, nil
}

// EstimateDuration is the time the narration of s takes at wordsPerMinute (0 for the default), per scene and in
// total, in milliseconds. On-screen text and notes take no time.
func EstimateDuration(s Script, wordsPerMinute int) (Estimate, *RuleError) {
	wpm, rerr := CheckWordsPerMinute(wordsPerMinute)
	if rerr != nil {
		return Estimate{}, rerr
	}

	e := Estimate{WordsPerMinute: wpm, Plain: s.Plain, Scenes: make([]SceneEstimate, 0, len(s.Scenes))}

	for i := range s.Scenes {
		sc := &s.Scenes[i]
		words, onScreen := 0, 0

		for _, p := range sc.Narration {
			words += CountWords(p)
		}

		for _, p := range sc.OnScreen {
			onScreen += CountWords(p)
		}

		// Rounded to the nearest millisecond: floor(2 * words * 60000 / wpm), plus one, halved, in integers.
		ms := (int64(words)*120000/int64(wpm) + 1) / 2

		e.Scenes = append(
			e.Scenes,
			SceneEstimate{Title: sc.Title, Words: words, OnScreenWords: onScreen, DurationMillis: ms},
		)
		e.Words += words
		e.OnScreenWords += onScreen
		e.DurationMillis += ms
	}

	return e, nil
}
