package contract

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// calendarCommand maps an operation of the day of the calendar to the command that runs it (SDD-020 section 30A).
func calendarCommand(s step) (name string, argv []string, stdin string, err error) {
	switch s.Call {
	case "PublicationDuplicate":
		var r service.PublicationDuplicateRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		return "publication:duplicate",
			slices.Concat([]string{r.ID}, optFlag("target", r.TargetID), optFlag("at", r.At), optFlag("zone", r.Zone)),
			"", nil
	case "CalendarPlan":
		var r service.CalendarPlanRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		argv = slices.Concat(optFlag("from", r.From), optFlag("until", r.To), optFlag("zone", r.Zone),
			optFlag("project", r.ProjectID))
		if r.Publications {
			argv = append(argv, "--publications")
		}

		if r.Limit > 0 {
			argv = append(argv, "--limit", strconv.Itoa(r.Limit))
		}

		return "calendar:plan", argv, "", nil
	case "CalendarDaySet":
		var r service.CalendarDaySetRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		argv = slices.Concat([]string{r.Date}, optFlag("zone", r.Zone))
		if r.Note != nil {
			argv = append(argv, "--note", *r.Note)
		}

		if r.Theme != nil {
			argv = append(argv, "--theme", *r.Theme)
		}

		return "calendar:day", argv, "", nil
	case "CalendarQuietSet":
		var r service.CalendarQuietSetRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		argv = slices.Concat([]string{r.Date}, optFlag("zone", r.Zone))
		if !r.Quiet {
			argv = append(argv, "--off")
		}

		return "calendar:quiet", argv, "", nil
	case "CalendarSlotSave":
		var r service.CalendarSlotSaveRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		return "calendar:slot", slices.Concat(optFlag("id", r.ID), optFlag("network", r.Network),
			optFlag("weekday", r.Weekday), optFlag("time", r.Time), optFlag("zone", r.Zone),
			optFlag("connection", r.ConnectionID)), "", nil
	case "CalendarSlotDelete":
		var r service.CalendarSlotDeleteRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		return "calendar:slot-remove", []string{r.ID}, "", nil
	case "CalendarExport":
		var r service.CalendarExportRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		return "calendar:export", slices.Concat(
			optFlag("from", r.From),
			optFlag("until", r.To),
			optFlag("zone", r.Zone),
			optFlag("project", r.ProjectID),
		), "", nil
	}

	return "", nil, "", fmt.Errorf("no CLI mapping for %s", s.Call)
}

func mkStep(t *testing.T, call string, v any) step {
	t.Helper()

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	return step{Call: call, Args: raw}
}

var surfaces = map[string]func(*testing.T, testenv.Options) driver{
	"in-process": newDirectWith, "http": newHTTPWith, "cli": newCLIWith, "mcp": newMCPWith,
}

