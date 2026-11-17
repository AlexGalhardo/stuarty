import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { type ColorChanges, colorOperations } from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import { registryRange } from "../fields/registryLimits";
import SelectField from "../fields/SelectField";
import SliderField from "../fields/SliderField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";
import CurveEditor from "./CurveEditor";
import HslControls from "./HslControls";
import WheelControls from "./WheelControls";

type SliderKey = Exclude<keyof ColorChanges, "lut_asset" | "lut_intensity">;

interface Row {
	key: SliderKey;
	// What the core uses when the item stores nothing.
	fallback: number;
}

const ROWS: readonly Row[] = [
	{ key: "brightness", fallback: 0 },
	{ key: "contrast", fallback: 1 },
	{ key: "saturation", fallback: 1 },
	{ key: "temperature", fallback: 0 },
	{ key: "tint", fallback: 0 },
	{ key: "hue", fallback: 0 },
];

const NO_LUT = "";

// The Colour section of SDD-012 section 12.1 for a clip: six sliders, the
// lookup table picker and the HSL, wheels and curves groups, each change one
// `clip.set_color` (sliders when released).
// The picker lists the assets of kind lut; "None" sends an intensity of 0, which
// the core reads as removing the table.
export default function ColorSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	const assets = useDocumentState((s) => s.document?.assets);
	if (item === undefined) return null;
	const luts = Object.values(assets ?? {}).filter(
		(asset) => asset.kind === "lut",
	);
	const lut = item.lut;
	return (
		<Section title={t("ui.editor.clip.color_title")}>
			{ROWS.map((row) => (
				<SliderField
					key={row.key}
					label={t(`ui.editor.clip.${row.key}`)}
					stored={String(item[row.key] ?? row.fallback)}
					disabled={!canEdit}
					{...registryRange("clip.set_color", row.key)}
					step={row.key === "hue" ? 1 : 0.01}
					commit={(value) =>
						send(colorOperations(itemId, { [row.key]: value }))
					}
				/>
			))}
			{luts.length > 0 && (
				<>
					<SelectField
						label={t("ui.editor.clip.lut")}
						stored={lut === undefined ? NO_LUT : lut.asset}
						disabled={!canEdit}
						options={[
							{ value: NO_LUT, label: t("ui.editor.clip.lut_none") },
							...luts.map((asset) => ({ value: asset.id, label: asset.name })),
						]}
						commit={(value) =>
							send(
								colorOperations(
									itemId,
									value === NO_LUT
										? { lut_intensity: 0 }
										: { lut_asset: value },
								),
							)
						}
					/>
					{lut !== undefined && (
						<SliderField
							label={t("ui.editor.clip.lut_intensity")}
							stored={String(lut.intensity)}
							disabled={!canEdit}
							{...registryRange("clip.set_color", "lut_intensity")}
							step={0.01}
							commit={(value) =>
								send(colorOperations(itemId, { lut_intensity: value }))
							}
						/>
					)}
				</>
			)}
			<HslControls itemId={itemId} />
			<WheelControls itemId={itemId} />
			<CurveEditor itemId={itemId} />
		</Section>
	);
}
