import {
	type ChangeEvent,
	type FocusEvent,
	type KeyboardEvent,
	useEffect,
	useRef,
	useSyncExternalStore,
} from "react";

import { useRuntime } from "../../state/EditorContext";
import {
	CommitField,
	type CommitResult,
	type FieldSnapshot,
} from "./commitField";

export interface CommitFieldBinding extends FieldSnapshot {
	field: CommitField;
	// Props for a text-like control: Enter or blur commits, Esc discards.
	input: {
		value: string;
		"aria-busy": boolean;
		"aria-invalid": true | undefined;
		onChange(event: ChangeEvent<HTMLInputElement>): void;
		onFocus(event: FocusEvent<HTMLInputElement>): void;
		onBlur(event: FocusEvent<HTMLInputElement>): void;
		onKeyDown(event: KeyboardEvent<HTMLInputElement>): void;
	};
}

// The commit rules of SDD-012 section 12.3 for one field. `stored` is the text
// of the value in the read model; `commit` turns a draft into a command and
// reports how it went. A field keeps a local draft while it has focus.
export function useCommitField(
	stored: string,
	commit: (draft: string) => Promise<CommitResult>,
): CommitFieldBinding {
	const runtime = useRuntime();
	const latest = useRef(commit);
	latest.current = commit;
	const ref = useRef<CommitField | null>(null);
	if (ref.current === null) {
		ref.current = new CommitField(stored, {
			commit: (draft) => latest.current(draft),
		});
	}
	const field = ref.current;

	useEffect(() => {
		const actor = runtime.activity.getState().entries[0]?.actor ?? null;
		field.setStored(stored, actor);
	}, [stored, field, runtime]);

	const snapshot = useSyncExternalStore(field.subscribe, field.snapshot);

	return {
		...snapshot,
		field,
		input: {
			value: snapshot.text,
			"aria-busy": snapshot.busy,
			"aria-invalid": snapshot.error === null ? undefined : true,
			onChange: (event) => field.change(event.target.value),
			onFocus: () => field.focus(),
			onBlur: () => void field.blur(),
			onKeyDown: (event) => {
				if (event.key === "Enter") {
					event.preventDefault();
					void field.commit();
				} else if (event.key === "Escape" && snapshot.drafting) {
					// Esc closes nothing while there is a draft to discard.
					event.preventDefault();
					event.stopPropagation();
					field.revert();
				}
			},
		},
	};
}
