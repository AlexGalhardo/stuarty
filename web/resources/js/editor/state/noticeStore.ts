import type { EditRequest } from "../api/types";
import { createStore, type ReadStore, readOnly } from "./createStore";

// A message the core's answer asks the UI to show (SDD-012 section 8.4): its
// message and hint come from the core and are shown as received.
export interface Notice {
	id: number;
	kind: "error" | "info";
	code?: string;
	message: string;
	hint?: string;
	// The `details` of the core's error, for the catalogue wording (VV-126).
	details?: { readonly [key: string]: unknown };
	// A catalogue key for the notices the UI itself words (undo skipped).
	messageKey?: string;
	count?: number;
	// The actor a UI-worded notice names (undo skipped, section 15).
	actor?: string;
	// An E_REVISION_CONFLICT: who changed what after the revision the batch was
	// based on, and the text the core made of the changes (SDD-012 section 4.5).
	conflict?: {
		actor: string;
		itemId: string | null;
		diff: string;
		revision: number;
	};
	// Values the catalogue text of `messageKey` names.
	params?: Readonly<Record<string, string | number>>;
	// An action the notice offers: the operations to send (the values come from
	// the core, `details.next_free`, never computed here), or an item to select.
	action?: {
		labelKey: string;
		operations?: EditRequest["operations"];
		select?: string;
	};
}

export interface NoticeState {
	notices: readonly Notice[];
}

export interface NoticeWriter {
	push(notice: Omit<Notice, "id">): void;
	dismiss(id: number): void;
}

export function createNoticeStore(): {
	store: ReadStore<NoticeState>;
	writer: NoticeWriter;
} {
	let next = 1;
	const store = createStore<NoticeState>({ notices: [] });
	return {
		store: readOnly(store),
		writer: {
			push: (notice) =>
				store.setState((s) => ({
					notices: [...s.notices.slice(-4), { ...notice, id: next++ }],
				})),
			dismiss: (id) =>
				store.setState((s) => ({
					notices: s.notices.filter((n) => n.id !== id),
				})),
		},
	};
}
