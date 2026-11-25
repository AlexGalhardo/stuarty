package project

import (
	"slices"
	"strings"
)

// StepID names a step of a workflow (SDD-000 section 1).
type StepID string

// The steps a workflow can hold, in the canonical order the steps of every kind follow (SDD-005 section 5.12.4).
const (
	StepBriefing    StepID = "briefing"
	StepChoice      StepID = "choice"
	StepScript      StepID = "script"
	StepWriting     StepID = "writing"
	StepRecording   StepID = "recording"
	StepEditing     StepID = "editing"
	StepFinishing   StepID = "finishing"
	StepReview      StepID = "review"
	StepApproval    StepID = "approval"
	StepPublication StepID = "publication"
)

// StepIDs lists every step id in the canonical order.
func StepIDs() []StepID {
	return []StepID{
		StepBriefing, StepChoice, StepScript, StepWriting, StepRecording, StepEditing, StepFinishing, StepReview,
		StepApproval, StepPublication,
	}
}

// StepState is where a step stands.
type StepState string

// The states of a step. A fact the service lacks gives StepUnknown, never StepDone.
const (
	StepDone     StepState = "done"
	StepCurrent  StepState = "current"
	StepWaiting  StepState = "waiting" // waits for a person
	StepUpcoming StepState = "upcoming"
	StepSkipped  StepState = "skipped"
	StepUnknown  StepState = "unknown"
)

// Actor is who does a step.
type Actor string

// The actors of a step.
const (
	ActorPerson Actor = "person"
	ActorAgent  Actor = "agent"
	ActorBoth   Actor = "both"
)

// Cost is an amount of one currency, in millionths of its unit (the estimate of an approval request).
type Cost struct {
	Micros   int64  `json:"micros"`
	Currency string `json:"currency"`
}

// Step is one step of a workflow as a surface shows it. The skill that does the step is not here: it comes from
// the skills of SDD-018 at read time, and the table of the core suggests none.
type Step struct {
	ID       StepID    `json:"id"`
	State    StepState `json:"state"`
	Optional bool      `json:"optional"`
	Actor    Actor     `json:"actor"`
	// Tool is the name of the service operation that does the step (SDD-000 section 1); empty when none does.
	Tool string `json:"tool,omitempty"`
	// Cost is what the approvals of the step estimate or cost; left out when there is none.
	Cost []Cost `json:"cost,omitempty"`
}

// Workflow is the real path of a project from its briefing to its publication.
type Workflow struct {
	// Kind is the effective kind of the project.
	Kind  Intent `json:"kind"`
	Steps []Step `json:"steps"`
	// Current is the first step that is open or waits for a person; empty when none is.
	Current StepID `json:"current,omitempty"`
	// Next is the first open step after Current; empty when none is.
	Next StepID `json:"next,omitempty"`
}

// Step returns the step with the id, and false when the workflow has none.
func (w *Workflow) Step(id StepID) (Step, bool) {
	for i := range w.Steps {
		if w.Steps[i].ID == id {
			return w.Steps[i], true
		}
	}

	return Step{}, false
}

// Facts is everything ComputeWorkflow reads. Each is something the service already has; what it could not read
// is marked, and the steps that depend on it read unknown.
type Facts struct {
	// Items are the items of the project, its publications among them when PublicationsKnown.
	Items []Item
	// Skipped are the steps the person skipped.
	Skipped []StepID
	// PublicationsKnown is false when the publications of the project could not be read: they are then ignored.
	PublicationsKnown bool
	// TimelineAssets and TimelineItems are what the first active timeline holds; zero without a timeline.
	TimelineAssets, TimelineItems int
	// Costs is the sum, by currency, of the estimates of the approvals of the project that are pending or approved.
	Costs []Cost
}

type stepDef struct {
	id    StepID
	actor Actor
	tool  string
}

var stepTable = map[StepID]stepDef{
	StepBriefing:    {StepBriefing, ActorBoth, "BriefingAnswer"},
	StepChoice:      {StepChoice, ActorPerson, "ProjectItemAdd"},
	StepScript:      {StepScript, ActorBoth, "TextWrite"},
	StepWriting:     {StepWriting, ActorBoth, "TextWrite"},
	StepRecording:   {StepRecording, ActorPerson, "MediaImport"},
	StepEditing:     {StepEditing, ActorBoth, "Edit"},
	StepFinishing:   {StepFinishing, ActorBoth, "JobStart"},
	StepReview:      {StepReview, ActorPerson, "TextWrite"},
	StepApproval:    {StepApproval, ActorPerson, "ApprovalDecide"},
	StepPublication: {StepPublication, ActorBoth, "PublicationCreate"},
}

