import type { ReactNode } from "react";

import type { CommitResult } from "./commitField";
import { FieldRow } from "./FieldFrame";
import { useCommitField } from "./useCommitField";

export interface SelectOption {
	value: string;
	label: string;
}

// The values the core listed as available in the error of the last commit
// (`details.available` of E_FONT_UNKNOWN), as strings.
function availableOf(details: { readonly [key: string]: unknown } | undefined) {
	const available = details?.available;
	return Array.isArray(available) ? available.map(String) : [];
}

// A select commits at once on change (SDD-012 section 12.3). After a refusal
// that names the available values, those options are highlighted.
export default function SelectField({
	label,
	stored,
	options,
	commit,
	disabled = false,
}: {
	label: string;
	stored: string;
	options: readonly SelectOption[];
	commit(value: string): Promise<CommitResult>;
	disabled?: boolean;
}): ReactNode {
	const binding = useCommitField(stored, commit);
	const available = availableOf(binding.error?.details);
	return (
		<FieldRow label={label} state={binding}>
			{(ids) => (
				<select
					id={ids.control}
					value={binding.text}
					disabled={disabled}
					aria-busy={binding.busy}
					aria-invalid={binding.error ? true : undefined}
					aria-describedby={binding.error ? ids.message : undefined}
					className="h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
					onChange={(event) => {
						binding.field.change(event.target.value);
						void binding.field.commit();
					}}
				>
					{options.map((option) => (
						<option
							key={option.value}
							value={option.value}
							data-available={available.includes(option.value) ? "" : undefined}
							className={
								available.includes(option.value) ? "font-bold" : undefined
							}
						>
							{option.label}
						</option>
					))}
				</select>
			)}
		</FieldRow>
	);
}
