import { type ReactNode, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { type ColorChanges, colorOperations } from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import type { FieldError } from "../fields/commitField";
import { FieldMessage, FieldRow } from "../fields/FieldFrame";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

const SELECT_CLASS =
	"h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30";

// A choice that only changes what the controls below it show: nothing is sent.
export function ChoiceRow<T extends string>({
	label,
	value,
	options,
	onChange,
}: {
	label: string;
	value: T;
	options: readonly { value: T; label: string }[];
	onChange(value: T): void;
}): ReactNode {
	return (
		<FieldRow label={label} state={{ error: null, remoteActor: null }}>
			{(ids) => (
				<select
					id={ids.control}
					value={value}
					className={SELECT_CLASS}
					onChange={(event) => {
						const next = options.find(
							(option) => option.value === event.target.value,
						);
						if (next !== undefined) onChange(next.value);
					}}
				>
					{options.map((option) => (
						<option key={option.value} value={option.value}>
							{option.label}
						</option>
					))}
				</select>
			)}
		</FieldRow>
	);
}

// A group of the Colour section that is collapsed until opened (HSL, wheels,
// curves), with its "Reset": the neutral value of what the item stores, which
// the core reads as removing it. A refusal shows under the button as received.
export default function ColorGroup({
	itemId,
	title,
	resetLabel,
	canReset,
	reset,
	children,
}: {
	itemId: string;
	title: string;
	resetLabel: string;
	canReset: boolean;
	reset(): ColorChanges;
	children: ReactNode;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const [error, setError] = useState<FieldError | null>(null);
	return (
		<Section title={title} defaultOpen={false}>
			{children}
			<div className="flex flex-col gap-1">
				<Button
					type="button"
					size="sm"
					variant="outline"
					className="self-start"
					disabled={!canEdit || !canReset}
					aria-label={t("ui.editor.clip.color_reset_of", { group: title })}
					onClick={() => {
						void send(colorOperations(itemId, reset())).then((result) =>
							setError(result.ok ? null : result.error),
						);
					}}
				>
					{resetLabel}
				</Button>
				<FieldMessage
					id={`color-reset-${itemId}-${title}`}
					state={{ error, remoteActor: null }}
				/>
			</div>
		</Section>
	);
}
