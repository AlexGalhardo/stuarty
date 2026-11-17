import { Diamond, RotateCcw } from "lucide-react";
import type { KeyboardEvent, ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useLocale } from "@/lib/i18n";
import { localizeDecimalInput } from "../../commands/units";
import type { CommitResult } from "./commitField";
import { FieldRow } from "./FieldFrame";
import { useCommitField } from "./useCommitField";

const NUMBER = /^-?\d+(?:\.\d+)?$/;

// A number: Enter or blur commits, the arrow keys step and a burst of presses
// is one command 400 ms after the last one (SDD-012 section 12.3). The decimal
// comma of pt-BR is accepted (D-030); the command receives a number.
export default function NumberField({
	label,
	stored,
	commit,
	step = 1,
	min,
	max,
	below,
	suffix,
	placeholder,
	marker,
	reset,
	trailing,
	disabled = false,
}: {
	label: string;
	stored: string;
	commit(value: number): Promise<CommitResult>;
	step?: number;
	min?: number;
	max?: number;
	// An upper bound the value must stay under, which `max` cannot say.
	below?: number;
	suffix?: string;
	// Shown while the field is empty, for a value the selected items do not share.
	placeholder?: string;
	// The accessible name of the keyframe marker of an animated property.
	marker?: string;
	// Puts the property back to its default, beside the keyframe marker. Drawn
	// only when given; `disabled` while the value already is the default.
	reset?: { label: string; disabled: boolean; onReset(): void };
	// A control after the reset button, such as the keyframe toggle of a row.
	trailing?: ReactNode;
	disabled?: boolean;
}): ReactNode {
	const locale = useLocale();
	const parse = (text: string): number | null => {
		const cleaned = localizeDecimalInput(text.trim(), locale);
		if (!NUMBER.test(cleaned)) return null;
		const value = Number(cleaned);
		if (min !== undefined && value < min) return null;
		if (max !== undefined && value > max) return null;
		if (below !== undefined && value >= below) return null;
		return value;
	};
	const binding = useCommitField(stored, async (draft) => {
		const value = parse(draft);
		if (value === null) {
			return {
				ok: false,
				error: {
					code: "E_VALIDATION",
					message: "",
					details: { min, max: below ?? max },
				},
			};
		}
		return commit(value);
	});

	const onKeyDown = (event: KeyboardEvent<HTMLInputElement>): void => {
		if (event.key === "ArrowUp" || event.key === "ArrowDown") {
			event.preventDefault();
			const current = parse(binding.text) ?? parse(stored) ?? 0;
			const next = current + (event.key === "ArrowUp" ? step : -step);
			const clamped = Math.min(
				max ?? Number.POSITIVE_INFINITY,
				Math.max(min ?? Number.NEGATIVE_INFINITY, next),
			);
			binding.field.change(String(Number(clamped.toFixed(6))), true);
			return;
		}
		binding.input.onKeyDown(event);
	};

	return (
		<FieldRow label={label} state={binding}>
			{(ids) => (
				<div className="flex items-center gap-1">
					<Input
						{...binding.input}
						id={ids.control}
						inputMode="decimal"
						autoComplete="off"
						disabled={disabled}
						placeholder={placeholder}
						aria-describedby={binding.error ? ids.message : undefined}
						className="h-8"
						onKeyDown={onKeyDown}
					/>
					{suffix !== undefined && (
						<span className="text-xs text-muted-foreground">{suffix}</span>
					)}
					{marker !== undefined && (
						<Diamond
							role="img"
							aria-label={marker}
							data-keyframe-marker=""
							className="size-3 shrink-0 fill-current text-ed-handle"
						/>
					)}
					{reset !== undefined && (
						<Button
							type="button"
							variant="ghost"
							size="icon-xs"
							aria-label={reset.label}
							title={reset.label}
							disabled={disabled || reset.disabled}
							onClick={reset.onReset}
						>
							<RotateCcw aria-hidden="true" />
						</Button>
					)}
					{trailing}
				</div>
			)}
		</FieldRow>
	);
}
