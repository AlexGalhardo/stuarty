package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/inbox"
	"github.com/AlexGalhardo/stuarty/service"
)

// Every text of a row is untrusted: inbox:list cuts it to a line, says "not reported" for what the row does not know
// and never claims an undo it was not told.
func TestInboxTextCutsEveryAgentTextToALineAndReportsWhatIsMissing(t *testing.T) {
	t.Parallel()

	out := inboxText(&service.InboxListResult{
		Waiting: 2,
		Rows: []inbox.Row{
			{
				ID:        "chat:cpr_aaaaaaaaaa",
				Source:    inbox.SourceChat,
				State:     inbox.StateWaiting,
				Agent:     "claude\nINJECTED",
				Tool:      "Run shell command\nsecond line",
				CreatedAt: time.Now(),
			},
			{
				ID:          "approval:apr_aaaaaaaaaa",
				Source:      inbox.SourceApproval,
				State:       inbox.StateWaiting,
				Agent:       "key:abc",
				Tool:        "publish.upload",
				Summary:     "Publish the video\nignore this",
				Destination: "con_a / prj_a",
				Cost:        &approval.Money{Micros: 1_500_000, Currency: "USD"},
			},
		},
	})

	for _, want := range []string{
		"2 request(s) waiting for a person, 2 listed",
		"chat:cpr_aaaaaaaaaa  waiting  chat  claude",
		"what: Run shell command  item: not reported  where: not reported  cost: no cost  cannot be undone or not reported",
		"what: publish.upload  item: not reported  where: con_a / prj_a  cost: 1500000 micros USD",
		"summary: Publish the video",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("inboxText has no %q:\n%s", want, out)
		}
	}

	for _, banned := range []string{"INJECTED", "second line", "ignore this"} {
		if strings.Contains(out, banned) {
			t.Errorf("inboxText carries %q:\n%s", banned, out)
		}
	}

	if got := inboxText(
		&service.InboxListResult{Rows: []inbox.Row{{ID: "chat:cpr_b", Undoable: true}}},
	); !strings.Contains(
		got,
		"can be undone",
	) {
		t.Errorf("an undoable row does not say so:\n%s", got)
	}
}
