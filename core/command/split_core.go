package command

import "github.com/AlexGalhardo/stuarty/core/timeline"

// splitCore is S(X, Tc) of R17, without the words (the command handles those):
// the left part keeps the id and the front, the right part is a copy with newID
// that starts at X.start + tc and takes transition_out and fade_out. reads says
// whether X reads from a video or audio asset, which advances source_in on the
// right. Keyframes follow R13; fades and transitions longer than their part are
// shortened to it.
func splitCore(x *timeline.Item, newID string, tc int, reads bool) (left, right timeline.Item) {
	left, right = kSplit(x, tc)
	left.Duration = tc
	right.ID, right.Start, right.Duration = newID, x.Start+tc, x.Duration-tc
	switch {
	case x.Freeze:
		// Both parts hold the same frame.
	case reads && x.Reverse:
		// The left part plays the end of the source range, the right part the start (SDD-004 section 3.12).
		left.SourceIn = x.SourceIn + x.SourceSpan() - ceilDiv(tc*x.Speed.Num, x.Speed.Den)
	case reads:
		right.SourceIn = x.SourceIn + tc*x.Speed.Num/x.Speed.Den
	}
	left.FadeOut, left.TransitionOut = 0, nil
	right.FadeIn, right.TransitionIn, right.Slot = 0, nil, nil
	for _, p := range []*timeline.Item{&left, &right} {
		p.FadeIn, p.FadeOut = min(p.FadeIn, p.Duration), min(p.FadeOut, p.Duration)
		p.TransitionIn, p.TransitionOut = clampTransition(
			p.TransitionIn,
			p.Duration,
		), clampTransition(
			p.TransitionOut,
			p.Duration,
		)
	}
	return left, right
}

func clampTransition(t *timeline.Transition, limit int) *timeline.Transition {
	if t == nil || t.Duration <= limit {
		return t
	}
	cp := *t
	cp.Duration = limit
	return &cp
}
