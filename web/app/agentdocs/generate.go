package agentdocs

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
	"github.com/AlexGalhardo/stuarty/service"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// CLICommand is a command of the CLI, as cli.Reference lists it. The CLI imports
// this package, so the caller passes the list in.
type CLICommand struct {
	Name    string
	Summary string
	Usage   string
	Pending bool
}

// Inputs is everything the templates read.
type Inputs struct {
	Version  string
	Commands []*command.Command
	Lint     []lint.Rule
	CLI      []CLICommand
}

// errorView is a row of the error table.
type errorView struct {
	Code string
	HTTP int
	Exit int
	Fix  string
}

// toolView is a row of the tool table.
type toolView struct {
	Tool     string
	Commands string
}

// view is the data the templates see.
type view struct {
	Inputs
	Errors  []errorView
	Tools   []toolView
	Rules   []ruleView
	Ops     []opGroup
	Recipes []recipeView
}

type recipeView struct {
	Title, Text string
}

type ruleView struct {
	Code, Severity, Blocks, Fix string
}

// opGroup is the operations of one namespace, alpha ones marked with a star.
type opGroup struct {
	Namespace, Names string
}

func newView(in Inputs) (view, error) {
	v := view{Inputs: in}

	recipes, err := Recipes()
	if err != nil {
		return v, err
	}

	for i := range recipes {
		v.Recipes = append(v.Recipes, recipeView{recipes[i].Title, recipes[i].Text()})
	}

	for _, c := range in.Commands {
		if c.Internal {
			continue
		}

		ns, action, _ := strings.Cut(c.Name, ".")
		if c.Phase != command.Prototype {
			action += "*"
		}

		if n := len(v.Ops); n > 0 && v.Ops[n-1].Namespace == ns {
			v.Ops[n-1].Names += ", " + action

			continue
		}

		v.Ops = append(v.Ops, opGroup{ns, action})
	}

	for _, r := range in.Lint {
		blocks := "no"
		if lint.Blocking(r.Code) {
			blocks = "yes"
		}

		v.Rules = append(v.Rules, ruleView{r.Code, r.Severity, blocks, r.Hint})
	}

	for _, e := range Errors {
		exit := 1
		if e.Retryable {
			exit = 2
		}

		v.Errors = append(v.Errors, errorView{
			Code: e.Code, HTTP: (&service.Error{Code: e.Code}).HTTPStatus(), Exit: exit, Fix: e.Fix,
		})
	}

	for _, t := range Tools {
		v.Tools = append(v.Tools, toolView{t.Tool, "`" + strings.Join(t.Commands, "`, `") + "`"})
	}

	return v, nil
}

// Generate renders the three files from the inputs. It is a pure function: the
// same inputs give the same bytes.
func Generate(in Inputs) (map[string][]byte, error) {
	if in.Version == "" {
		in.Version = Version
	}

	tmpl, err := template.New("agentdocs").Option("missingkey=error").ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("agentdocs: parse the templates: %w", err)
	}

	v, err := newView(in)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}

	for file, name := range map[string]string{
		SkillFile: "SKILL.md.tmpl", SnippetFile: "AGENTS.snippet.md.tmpl", InstructionsFile: "instructions.txt.tmpl",
	} {
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, name, v); err != nil {
			return nil, fmt.Errorf("agentdocs: render %s: %w", name, err)
		}

		// One final newline: `agent:docs --print` writes exactly the file.
		out[file] = append(bytes.TrimRight(buf.Bytes(), "\n"), '\n')
	}

	return out, nil
}
