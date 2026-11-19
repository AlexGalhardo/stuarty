import type { ChatEvent, ChatState } from "../../editor/api/types";

// What the panel keeps of a session: the events by number. It joins what the server says; it
// decides nothing (rule of SDD-012 section 1). A frame with `append` continues the text of the
// event that has the same number; any other frame replaces or adds the event.
export function applyChatEvent(
	events: readonly ChatEvent[],
	frame: ChatEvent,
): ChatEvent[] {
	const at = events.findIndex((e) => e.seq === frame.seq);
	if (at === -1) {
		return insertSorted(events, { ...frame, append: undefined });
	}
	const next = events.slice();
	const was = events[at];
	next[at] =
		frame.append === true && was !== undefined
			? { ...was, text: (was.text ?? "") + (frame.text ?? "") }
			: { ...frame, append: undefined };
	return next;
}

function insertSorted(
	events: readonly ChatEvent[],
	frame: ChatEvent,
): ChatEvent[] {
	const next = events.slice();
	let i = next.length;
	while (i > 0 && (next[i - 1]?.seq ?? 0) > frame.seq) i--;
	next.splice(i, 0, frame);
	return next;
}

// The snapshot a stream opens with replaces what the panel had, so a reconnect cannot show an
// event twice or leave a hole.
export function applySnapshot(
	events: readonly ChatEvent[],
	frames: readonly ChatEvent[],
): ChatEvent[] {
	let next = events.slice();
	for (const f of frames) next = applyChatEvent(next, f);
	return next;
}

// The state of the session is the one the last state event says.
export function stateOf(events: readonly ChatEvent[]): ChatState | null {
	for (let i = events.length - 1; i >= 0; i--) {
		const e = events[i];
		if (e?.kind === "state" && e.state !== undefined) return e.state.state;
	}
	return null;
}

// The permission requests that nobody answered yet: a request has an answer event with the same
// request id once it was answered.
export function openRequests(events: readonly ChatEvent[]): ChatEvent[] {
	const answered = new Set<string>();
	for (const e of events) {
		if (e.kind === "permission_answer" && e.permission !== undefined) {
			answered.add(e.permission.request_id);
		}
	}
	return events.filter(
		(e) =>
			e.kind === "permission_request" &&
			e.permission !== undefined &&
			!answered.has(e.permission.request_id),
	);
}

// Events the panel does not draw as a line of the conversation.
export function isVisible(e: ChatEvent): boolean {
	return e.kind !== "state";
}

// Whether the person may send now.
export function canSend(state: ChatState | null): boolean {
	return state === "idle";
}

// Whether a turn runs, so that a cancel makes sense.
export function isBusy(state: ChatState | null): boolean {
	return state === "running" || state === "awaiting_permission";
}
