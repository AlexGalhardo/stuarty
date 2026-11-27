import type { ReactNode } from "react";

import { Input } from "@/components/ui/input";
import { useT } from "@/lib/i18n";
import type { CommitResult } from "./commitField";
import { FieldRow } from "./FieldFrame";
import { useCommitField } from "./useCommitField";

// A text value: Enter or blur commits, Esc discards (SDD-012 section 12.3).
export default function TextField({
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
	const t = useT();
	const binding = useCommitField(stored, commit);
	return (
		<FieldRow label={label} state={binding}>
			{(ids) => (
				<Input
					{...binding.input}
					id={ids.control}
					placeholder={t("ui.help.g5.text_value_ph")}
					aria-describedby={binding.error ? ids.message : undefined}
					disabled={disabled}
					maxLength={maxLength}
					className="h-8"
				/>
			)}
		</FieldRow>
	);
}
