import { type ReactNode, useEffect, useRef } from "react";

import { Input } from "@/components/ui/input";
import { Slider } from "@/components/ui/slider";
import { useT } from "@/lib/i18n";
import { useBrandKit } from "../useBrandKit";
import type { CommitResult } from "./commitField";
import { FieldRow } from "./FieldFrame";
import { useCommitField } from "./useCommitField";

const HEX = /^#([0-9a-fA-F]{6})([0-9a-fA-F]{2})?$/;

export function splitHex(text: string): { rgb: string; alpha: number } | null {
	const match = HEX.exec(text.trim());
	if (match === null) return null;
	return {
		rgb: `#${(match[1] as string).toUpperCase()}`,
		alpha: match[2] === undefined ? 255 : Number.parseInt(match[2], 16),
	};
}

// `#RRGGBB` when opaque, `#RRGGBBAA` otherwise (SDD-012 section 12.1).
export function joinHex(rgb: string, alpha: number): string {
	if (alpha >= 255) return rgb.toUpperCase();
	return `${rgb.toUpperCase()}${alpha.toString(16).padStart(2, "0").toUpperCase()}`;
}

// A colour: hex text (Enter or blur commits), the native colour input (commits
// when its picker closes) and an alpha slider (commits on release).
export default function ColorField({
	label,
	stored,
	commit,
	disabled = false,
}: {
	label: string;
	stored: string;
	commit(hex: string): Promise<CommitResult>;
	disabled?: boolean;
}): ReactNode {
	const t = useT();
	const swatches = Object.entries(useBrandKit()?.colors ?? {});
	const binding = useCommitField(stored, async (draft) => {
		const parts = splitHex(draft);
		if (parts === null) {
			return {
				ok: false,
				error: { code: "E_VALIDATION", message: "" },
			};
		}
		return commit(joinHex(parts.rgb, parts.alpha));
	});
	const parts = splitHex(binding.text);
	const picker = useRef<HTMLInputElement>(null);
	const { field } = binding;

	// The native picker reports `change` when it closes; React reports every
	// step as `input`, so the commit listens to the native event.
	useEffect(() => {
		const node = picker.current;
		if (node === null) return;
		const onChange = (): void => void field.commit();
		node.addEventListener("change", onChange);
		return () => node.removeEventListener("change", onChange);
	}, [field]);

	return (
		<FieldRow label={label} state={binding}>
			{(ids) => (
				<div className="flex flex-col gap-1.5">
					<div className="flex items-center gap-1.5">
						<input
							ref={picker}
							type="color"
							aria-label={t("ui.editor.inspector.color_picker", { label })}
							disabled={disabled}
							value={parts === null ? "#000000" : parts.rgb.toLowerCase()}
							className="h-8 w-8 shrink-0 cursor-pointer rounded-lg border border-input bg-transparent p-0.5"
							onChange={(event) =>
								field.change(joinHex(event.target.value, parts?.alpha ?? 255))
							}
						/>
						<Input
							{...binding.input}
							id={ids.control}
							autoComplete="off"
							spellCheck={false}
							disabled={disabled}
							aria-describedby={binding.error ? ids.message : undefined}
							className="h-8 font-mono"
						/>
					</div>
					<Slider
						aria-label={t("ui.editor.inspector.alpha", { label })}
						disabled={disabled || parts === null}
						min={0}
						max={255}
						step={1}
						value={[parts?.alpha ?? 255]}
						onValueChange={([alpha]) =>
							parts !== null &&
							alpha !== undefined &&
							field.change(joinHex(parts.rgb, alpha))
						}
						onValueCommit={() => void field.commit()}
					/>
					{swatches.length > 0 && (
						<fieldset className="flex flex-wrap gap-1">
							<legend className="sr-only">
								{t("ui.editor.inspector.brand_colors", { label })}
							</legend>
							{swatches.map(([name, hex]) => (
								<button
									key={name}
									type="button"
									disabled={disabled}
									title={`${name} ${hex}`}
									aria-label={t("ui.editor.inspector.brand_color_use", {
										name,
									})}
									data-brand-color={name}
									className="size-5 rounded-md border border-input disabled:opacity-50"
									style={{ backgroundColor: hex }}
									onClick={() => void commit(hex)}
								/>
							))}
						</fieldset>
					)}
				</div>
			)}
		</FieldRow>
	);
}
