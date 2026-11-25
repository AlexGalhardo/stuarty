import { describe, expect, test } from "vitest";

import type { InboxRow } from "../../editor/api/types";
import { inboxRow } from "../../shell/testSupport";
import { drawForTest } from "../../workflow/drawForTest";
import type { Workflow } from "../../workflow/types";
import { dayPart, greetingName } from "./greeting";
import { HomeView, summaryLine } from "./Home";
import type { HomeState } from "./useHome";

const PROJECT = {
	id: "prj_aaaaaaaaaa",
	name: "Reels: 5 filming mistakes",
	revision: 3,
	state: "active" as const,
	width: 1080,
	height: 1920,
	fps: "30",
	duration: { frame: 0, seconds: 0 },
	tracks: 1,
	items: 1,
	assets: 1,
	timeline_id: "tl_aaaaaaaaaa",
	timelines: 1,
	updated_at: "2026-11-24T10:00:00Z",
	kind: "video" as const,
};

const TEXT_PROJECT = {
	...PROJECT,
	id: "prj_bbbbbbbbbb",
	name: "Post: natural light at home",
	kind: "text" as const,
	text_summary: {
		item_id: "itm_aaaaaaaaaa",
		text_kind: "post",
		words: 842,
		reading_minutes: 4,
		excerpt: "The best light in your home costs nothing.",
	},
};

const VIDEO: Workflow = {
	kind: "video",
	current: "editing",
	next: "finishing",
	steps: [
		{ id: "briefing", state: "done", optional: false, actor: "both" },
		{ id: "script", state: "done", optional: false, actor: "both" },
		{ id: "recording", state: "done", optional: true, actor: "person" },
		{ id: "editing", state: "current", optional: false, actor: "both" },
		{ id: "finishing", state: "upcoming", optional: true, actor: "agent" },
		{ id: "approval", state: "upcoming", optional: false, actor: "person" },
		{ id: "publication", state: "upcoming", optional: false, actor: "person" },
	],
};

const TEXT: Workflow = {
	kind: "text",
	current: "writing",
	steps: [
		{ id: "briefing", state: "done", optional: false, actor: "both" },
		{ id: "writing", state: "current", optional: false, actor: "both" },
	],
};

const READY: HomeState = {
	state: "ready",
	projects: [PROJECT, TEXT_PROJECT],
	workflows: { [PROJECT.id]: VIDEO, [TEXT_PROJECT.id]: TEXT },
};

function rows(count: number): InboxRow[] {
	return Array.from({ length: count }, (_, i) =>
		inboxRow({
			id: `approval:apr_${i}`,
			source: "approval",
			source_id: `apr_${i}`,
			tool: "voice.generate",
			summary: `Generate a voice-over ${i}`,
			destination: "OpenAI",
			cost: { micros: 180_000, currency: "USD" },
		}),
	);
}

const draw = (
	over: Partial<{
		home: HomeState;
		rows: InboxRow[] | null;
		name: string | null;
		hour: number;
	}> = {},
	locale: "en" | "pt-BR" = "en",
): string =>
	drawForTest(
		<HomeView
			home={over.home ?? READY}
			rows={over.rows === undefined ? rows(3) : over.rows}
			name={over.name === undefined ? "Alex" : over.name}
			hour={over.hour ?? 9}
		/>,
		locale,
	);

describe("the greeting", () => {
	test("a person named Alex in the morning is greeted by name", () => {
		const html = draw();
		expect(html).toMatch(/<h1[^>]*>Good morning, Alex\.<\/h1>/);
	});

	test("with no name the greeting is only the part of the day", () => {
		expect(draw({ name: null })).toMatch(/<h1[^>]*>Good morning\.<\/h1>/);
	});

	test("the word follows the local hour, in both languages", () => {
		expect(dayPart(4)).toBe("evening");
		expect(dayPart(5)).toBe("morning");
		expect(dayPart(11)).toBe("morning");
		expect(dayPart(12)).toBe("afternoon");
		expect(dayPart(17)).toBe("afternoon");
		expect(dayPart(18)).toBe("evening");
		expect(draw({ hour: 14 })).toContain("Good afternoon, Alex.");
		expect(draw({ hour: 21 })).toContain("Good evening, Alex.");
		expect(draw({ hour: 9 }, "pt-BR")).toContain("Bom dia, Alex.");
		expect(draw({ hour: 20 }, "pt-BR")).toContain("Boa noite, Alex.");
	});

	test("the name is the username of the unlocked local account, in capitals at the start", () => {
		expect(greetingName({ state: "unlocked", username: "alex" })).toBe("Alex");
		expect(greetingName({ state: "unlocked", username: " marina " })).toBe(
			"Marina",
		);
		expect(greetingName({ state: "locked", username: "alex" })).toBeNull();
		expect(greetingName({ state: "none", username: "" })).toBeNull();
		expect(greetingName(undefined)).toBeNull();
	});
});

