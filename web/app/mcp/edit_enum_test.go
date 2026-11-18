package mcp

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
)

// The op enum of the edit tool is written by hand in definitions/edit.json; it
// must name exactly the public commands of the build, or a client that
// validates against the schema refuses a command the server accepts.
func TestEditEnumListsEveryPublicCommand(t *testing.T) {
	t.Parallel()

	raw, err := definitionFiles.ReadFile("definitions/edit.json")
	if err != nil {
		t.Fatal(err)
	}

	var def struct {
		InputSchema struct {
			Properties struct {
				Operations struct {
					Items struct {
						Properties struct {
							Op struct {
								Enum []string `json:"enum"`
							} `json:"op"`
						} `json:"properties"`
					} `json:"items"`
				} `json:"operations"`
			} `json:"properties"`
		} `json:"inputSchema"`
	}
	if err := json.Unmarshal(raw, &def); err != nil {
		t.Fatal(err)
	}

	got := slices.Clone(def.InputSchema.Properties.Operations.Items.Properties.Op.Enum)
	slices.Sort(got)

	reg := command.Default()

	var want []string

	for _, name := range reg.Names() {
		if c, ok := reg.Get(name); ok && !c.Internal {
			want = append(want, name)
		}
	}

	if !slices.Equal(got, want) {
		t.Errorf(
			"the op enum of the edit tool differs from the public commands:\n only in the enum: %v\n only in the registry: %v",
			diff(got, want),
			diff(want, got),
		)
	}
}

func diff(a, b []string) []string {
	var out []string

	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}

	return out
}
