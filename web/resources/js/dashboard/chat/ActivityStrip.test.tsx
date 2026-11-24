import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";
import type { ChatActivity } from "../../editor/api/types";
import { activity, Frame } from "../../shell/testSupport";
import ActivityStrip from "./ActivityStrip";

function draw(a: ChatActivity | null): string {
	return renderToString(
		<Frame>
			<ActivityStrip activity={a} />
		</Frame>,
	);
}

describe("ActivityStrip", () => {
	test("says not reported until the service answers", () => {
		const html = draw(null);
		expect(html).toMatch(/chat-activity-skill.*Not reported/s);
		expect(html).toMatch(/chat-activity-tool.*Not reported/s);
	});

	test("a loaded skill and a tool of Stuarty: the name, the description and the technical name", () => {
		const html = draw(
			activity({
				skill: { name: "captions", state: "loaded" },
				tool: {
					title: "mcp__stuarty__library",
					technical_name: "library",
					action: "get",
					description_key: "ui.agent_activity.action.library_get",
					status: "completed",
					from_agent: false,
				},
			}),
		);
		expect(html).toMatch(
			/chat-activity-skill.*captions.*loaded in this session/s,
		);
		expect(html).toMatch(
			/chat-activity-tool.*Reads one note, skill or prompt\..*library · done.*text from the agent: .*dir="auto"[^>]*>mcp__stuarty__library</s,
		);
	});

	test("a tool of the agent is its title, marked as text from the agent; markup stays text", () => {
		const html = draw(
			activity({
				tool: {
					title: "<img src=x onerror=1>",
					status: "in_progress",
					from_agent: true,
				},
			}),
		);
		expect(html).toContain("&lt;img src=x onerror=1&gt;");
		expect(html).not.toContain("<img");
		expect(html).toContain("text from the agent");
	});

	test("no skill loaded says so, and whether a reviewed one is available", () => {
		expect(draw(activity())).toContain("No skill in the library");
		expect(
			draw(activity({ skill: { name: "", state: "available" } })),
		).toContain("A reviewed skill is available");
	});
});