// The day of the calendar answers the same on the Service, HTTP, the CLI and MCP (epic 97): the same note, slot,
// plan, copy and removal, byte for byte. The export and the quiet mark have no tool, so MCP skips those two.
func TestCalendarDayOperationsAreTheSameOnEverySurface(t *testing.T) {
	got := map[string]map[string]string{}

	for name, newDriver := range surfaces {
		t.Run(name, func(t *testing.T) {
			fx := newPublishFixture(t, newDriver)
			d := fx.d
			results := map[string]string{}

			run := func(label string, s step) json.RawMessage {
				raw, _, fail := d.do(t, s)
				if fail != nil {
					t.Fatalf("%s %s: %+v", d.name(), label, fail)
				}

				results[label] = normalize(t, d, raw)

				return raw
			}

			kids := false
			note, theme := "long video day", "amber"

			made := run("create", mkStep(t, "PublicationCreate", service.PublicationCreateRequest{
				ProjectID: fx.project, Title: "Week one", Text: "A caption, with; punctuation", Export: fx.export,
				Targets: []pub.TargetInput{{
					ConnectionID: fx.connection, At: "2026-12-01T09:00", Zone: "America/Sao_Paulo",
					Fields: pub.Fields{MadeForKids: &kids},
				}},
			}))

			var p service.Publication
			if err := json.Unmarshal(made, &p); err != nil || len(p.Targets) != 1 {
				t.Fatalf("%s: %v in %s", d.name(), err, made)
			}

			run("day", mkStep(t, "CalendarDaySet", service.CalendarDaySetRequest{
				Date: "2026-12-01", Zone: "America/Sao_Paulo", Note: &note, Theme: &theme,
			}))

			slot := run("slot", mkStep(t, "CalendarSlotSave", service.CalendarSlotSaveRequest{
				Network: "fake", Weekday: "tue", Time: "09:00", Zone: "America/Sao_Paulo",
			}))

			var s service.CalendarSlot
			if err := json.Unmarshal(slot, &s); err != nil || s.ID == "" {
				t.Fatalf("%s: %v in %s", d.name(), err, slot)
			}

			plan := run("plan", mkStep(t, "CalendarPlan", service.CalendarPlanRequest{
				From: "2026-12-01", To: "2026-12-02", Zone: "America/Sao_Paulo", Publications: true,
			}))

			var pl service.CalendarPlanResult
			if err := json.Unmarshal(plan, &pl); err != nil || len(pl.Days) != 1 || pl.Days[0].Note != note ||
				len(pl.Slots) != 1 || pl.Slots[0].Filled == nil || len(pl.Publications) != 1 {
				t.Fatalf("%s: the plan of the day = %s, %v", d.name(), plan, err)
			}

			run("duplicate", mkStep(t, "PublicationDuplicate", service.PublicationDuplicateRequest{
				ID: p.ID, At: "2026-12-08T09:00", Zone: "America/Sao_Paulo",
			}))
			run("original", mkStep(t, "PublicationGet", service.PublicationGetRequest{ID: p.ID}))

			if d.name() != "mcp" {
				export := run("export", mkStep(t, "CalendarExport", service.CalendarExportRequest{
					From: "2026-12-01", To: "2026-12-31", Zone: "UTC",
				}))

				var res service.CalendarExportResult
				if err := json.Unmarshal(export, &res); err != nil || res.ContentType != service.ICSContentType ||
					!strings.HasPrefix(
						res.Filename,
						"stuarty-calendar-",
					) || strings.Count(res.Body, "BEGIN:VEVENT") != 2 {
					t.Fatalf("%s: the export = %+v, %v", d.name(), res, err)
				}

				// An agent cannot mark a quiet day, on any surface that is not the person's.
				_, _, fail := d.do(t, mkStep(t, "CalendarQuietSet", service.CalendarQuietSetRequest{
					Date: "2026-12-25", Quiet: true,
				}))
				if fail == nil || fail.Code != service.CodePrivilegedSession {
					t.Fatalf("%s: marking a quiet day answered %+v, want E_PRIVILEGED_SESSION", d.name(), fail)
				}
			}

			run("remove", mkStep(t, "CalendarSlotDelete", service.CalendarSlotDeleteRequest{ID: s.ID}))

			_, _, fail := d.do(t, mkStep(t, "CalendarSlotDelete", service.CalendarSlotDeleteRequest{ID: s.ID}))
			if fail == nil || fail.Code != "E_NOT_FOUND" {
				t.Fatalf("%s: removing a slot twice answered %+v, want E_NOT_FOUND", d.name(), fail)
			}

			got[name] = results
		})
	}

	base := got["in-process"]
	for name, other := range got {
		for label, want := range other {
			if canonical(t, json.RawMessage(base[label])) != canonical(t, json.RawMessage(want)) {
				t.Errorf("%s differs between in-process and %s:\n%s\n%s", label, name, base[label], want)
			}
		}
	}
}
