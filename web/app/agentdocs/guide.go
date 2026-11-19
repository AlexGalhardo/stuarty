package agentdocs

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"text/template"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
)

// The guide has one page per domain (SDD-003 section 8.1, epic 13): what the domain is, the MCP tools and the CLI
// commands that reach it, and its edit operations. The pages are generated from the registries, the tool and
// command tables and the domain table, like the three files of section 8.1, committed under out/guide/ and
// embedded in the binary; the site publishes them under Agents.

// GuideDir is the folder of the pages inside out/.
const GuideDir = "guide"

// GuideFile is the name of the page of a domain, relative to out/.
func GuideFile(domain string) string { return GuideDir + "/" + domain + ".md" }

// GuideFiles lists the pages in the order of the domains (SDD-000 section 1.1).
func GuideFiles() []string {
	names := command.DomainNames()
	files := make([]string, len(names))

	for i, d := range names {
		files[i] = GuideFile(d)
	}

	return files
}

// Guide returns the page of a domain as committed.
func Guide(domain string) ([]byte, error) {
	if !command.KnownDomain(domain) {
		return nil, fmt.Errorf("agentdocs: %q is not a domain (%s)", domain, strings.Join(command.DomainNames(), ", "))
	}

	return File(GuideFile(domain))
}

// guideView is what the page template sees.
type guideView struct {
	Version string
	Name    string
	Summary string
	// Tools are the MCP tools that belong to the domain, in the order of the tool table; Routed are those among them
	// that take the domain as an argument.
	Tools  []toolView
	Routed []string
	// CLI are the commands of the domain.
	CLI     []CLICommand
	Ops     []opGroup
	OpCount int
}

// groupOps groups the public commands by namespace, in the order they come.
func groupOps(cmds []*command.Command) []opGroup {
	var out []opGroup

	for _, c := range cmds {
		if c.Internal {
			continue
		}

		ns, action, _ := strings.Cut(c.Name, ".")
		if n := len(out); n > 0 && out[n-1].Namespace == ns {
			out[n-1].Names += ", " + action

			continue
		}

		out = append(out, opGroup{ns, action})
	}

	return out
}

func countPublic(cmds []*command.Command) int {
	n := 0

	for _, c := range cmds {
		if !c.Internal {
			n++
		}
	}

	return n
}

// operationsOf are the commands of the registry of a domain: the timeline's for video, the image documents' for
// image, none for a domain with no registry yet.
func operationsOf(in Inputs, domain string) []*command.Command {
	switch domain {
	case command.DomainVideo:
		return in.Commands
	case command.DomainImage:
		return coreimage.Registry().List()
	}

	return nil
}

func guideViews(in Inputs) []guideView {
	var views []guideView

	for _, d := range command.Domains() {
		v := guideView{Version: in.Version, Name: d.Name, Summary: d.Summary}

		for _, t := range Tools {
			if !slices.Contains(in.ToolDomains[t.Tool], d.Name) {
				continue
			}

			v.Tools = append(v.Tools, toolView{t.Tool, "`" + strings.Join(t.Commands, "`, `") + "`"})

			if slices.Contains(in.ToolRoutes[t.Tool], d.Name) {
				v.Routed = append(v.Routed, t.Tool)
			}
		}

		for _, c := range in.GuideCLI {
			if c.Domain == d.Name {
				v.CLI = append(v.CLI, c)
			}
		}

		ops := operationsOf(in, d.Name)
		v.Ops, v.OpCount = groupOps(ops), countPublic(ops)

		views = append(views, v)
	}

	return views
}

// generateGuides renders the page of every domain into out, under its name relative to out/.
func generateGuides(in Inputs, tmpl *template.Template, out map[string][]byte) error {
	views := guideViews(in)
	for i := range views {
		v := &views[i]
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "guide.md.tmpl", v); err != nil {
			return fmt.Errorf("agentdocs: render the guide of %s: %w", v.Name, err)
		}

		out[GuideFile(v.Name)] = append(bytes.TrimRight(buf.Bytes(), "\n"), '\n')
	}

	return nil
}
