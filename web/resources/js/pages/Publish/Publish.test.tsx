import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { CalendarView } from "./CalendarView";
import { ListView } from "./ListView";
import { actionsOf, applyPublicationEvent, linkOf } from "./publicationState";
import type { Publication, Target, TargetState } from "./types";
import { TARGET_STATES } from "./types";

const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

const messages: Record<string, string> = {
	"ui.publish.moving": "Moving to {time}",
	"ui.publish.empty": "Nothing is planned.",
	"ui.publish.list_caption": "Publications",
	"ui.publish.col.when": "When",
	"ui.publish.col.title": "Title",
	"ui.publish.col.account": "Account",
	"ui.publish.col.state": "State",
	"ui.publish.col.result": "Result",
	"ui.publish.col.actions": "Actions",
	"ui.publish.no_time": "No time",
	"ui.publish.open": "Open",
	"ui.publish.error": "Error",
};
for (const state of TARGET_STATES) {
	messages[`ui.publish.state.${state}`] = `State ${state}`;
}

const target = (extra: Partial<Target> = {}): Target => ({
	id: "tgt_1",
	publication_id: "pub_1",
	connection_id: "con_1",
	network: "youtube",
	fields: {},
	state: "scheduled",
	scheduled_at: "2026-11-20T12:00:00Z",
	zone: "America/Sao_Paulo",
	local_time: "2026-11-20 09:00",
	late_window_minutes: 0,
	attempts: 0,
	updated_at: "2026-11-19T12:00:00Z",
	...extra,
});

const publication = (targets: Target[], title = "Week one"): Publication => ({
	id: "pub_1",
	project_id: "prj_1",
	title,
	export: "job_1",
	targets,
	created_at: "2026-11-19T12:00:00Z",
	updated_at: "2026-11-19T12:00:00Z",
});

function wrap(node: React.ReactNode): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			{node}
		</I18nProvider>,
	);
}

describe("the calendar puts each target on the day of its time in the zone of the person", () => {
	const render = (view: "month" | "week", zone: string, p: Publication) =>
		wrap(
			<CalendarView
				view={view}
				anchor="2026-11-20"
				weekStart={0}
				zone={zone}
				publications={[p]}
				onOpen={() => undefined}
				onMove={() => undefined}
			/>,
		);

	test("a month of six weeks, with the target as a button with its time, network and state in words", () => {
		const html = render("month", "America/Sao_Paulo", publication([target()]));
		expect(html.match(/data-testid="calendar-day"/g)).toHaveLength(42);
		expect(html).toContain('data-testid="target-chip"');
		expect(html).toContain('data-target-id="tgt_1"');
		expect(html).toContain("09:00");
		expect(html).toContain("youtube");
		expect(html).toContain("State scheduled");
		const chip = html.indexOf('data-testid="target-chip"');
		const day = html.lastIndexOf('data-day="', chip);
		expect(html.slice(day, day + 24)).toContain("2026-11-20");
	});

	test("the day moves with the zone", () => {
		const html = render("week", "Asia/Tokyo", publication([target()]));
		const chip = html.indexOf('data-testid="target-chip"');
		const day = html.lastIndexOf('data-day="', chip);
		expect(html.slice(day, day + 24)).toContain("2026-11-20");
		expect(html).toContain("21:00");
	});

	test("a week of seven days", () => {
		const html = render("week", "UTC", publication([]));
		expect(html.match(/data-testid="calendar-day"/g)).toHaveLength(7);
	});

	test("a target that went out cannot be picked up, and one that waits can", () => {
		const done = render(
			"month",
			"UTC",
			publication([target({ state: "published" })]),
		);
		expect(done).not.toContain("aria-pressed");
		expect(done).toContain('draggable="false"');
		const waiting = render("month", "UTC", publication([target()]));
		expect(waiting).toContain('aria-pressed="false"');
		expect(waiting).toContain('draggable="true"');
	});

	test("the text of a title is isolated and never markup", () => {
		const html = render("month", "UTC", publication([target()], PAYLOAD));
		expect(html).not.toContain(PAYLOAD);
		expect(html).toContain("&lt;img src=x onerror=window.__pwned=1&gt;");
	});

	test("a target with no time is not on the calendar", () => {
		const html = render(
			"month",
			"UTC",
			publication([target({ scheduled_at: null, state: "draft" })]),
		);
		expect(html).not.toContain('data-testid="target-chip"');
	});
});

