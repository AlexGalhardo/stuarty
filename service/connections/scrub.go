package connections

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"html"
	"net/url"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// minScrubbed is the shortest secret a body is checked for, and the length of the window that drops
// the body: a destination that echoes any eight consecutive bytes of a credential, in any form it
// writes it back, has echoed it, even when the rest was cut or escaped on the way (SDD-014 section 9).
// It is the same floor as the vault redaction (a shorter secret would drop any text it is in).
const minScrubbed = 8

// droppedBody replaces the body of an answer that held the credential.
const droppedBody = "the destination echoed the credential: its answer was dropped."

// echoesSecret says whether body holds the secret, or any minScrubbed consecutive bytes of it, in
// the forms a destination writes it back: as is, HTML, URL and JSON escaped, base64 and hex (lower
// and upper case). It looks at the raw bytes, before any cut or escape, and also at them after the
// cleaning the text goes through, so a secret split by a control character is found too. The windows
// of the two bodies go into a set once, and every window of every form is looked up in it, so the cost
// is linear in the body plus the forms, whatever the size of the secret (up to MaxSecret).
func echoesSecret(body, secret []byte) bool {
	if len(secret) < minScrubbed {
		return false
	}

	seen := make(map[[minScrubbed]byte]struct{}, 2*len(body))

	for _, h := range [][]byte{body, []byte(untrusted.Clean(string(body)))} {
		for i := 0; i+minScrubbed <= len(h); i++ {
			seen[[minScrubbed]byte(h[i:i+minScrubbed])] = struct{}{}
		}
	}

	for _, f := range secretForms(secret) {
		for i := 0; i+minScrubbed <= len(f); i++ {
			if _, ok := seen[[minScrubbed]byte(f[i:i+minScrubbed])]; ok {
				return true
			}
		}
	}

	return false
}

// secretForms are the forms a destination writes the secret back in. Base64 is taken from the three
// byte alignments of the secret, so the base64 of a part of it that starts anywhere shares a window
// with one of them; the unpadded base64 is a prefix of the padded one and needs no form of its own.
// Hex, HTML, URL and JSON escape byte by byte, so a part of the secret is a part of their form.
func secretForms(secret []byte) [][]byte {
	text := string(secret)
	forms := []string{
		text,
		html.EscapeString(text),
		url.QueryEscape(text),
		url.PathEscape(text),
		hex.EncodeToString(secret),
		strings.ToUpper(hex.EncodeToString(secret)),
	}

	for k := range min(3, len(secret)) {
		part := secret[k:]
		forms = append(forms, base64.StdEncoding.EncodeToString(part), base64.URLEncoding.EncodeToString(part))
	}

	forms = append(forms, jsonForms(text)...)

	out := make([][]byte, len(forms))
	for i, f := range forms {
		out[i] = []byte(f)
	}

	return out
}

// jsonForms are text inside a JSON string, without the quotes, as the encoders write it: with the
// less-than, greater-than and ampersand signs as six-character Unicode escapes (Go) or as is, and
// with the slash as is or escaped with a backslash (PHP).
func jsonForms(text string) []string {
	var forms []string

	for _, escapeHTML := range []bool{true, false} {
		var b strings.Builder

		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(escapeHTML)

		if err := enc.Encode(text); err != nil {
			continue
		}

		s := strings.TrimSuffix(b.String(), "\n")
		s = s[1 : len(s)-1]
		forms = append(forms, s, strings.ReplaceAll(s, "/", `\/`))
	}

	return forms
}

// SafeMessage is the text of a test answer for a connector to return: the body as an untrusted block,
// or, when the raw body echoes the secret, only a fixed sentence. Connectors build their message with
// it so the redaction sees the body before the cut and the escape.
func SafeMessage(c Credential, body []byte) string {
	if echoesSecret(body, c.Secret) {
		body = []byte(droppedBody)
	}

	attrs := untrusted.Attrs{"connection": c.Connection.ID}

	return untrusted.Block(untrusted.SourceNetworkText, attrs, string(body), testMessageBudget)
}

// Echoes reports whether text holds the credential of c, or any eight consecutive bytes of it, in the forms
// a destination writes it back (as is, HTML, URL and JSON escaped, base64 and hex). A connector that returns
// text a destination wrote, other than an error body that SafeMessage already checks, asks it before it
// does, so a destination that echoes the credential does not hand it to a caller (SDD-014 section 9).
func (c Credential) Echoes(text string) bool { return echoesSecret([]byte(text), c.Secret) }

// EchoedAnswer replaces the text of an answer that echoed the credential.
const EchoedAnswer = droppedBody
