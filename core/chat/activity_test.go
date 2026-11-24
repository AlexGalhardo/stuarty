package chat_test

import (
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/chat"
)

var t0 = time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)

func skill(seq int64, name, hash string) chat.Event {
	return chat.Event{
		Seq: seq, Kind: chat.KindSkillLoaded, At: t0.Add(time.Duration(seq) * time.Second),
		Skill: &chat.SkillRef{Name: name, Hash: hash},
	}
}

func call(seq int64, title, action string, status chat.ToolStatus) chat.Event {
	return chat.Event{
		Seq: seq, Kind: chat.KindToolCall, At: t0.Add(time.Duration(seq) * time.Second),
		Tool: &chat.Tool{CallID: "c", Title: title, Action: action, Status: status},
	}
}

func message(seq int64) chat.Event {
	return chat.Event{Seq: seq, Kind: chat.KindAgentMessage, At: t0.Add(time.Duration(seq) * time.Second), Text: "hi"}
}

func TestReduceSkill(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		events   []chat.Event
		reviewed bool
		state    chat.SkillState
		current  string
		loaded   []string
	}{
		{"no events, nothing reviewed", nil, false, chat.SkillNone, "", nil},
		{"no events, a reviewed skill exists", nil, true, chat.SkillAvailable, "", nil},
		{"messages only", []chat.Event{message(1), message(2)}, true, chat.SkillAvailable, "", nil},
		{
			"one skill read",
			[]chat.Event{message(1), skill(2, "captions", "h1")},
			true, chat.SkillLoaded, "captions",
			[]string{"captions"},
		},
		{
			"loaded wins when nothing is reviewed",
			[]chat.Event{skill(1, "captions", "h1")},
			false, chat.SkillLoaded,
			"captions",
			[]string{"captions"},
		},
		{
			"two skills, the last is current",
			[]chat.Event{skill(1, "a", "h"), skill(2, "b", "h")},
			true, chat.SkillLoaded, "b",
			[]string{"a", "b"},
		},
		{
			"a skill read again moves to the end",
			[]chat.Event{skill(1, "a", "h"), skill(2, "b", "h"), skill(3, "a", "h2")},
			true, chat.SkillLoaded, "a",
			[]string{"b", "a"},
		},
		{
			"an event with no skill body is ignored",
			[]chat.Event{{Seq: 1, Kind: chat.KindSkillLoaded, At: t0}},
			false,
			chat.SkillNone, "", nil,
		},
		{"a skill with no name is ignored", []chat.Event{skill(1, "", "h")}, true, chat.SkillAvailable, "", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := chat.Reduce("cht_aaaaaaaaaa", chat.StateRunning, tc.events, tc.reviewed)

			if a.Skill.State != tc.state || a.Skill.Name != tc.current {
				t.Fatalf("skill = %+v, want %s %q", a.Skill, tc.state, tc.current)
			}

			var names []string
			for _, s := range a.SkillsLoaded {
				names = append(names, s.Name)
			}

			if strings.Join(names, ",") != strings.Join(tc.loaded, ",") {
				t.Fatalf("loaded = %v, want %v", names, tc.loaded)
			}

			if a.SkillsLoaded == nil {
				t.Fatal("skills_loaded must be an empty list, not null")
			}

			if a.SessionID != "cht_aaaaaaaaaa" || a.State != chat.StateRunning {
				t.Fatalf("identity = %q %q", a.SessionID, a.State)
			}
		})
	}
}

func TestReduceSkillKeepsHashAndTime(t *testing.T) {
	t.Parallel()

	a := chat.Reduce("s", chat.StateIdle, []chat.Event{skill(1, "a", "old"), skill(5, "a", "new")}, false)

	if len(a.SkillsLoaded) != 1 || a.SkillsLoaded[0].Hash != "new" ||
		!a.SkillsLoaded[0].At.Equal(t0.Add(5*time.Second)) {
		t.Fatalf("loaded = %+v", a.SkillsLoaded)
	}

	if !a.UpdatedAt.Equal(t0.Add(5 * time.Second)) {
		t.Fatalf("updated = %v", a.UpdatedAt)
	}
}

func TestReduceTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		events    []chat.Event
		want      *chat.ToolActivity
		updatedAt time.Time
	}{
		{"none yet", []chat.Event{message(1)}, nil, time.Time{}},
		{
			"a tool of Stuarty with an action",
			[]chat.Event{call(1, "mcp__stuarty__library", "get", chat.ToolInProgress)},
			&chat.ToolActivity{
				Title: "mcp__stuarty__library", TechnicalName: "library", Action: "get",
				DescriptionKey: "ui.agent_activity.action.library_get", Status: chat.ToolInProgress,
			},
			t0.Add(time.Second),
		},
		{
			"an action that is not listed gets the key of the tool",
			[]chat.Event{call(1, "stuarty/library", "frobnicate", chat.ToolCompleted)},
			&chat.ToolActivity{
				Title: "stuarty/library", TechnicalName: "library",
				DescriptionKey: "ui.agent_activity.tool.library", Status: chat.ToolCompleted,
			},
			t0.Add(time.Second),
		},
		{
			"a tool with no action",
			[]chat.Event{call(1, "Stuarty: project_get", "", chat.ToolPending)},
			&chat.ToolActivity{
				Title: "Stuarty: project_get", TechnicalName: "project_get",
				DescriptionKey: "ui.agent_activity.tool.project_get", Status: chat.ToolPending,
			},
			t0.Add(time.Second),
		},
		{
			"a tool outside the table is text from the agent",
			[]chat.Event{call(1, "Reading the project", "get", chat.ToolCompleted)},
			&chat.ToolActivity{Title: "Reading the project", Status: chat.ToolCompleted, FromAgent: true},
			t0.Add(time.Second),
		},
		{
			"a bare name may be the tool of another server",
			[]chat.Event{call(1, "library", "get", chat.ToolCompleted)},
			&chat.ToolActivity{Title: "library", Status: chat.ToolCompleted, FromAgent: true},
			t0.Add(time.Second),
		},
		{
			"a tool of Stuarty that does not exist is text from the agent",
			[]chat.Event{call(1, "mcp__stuarty__publish_everything", "", chat.ToolCompleted)},
			&chat.ToolActivity{Title: "mcp__stuarty__publish_everything", Status: chat.ToolCompleted, FromAgent: true},
			t0.Add(time.Second),
		},
		{
			"the last call wins",
			[]chat.Event{
				call(
					1,
					"mcp__stuarty__edit",
					"",
					chat.ToolCompleted,
				),
				message(2),
				call(3, "Bash", "", chat.ToolInProgress),
			},
			&chat.ToolActivity{Title: "Bash", Status: chat.ToolInProgress, FromAgent: true},
			t0.Add(3 * time.Second),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := chat.Reduce("s", chat.StateRunning, tc.events, false)

			switch {
			case tc.want == nil && a.Tool != nil:
				t.Fatalf("tool = %+v, want none", a.Tool)
			case tc.want != nil && (a.Tool == nil || *a.Tool != *tc.want):
				t.Fatalf("tool = %+v, want %+v", a.Tool, tc.want)
			}

			if !a.UpdatedAt.Equal(tc.updatedAt) {
				t.Fatalf("updated = %v, want %v", a.UpdatedAt, tc.updatedAt)
			}
		})
	}
}

func TestReduceDoesNotMutateItsInput(t *testing.T) {
	t.Parallel()

	events := []chat.Event{skill(1, "a", "h"), call(2, "mcp__stuarty__library", "get", chat.ToolCompleted)}
	chat.Reduce("s", chat.StateIdle, events, true)

	if events[1].Tool.Title != "mcp__stuarty__library" || events[0].Skill.Name != "a" {
		t.Fatal("the reducer changed its input")
	}
}

func TestStuartyToolName(t *testing.T) {
	t.Parallel()

	for title, want := range map[string]string{
		"mcp__stuarty__library":  "library",
		"MCP__Stuarty__Library":  "library",
		"stuarty/job_start":      "job_start",
		"stuarty:project_create": "project_create",
		"stuarty.lint (MCP)":     "lint",
		"stuarty - view":         "view",
		"  stuarty library  ":    "library",
		"mcp__other__library":    "",
		"library":                "",
		"stuarty/":               "",
		"stuarty/libraryx":       "",
		"":                       "",
	} {
		got, ok := chat.StuartyToolName(title)
		if got != want || ok != (want != "") {
			t.Errorf("StuartyToolName(%q) = %q, %v; want %q", title, got, ok, want)
		}
	}
}

func TestDescriptionKey(t *testing.T) {
	t.Parallel()

	if got := chat.DescriptionKey("history", "undo"); got != "ui.agent_activity.action.history_undo" {
		t.Fatalf("got %q", got)
	}

	if got := chat.DescriptionKey("edit", "anything"); got != "ui.agent_activity.tool.edit" {
		t.Fatalf("got %q", got)
	}
}

func TestSkillLoadedIsAKindAndRendersInMarkdown(t *testing.T) {
	t.Parallel()

	if !chat.KindSkillLoaded.Valid() {
		t.Fatal("skill_loaded is not a valid kind")
	}

	md := chat.RenderMarkdown(chat.Transcript{
		SessionID: "cht_aaaaaaaaaa", Events: []chat.Event{skill(1, "captions```\n# x", "h")},
	})

	if !strings.Contains(md, "Skill read") || !strings.Contains(md, "````\ncaptions```\n# x\n````") {
		t.Fatalf("markdown = %q", md)
	}
}
