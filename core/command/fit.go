package command

import "github.com/AlexGalhardo/stuarty/core/timeline"

// checkFit is R14: fades and transitions must still fit the duration.
func checkFit(it *timeline.Item, what string) *Error {
	if over := it.FadeIn + it.FadeOut - it.Duration; over > 0 {
		return Errorf(
			CodeInvalidState,
			"%s leaves fade_in %d and fade_out %d longer than the duration %d by %d frames.",
			what,
			it.FadeIn,
			it.FadeOut,
			it.Duration,
			over,
		).
			WithHint("Shorten the fades by %d frames with item.set_audio first.", over)
	}
	sum := 0
	for _, t := range []*timeline.Transition{it.TransitionIn, it.TransitionOut} {
		if t != nil {
			sum += t.Duration
		}
	}
	if over := sum - it.Duration; over > 0 {
		return Errorf(
			CodeInvalidState,
			"%s leaves transitions of %d frames in total, longer than the duration %d by %d frames.",
			what,
			sum,
			it.Duration,
			over,
		).
			WithHint("Shorten the transitions by %d frames with item.set_transition first.", over)
	}
	return nil
}

// fitFix is the fit fix of R14, for the commands that change a duration or a rate
// on their own (settings.set_fps, clip.set_speed, clip.remove_silences): reduce
// fade_out by the excess (not below 0), then fade_in; reduce transition_out, removing
// it when less than 1 frame is left, then transition_in the same way.
func fitFix(it *timeline.Item) {
	if over := it.FadeIn + it.FadeOut - it.Duration; over > 0 {
		cut := min(over, it.FadeOut)
		it.FadeOut -= cut
		it.FadeIn -= over - cut
	}
	over := transitionSum(it) - it.Duration
	over = shrinkTransition(&it.TransitionOut, over)
	shrinkTransition(&it.TransitionIn, over)
}

func transitionSum(it *timeline.Item) int {
	sum := 0
	for _, t := range []*timeline.Transition{it.TransitionIn, it.TransitionOut} {
		if t != nil {
			sum += t.Duration
		}
	}
	return sum
}

// shrinkTransition takes excess frames off *t (a copy is stored, never the shared
// value), drops it when less than 1 frame is left and returns what is still to take.
func shrinkTransition(t **timeline.Transition, excess int) int {
	if excess <= 0 || *t == nil {
		return max(excess, 0)
	}
	dur := (*t).Duration
	if dur-excess < 1 {
		*t = nil
		return excess - dur
	}
	cp := **t
	cp.Duration = dur - excess
	*t = &cp
	return 0
}
