package publish

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/writing"
)

// Price is what a network charges for one post, as data with where and when it was read (SDD-020 section 35,
// RF-408). The amounts are in millionths of the currency, as the cost of the approval queue is.
type Price struct {
	Currency string `json:"currency"`
	// PostMicros is the price of a post; LinkMicros the price of a post whose text holds a link.
	PostMicros int64 `json:"post_micros"`
	LinkMicros int64 `json:"link_micros"`
	// Source is the official page and Read the date it was read.
	Source string `json:"source"`
	Read   string `json:"read"`
	// Edited is the date a person last changed the amounts, empty for the ones that were read.
	Edited string `json:"edited,omitempty"`
}

// MaxPriceMicros is the highest price a person may enter, 100 USD: a typing mistake must not make a post
// look free or absurdly dear.
const MaxPriceMicros = 100_000_000

// ErrPrice is a price that is not allowed.
var ErrPrice = errors.New("publish: invalid price")

// XPrice is the price of a post on X, read on 2026-11-20: $0.015, and $0.200 for a post with a URL.
func XPrice() Price {
	return Price{
		Currency: "USD", PostMicros: 15_000, LinkMicros: 200_000,
		Source: "https://docs.x.com/x-api/getting-started/pricing", Read: networksRead,
	}
}

// Valid says whether the amounts may be saved: whole, positive and at most MaxPriceMicros.
func (p Price) Valid() error {
	for _, v := range []int64{p.PostMicros, p.LinkMicros} {
		if v <= 0 || v > MaxPriceMicros {
			return ErrPrice
		}
	}

	return nil
}

// For is the price of a post with this text and whether it is the price of a post with a link. The page does
// not say how X decides that a post has a URL, so a bare domain counts as one too: the rule that cannot
// understate the price.
func (p Price) For(text string) (micros int64, link bool) {
	if HasLink(text) {
		return p.LinkMicros, true
	}

	return p.PostMicros, false
}

// bareDomain finds a word that is a domain name: labels, a dot and a top-level domain of two letters or more.
var bareDomain = regexp.MustCompile(
	`(?i)(^|[^\p{L}\p{N}@._-])[\p{L}\p{N}][\p{L}\p{N}-]*(\.[\p{L}\p{N}-]+)*\.\p{L}{2,}($|[^\p{L}\p{N}-])`,
)

// HasLink says whether a text holds a link: an http or https address, or a bare domain such as example.com.
func HasLink(text string) bool {
	if writing.ValidateCaption(text, "").Counts.Links > 0 {
		return true
	}

	return bareDomain.MatchString(strings.ToLower(text))
}

// FormatMicros writes an amount in millionths of a currency as a decimal with at least two digits after the
// point, and as many more as it needs: 15000 is "0.015" and 200000 is "0.20".
func FormatMicros(micros int64) string {
	whole, frac := micros/1_000_000, micros%1_000_000
	digits := strings.TrimRight(fmt.Sprintf("%06d", frac), "0")

	for len(digits) < 2 {
		digits += "0"
	}

	return fmt.Sprintf("%d.%s", whole, digits)
}
