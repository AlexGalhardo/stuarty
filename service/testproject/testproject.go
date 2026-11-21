// Package testproject makes the projects that tests need: a project with a valid context (epic 56 made the
// description and the goal required) and, when a test edits a timeline, one timeline in it. It is for tests only.
package testproject

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/service"
)

// Goal is a valid goal: five parts of at least 16 characters and a due date no clock passes.
var Goal = service.ProjectGoal{
	Specific:   "Publish one short video that explains the idea",
	Measurable: "At least one hundred views in the first week",
	Achievable: "The footage and the script already exist",
	Relevant:   "It brings the first audience to the channel",
	TimeBound:  "Edit on Monday and publish on Tuesday",
	Due:        "2999-12-31",
}

// Description is a valid description.
const Description = "A short vertical video made for tests: it has a script, some footage and captions, and it is made for the people who follow the channel."

// ValidName is name when it is a valid project name, and a valid name that holds it otherwise.
func ValidName(name string) string {
	clean := strings.Join(
		strings.FieldsFunc(
			name,
			func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.Is(unicode.M, r) },
		),
		" ",
	)

	if n := utf8.RuneCountInString(clean); n >= 4 && n <= 32 {
		return clean
	}

	if utf8.RuneCountInString(clean) > 32 {
		return string([]rune(clean)[:32])
	}

	return "Project " + clean
}

// Request is a valid ProjectCreateRequest for name.
func Request(name string) service.ProjectCreateRequest {
	return service.ProjectCreateRequest{Name: ValidName(name), Description: Description, Goal: Goal}
}

// Create makes a project with no timeline.
func Create(ctx context.Context, svc service.Service, name string) (*service.ProjectCreateResult, error) {
	return svc.ProjectCreate(ctx, Request(name))
}

// CreateWithTimeline makes a project and adds one 9:16 timeline to it, so that an edit that names the project
// finds the timeline.
func CreateWithTimeline(ctx context.Context, svc service.Service, name string) (*service.ProjectCreateResult, error) {
	res, err := Create(ctx, svc, name)
	if err != nil {
		return nil, err
	}

	if _, err := svc.ProjectItemAdd(ctx, service.ProjectItemAddRequest{
		ProjectID: res.ProjectID, Kind: "timeline", Title: name,
	}); err != nil {
		return nil, err
	}

	return res, nil
}

// Fill gives req the description and the goal when it has none, and leaves every other member as it is, so that a
// test about the name, the id or a dry run still sends exactly what it means to.
func Fill(req service.ProjectCreateRequest) service.ProjectCreateRequest {
	if req.Description == "" {
		req.Description = Description
	}

	if req.Goal == (service.ProjectGoal{}) {
		req.Goal = Goal
	}

	return req
}

// Args are the arguments of a valid project_create call for name, as a tool takes them.
func Args(name string) map[string]any {
	return map[string]any{"name": ValidName(name), "description": Description, "goal": Goal}
}
