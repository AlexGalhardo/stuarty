package untrusted

// The budgets of SDD-007 section 8.3, in characters (a token is 3.2 characters).
const (
	TranscriptConcise = 8000
	TranscriptFull    = 32000

	AssetNameConcise = 120
	AssetNameFull    = 200

	MetadataTagsConcise  = 10
	MetadataKeyConcise   = 40
	MetadataValueConcise = 200
	MetadataTotalConcise = 1500
	MetadataTagsFull     = 20
	MetadataValueFull    = 400
	MetadataTotalFull    = 4000

	TimelineText = 200 // the text of a text item and caption words in the timeline view
	Description  = 200 // template and slot descriptions
	CompactMax   = 80  // a value up to this long is written in quotes, longer in a block
)

// Transcript is the character budget of one page of a transcript.
func Transcript(full bool) int {
	if full {
		return TranscriptFull
	}

	return TranscriptConcise
}

// AssetName is the character budget of an asset name.
func AssetName(full bool) int {
	if full {
		return AssetNameFull
	}

	return AssetNameConcise
}

// Tag is one metadata tag.
type Tag struct{ Key, Value string }

// Metadata applies the metadata budget: at most the allowed number of tags,
// each key and value cleaned, cut and escaped, and a total of characters that
// stops the list. dropped counts the tags left out.
func Metadata(tags []Tag, full bool) (kept []Tag, dropped int) {
	maxTags, maxKey, maxVal, maxTotal := MetadataTagsConcise, MetadataKeyConcise, MetadataValueConcise, MetadataTotalConcise
	if full {
		maxTags, maxVal, maxTotal = MetadataTagsFull, MetadataValueFull, MetadataTotalFull
	}

	total := 0

	for _, t := range tags {
		k, _ := Cut(Clean(t.Key), maxKey)
		v, _ := Cut(Clean(t.Value), maxVal)

		size := len([]rune(k)) + len([]rune(v))
		if len(kept) == maxTags || total+size > maxTotal {
			dropped++

			continue
		}

		total += size

		kept = append(kept, Tag{Key: Escape(k), Value: Escape(v)})
	}

	return kept, dropped
}

// Text is a value of the timeline view cut to its budget: clean, cut, escaped.
func Text(s string, budget int) string {
	kept, _ := Cut(Clean(s), budget)

	return Escape(kept)
}
