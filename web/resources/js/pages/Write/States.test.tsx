import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { activity, Frame } from "../../shell/testSupport";
import { AgentContext, type Reading } from "./AgentPanel";
import type { DraftJob, FlowStep, TextItem } from "./client";
import { DraftStatus } from "./DraftPanel";
import EditorState from "./EditorState";
import TextList, { type Loaded } from "./TextList";

function item(over: Partial<TextItem> = {}): TextItem {
	return {
		id: "itm_1",
		project_id: "prj_1",
		title: "Launch copy",
		text_kind: "copy",
		state: "active",
		words: 40,
		revision: 3,
		updated_at: "2026-11-24T10:00:00Z",
		...over,
	};
}

function list(
	loaded: Loaded,
	filtered = false,
	currentId: string | null = null,
) {
	return renderToString(
		<Frame>
			<TextList
				loaded={loaded}
				currentId={currentId}
				filtered={filtered}
				onPick={() => {}}
				onRetry={() => {}}
				onMore={() => {}}
			/>
		</Frame>,
	);
}

describe("the list of texts", () => {
	test("loading says so in words, in a polite region", () => {
		const html = list({ state: "loading" });
		expect(html).toContain("Loading.");
		expect(html).toContain('role="status"');
	});

	test("failed is an alert with a way to try again", () => {
		const html = list({ state: "failed" });
		expect(html).toContain('role="alert"');
		expect(html).toContain("The texts could not be loaded.");
		expect(html).toContain("Try again");
	});

	test("empty tells what a text is and names no video", () => {
		const html = list({ state: "ready", texts: [] });
		expect(html).toContain("No texts yet.");
		expect(html).toContain("post, a copy, a script or a caption");
		for (const word of ["video", "clip", "timeline"]) {
			expect(html.toLowerCase()).not.toContain(word);
		}
	});

	test("empty because of the filters says so and points at the filters", () => {
		const html = list({ state: "ready", texts: [] }, true);
		expect(html).toContain("No text matches.");
		expect(html).toContain("Change the project or the kind above");
	});

	test("a row names its kind, revision and words, marks the open one, and the title is text", () => {
		const html = list(
			{
				state: "ready",
				texts: [
					item({ title: "<img src=x onerror=1>" }),
					item({ id: "itm_2", title: "Other", text_kind: "post" }),
				],
				next: "cursor",
			},
			false,
			"itm_2",
		);
		expect(html).not.toContain("<img");
		expect(html).toContain("&lt;img src=x onerror=1&gt;");
		expect(html).toContain("Revision 3, 40 words");
		expect(html).toContain("Blog post");
		expect(html.match(/aria-current="true"/g)?.length).toBe(1);
		expect(html).toContain("More");
	});
});

describe("the state of the editor before a text", () => {
	test("loading says so", () => {
		const html = renderToString(
			<Frame>
				<EditorState failure={null} onRetry={() => {}} />
			</Frame>,
		);
		expect(html).toContain("Loading the text.");
		expect(html).not.toContain("Read the text again");
	});

	test("a failure is shown as the service gave it, with a button to read again", () => {
		const html = renderToString(
			<Frame>
				<EditorState
					failure={{ code: "E_NOT_FOUND", message: "no such <b>text</b>" }}
					onRetry={() => {}}
				/>
			</Frame>,
		);
		expect(html).not.toContain("<b>");
		expect(html).toContain("Read the text again");
		expect(html).not.toContain("Loading the text.");
	});
});

function context(reading: Reading, step?: FlowStep): string {
	return renderToString(
		<Frame>
			<AgentContext reading={reading} step={step} onAgain={() => {}} />
		</Frame>,
	);
}

describe("the agent context", () => {
	test("while it reads, the skill and the tool are not reported and the sentence says it reads", () => {
		const html = context({ state: "loading" });
		expect(html).toContain("Reading what the agent is doing.");
		expect(html).toMatch(/chat-activity-skill.*Not reported/s);
	});

	test("with no session it says none started", () => {
		const html = context({ state: "ready", activity: null });
		expect(html).toContain("No agent session has started in this project yet.");
		expect(html).toContain("Read again");
	});

	test("a failed read is an alert with a retry", () => {
		const html = context({ state: "failed" });
		expect(html).toContain('role="alert"');
		expect(html).toContain("could not be read");
		expect(html).toContain("Try again");
	});

	test("a loaded skill and the tool of the session are shown, and the agent's words stay text", () => {
		const html = context({
			state: "ready",
			activity: activity({
				skill: { name: "<b>blog-post</b>", state: "loaded" },
				tool: {
					title: "<img src=x onerror=1>",
					status: "in_progress",
					from_agent: true,
				},
			}),
		});
		expect(html).toMatch(/chat-activity-skill.*loaded in this session/s);
		expect(html).not.toContain("<b>");
		expect(html).not.toContain("<img");
		expect(html).toContain("&lt;b&gt;blog-post&lt;/b&gt;");
	});

	test("the step of the path says who does it and with which operation", () => {
		const html = context(
			{ state: "ready", activity: null },
			{
				id: "review",
				state: "waiting",
				optional: false,
				actor: "person",
				tool: "PostCheck",
			},
		);
		expect(html).toContain("Now in the path: Review. You do it.");
		expect(html).toContain("operation PostCheck");
	});
});

function status(
	job: DraftJob | null,
	estimate: DraftJob | null = null,
): string {
	return renderToString(
		<Frame>
			<DraftStatus job={job} estimate={estimate} />
		</Frame>,
	);
}

const money = { micros: 50, currency: "USD" };

describe("the state of a draft", () => {
	test("nothing is said before a draft is asked for", () => {
		expect(status(null)).not.toContain("<p");
	});

	test("the estimate comes first", () => {
		expect(
			status(null, { job_id: "j", state: "estimated", estimate: money }),
		).toContain("Estimated cost: $0.00005.");
	});

	test("a draft that waits is a notice that names the approval queue and says the model writes nothing yet", () => {
		const html = status({
			job_id: "j",
			state: "waiting_approval",
			estimate: money,
		});
		expect(html).toContain("Waiting for your approval.");
		expect(html).toContain("writes nothing until you approve");
		expect(html).toContain('href="/approvals"');
	});

	test("a draft that runs says the model is writing", () => {
		expect(status({ job_id: "j", state: "running" })).toContain(
			"The model is writing.",
		);
	});

	test("a draft that ends names the revision and the cost", () => {
		const html = status({
			job_id: "j",
			state: "succeeded",
			result: {
				text: {
					provider: "openai",
					model: "m",
					revision: 5,
					cost: money,
				},
			},
		});
		expect(html).toContain("The draft is revision 5. It cost $0.00005.");
	});

	test("a draft that fails is an alert, and the provider's words are text", () => {
		const html = status({
			job_id: "j",
			state: "failed",
			error: { code: "E_X", message: "<script>alert(1)</script>" },
		});
		expect(html).toContain('role="alert"');
		expect(html).not.toContain("<script>");
		expect(html).toContain("&lt;script&gt;");
	});
});
