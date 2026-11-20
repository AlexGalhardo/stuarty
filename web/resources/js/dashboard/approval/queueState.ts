import type { ApprovalRequest } from "../../editor/api/types";

// The queue of requests that wait for a person, kept in step with the events of
// GET /approvals/events (SDD-015 section 7). The service decides what is pending: an event only
// says which request to show or to drop.

// newest first, as the list of the service
export function newestFirst(a: ApprovalRequest, b: ApprovalRequest): number {
	return a.created_at < b.created_at ? 1 : a.created_at > b.created_at ? -1 : 0;
}

export function parseRequest(data: string): ApprovalRequest | null {
	try {
		const value: unknown = JSON.parse(data);
		if (typeof value !== "object" || value === null) return null;
		const request = value as Partial<ApprovalRequest>;
		return typeof request.id === "string" && typeof request.state === "string"
			? (request as ApprovalRequest)
			: null;
	} catch {
		return null;
	}
}

// approval.requested shows the request (once, however often the stream sends it again, as it does
// after a reconnect); approval.decided drops it; every other event is not for the queue.
export function applyApprovalEvent(
	list: readonly ApprovalRequest[],
	name: string | null,
	data: string,
): ApprovalRequest[] {
	if (name !== "approval.requested" && name !== "approval.decided") {
		return [...list];
	}
	const request = parseRequest(data);
	if (request === null) return [...list];
	const others = list.filter((r) => r.id !== request.id);
	if (name === "approval.decided" || request.state !== "pending") return others;
	return [...others, request].sort(newestFirst);
}
