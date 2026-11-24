import { describe, expect, test } from "vitest";

import type {
	ApprovalRequest,
	ChatEvent,
	ChatSession,
} from "../editor/api/types";
import { notificationsOf, waitingCount } from "./notifications";

const session = (over: Partial<ChatSession> = {}): ChatSession => ({
	id: "chs_1",
	project_id: "prj_1",
	profile: "Claude Code",
	state: "awaiting_permission",
	title: "",
	created_at: "",
	updated_at: "",
	event_count: 0,
	live: true,
	...over,
});

let seq = 0;
const ask = (
	id: string,
	title: string,
	at = "2026-01-01T10:00:00Z",
): ChatEvent => ({
	seq: ++seq,
	kind: "permission_request",
	at,
	permission: {
		request_id: id,
		title,
		options: [{ option_id: "x", name: "Always allow", kind: "allow_always" }],
	},
});
const answer = (id: string, a: "allow" | "reject" | "cancel"): ChatEvent => ({
	seq: ++seq,
	kind: "permission_answer",
	at: "2026-01-01T10:05:00Z",
	permission: { request_id: id, answer: a },
});

const approval = (id: string, at: string): ApprovalRequest => ({
	id,
	actor: "agent:claude",
	kind: "publish.post",
	summary: `Publish ${id}`,
	connection_id: null,
	project_id: null,
	job_id: null,
	payload_ref: "",
	estimate: { micros: 0, currency: "" } as ApprovalRequest["estimate"],
	state: "pending",
	decided_by: null,
	rule_id: null,
	created_at: at,
	decided_at: null,
	expires_at: "",
});

describe("notificationsOf", () => {
	test("nothing known is nothing listed", () => {
		expect(notificationsOf(null, [], null)).toEqual([]);
		expect(notificationsOf(null, [], [])).toEqual([]);
	});

	test("an open permission waits, and carries the agent and the title as the service said them", () => {
		const list = notificationsOf(
			session(),
			[ask("r1", "Remove silences")],
			null,
		);
		expect(list).toEqual([
			{
				kind: "permission",
				id: "r1",
				state: "waiting",
				agent: "Claude Code",
				title: "Remove silences",
				sessionId: "chs_1",
				at: "2026-01-01T10:00:00Z",
			},
		]);
		expect(waitingCount(list)).toBe(1);
	});

	test("a pending approval waits and leads to Approvals", () => {
		const list = notificationsOf(
			null,
			[],
			[approval("apr_1", "2026-01-01T09:00:00Z")],
		);
		expect(list).toHaveLength(1);
		expect(list[0]).toMatchObject({
			kind: "approval",
			state: "waiting",
			title: "Publish apr_1",
		});
		expect(waitingCount(list)).toBe(1);
	});

	test("the count is the open permissions plus the pending approvals", () => {
		const list = notificationsOf(
			session(),
			[ask("r1", "a"), ask("r2", "b"), answer("r1", "allow")],
			[
				approval("apr_1", "2026-01-01T09:00:00Z"),
				approval("apr_2", "2026-01-01T09:30:00Z"),
			],
		);
		expect(waitingCount(list)).toBe(3);
	});

	test("an answered request is history, as allowed or denied", () => {
		const list = notificationsOf(
			session({ state: "idle" }),
			[
				ask("r1", "a"),
				answer("r1", "allow"),
				ask("r2", "b"),
				answer("r2", "reject"),
				ask("r3", "c"),
				answer("r3", "cancel"),
			],
			null,
		);
		expect(list.map((n) => [n.id, n.state])).toEqual(
			[
				["r1", "allowed"],
				["r2", "denied"],
				["r3", "denied"],
			].sort((a, b) => (a[0] as string).localeCompare(b[0] as string)),
		);
		expect(waitingCount(list)).toBe(0);
	});

	test("what waits comes before what was answered, newest first", () => {
		const list = notificationsOf(
			session(),
			[
				ask("old", "a", "2026-01-01T08:00:00Z"),
				answer("old", "allow"),
				ask("r1", "b", "2026-01-01T10:00:00Z"),
			],
			[approval("apr_1", "2026-01-01T11:00:00Z")],
		);
		expect(list.map((n) => n.id)).toEqual(["apr_1", "r1", "old"]);
	});

	test("a session that ended leaves no request to answer", () => {
		const list = notificationsOf(
			session({ state: "ended", live: false }),
			[ask("r1", "a")],
			null,
		);
		expect(waitingCount(list)).toBe(0);
	});

	test("keeps the last five answered ones only", () => {
		const events: ChatEvent[] = [];
		for (let i = 0; i < 8; i++)
			events.push(ask(`r${i}`, "t"), answer(`r${i}`, "allow"));
		expect(
			notificationsOf(session({ state: "idle" }), events, null),
		).toHaveLength(5);
	});

	test("the standing options of an agent never become a row or an action: a row has no options", () => {
		const [row] = notificationsOf(session(), [ask("r1", "t")], null);
		expect(JSON.stringify(row)).not.toMatch(/always/i);
	});
});
