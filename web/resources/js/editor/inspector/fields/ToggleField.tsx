import type { ReactNode } from "react";

import type { CommitResult } from "./commitField";
import { FieldRow } from "./FieldFrame";
import { useCommitField } from "./useCommitField";

// A checkbox commits at once on change (SDD-012 section 12.3). The stored text
// is "true" or "false".
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
	const binding = useCommitField(String(stored), (draft) =>
		commit(draft === "true"),
	);
	return (
		<FieldRow label={label} state={binding}>
			{(ids) => (
				<input
					id={ids.control}
					type="checkbox"
					checked={binding.text === "true"}
					disabled={disabled}
					aria-busy={binding.busy}
					aria-invalid={binding.error ? true : undefined}
					aria-describedby={binding.error ? ids.message : undefined}
					className="mt-1.5 size-4 accent-primary"
					onChange={(event) => {
						binding.field.change(String(event.target.checked));
						void binding.field.commit();
					}}
				/>
			)}
		</FieldRow>
	);
}
