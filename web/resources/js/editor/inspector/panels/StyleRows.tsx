import type { ReactNode } from "react";

import type { CaptionStyle } from "@/generated/timeline";
import { useT } from "@/lib/i18n";
import type { StyleChanges, StyleValue } from "../../commands/builders";
import ColorField from "../fields/ColorField";
import type { CommitResult } from "../fields/commitField";
import NumberField from "../fields/NumberField";
import SelectField from "../fields/SelectField";
import ToggleField from "../fields/ToggleField";
import { useFonts } from "../useFonts";

export type StyleRow =
	| { key: string; kind: "font" }
	| { key: string; kind: "weight" }
	| { key: string; kind: "color" }
	| { key: string; kind: "toggle" }
	| {
			key: string;
			kind: "number";
			step: number;
			min?: number;
			max?: number;
	  }
	| { key: string; kind: "choice"; options: readonly string[] };

// The style fields of a text item (SDD-012 section 12.1, row Text), in the
// order of the table: the keys are those of the partial TextStyle.
export const TEXT_STYLE_ROWS: readonly StyleRow[] = [
	{ key: "font_family", kind: "font" },
	{ key: "font_weight", kind: "weight" },
	{ key: "size", kind: "number", step: 1, min: 8, max: 1000 },
	{ key: "color", kind: "color" },
	{ key: "stroke_color", kind: "color" },
	{ key: "stroke_width", kind: "number", step: 1, min: 0, max: 40 },
	{ key: "shadow_color", kind: "color" },
	{ key: "shadow_dx", kind: "number", step: 1 },
	{ key: "shadow_dy", kind: "number", step: 1 },
	{ key: "shadow_blur", kind: "number", step: 1, min: 0, max: 100 },
	{ key: "background_color", kind: "color" },
	{ key: "background_padding", kind: "number", step: 1, min: 0 },
	{ key: "background_radius", kind: "number", step: 1, min: 0 },
	{ key: "align", kind: "choice", options: ["left", "center", "right"] },
	{ key: "line_height", kind: "number", step: 0.1, min: 0.8, max: 3 },
	{ key: "max_width", kind: "number", step: 10, min: 1 },
	{ key: "uppercase", kind: "toggle" },
];

const text = (value: unknown): string =>
	typeof value === "string" || typeof value === "number" ? String(value) : "";

const OK: CommitResult = { ok: true };

// The rows of a style: each is bound to one key and commits a partial style
// with only that key. `stored` is the style of the read model; a key it lacks
// shows empty, because the defaults belong to the core.
export default function StyleRows({
	rows,
	stored,
	commit,
	disabled,
}: {
	rows: readonly StyleRow[];
	stored: CaptionStyle;
	commit(style: StyleChanges): Promise<CommitResult>;
	disabled: boolean;
}): ReactNode {
	const t = useT();
	const { families, reload } = useFonts();
	const family = text(stored.font_family);
	const send = (key: string, value: StyleValue): Promise<CommitResult> =>
		commit({ [key]: value });
	const unset = { value: "", label: t("ui.editor.inspector.style.default") };
	const renderRow = (row: StyleRow): ReactNode => {
		const label = t(`ui.editor.inspector.style.${row.key}`);
		switch (row.kind) {
			case "font":
				return (
					<SelectField
						key={row.key}
						label={label}
						stored={family}
						disabled={disabled}
						options={[
							unset,
							...families.map((f) => ({
								value: f.family,
								label: f.family,
							})),
						]}
						commit={async (value) => {
							if (value === "") return OK;
							const result = await send(row.key, value);
							if (!result.ok && result.error.code === "E_FONT_UNKNOWN") {
								reload();
							}
							return result;
						}}
					/>
				);
			case "weight": {
				const weights =
					families.find((f) => f.family === (family || "Inter"))?.weights ?? [];
				return (
					<SelectField
						key={row.key}
						label={label}
						stored={text(stored.font_weight)}
						disabled={disabled}
						options={[
							unset,
							...weights.map((w) => ({
								value: String(w),
								label: String(w),
							})),
						]}
						commit={async (value) =>
							value === "" ? OK : send(row.key, Number(value))
						}
					/>
				);
			}
			case "color":
				return (
					<ColorField
						key={row.key}
						label={label}
						stored={text(stored[row.key as keyof CaptionStyle])}
						disabled={disabled}
						commit={(value) => send(row.key, value)}
					/>
				);
			case "toggle":
				return (
					<ToggleField
						key={row.key}
						label={label}
						stored={stored.uppercase === true}
						disabled={disabled}
						commit={(value) => send(row.key, value)}
					/>
				);
			case "number":
				return (
					<NumberField
						key={row.key}
						label={label}
						stored={text(stored[row.key as keyof CaptionStyle])}
						step={row.step}
						min={row.min}
						max={row.max}
						disabled={disabled}
						commit={(value) => send(row.key, value)}
					/>
				);
			case "choice":
				return (
					<SelectField
						key={row.key}
						label={label}
						stored={text(stored[row.key as keyof CaptionStyle])}
						disabled={disabled}
						options={[
							unset,
							...row.options.map((option) => ({
								value: option,
								label: t(`ui.editor.inspector.style.${row.key}_${option}`),
							})),
						]}
						commit={async (value) => (value === "" ? OK : send(row.key, value))}
					/>
				);
		}
	};
	return <>{rows.map(renderRow)}</>;
}
