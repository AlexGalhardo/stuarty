package service

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// The list of one domain and the error of an unknown name stay inside their caps, and still name every operation (VV-311).
func TestCommandHelpStaysInsideItsCaps(t *testing.T) {
	t.Parallel()

	reg := command.Default()
	list := commandList(reg, commandListCap)

	if len(list.Text) > commandListCap {
		t.Errorf("the list is %d characters, cap %d", len(list.Text), commandListCap)
	}

	for _, c := range list.Commands {
		if !strings.Contains(list.Text, c.Name) {
			t.Errorf("%s is not in the text of the list", c.Name)
		}
	}

	err := unknownCommand(reg, "", "nope.op")
	if got := len(err.Text(true)); got > errorTextCap {
		t.Errorf("the error is %d characters, cap %d", got, errorTextCap)
	}

	if got := len(err.Hint); got > 1000 {
		t.Errorf("the hint is %d characters", got)
	}

	close := unknownCommand(reg, "", "track.ad")
	if !strings.Contains(close.Hint, "track.add") {
		t.Errorf("the closest names of track.ad are missing track.add: %q", close.Hint)
	}
}
