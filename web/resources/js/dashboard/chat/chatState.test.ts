import { describe, expect, it } from "vitest";

import type { ChatEvent } from "../../editor/api/types";
import {
	applyChatEvent,
	applySnapshot,
	canSend,
	isBusy,
	isVisible,
	openRequests,
	stateOf,
} from "./chatState";

const at = "2026-11-19T12:00:00Z";

const message = (seq: number, text: string, append = false): ChatEvent => ({
	seq,
	kind: "agent_message",
	at,
	text,
	...(append ? { append: true } : {}),
});

describe("applyChatEvent", () => {
	it("adds an event in order of its number", () => {
		let events: ChatEvent[] = [];
		events = applyChatEvent(events, message(3, "c"));
		events = applyChatEvent(events, message(1, "a"));
		events = applyChatEvent(events, message(2, "b"));
		expect(events.map((e) => e.seq)).toEqual([1, 2, 3]);
	});

	it("continues the text of the event a chunk names", () => {
		let events = applyChatEvent([], message(1, "Hel"));
		events = applyChatEvent(events, message(1, "lo ", true));
		events = applyChatEvent(events, message(1, "world", true));
		expect(events).toHaveLength(1);
		expect(events[0]?.text).toBe("Hello world");
	});

	it("replaces an event that arrives whole again, as a tool call does", () => {
		const call = (status: "pending" | "completed"): ChatEvent => ({
			seq: 4,
			kind: "tool_call",
			at,
			tool: { tool_call_id: "c", title: "Read", status },
		});
		let events = applyChatEvent([], call("pending"));
		events = applyChatEvent(events, call("completed"));
		expect(events).toHaveLength(1);
		expect(events[0]?.tool?.status).toBe("completed");
	});

	it("never mutates what it was given", () => {
		const first = applyChatEvent([], message(1, "a"));
		const frozen = JSON.stringify(first);
		applyChatEvent(first, message(1, "b", true));
		expect(JSON.stringify(first)).toBe(frozen);
	});

	it("keeps markup as text", () => {
		const events = applyChatEvent([], message(1, "<img src=x onerror=1>"));
		expect(events[0]?.text).toBe("<img src=x onerror=1>");
	});
});

describe("applySnapshot", () => {
	it("does not show an event twice when the stream reconnects", () => {
		const first = applySnapshot([], [message(1, "a"), message(2, "b")]);
		const again = applySnapshot(first, [message(1, "a"), message(2, "bc")]);
		expect(again.map((e) => e.text)).toEqual(["a", "bc"]);
	});
});

describe("session state", () => {
	const state = (seq: number, s: "idle" | "running" | "ended"): ChatEvent => ({
		seq,
		kind: "state",
		at,
		state: { state: s },
	});

	it("is the last state event", () => {
		expect(stateOf([state(1, "idle"), state(2, "running")])).toBe("running");
		expect(stateOf([])).toBeNull();
	});

	it("says when the person may send and when a cancel makes sense", () => {
		expect(canSend("idle")).toBe(true);
		expect(canSend("running")).toBe(false);
		expect(canSend(null)).toBe(false);
		expect(isBusy("running")).toBe(true);
		expect(isBusy("awaiting_permission")).toBe(true);
		expect(isBusy("idle")).toBe(false);
	});

	it("draws no line for a state event", () => {
		expect(isVisible(state(1, "idle"))).toBe(false);
		expect(isVisible(message(1, "a"))).toBe(true);
	});
});

describe("openRequests", () => {
	const ask = (seq: number, id: string): ChatEvent => ({
		seq,
		kind: "permission_request",
		at,
		permission: { request_id: id, title: "Delete" },
	});
	const answer = (seq: number, id: string): ChatEvent => ({
		seq,
		kind: "permission_answer",
		at,
		permission: { request_id: id, answer: "allow" },
	});

	it("lists the requests nobody answered", () => {
		const events = [ask(1, "cpr_a"), ask(2, "cpr_b"), answer(3, "cpr_a")];
		expect(openRequests(events).map((e) => e.permission?.request_id)).toEqual([
			"cpr_b",
		]);
	});
});
