package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/vukem-video/engine/ingest"
	"github.com/AlexGalhardo/vukem-video/service/untrusted"
)

// TranscriptLine is one line of the transcript of an asset: a segment of words
// with the time it starts at. Its text is from the media, so it is untrusted.
type TranscriptLine struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"     untrusted:"true"`
}

const noTranscriptYet = "\ntranscript: none yet (start a transcribe job for this asset)"

// noTranscript is the section of an asset without a transcript: the phrase to start
// a job, or, when no provider can run, E_TRANSCRIBE_UNAVAILABLE with its hint
// (SDD-003 section 3.8).
func (s *svc) noTranscript(ctx context.Context) string {
	var err error
	if s.transcriber != nil {
		if err = s.transcriber.Check(ctx); err == nil {
			return noTranscriptYet
		}
	}

	model := ""
	if s.transcriber != nil {
		model = s.transcriber.Model()
	}

	return "\ntranscript: unavailable (" + codeTranscribeOff + "): " + unavailable(err, model).Hint
}

// transcriptSection is the transcript of media_inspect (SDD-003 section 2.6,
// SDD-007 section 8): the lines of the newest transcript artifact, one page of
// at most the concise budget of characters, inside an untrusted block, with a
// cursor for the next page (RF-116). The cursor is an offset into the lines of
// this artifact; it goes stale only when the transcript is replaced
// (details.reason "stale").
func (s *svc) transcriptSection(
	ctx context.Context,
	arts []ArtifactRow,
	assetID, cursor string,
	res *MediaInspectResult,
) (string, *Error) {
	raw := s.newestTranscript(arts)
	if raw == nil {
		return s.noTranscript(ctx), nil
	}

	var doc struct {
		Language string `json:"language"`
		Words    []struct {
			Text string `json:"text"`
		} `json:"words"`
		Segments []struct {
			StartMS   int64 `json:"start_ms"`
			EndMS     int64 `json:"end_ms"`
			FirstWord int   `json:"first_word"`
			LastWord  int   `json:"last_word"`
		} `json:"segments"`
	}

	if !readable(raw, &doc) || len(doc.Segments) == 0 {
		return s.noTranscript(ctx), nil
	}

	// The cursor names the transcript it was issued for: a replaced artifact is
	// another list.
	kind := "transcript-" + strconv.Itoa(len(raw))

	start := 0

	if cursor != "" {
		off, cerr := decodeOffset(cursor, kind, assetID)
		if cerr != nil {
			return "", cerr.withDetail("reason", "stale").withHint("repeat the call without a cursor")
		}

		start = off
	}

	budget := untrusted.Transcript(false)
	used := 0

	var (
		lines []string
		i     = start
	)

	for ; i < len(doc.Segments); i++ {
		seg := doc.Segments[i]
		if seg.FirstWord < 0 || seg.LastWord >= len(doc.Words) || seg.FirstWord > seg.LastWord {
			continue
		}

		words := make([]string, 0, seg.LastWord-seg.FirstWord+1)
		for _, w := range doc.Words[seg.FirstWord : seg.LastWord+1] {
			words = append(words, w.Text)
		}

		plain, _ := untrusted.Cut(untrusted.Clean(strings.Join(words, " ")), untrusted.Transcript(false))
		line := fmt.Sprintf("[%s] %s", clockMS(seg.StartMS), plain)

		if used+len(line)+1 > budget && len(lines) > 0 {
			break
		}

		used += len(line) + 1

		lines = append(lines, line)

		td := TranscriptLine{
			StartMS: seg.StartMS,
			EndMS:   seg.EndMS,
			Text:    untrusted.Value(plain, untrusted.Transcript(false)),
		}
		res.Untrusted.Transcript = append(res.Untrusted.Transcript, td)
	}

	res.Available = append(res.Available, "transcript")

	if i < len(doc.Segments) {
		next := offsetCursor(kind, assetID, i)
		res.NextCursor = &next
	}

	block := untrusted.Block(
		untrusted.SourceTranscript,
		untrusted.Attrs{"asset": assetID},
		strings.Join(lines, "\n"),
		budget,
	)

	return fmt.Sprintf(
		"\ntranscript (%s, lines %d to %d of %d):\n%s",
		doc.Language,
		start+1,
		i,
		len(doc.Segments),
		block,
	), nil
}

// clockMS is hh:mm:ss.mmm.
func clockMS(ms int64) string {
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

// readable decodes a stored artifact; one that cannot be read is no artifact.
func readable(raw []byte, into any) bool { return json.Unmarshal(raw, into) == nil }

// probeSection is the probe line of media_inspect followed by the metadata of
// the file; empty when the asset has no readable probe.
func (s *svc) probeSection(
	ctx context.Context,
	project *ProjectRow,
	ar *AssetRow,
	assetID string,
	res *MediaInspectResult,
) string {
	p := s.probeOf(ctx, project, ar)
	if p == nil {
		return ""
	}

	res.Probe = p
	res.Available = append(res.Available, "probe")
	text := "\nprobe: " + probeText(projectFPS(project), p)

	if sum := s.readSummary(ctx, ar.Hash); sum != nil {
		text += metadataSection(sum.Tags, assetID, res)
	}

	return text
}

var metadataUnescape = strings.NewReplacer("&lt;", "<", "&amp;", "&")

// metadataSection is the metadata of media_inspect (SDD-003 section 2.6, SDD-007
// section 8.3): the tags the probe kept, keys sorted, one `key: value` per line
// inside an untrusted block, at most the concise number of tags and characters.
// It fills res.Untrusted.Metadata and returns the text; empty when there are no tags.
func metadataSection(tags map[string]ingest.Tag, assetID string, res *MediaInspectResult) string {
	if len(tags) == 0 {
		return ""
	}

	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	all := make([]untrusted.Tag, 0, len(keys))

	for _, k := range keys {
		all = append(all, untrusted.Tag{
			Key:   oneLine(k, untrusted.MetadataKeyConcise, false),
			Value: oneLine(tags[k].Value, untrusted.MetadataValueConcise, tags[k].Truncated),
		})
	}

	// Metadata escapes what it keeps; the block and the structured value escape
	// on their own, so the raw text is recovered from the kept tags.
	kept, dropped := untrusted.Metadata(all, false)
	if len(kept) == 0 {
		return ""
	}

	lines := make([]string, 0, len(kept))
	structured := make(map[string]string, len(kept))

	for _, t := range kept {
		key, value := metadataUnescape.Replace(t.Key), metadataUnescape.Replace(t.Value)

		lines = append(lines, key+": "+value)
		structured[untrusted.Value(key, untrusted.MetadataKeyConcise)] = untrusted.Value(
			value,
			untrusted.MetadataValueConcise,
		)
	}

	res.Untrusted.Metadata = structured

	head := fmt.Sprintf("\nmetadata %d keys", len(kept))
	if dropped > 0 {
		head += fmt.Sprintf(" (%d more not shown)", dropped)
	}

	block := untrusted.Block(untrusted.SourceMetadata, untrusted.Attrs{"asset": assetID}, strings.Join(lines, "\n"), 0)

	return head + "\n" + block
}

// oneLine cleans s onto a single line and cuts it to n characters, the last of
// them an ellipsis when it was cut (or already cut by the probe).
func oneLine(s string, n int, wasCut bool) string {
	s = strings.Join(strings.Fields(untrusted.Clean(s)), " ")

	kept, more := untrusted.Cut(s, n)
	if more == 0 && !wasCut {
		return kept
	}

	kept, _ = untrusted.Cut(kept, n-1)

	return kept + "…"
}
