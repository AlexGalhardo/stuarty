import { Diamond } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import {
	COLOUR_FALLBACK,
	type ColourKeyframeProperty,
	colorOperations,
	containsInclusive,
	keyframeAt,
	keyframeRemoveOperations,
	keyframeSetOperations,
	shownColourKeyframe,
} from "../../commands/builders";
import { registryRange } from "../../commands/registryLimits";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState, useUiState } from "../../state/EditorContext";
import SelectField from "../fields/SelectField";
import SliderField from "../fields/SliderField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";
import CurveEditor from "./CurveEditor";
import HslControls from "./HslControls";
import WheelControls from "./WheelControls";

const ROWS: readonly ColourKeyframeProperty[] = [
	"brightness",
	"contrast",
	"saturation",
	"temperature",
	"tint",
	"hue",
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
	const playhead = useUiState((s) => s.playheadFrame);
	const assets = useDocumentState((s) => s.document?.assets);
	if (item === undefined) return null;
	const luts = Object.values(assets ?? {}).filter(
		(asset) => asset.kind === "lut",
	);
	const lut = item.lut;
	return (
		<Section title={t("ui.editor.clip.color_title")}>
			{ROWS.map((key) => {
				const keys = item.keyframes?.[key] ?? [];
				const animated = keys.length > 0;
				const here = keyframeAt(keys, item.start, playhead);
				const inside = containsInclusive(
					{ id: itemId, start: item.start, duration: item.duration },
					playhead,
				);
				const label = t(`ui.editor.clip.${key}`);
				// A property with keyframes shows the keyframe at the playhead (or the
				// nearest before it) and an edit becomes a keyframe there, as for the
				// Transform fields; `clip.set_color` refuses a static value for it.
				const shown = animated
					? (shownColourKeyframe(keys, item.start, playhead)?.value ??
						COLOUR_FALLBACK[key])
					: (item[key] ?? COLOUR_FALLBACK[key]);
				return (
					<SliderField
						key={key}
						label={label}
						stored={String(shown)}
						disabled={!canEdit}
						{...registryRange("clip.set_color", key)}
						step={key === "hue" ? 1 : 0.01}
						commit={(value) =>
							send(
								animated
									? keyframeSetOperations({
											itemId,
											property: key,
											frame: playhead,
											value,
											easing: here?.easing,
											bezier: here?.bezier,
										})
									: colorOperations(itemId, { [key]: value }),
							)
						}
						trailing={
							<Button
								type="button"
								size="icon-xs"
								variant="ghost"
								disabled={!canEdit || !inside}
								aria-pressed={here !== undefined}
								data-keyframe-button={key}
								aria-label={t(
									here === undefined
										? "ui.editor.clip.color_keyframe_add"
										: "ui.editor.clip.color_keyframe_remove",
									{ property: label },
								)}
								onClick={() =>
									void send(
										here === undefined
											? keyframeSetOperations({
													itemId,
													property: key,
													frame: playhead,
													value: shown,
												})
											: keyframeRemoveOperations(itemId, key, playhead),
									)
								}
							>
								<Diamond
									aria-hidden="true"
									className={
										here === undefined
											? "size-3"
											: "size-3 fill-current text-ed-handle"
									}
								/>
							</Button>
						}
					/>
				);
			})}
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
