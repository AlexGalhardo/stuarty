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
	// A catalogue key for the notices the UI itself words (undo skipped).
	messageKey?: string;
	count?: number;
	// An action the notice offers, as the operations to send. The values come
	// from the core (`details.next_free`), never computed here.
	action?: {
		labelKey: string;
		operations: EditRequest["operations"];
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
