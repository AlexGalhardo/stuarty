import { describe, expect, test } from "vitest";

import { drawForTest as draw } from "../../workflow/drawForTest";
import type { Workflow } from "../../workflow/types";
import { WorkflowSummary } from "./WorkflowSection";

const VIDEO_WORKFLOW: Workflow = {
	kind: "video",
	current: "editing",
	next: "finishing",
	steps: [
		{ id: "briefing", state: "done", optional: false, actor: "person" },
		{ id: "script", state: "done", optional: false, actor: "agent" },
		{ id: "editing", state: "current", optional: false, actor: "agent" },
		{
			id: "approval",
			state: "waiting",
			optional: false,
			actor: "person",
			cost: [{ micros: 180_000, currency: "USD" }],
		},
		{ id: "publication", state: "upcoming", optional: false, actor: "person" },
	],
};

const TEXT_WORKFLOW: Workflow = {
	kind: "text",
	current: "writing",
	next: "review",
	steps: [
		{ id: "briefing", state: "done", optional: false, actor: "person" },
		{ id: "writing", state: "current", optional: false, actor: "agent" },
		{ id: "review", state: "upcoming", optional: false, actor: "person" },
		{ id: "approval", state: "upcoming", optional: false, actor: "person" },
		{ id: "publication", state: "upcoming", optional: false, actor: "person" },
	],
};

describe("WorkflowSummary", () => {
	test("shows the estimated cost chip and renders next post as plain text (screen 22)", () => {
		const html = draw(
			<WorkflowSummary workflow={VIDEO_WORKFLOW} next="2026-11-27T18:00:00Z" />,
			"en",
		);
		expect(html).toContain('data-testid="workflow-cost"');
		expect(html).toContain("Estimated cost: $0.18");
		expect(html).toMatch(
			/<p[^>]*data-testid="flow-next-post"[^>]*>Scheduled for:/,
		);
		expect(html).not.toContain("2026-11-27T18:00:00Z");
	});

	test("shows Free (Grátis) estimated cost and text facts for a text project (screen 33c)", () => {
		const html = draw(
			<WorkflowSummary
				workflow={TEXT_WORKFLOW}
				textKind="post"
				text={{
					item_id: "pit_1",
					text_kind: "post",
					words: 842,
					reading_minutes: 4,
					excerpt: "Natural light at home",
				}}
				next="2026-11-27T12:00:00Z"
			/>,
			"pt-BR",
		);
		expect(html).toContain("Post de blog");
		expect(html).toContain("842 palavras");
		expect(html).toContain("Custo previsto: Grátis");
		expect(html).toMatch(
			/<p[^>]*data-testid="flow-next-post"[^>]*>Publicação prevista:/,
		);
	});

	test("omits estimated cost chip on undecided workflows without an approval step", () => {
		const undecided: Workflow = {
			kind: "undecided",
			current: "choice",
			steps: [
				{ id: "briefing", state: "done", optional: false, actor: "person" },
				{ id: "choice", state: "current", optional: false, actor: "person" },
			],
		};
		const html = draw(<WorkflowSummary workflow={undecided} />, "en");
		expect(html).not.toContain('data-testid="workflow-cost"');
	});
});
