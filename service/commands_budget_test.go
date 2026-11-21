package service

import (
	"slices"
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

	near := unknownCommand(reg, "", "track.ad")
	if !strings.Contains(near.Hint, "track.add") {
		t.Errorf("the closest names of track.ad are missing track.add: %q", near.Hint)
	}
}

// A typo of any operation suggests that operation, also in a group with more names than the error lists.
func TestUnknownNameSuggestsTheNameMeant(t *testing.T) {
	t.Parallel()

	reg := command.Default()

	for _, c := range reg.List() {
		if c.Internal {
			continue
		}

		typo := c.Name[:len(c.Name)-1]
		if _, exists := reg.Get(typo); exists {
			continue
		}

		if e := unknownCommand(reg, "", typo); !slices.Contains(strings.FieldsFunc(e.Hint, func(r rune) bool {
			return r == ' ' || r == ',' || r == ';'
		}), c.Name) {
			t.Errorf("%q does not suggest %s: %q", typo, c.Name, e.Hint)
		}
	}
}

// The list an error points at is the list of the domain that was asked.
func TestUnknownNameInADomainPointsAtThatDomain(t *testing.T) {
	t.Parallel()

	if e := unknownCommand(command.Default(), "image", "nope.op"); !strings.Contains(e.Hint, "domain=image") {
		t.Errorf("hint = %q, want the list of domain=image", e.Hint)
	}
}
