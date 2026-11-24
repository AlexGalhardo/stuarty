import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import type { ChatEvent, ChatSession } from "../editor/api/types";
import SceneBar from "./SceneBar";
import { type SceneModel, sceneOf } from "./sceneState";
import { activity, Frame } from "./testSupport";

function session(over: Partial<ChatSession> = {}): ChatSession {
	return {
		id: "chs_1",
		project_id: "prj_1",
		profile: "Claude Code",
		state: "idle",
		title: "",
		created_at: "",
		updated_at: "",
		event_count: 0,
		live: true,
		...over,
	};
}

const events = (...list: Partial<ChatEvent>[]): ChatEvent[] =>
	list.map((e, i) => ({ seq: i + 1, kind: "notice", at: "", ...e }));

function draw(
	model: SceneModel,
	options: { connected?: boolean | null; privileged?: boolean } = {},
): string {
	return renderToString(
		<Frame privileged={options.privileged ?? true}>
			<SceneBar
				model={model}
				connected={options.connected ?? true}
				busy={false}
				onAnswer={() => undefined}
				onOpenChat={() => undefined}
			/>
		</Frame>,
	);
}

const tool = (title = "clip.remove_silences"): Partial<ChatEvent> => ({
	kind: "tool_call",
	tool: { tool_call_id: "t1", title, status: "in_progress" },
});
const ask: Partial<ChatEvent> = {
	kind: "permission_request",
	permission: {
		request_id: "r1",
		title: "Remove 12 silences",
		options: [
			{ option_id: "a", name: "Always allow", kind: "allow_always" },
			{ option_id: "b", name: "Allow once", kind: "allow_once" },
			{ option_id: "c", name: "Reject", kind: "reject_once" },
		],
	},
};
const refusal = (answer: "reject" | "cancel"): Partial<ChatEvent> => ({
	kind: "permission_answer",
	permission: { request_id: "r1", answer },
});