describe("the list shows the result of a target", () => {
	const list = (p: Publication) =>
		wrap(<ListView publications={[p]} onOpen={() => undefined} />);

	test("an address as a link when it is https, with no markup of the network", () => {
		const html = list(
			publication([
				target({ state: "published", url: "https://fake.example/watch/a" }),
			]),
		);
		expect(html).toContain('href="https://fake.example/watch/a"');
		expect(html).toContain('rel="noreferrer noopener"');
		expect(html).toContain("2026-11-20 09:00");
		expect(html).toContain("America/Sao_Paulo");
		const other = list(
			publication([target({ state: "published", url: "javascript:alert(1)" })]),
		);
		expect(other).not.toContain("href=");
	});

	test("the error of a failed target with what to do", () => {
		const html = list(
			publication([
				target({
					state: "failed",
					error: {
						code: "E_PUBLISH",
						kind: "quota",
						message: "The daily limit was used.",
						hint: "Publish again tomorrow.",
						retryable: false,
					},
				}),
			]),
		);
		expect(html).toContain("State failed");
		expect(html).toContain("Error");
		expect(html).toContain("The daily limit was used.");
		expect(html).toContain("Publish again tomorrow.");
	});

	test("a draft is told apart from a publication and says it is finished in the app of the network", () => {
		const draft = list(
			publication([target({ state: "sent_as_draft", network: "tiktok" })]),
		);
		expect(draft).toContain('data-state="sent_as_draft"');
		expect(draft).toContain("State sent_as_draft");
		expect(draft).toContain("ui.publish.draft_note");
		expect(draft).not.toContain("State published");
		const published = list(publication([target({ state: "published" })]));
		expect(published).not.toContain("ui.publish.draft_note");
	});

	test("an empty calendar says so", () => {
		const html = wrap(<ListView publications={[]} onOpen={() => undefined} />);
		expect(html).toContain("Nothing is planned.");
	});
});

describe("what a state offers", () => {
	const offers = (state: TargetState) => actionsOf(state);

	test("a target that went out or is going out offers nothing", () => {
		for (const state of [
			"publishing",
			"published",
			"sent_as_draft",
			"cancelled",
		] as const) {
			expect(Object.values(offers(state)).every((v) => !v)).toBe(true);
		}
	});

	test("a failed one is retried, a missed one is sent now, a waiting one points at the approval", () => {
		expect(offers("failed").retry).toBe(true);
		expect(offers("missed").sendNow).toBe(true);
		expect(offers("missed").retry).toBe(false);
		expect(offers("waiting_approval").approve).toBe(true);
		expect(offers("draft").schedule).toBe(true);
	});

	test("every state has an answer", () => {
		for (const state of TARGET_STATES) expect(offers(state)).toBeDefined();
	});
});

describe("the events of the stream", () => {
	const one = publication([target()]);

	test("a publication that is on screen is replaced", () => {
		const next = publication([target({ state: "published" })], "New title");
		const result = applyPublicationEvent(
			[one],
			"publication.updated",
			JSON.stringify(next),
		);
		expect(result.known).toBe(true);
		expect(result.next[0]?.title).toBe("New title");
	});

	test("one that is not on screen is not added", () => {
		const other = { ...one, id: "pub_2" };
		const result = applyPublicationEvent(
			[one],
			"publication.updated",
			JSON.stringify(other),
		);
		expect(result.known).toBe(false);
		expect(result.next).toHaveLength(1);
	});

	test("the heartbeat and what is not a publication change nothing", () => {
		expect(applyPublicationEvent([one], null, "").next).toEqual([one]);
		expect(
			applyPublicationEvent([one], "publication.updated", "{}").next,
		).toEqual([one]);
		expect(
			applyPublicationEvent([one], "publication.updated", "nope").next,
		).toEqual([one]);
	});
});

describe("an address a network gave", () => {
	test("is a link only when it is https", () => {
		expect(linkOf("https://example.com/a")).toBe("https://example.com/a");
		expect(linkOf("http://example.com/a")).toBeNull();
		expect(linkOf("javascript:alert(1)")).toBeNull();
		expect(linkOf("not a url")).toBeNull();
		expect(linkOf(undefined)).toBeNull();
	});
});
