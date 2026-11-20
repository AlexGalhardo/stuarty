package writing

import "slices"

// Unit is what the length limit of a network counts (SDD-019 section 5.1).
type Unit string

// The units of a length.
const (
	UnitBytes      Unit = "bytes"       // bytes of UTF-8
	UnitCharacters Unit = "characters"  // Unicode code points
	UnitUTF16      Unit = "utf16_units" // UTF-16 code units
	UnitWeighted   Unit = "weighted"    // the weighted count of X: a link weighs 23, an emoji or a CJK character 2
)

// Network is the row of the table of limits of one destination of a caption. A limit of 0 is one the network does
// not publish. Checked and Source say when and where each figure was read, so that a change of a network is a
// change of one row; the connectors of epics 18, 19 and 22 validate against this table (SDD-019 section 5.1).
type Network struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Unit is what MaxLength counts.
	Unit        Unit `json:"unit"`
	MaxLength   int  `json:"max_length"`
	MaxHashtags int  `json:"max_hashtags"`
	MaxMentions int  `json:"max_mentions"`
	// LinksClickable is whether a link in the text is a link for the reader.
	LinksClickable bool `json:"links_clickable"`
	// LinkWeight is what a link counts in a weighted length; 0 for the units that count the characters themselves.
	LinkWeight int `json:"link_weight,omitempty"`
	// Forbidden lists the characters the network refuses in the text.
	Forbidden string `json:"forbidden,omitempty"`
	// Checked is the date the figures were read, YYYY-MM-DD.
	Checked string `json:"checked"`
	// Source lists the pages the figures were read from.
	Source []string `json:"source"`
	// Note says what is not a published figure.
	Note string `json:"note,omitempty"`
}

const checkedOn = "2026-11-19"

// networks is the table of limits (SDD-019 section 5.1). Change a row here and nowhere else.
var networks = []Network{
	{
		ID: "youtube", Name: "YouTube (the description of a video)", Unit: UnitBytes, MaxLength: 5000,
		MaxHashtags: 60, LinksClickable: true, Forbidden: "<>", Checked: checkedOn,
		Source: []string{
			"https://developers.google.com/youtube/v3/docs/videos",
			"https://support.google.com/youtube/answer/6390658",
		},
		Note: "no limit is published for mentions",
	},
	{
		ID: "instagram", Name: "Instagram (the caption of a post)", Unit: UnitCharacters, MaxLength: 2200,
		MaxHashtags: 30, MaxMentions: 20, LinksClickable: false, Checked: checkedOn,
		Source: []string{
			"https://developers.facebook.com/documentation/instagram-platform/instagram-graph-api/reference/ig-user/media",
		},
	},
	{
		ID:             "facebook",
		Name:           "Facebook (the message of a Page post)",
		Unit:           UnitCharacters,
		MaxLength:      63206,
		LinksClickable: true,
		Checked:        checkedOn,
		Source:         []string{"https://developers.facebook.com/docs/graph-api/reference/page/feed/"},
		Note:           "Meta documents no limit for the message; 63,206 is the long-published figure, kept until Meta publishes one",
	},
	{
		ID: "x", Name: "X (a post)", Unit: UnitWeighted, MaxLength: 280, LinksClickable: true, LinkWeight: 23,
		Checked: checkedOn, Source: []string{"https://docs.x.com/fundamentals/counting-characters"},
		Note: "text is counted after Unicode NFC; no limit is published for hashtags or mentions",
	},
	{
		ID: "tiktok", Name: "TikTok (the title of a direct post)", Unit: UnitUTF16, MaxLength: 2200,
		LinksClickable: false, Checked: checkedOn,
		Source: []string{"https://developers.tiktok.com/doc/content-posting-api-reference-direct-post"},
		Note:   "no limit is published for hashtags or mentions",
	},
}

// Networks lists the table of limits, in the order of the delivery of the connectors (ADR-0035). The rows are
// copies: changing one changes nothing.
func Networks() []Network {
	out := make([]Network, len(networks))
	for i := range networks {
		out[i] = networks[i]
		out[i].Source = slices.Clone(networks[i].Source)
	}

	return out
}

// NetworkIDs lists the identifiers of the networks of the table.
func NetworkIDs() []string {
	ids := make([]string, len(networks))
	for i := range networks {
		ids[i] = networks[i].ID
	}

	return ids
}

// Lookup finds a network of the table by its identifier, in lower case.
func Lookup(id string) (Network, bool) {
	for i := range networks {
		if networks[i].ID == id {
			n := networks[i]
			n.Source = slices.Clone(n.Source)

			return n, true
		}
	}

	return Network{}, false
}
