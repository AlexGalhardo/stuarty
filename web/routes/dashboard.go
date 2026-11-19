package routes

import (
	"github.com/AlexGalhardo/vukem/inertia"
	"github.com/AlexGalhardo/vukem/router"
	"github.com/AlexGalhardo/vukem/web"
)

// dashboardSections are the sections of the sidebar that have no page of their
// own yet (SDD-012 section 23.1): each path shows the empty state of
// Dashboard/Section, which says what the section will hold. When the epic of a
// section builds its page, it registers its route and drops its key from this
// list. Projects (/projects) and the settings pages are not here.
//
// The keys are the paths the sidebar links to in resources/js/dashboard/sections.ts;
// TestDashboardSectionsMatchTheSidebar compares the two lists.
var dashboardSections = []string{
	"edit-video", "edit-image", "write", "visual-assets", "video-assets",
	"publish", "skills", "prompts", "memory", "connections",
}

// dashboardPages installs the home and one empty-state route for each section
// of dashboardSections. A handler only names the page; it computes nothing.
func dashboardPages(r *router.Router) {
	r.Get("/{$}", func(c *web.Ctx) error {
		return inertia.Render(c, "Dashboard/Home", inertia.Props{})
	}).Name("home")

	for _, key := range dashboardSections {
		r.Get("/"+key, func(c *web.Ctx) error {
			return inertia.Render(c, "Dashboard/Section", inertia.Props{"section": key})
		}).Name("dashboard." + key)
	}
}
