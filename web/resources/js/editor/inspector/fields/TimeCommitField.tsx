import type { ReactNode } from "react";

import { useLocale } from "@/lib/i18n";
import { localizeDecimalInput, normalizeTimeInput } from "../../commands/units";
import { readTimeUnit } from "../../state/prefs";
import TimeField from "../../ui/TimeField";
import type { CommitResult } from "./commitField";
import { FieldRow } from "./FieldFrame";
import { useCommitField } from "./useCommitField";

// A time field of the inspector: the stored value is shown as the display
// mode formats it, what is typed is checked and given its unit by
// `normalizeTimeInput`, and only a string with a unit reaches the command
// (rule T8). A text that is not a time is not sent; the field says so.
export default function TimeCommitField({
	label,
	stored,
	commit,
	disabled = false,
}: {
	label: string;
	stored: string;
	commit(time: string): Promise<CommitResult>;
	disabled?: boolean;
}): ReactNode {
	const locale = useLocale();
	const binding = useCommitField(stored, async (draft) => {
		const result = normalizeTimeInput(
			localizeDecimalInput(draft, locale),
			readTimeUnit(),
		);
		if (!result.ok) {
			return {
				ok: false,
				error: { code: "E_INVALID_TIME", message: "" },
			};
		}
		return commit(result.value);
	});
	return (
		<FieldRow label={label} state={binding}>
			{(ids) => (
				<TimeField
					id={ids.control}
					value={binding.input.value}
					disabled={disabled}
					aria-busy={binding.busy}
					aria-describedby={binding.error ? ids.message : undefined}
					onValueChange={(text) => binding.field.change(text)}
					onFocus={binding.input.onFocus}
					onBlur={binding.input.onBlur}
					onKeyDown={binding.input.onKeyDown}
				/>
			)}
		</FieldRow>
	);
}
