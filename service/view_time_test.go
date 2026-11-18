package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func TestParseTime_HintNamesTheLastTimeWithOneSuffix(t *testing.T) {
	doc := &timeline.Document{
		Settings: timeline.Settings{FPS: timeline.Rational{Num: 30, Den: 1}},
		Items:    map[string]timeline.Item{"itm_aaaaaaaaaa": {Start: 0, Duration: 600}},
	}
	v := &view{doc: doc}

	_, err := v.parseTime("at", "600f")
	if err == nil {
		t.Fatal("expected E_INVALID_TIME past the last frame")
	}

	var se *Error
	if !errors.As(err, &se) || !strings.HasSuffix(se.Hint, "19.967s") {
		t.Fatalf("hint = %+v, want it to end with 19.967s", err)
	}

	if _, err := v.parseTime("at", "599f"); err != nil {
		t.Fatalf("the last frame must parse: %v", err)
	}
}
