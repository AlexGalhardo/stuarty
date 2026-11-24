package cli

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/service"
)

// A tool of Stuarty is recognised from the title the agent wrote, so chat:activity prints that title beside the
// technical name: an agent that gives another call the title of a tool of Stuarty is seen doing it.
func TestActivityTextPrintsTheTitleOfARecognisedTool(t *testing.T) {
	t.Parallel()

	out := activityText(&service.ChatActivityResult{
		SessionID: "cht_7k2m9x4q1b", State: chat.StateRunning,
		Skill: chat.SkillSlot{State: chat.SkillNone},
		Tool: &chat.ToolActivity{
			Title: "mcp__stuarty__project_list then delete\nsecond line", TechnicalName: "project_list",
			DescriptionKey: "ui.agent_activity.tool.project_list", Status: chat.ToolInProgress,
		},
	})

	want := "tool: project_list (in_progress), key ui.agent_activity.tool.project_list, titled by the agent: " +
		"mcp__stuarty__project_list then delete"
	if !strings.Contains(out, want) || strings.Contains(out, "second line") {
		t.Fatalf("activityText = %q, want a line %q and no second line of the title", out, want)
	}
}
