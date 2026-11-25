import { useCallback, useEffect, useRef, useState } from "react";

import { type Failure, failureOf } from "@/components/Failure";
import { createClient } from "../editor/api/client";
import { HttpTransport } from "../editor/api/http";
import type { InboxRow } from "../editor/api/types";
import { parseInbox } from "./notifications";

const STREAM = "/inbox/events";

export interface InboxState {
	// null until the service answered
	rows: InboxRow[] | null;
	// the stream is open: the list updates by itself
	live: boolean;
	failure: Failure | null;
	// the id of the row being answered
	busyId: string | null;
	// Answers a row that waits: a chat permission through the call of the chat (chat.sessions.answer), an approval
	// through the same operation as the Approvals page (approvals.decide). Both are a person's call: the service
	// refuses any other caller, and the stream carries the result back.
	answer(row: InboxRow, choice: "allow" | "reject"): Promise<void>;
}

// What waits for a person, read once and then kept live by the stream of the inbox: the service sends the whole
// list when the stream opens and again each time it changes, so a reconnect catches up by itself. `active`
// false closes the stream (a tab that is not on the screen); made true again, the list is read afresh.
export function useInbox(active = true): InboxState {
	const [rows, setRows] = useState<InboxRow[] | null>(null);
	const [live, setLive] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busyId, setBusyId] = useState<string | null>(null);
	const transport = useRef<HttpTransport | null>(null);
	if (transport.current === null) {
		transport.current = new HttpTransport("/api/v1");
	}

	useEffect(() => {
		const http = transport.current;
		if (http === null || !active) return;
		let alive = true;
		// The stream sends the list first; a read that comes back after it must not put an older one over it.
		let streamed = false;
		createClient(http)
			.listInbox()
			.then((res) => {
				if (alive && !streamed) {
					setRows(res.rows);
					setFailure(null);
				}
			})
			.catch((error: unknown) => {
				if (alive) setFailure(failureOf(error));
			});
		const stream = http.openStream(STREAM, {
			onOpen: () => setLive(true),
			onMessage: (name, data) => {
				if (name !== "inbox.snapshot") return;
				const list = parseInbox(data);
				if (list === null) return;
				streamed = true;
				setRows(list.rows);
				setFailure(null);
			},
			onError: () => setLive(false),
		});
		return () => {
			alive = false;
			stream.close();
			setLive(false);
		};
	}, [active]);

	const answer = useCallback(
		async (row: InboxRow, choice: "allow" | "reject"): Promise<void> => {
			const http = transport.current;
			if (http === null) return;
			setFailure(null);
			setBusyId(row.id);
			try {
				if (row.source === "chat") {
					await createClient(http).answerChatPermission(
						row.session_id,
						row.source_id,
						choice,
					);
				} else {
					await createClient(http).decideApproval(
						row.source_id,
						choice === "allow" ? "approve" : "reject",
					);
				}
			} catch (error) {
				setFailure(failureOf(error));
			} finally {
				setBusyId(null);
			}
		},
		[],
	);

	return { rows, live, failure, busyId, answer };
}
