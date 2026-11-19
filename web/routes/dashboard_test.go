package routes

import (
	"os"
	"regexp"
	"slices"
	"testing"
)

// The sidebar and the routes agree on the sections (SDD-012 section 23.1): every
// key of SECTION_KEYS is either a section with a page of its own (projects) or a
// route of dashboardSections, and no route lacks a sidebar entry.
func TestDashboardSectionsMatchTheSidebar(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("../resources/js/dashboard/sections.ts")
	if err != nil {
		t.Fatal(err)
	}

	list := regexp.MustCompile(`(?s)SECTION_KEYS = \[(.*?)\] as const`).FindSubmatch(src)
	if list == nil {
		t.Fatal("sections.ts has no SECTION_KEYS list")
	}

	var keys []string
	for _, m := range regexp.MustCompile(`"([a-z-]+)"`).FindAllSubmatch(list[1], -1) {
		keys = append(keys, string(m[1]))
	}

	if len(keys) != 11 {
		t.Fatalf("the sidebar lists %d sections, want the eleven of RF-311: %v", len(keys), keys)
	}

	// The sections with a page of their own, served by Pages.
	own := []string{"projects"}

	for _, key := range keys {
		if !slices.Contains(own, key) && !slices.Contains(dashboardSections, key) {
			t.Errorf("the sidebar section %q has no route", key)
		}
	}

	for _, key := range dashboardSections {
		if !slices.Contains(keys, key) {
			t.Errorf("the route /%s has no entry in the sidebar", key)
		}
	}
}
