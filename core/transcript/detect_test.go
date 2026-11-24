package transcript

import (
	"reflect"
	"strings"
	"testing"
)

// speak lays the words out 300 ms each with a 100 ms gap, so that a repetition is always inside the
// default window.
func speak(text string) []SpokenWord {
	var out []SpokenWord
	at := int64(0)
	for w := range strings.FieldsSeq(text) {
		out = append(out, SpokenWord{Text: w, StartMS: at, EndMS: at + 300})
		at += 400
	}
	return out
}

type found struct {
	kind     Kind
	strength Strength
	from, to int
}

func summary(s []Suggestion) []found {
	out := []found{}
	for _, x := range s {
		out = append(out, found{x.Kind, x.Strength, x.FromWord, x.ToWord})
	}
	return out
}

// A word already cut is never suggested again, the indices stay those of the transcript, and two copies that a cut
// made adjacent are a repetition.
func TestDetectShown(t *testing.T) {
	words := speak("go um go now um")
	shown := []bool{true, false, true, true, true}

	got := summary(DetectShown(words, shown, Options{Language: "en"}))
	want := []found{{KindRepetition, Strong, 0, 0}, {KindFiller, Strong, 4, 4}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	if got := DetectShown(words, make([]bool, len(words)), Options{Language: "en"}); len(got) != 0 {
		t.Errorf("with every word cut: %+v", got)
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name, lang, text string
		want             []found
	}{
		{"english hesitations", "en", "So, um, I think uh we should Umm start", []found{
			{KindFiller, Strong, 1, 1}, {KindFiller, Strong, 4, 4}, {KindFiller, Strong, 7, 7},
		}},
		{"english lengthened and punctuated", "en", "well Hmmm... that is Uhhh, fine", []found{
			{KindFiller, Strong, 1, 1}, {KindFiller, Strong, 4, 4},
		}},
		{"english weak markers", "en", "it is you know the one I mean ah", []found{
			{KindFiller, Weak, 2, 3}, {KindFiller, Weak, 6, 7}, {KindFiller, Weak, 8, 8},
		}},
		{
			"portuguese hesitations",
			"pt",
			"então ééé hoje hum eu quero ahn falar éé bem",
			[]found{
				{
					KindFiller,
					Strong,
					1,
					1,
				},
				{KindFiller, Strong, 3, 3},
				{KindFiller, Strong, 6, 6},
				{KindFiller, Strong, 8, 8},
			},
		},
		{"portuguese real words are not fillers", "pt", "um vídeo é rápido e é bom", []found{}},
		{"portuguese weak markers", "pt", "o assunto né tipo isso", []found{
			{KindFiller, Weak, 2, 2}, {KindFiller, Weak, 3, 3},
		}},
		{"unlisted language has no filler rule", "de", "also um äh ja", []found{}},
		{"repeated word", "en", "the the camera", []found{{KindRepetition, Strong, 0, 0}}},
		{
			"repeated phrase keeps the later copy",
			"en",
			"we should we should start",
			[]found{{KindRepetition, Strong, 0, 1}},
		},
		{"triple copy removes the first two", "en", "go go go now", []found{
			{KindRepetition, Strong, 0, 0}, {KindRepetition, Strong, 1, 1},
		}},
		{"intensifier is weak", "en", "that was very very good", []found{{KindRepetition, Weak, 2, 2}}},
		{"filler claims its words before a repetition", "en", "um um start", []found{
			{KindFiller, Strong, 0, 0}, {KindFiller, Strong, 1, 1},
		}},
		{"filler between copies", "en", "we should um we should go", []found{
			{KindFiller, Strong, 2, 2},
		}},
		{"no suggestion", "en", "this is a clean sentence", []found{}},
		{"empty", "en", "", []found{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := summary(Detect(speak(c.text), Options{Language: c.lang}))
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestDetectRepetitionWindow(t *testing.T) {
	words := []SpokenWord{{"the", 0, 300}, {"the", 1500, 1800}, {"cat", 1900, 2200}}
	if got := Detect(words, Options{Language: "en"}); len(got) != 0 {
		t.Fatalf("a 1200 ms gap is outside the default window: %v", got)
	}
	got := Detect(words, Options{Language: "en", RepeatWindowMS: 1500})
	if len(got) != 1 || got[0].FromWord != 0 || got[0].ToWord != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestDetectSuggestionFields(t *testing.T) {
	got := Detect(speak("go um now"), Options{Language: "en"})
	want := []Suggestion{{
		Kind: KindFiller, Strength: Strong, FromWord: 1, ToWord: 1, Text: "um", StartMS: 400, EndMS: 700,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestDetectIsDeterministicAndOrdered(t *testing.T) {
	words := speak("so um we should we should go uh go now hmm")
	first := Detect(words, Options{Language: "en"})
	for range 5 {
		if !reflect.DeepEqual(first, Detect(words, Options{Language: "en"})) {
			t.Fatal("two runs differ")
		}
	}
	for i := 1; i < len(first); i++ {
		if first[i].FromWord <= first[i-1].ToWord {
			t.Fatalf("suggestions overlap or are unordered: %v", first)
		}
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"Um,": "um", "“Hmm…”": "hmm", "don't": "don't", "'quoted'": "quoted", "É!": "é", "---": "",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
