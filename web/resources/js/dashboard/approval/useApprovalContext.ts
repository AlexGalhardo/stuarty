import { useEffect, useMemo, useState } from "react";

import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { ApprovalRequest, InboxRow } from "../../editor/api/types";
import { createPublishApi } from "../../pages/Publish/api";
import type { Publication, Target } from "../../pages/Publish/types";
import { writingClient } from "../../pages/Write/client";

// What the page knows about a request beyond the request itself, every member read from the service:
// the row of the unified read (undo, the skill the agent had read, the tool), the publication and the target
// that wait on it, and, for a blog post, its words and its text. A member that did not arrive stays undefined and
// the item says "Not reported".
export interface ApprovalContext {
	row?: InboxRow;
	publication?: Publication;
	target?: Target;
	post?: { words: number; text: string; slug: string };
}

export const SETTLE_MS = 250;

// The request ids make the key: a request that came or went is read again, a stream event that changes nothing
// is not.
export function useApprovalContext(
	requests: readonly ApprovalRequest[] | null,
): Readonly<Record<string, ApprovalContext>> {
	const key = (requests ?? []).map((r) => r.id).join(",");
	const [context, setContext] = useState<Record<string, ApprovalContext>>({});

	useEffect(() => {
		if (key === "") return;
		let alive = true;
		const http = new HttpTransport("/api/v1");
		const ids = new Set(key.split(","));
		const read = async (): Promise<void> => {
			const next: Record<string, ApprovalContext> = {};
			for (const id of ids) next[id] = {};
			try {
				const inbox = await createClient(http).listInbox();
				for (const row of inbox.rows) {
					if (row.source === "approval" && ids.has(row.source_id)) {
						next[row.source_id] = { ...next[row.source_id], row };
					}
				}
			} catch {
				// the item reads "Not reported" for what it could not read
			}
			try {
				const list = await createPublishApi(http).list({ limit: 500 });
				for (const publication of list.publications) {
					for (const target of publication.targets) {
						const id = target.approval_id;
						if (id !== undefined && ids.has(id)) {
							next[id] = { ...next[id], publication, target };
						}
					}
				}
			} catch {
				// as above
			}
			const writing = writingClient(http);
			for (const entry of Object.values(next)) {
				const publication = entry.publication;
				if (publication === undefined || (publication.post ?? "") === "") {
					continue;
				}
				try {
					const checked = await writing.checkPost(
						publication.project_id,
						publication.post ?? "",
					);
					entry.post = {
						words: checked.words,
						text: checked.markdown,
						slug: checked.slug,
					};
				} catch {
					// the text stays unreported
				}
			}
			if (alive) setContext(next);
		};
		// A burst of stream events (many requests made at once) changes the key many times: only the set that
		// stays for a moment is read, so the page does not send three reads for each event.
		const timer = setTimeout(() => void read(), SETTLE_MS);
		return () => {
			alive = false;
			clearTimeout(timer);
		};
	}, [key]);

	return useMemo(() => (key === "" ? {} : context), [key, context]);
}
