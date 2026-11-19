package skills_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/image"
	library "github.com/AlexGalhardo/stuarty/core/library"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/agentdocs"
	"github.com/AlexGalhardo/stuarty/web/app/cli"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
	"github.com/AlexGalhardo/stuarty/web/resources/skills"
)

// loaded is a built-in skill as the product installs it.
type loaded struct {
	name  string
	files []library.File
	skill library.Skill
}

func loadAll(t *testing.T) []loaded {
	t.Helper()

	set := skills.FS()

	dirs, err := fs.ReadDir(set, ".")
	if err != nil {
		t.Fatal(err)
	}

	var out []loaded

	for _, d := range dirs {
		if !d.IsDir() {
			t.Errorf("%s is not a skill folder", d.Name())

			continue
		}

		var files []library.File

		err := fs.WalkDir(set, d.Name(), func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return err
			}

			data, err := fs.ReadFile(set, p)
			files = append(files, library.File{Path: strings.TrimPrefix(p, d.Name()+"/"), Data: data})

			return err
		})
		if err != nil {
			t.Fatal(err)
		}

		s, err := library.ParseSkill(files)
		if err != nil {
			t.Fatalf("%s: %v", d.Name(), err)
		}

		out = append(out, loaded{name: d.Name(), files: files, skill: s})
	}

	return out
}

// names is what a skill may name: the registry of operations of the product.
type names struct {
	ops      map[string]bool // service operations
	cli      map[string]bool // commands of the CLI
	edit     map[string]bool // operations of edit and of image edit
	spaces   map[string]bool // the namespaces of the edit operations
	tools    map[string]bool // MCP tools
	words    map[string]bool // property names of the MCP tools and the other words a skill may write in snake case
	routes   map[string]bool // "GET /projects/{}/brief"
	errors   map[string]bool
	lintCode map[string]bool
}

func set(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, i := range items {
		m[i] = true
	}

	return m
}

func registry(t *testing.T) names {
	t.Helper()

	n := names{
		ops: map[string]bool{}, tools: map[string]bool{}, words: map[string]bool{}, routes: map[string]bool{},
		errors: map[string]bool{}, lintCode: map[string]bool{}, spaces: map[string]bool{},
	}

	for op := range service.Scopes {
		n.ops[op] = true
	}

	all, _ := cli.CommandNames()
	n.cli = set(all...)

	edit := append(command.Default().Names(), image.Registry().Names()...)
	n.edit = set(edit...)

	for _, e := range edit {
		before, _, _ := strings.Cut(e, ".")
		n.spaces[before] = true
	}

	for _, p := range image.PresetNames() {
		n.words[p] = true
	}

	for _, e := range httpapi.Endpoints {
		n.routes[e.Method+" "+braces(e.Path)] = true
	}

	for _, e := range agentdocs.Errors {
		n.errors[e.Code] = true
	}

	for _, r := range lint.Catalog() {
		n.lintCode[r.Code] = true
	}

	files, err := filepath.Glob(filepath.Join("..", "..", "app", "mcp", "definitions", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("the definitions of the MCP tools: %v %v", files, err)
	}

	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}

		var def struct {
			Name        string `json:"name"`
			InputSchema any    `json:"inputSchema"`
		}
		if err := json.Unmarshal(raw, &def); err != nil {
			t.Fatal(err)
		}

		n.tools[def.Name] = true
		collectProps(def.InputSchema, n.words)
	}

	return n
}

// collectProps adds the property names of a JSON Schema, at any depth.
func collectProps(node any, into map[string]bool) {
	switch v := node.(type) {
	case map[string]any:
		if props, ok := v["properties"].(map[string]any); ok {
			for k := range props {
				into[k] = true
			}
		}

		for _, child := range v {
			collectProps(child, into)
		}
	case []any:
		for _, child := range v {
			collectProps(child, into)
		}
	}
}

var braceRE = regexp.MustCompile(`\{[^}]*\}`)

func braces(p string) string { return braceRE.ReplaceAllString(p, "{}") }

