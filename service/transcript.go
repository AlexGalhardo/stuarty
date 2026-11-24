package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/transcript"
	"github.com/AlexGalhardo/stuarty/service/textview"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// The reads of editing by text (SDD-023 section 5). They carry no rule: core/transcript maps the words to the
// timeline and finds the fillers, and the cut itself is the command transcript.delete_range of core/command. Both
// read the newest transcript of the asset and start no job.

// TranscriptViewRequest asks for the words of an asset with the timeline range of each.
type TranscriptViewRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"` // SDD-003 section 1.7
	AssetID    string `json:"asset_id"`
}

// TranscriptWord is a word of the transcript with where the timeline shows it. Start and End are frames of the
// asset, the ranges frames of the timeline; a word with no range was cut or lies outside every item.
type TranscriptWord struct {
	Index  int                `json:"index"`
	Text   string             `json:"text"   untrusted:"true"`
	Start  int                `json:"start"`
	End    int                `json:"end"`
	Ranges []transcript.Range `json:"ranges"`
}

// TranscriptViewResult is the transcript as the timeline shows it at one revision.
type TranscriptViewResult struct {
	UntrustedNotice string           `json:"untrusted_notice,omitempty"`
	ProjectID       string           `json:"project_id"`
	Revision        int64            `json:"revision"`
	AssetID         string           `json:"asset_id"`
	Language        string           `json:"language"`
	Words           []TranscriptWord `json:"words"`
	// SkippedItems show the asset but play it with a speed curve or backwards, so no word is placed on them.
	SkippedItems []string `json:"skipped_items"`
	Text         string   `json:"text"`
}

// TranscriptDetectRequest asks for the filler words and repetitions of an asset.
type TranscriptDetectRequest struct {
	ProjectID  string `json:"project_id"`
	TimelineID string `json:"timeline_id,omitempty"`
	AssetID    string `json:"asset_id"`
	// RepeatWindowMS is the longest gap between the two copies of a repetition; 0 is the default, 700.
	RepeatWindowMS int64 `json:"repeat_window_ms,omitempty"`
}

// TranscriptSuggestion is a range of words a person may remove. Nothing is removed by asking for it.
type TranscriptSuggestion struct {
	Kind     string `json:"kind"`
	Strength string `json:"strength"`
	FromWord int    `json:"from_word"`
	ToWord   int    `json:"to_word"`
	Text     string `json:"text"      untrusted:"true"`
	StartMS  int64  `json:"start_ms"`
	EndMS    int64  `json:"end_ms"`
}

// TranscriptDetectResult is the list of suggestions at one revision. The revision does not change by asking.
type TranscriptDetectResult struct {
	UntrustedNotice string                 `json:"untrusted_notice,omitempty"`
	ProjectID       string                 `json:"project_id"`
	Revision        int64                  `json:"revision"`
	AssetID         string                 `json:"asset_id"`
	Language        string                 `json:"language"`
	Suggestions     []TranscriptSuggestion `json:"suggestions"`
	// Rule says what was checked, so that an empty list is not read as "no fillers": the language the rules
	// know, or that only repetitions were checked.
	Rule string `json:"rule"`
	Text string `json:"text"`
}

// transcriptSource is the transcript of an asset, in the two forms the reads need.
type transcriptSource struct {
	row      ProjectRow
	doc      *timeline.Document
	language string
	spoken   []transcript.SpokenWord
	frames   []command.ResolvedWord
}

