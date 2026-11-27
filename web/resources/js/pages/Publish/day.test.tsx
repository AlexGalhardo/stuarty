import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import { ApiError } from "../../editor/api/errors";
import { CalendarView, shouldOpenDayOnDoubleClick } from "./CalendarView";
import { dateRange, daysOf, queryRange } from "./calendar";
import { createCalendarApi, exportHref } from "./calendarApi";
import { DayView } from "./DayView";
import { type CalendarPlan, isTheme, THEMES } from "./dayTypes";
import { MiniMonth } from "./MiniMonth";
import { THEME_CLASSES, themeSurface } from "./theme";
import type { Publication, Target } from "./types";
import { isQuietRefusal, QUIET_PATH, QuietDeclined } from "./useQuietConfirm";

const messages: Record<string, string> = {
	"ui.publish.moving": "Moving to {time}",
	"ui.publish.mini.previous": "Show the month before",
	"ui.publish.mini.next": "Show the month after",
	"ui.publish.day.open": "Open {day}",
	"ui.publish.day.add_at": "New post at {time}",
	"ui.publish.day.adjust": "Adjust",
	"ui.publish.day.adjust_save": "Apply",
	"ui.publish.day.time": "Time",
	"ui.publish.day.text": "Text for this account",
	"ui.publish.day.slot_empty": "Empty slot: fill it",
	"ui.publish.day.plan": "Plan of the day",
	"ui.publish.day.note": "Note",
	"ui.publish.day.note_help": "Saved when you leave the field.",
	"ui.publish.day.theme": "Theme",
	"ui.publish.day.theme_none": "None",
	"ui.publish.quiet.badge": "No posts",
	"ui.publish.quiet.switch": "No posts this day",
	"ui.publish.quiet.help": "A quiet day is a rule.",
	"ui.publish.mini.label": "Small month",
	"ui.publish.mini.has_note": "has a note",
	"ui.publish.mini.go_label": "Go to date",
	"ui.publish.mini.go": "Go",
	"ui.publish.editor.account": "Account",
	"ui.publish.state.scheduled": "Scheduled",
	"ui.publish.state.draft": "Draft",
};
for (const name of THEMES) messages[`ui.publish.day.theme_${name}`] = name;

function wrap(node: React.ReactNode): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			{node}
		</I18nProvider>,
	);
}

const target = (extra: Partial<Target> = {}): Target => ({
	id: "tgt_1",
	publication_id: "pub_1",
	connection_id: "con_1",
	network: "instagram",
	fields: {},
	state: "scheduled",
	scheduled_at: "2026-12-01T21:30:00Z",
	zone: "America/Sao_Paulo",
	local_time: "2026-12-01 18:30",
	late_window_minutes: 0,
	attempts: 0,
	updated_at: "2026-11-19T12:00:00Z",
	...extra,
});

const publication = (targets: Target[]): Publication => ({
	id: "pub_1",
	project_id: "prj_1",
	title: "Week one",
	export: "job_1",
	targets,
	created_at: "2026-11-19T12:00:00Z",
	updated_at: "2026-11-19T12:00:00Z",
});

const plan = (extra: Partial<CalendarPlan> = {}): CalendarPlan => ({
	from: "2026-12-01T03:00:00Z",
	to: "2026-12-02T03:00:00Z",
	zone: "America/Sao_Paulo",
	days: [],
	rules: [],
	slots: [],
	publications: [],
	total: 0,
	...extra,
});

const noApi = createCalendarApi({
	send: () => Promise.reject(new Error("no request in this test")),
} as never);

function day(plan_: CalendarPlan | null, pubs: Publication[] = []): string {
	return wrap(
		<DayView
			date="2026-12-01"
			zone="America/Sao_Paulo"
			plan={plan_}
			publications={pubs}
			accounts={[]}
			calendarApi={noApi}
			onOpen={() => undefined}
			onCreate={() => undefined}
			onMove={() => undefined}
			onAdjust={() => Promise.resolve(true)}
			onChanged={() => undefined}
			onFail={() => undefined}
		/>,
	);
}

