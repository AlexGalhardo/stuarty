package mcp

// The limits of NFR-020 (PRD-002), revised once by epic 13 (D-069) from the measurement of the shape of SDD-003
// section 4.8 and written in SDD-003 section 4.1. `go run ./internal/tools/surfacebudget`, which `task lint`
// runs, and TestToolDefinitionBudget hold the surface to them. Tokens are characters divided by 3.2 (SDD-003
// section 3.10), so 22,400 bytes are 7,000 tokens. They are not raised again for a new domain: a domain adds
// operations, found through command_help, and a tool is added only when no existing intent fits.
const (
	// MaxTools is the number of tools of tools/list.
	MaxTools = 20
	// MaxToolListBytes is the minified tools/list result a client receives: 7,000 tokens. It was set from a
	// measurement of about 21,800 bytes, a margin of about 3 percent.
	MaxToolListBytes = 22_400
	// MaxToolBytes is the largest definition of any one tool. Measured about 1,600 bytes (`media_inspect`, `edit`).
	MaxToolBytes = 1_700
	// MaxDiscoveryChars is the text of the first discovery call, command_help with no argument: the operations of
	// the video domain and one line per domain, 2,340 tokens. Measured about 7,200 characters; it grows by about 90 for every operation added to the video domain.
	MaxDiscoveryChars = 7_500
)
