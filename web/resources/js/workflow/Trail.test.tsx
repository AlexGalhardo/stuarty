import { describe, expect, test } from "vitest";

import { drawForTest as draw } from "./drawForTest";
import { Trail } from "./Trail";
import { TrailDetail } from "./TrailDetail";
import type { Workflow } from "./types";

// A fixed payload of a text project, as `ProjectGet` sends it (SDD-000 section 1).
const TEXT: Workflow = {
	kind: "text",
	current: "writing",
	next: "review",
	steps: [
		{
			id: "briefing",
			state: "done",
			optional: false,
			actor: "both",
			tool: "BriefingAnswer",
		},
		{
			id: "writing",
			state: "current",
			optional: false,
			actor: "both",
			tool: "TextWrite",
		},
		{ id: "review", state: "upcoming", optional: false, actor: "person" },
		{
			id: "approval",
			state: "waiting",
			optional: false,
			actor: "person",
			tool: "ApprovalDecide",
			cost: [{ micros: 1_500_000, currency: "USD" }],
		},
		{
			id: "publication",
			state: "skipped",
			optional: true,
			actor: "both",
			tool: "PublicationCreate",
		},
	],
};

describe("Trail", () => {
	test("draws the steps the server gave, in its order and with its states", () => {
		const html = draw(<Trail workflow={TEXT} />);
		const order = ["Briefing", "Writing", "Review", "Approval", "Publication"];
		let from = 0;
		for (const title of order) {
			const at = html.indexOf(title, from);
			expect(at, title).toBeGreaterThanOrEqual(from);
			from = at;
		}
		expect(html.match(/data-slot="numbered-step"/g)).toHaveLength(5);
		expect(html).toContain('data-kind="text"');
		expect(html).toContain("Done");
		expect(html).toContain("Waiting for you");
		expect(html).toContain("Skipped");
		expect(html).not.toContain("Script");
	});

	test("adds no step for a text project and none of a video", () => {
		const html = draw(<Trail workflow={{ ...TEXT, steps: [] }} />);
		expect(html).not.toContain("numbered-step");
	});
});

describe("TrailDetail", () => {
	test("shows the actor, the tool, the state and the cost of every step", () => {
		const html = draw(<TrailDetail workflow={TEXT} />);
		expect(html.match(/data-slot="trail-detail-step"/g)).toHaveLength(5);
		expect(html.match(/Who does it/g)).toHaveLength(5);
		expect(html.match(/Tool<\/dt>/g)).toHaveLength(5);
		expect(html.match(/Cost<\/dt>/g)).toHaveLength(5);
		expect(html).toContain("You and the agent");
		expect(html).toContain("Write the text");
		expect(html).toContain("$1.50");
	});

	test("reads Not reported for a value the server did not give", () => {
		const html = draw(
			<TrailDetail
				workflow={{
					kind: "text",
					steps: [
						{
							id: "review",
							state: "unknown",
							optional: false,
							actor: "person",
						},
					],
				}}
			/>,
		);
		// The state, the tool and the cost.
		expect(html.match(/Not reported/g)).toHaveLength(3);
		expect(html).not.toContain("ui.workflow");
	});

	test("reads Not reported for an actor or a state it does not know", () => {
		const html = draw(
			<TrailDetail
				workflow={{
					kind: "video",
					steps: [
						{
							id: "editing",
							state: "mystery",
							optional: false,
							actor: "robot",
							tool: "Edit",
							cost: [{ micros: 0, currency: "USD" }],
						} as unknown as Workflow["steps"][number],
					],
				}}
			/>,
		);
		expect(html.match(/Not reported/g)).toHaveLength(2);
		expect(html).not.toContain("ui.workflow");
	});

	test("offers skip to an optional step and restore to a skipped one, not to a required one", () => {
		const html = draw(<TrailDetail workflow={TEXT} onSkip={() => undefined} />);
		expect(html).toContain('data-testid="restore-publication"');
		expect(html).not.toContain('data-testid="skip-writing"');
		const video = draw(
			<TrailDetail
				workflow={{
					kind: "video",
					steps: [
						{
							id: "recording",
							state: "upcoming",
							optional: true,
							actor: "person",
							tool: "MediaImport",
						},
					],
				}}
				onSkip={() => undefined}
			/>,
		);
		expect(video).toContain('data-testid="skip-recording"');
		expect(draw(<TrailDetail workflow={TEXT} />)).not.toContain("restore-");
	});

	test("is translated: every state and step has a word in Portuguese", () => {
		const html = draw(<TrailDetail workflow={TEXT} />, "pt-BR");
		expect(html).toContain("Esperando você");
		expect(html).toContain("Escrita");
		expect(html).not.toContain("ui.workflow");
	});
});
