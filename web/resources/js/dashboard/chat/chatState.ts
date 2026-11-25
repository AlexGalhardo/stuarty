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

// A value that changes when the activity of the session may have changed: a skill was read, or the last tool
// call is another one or moved on. It only says when to ask the service again; what the activity is, the
// service tells (SDD-017 section 8.1).
export function activityStamp(events: readonly ChatEvent[]): string {
	let skills = 0;
	let tool = "";
	for (const e of events) {
		if (e.kind === "skill_loaded") skills++;
		if (e.kind === "tool_call" && e.tool !== undefined) {
			tool = `${e.seq}|${e.tool.status}|${e.tool.title}|${e.tool.action ?? ""}`;
		}
	}
	return `${skills}#${tool}`;
}

// The project the chat talks about once the list of projects is known (VV-427): the one it has when the service
// lists it, else the one the person picked last, else the first. A project that is not in the list (trashed, or
// deleted while the page of another one was open) is never kept, or the select would show the first option while
// the session was read for an id the service no longer has.
export function chooseProject(
	projects: readonly { id: string }[],
	current: string | null,
	stored: string | null,
): string | null {
	const listed = (id: string | null): id is string =>
		id !== null && projects.some((p) => p.id === id);
	if (listed(current)) return current;
	if (listed(stored)) return stored;
	return projects[0]?.id ?? null;
}
