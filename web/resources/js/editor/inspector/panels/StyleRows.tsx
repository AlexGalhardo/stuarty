import { Fragment, type ReactNode } from "react";

import type { CaptionStyle } from "@/generated/timeline";
import { useT } from "@/lib/i18n";
import type { StyleChanges, StyleValue } from "../../commands/builders";
import { fontFamilyChanges } from "../../commands/fontWeight";
import { useFonts } from "../../commands/useFonts";
import { useDocumentState } from "../../state/EditorContext";
import ColorField from "../fields/ColorField";
import type { CommitResult } from "../fields/commitField";
import NumberField from "../fields/NumberField";
import SelectField from "../fields/SelectField";
import ToggleField from "../fields/ToggleField";
import { useBrandKit } from "../useBrandKit";

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

// The caption style adds the position and the highlight of the spoken word
// (RF-032) to the text style fields.
export const CAPTION_STYLE_ROWS: readonly StyleRow[] = [
	...TEXT_STYLE_ROWS,
	{
		key: "position",
		kind: "choice",
		options: ["bottom", "middle", "top", "custom"],
	},
	{ key: "highlight", kind: "choice", options: ["none", "word"] },
	{ key: "highlight_color", kind: "color" },
	{ key: "highlight_background", kind: "color" },
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
	// Brand kit families the project does not hold yet are not in the select;
	// the hint says where they are brought in (VV-183).
	const projectFonts = useDocumentState((s) => s.document?.fonts);
	const kitFonts = useBrandKit()?.fonts;
	const brandPending = Object.keys(kitFonts ?? {}).some(
		(name) =>
			projectFonts?.[name] === undefined &&
			!families.some((f) => f.family === name),
	);
	const family = text(stored.font_family);
	const send = (key: string, value: StyleValue): Promise<CommitResult> =>
		commit({ [key]: value });
	const unset = { value: "", label: t("ui.editor.inspector.style.default") };
	const renderRow = (row: StyleRow): ReactNode => {
		const label = t(`ui.editor.inspector.style.${row.key}`);
		switch (row.kind) {
			case "font":
				return (
					<Fragment key={row.key}>
						<SelectField
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
								const weights =
									families.find((f) => f.family === value)?.weights ?? [];
								const result = await commit(
									fontFamilyChanges(value, weights, stored.font_weight),
								);
								if (!result.ok && result.error.code === "E_FONT_UNKNOWN") {
									reload();
								}
								return result;
							}}
						/>
						{brandPending && (
							<p className="pl-[104px] text-xs text-muted-foreground">
								{t("ui.editor.inspector.style.brand_fonts_hint")}
							</p>
						)}
					</Fragment>
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
