package command

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

// The table of SDD-002 §A.8.
var sddCodes = []string{
	"E_FONT_UNKNOWN", "E_ID_EXISTS", "E_IDEMPOTENCY_MISMATCH", "E_INTERNAL", "E_INVALID_STATE", "E_INVALID_TIME",
	"E_IN_USE", "E_LIMIT", "E_NOT_EMPTY", "E_NOT_FOUND", "E_OUT_OF_SOURCE", "E_OVERLAP", "E_REVISION_CONFLICT",
	"E_SLOT", "E_TYPE_MISMATCH", "E_UNKNOWN_COMMAND", "E_VALIDATION",
}

func TestCodesMatchTheSDDTable(t *testing.T) {
	got, want := Codes(), slices.Sorted(slices.Values(sddCodes))
	if !slices.Equal(got, want) {
		t.Errorf("codes %v, want %v", got, want)
	}
	for _, c := range got {
		if !KnownCode(c) {
			t.Errorf("%s not known", c)
		}
		if retry := Errorf(c, "m").Retryable; retry != (c == CodeRevisionConflict) {
			t.Errorf("%s retryable = %v", c, retry)
		}
	}
	if KnownCode("E_NOPE") {
		t.Error("E_NOPE is known")
	}
}

func TestErrorShapeOfSDD(t *testing.T) {
	idx := 1
	e := Errorf(CodeOverlap, "Item %s would occupy frames 100-149.", "itm_m1n2p3q4r5").
		WithHint("Use start %q.", "180f").WithPath("args.start").WithDetail("next_free", 180)
	e.OpIndex = &idx
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"code":"E_OVERLAP","message":"Item itm_m1n2p3q4r5 would occupy frames 100-149.","hint":"Use start \"180f\".","retryable":false,"op_index":1,"path":"args.start","details":{"next_free":180}}`
	if string(b) != want {
		t.Errorf("got %s", b)
	}
	if e.Error() != "E_OVERLAP: Item itm_m1n2p3q4r5 would occupy frames 100-149." {
		t.Error(e.Error())
	}
	if err := CheckError(e); err != nil {
		t.Error(err)
	}
}

func TestAsErrorKeepsCoreErrorsAndHidesTheRest(t *testing.T) {
	core := Errorf(CodeNotFound, "x").WithHint("y")
	if AsError(core) != core {
		t.Error("a core error must pass through")
	}
	if AsError(errors.Join(errors.New("a"), core)) != core {
		t.Error("a wrapped core error must be found")
	}
	got := AsError(errors.New("open C:\\secret\\file.go:12: boom"))
	if got.Code != CodeInternal || strings.Contains(got.Message, "secret") {
		t.Errorf("%+v", got)
	}
	if err := CheckError(got); err != nil {
		t.Error(err)
	}
}

// The message and hint rules of SDD-002 §A.8: never a stack trace, a file path of the
// server or SQL.
func TestCheckTextRejectsPathsStackTracesAndSQL(t *testing.T) {
	bad := []string{
		`failed at C:\Users\me\project\main.go`,
		`cannot open C:/work/db.sqlite`,
		`read /var/lib/vukem/db.sqlite failed`,
		`see (/usr/local/bin/ffmpeg)`,
		`error in editor.go:123`,
		`panic: runtime error: index out of range`,
		`goroutine 17 [running]:`,
		`at command.Apply(`,
		`SELECT id FROM operations WHERE x`,
		`insert into items values`,
		`UPDATE items SET start = 3`,
		`loaded ../secret/file`,
		`script helper.ts failed`,
		`\Users\me\x`,
	}
	for _, s := range bad {
		if CheckText(s) == nil {
			t.Errorf("CheckText(%q) passed", s)
		}
	}
	good := []string{
		`Item itm_m1n2p3q4r5 would occupy frames 100-149 on track trk_f6g7h8j9k0, where item itm_c3d4e5f6g7 already is (frames 90-179).`,
		`Use start "180f" (6.000s), the next free position on that track, or move to another track.`,
		`The time "12" has no unit; use "12.5s", "375f" or "00:00:12.500".`,
		`Update the start to 12.467s/374f and select the item.`,
		`Choose between 1/4 and 4 for the speed; track.add or item.split also work.`,
	}
	for _, s := range good {
		if err := CheckText(s); err != nil {
			t.Errorf("CheckText(%q): %v", s, err)
		}
	}
}

func TestCheckError(t *testing.T) {
	neg := -1
	cases := map[string]*Error{
		"nil":          nil,
		"unknown code": {Code: "E_X", Message: "m", Hint: "h"},
		"no message":   {Code: CodeNotFound, Hint: "h"},
		"no hint":      {Code: CodeNotFound, Message: "m"},
		"retryable":    {Code: CodeRevisionConflict, Message: "m", Hint: "h"},
		"op index":     {Code: CodeNotFound, Message: "m", Hint: "h", OpIndex: &neg},
		"path in text": {Code: CodeNotFound, Message: "failed at C:\\x\\y", Hint: "h"},
		"path in hint": {Code: CodeNotFound, Message: "m", Hint: "see /var/lib/x"},
	}
	for name, e := range cases {
		if CheckError(e) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := CheckError(&Error{Code: CodeInternal, Message: "bug"}); err != nil {
		t.Errorf("E_INTERNAL needs no hint: %v", err)
	}
}

// VV-196: a validation error does not give back the whole value that was sent.
func TestErrorfDoesNotEchoALongValue(t *testing.T) {
	long := strings.Repeat("x", 100_000)
	e := Errorf(CodeValidation, "args.mode %q is not a blend mode.", long)

	if len(e.Message) > 400 || !strings.Contains(e.Message, "is not a blend mode.") ||
		!strings.Contains(e.Message, "100000 characters") {
		t.Errorf("message = %d bytes: %.120s", len(e.Message), e.Message)
	}

	short := Errorf(CodeValidation, "args.mode %q is not a blend mode.", "nope")
	if short.Message != `args.mode "nope" is not a blend mode.` {
		t.Errorf("short message = %q", short.Message)
	}
}

// VV-376: a message may repeat a value the caller sent only escaped by %q and cut at quotedMax (SDD-003 section 1.2
// rule 9), so a value cannot forge a line, reorder the text around it or come back whole.
func TestQuotedCallerValueComesBackEscapedAndCut(t *testing.T) {
	hostile := []string{
		"line one\nE_FORGED: second line",
		"evil" + string(rune(0x202E)) + "fdp" + string(rune(0x2066)),
		"nul\x00byte\r\x1b[31m",
		strings.Repeat(string(rune(0x202E)), 5000),
	}

	unknown := NewRegistry("test").unknown
	fontAdd := func(v string) *Error {
		e, _ := errors.AsType[*Error](applyFontAdd(nil, &fontAddArgs{Family: v}))
		return e
	}

	for _, v := range hostile {
		for name, e := range map[string]*Error{"unknown command": unknown(v), "font.add family": fontAdd(v)} {
			if e == nil {
				t.Fatalf("%s: no error for %q", name, v[:min(len(v), 20)])
			}

			for _, r := range e.Message {
				if isRawControl(r) {
					t.Errorf("%s: the message has the raw character %U: %.80q", name, r, e.Message)
					break
				}
			}

			if len(e.Message) > 2000 {
				t.Errorf("%s: the message is %d bytes long", name, len(e.Message))
			}
		}
	}
}

// isRawControl is a character that can forge a line or reorder the text around it.
func isRawControl(r rune) bool {
	return r == '\n' || r == '\r' || r == 0 || r == 0x1b || (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}