// stepsOf lists the steps of a project, by the canonical order. A medium the project holds adds its own steps and
// removes none, so that what is done stays done when a video joins a text.
func stepsOf(kind Intent, h holdings) []StepID {
	if kind == KindUndecided {
		return []StepID{StepBriefing, StepChoice}
	}

	text := kind == KindText
	video := holdsVideo(kind, h)
	image := kind == KindImage || h.images > 0
	media := video || image

	var out []StepID

	for _, id := range StepIDs() {
		in := false

		switch id {
		case StepBriefing:
			in = true
		case StepChoice:
		case StepScript, StepRecording, StepFinishing:
			in = video
		case StepWriting, StepReview:
			in = text
		case StepEditing:
			in = media
		case StepApproval, StepPublication:
			in = text || media
		}

		if in {
			out = append(out, id)
		}
	}

	return out
}

// optional reports whether a person may skip the step in a project. A copy ends in copy or export (D-137): the
// publication of a text project that holds no post and no publication is optional.
func optional(id StepID, kind Intent, h holdings) bool {
	switch id {
	case StepRecording, StepFinishing:
		return true
	case StepPublication:
		return kind == KindText && !h.hasPost && h.publications == 0
	case StepBriefing, StepChoice, StepScript, StepWriting, StepEditing, StepReview, StepApproval:
	}

	return false
}

// stepOpen is the internal state of a step with no fact for or against it; order turns it into current or
// upcoming, so it never leaves ComputeWorkflow.
const stepOpen StepState = "open"

// baseState is what the facts say about a step, before the order of the steps decides which one is current.
func baseState(id StepID, kind Intent, h holdings, f *Facts) StepState {
	known := f.PublicationsKnown
	others := h.publications - h.pending - h.published // a draft, or an approved one that waits for its time

	switch id {
	case StepBriefing:
		// The briefing says what to make, for whom and by when. Work that already stands behind it (a text, a
		// clip on the timeline, an output, a publication) means the project went past it, so the trail never
		// shows later steps done under a briefing that is still the current step.
		return doneIf(h.hasBrief || h.writing || f.TimelineAssets > 0 || h.outputs > 0 || h.publications > 0)
	case StepChoice:
		return stepOpen
	case StepScript:
		// A clip already on the timeline means the video went past its script (see the briefing).
		return doneIf(h.writing || f.TimelineAssets > 0)
	case StepWriting:
		return doneIf(h.writing)
	case StepRecording:
		return doneIf(f.TimelineAssets > 0)
	case StepEditing:
		if kind == KindImage || !holdsVideo(kind, h) {
			// No image document belongs to a project yet, so the editing of an image has no fact to read, and a
			// timeline alone does not say the image is edited.
			return StepUnknown
		}

		return doneIf(f.TimelineItems > 0)
	case StepFinishing:
		return doneIf(h.outputs > 0)
	case StepReview:
		switch {
		case h.outputs > 0 || h.publications > 0:
			return StepDone
		case !known:
			return StepUnknown
		}

		return stepOpen
	case StepApproval:
		switch {
		case !known:
			return StepUnknown
		case h.pending > 0:
			return StepWaiting
		case h.published > 0:
			return StepDone
		case others > 0:
			return StepUnknown
		}

		return stepOpen
	case StepPublication:
		switch {
		case !known:
			return StepUnknown
		case h.published > 0:
			return StepDone
		case others > 0:
			return StepUnknown
		}

		return stepOpen
	}

	return StepUnknown
}

func doneIf(ok bool) StepState {
	if ok {
		return StepDone
	}

	return stepOpen
}

