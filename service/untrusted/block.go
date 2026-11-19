package untrusted

import (
	"fmt"
	"sort"
	"strings"
)

// The sources of a block (SDD-007 section 8.2).
const (
	SourceTranscript   = "transcript"
	SourceAssetName    = "asset_name"
	SourceMetadata     = "metadata"
	SourceProjectText  = "project_text"
	SourceTemplateText = "template_text"
	SourceLibraryText  = "library_text"
)

// Sources lists every source kind.
var Sources = []string{
	SourceTranscript, SourceAssetName, SourceMetadata, SourceProjectText, SourceTemplateText, SourceLibraryText,
}

// Attrs are the orientation attributes of a block: asset, project or item.
type Attrs map[string]string

const (
	openTag  = "<untrusted_media_text"
	closeTag = "</untrusted_media_text>"
)

// Block writes text as a block of the given source. The text is cleaned, cut
// to budget characters (0 means no cut) and escaped; when it was cut the block
// ends with `[truncated: N more characters]` and says truncated="true".
func Block(source string, attrs Attrs, text string, budget int) string {
	clean := Clean(text)
	more := 0

	if budget > 0 {
		clean, more = Cut(clean, budget)
	}

	var b strings.Builder

	b.WriteString(openTag + ` source="` + source + `"`)
	writeAttrs(&b, attrs)
	fmt.Fprintf(&b, ` truncated="%t">`, more > 0)
	b.WriteByte('\n')
	b.WriteString(Escape(clean))
	b.WriteByte('\n')

	if more > 0 {
		fmt.Fprintf(&b, "[truncated: %d more characters]\n", more)
	}

	b.WriteString(closeTag)

	return b.String()
}

// Inline is the single-line form of a block, for a short value inside a line of
// generated text: inline="true", and the value on one line.
func Inline(source string, attrs Attrs, text string, budget int) string {
	clean := strings.Join(strings.Fields(Clean(text)), " ")
	more := 0

	if budget > 0 {
		clean, more = Cut(clean, budget)
	}

	var b strings.Builder

	b.WriteString(openTag + ` source="` + source + `"`)
	writeAttrs(&b, attrs)
	fmt.Fprintf(&b, ` inline="true" truncated="%t">`, more > 0)
	b.WriteString(Escape(clean))
	b.WriteString(closeTag)

	return b.String()
}

func writeAttrs(b *strings.Builder, attrs Attrs) {
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		fmt.Fprintf(b, ` %s="%s"`, k, strings.ReplaceAll(Escape(Clean(attrs[k])), `"`, "&quot;"))
	}
}
