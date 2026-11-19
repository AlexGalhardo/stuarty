package mcp

// The limits of NFR-020 (PRD-002), set by epic 13 (D-069) from the measurement of the shape of SDD-003 section
// 4.8, raised by the author (D-094) when epics 10 and 17 met the ceiling, and written in SDD-003 section 4.1.
// `go run ./internal/tools/surfacebudget`, which `task lint` runs, and TestToolDefinitionBudget hold the surface
// to them. Tokens are characters divided by 3.2 (SDD-003 section 3.10), so 23,400 bytes are about 7,300 tokens.
// A domain adds operations, found through command_help, and a tool is added only when no existing intent fits.
const (
	// MaxTools is the number of tools of tools/list.
	MaxTools = 20
	// MaxToolListBytes is the minified tools/list result a client receives: about 7,300 tokens.
	MaxToolListBytes = 23_400
	// MaxToolBytes is the largest definition of any one tool. Measured about 1,600 bytes (`media_inspect`, `edit`).
	MaxToolBytes = 1_700
	// MaxDiscoveryChars is the text of the first discovery call, command_help with no argument: the operations of
	// the video domain and one line per domain, about 2,650 tokens. It grows by about 40 to 90 characters for every
	// operation added to the video domain (raised from 7,500 by D-094).
	MaxDiscoveryChars = 8_500
)