describe("SceneBar variants", () => {
	test("none: says there is no session and invents no skill, tool or step", () => {
		const html = draw(sceneOf(null, []));
		expect(html).toContain('data-variant="none"');
		expect(html).toContain("No agent session");
		expect(html).not.toContain('data-testid="scene-skill"');
		expect(html).not.toContain('data-testid="scene-tool"');
		expect(html).not.toContain("scene-allow");
		expect(html).toContain("Open chat");
	});

	test("none with no agent connected offers to connect one", () => {
		expect(draw(sceneOf(null, []), { connected: false })).toContain(
			"Connect an agent",
		);
	});

	test("idle: the agent and its state, the skill and the tool say Not reported until the service answers", () => {
		const html = draw(sceneOf(session(), events({ kind: "user_message" })));
		expect(html).toContain('data-variant="idle"');
		expect(html).toContain("Claude Code");
		expect(html).toContain("Waiting for you");
		expect(html).toMatch(/data-testid="scene-skill".*Not reported/s);
		expect(html).toMatch(/data-testid="scene-tool".*Not reported/s);
		expect(html).not.toContain("scene-allow");
	});

	test("idle: with an activity and no tool call, the tool slot says so and the skill is none", () => {
		const html = draw(
			sceneOf(session(), events({ kind: "user_message" }), activity()),
		);
		expect(html).toContain("No tool called yet");
		expect(html).toMatch(/data-testid="scene-skill".*none loaded/s);
		expect(html).toContain("No skill in the library");
		expect(html).not.toContain("Not reported");
	});

	test("a reviewed skill that was not read is available, not loaded", () => {
		const html = draw(
			sceneOf(
				session(),
				[],
				activity({ skill: { name: "", state: "available" } }),
			),
		);
		expect(html).toContain("A reviewed skill is available");
		expect(html).not.toContain("loaded in this session");
	});

	test("a skill the agent read is loaded in this session, named, never in use", () => {
		const html = draw(
			sceneOf(
				session({ state: "running" }),
				[],
				activity({ skill: { name: "captions", state: "loaded" } }),
			),
		);
		expect(html).toMatch(
			/data-testid="scene-skill".*Skill · loaded in this session.*captions/s,
		);
		expect(html).not.toMatch(/in use/i);
	});

	test("the name of a skill is text: markup in it is not markup", () => {
		const html = draw(
			sceneOf(
				session({ state: "running" }),
				[],
				activity({ skill: { name: "<img src=x onerror=1>", state: "loaded" } }),
			),
		);
		expect(html).toContain("&lt;img src=x onerror=1&gt;");
		expect(html).not.toContain("<img src=x");
	});

	test("a tool of Stuarty shows its plain description and its technical name", () => {
		const html = draw(
			sceneOf(
				session({ state: "running" }),
				[],
				activity({
					tool: {
						title: "mcp__stuarty__library",
						technical_name: "library",
						action: "get",
						description_key: "ui.agent_activity.action.library_get",
						status: "in_progress",
						from_agent: false,
					},
				}),
			),
		);
		expect(html).toMatch(
			/data-testid="scene-tool".*Tool · library · running.*Reads one note, skill or prompt\./s,
		);
		expect(html).not.toContain("text from the agent");
	});

	test("working: any other tool shows the title of the call as text from the agent", () => {
		const html = draw(
			sceneOf(
				session({ state: "running" }),
				events(tool()),
				activity({
					tool: {
						title: "clip.remove_silences",
						status: "in_progress",
						from_agent: true,
					},
				}),
			),
		);
		expect(html).toContain('data-variant="working"');
		expect(html).toContain("Working");
		expect(html).toContain("clip.remove_silences");
		expect(html).toContain("text from the agent");
		expect(html).toContain("running");
		expect(html).not.toContain("scene-allow");
	});

	test("working: markup in a tool title stays text", () => {
		const html = draw(
			sceneOf(
				session({ state: "running" }),
				[],
				activity({
					tool: {
						title: "<img src=x onerror=1>",
						status: "in_progress",
						from_agent: true,
					},
				}),
			),
		);
		expect(html).toContain("&lt;img src=x onerror=1&gt;");
		expect(html).not.toContain("<img src=x");
	});

	test("waiting: Allow once and Deny, and no always", () => {
		const html = draw(
			sceneOf(session({ state: "awaiting_permission" }), events(tool(), ask)),
		);
		expect(html).toContain('data-variant="waiting"');
		expect(html).toContain("Asks your permission");
		expect(html).toContain('data-testid="scene-allow"');
		expect(html).toContain('data-testid="scene-deny"');
		expect(html).toContain("Allow once");
		expect(html).not.toMatch(/always/i);
	});

	test("waiting is announced to a reader, with the request", () => {
		const html = draw(
			sceneOf(session({ state: "awaiting_permission" }), events(ask)),
		);
		expect(html).toMatch(
			/role="status"[^>]*data-testid="scene-announce"[^>]*>Claude Code: Asks your permission: Remove 12 silences</,
		);
	});

	test("a session that is not a person sees Allow and Deny disabled, with the reason", () => {
		const html = draw(
			sceneOf(session({ state: "awaiting_permission" }), events(ask)),
			{ privileged: false },
		);
		expect(html).toMatch(/<button[^>]*disabled[^>]*data-testid="scene-allow"/);
		expect(html).toMatch(/<button[^>]*disabled[^>]*data-testid="scene-deny"/);
		expect(html).toContain("aria-describedby=");
		// the reason takes a line of its own, so the bar may grow past one row
		expect(html).toContain("basis-full");
		expect(html).not.toContain("md:flex-nowrap");
	});

	test("waiting, for a person: one row of 56 px, with no full-width line to crush the actions", () => {
		const html = draw(
			sceneOf(session({ state: "awaiting_permission" }), events(ask)),
		);
		expect(html).toContain("md:h-14 md:flex-nowrap");
		expect(html).not.toContain("basis-full");
	});

	test("denied: says so and offers no action on a request", () => {
		const html = draw(
			sceneOf(
				session(),
				events({ kind: "user_message" }, ask, refusal("reject")),
			),
		);
		expect(html).toContain('data-variant="denied"');
		expect(html).toContain("You denied it");
		expect(html).not.toContain("scene-allow");
	});

	test("done: a session ended by the person", () => {
		const html = draw(
			sceneOf(session({ state: "ended", live: false, end_reason: "user" }), []),
		);
		expect(html).toContain('data-variant="done"');
		expect(html).toContain("Finished");
	});

	test("failed: says why, and is announced", () => {
		const html = draw(
			sceneOf(
				session({ state: "ended", live: false, end_reason: "timeout" }),
				[],
			),
		);
		expect(html).toContain('data-variant="failed"');
		expect(html).toContain("Failed");
		expect(html).toContain("it took too long");
		expect(html).toMatch(
			/data-testid="scene-announce"[^>]*>Claude Code: Failed/,
		);
	});

	test("only a waiting for a person and a failure are announced", () => {
		for (const state of ["running", "idle"] as const) {
			const html = draw(sceneOf(session({ state }), events(tool())));
			expect(html).toMatch(/data-testid="scene-announce"[^>]*><\/p>/);
		}
	});

	test("every variant tells its state with a word, not a colour", () => {
		const words: Record<string, string> = {
			none: "No agent session",
			idle: "Waiting for you",
			working: "Working",
			waiting: "Asks your permission",
			denied: "You denied it",
			done: "Finished",
			failed: "Failed",
		};
		const cases: [string, SceneModel][] = [
			["none", sceneOf(null, [])],
			["idle", sceneOf(session(), [])],
			["working", sceneOf(session({ state: "running" }), [])],
			[
				"waiting",
				sceneOf(session({ state: "awaiting_permission" }), events(ask)),
			],
			[
				"denied",
				sceneOf(
					session(),
					events({ kind: "user_message" }, ask, refusal("cancel")),
				),
			],
			["done", sceneOf(session({ state: "ended", end_reason: "idle" }), [])],
			[
				"failed",
				sceneOf(session({ state: "ended", end_reason: "exited" }), []),
			],
		];
		for (const [variant, model] of cases) {
			const html = draw(model);
			expect(html).toContain(`data-variant="${variant}"`);
			expect(html).toContain(words[variant] ?? "?");
		}
	});

	test("names the project of the session, as untrusted text, on the bar and to a reader", () => {
		const html = renderToString(
			<Frame>
				<SceneBar
					model={sceneOf(
						session({ state: "awaiting_permission" }),
						events(ask),
					)}
					project="<b>Launch</b>"
					connected
					busy={false}
					onAnswer={() => undefined}
					onOpenChat={() => undefined}
				/>
			</Frame>,
		);
		expect(html).toContain('data-testid="scene-project"');
		expect(html).toContain("&lt;b&gt;Launch&lt;/b&gt;");
		expect(html).not.toContain("<b>Launch");
		expect(html).toContain(
			"Claude Code (&lt;b&gt;Launch&lt;/b&gt;): Asks your permission",
		);
	});

	test("with no project name it names none", () => {
		const html = draw(sceneOf(session(), []));
		expect(html).not.toContain("scene-project");
	});

	test("is a labelled region", () => {
		expect(draw(sceneOf(null, []))).toMatch(
			/<section aria-label="Agent status" data-testid="scene-bar"/,
		);
	});
});