describe("the home", () => {
	test("says how many productions are at a step and how many requests wait", () => {
		expect(draw()).toContain(
			"2 productions in progress and 3 requests waiting for your yes.",
		);
		expect(draw({ rows: rows(1) })).toContain(
			"2 productions in progress and 1 request waiting for your yes.",
		);
	});

	test("the summary names only what exists", () => {
		const t = (key: string, params?: Record<string, string | number>): string =>
			`${key}${params ? JSON.stringify(params) : ""}`;
		expect(summaryLine(t, 0, 0)).toBe("ui.dashboard.home.lead");
		expect(summaryLine(t, 2, 0)).toContain("ui.dashboard.home.summary.only");
		expect(summaryLine(t, 0, 2)).toContain("ui.dashboard.home.summary.only");
	});

	test("the project in progress shows the path the service drew and one button for its current step", () => {
		const html = draw();
		expect(html).toContain('data-testid="home-progress"');
		expect(html).toContain("Reels: 5 filming mistakes");
		for (const title of [
			"Briefing",
			"Script",
			"Footage",
			"Edit",
			"Finishing",
			"Approval",
			"Publishing",
		]) {
			expect(html, title).toContain(title);
		}
		expect(html).toContain("Optional, done");
		expect(html).toMatch(
			/href="\/editor\/prj_aaaaaaaaaa"[^>]*>.*Continue editing/,
		);
		expect(html).toContain("Open project");
		expect(html).toContain("You are at Edit.");
	});

	test("the studio has six areas that link to pages that exist", () => {
		const html = draw();
		expect(html).toContain("Your studio");
		for (const key of [
			"projects",
			"writing",
			"assets",
			"agent",
			"publish",
			"connections",
		]) {
			expect(html).toContain(`data-testid="home-area-${key}"`);
		}
		expect(html).toContain('href="/write"');
		expect(html).toContain('href="/connections"');
	});

	test("what waits is a list of rows from the inbox, three at most, with the way to the queue", () => {
		const html = draw({ rows: rows(5) });
		expect(html.match(/data-testid="home-waiting-row"/g)).toHaveLength(3);
		expect(html).toContain("Generate a voice-over 0");
		expect(html).toContain("Reels: 5 filming mistakes, $0.18");
		expect(html).toContain("And 2 more requests in the queue.");
		expect(html).toContain("Nothing leaves without your yes.");
		expect(html).toContain('href="/approvals"');
		expect(html).toContain("Waiting for you");
	});

	test("an answered request is not a row that waits, and an empty inbox says so", () => {
		const html = draw({
			rows: [{ ...rows(1)[0], state: "approved" } as InboxRow],
		});
		expect(html).not.toContain('data-testid="home-waiting-row"');
		expect(html).toContain("Nothing waits for you now.");
	});

	test("the other projects show their kind, their words and the step they are at", () => {
		const html = draw();
		expect(html).toContain("Post: natural light at home");
		expect(html).toContain("Blog post, 842 words");
		expect(html).toContain(">Writing<");
	});

	test("no project says nothing of video and offers the first step and the studio", () => {
		const html = draw({
			home: { state: "ready", projects: [], workflows: {} },
			rows: [],
		});
		expect(html).toContain("Make your first project");
		expect(html).toContain("How it works");
		expect(html).toContain("Your studio");
		expect(html).not.toContain('data-testid="home-progress"');
	});

	test("a failed read shows the failure and a read that has not ended says so", () => {
		expect(draw({ home: { state: "loading" } })).toContain(
			"Reading your projects.",
		);
	});
});
