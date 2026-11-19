package connections

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"html"
	"net/url"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// minScrubbed is the shortest secret a body is checked for, and the length of the prefix and of the
// suffix of a longer one that is enough to drop the body: a destination that echoes eight bytes of a
// credential has echoed it, even when the rest was cut or escaped on the way (SDD-014 section 9). It
// is the same floor as the vault redaction (a shorter secret would drop any text it is in).
const minScrubbed = 8

// droppedBody replaces the body of an answer that held the credential.
const droppedBody = "the destination echoed the credential: its answer was dropped."

// echoesSecret says whether body holds the secret, or minScrubbed bytes of its start or its end, in
// the forms a destination writes it back: as is, HTML, URL and JSON escaped, base64 and hex (lower
// and upper case). It
// looks at the raw bytes, before any cut or escape, and also at them after the cleaning the text
// goes through, so a secret split by a control character is found too.
func echoesSecret(body, secret []byte) bool {
	if len(secret) < minScrubbed {
		return false
	}

	text := string(secret)
	forms := []string{
		text,
		html.EscapeString(text),
		url.QueryEscape(text),
		url.PathEscape(text),
		base64.StdEncoding.EncodeToString(secret),
		base64.RawStdEncoding.EncodeToString(secret),
		base64.URLEncoding.EncodeToString(secret),
		base64.RawURLEncoding.EncodeToString(secret),
		hex.EncodeToString(secret),
		strings.ToUpper(hex.EncodeToString(secret)),
	}
	forms = append(forms, jsonForms(text)...)

	haystacks := [][]byte{body, []byte(untrusted.Clean(string(body)))}

	for _, f := range forms {
		for _, needle := range []string{f[:minScrubbed], f[len(f)-minScrubbed:]} {
			for _, h := range haystacks {
				if bytes.Contains(h, []byte(needle)) {
					return true
				}
			}
		}
	}

	return false
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
