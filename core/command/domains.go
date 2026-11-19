package command

import "slices"

// The domains of the product (SDD-000 section 1.1, ADR-0043). Every operation belongs to one of them; a
// registry belongs to one, and the commands it holds inherit it.
const (
	DomainProject   = "project"
	DomainVideo     = "video"
	DomainImage     = "image"
	DomainWriting   = "writing"
	DomainAsset     = "asset"
	DomainPublish   = "publish"
	DomainProvider  = "provider"
	DomainMemory    = "memory"
	DomainWorkspace = "workspace"
)

// DomainInfo is a domain and the line that describes it to an agent (at most MaxDomainSummary characters).
type DomainInfo struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

// MaxDomainSummary is the budget of one domain description (SDD-003 section 4.8, NFR-020).
const MaxDomainSummary = 160

var domains = []DomainInfo{
	{DomainProject, "Productions: projects, their timelines and images as items, the brief, bundles."},
	{
		DomainVideo,
		"The timeline: edit operations, history, lint, frames and sheets, export and transcribe jobs, templates.",
	},
	{DomainImage, "Image documents: layers, text, shapes, templates with slots, export to PNG, JPEG or WebP."},
	{
		DomainWriting,
		"Scripts, copy and captions of a production: revisions, network limits, duration estimate, drafts by a model.",
	},
	{
		DomainAsset,
		"Files of the workspace: media import and inspection, the asset library (origin, tags, reuse), fonts.",
	},
	{DomainPublish, "Publications and the approval queue that gates them and costly generations."},
	{DomainProvider, "AI providers and generation: models, prices, text generation, the spend and its ceilings."},
	{DomainMemory, "The library: memory notes, skills, prompts and search."},
	{DomainWorkspace, "The installation: command help, connections, brand kit, health, audit."},
}

// Domains lists the domains in the order of SDD-000 section 1.1.
func Domains() []DomainInfo { return slices.Clone(domains) }

// KnownDomain reports whether name is a domain of the product.
func KnownDomain(name string) bool {
	return slices.ContainsFunc(domains, func(d DomainInfo) bool { return d.Name == name })
}

// DomainNames lists the names of the domains.
func DomainNames() []string {
	names := make([]string, len(domains))
	for i, d := range domains {
		names[i] = d.Name
	}

	return names
}
