import { type ReactNode, useRef } from "react";

import type { CommitResult } from "./commitField";
import { FieldRow } from "./FieldFrame";
import { useCommitField } from "./useCommitField";

// A value on a slider: dragging or holding a key only changes the draft, and
// the value is sent once, when the pointer or the key is released (SDD-012
// section 12.3). A change that comes with no pointer or key held, such as one
// made by assistive software, is sent at once. The number beside it shows the
// draft.
export default function SliderField({
	label,
	stored,
	min,
	max,
	step,
	commit,
	disabled = false,
}: {
	label: string;
	stored: string;
	min: number;
	max: number;
	step: number;
	commit(value: number): Promise<CommitResult>;
	disabled?: boolean;
}): ReactNode {
	const binding = useCommitField(stored, async (draft) => {
		const value = Number(draft);
		if (draft.trim() === "" || Number.isNaN(value)) {
			return { ok: false, error: { code: "E_VALIDATION", message: "" } };
		}
		return commit(value);
	});
	const held = useRef(false);
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
							// Released anywhere on the page, not only over the control.
							window.addEventListener("pointerup", release, { once: true });
						}}
						onKeyDown={() => {
							held.current = true;
						}}
						onChange={(event) => {
							binding.field.change(event.target.value);
							if (!held.current) void binding.field.commit();
						}}
						onKeyUp={release}
						onBlur={release}
					/>
					<output
						htmlFor={ids.control}
						className="w-12 shrink-0 text-right font-mono text-xs tabular-nums"
					>
						{binding.text}
					</output>
				</div>
			)}
		</FieldRow>
	);
}
