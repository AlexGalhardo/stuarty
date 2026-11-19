package contract

import "testing"

// TestImageScenarioIsTheSameThroughEverySurface (SDD-021 section 11): the image operations give the same
// results, errors and texts through the in-process Service, the HTTP client, the CLI and the MCP tool, byte for
// byte: a document made blank and from a template, edited in a batch, dry run, replayed by key, undone and redone.
func TestImageScenarioIsTheSameThroughEverySurface(t *testing.T) {
	sameThroughEverySurface(t, "image")
}
