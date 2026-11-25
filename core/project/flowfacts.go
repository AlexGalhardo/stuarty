package project

import (
	"slices"
	"strings"
)

// Spend is what one kind of job cost a project, settled: the real cost of a ledger entry that finished, in one
// currency (SDD-015 section 3). The service reads the ledger and hands the sums over.
type Spend struct {
	JobKind  string
	Currency string
	Micros   int64
}

// stepSkills is the skill of the library that leads each step, by the kind of the workflow it is in
// (SDD-018 section 16). A step with no entry has no skill. Whether the person still has the skill, reviewed, is the
// service's to say (Facts.Skills): the table only names it.
var stepSkills = map[Intent]map[StepID]string{
	KindVideo: {
		StepBriefing:    "brief-a-production",
		StepScript:      "short-script",
		StepEditing:     "short-vertical-video",
		StepFinishing:   "captions-for-shorts",
		StepPublication: "caption-per-network",
	},
	KindText: {
		StepBriefing:    "brief-a-production",
		StepWriting:     "write-a-hook",
		StepPublication: "caption-per-network",
	},
	KindImage: {
		StepBriefing: "brief-a-production",
	},
}

// SkillOf is the skill that leads a step of a workflow of the kind, or "" when none does.
func SkillOf(kind Intent, id StepID) string { return stepSkills[kind][id] }

// stepOfJobKind is the step of a workflow that the cost of a job kind belongs to: the writing or the script for
// generated text, the editing for generated pictures, voice and video, the publication for a post. A kind the
// table does not know, or whose step the workflow does not hold, belongs to no step.
func stepOfJobKind(kind string, steps []Step) (StepID, bool) {
	var want []StepID

	switch {
	case kind == "text.generate":
		want = []StepID{StepWriting, StepScript}
	case strings.HasPrefix(kind, "image.") || kind == "voice.generate" || kind == "video.generate":
		want = []StepID{StepEditing}
	case strings.HasPrefix(kind, "publish."):
		want = []StepID{StepPublication}
	}

	for _, id := range want {
		if slices.ContainsFunc(steps, func(s Step) bool { return s.ID == id }) {
			return id, true
		}
	}

	return "", false
}

// applyFlowFacts sets the skill of each step that the library offers and the settled cost of each step.
func applyFlowFacts(w *Workflow, f *Facts) {
	for i := range w.Steps {
		st := &w.Steps[i]
		if name := SkillOf(w.Kind, st.ID); name != "" && slices.Contains(f.Skills, name) {
			st.Skill = name
		}
	}

	for _, sp := range f.Spent {
		id, ok := stepOfJobKind(sp.JobKind, w.Steps)
		if !ok || sp.Micros <= 0 || sp.Currency == "" {
			continue
		}

		for i := range w.Steps {
			if w.Steps[i].ID == id {
				w.Steps[i].Spent = addCost(w.Steps[i].Spent, sp.Currency, sp.Micros)
			}
		}
	}

	for i := range w.Steps {
		slices.SortFunc(w.Steps[i].Spent, func(a, b Cost) int { return strings.Compare(a.Currency, b.Currency) })
	}
}

func addCost(sums []Cost, currency string, micros int64) []Cost {
	for i := range sums {
		if sums[i].Currency == currency {
			sums[i].Micros += micros

			return sums
		}
	}

	return append(sums, Cost{Micros: micros, Currency: currency})
}
