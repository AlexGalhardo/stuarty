import type { ReactNode } from "react";

import { Textarea } from "@/components/ui/textarea";
import type { CommitResult } from "./commitField";
import { FieldRow } from "./FieldFrame";
import { useCommitField } from "./useCommitField";

// Multi-line text: blur or Ctrl+Enter commits, Esc discards (SDD-012 section
// 12.3). Enter alone is a line break.
export default function TextAreaField({
	label,
	stored,
	commit,
	disabled = false,
	maxLength,
}: {
	label: string;
	stored: string;
	commit(text: string): Promise<CommitResult>;
	disabled?: boolean;
	maxLength?: number;
}): ReactNode {
	const binding = useCommitField(stored, commit);
	const { field } = binding;
	return (
		<FieldRow label={label} state={binding}>
			{(ids) => (
				<Textarea
					id={ids.control}
					value={binding.text}
					disabled={disabled}
					maxLength={maxLength}
					aria-busy={binding.busy}
					aria-invalid={binding.error ? true : undefined}
					aria-describedby={binding.error ? ids.message : undefined}
					onChange={(event) => field.change(event.target.value)}
					onFocus={() => field.focus()}
					onBlur={() => void field.blur()}
					onKeyDown={(event) => {
						if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
							event.preventDefault();
							void field.commit();
						} else if (event.key === "Escape" && binding.drafting) {
							event.preventDefault();
							event.stopPropagation();
							field.revert();
						}
					}}
				/>
			)}
		</FieldRow>
	);
}
