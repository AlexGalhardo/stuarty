import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { type MaskArgs, maskOperations } from "../../commands/builders";
import { registryLimits } from "../../commands/registryLimits";
import { useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import { useDocumentState } from "../../state/EditorContext";
import NumberField from "../fields/NumberField";
import SelectField from "../fields/SelectField";
import ToggleField from "../fields/ToggleField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

// What a mask starts with when a shape is picked: an inset box, hard edged.
const DEFAULT_MASK: MaskArgs = {
	shape: "rect",
	x: 0.1,
	y: 0.1,
	width: 0.8,
	height: 0.8,
	feather: 0,
	invert: false,
};

const NONE = "none";

const isShape = (value: string): value is MaskArgs["shape"] =>
	value === "rect" || value === "ellipse";

// The Mask section of a visual item (VV-015): the shape, its box in fractions
// of the item, the feather and the invert switch, one `clip.set_mask` per
// change that sends the whole mask (null for none). The preview and the export
// draw it from the plan, not from these values.
export default function MaskSection({ itemId }: { itemId: string }): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const exposed = useCommandsExposed("clip.set_mask");
	const item = useDocumentState((s) => s.document?.items[itemId]);
	if (!exposed || item === undefined) return null;
	const stored = item.mask;
	const current: MaskArgs | null =
		stored === undefined ? null : { ...stored, invert: stored.invert === true };
	const box = registryLimits("clip.set_mask", "mask", "x");
	const feather = registryLimits("clip.set_mask", "mask", "feather");
	const number = (
		label: string,
		key: "x" | "y" | "width" | "height",
		mask: MaskArgs,
	): ReactNode => (
		<NumberField
			label={t(label)}
			stored={String(mask[key])}
			disabled={!canEdit}
			step={0.05}
			min={box.min}
			max={box.max}
			percentInput
			suffix={t("ui.editor.clip.mask_unit")}
			commit={(value) =>
				send(maskOperations(itemId, { ...mask, [key]: value }))
			}
		/>
	);
	return (
		<Section title={t("ui.editor.clip.mask_title")}>
			<SelectField
				label={t("ui.editor.clip.mask_shape")}
				stored={current?.shape ?? NONE}
				disabled={!canEdit}
				options={[
					{ value: NONE, label: t("ui.editor.clip.mask_none") },
					{ value: "rect", label: t("ui.editor.clip.mask_rect") },
					{ value: "ellipse", label: t("ui.editor.clip.mask_ellipse") },
				]}
				commit={(value) => {
					if (value === NONE) return send(maskOperations(itemId, null));
					if (!isShape(value)) return Promise.resolve({ ok: true });
					return send(
						maskOperations(itemId, {
							...(current ?? DEFAULT_MASK),
							shape: value,
						}),
					);
				}}
			/>
			{current !== null && (
				<>
					{number("ui.editor.clip.mask_x", "x", current)}
					{number("ui.editor.clip.mask_y", "y", current)}
					{number("ui.editor.clip.mask_width", "width", current)}
					{number("ui.editor.clip.mask_height", "height", current)}
					<NumberField
						label={t("ui.editor.clip.mask_feather")}
						stored={String(current.feather)}
						disabled={!canEdit}
						step={1}
						min={feather.min}
						max={feather.max}
						suffix="px"
						commit={(value) =>
							send(maskOperations(itemId, { ...current, feather: value }))
						}
					/>
					<ToggleField
						label={t("ui.editor.clip.mask_invert")}
						stored={current.invert}
						disabled={!canEdit}
						commit={(invert) =>
							send(maskOperations(itemId, { ...current, invert }))
						}
					/>
				</>
			)}
		</Section>
	);
}
