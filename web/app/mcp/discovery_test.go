package mcp

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
)

// publicNames are the names of the commands of a registry that an agent may issue.
func publicNames(reg *command.Registry) []string {
	var names []string

	for _, c := range reg.List() {
		if !c.Internal {
			names = append(names, c.Name)
		}
	}

	return names
}

type discoveredCommand struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

type discovery struct {
	Commands []discoveredCommand `json:"commands"`
	Domains  []struct {
		Name       string `json:"name"`
		Summary    string `json:"summary"`
		Operations int    `json:"operations"`
	} `json:"domains"`
}

func discover(t *testing.T, args map[string]any) (discovery, string) {
	t.Helper()

	cs := connect(t, newService(t), Options{}, "claude-code")
	res := ok(t, call(t, cs, "command_help", args))

	var out discovery

	if err := json.Unmarshal(marshal(t, res.StructuredContent), &out); err != nil {
		t.Fatal(err)
	}

	return out, textOf(t, res)
}

// TestDiscoveryListsTheRegistry: the one discovery call lists the operations of the default domain exactly as the
// registry has them, and one line for every domain (SDD-003 section 4.8).
func TestDiscoveryListsTheRegistry(t *testing.T) {
	t.Parallel()

	out, text := discover(t, map[string]any{})

	var got []string
	for _, c := range out.Commands {
		got = append(got, c.Name)
	}

	if want := publicNames(command.Default()); !slices.Equal(got, want) {
		t.Errorf("the operations are %v, want the registry's %v", got, want)
	}

	var domains []string

	for _, d := range out.Domains {
		domains = append(domains, d.Name)

		if !strings.Contains(text, d.Name+": ") {
			t.Errorf("the text has no line for the domain %s", d.Name)
		}

		var want int

		switch d.Name {
		case "video":
			want = len(publicNames(command.Default()))
		case "image":
			want = len(publicNames(coreimage.Registry()))
		}

		if d.Operations != want {
			t.Errorf("%s: %d operations, want %d", d.Name, d.Operations, want)
		}

		if d.Summary == "" || len(d.Summary) > command.MaxDomainSummary {
			t.Errorf("%s: the summary is %d characters", d.Name, len(d.Summary))
		}
	}

	if !slices.Equal(domains, command.DomainNames()) {
		t.Errorf("the domains are %v, want %v", domains, command.DomainNames())
	}
}

// TestDiscoveryByDomain: a domain lists its own operations; a domain with none says so; an unknown one is refused.
func TestDiscoveryByDomain(t *testing.T) {
	t.Parallel()

	image, _ := discover(t, map[string]any{"domain": "image"})

	var got []string
	for _, c := range image.Commands {
		got = append(got, c.Name)
	}

	if want := publicNames(coreimage.Registry()); !slices.Equal(got, want) || len(want) == 0 {
		t.Errorf("the image operations are %v, want %v", got, want)
	}

	if len(image.Domains) != 0 {
		t.Error("a call with a domain also listed the domains")
	}

	for _, empty := range []string{"writing", "publish", "provider", "memory", "asset", "project", "workspace"} {
		out, text := discover(t, map[string]any{"domain": empty})
		if len(out.Commands) != 0 || !strings.Contains(text, "No edit operations in the domain "+empty) {
			t.Errorf("%s: %q, want no operations and a line that says so", empty, text)
		}
	}

	cs := connect(t, newService(t), Options{}, "claude-code")
	res := call(t, cs, "command_help", map[string]any{"domain": "videos"})

	if !res.IsError || !strings.Contains(textOf(t, res), `"path":"domain"`) {
		t.Errorf("an unknown domain was not refused: %q", textOf(t, res))
	}
}

// TestDiscoveryByName: the schema of an operation is fetched on demand from the domain that has it, and not from
// another one.
func TestDiscoveryByName(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")

	video := ok(t, call(t, cs, "command_help", map[string]any{"name": "item.split"}))
	if !strings.Contains(textOf(t, video), "JSON Schema") {
		t.Errorf("item.split has no schema: %q", textOf(t, video))
	}

	imageOp := publicNames(coreimage.Registry())[0]

	if res := call(t, cs, "command_help", map[string]any{"name": imageOp}); !res.IsError {
		t.Errorf("the image operation %s was found in the video domain", imageOp)
	}

	img := ok(t, call(t, cs, "command_help", map[string]any{"name": imageOp, "domain": "image"}))
	if !strings.Contains(textOf(t, img), imageOp) {
		t.Errorf("%s has no help in the image domain: %q", imageOp, textOf(t, img))
	}

	empty := call(t, cs, "command_help", map[string]any{"name": "item.split", "domain": "writing"})
	if !empty.IsError || !strings.Contains(textOf(t, empty), "no edit operations yet") {
		t.Errorf("a name in an empty domain: %q", textOf(t, empty))
	}
}
