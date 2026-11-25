import type { InboxList, InboxRow } from "../editor/api/types";
import type { Publication } from "../pages/Publish/types";

// What the bell counts and the notification center lists (SDD-012 section 23.4): one list the service builds
// from the permission requests of the hosted chat and from the approval queue (GET /inbox, SDD-015 section 4.1,
// epic 77). Every row is something the service said, already ordered: the requests that wait first, then the
// last answered. The UI computes none of it; it counts the rows that wait and draws the rest.
export type Notification = InboxRow;

// A failed publication target shown in the recent history of the notification center. Every value comes directly
// from GET /publications?state=failed; the UI decides nothing.
export interface FailedPublicationNotice {
	id: string;
	publication_id: string;
	target_id: string;
	title: string;
	connection_id: string;
	network: string;
	message: string;
	hint: string;
	updated_at: string;
}

// Splits the inbox rows the service sent into pending requests and recent history.
export function splitNotifications(rows: readonly Notification[]): {
	waiting: Notification[];
	history: Notification[];
} {
	return {
		waiting: rows.filter((r) => r.state === "waiting"),
		history: rows.filter((r) => r.state !== "waiting"),
	};
}

// Turns the failed targets of a publications read into history items for the notification center, newest first.
export function failedPublicationNotices(
	publications: readonly Publication[],
): FailedPublicationNotice[] {
	return publications
		.flatMap((pub) =>
			pub.targets
				.filter((target) => target.state === "failed")
				.map(
					(target): FailedPublicationNotice => ({
						id: `publication:${target.id}`,
						publication_id: pub.id,
						target_id: target.id,
						title: target.fields.title || pub.title,
						connection_id: target.connection_id,
						network: target.network,
						message: target.error?.message ?? "",
						hint: target.error?.hint ?? "",
						updated_at: target.updated_at,
					}),
				),
		)
		.sort((a, b) => b.updated_at.localeCompare(a.updated_at));
}

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
