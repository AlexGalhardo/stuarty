package servicetest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// SDD-007 T-60, SDD-020 section 17: a post goes to the account a person chose, and to no other. The choice is
// the person's alone, and a new sign-in does not keep the earlier one.

func TestSecurity_T60_PostGoesOnlyToThePageThePersonChose(t *testing.T) {
	m := newMetaEnv(t)
	person := m.browser(ui)

	conn, acc := m.connect("facebook")
	if len(acc.Choices) != 2 {
		t.Fatalf("the sign-in offered %d accounts, want 2", len(acc.Choices))
	}

	if _, err := m.selectAccount(person, conn, "P2"); err != nil {
		t.Fatal(err)
	}

	exp := m.seedReel("job_0000000971", 1080, 1920, 20)

	job, err := m.pub.PublishNow(m.as(token), service.PublishNowRequest{
		ProjectID: m.project, PublishArgs: m.reelArgs(conn, exp),
	})
	if err != nil {
		t.Fatal(err)
	}

	m.decide(job, "approve")

	if done := m.job(job.JobID); done.State != service.JobSucceeded {
		t.Fatalf("job = %+v (error %+v)", done, done.Error)
	}

	for _, call := range m.graph.seen() {
		if strings.Contains(call, "/P1") {
			t.Errorf("a request went to the Page that was not chosen: %s", call)
		}
	}

	for _, field := range []string{"page_id", "account_id", "account", "page", "target"} {
		// The post has no member that names where it goes: a stale or hostile argument cannot redirect it.
		args := `{"connection_id":"` + conn + `","export":"` + exp + `","title":"t","visibility":"public","` +
			field + `":"P1"}`

		_, jerr := m.svc.JobStart(m.as(token), service.JobStartRequest{
			ProjectID: m.project, Kind: service.JobPublishUpload, Args: json.RawMessage(args),
		})
		if !isCode(jerr, "E_VALIDATION") {
			t.Errorf("a post that names %q = %v, want E_VALIDATION", field, jerr)
		}
	}
}

func TestSecurity_T60_AnAgentCannotChooseOrChangeTheAccount(t *testing.T) {
	m := newMetaEnv(t)
	person := m.browser(ui)

	conn, _ := m.connect("facebook")
	exp := m.seedReel("job_0000000972", 1080, 1920, 20)

	// Before the choice: the local token cannot choose, and a post is refused.
	if _, err := m.selectAccount(m.as(token), conn, "P1"); !isCode(err, service.CodePrivilegedSession) {
		t.Fatalf("the local token choosing = %v, want E_PRIVILEGED_SESSION", err)
	}

	if _, err := m.pub.PublishNow(m.as(token), service.PublishNowRequest{
		ProjectID: m.project, PublishArgs: m.reelArgs(conn, exp),
	}); !isCode(err, "E_VALIDATION") {
		t.Fatalf("a post with no chosen account = %v, want E_VALIDATION", err)
	}

	// After the person chose: the agent cannot move the connection to another account.
	if _, err := m.selectAccount(person, conn, "P2"); err != nil {
		t.Fatal(err)
	}

	for _, as := range []string{token, "local:agent"} {
		if _, err := m.selectAccount(m.as(as), conn, "P1"); !isCode(err, service.CodePrivilegedSession) {
			t.Errorf("%s moving the account = %v, want E_PRIVILEGED_SESSION", as, err)
		}
	}

	listed, err := m.ops.SocialAccountList(person, service.SocialAccountListRequest{})
	if err != nil || len(listed.Accounts) != 1 || listed.Accounts[0].AccountID != "P2" {
		t.Fatalf("accounts = %+v, %v; the Page must still be the chosen one", listed, err)
	}
}

func TestSecurity_T60_ANewSignInDoesNotKeepTheEarlierChoice(t *testing.T) {
	m := newMetaEnv(t)
	person := m.browser(ui)

	conn, _ := m.connect("facebook")

	if _, err := m.selectAccount(person, conn, "P2"); err != nil {
		t.Fatal(err)
	}

	// The person signs in again (perhaps as another user of Facebook): the choice is theirs to make again.
	begun, err := m.ops.SocialConnectBegin(person, service.SocialConnectBeginRequest{
		Network: "facebook", ConnectionID: conn, RedirectURI: redirect,
	})
	if err != nil {
		t.Fatal(err)
	}

	again, err := m.ops.SocialConnectComplete(
		person, service.SocialConnectCompleteRequest{State: stateOf(m.tt, begun.AuthorizeURL), Code: "c2"},
	)
	if err != nil || again.State != service.AccountNeedsAccount || again.AccountID != "" {
		t.Fatalf("the second sign-in = %+v, %v; want needs_account with no account kept", again, err)
	}

	exp := m.seedReel("job_0000000973", 1080, 1920, 20)

	if _, perr := m.pub.PublishNow(m.as(token), service.PublishNowRequest{
		ProjectID: m.project, PublishArgs: m.reelArgs(conn, exp),
	}); !isCode(perr, "E_VALIDATION") {
		t.Errorf("a post after a new sign-in, before a new choice = %v, want E_VALIDATION", perr)
	}
}

