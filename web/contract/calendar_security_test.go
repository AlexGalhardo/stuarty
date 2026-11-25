package contract

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
)

func personCtx() service.Caller {
	return service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true}
}

// pendingRequests is how many approval requests wait for a person.
func pendingRequests(t *testing.T, fx publishFixture) int {
	t.Helper()

	res, err := fx.d.service().ApprovalList(callerCtx(), service.ApprovalListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	return len(res.Requests)
}

// T-180: a quiet day is a rule, not a hint. An agent, on any surface and by any path that asks for approval, is
// refused on a day a person marked quiet, whatever it passes; nothing is asked and nothing leaves the machine; a
// person is asked to confirm, and only the confirmed call schedules.
func TestSecurity_T180_AnAgentCannotScheduleOnAQuietDayByAnyPathOrSurface(t *testing.T) {
	for name, newDriver := range surfaces {
		t.Run(name, func(t *testing.T) {
			fx := newPublishFixture(t, newDriver)
			d, svc := fx.d, fx.d.service()
			person := service.WithCaller(callerCtx(), personCtx())
			kids := false

			// The day of the post and the day of the clock (2026-11-14) are quiet.
			for _, date := range []string{"2026-12-25", "2026-11-14"} {
				if _, err := svc.CalendarQuietSet(person, service.CalendarQuietSetRequest{
					Date: date, Zone: "UTC", Quiet: true,
				}); err != nil {
					t.Fatal(err)
				}
			}

			made, err := svc.PublicationCreate(person, service.PublicationCreateRequest{
				ProjectID: fx.project, Title: "Christmas", Export: fx.export, Targets: []pub.TargetInput{
					{
						ConnectionID: fx.connection,
						At:           "2026-12-25T10:00",
						Zone:         "UTC",
						Fields:       pub.Fields{MadeForKids: &kids},
					},
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			target := made.Targets[0].ID

			refuse := func(label string, s step) {
				t.Helper()

				_, _, fail := d.do(t, s)
				if fail == nil || fail.Code != service.CodePrivilegedSession {
					t.Errorf("%s over %s answered %+v, want E_PRIVILEGED_SESSION", label, name, fail)
				}
			}

			// An agent that also passes the confirmation of a person is refused all the same; the MCP tool has no
			// member for it and refuses the member itself.
			refuse("schedule", mkStep(t, "PublicationSchedule", service.PublicationScheduleRequest{
				ID: made.ID, TargetID: target,
			}))

			if name != "mcp" {
				refuse(
					"schedule with the confirmation",
					mkStep(t, "PublicationSchedule", service.PublicationScheduleRequest{
						ID: made.ID, TargetID: target, ConfirmQuietDay: true,
					}),
				)
			} else if _, _, fail := d.do(t, mkStep(t, "PublicationSchedule", service.PublicationScheduleRequest{
				ID: made.ID, TargetID: target, ConfirmQuietDay: true,
			})); fail == nil || fail.Code != "E_VALIDATION" {
				t.Errorf("the tool accepted the confirmation member: %+v", fail)
			}

			if name != "mcp" {
				refuse("send now", mkStep(t, "PublicationSendNow", service.PublicationSendNowRequest{
					ID: made.ID, TargetID: target, ConfirmQuietDay: true,
				}))
			}

			// The job that goes at the moment of the approval is refused on the day of the clock.
			refuse("publish now", mkStep(t, "PublishNow", service.PublishNowRequest{
				ProjectID:    fx.project,
				ConnectionID: fx.connection, Export: fx.export, Title: "Today", MadeForKids: &kids,
			}))

			got, err := svc.PublicationGet(person, service.PublicationGetRequest{ID: made.ID})
			if err != nil || got.Targets[0].State != "draft" || got.Targets[0].ApprovalID != "" {
				t.Fatalf("a refused call changed the target: %+v, %v", got, err)
			}

			if n := pendingRequests(t, fx); n != 0 {
				t.Errorf("%d approval requests after the refusals, want none", n)
			}

			if n := fx.uploads.Load(); n != 0 {
				t.Errorf("%d requests left the machine", n)
			}

			// A person is asked to confirm, and the confirmed call is the one that schedules.
			_, err = svc.PublicationSchedule(person, service.PublicationScheduleRequest{ID: made.ID, TargetID: target})

			var se *service.Error
			if !errors.As(err, &se) || se.Code != "E_INVALID_STATE" || se.Path != "confirm_quiet_day" {
				t.Fatalf("a person without the confirmation got %v, want E_INVALID_STATE at confirm_quiet_day", err)
			}

			if n := pendingRequests(t, fx); n != 0 {
				t.Fatalf("%d requests before the person confirmed", n)
			}

			done, err := svc.PublicationSchedule(person, service.PublicationScheduleRequest{
				ID: made.ID, TargetID: target, ConfirmQuietDay: true,
			})
			if err != nil || done.Targets[0].State != "waiting_approval" || pendingRequests(t, fx) != 1 {
				t.Fatalf("the confirmed call = %+v, %v", done, err)
			}

			// An agent cannot mark the day it wants, or clear the one a person marked.
			for _, quiet := range []bool{true, false} {
				_, err := svc.CalendarQuietSet(callerCtx(), service.CalendarQuietSetRequest{
					Date: "2026-12-25", Zone: "UTC", Quiet: quiet,
				})
				if !errors.As(err, &se) || se.Code != service.CodePrivilegedSession {
					t.Errorf("an agent set quiet=%v and got %v", quiet, err)
				}
			}

			// Nor move the day by writing its note in another zone, which would free hours of it.
			moved := "moved"
			refuse("the zone of the quiet day", mkStep(t, "CalendarDaySet", service.CalendarDaySetRequest{
				Date: "2026-12-25", Zone: "Pacific/Kiritimati", Note: &moved,
			}))

			plan, err := svc.CalendarPlan(
				callerCtx(),
				service.CalendarPlanRequest{From: "2026-12-25", To: "2026-12-26"},
			)
			if err != nil || len(plan.Days) != 1 || !plan.Days[0].Quiet || plan.Days[0].Zone != "UTC" ||
				plan.Days[0].Note != "" {
				t.Errorf("the day is not as the person left it: %+v, %v", plan, err)
			}
		})
	}
}

// T-181: a slot is a reminder. Saving one, reading a plan that holds it and expanding it for any range ask for no
// approval, plan no publication, open no connection and keep nothing but a weekday, a time, a zone and a network.
func TestSecurity_T181_ASlotNeverSendsPlansOrAsksForApproval(t *testing.T) {
	fx := newPublishFixture(t, newMCPWith)
	d, svc := fx.d, fx.d.service()

	raw, _, fail := d.do(t, mkStep(t, "CalendarSlotSave", service.CalendarSlotSaveRequest{
		Network: "fake", Weekday: "mon", Time: "09:00", Zone: "UTC", ConnectionID: fx.connection,
	}))
	if fail != nil {
		t.Fatalf("%+v", fail)
	}

	if strings.Contains(string(raw), "token") || strings.Contains(string(raw), "secret") {
		t.Errorf("the slot carries a credential: %s", raw)
	}

	for _, rng := range [][2]string{{"2026-11-16", "2026-11-17"}, {"2026-11-01", "2027-11-01"}, {"2030-01-01", "2030-02-01"}} {
		plan, _, fail := d.do(
			t,
			mkStep(t, "CalendarPlan", service.CalendarPlanRequest{From: rng[0], To: rng[1], Publications: true}),
		)
		if fail != nil {
			t.Fatalf("%+v", fail)
		}

		var res service.CalendarPlanResult
		if err := json.Unmarshal(
			plan,
			&res,
		); err != nil || len(res.Publications) != 0 || res.Total != 0 ||
			len(res.Slots) == 0 {
			t.Fatalf("the plan of %v = %s, %v", rng, plan, err)
		}

		for _, s := range res.Slots {
			if s.Filled != nil {
				t.Errorf("a slot with nothing planned is filled: %+v", s)
			}
		}
	}

	list, err := svc.PublicationList(callerCtx(), service.PublicationListRequest{})
	if err != nil || list.Total != 0 {
		t.Errorf("a slot planned a publication: %+v, %v", list, err)
	}

	if n := pendingRequests(t, fx); n != 0 {
		t.Errorf("a slot asked for approval: %d requests", n)
	}

	if n := fx.uploads.Load(); n != 0 {
		t.Errorf("a slot opened a connection: %d requests left the machine", n)
	}
}

// T-182: the .ics file holds the instant, the network, the state and the words of the person, escaped, and no
// credential, no token, and no id that grants access: not the connection, the publication, the target, the approval
// or the file.
func TestSecurity_T182_TheICSExportHoldsNoCredentialOrAccessId(t *testing.T) {
	fx := newPublishFixture(t, newDirectWith)
	svc := fx.d.service()
	approved := approvedByAPerson(t, fx)

	person := service.WithCaller(callerCtx(), personCtx())
	title := "Title; with, punctuation \\ and ATTACH:http://evil.invalid/x"
	hostile := "line\nEND:VEVENT\nBEGIN:VEVENT\nATTACH:http://evil.invalid/x\nX-TOKEN:vvl_abc"

	if _, err := svc.PublicationUpdate(person, service.PublicationUpdateRequest{
		ID: approved.ID, Title: &title, Text: &hostile,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.CalendarExport(callerCtx(), service.CalendarExportRequest{From: "2026-12-01", To: "2026-12-02"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.PublicationGet(callerCtx(), service.PublicationGetRequest{ID: approved.ID})
	if err != nil {
		t.Fatal(err)
	}

	forbidden := []string{
		fx.connection, got.ID, got.Targets[0].ID, got.Targets[0].ApprovalID, fx.export, fx.project,
		"vvl_c0ntr4ct", "client_secret", "access_token", "refresh_token", "Bearer", "secret",
	}

	for _, f := range forbidden {
		if f != "" && strings.Contains(res.Body, f) {
			t.Errorf("the file holds %q:\n%s", f, res.Body)
		}
	}

	if n := strings.Count("\r\n"+res.Body, "\r\nBEGIN:VEVENT\r\n"); n != 1 {
		t.Errorf("%d events, the title started another:\n%s", n, res.Body)
	}

	// The words of the person are in the file as values, never as a line of their own.
	for line := range strings.SplitSeq(res.Body, "\r\n") {
		if strings.HasPrefix(line, "ATTACH") || strings.HasPrefix(line, "X-TOKEN") ||
			strings.HasPrefix(line, "END:VEVENT\\n") {
			t.Errorf("a line made by the words of the person: %q", line)
		}
	}

	// The edit of a person returned the post to waiting for approval, so the event is tentative.
	if !strings.Contains(res.Body, "DTSTART:20261201T090000Z") || !strings.Contains(res.Body, "STATUS:TENTATIVE") {
		t.Errorf("the event:\n%s", res.Body)
	}

	// Reading is all a key with read can do, and the file is the same for it.
	if _, err := svc.CalendarExport(service.WithCaller(callerCtx(), service.Caller{
		Actor: "key:reader", Scopes: []string{service.ScopeRead},
	}), service.CalendarExportRequest{From: "2026-12-01", To: "2026-12-02"}); err != nil {
		t.Errorf("a read key exports: %v", err)
	}
}

// T-183: a copy carries no yes. Duplicating what a person approved makes a draft with no request; the original is as
// the person left it; scheduling the copy asks again, with a request of its own, and nothing is sent.
func TestSecurity_T183_ADuplicateCarriesNoApprovalAndLeavesTheOriginal(t *testing.T) {
	fx := newPublishFixture(t, newDirectWith)
	svc := fx.d.service()
	approved := approvedByAPerson(t, fx)

	cp, err := svc.PublicationDuplicate(callerCtx(), service.PublicationDuplicateRequest{
		ID: approved.ID, At: "2026-12-08T09:00", Zone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}

	tg := cp.Targets[0]
	if cp.ID == approved.ID || tg.State != "draft" || tg.ApprovalID != "" || tg.Error != nil || tg.Attempts != 0 {
		t.Fatalf("the copy = %+v", cp)
	}

	stillApproved(t, fx, approved)

	if n := pendingRequests(t, fx); n != 0 {
		t.Errorf("a copy made %d approval requests", n)
	}

	scheduled, err := svc.PublicationSchedule(callerCtx(), service.PublicationScheduleRequest{ID: cp.ID})
	if err != nil || scheduled.Targets[0].State != "waiting_approval" ||
		scheduled.Targets[0].ApprovalID == approved.Targets[0].ApprovalID || scheduled.Targets[0].ApprovalID == "" {
		t.Fatalf("scheduling the copy = %+v, %v", scheduled, err)
	}

	if n := pendingRequests(t, fx); n != 1 {
		t.Errorf("%d requests wait, want the copy's own", n)
	}

	stillApproved(t, fx, approved)

	if n := fx.uploads.Load(); n != 0 {
		t.Errorf("%d requests left the machine", n)
	}
}
