import { type ReactNode, useRef } from "react";

import type { CommitResult } from "./commitField";
import { FieldRow } from "./FieldFrame";
import { useCommitField } from "./useCommitField";

// A value on a slider: dragging only changes the draft, and the value is sent
// once, when the pointer is released (SDD-012 section 12.3). Arrow keys, held
// or pressed again and again, are a burst: one command with the final value,
// 400 ms after the last key, and no step is lost while an earlier one is in
// flight (VV-182). A change that comes with no pointer or key, such as one made
// by assistive software, is sent at once. The number beside it shows the draft.
export default function SliderField({
	label,
	stored,
	min,
	max,
	step,
	commit,
	disabled = false,
	trailing,
}: {
	label: string;
	stored: string;
	min: number;
	max: number;
	step: number;
	commit(value: number): Promise<CommitResult>;
	disabled?: boolean;
	// A control after the value, such as the keyframe button of a row.
	trailing?: ReactNode;
}): ReactNode {
	const binding = useCommitField(stored, async (draft) => {
		const value = Number(draft);
		if (draft.trim() === "" || Number.isNaN(value)) {
			return { ok: false, error: { code: "E_VALIDATION", message: "" } };
		}
		return commit(value);
	});
	const held = useRef(false);
	const keyed = useRef(false);
	const release = (): void => {
		if (!held.current) return;
		held.current = false;
		void binding.field.commit();
	};
	return (
		<FieldRow label={label} state={binding}>
			{(ids) => (
				<div className="flex items-center gap-2">
					<input
						id={ids.control}
						type="range"
						disabled={disabled}
						min={min}
						max={max}
						step={step}
						value={binding.text}
						aria-describedby={binding.error ? ids.message : undefined}
						className="h-8 min-w-0 flex-1 accent-primary"
						onPointerDown={() => {
							held.current = true;
							keyed.current = false;
							// Released anywhere on the page, not only over the control.
							window.addEventListener("pointerup", release, { once: true });
						}}
						onKeyDown={() => {
							if (!held.current) keyed.current = true;
						}}
						onChange={(event) => {
							if (keyed.current) {
								binding.field.change(event.target.value, true);
								return;
							}
							binding.field.change(event.target.value);
							if (!held.current) void binding.field.commit();
						}}
						onBlur={() => {
							release();
							keyed.current = false;
							void binding.field.commit();
						}}
					/>
					<output
						htmlFor={ids.control}
						className="w-12 shrink-0 text-right font-mono text-xs tabular-nums"
					>
						{binding.text}
					</output>
					{trailing}
				</div>
			)}
		</FieldRow>
	);
}