// SDD-007 T-61, SDD-020 section 17: a token of Meta (the user token, the long-lived one, a Page token) and the
// app secret are in no answer, no log, no job and no address of a request, whether the flow works or fails.

func TestSecurity_T61_NoTokenIsInAnyAnswerLogOrJob(t *testing.T) {
	m := newMetaEnv(t)
	person := m.browser(ui)

	conn, acc := m.connect("instagram")

	chosen, err := m.selectAccount(person, conn, "IG1")
	if err != nil {
		t.Fatal(err)
	}

	exp := m.seedReel("job_0000000974", 1080, 1920, 20)

	ok, err := m.pub.PublishNow(m.as(token), service.PublishNowRequest{
		ProjectID: m.project, PublishArgs: m.reelArgs(conn, exp),
	})
	if err != nil {
		t.Fatal(err)
	}

	m.decide(ok, "approve")

	// A second post that Meta refuses: an error body and a failed job, the paths that log the most.
	m.graph.mu.Lock()
	m.graph.failMedia = 190
	m.graph.mu.Unlock()

	bad, err := m.pub.PublishNow(m.as(token), service.PublishNowRequest{
		ProjectID: m.project, PublishArgs: m.reelArgs(conn, exp), TimelineID: "",
	})
	if err != nil {
		t.Fatal(err)
	}

	m.decide(bad, "approve")

	results := map[string]any{"complete": acc, "select": chosen}
	add := func(name string, v any, err error) {
		t.Helper()

		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		results[name] = v
	}

	networks, err := m.ops.SocialNetworkList(person, service.SocialNetworkListRequest{})
	add("SocialNetworkList", networks, err)

	accounts, err := m.ops.SocialAccountList(person, service.SocialAccountListRequest{})
	add("SocialAccountList", accounts, err)

	checked, err := m.pub.PublishValidate(person, service.PublishValidateRequest{
		ProjectID: m.project, PublishArgs: m.reelArgs(conn, exp),
	})
	add("PublishValidate", checked, err)

	for name, job := range map[string]*service.Job{"ok": ok, "bad": bad} {
		add("JobGet "+name, m.job(job.JobID), nil)

		st, serr := m.pub.PublishStatus(person, service.PublishStatusRequest{JobID: job.JobID})
		add("PublishStatus "+name, st, serr)

		req, aerr := m.svc.ApprovalGet(person, service.ApprovalRef{ID: job.ApprovalID})
		add("ApprovalGet "+name, req, aerr)
	}

	got, err := m.svc.ConnectionGet(person, service.ConnectionRef{ID: conn})
	add("ConnectionGet", got, err)

	list, err := m.svc.ConnectionList(person, service.ConnectionListRequest{})
	add("ConnectionList", list, err)

	tested, err := m.svc.ConnectionTest(person, service.ConnectionRef{ID: conn})
	add("ConnectionTest", tested, err)

	if failed := m.job(bad.JobID); failed.State != service.JobFailed || failed.Error == nil {
		t.Fatalf("the refused post = %+v; the test needs a failed job to look at", failed)
	}

	for name, v := range results {
		secretsIn(t, name, asJSON(v))
	}

	secretsIn(t, "the logs", m.logs.String())

	// A token is in a header or a form body, never in an address.
	m.graph.mu.Lock()
	defer m.graph.mu.Unlock()

	for _, u := range m.graph.urls {
		secretsIn(t, "the address "+u, u)
	}
}

func TestSecurity_T61_ARefusedSignInLeavesNoTokenBehind(t *testing.T) {
	m := newMetaEnv(t)
	person := m.browser(ui)

	begun, err := m.ops.SocialConnectBegin(person, service.SocialConnectBeginRequest{
		Network: "instagram", ClientID: "meta-app-id", ClientSecret: metaAppSecret, RedirectURI: redirect,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The person said no on the consent screen: Meta sends an error and no code.
	_, cerr := m.ops.SocialConnectComplete(
		person, service.SocialConnectCompleteRequest{State: stateOf(m.tt, begun.AuthorizeURL), Error: "access_denied"},
	)
	if !isCode(cerr, "E_VALIDATION") {
		t.Fatalf("a refused consent = %v, want E_VALIDATION", cerr)
	}

	if m.graph.count() != 0 {
		t.Errorf("a refused sign-in made requests: %v", m.graph.seen())
	}

	list, err := m.ops.SocialAccountList(person, service.SocialAccountListRequest{})
	if err != nil || len(list.Accounts) != 1 || list.Accounts[0].State != service.AccountNeedsSignIn ||
		len(list.Accounts[0].Choices) != 0 {
		t.Fatalf("accounts = %+v, %v", list, err)
	}

	secretsIn(t, "the account list", asJSON(list))
	secretsIn(t, "the logs", m.logs.String())
}
