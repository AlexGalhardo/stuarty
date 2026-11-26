import { useCallback, useEffect, useRef, useState } from "react";

import { type Failure, failureOf } from "@/components/Failure";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { ApprovalRequest } from "../../editor/api/types";
import { applyApprovalEvent, newestFirst } from "./queueState";

const STREAM = "/approvals/events";

export interface ApprovalQueueState {
	// null until the first answer
	requests: ApprovalRequest[] | null;
	// the stream is open: the list updates by itself
	live: boolean;
	failure: Failure | null;
	busyId: string | null;
	decide(
		id: string,
		decision: "approve" | "reject",
		reason?: string,
	): Promise<void>;
}

// The pending requests, read once and then kept live by the event stream. The stream sends the
// pending requests when it opens, so a reconnect catches up by itself. Deciding is a call of the
// person; the answer and the event both drop the request, and dropping twice changes nothing.
// `active` false closes the stream (the shell, in a tab that is not on the screen); made true again, the
// queue is read afresh, so a request decided meanwhile does not stay.
export function useApprovalQueue(active = true): ApprovalQueueState {
	const [requests, setRequests] = useState<ApprovalRequest[] | null>(null);
	const [live, setLive] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busyId, setBusyId] = useState<string | null>(null);
	const transport = useRef<HttpTransport | null>(null);
	if (transport.current === null) {
		transport.current = new HttpTransport("/api/v1");
	}

	const started = useRef(false);
	const decidedRef = useRef<Set<string>>(new Set());
	useEffect(() => {
		const http = transport.current;
		if (http === null || !active) return;
		// a second start follows a pause: what was held may have been decided since
		if (started.current) setRequests(null);
		started.current = true;
		let alive = true;
		const decided = new Set<string>();
		decidedRef.current = decided;
		createClient(http)
			.listApprovals("pending")
			.then((res) => {
				// the stream may already have shown newer requests or removed decided ones
				if (alive) {
					setRequests((was) => mergeList(was ?? [], res, decided));
				}
			})
			.catch((error: unknown) => {
				if (alive) setFailure(failureOf(error));
			});
		const stream = http.openStream(STREAM, {
			onOpen: () => setLive(true),
			onMessage: (name, data) => {
				if (name === "approval.decided" || name === "approval.expired") {
					try {
						const parsed = JSON.parse(data) as {
							approval?: { id?: string };
							id?: string;
						};
						const id = parsed.approval?.id ?? parsed.id;
						if (typeof id === "string") decided.add(id);
					} catch {
						// ignore malformed payload; applyApprovalEvent handles it
					}
				}
				setRequests((was) => applyApprovalEvent(was ?? [], name, data));
			},
			onError: () => setLive(false),
		});
		return () => {
			alive = false;
			stream.close();
			setLive(false);
		};
	}, [active]);

	const decide = useCallback(
		async (
			id: string,
			decision: "approve" | "reject",
			reason?: string,
		): Promise<void> => {
			const http = transport.current;
			if (http === null) return;
			setFailure(null);
			setBusyId(id);
			try {
				await createClient(http).decideApproval(id, decision, reason);
				decidedRef.current.add(id);
				setRequests((was) => (was ?? []).filter((r) => r.id !== id));
			} catch (error) {
				setFailure(failureOf(error));
			} finally {
				setBusyId(null);
			}
		},
		[],
	);

	return { requests, live, failure, busyId, decide };
}

function mergeList(
	was: readonly ApprovalRequest[],
	res: { requests: ApprovalRequest[] },
	decided: ReadonlySet<string>,
): ApprovalRequest[] {
	const seen = new Set(was.map((r) => r.id));
	return [
		...was,
		...res.requests.filter((r) => !seen.has(r.id) && !decided.has(r.id)),
	].sort(newestFirst);
}
