import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { Frame } from "../../shell/testSupport";
import AgentPanel from "./AgentPanel";
import CaptionPanel from "./CaptionPanel";
import type { FlowStep, ProjectFlow, TextItem, WritingClient } from "./client";
import { PRESETS } from "./DraftPanel";
import PostPanel from "./PostPanel";
import ScriptPanel from "./ScriptPanel";

// The panels read the service in effects, which a server render does not run: what is drawn here is the first
// state of each, before an answer.
const client = {} as WritingClient;

const item: TextItem = {
	id: "itm_1",
	project_id: "prj_1",
	title: "Hello",
	text_kind: "copy",
	state: "active",
	words: 12,
	revision: 1,
	updated_at: "2026-11-24T10:00:00Z",
};

function step(id: FlowStep["id"], state: FlowStep["state"]): FlowStep {
	return { id, state, optional: false, actor: "agent", tool: "TextWrite" };
}

function draw(flow: ProjectFlow | null): string {
	return renderToString(
		<Frame>
			<AgentPanel client={client} item={item} flow={flow} onDone={() => {}} />
		</Frame>,
	);
}

describe("the agent panel of a text project", () => {
	const flow: ProjectFlow = {
		kind: "text",
		steps: [step("writing", "current")],
		current: "writing",
	};

	test("says that cut, video caption and export do not apply", () => {
		const html = draw(flow);
		expect(html).toContain("This project is text only");
		expect(html).toContain("Cut, video caption and export do not apply.");
	});

	test("offers the text actions and no video one", () => {
		// The presets are inside the form that a connection opens, so they are checked at their source.
		expect([...PRESETS]).toEqual(["draft", "rewrite", "shorten"]);
		const html = draw(flow);
		expect(html).toContain("draft, rewrite and shorten");
		for (const word of [">Cut<", "Add b-roll", ">Render<", ">Export<"]) {
			expect(html).not.toContain(word);
		}
	});

	test("shows the skill and the tool before an answer as not reported, and the step of the path", () => {
		const html = draw(flow);
		expect(html).toMatch(/chat-activity-skill.*Not reported/s);
		expect(html).toMatch(/chat-activity-tool.*Not reported/s);
		expect(html).toContain("Reading what the agent is doing.");
		expect(html).toContain("Now in the path: Write. The agent does it.");
		expect(html).toContain("operation TextWrite");
	});

	test("an unread project is drawn as text too: no video is assumed", () => {
		expect(draw(null)).toContain("This project is text only");
	});

	test("a video project does not carry the text only sentence", () => {
		const video: ProjectFlow = { ...flow, kind: "video" };
		expect(draw(video)).not.toContain("This project is text only");
	});
});

describe("the panels share one frame", () => {
	test("each panel is a labelled region with its heading", () => {
		const html = renderToString(
			<Frame>
				<ScriptPanel client={client} projectId="prj_1" body="" />
				<CaptionPanel client={client} projectId="prj_1" body="" />
				<PostPanel
					client={client}
					projectId="prj_1"
					itemId="itm_1"
					revision={1}
					dirty={false}
				/>
			</Frame>,
		);
		for (const id of ["script-heading", "caption-heading", "post-heading"]) {
			expect(html).toContain(`id="${id}"`);
			expect(html).toContain(`aria-labelledby="${id}"`);
		}
		expect(html).toContain("Write some narration to see how long it takes.");
		expect(html).toContain("Write the caption to see the counters.");
		expect(html).toContain("Save the post to see its check.");
	});
});
