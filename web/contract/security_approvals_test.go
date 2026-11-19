package contract

import (
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// the four operations that need a person (SDD-015 section 4) with arguments an agent would send.
var personOnly = []step{
	{Call: "ApprovalDecide", Args: json.RawMessage(`{"id":"apr_0000000001","decision":"approve"}`)},
	{Call: "ApprovalDecide", Args: json.RawMessage(`{"id":"apr_0000000001","decision":"reject","reason":"no"}`)},
	{Call: "ApprovalRuleSet", Args: json.RawMessage(`{"kind":"publish.post"}`)},
	{
		Call: "ApprovalRuleSet",
		Args: json.RawMessage(`{"kind":"generate.image","currency":"USD","per_item_micros":9000000000000}`),
	},
	{Call: "ApprovalRuleRemove", Args: json.RawMessage(`{"id":"rul_0000000001"}`)},
	{Call: "CostCeilingSet", Args: json.RawMessage(`{"currency":"USD","per_day_micros":999999999999}`)},
}

func privilegedRefusals(t *testing.T, newDriver func(*testing.T, testenv.Options) driver, calls []step) {
	t.Helper()

	d := newDriver(t, testenv.Options{})

	for _, s := range calls {
		result, _, fail := d.do(t, s)

		if fail == nil || fail.Code != service.CodePrivilegedSession {
			t.Errorf("%s %s with the local token: result %s, error %+v; want E_PRIVILEGED_SESSION",
				d.name(), s.Call, result, fail)

			continue
		}

		if fail.Hint == "" {
			t.Errorf("%s %s: the refusal has no hint", d.name(), s.Call)
		}
	}

	// Nothing a refused call did stuck: no rule, no ceiling.
	for _, check := range []step{{Call: "ApprovalRuleList", Args: json.RawMessage(`{}`)}, {Call: "CostSummary", Args: json.RawMessage(`{}`)}} {
		result, _, fail := d.do(t, check)
		if fail != nil {
			t.Fatalf("%s %s: %+v", d.name(), check.Call, fail)
		}

		if strings.Contains(string(result), "publish.post") || strings.Contains(string(result), "999999999999") {
			t.Errorf("%s %s shows what a refused call wrote: %s", d.name(), check.Call, result)
		}
	}
}

// SDD-007 T-48: an agent cannot approve or reject a request, on any surface: the local token that an
// agent holds is refused by the service with one stable code, over HTTP, in the CLI and in process,
// and the MCP server has no tool that decides.
func TestSecurity_T48_AgentCannotApproveItsOwnRequest(t *testing.T) {
	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "cli": newCLIWith,
	} {
		t.Run(name, func(t *testing.T) { privilegedRefusals(t, newDriver, personOnly[:2]) })
	}

	t.Run("mcp", func(t *testing.T) { noToolDecides(t, "approve") })
}

// SDD-007 T-49: an agent cannot create, change or remove a rule, nor set a ceiling, on any surface.
func TestSecurity_T49_AgentCannotWriteARule(t *testing.T) {
	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "cli": newCLIWith,
	} {
		t.Run(name, func(t *testing.T) { privilegedRefusals(t, newDriver, personOnly[2:]) })
	}

	t.Run("mcp", func(t *testing.T) { noToolDecides(t, "rule") })
}

// noToolDecides checks that no tool of the MCP server decides, writes a rule or sets a ceiling: none
// of the person-only operations has a tool, the one tool of the queue reads only, and a member that
// would decide is refused by its input schema.
func noToolDecides(t *testing.T, member string) {
	t.Helper()

	d := newMCP(t).(*mcpDriver)

	for _, s := range personOnly {
		if tool, _, err := toolCall(s); err == nil {
			t.Errorf("%s is reachable through the tool %s", s.Call, tool)
		}
	}

	list, err := d.cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, tool := range list.Tools {
		text := strings.ToLower(tool.Name + " " + tool.Description)

		if strings.Contains(tool.Name, "decide") || strings.Contains(tool.Name, "rule") ||
			strings.Contains(tool.Name, "ceiling") {
			t.Errorf("the tool %s sounds like a decision", tool.Name)
		}

		if tool.Name == "approval" {
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
				t.Error("the approval tool is not read only")
			}

			if !strings.Contains(text, "cannot approve") {
				t.Errorf("the approval tool does not tell the agent it cannot decide: %s", tool.Description)
			}
		}
	}

	// An agent that guesses a tool is refused by the protocol, with a stable error, and nothing changes.
	for _, guess := range []struct {
		name string
		args map[string]any
	}{
		{"approval", map[string]any{"action": "decide", "id": "apr_0000000001", "decision": "approve"}},
		{"approval_decide", map[string]any{"id": "apr_0000000001", "decision": "approve"}},
		{"approval_rule_set", map[string]any{"kind": "publish.post"}},
		{"cost_ceiling_set", map[string]any{"currency": "USD", "per_day_micros": 999999999999}},
	} {
		res, err := d.cs.CallTool(t.Context(), &sdk.CallToolParams{Name: guess.name, Arguments: guess.args})
		if err == nil && (res == nil || !res.IsError) {
			t.Errorf("the guessed tool %s answered %+v, want a refusal", guess.name, res)
		}
	}

	for _, check := range []step{
		{Call: "ApprovalRuleList", Args: json.RawMessage(`{}`)},
		{Call: "CostSummary", Args: json.RawMessage(`{}`)},
	} {
		reader := service.WithCaller(t.Context(), service.Caller{Actor: "local:check", Scopes: service.AllScopes})

		result, err := callService(reader, d.service(), check)
		if err != nil {
			t.Fatal(err)
		}

		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(string(raw), "publish.post") || strings.Contains(string(raw), "999999999999") {
			t.Errorf("%s shows what a guessed tool wrote: %s", check.Call, raw)
		}
	}
}
