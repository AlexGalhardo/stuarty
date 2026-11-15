package main

// scenario is one task the agent is given, and what counts as done.
type scenario struct {
	Name   string
	Prompt string
	// Required lists the commands that must have run with exit code 0 at least once.
	Required []string
	// NeedsClip is true when the task imports a video file the harness supplies as clip.mp4.
	NeedsClip bool
}

// scenarios are the two of PRD-002 success metrics 1 and 2 (SDD-003 section 9, check 29). The
// agent is told nothing about the commands: SKILL.md and `help` are its only sources.
var scenarios = []scenario{
	{
		Name: "prototype",
		Prompt: "Make a 2 second vertical teaser from the file clip.mp4 in the working directory. " +
			"Create a project named \"Teaser\", import the clip, put it on a video track, " +
			"cut it to its first 2 seconds, add the caption \"Launch day\" over it, " +
			"check the project for problems and export it. Stop when the export has finished.",
		Required:  []string{"project:create", "media:import", "edit", "export"},
		NeedsClip: true,
	},
	{
		Name: "template",
		Prompt: "Create a project named \"Base\" with one text item that says \"Headline\", " +
			"mark that item as a template slot named headline, save the project as a template " +
			"named \"Spring sale\", create a new project from that template with the headline " +
			"\"Spring sale\", and export the new project. Stop when the export has finished.",
		Required: []string{"project:create", "edit", "template:save", "template:new", "export"},
	},
}

func findScenarios(name string) ([]scenario, bool) {
	if name == "all" {
		return scenarios, true
	}

	for _, s := range scenarios {
		if s.Name == name {
			return []scenario{s}, true
		}
	}

	return nil, false
}
