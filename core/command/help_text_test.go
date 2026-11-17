package command

import (
	"strings"
	"testing"
)

// The help of track.set_caption_style once said highlight accepts only "none" while
// the schema also takes "word" (VV-100).
func TestCaptionStyleHelpMatchesHighlightEnum(t *testing.T) {
	help, err := Default().Help("track.set_caption_style")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help, "accepts only") {
		t.Errorf("help is out of date: %s", help)
	}
}