var (
	spanRE   = regexp.MustCompile("`([^`\n]+)`")
	routeRE  = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE) (/\S*)$`)
	pascalRE = regexp.MustCompile(`^[A-Z][a-z0-9]+(?:[A-Z][a-z0-9]+)+$`)
	cliRE    = regexp.MustCompile(`^[a-z]+:[a-z]+$`)
	dottedRE = regexp.MustCompile(`^([a-z]+)\.([a-z_]+)$`)
	snakeRE  = regexp.MustCompile(`^[a-z]+(?:_[a-z]+)+$`)
	errRE    = regexp.MustCompile(`^E_[A-Z_]+$`)
	lintRE   = regexp.MustCompile(`^L_[A-Z_]+$`)
)

// notYet is the marker of a step that waits for an operation of a later epic: the line is not checked, and the
// epic that adds the operation removes the marker.
const notYet = "Not available yet:"

// unknownNames returns what the text names in code spans that the registry does not have.
func unknownNames(text string, n names) []string {
	var bad []string

	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), notYet) {
			continue
		}

		for _, m := range spanRE.FindAllStringSubmatch(line, -1) {
			bad = append(bad, checkSpan(m[1], n)...)
		}
	}

	return bad
}

func checkSpan(span string, n names) []string {
	span = strings.TrimSpace(span)

	if m := routeRE.FindStringSubmatch(span); m != nil {
		if !n.routes[m[1]+" "+braces(m[2])] {
			return []string{"route " + span}
		}

		return nil
	}

	span = strings.TrimPrefix(span, "stuarty ")
	tokens := strings.Fields(span)

	if len(tokens) == 0 {
		return nil
	}

	var bad []string

	// A command line: the first word is a command of the CLI when the span gives it arguments.
	if len(tokens) > 1 && (strings.Contains(span, " --") || strings.Contains(span, "<")) && !strings.Contains(tokens[0], "=") {
		if first := tokens[0]; (cliRE.MatchString(first) || n.cli[first]) && !n.cli[first] {
			bad = append(bad, "command "+first)
		} else if !cliRE.MatchString(first) && !n.cli[first] && !dottedRE.MatchString(first) && !snakeRE.MatchString(first) {
			bad = append(bad, "command "+first)
		}
	}

	for _, tok := range tokens {
		tok = strings.Trim(tok, ".,;:()[]<>\"")
		if tok == "" || strings.ContainsAny(tok, "=<>") || strings.HasPrefix(tok, "-") {
			continue
		}

		switch {
		case errRE.MatchString(tok):
			if !n.errors[tok] {
				bad = append(bad, "error code "+tok)
			}
		case lintRE.MatchString(tok):
			if !n.lintCode[tok] {
				bad = append(bad, "lint code "+tok)
			}
		case pascalRE.MatchString(tok):
			if !n.ops[tok] {
				bad = append(bad, "operation "+tok)
			}
		case cliRE.MatchString(tok):
			if !n.cli[tok] {
				bad = append(bad, "command "+tok)
			}
		case dottedRE.MatchString(tok):
			if m := dottedRE.FindStringSubmatch(tok); n.spaces[m[1]] && !n.edit[tok] {
				bad = append(bad, "edit operation "+tok)
			}
		case snakeRE.MatchString(tok):
			if !n.tools[tok] && !n.words[tok] {
				bad = append(bad, "tool or argument "+tok)
			}
		}
	}

	return bad
}

// Every operation, tool, command, route, error code and lint code a built-in skill names exists (SDD-018
// section 16): a renamed operation breaks the build, not the agent.
func TestEverySkillNamesRealOperations(t *testing.T) {
	t.Parallel()

	n := registry(t)

	for _, s := range loadAll(t) {
		for _, f := range s.files {
			if !strings.HasSuffix(f.Path, ".md") {
				continue
			}

			if bad := unknownNames(string(f.Data), n); len(bad) > 0 {
				t.Errorf("%s/%s names what the product does not have: %v", s.name, f.Path, bad)
			}
		}
	}
}

// The check is not a rubber stamp: a skill that names an unknown operation fails it.
func TestTheRegistryCheckCatchesAnUnknownName(t *testing.T) {
	t.Parallel()

	n := registry(t)

	good := "Use `project:create --name X`, `BriefSet`, `clip.add`, `project_item`, `PUT /projects/{id}/briefing`, " +
		"`E_VALIDATION` and `L_SAFE_AREA`.\nNot available yet: `PublicationPrepare` and `publish:prepare`.\n"
	if bad := unknownNames(good, n); len(bad) != 0 {
		t.Fatalf("a good text was refused: %v", bad)
	}

	for name, text := range map[string]string{
		"an operation":     "Call `BriefDestroy` now.",
		"a command":        "Run `brief:vanish` first.",
		"an unknown flow":  "Run `stuarty vanish --project <id>`.",
		"an edit op":       "Apply `clip.explode`.",
		"an image op":      "Apply `layer.explode`.",
		"a tool":           "Use the `project_itemz` tool.",
		"a route":          "Send `POST /projects/{id}/nothing`.",
		"an error code":    "Expect `E_NOPE`.",
		"a lint code":      "Read `L_NOPE`.",
		"a name in a line": "1. Run `edit --project <id> clip.explode id=x`.",
	} {
		if bad := unknownNames(text, n); len(bad) == 0 {
			t.Errorf("%s was not caught: %q", name, text)
		}
	}
}
