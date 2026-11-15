package cli

import "testing"

func TestReferenceListsEveryCommandWithItsFlags(t *testing.T) {
	t.Parallel()

	doc := Reference()
	if len(doc.Commands) != len(registry()) {
		t.Fatalf("%d commands in the reference, %d in the registry", len(doc.Commands), len(registry()))
	}

	for _, c := range doc.Commands {
		if c.Summary == "" || len(c.Usage) == 0 {
			t.Errorf("%s: the reference needs a summary and a usage line", c.Name)
		}

		if c.Pending != "" {
			continue
		}

		for _, g := range c.Globals {
			if g == "" {
				t.Errorf("%s: an empty global flag name", c.Name)
			}
		}
	}

	if len(doc.ExitCodes) != 3 || len(doc.Globals) == 0 {
		t.Errorf("exit codes %d, globals %d", len(doc.ExitCodes), len(doc.Globals))
	}
}