// loadTranscript authorizes the call, loads the timeline and reads the newest transcript of the asset.
func (s *svc) loadTranscript(
	ctx context.Context,
	method, projectID, timelineID, assetID string,
) (*transcriptSource, error) {
	c, err := s.authorizeProject(ctx, method, projectID)
	if err != nil {
		return nil, err
	}

	if err := s.useTimeline(ctx, c, &projectID, timelineID); err != nil {
		return nil, err
	}

	if !timeline.ValidID(timeline.PrefixAsset, assetID) {
		return nil, newError(command.CodeValidation, "the asset id is not valid.").withPath("asset_id").
			withHint("an asset id is ast_ and 10 characters from 0-9 and a-z, without i, l, o and u")
	}

	row, lerr := s.loadProject(ctx, c, projectID)
	if lerr != nil {
		return nil, lerr
	}

	doc, derr := decodeRow(&row)
	if derr != nil {
		return nil, derr
	}

	asset, ok := doc.Assets[assetID]
	if !ok {
		return nil, newError(command.CodeNotFound, "asset %s is not in the timeline.", assetID).withPath("asset_id").
			withHint("add a clip of the asset to the timeline first; media_inspect lists the assets of the project")
	}

	if asset.Kind == timeline.AssetImage || asset.Kind == timeline.AssetLUT ||
		asset.Kind == timeline.AssetVideo && !asset.HasAudio {
		return nil, newError(command.CodeTypeMismatch, "asset %s has no audio, so it has no transcript.", assetID).
			withPath("asset_id").withHint("choose an asset whose audio was transcribed")
	}

	arts, aerr := s.store.Assets().Artifacts(ctx, asset.Hash)
	if aerr != nil {
		return nil, asError(aerr)
	}

	raw := s.newestTranscript(arts)
	if raw == nil {
		return nil, newError(command.CodeInvalidState, "asset %s has no transcript yet.", assetID).
			withPath("asset_id").
			withHint("start the transcription with job_start (kind \"transcribe\") on this asset, wait for it to succeed, then ask again")
	}

	src := &transcriptSource{row: row, doc: doc}

	var in struct {
		Language string `json:"language"`
		Words    []struct {
			Text    string `json:"text"`
			StartMS int64  `json:"start_ms"`
			EndMS   int64  `json:"end_ms"`
		} `json:"words"`
	}

	if json.Unmarshal(raw, &in) != nil {
		return nil, newError(command.CodeInvalidState, "the transcript of asset %s cannot be read.", assetID).
			withPath("asset_id").withHint("transcribe the asset again with job_start kind transcribe")
	}

	src.language = safeLanguage(in.Language)
	for _, w := range in.Words {
		src.spoken = append(src.spoken, transcript.SpokenWord{Text: w.Text, StartMS: w.StartMS, EndMS: w.EndMS})
	}

	frames, fok := transcriptFrames(raw, doc.Settings.FPS)
	if !fok {
		return nil, newError(command.CodeInvalidState, "the transcript of asset %s has no usable words.", assetID).
			withPath("asset_id").withHint("transcribe the asset again with job_start kind transcribe")
	}

	src.frames = frames

	return src, nil
}

func (s *svc) TranscriptView(ctx context.Context, req TranscriptViewRequest) (*TranscriptViewResult, error) {
	src, err := s.loadTranscript(ctx, "TranscriptView", req.ProjectID, req.TimelineID, req.AssetID)
	if err != nil {
		return nil, err
	}

	words := make([]transcript.Word, len(src.frames))
	for i, w := range src.frames {
		words[i] = transcript.Word{Text: w.Text, Start: w.Start, End: w.End}
	}

	view := transcript.Map(src.doc, req.AssetID, words)
	res := &TranscriptViewResult{
		UntrustedNotice: UntrustedNotice, ProjectID: src.row.ID, Revision: src.row.Revision,
		AssetID: req.AssetID, Language: src.language, Words: make([]TranscriptWord, len(view.Words)),
		SkippedItems: view.Skipped,
	}

	cut := 0

	for i := range view.Words {
		w := &view.Words[i]
		res.Words[i] = TranscriptWord{
			Index: w.Index, Text: untrusted.Clean(w.Text), Start: w.Start, End: w.End, Ranges: w.Ranges,
		}

		if len(w.Ranges) == 0 {
			cut++
		}
	}

	var sb strings.Builder

	sb.WriteString(textview.Note + "\n")
	fmt.Fprintf(&sb, "transcript of %s, %s, r%d: %d words, %d with no range on the timeline",
		req.AssetID, orDash(src.language), src.row.Revision, len(res.Words), cut)

	if len(view.Skipped) > 0 {
		fmt.Fprintf(
			&sb,
			"\nnot placed: %s play the asset with a speed curve or backwards",
			strings.Join(view.Skipped, ", "),
		)
	}

	res.Text = sb.String()

	return res, nil
}

