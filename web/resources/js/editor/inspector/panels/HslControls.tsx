import { type ReactNode, useState } from "react";

import { useT } from "@/lib/i18n";
import {
	colorOperations,
	colorResetChanges,
	HSL_RANGES,
	type HslRange,
} from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import { registryRange } from "../fields/registryLimits";
import SliderField from "../fields/SliderField";
import { useFieldCommit } from "../useFieldCommit";
import ColorGroup, { ChoiceRow } from "./ColorGroup";

const CHANNELS = ["hue", "saturation", "lightness"] as const;

// HSL by colour range (SDD-012 section 12.1): the range is a choice that only
// picks which three sliders show; each slider sends the whole band of that
// range in `clip.set_color {hsl: [band]}`. A band at 0, 0, 0 is neutral and the
// core drops it.
export default function HslControls({ itemId }: { itemId: string }): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	const [range, setRange] = useState<HslRange>("red");
	if (item === undefined) return null;
	const bands = item.hsl ?? [];
	const band = bands.find((entry) => entry.range === range);
	const stored = {
		hue: band?.hue ?? 0,
		saturation: band?.saturation ?? 0,
		lightness: band?.lightness ?? 0,
	};
	return (
		<ColorGroup
			itemId={itemId}
			title={t("ui.editor.clip.hsl_title")}
			resetLabel={t("ui.editor.clip.color_reset")}
			canReset={bands.length > 0}
			reset={() =>
				colorResetChanges("hsl", {
					hsl: bands.map((entry) => entry.range),
					wheels: [],
					curves: [],
				})
			}
		>
			<ChoiceRow<HslRange>
				label={t("ui.editor.clip.hsl_range")}
				value={range}
				options={HSL_RANGES.map((name) => ({
					value: name,
					label: t(`ui.editor.clip.hsl_range_name.${name}`),
				}))}
				onChange={setRange}
			/>
			{CHANNELS.map((channel) => (
				<SliderField
					key={`${range}-${channel}`}
					label={t(`ui.editor.clip.hsl_${channel}`)}
					stored={String(stored[channel])}
					disabled={!canEdit}
					{...registryRange("clip.set_color", "hsl", "items", channel)}
					step={0.01}
					commit={(value) =>
						send(
							colorOperations(itemId, {
								hsl: [{ range, ...stored, [channel]: value }],
							}),
						)
					}
				/>
			))}
		</ColorGroup>
	);
}
