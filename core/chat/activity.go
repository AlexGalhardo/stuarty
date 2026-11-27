package chat

import (
	"slices"
	"strings"
	"time"
)

// SkillRef is the body of a skill_loaded event: which skill a session read and the content hash of
// the text it read (SDD-017 section 8.1). Both come from the library, not from the agent.
type SkillRef struct {
	Name string `json:"name"`
	Hash string `json:"hash,omitempty"`
}

// SkillState says whether a session has a skill: observed in this session, merely present in the
// library, or neither.
type SkillState string

// The states of a skill. "Loaded" means read in this session, never followed: Stuarty cannot prove
// what an agent does with a text it read.
const (
	SkillLoaded    SkillState = "loaded"
	SkillAvailable SkillState = "available"
	SkillNone      SkillState = "none"
)

// SkillSlot is the current skill of a session.
type SkillSlot struct {
	// Name is the last skill read; empty unless State is loaded.
	Name  string     `json:"name"`
	State SkillState `json:"state"`
}

// LoadedSkill is a skill a session read, with the time of its last read.
type LoadedSkill struct {
	Name string    `json:"name"`
	Hash string    `json:"hash,omitempty"`
	At   time.Time `json:"at"`
}

// ToolActivity is the last tool call of a session. Title is text from the agent. For a tool of
// Stuarty's own MCP server TechnicalName and DescriptionKey are set and FromAgent is false; for
// any other call they are empty and FromAgent is true.
type ToolActivity struct {
	Title          string     `json:"title"`
	TechnicalName  string     `json:"technical_name,omitempty"`
	Action         string     `json:"action,omitempty"`
	DescriptionKey string     `json:"description_key,omitempty"`
	Status         ToolStatus `json:"status"`
	FromAgent      bool       `json:"from_agent"`
}

// Activity is what a session is doing, told from its events and nothing else (SDD-017 section 8.1).
type Activity struct {
	SessionID string    `json:"session_id"`
	State     State     `json:"state"`
	Skill     SkillSlot `json:"skill"`
	// SkillsLoaded is every skill read, in the order of its last read: the last is the current one.
	SkillsLoaded []LoadedSkill `json:"skills_loaded"`
	Tool         *ToolActivity `json:"tool"`
	// UpdatedAt is the time of the last event that changed the skill or the tool; zero when none did.
	UpdatedAt time.Time `json:"updated_at"`
}

// HasLoaded reports whether the session read the skill of that name (SDD-018 section 7, the "Loaded" mark of
// the skills list).
func (a *Activity) HasLoaded(name string) bool {
	return slices.ContainsFunc(a.SkillsLoaded, func(s LoadedSkill) bool { return s.Name == name })
}

// Reduce is the one function that tells what a session is doing. reviewedSkills says whether the
// library holds a skill that a person reviewed, which is what separates "available" from "none".
func Reduce(sessionID string, state State, events []Event, reviewedSkills bool) Activity {
	a := Activity{SessionID: sessionID, State: state, SkillsLoaded: []LoadedSkill{}}

	for i := range events {
		e := &events[i]

		switch {
		case e.Kind == KindSkillLoaded && e.Skill != nil && e.Skill.Name != "":
			// A skill read again moves to the end: the last one read is the current one.
			a.SkillsLoaded = slices.DeleteFunc(
				a.SkillsLoaded,
				func(s LoadedSkill) bool { return s.Name == e.Skill.Name },
			)
			a.SkillsLoaded = append(a.SkillsLoaded, LoadedSkill{Name: e.Skill.Name, Hash: e.Skill.Hash, At: e.At})
			a.UpdatedAt = e.At
		case e.Kind == KindToolCall && e.Tool != nil:
			a.Tool = describeCall(e.Tool)
			a.UpdatedAt = e.At
		}
	}

	switch {
	case len(a.SkillsLoaded) > 0:
		a.Skill = SkillSlot{Name: a.SkillsLoaded[len(a.SkillsLoaded)-1].Name, State: SkillLoaded}
	case reviewedSkills:
		a.Skill = SkillSlot{State: SkillAvailable}
	default:
		a.Skill = SkillSlot{State: SkillNone}
	}

	return a
}

