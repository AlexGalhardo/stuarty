package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/project"
	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/core/writing"
)

// JobTextDraft is the job kind of a draft of a text made with a model (SDD-019 section 6).
const JobTextDraft = "text.draft"

// DraftArgs are the arguments of a text.draft job.
type DraftArgs struct {
	ItemID       string   `json:"item_id"`
	ConnectionID string   `json:"connection_id"`
	Model        string   `json:"model"`
	Instruction  string   `json:"instruction"`
	Network      string   `json:"network,omitempty"`
	MaxTokens    int      `json:"max_tokens,omitempty"`
	Temperature  *float64 `json:"temperature,omitempty"`
}

// draftInput is the input column of a text.draft job: what text.generate keeps (the request, the price the person
// saw and the estimate) and the text the answer is written to.
type draftInput struct {
	textInput

	ItemID  string `json:"item_id"`
	Network string `json:"network,omitempty"`
}

// textDraftKind is the approvable kind of a draft: text.generate with a prompt that core/writing built and a text to
// put the answer in, under the same estimate, ceilings and approval (SDD-015 section 5, SDD-016 section 6).
func (s *svc) textDraftKind() ApprovableKind {
	return ApprovableKind{
		Name: JobTextDraft, Scope: ScopeEdit, ProjectWide: true,
		Prepare: s.prepareDraft, Run: s.runDraft,
	}
}

// TextDraft asks a model for a draft of a text. It starts a text.draft job that shows its estimate and waits for a
// person or a rule; when it succeeds the draft is a new revision of the text, never an overwrite.
func (s *svc) TextDraft(ctx context.Context, req TextDraftRequest) (*Job, error) {
	_, err := s.authorizeProject(ctx, "TextDraft", req.ProjectID)
	if err != nil {
		return nil, err
	}

	args, err := json.Marshal(DraftArgs{
		ItemID: req.ItemID, ConnectionID: req.ConnectionID, Model: req.Model, Instruction: req.Instruction,
		Network: req.Network, MaxTokens: req.MaxTokens, Temperature: req.Temperature,
	})
	if err != nil {
		return nil, newError(command.CodeInternal, "the request cannot be encoded.")
	}

	return s.JobStart(ctx, JobStartRequest{
		ProjectID: req.ProjectID, Kind: JobTextDraft, Args: args, WaitMS: req.WaitMS,
		DryRun: req.DryRun,
	})
}

func decodeDraftArgs(raw json.RawMessage) (DraftArgs, *Error) {
	var a DraftArgs

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if len(bytes.TrimSpace(raw)) == 0 || dec.Decode(&a) != nil || dec.More() {
		return a, newError(
			command.CodeValidation,
			"args must be one JSON object with item_id, connection_id, model, instruction and optionally network, "+
				"max_tokens and temperature.",
		).withPath("args")
	}

	return a, nil
}

// prepareDraft checks the draft, builds the prompt and computes the estimate. It makes no outbound call.
func (s *svc) prepareDraft(ctx context.Context, req JobStartRequest) (Prepared, error) {
	a, aerr := decodeDraftArgs(req.Args)
	if aerr != nil {
		return Prepared{}, aerr
	}

	c, ok := CallerFrom(ctx)
	if !ok {
		return Prepared{}, newError(CodeUnauthorized, "the request carries no credentials.")
	}

	if v := writing.CheckInstruction(a.Instruction); v != nil {
		return Prepared{}, writingViolation(v).withPath("args." + v.Path)
	}

	repo := s.store.Productions()

	head, err := s.loadProduction(ctx, repo, c, req.ProjectID)
	if err != nil {
		return Prepared{}, err
	}

	it, ierr := repo.Item(ctx, a.ItemID)
	if ierr != nil || it.ProjectID != head.ID {
		if ierr == nil || errors.Is(ierr, ErrNotFound) {
			return Prepared{}, itemNotFound(a.ItemID).withPath("args.item_id")
		}

		return Prepared{}, asError(ierr)
	}

	if _, err := checkIsText(&it); err != nil {
		return Prepared{}, err
	}

	if it.State == ProjectTrashed {
		return Prepared{}, textInTrash(it.ID)
	}

	kind := writing.Kind(it.TextKind)
	if v := writing.CheckDraftNetwork(kind, a.Network); v != nil {
		return Prepared{}, writingViolation(v).withPath("args." + v.Path)
	}

	rows, err := repo.Items(ctx, head.ID)
	if err != nil {
		return Prepared{}, asError(err)
	}

	in := writing.DraftInput{
		Kind:        kind,
		Title:       it.Title,
		Current:     it.Body,
		Instruction: a.Instruction,
		Network:     a.Network,
	}
	if b := briefOf(rows); b != nil {
		in.Brief = b.Body
	}

	system, prompt := writing.DraftPrompt(&in)

	request := provider.TextRequest{
		Model: a.Model, System: system, Prompt: prompt, MaxTokens: a.MaxTokens, Temperature: a.Temperature,
	}

	targs, merr := json.Marshal(TextArgs{ConnectionID: a.ConnectionID, TextRequest: request})
	if merr != nil {
		return Prepared{}, merr
	}

	prep, err := s.prepareText(ctx, JobStartRequest{ProjectID: req.ProjectID, Kind: JobTextGenerate, Args: targs})
	if err != nil {
		return Prepared{}, err
	}

	var ti textInput
	if err := json.Unmarshal(prep.Input, &ti); err != nil {
		return Prepared{}, newError(command.CodeInternal, "the input of the draft cannot be read.")
	}

	input, merr := json.Marshal(draftInput{textInput: ti, ItemID: it.ID, Network: a.Network})
	if merr != nil {
		return Prepared{}, merr
	}

	_, gen, perr := s.textProvider(ctx, a.ConnectionID)
	if perr != nil {
		return Prepared{}, perr
	}

	prep.Input = input
	prep.Summary = draftSummary(gen.Descriptor().Name, &it, a.Network, &ti.Request, a.Instruction)

	return prep, nil
}