// ComputeWorkflow is the workflow of a project: the steps of its kind with their actor, tool and cost, and the
// state of each computed from facts the service already has (SDD-005 section 5.12.4). It is pure and does no I/O.
func ComputeWorkflow(stored Intent, f *Facts) Workflow {
	items := f.Items
	if !f.PublicationsKnown {
		items = slices.DeleteFunc(slices.Clone(items), func(it Item) bool { return it.Kind == Publication })
	}

	h := holdingsOf(items)
	kind := EffectiveKind(stored, items)
	ids := stepsOf(kind, h)
	w := Workflow{Kind: kind, Steps: make([]Step, 0, len(ids))}

	for _, id := range ids {
		def := stepTable[id]
		st := Step{ID: id, Optional: optional(id, kind, h), Actor: def.actor, Tool: def.tool}

		if id == StepEditing && !holdsVideo(kind, h) {
			st.Tool = "ImageEdit"
		}

		if id == StepApproval && len(f.Costs) > 0 {
			st.Cost = slices.Clone(f.Costs)
		}

		st.State = baseState(id, kind, h, f)

		if st.Optional && st.State != StepDone && slices.Contains(f.Skipped, id) {
			st.State = StepSkipped
		}

		w.Steps = append(w.Steps, st)
	}

	followSkippedPublication(&w)
	order(&w)

	return w
}

// holdsVideo reports whether the video steps are in the workflow: the project is a video or holds a timeline.
func holdsVideo(kind Intent, h holdings) bool { return kind == KindVideo || h.timelines > 0 }

// followSkippedPublication skips the approval of a publication the person skipped: the approval exists only to
// let a publication out, so a copy that ends in copy or export (D-137) is not left waiting at it. An approval
// that a fact decides (done, waiting or unknown) keeps its state, and the approval is never skipped on its own.
func followSkippedPublication(w *Workflow) {
	if p, ok := w.Step(StepPublication); !ok || p.State != StepSkipped {
		return
	}

	for i := range w.Steps {
		if w.Steps[i].ID == StepApproval && w.Steps[i].State == stepOpen {
			w.Steps[i].State = StepSkipped
		}
	}
}

// order turns the open steps into current and upcoming and names the current and the next step.
func order(w *Workflow) {
	for i := range w.Steps {
		st := &w.Steps[i]

		if st.State == StepWaiting && w.Current == "" {
			w.Current = st.ID
		}

		if st.State != stepOpen {
			continue
		}

		if w.Current == "" {
			st.State, w.Current = StepCurrent, st.ID
		} else {
			st.State = StepUpcoming

			if w.Next == "" {
				w.Next = st.ID
			}
		}
	}
}

// ParseSkipped reads the steps a project row stores: ids joined by commas. An id that is not a step is dropped.
func ParseSkipped(s string) []StepID {
	var out []StepID

	for part := range strings.SplitSeq(s, ",") {
		if id := StepID(part); slices.Contains(StepIDs(), id) && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}

	return out
}

// JoinSkipped is the form a project row stores: the ids in the canonical order, joined by commas.
func JoinSkipped(ids []StepID) string {
	var parts []string

	for _, id := range StepIDs() {
		if slices.Contains(ids, id) {
			parts = append(parts, string(id))
		}
	}

	return strings.Join(parts, ",")
}

// CheckStep is the rule of ProjectStepSkip against the workflow a project has now: the step must be one of the
// workflow and optional in it (E_VALIDATION, path "step"), and a step that is done cannot be skipped
// (E_INVALID_STATE). A restore is held to the first two.
func CheckStep(w *Workflow, step string, restore bool) (StepID, *RuleError) {
	id := StepID(step)
	if !slices.Contains(StepIDs(), id) {
		return "", validation("step", "use one of: "+joinIDs(StepIDs()), "the step %q is not known", step)
	}

	st, ok := w.Step(id)
	if !ok {
		return "", validation("step", "use one of the steps of this project: "+joinIDs(stepIDsOf(w)),
			"the step %q is not in the workflow of this project", step)
	}

	if !st.Optional {
		return "", validation("step", "only an optional step can be skipped: "+joinIDs(optionalIDsOf(w)),
			"the step %q is required", step)
	}

	if !restore && st.State == StepDone {
		return "", &RuleError{
			Code: "E_INVALID_STATE", Path: "step", Hint: "a step that is done needs no skip",
			Message: "the step " + step + " is already done",
		}
	}

	return id, nil
}

func stepIDsOf(w *Workflow) []StepID {
	out := make([]StepID, len(w.Steps))
	for i := range w.Steps {
		out[i] = w.Steps[i].ID
	}

	return out
}

func optionalIDsOf(w *Workflow) []StepID {
	var out []StepID

	for i := range w.Steps {
		if w.Steps[i].Optional {
			out = append(out, w.Steps[i].ID)
		}
	}

	return out
}

func joinIDs(ids []StepID) string {
	if len(ids) == 0 {
		return "none"
	}

	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = string(id)
	}

	return strings.Join(parts, ", ")
}