describe("the day view", () => {
	test("has the 24 hours as a column, an empty hour is a button, and the note, the theme and the mark are there", () => {
		const html = day(plan());
		expect(html.match(/data-testid="day-hour"/g)).toHaveLength(24);
		expect(html).toContain('data-testid="day-hour-add-9"');
		expect(html).toContain("New post at 09:00");
		expect(html).toContain('data-testid="day-note"');
		expect(html).toContain('data-testid="day-quiet"');
		for (const name of THEMES) {
			expect(html).toContain(`data-testid="day-theme-${name}"`);
		}
		expect(html).not.toContain('data-testid="day-quiet-badge"');
	});

	test("puts a post at its hour in the zone of the person, with its state in words", () => {
		const html = day(plan(), [publication([target()])]);
		const chip = html.indexOf('data-testid="target-chip"');
		expect(chip).toBeGreaterThan(0);
		const hour = html.lastIndexOf('data-hour="', chip);
		expect(html.slice(hour, hour + 14)).toContain('data-hour="18"');
		expect(html).toContain("18:30");
		expect(html).toContain("Scheduled");
		expect(html).toContain('data-testid="day-adjust"');
	});

	test("a post that went out cannot be adjusted in place", () => {
		const html = day(plan(), [publication([target({ state: "published" })])]);
		expect(html).toContain('data-testid="target-chip"');
		expect(html).not.toContain('data-testid="day-adjust"');
	});

	test("shows the note, the theme and the quiet mark the service answered, and an empty slot to fill", () => {
		const html = day(
			plan({
				days: [
					{
						date: "2026-12-01",
						zone: "America/Sao_Paulo",
						note: "long video day",
						theme: "amber",
						quiet: true,
						updated_at: "2026-11-19T12:00:00Z",
					},
				],
				slots: [
					{
						slot_id: "slt_1",
						network: "instagram",
						date: "2026-12-01",
						time: "09:00",
						zone: "America/Sao_Paulo",
						at: "2026-12-01T12:00:00Z",
					},
				],
			}),
		);
		expect(html).toContain("long video day");
		expect(html).toContain('data-theme="amber"');
		expect(html).toContain('data-testid="day-quiet-badge"');
		expect(html).toContain('data-testid="day-slot-empty"');
		const slot = html.indexOf('data-testid="day-slot-empty"');
		const hour = html.lastIndexOf('data-hour="', slot);
		expect(html.slice(hour, hour + 14)).toContain('data-hour="9"');
	});

	test("a slot that a post fills is not shown as empty", () => {
		const html = day(
			plan({
				slots: [
					{
						slot_id: "slt_1",
						network: "instagram",
						date: "2026-12-01",
						time: "18:00",
						zone: "America/Sao_Paulo",
						at: "2026-12-01T21:00:00Z",
						filled: { publication_id: "pub_1", target_id: "tgt_1" },
					},
				],
			}),
			[publication([target()])],
		);
		expect(html).not.toContain('data-testid="day-slot-empty"');
	});

	test("the text of a note is the person's and is drawn as text, never as markup", () => {
		const html = day(
			plan({
				days: [
					{
						date: "2026-12-01",
						zone: "UTC",
						note: "<img src=x onerror=window.__pwned=1>",
						updated_at: "2026-11-19T12:00:00Z",
					},
				],
			}),
		);
		expect(html).not.toContain("<img");
		expect(html).toContain("&lt;img");
	});
});

describe("the small month", () => {
	const render = (days: CalendarPlan["days"], selected = "2026-12-01") =>
		wrap(
			<MiniMonth
				month="2026-12-01"
				selected={selected}
				weekStart={0}
				zone="UTC"
				days={days}
				onMonth={() => undefined}
				onPick={() => undefined}
			/>,
		);

	test("is a month of six weeks with a date field to go to", () => {
		const html = render([]);
		expect(html.match(/data-testid="mini-day"/g)).toHaveLength(42);
		expect(html).toContain('data-testid="mini-go"');
		expect(html).toContain("December 2026");
		expect(html).toContain('aria-pressed="true"');
	});

	test("the days that hold a note or a mark carry a sign in words for a screen reader", () => {
		const html = render([
			{ date: "2026-12-24", zone: "UTC", note: "prep", updated_at: "" },
			{
				date: "2026-12-25",
				zone: "UTC",
				quiet: true,
				theme: "red",
				updated_at: "",
			},
		]);
		const label = (date: string): string => {
			const at = html.indexOf(`data-day="${date}"`);
			const start = html.lastIndexOf("<button", at);
			return html.slice(start, html.indexOf(">", at));
		};
		expect(label("2026-12-24")).toContain("has a note");
		expect(label("2026-12-25")).toContain("No posts");
		expect(label("2026-12-25")).toContain("bg-danger-soft");
	});
});

describe("the month and the week open a day", () => {
	const render = (view: "month" | "week", marks: CalendarPlan["days"]) =>
		wrap(
			<CalendarView
				view={view}
				anchor="2026-12-01"
				weekStart={0}
				zone="UTC"
				publications={[]}
				marks={new Map(marks.map((d) => [d.date, d]))}
				onOpen={() => undefined}
				onOpenDay={() => undefined}
				onMove={() => undefined}
			/>,
		);

	test("the number of every day is a button with the name of the day", () => {
		const html = render("month", []);
		expect(html.match(/data-testid="calendar-day-open"/g)).toHaveLength(42);
		expect(html).toContain("Open Tuesday, December 1");
	});

	test("a day with a theme, a note and the mark shows them", () => {
		const html = render("week", [
			{
				date: "2026-12-01",
				zone: "UTC",
				note: "series",
				theme: "teal",
				quiet: true,
				updated_at: "",
			},
		]);
		expect(html).toContain('data-theme="teal"');
		expect(html).toContain("data-quiet");
		expect(html).toContain('data-testid="calendar-day-quiet"');
		expect(html).toContain("series");
	});

	test("double-click opens the day on cell background but not on a post chip or day button", () => {
		const fakeEl = (matches: boolean): EventTarget =>
			({
				closest: () => (matches ? {} : null),
			}) as unknown as EventTarget;
		expect(shouldOpenDayOnDoubleClick(fakeEl(false))).toBe(true);
		expect(shouldOpenDayOnDoubleClick(fakeEl(true))).toBe(false);
		expect(shouldOpenDayOnDoubleClick(null)).toBe(true);
	});
});

