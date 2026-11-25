import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import type { CommitResult } from "./commitField";
import { FieldRow } from "./FieldFrame";
import { useCommitField } from "./useCommitField";

// A checkbox drawn as a switch, with the state in a word (never colour alone), commits at once on change
// (SDD-012 section 12.3). It stays a native checkbox, so the keyboard, the label and the name are the browser's.
// The stored text is "true" or "false".
export default function ToggleField({
	label,
	stored,
	commit,
	disabled = false,
}: {
	label: string;
	stored: boolean;
	commit(value: boolean): Promise<CommitResult>;
	disabled?: boolean;
}): ReactNode {
	const t = useT();
	const binding = useCommitField(String(stored), (draft) =>
		commit(draft === "true"),
	);
	const on = binding.text === "true";
	return (
		<FieldRow label={label} state={binding}>
			{(ids) => (
				<span className="flex items-center gap-2 pt-1">
					<input
						id={ids.control}
						type="checkbox"
						checked={on}
						disabled={disabled}
						aria-busy={binding.busy}
						aria-invalid={binding.error ? true : undefined}
						aria-describedby={binding.error ? ids.message : undefined}
						className="relative h-[18px] w-8 shrink-0 cursor-pointer appearance-none rounded-full border border-border-strong bg-surface-sunken before:absolute before:top-px before:left-px before:size-3.5 before:rounded-full before:bg-text-muted checked:border-transparent checked:bg-accent checked:before:translate-x-3.5 checked:before:bg-text-on-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:cursor-not-allowed disabled:opacity-50 after:absolute after:-inset-x-1 after:-inset-y-1.5"
						onChange={(event) => {
							binding.field.change(String(event.target.checked));
							void binding.field.commit();
						}}
					/>
					<span className="text-sm">
						{t(on ? "ui.editor.toggle.on" : "ui.editor.toggle.off")}
					</span>
				</span>
			)}
		</FieldRow>
	);
}