func describeCall(t *Tool) *ToolActivity {
	out := &ToolActivity{Title: t.Title, Status: t.Status}

	name, ok := StuartyToolName(t.Title)
	if !ok {
		out.FromAgent = true

		return out
	}

	out.TechnicalName = name
	out.Action = ToolAction(name, t.Action)
	out.DescriptionKey = DescriptionKey(name, out.Action)

	return out
}

// toolPrefixes are the forms in which an agent names a tool of an MCP server called stuarty in the
// title of a call; each is followed by the tool name.
var toolPrefixes = []string{"mcp__stuarty__", "stuarty__", "stuarty/", "stuarty:", "stuarty.", "stuarty-", "stuarty "}

// StuartyToolName reads the name of a tool of Stuarty's own MCP server from the title an agent gave
// a call. The title is text from the agent, so only a name in the catalogue is believed, and only
// when the title names the server: a bare "library" may be the tool of another server.
func StuartyToolName(title string) (string, bool) {
	t := strings.ToLower(strings.TrimSpace(title))

	for _, p := range toolPrefixes {
		rest, found := strings.CutPrefix(t, p)
		if !found {
			continue
		}

		rest = strings.TrimLeft(rest, " :/-.")

		end := 0
		for end < len(rest) && (rest[end] == '_' || rest[end] >= 'a' && rest[end] <= 'z' || rest[end] >= '0' && rest[end] <= '9') {
			end++
		}

		if _, known := toolCatalogue[rest[:end]]; known {
			return rest[:end], true
		}
	}

	return "", false
}

// ToolAction returns action when the catalogue lists it for the tool, else the empty string.
func ToolAction(tool, action string) string {
	if slices.Contains(toolCatalogue[tool], action) {
		return action
	}

	return ""
}

// Message keys of the plain descriptions (ui.agent_activity in the language files).
const (
	toolKeyPrefix   = "ui.agent_activity.tool."
	actionKeyPrefix = "ui.agent_activity.action."
)

// DescriptionKey is the message key of the plain description of a tool of Stuarty, or of one of
// its actions when action is listed for it.
func DescriptionKey(tool, action string) string {
	if ToolAction(tool, action) != "" {
		return actionKeyPrefix + tool + "_" + action
	}

	return toolKeyPrefix + tool
}

// StuartyTools returns a copy of the catalogue: each tool with its actions, for the tests.
func StuartyTools() map[string][]string {
	out := make(map[string][]string, len(toolCatalogue))
	for k, v := range toolCatalogue {
		out[k] = slices.Clone(v)
	}

	return out
}

// toolCatalogue is every tool of Stuarty's MCP server (web/app/mcp/definitions) with the values of
// its action member. A test of web/app/mcp fails when a tool or an action is missing here.
var toolCatalogue = map[string][]string{
	"project_list":   nil,
	"project_create": nil,
	"project_get":    nil,
	"project_trash":  nil,
	"edit":           nil,
	"command_help":   nil,
	"history": {
		"list", "activity", "undo", "redo", "revert", "diff", "checkpoints", "checkpoint", "rename_checkpoint",
		"remove_checkpoint",
	},
	"media_import":  nil,
	"media_inspect": nil,
	"view":          nil,
	"lint":          nil,
	"job_start":     nil,
	"job_get":       nil,
	"job_cancel":    nil,
	"template":      {"list", "save", "new", "export", "import"},
	"font_import":   nil,
	"connection":    nil,
	"image":         {"create", "list", "show", "edit", "undo", "redo", "export", "import"},
	"project_item": {
		"add", "move", "rename", "trash", "restore", "brief", "briefing", "text", "revisions", "revert", "check",
		"update", "promote", "skip", "unskip",
	},
	"library":     {"search", "list", "get", "write", "render", "lint"},
	"publication": {"create", "schedule", "show", "list", "duplicate", "plan", "day", "slot"},
}
