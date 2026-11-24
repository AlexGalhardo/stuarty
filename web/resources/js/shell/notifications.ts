import { openRequests } from "../dashboard/chat/chatState";
import type {
	ApprovalRequest,
	ChatEvent,
	ChatSession,
} from "../editor/api/types";

// What the bell counts and the notification center lists (SDD-012 section 23.4): the open permission
// requests of the chat session the client knows and the pending approvals the queue reports. Every row
// is something the service said; a row is a request that waits for a person, or one a person answered.
// Epic 77 replaces both reads with one read model.
export type NotificationState = "waiting" | "allowed" | "denied";

export interface PermissionNotification {
	kind: "permission";
	// the key of the row: the request id of the agent
	id: string;
	state: NotificationState;
	// the profile that runs the session, and the title of the tool that asks: untrusted text
	agent: string;
	title: string;
	sessionId: string;
	at: string;
}

export interface ApprovalNotification {
	kind: "approval";
	id: string;
	state: "waiting";
	// who asked and what, as the queue holds them: untrusted text
	agent: string;
	title: string;
	at: string;
}

export type Notification = PermissionNotification | ApprovalNotification;

// How many answered requests the center keeps in view under the ones that wait.
const HISTORY = 5;

export function notificationsOf(
	session: ChatSession | null,
	events: readonly ChatEvent[],
	approvals: readonly ApprovalRequest[] | null,
): Notification[] {
	const waiting: Notification[] = [];
	const answered: PermissionNotification[] = [];

	if (session !== null) {
		const open = new Set(
			session.state === "ended"
				? []
				: openRequests(events).map((e) => e.permission?.request_id ?? ""),
		);
		const answers = new Map<string, string>();
		for (const e of events) {
			if (e.kind === "permission_answer" && e.permission !== undefined) {
				answers.set(e.permission.request_id, e.permission.answer ?? "cancel");
			}
		}
		for (const e of events) {
			if (e.kind !== "permission_request" || e.permission === undefined)
				continue;
			const id = e.permission.request_id;
			const row = {
				kind: "permission" as const,
				id,
				agent: session.profile,
				title: e.permission.title ?? "",
				sessionId: session.id,
				at: e.at,
			};
			if (open.has(id)) waiting.push({ ...row, state: "waiting" });
			else if (answers.has(id)) {
				answered.push({
					...row,
					state: answers.get(id) === "allow" ? "allowed" : "denied",
				});
			}
		}
	}
	for (const request of approvals ?? []) {
		waiting.push({
			kind: "approval",
			id: request.id,
			state: "waiting",
			agent: request.actor,
			title: request.summary,
			at: request.created_at,
		});
	}
	const newestFirst = (a: Notification, b: Notification): number =>
		a.at < b.at ? 1 : a.at > b.at ? -1 : 0;
	return [
		...waiting.sort(newestFirst),
		...answered.sort(newestFirst).slice(0, HISTORY),
	];
}

// The number on the bell: what waits for a person.
export function waitingCount(list: readonly Notification[]): number {
	return list.filter((n) => n.state === "waiting").length;
}
