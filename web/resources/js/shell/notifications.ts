import type { InboxList, InboxRow } from "../editor/api/types";

// What the bell counts and the notification center lists (SDD-012 section 23.4): one list the service builds
// from the permission requests of the hosted chat and from the approval queue (GET /inbox, SDD-015 section 4.1,
// epic 77). Every row is something the service said, already ordered: the requests that wait first, then the
// last answered. The UI computes none of it; it counts the rows that wait and draws the rest.
export type Notification = InboxRow;

// The number on the bell: what waits for a person.
export function waitingCount(rows: readonly Notification[]): number {
	return rows.filter((r) => r.state === "waiting").length;
}

// How many approvals wait, for the count of the Approvals item of the sidebar; 0 until the list was read.
export function approvalsWaiting(rows: readonly Notification[] | null): number {
	return (rows ?? []).filter(
		(r) => r.source === "approval" && r.state === "waiting",
	).length;
}

// The list the service sent, or null when the data is not one (a frame that is not JSON, or has another
// shape, is dropped and the last list stays).
export function parseInbox(data: string): InboxList | null {
	let value: unknown;
	try {
		value = JSON.parse(data);
	} catch {
		return null;
	}
	if (typeof value !== "object" || value === null) return null;
	const rows = (value as { rows?: unknown }).rows;
	if (!Array.isArray(rows)) return null;
	return value as InboxList;
}

// The chat request of a session that waits for a person, as the list holds it: the scene bar answers it. A row
// of an approval is never one: it is decided in Approvals.
export function waitingChatRequest(
	rows: readonly Notification[] | null,
	sessionId: string,
): Notification | null {
	return (
		rows?.find(
			(r) =>
				r.source === "chat" &&
				r.state === "waiting" &&
				r.session_id === sessionId,
		) ?? null
	);
}
