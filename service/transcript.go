package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
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

	// Only the words the timeline still shows can be cut: a suggestion for a word already cut would make the
	// whole batch of the review fail.
	words := make([]transcript.Word, len(src.frames))
	for i, w := range src.frames {
		words[i] = transcript.Word{Text: w.Text, Start: w.Start, End: w.End}
	}

	placed := transcript.Map(src.doc, req.AssetID, words)
	shown := make([]bool, len(placed.Words))

	for i := range placed.Words {
		shown[i] = len(placed.Words[i].Ranges) > 0
	}

	found := transcript.DetectShown(
		src.spoken,
		shown,
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

// The caps of the text of a word and of a suggestion on the agent surfaces (SDD-023 section 5). A word of a caption is
// capped at the same 80 characters by its schema; a suggestion holds at most a few words.
const (
	agentWordChars       = 80
	agentSuggestionChars = 200
)

func wordLine(w *TranscriptWord) string {
	mark := ""
	if len(w.Ranges) == 0 {
		mark = "~"
	}

	return fmt.Sprintf("%d%s:%s", w.Index, mark, oneLine(w.Text, agentWordChars, false))
}

func suggestionLine(f *TranscriptSuggestion) string {
	return fmt.Sprintf("%d-%d %s %s: %s", f.FromWord, f.ToWord, f.Kind, f.Strength,
		oneLine(f.Text, agentSuggestionChars, false))
}

// fitLines takes lines from start while their escaped length fits the budget (0 is no budget), always at least one,
// and returns them with the index after the last.
func fitLines(n, start, budget int, line func(int) string) (lines []string, next int) {
	used := 0

	for next = start; next < n; next++ {
		l := line(next)
		cost := len(untrusted.Escape(l)) + 1

		if budget > 0 && used+cost > budget && len(lines) > 0 {
			break
		}

		used += cost

		lines = append(lines, l)
	}

	return lines, next
}

func agentWords(words []TranscriptWord) []TranscriptWord {
	out := make([]TranscriptWord, len(words))
	for i := range words {
		out[i] = words[i]
		out[i].Text = untrusted.Value(strings.Join(strings.Fields(words[i].Text), " "), agentWordChars)
	}

	return out
}

func agentSuggestions(found []TranscriptSuggestion) []TranscriptSuggestion {
	out := make([]TranscriptSuggestion, len(found))
	for i := range found {
		out[i] = found[i]
		out[i].Text = untrusted.Value(strings.Join(strings.Fields(found[i].Text), " "), agentSuggestionChars)
	}

	return out
}

// ForAgent is the view for a surface an agent reads (the CLI): the text of each word is escaped and capped, and the
// text of the result lists the words inside an untrusted block, each with the index transcript.delete_range names.
func (r *TranscriptViewResult) ForAgent() *TranscriptViewResult {
	out := *r
	out.Words = agentWords(r.Words)

	lines, _ := fitLines(len(r.Words), 0, 0, func(i int) string { return wordLine(&r.Words[i]) })
	if len(lines) > 0 {
		out.Text = r.Text + "\nwords (index:text, ~ is not on the timeline):\n" + untrusted.Block(
			untrusted.SourceTranscript, untrusted.Attrs{"asset": r.AssetID}, strings.Join(lines, "\n"), 0,
		)
	}

	return &out
}

// ForAgent is the list of suggestions for a surface an agent reads, as TranscriptViewResult.ForAgent.
func (r *TranscriptDetectResult) ForAgent() *TranscriptDetectResult {
	out := *r
	out.Suggestions = agentSuggestions(r.Suggestions)

	lines, _ := fitLines(len(r.Suggestions), 0, 0, func(i int) string { return suggestionLine(&r.Suggestions[i]) })
	if len(lines) > 0 {
		out.Text = r.Text + "\nsuggestions (from-to kind strength: text):\n" + untrusted.Block(
			untrusted.SourceTranscript, untrusted.Attrs{"asset": r.AssetID}, strings.Join(lines, "\n"), 0,
		)
	}

	return &out
}

// inspectPaging says which section of a media_inspect call the cursor pages (the transcript, else the words, else the
// fillers) and the characters each of the sections words and fillers may use, so that the whole text stays within its
// cap of 12,800 (SDD-003 section 3.10).
func inspectPaging(sections []string) (owner string, budget int) {
	switch {
	case slices.Contains(sections, "transcript"):
		return "transcript", 1500
	case slices.Contains(sections, "words") && slices.Contains(sections, "fillers"):
		return "words", untrusted.TranscriptConcise / 2
	case slices.Contains(sections, "words"):
		return "words", untrusted.TranscriptConcise
	}

	return "fillers", untrusted.TranscriptConcise
}

// inspectPage writes one page of a list of the sections words and fillers, in an untrusted block, and returns the
// bounds of the page. Only the section that owns the cursor reads it and sets next_cursor.
func inspectPage(
	req *MediaInspectRequest,
	assetID, section, legend string,
	n int,
	owned bool,
	budget int,
	line func(int) string,
	res *MediaInspectResult,
	text *strings.Builder,
) (start, next int, err error) {
	kind := section + "-" + strconv.Itoa(n)

	if owned && req.Cursor != "" {
		off, cerr := decodeOffset(req.Cursor, kind, assetID)
		if cerr != nil {
			return 0, 0, cerr.withDetail("reason", "stale").withHint("repeat the call without a cursor")
		}

		if off > n {
			return 0, 0, newError(command.CodeValidation, "the cursor is past the end of the list.").
				withPath("cursor").withDetail("reason", "stale").withHint("repeat the call without a cursor")
		}

		start = off
	}

	lines, next := fitLines(n, start, budget, line)

	fmt.Fprintf(text, "\n%s %d to %d of %d (%s)", section, min(start+1, next), next, n, legend)

	if next < n {
		if owned {
			cur := offsetCursor(kind, assetID, next)
			res.NextCursor = &cur
		} else {
			text.WriteString("; ask for this section alone to page the rest")
		}
	}

	if len(lines) > 0 {
		text.WriteString(":\n" + untrusted.Block(
			untrusted.SourceTranscript, untrusted.Attrs{"asset": assetID}, strings.Join(lines, "\n"), 0,
		))
	}

	return start, next, nil
}

// inspectWords answers the sections words and fillers of media_inspect (SDD-023): one page of the words or the
// suggestions, escaped and capped, under `untrusted` and in an untrusted block of the text, because their text comes
// from the media. The summaries carry the revision, the language and the rule.
func (s *svc) inspectWords(
	ctx context.Context,
	req *MediaInspectRequest,
	assetID, section string,
	res *MediaInspectResult,
	text *strings.Builder,
) error {
	owner, budget := inspectPaging(req.Sections)

	if section == "words" {
		view, err := s.TranscriptView(ctx, TranscriptViewRequest{
			ProjectID: req.ProjectID, TimelineID: req.TimelineID, AssetID: assetID,
		})
		if err != nil {
			return err
		}

		text.WriteString("\n" + strings.TrimPrefix(view.Text, textview.Note+"\n"))

		start, next, err := inspectPage(req, assetID, section, "index:text, ~ is not on the timeline", len(view.Words),
			owner == section, budget, func(i int) string { return wordLine(&view.Words[i]) }, res, text)
		if err != nil {
			return err
		}

		res.Untrusted.Words = agentWords(view.Words[start:next])
		summary := *view
		summary.Words = []TranscriptWord{}
		res.Words = &summary
	} else {
		found, err := s.TranscriptDetectFillers(ctx, TranscriptDetectRequest{
			ProjectID: req.ProjectID, TimelineID: req.TimelineID, AssetID: assetID,
		})
		if err != nil {
			return err
		}

		text.WriteString("\n" + strings.TrimPrefix(found.Text, textview.Note+"\n"))

		start, next, err := inspectPage(req, assetID, section, "from-to kind strength: text", len(found.Suggestions),
			owner == section, budget, func(i int) string { return suggestionLine(&found.Suggestions[i]) }, res, text)
		if err != nil {
			return err
		}

		res.Untrusted.Suggestions = agentSuggestions(found.Suggestions[start:next])
		summary := *found
		summary.Suggestions = []TranscriptSuggestion{}
		res.Fillers = &summary
	}

	res.Available = append(res.Available, section)

	return nil
}