describe("the range of a day", () => {
	test("a day is one date, and its range runs to the next date", () => {
		const days = daysOf("day", "2026-12-31", 0);
		expect(days).toEqual([{ key: "2026-12-31", inMonth: true }]);
		expect(dateRange(days)).toEqual({ from: "2026-12-31", to: "2027-01-01" });
		expect(queryRange(days).from).toBe("2026-12-30T00:00:00Z");
	});

	test("a week is seven dates and a month six weeks", () => {
		expect(dateRange(daysOf("week", "2026-12-01", 0))).toEqual({
			from: "2026-11-29",
			to: "2026-12-06",
		});
		expect(dateRange(daysOf("month", "2026-12-01", 0)).from).toBe("2026-11-29");
	});
});

describe("the calls of the day", () => {
	function recording() {
		const seen: { method: string; path: string; body: unknown }[] = [];
		const transport = {
			send: (r: { method: string; path: string; body?: unknown }) => {
				seen.push({ method: r.method, path: r.path, body: r.body });
				return Promise.resolve({ status: 200, body: {} });
			},
		};
		return { seen, api: createCalendarApi(transport as never) };
	}

	test("the plan asks for a range in a zone, and for the publications only when told", async () => {
		const { seen, api } = recording();
		await api.plan({
			from: "2026-12-01",
			to: "2026-12-02",
			zone: "Europe/Lisbon",
		});
		await api.plan({
			from: "2026-12-01",
			to: "2026-12-02",
			zone: "UTC",
			publications: true,
		});
		expect(seen[0]?.path).toBe(
			"/calendar/plan?from=2026-12-01&to=2026-12-02&zone=Europe%2FLisbon",
		);
		expect(seen[1]?.path).toContain("publications=true");
	});

	test("the day, the mark, the slot and the copy go to their routes with what the person did", async () => {
		const { seen, api } = recording();
		await api.setDay("2026-12-01", { zone: "UTC", note: "n", theme: "blue" });
		await api.setQuiet("2026-12-01", "UTC", true);
		await api.saveSlot({ network: "instagram", weekday: "tue", time: "18:00" });
		await api.removeSlot("slt_1");
		await api.duplicate("pub_1", { at: "2026-12-08T09:00", zone: "UTC" });
		expect(seen.map((s) => `${s.method} ${s.path}`)).toEqual([
			"PUT /calendar/days/2026-12-01",
			"PUT /calendar/quiet/2026-12-01",
			"PUT /calendar/slots",
			"DELETE /calendar/slots/slt_1",
			"POST /publications/pub_1/duplicate",
		]);
		expect(seen[1]?.body).toEqual({ zone: "UTC", quiet: true });
	});

	test("the export is a plain link to the file of the range", () => {
		expect(
			exportHref({ from: "2026-12-01", to: "2027-01-01", zone: "UTC" }),
		).toBe("/api/v1/calendar/export?from=2026-12-01&to=2027-01-01&zone=UTC");
	});
});

describe("a quiet day", () => {
	test("the service names it by a path, and only that refusal asks the person", () => {
		const quiet = new ApiError(409, {
			code: "E_INVALID_STATE",
			message: "2026-12-25 is marked as a quiet day (no posts).",
			path: QUIET_PATH,
		});
		expect(isQuietRefusal(quiet)).toBe(true);
		expect(
			isQuietRefusal(
				new ApiError(409, { code: "E_INVALID_STATE", message: "x" }),
			),
		).toBe(false);
		expect(
			isQuietRefusal(
				new ApiError(403, {
					code: "E_PRIVILEGED_SESSION",
					message: "x",
					path: QUIET_PATH,
				}),
			),
		).toBe(false);
		expect(isQuietRefusal(new Error("x"))).toBe(false);
		expect(new QuietDeclined().name).toBe("QuietDeclined");
	});
});

describe("the themes", () => {
	test("are the names the service knows, and the page knows no others", () => {
		expect(THEMES).toEqual(["blue", "green", "amber", "red", "teal", "gray"]);
		expect(isTheme("amber")).toBe(true);
		expect(isTheme("#ff0000")).toBe(false);
		expect(themeSurface("pink")).toBe("");
		expect(themeSurface(undefined)).toBe("");
	});

	test("each is drawn in a pair whose contrast scripts/calendar-themes.test.ts measures", () => {
		for (const name of THEMES) {
			expect(THEME_CLASSES[name].surface).toMatch(/^bg-\S+ text-\S+$/);
		}
	});
});