// draftSummary is what a person reads before approving a draft (SDD-016 section 6): the text, the provider, the
// model and an excerpt of the task, all cleaned of what could hide something, in at most 500 characters.
func draftSummary(
	providerName string,
	it *ItemRow,
	network string,
	r *provider.TextRequest,
	instruction string,
) string {
	what := "the " + it.TextKind + " “" + excerpt(it.Title, 60) + "”"
	if network != "" {
		what += " for " + network
	}

	head := fmt.Sprintf("Draft %s with %s, model %s: up to %d tokens back.", what, providerName, r.Model, r.MaxTokens)

	room := min(maxExcerpt, core.MaxSummary-utf8.RuneCountInString(head)-len(" Task: “”…"))
	if room < 10 {
		return head
	}

	return head + " Task: “" + excerpt(instruction, room) + "”…"
}

// runDraft is the job once approved: the generation of text.generate, and then the answer written as the next
// revision of the text with the provider, the model and the real cost on it.
func (s *svc) runDraft(ctx context.Context, run JobRun) (JobOutcome, error) {
	var in draftInput
	if err := json.Unmarshal(run.Input, &in); err != nil {
		return JobOutcome{}, newError(command.CodeInternal, "the input of the job is not readable.")
	}

	raw, merr := json.Marshal(in.textInput)
	if merr != nil {
		return JobOutcome{}, merr
	}

	textRun := run
	textRun.Input = raw

	out, err := s.runText(ctx, textRun)
	if err != nil {
		return out, err
	}

	var res JobResult
	if err := json.Unmarshal(out.Result, &res); err != nil || res.Text == nil {
		return out, newError(command.CodeInternal, "the result of the generation is not readable.")
	}

	body := writing.CleanDraft(res.Text.Text)
	if body == "" {
		return out, newError(CodeProvider, "the provider answered with no text.").
			withHint("try again, or with another model")
	}

	rev, err := s.applyDraft(ctx, &run, &in, res.Text, body)
	if err != nil {
		return out, err
	}

	res.Text.ItemID, res.Text.Revision = in.ItemID, rev

	out.Result, merr = json.Marshal(res)
	if merr != nil {
		return out, merr
	}

	return out, nil
}

// applyDraft writes the draft as a new revision of its text, in one transaction. The text may have changed since the
// job began: the draft is still the next revision, so what the other writer wrote stays one revision before it.
func (s *svc) applyDraft(ctx context.Context, run *JobRun, in *draftInput, res *TextResult, body string) (int, error) {
	var revision int

	err := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		repo := tx.Productions()

		it, err := repo.Item(ctx, in.ItemID)
		if err != nil {
			return textGone(err)
		}

		head, err := repo.Get(ctx, it.ProjectID)
		if err != nil {
			return textGone(err)
		}

		if it.State == ProjectTrashed || head.State == ProjectTrashed {
			return newError(
				command.CodeInvalidState,
				"the text %s was moved to the trash while the draft was written.",
				it.ID,
			).
				withHint("restore the text and ask for the draft again; the cost of this one was recorded")
		}

		latest, err := repo.TextLatest(ctx, it.ID)
		if err != nil {
			return textGone(err)
		}

		next := body
		if in.Network != "" {
			next = writing.SetVariant(latest.Body, in.Network, body)
		}

		if v := project.CheckText(next); v != nil {
			return violation(v)
		}

		now := s.nowMs()
		micros := res.Cost.Micros
		rev := TextRevisionRow{
			ItemID: it.ID, Revision: writing.NextRevision(latest.Revision), Body: next, Actor: run.Actor,
			Source: string(writing.SourceGenerated), Provider: res.Provider, Model: res.Model, CostMicros: &micros,
			Currency: res.Cost.Currency, JobID: run.ID, CreatedMs: now,
		}

		if err := repo.AddTextRevision(ctx, rev, writing.PruneBefore(rev.Revision)); err != nil {
			return err
		}

		if err := repo.Touch(ctx, head.ID, now); err != nil {
			return err
		}

		if err := appendAudit(ctx, tx, now, run.Actor, "text.draft", head.ID, it.ID); err != nil {
			return err
		}

		revision = rev.Revision

		tx.AfterCommit(s.emitProject(&head, head.State, run.Actor, now))

		return nil
	})
	if err != nil {
		return 0, asError(err)
	}

	return revision, nil
}

func textGone(err error) error {
	if errors.Is(err, ErrNotFound) {
		return newError(command.CodeNotFound, "the text of the draft no longer exists.").
			withHint("the cost of the draft was recorded")
	}

	return err
}
