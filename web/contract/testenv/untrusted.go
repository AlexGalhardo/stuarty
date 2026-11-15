package testenv

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"
	"unicode"

	"github.com/AlexGalhardo/vukem-video/service/textview"
)

// HostileName is one file name of web/app/mcp/testdata/untrusted/names.json;
// Windows says whether the name can exist on a Windows file system.
type HostileName struct {
	Name    string `json:"name"`
	Windows bool   `json:"windows"`
}

// HostileNames reads the fixture at path and keeps the names this operating
// system can create and the import guard accepts: a control character in a
// path is refused by the guard (SDD-007 section 7.1, tested in pathguard), so
// such a name never reaches the text of a result.
func HostileNames(tb testing.TB, path string) []HostileName {
	tb.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}

	var all, usable []HostileName
	if err := json.Unmarshal(raw, &all); err != nil {
		tb.Fatal(err)
	}

	for _, n := range all {
		if strings.IndexFunc(n.Name, unicode.IsControl) >= 0 {
			continue
		}

		if n.Windows || runtime.GOOS != "windows" {
			usable = append(usable, n)
		}
	}

	return usable
}

func hostileRune(r rune) bool {
	return unicode.IsControl(r) || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069)
}

// StoredName is a name as the Service stores it: a control or bidirectional
// character becomes an underscore at import.
func StoredName(name string) string {
	return strings.Map(func(r rune) rune {
		if hostileRune(r) {
			return '_'
		}

		return r
	}, name)
}

// UntrustedTextProblems is what SDD-003 section 9 check 11 asks of a text that
// quotes a name: the note leads it, no control or bidirectional character
// survives, and the delimiters of an untrusted block never appear raw.
func UntrustedTextProblems(text string) []string {
	var problems []string

	if !strings.HasPrefix(text, textview.Note) {
		problems = append(problems, "the text does not start with the note line")
	}

	body := strings.TrimPrefix(text, textview.Note)

	for _, r := range body {
		if r != '\n' && hostileRune(r) {
			problems = append(problems, "a control or bidirectional character survived: "+string(r))

			break
		}
	}

	if openingBlocks(body) != strings.Count(body, "</untrusted_media_text>") {
		problems = append(problems, "unbalanced untrusted blocks")
	}

	return problems
}

// openingBlocks counts the opening delimiters, with or without attributes.
func openingBlocks(body string) int {
	return strings.Count(body, "<untrusted_media_text>") + strings.Count(body, "<untrusted_media_text ")
}
