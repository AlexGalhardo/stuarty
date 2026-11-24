import { describe, expect, test } from "vitest";

import type { ChatEvent, ChatSession } from "../editor/api/types";
import { lastTool, projectNameOf, sceneOf } from "./sceneState";

function session(over: Partial<ChatSession> = {}): ChatSession {
	return {
		id: "chs_1",
		project_id: "prj_1",
		profile: "claude",
		state: "idle",
		title: "",
		created_at: "",
		updated_at: "",
		event_count: 0,
		live: true,
		...over,
	};
}

let seq = 0;
function event(
	kind: ChatEvent["kind"],
	rest: Partial<ChatEvent> = {},
): ChatEvent {
	seq++;
	return { seq, kind, at: "", ...rest };
}

const ask = (id = "r1", title = "Remove silences"): ChatEvent =>
	event("permission_request", { permission: { request_id: id, title } });
const answer = (id: string, a: "allow" | "reject" | "cancel"): ChatEvent =>
	event("permission_answer", { permission: { request_id: id, answer: a } });
const tool = (
	title: string,
	status: "pending" | "failed" = "pending",
): ChatEvent =>
	event("tool_call", { tool: { tool_call_id: title, title, status } });

describe("sceneOf", () => {
	test("no session is none: no agent, no tool, no request", () => {
		expect(sceneOf(null, [tool("x")])).toEqual({
			variant: "none",
			agent: null,
			state: null,
			endReason: null,
			tool: null,
			request: null,
		});
	});

	test("a session that waits for the person is idle", () => {
		const m = sceneOf(session(), [event("user_message", { text: "hi" })]);
		expect(m.variant).toBe("idle");
		expect(m.agent).toBe("claude");
	});

	test("a turn that runs, or a session that starts, is working", () => {
		expect(sceneOf(session({ state: "running" }), []).variant).toBe("working");
		expect(sceneOf(session({ state: "starting" }), []).variant).toBe("working");
	});

	test("a session that asks is waiting and carries the open request", () => {
		const m = sceneOf(session({ state: "awaiting_permission" }), [
			tool("clip.remove_silences"),
			ask("r9", "Remove 12 silences"),
		]);
		expect(m.variant).toBe("waiting");
		expect(m.request).toEqual({ requestId: "r9", title: "Remove 12 silences" });
		expect(m.tool?.title).toBe("clip.remove_silences");
	});

	test("an answered request is no longer open", () => {
		const m = sceneOf(session({ state: "running" }), [
			ask("r1"),
			answer("r1", "allow"),
		]);
		expect(m.request).toBeNull();
	});

	test("a refusal in this turn is denied; the next message of the person ends it", () => {
		const refused = [event("user_message"), ask("r1"), answer("r1", "reject")];
		expect(sceneOf(session(), refused).variant).toBe("denied");
		expect(
			sceneOf(session(), [...refused, event("user_message")]).variant,
		).toBe("idle");
		expect(
			sceneOf(session(), [
				event("user_message"),
				ask("r2"),
				answer("r2", "cancel"),
			]).variant,
		).toBe("denied");
	});

	test("an allowed request is not a denial", () => {
		const m = sceneOf(session(), [
			event("user_message"),
			ask("r1"),
			answer("r1", "allow"),
		]);
		expect(m.variant).toBe("idle");
	});

	test("a session that ended by choice is done, for any other reason it failed", () => {
		for (const reason of ["user", "idle", "shutdown"]) {
			expect(
				sceneOf(
					session({ state: "ended", live: false, end_reason: reason }),
					[],
				).variant,
			).toBe("done");
		}
		for (const reason of [
			"exited",
			"protocol",
			"timeout",
			"flood",
			"start_failed",
		]) {
			const m = sceneOf(
				session({ state: "ended", live: false, end_reason: reason }),
				[],
			);
			expect(m.variant).toBe("failed");
			expect(m.endReason).toBe(reason);
		}
	});

	test("an ended session offers no request to answer", () => {
		const m = sceneOf(
			session({ state: "ended", live: false, end_reason: "user" }),
			[ask("r1")],
		);
		expect(m.request).toBeNull();
	});
});

describe("projectNameOf", () => {
	const projects = [
		{ id: "prj_1", name: "Launch" },
		{ id: "prj_2", name: "Blog post" },
	];

	test("is the name of the project the session belongs to", () => {
		expect(projectNameOf({ project_id: "prj_2" }, projects)).toBe("Blog post");
	});

	test("is null with no session, or a project the list does not hold", () => {
		expect(projectNameOf(null, projects)).toBeNull();
		expect(projectNameOf({ project_id: "prj_9" }, projects)).toBeNull();
	});
});

describe("lastTool", () => {
	test("is the title and status of the last tool_call, as the service said them", () => {
		expect(lastTool([tool("a"), tool("b", "failed")])).toEqual({
			title: "b",
			status: "failed",
		});
	});

	test("is null when the agent called no tool", () => {
		expect(lastTool([event("agent_message", { text: "hi" })])).toBeNull();
	});
});
