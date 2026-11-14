package command

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func TestFitFix(t *testing.T) {
	tr := func(d int) *timeline.Transition { return &timeline.Transition{Kind: "fade", Duration: d} }
	// fade excess 10 comes off fade_out (15 -> 5), fade_in is kept.
	it := timeline.NewItem("itm_k000000001", timeline.ItemClip, vidTr, 0, 20)
	it.FadeIn, it.FadeOut = 5, 25
	fitFix(&it)
	if it.FadeIn != 5 || it.FadeOut != 15 {
		t.Errorf("fade_out first: %d %d", it.FadeIn, it.FadeOut)
	}
	// fade_out is exhausted, the rest comes off fade_in.
	it.FadeIn, it.FadeOut, it.Duration = 15, 10, 20
	fitFix(&it)
	if it.FadeIn != 15 || it.FadeOut != 5 {
		t.Errorf("fades %d %d", it.FadeIn, it.FadeOut)
	}
	it.FadeIn, it.FadeOut, it.Duration = 15, 10, 4
	fitFix(&it)
	if it.FadeOut != 0 || it.FadeIn != 4 {
		t.Errorf("fades %d %d", it.FadeIn, it.FadeOut)
	}

	// transition_out takes the excess first and is kept while it has 1 frame.
	shared := tr(10)
	it = timeline.NewItem("itm_k000000002", timeline.ItemClip, vidTr, 0, 20)
	it.TransitionIn, it.TransitionOut = shared, tr(15)
	fitFix(&it)
	if it.TransitionOut.Duration != 10 || it.TransitionIn != shared || shared.Duration != 10 {
		t.Errorf("transitions %+v %+v", it.TransitionIn, it.TransitionOut)
	}
	// transition_out is removed, transition_in takes what is left.
	it = timeline.NewItem("itm_k000000003", timeline.ItemClip, vidTr, 0, 10)
	it.TransitionIn, it.TransitionOut = tr(8), tr(5)
	fitFix(&it)
	if it.TransitionOut == nil || it.TransitionOut.Duration != 2 || it.TransitionIn.Duration != 8 {
		t.Errorf("transitions %+v %+v", it.TransitionIn, it.TransitionOut)
	}
	it.TransitionIn, it.TransitionOut, it.Duration = tr(8), tr(5), 3
	fitFix(&it)
	if it.TransitionOut != nil || it.TransitionIn == nil || it.TransitionIn.Duration != 3 {
		t.Errorf("transitions %+v %+v", it.TransitionIn, it.TransitionOut)
	}
	it.TransitionIn, it.TransitionOut, it.Duration = tr(8), tr(5), 0
	fitFix(&it)
	if it.TransitionOut != nil || it.TransitionIn != nil {
		t.Errorf("transitions %+v %+v", it.TransitionIn, it.TransitionOut)
	}
}

func TestCheckFitNamesTheAmountAndTheCommand(t *testing.T) {
	it := timeline.NewItem("itm_k000000001", timeline.ItemClip, vidTr, 0, 20)
	it.FadeIn, it.FadeOut = 15, 15
	err := checkFit(&it, "Trimming")
	if err == nil || err.Code != CodeInvalidState || !strings.Contains(err.Hint, "10 frames") ||
		!strings.Contains(err.Hint, "item.set_audio") {
		t.Errorf("%+v", err)
	}
	it.FadeIn, it.FadeOut = 0, 0
	it.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 15}
	it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 15}
	err = checkFit(&it, "Trimming")
	if err == nil || !strings.Contains(err.Hint, "item.set_transition") {
		t.Errorf("%+v", err)
	}
	it.TransitionOut.Duration = 5
	if err := checkFit(&it, "x"); err != nil {
		t.Errorf("%+v", err)
	}
}

func TestMaxDur(t *testing.T) {
	if got := maxDur(900, 800, timeline.Rational{Num: 1, Den: 1}); got != 100 {
		t.Errorf("%d", got)
	}
	if got := maxDur(900, 800, timeline.Rational{Num: 2, Den: 1}); got != 50 {
		t.Errorf("%d", got)
	}
	if got := maxDur(900, 1000, timeline.Rational{Num: 1, Den: 1}); got != 0 {
		t.Errorf("%d", got)
	}
}
