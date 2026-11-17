package timeline

import (
	"crypto/rand"
	"strconv"
	"strings"
)

// idAlphabet is lower-case Crockford base32: 0-9 and a-z without i, l, o, u.
const idAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

const idLen = 10

// NewID returns "<prefix>_<10 characters>" from the cryptographic random source. It
// is for the callers of the core (the service, the CLI, tests): a command never
// generates an identifier, it takes them from its arguments or from Env.NewIDs
// (SDD-002 §A.2).
func NewID(prefix string) string {
	var b [idLen]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("timeline: random source failed: " + err.Error())
	}
	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteByte('_')
	for _, c := range b {
		sb.WriteByte(idAlphabet[c&31]) // 256 is a multiple of 32: no modulo bias
	}
	return sb.String()
}

// IDPattern is the regular expression of the ids ValidID accepts for prefix, for the JSON Schema
// "pattern" keyword: the schema and the validator cannot drift.
func IDPattern(prefix string) string {
	return "^" + prefix + "_[" + idAlphabet + "]{" + strconv.Itoa(idLen) + "}$"
}

// ValidID reports whether id is "<prefix>_<10 Crockford base32 characters>".
func ValidID(prefix, id string) bool {
	rest, ok := strings.CutPrefix(id, prefix+"_")
	if !ok || len(rest) != idLen {
		return false
	}
	for i := range len(rest) {
		if strings.IndexByte(idAlphabet, rest[i]) < 0 {
			return false
		}
	}
	return true
}

const orderDigits = "0123456789abcdefghijklmnopqrstuvwxyz"

func orderDigit(s string, i int, missing int) int {
	if i >= len(s) {
		return missing
	}
	return strings.IndexByte(orderDigits, s[i])
}

// ValidOrder reports whether s is a non-empty string of 0-9a-z.
func ValidOrder(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if strings.IndexByte(orderDigits, s[i]) < 0 {
			return false
		}
	}
	return true
}

// OrderBetween returns a track order that sorts strictly between a and b as byte
// strings (SDD-001 §3.3.1). "" means "before the first" for a and "after the last"
// for b. The result never ends in "0". It returns "" when a and b are not valid or
// when a does not sort before b.
func OrderBetween(a, b string) string {
	if a != "" && !ValidOrder(a) || b != "" && !ValidOrder(b) || b != "" && a >= b {
		return ""
	}
	var out []byte
	i := 0
	for ; ; i++ {
		da := orderDigit(a, i, 0)
		db := orderDigit(b, i, 0)
		if b == "" && i == 0 {
			db = 36
		}
		if i >= len(b) && b != "" {
			return "" // a is not below b numerically (for example "a" and "a0")
		}
		if da == db {
			out = append(out, orderDigits[da])
			continue
		}
		if db-da > 1 {
			return string(append(out, orderDigits[(da+db)/2]))
		}
		out = append(out, orderDigits[da])
		break
	}
	for i++; ; i++ {
		d := orderDigit(a, i, 0)
		if d < 35 {
			return string(append(out, orderDigits[(d+36)/2]))
		}
		out = append(out, 'z')
	}
}