func (s *svc) TranscriptDetectFillers(
	ctx context.Context,
	req TranscriptDetectRequest,
) (*TranscriptDetectResult, error) {
	if req.RepeatWindowMS < 0 || req.RepeatWindowMS > 5000 {
		return nil, newError(
			command.CodeValidation,
			"repeat_window_ms %d is not between 0 and 5000.",
			req.RepeatWindowMS,
		).
			withPath("repeat_window_ms").
			withHint("use 0 for the default of 700 ms, or up to 5000")
	}

	src, err := s.loadTranscript(ctx, "TranscriptDetectFillers", req.ProjectID, req.TimelineID, req.AssetID)
	if err != nil {
		return nil, err
	}

	found := transcript.Detect(
		src.spoken,
		transcript.Options{Language: src.language, RepeatWindowMS: req.RepeatWindowMS},
	)
	res := &TranscriptDetectResult{
		UntrustedNotice: UntrustedNotice, ProjectID: src.row.ID, Revision: src.row.Revision,
		AssetID: req.AssetID, Language: src.language, Suggestions: make([]TranscriptSuggestion, len(found)),
		Rule: transcript.RuleDescription(src.language),
	}

	for i, f := range found {
		res.Suggestions[i] = TranscriptSuggestion{
			Kind: string(f.Kind), Strength: string(f.Strength), FromWord: f.FromWord, ToWord: f.ToWord,
			Text: untrusted.Clean(f.Text), StartMS: f.StartMS, EndMS: f.EndMS,
		}
	}

	var sb strings.Builder

	sb.WriteString(textview.Note + "\n")
	fmt.Fprintf(&sb, "%d suggestion(s) in %s r%d; nothing was removed. checked: %s", len(found), req.AssetID,
		src.row.Revision, res.Rule)

	res.Text = sb.String()

	return res, nil
}

// safeLanguage keeps a language code and drops anything else: the field comes from a provider, and it is
// quoted in the text an agent reads.
func safeLanguage(s string) string {
	if s == "" || len(s) > 12 {
		return ""
	}

	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '-' {
			return ""
		}
	}

	return s
}

// inspectWords answers the sections words and fillers of media_inspect (SDD-023): the words and the suggestions go
// under `untrusted`, because their text comes from the media, and the summaries carry the revision, the language
// and the rule.
func (s *svc) inspectWords(
	ctx context.Context,
	req *MediaInspectRequest,
	assetID, section string,
	res *MediaInspectResult,
	text *strings.Builder,
) error {
	if section == "words" {
		view, err := s.TranscriptView(ctx, TranscriptViewRequest{
			ProjectID: req.ProjectID, TimelineID: req.TimelineID, AssetID: assetID,
		})
		if err != nil {
			return err
		}

		res.Untrusted.Words = view.Words
		summary := *view
		summary.Words = []TranscriptWord{}
		res.Words = &summary

		text.WriteString("\n" + strings.TrimPrefix(view.Text, textview.Note+"\n"))
	} else {
		found, err := s.TranscriptDetectFillers(ctx, TranscriptDetectRequest{
			ProjectID: req.ProjectID, TimelineID: req.TimelineID, AssetID: assetID,
		})
		if err != nil {
			return err
		}

		res.Untrusted.Suggestions = found.Suggestions
		summary := *found
		summary.Suggestions = []TranscriptSuggestion{}
		res.Fillers = &summary

		text.WriteString("\n" + strings.TrimPrefix(found.Text, textview.Note+"\n"))
	}

	res.Available = append(res.Available, section)

	return nil
}
