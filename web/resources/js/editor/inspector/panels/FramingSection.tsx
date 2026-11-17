import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import {
	CROP_SIDES,
	FITS,
	type Fit,
	framingOperations,
} from "../../commands/builders";
import { registryLimits } from "../../commands/registryLimits";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import SelectField from "../fields/SelectField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";
import KeyframedNumberField from "./KeyframedNumberField";

// The side that shares an axis with each side: the core refuses a crop where
// the two add up to 1 or more (`clip.set_framing`), and the field keeps the person
// from typing one, so no request goes out to be refused.
const OPPOSITE = {
	left: "right",
	right: "left",
	top: "bottom",
	bottom: "top",
} as const;

const isFit = (value: string): value is Fit =>
	(FITS as readonly string[]).includes(value);

// The Framing section of SDD-012 section 12.1 for a clip: how the picture fits
// the frame and how much of each side is cut. A crop change names one side; the
// core merges it with the stored crop and judges the sums.
export default function FramingSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	if (item === undefined) return null;
	return (
		<Section title={t("ui.editor.clip.framing_title")}>
			<SelectField
				label={t("ui.editor.clip.fit")}
				stored={item.fit ?? "cover"}
				disabled={!canEdit}
				options={FITS.map((fit) => ({
					value: fit,
					label: t(`ui.editor.clip.fit_${fit}`),
				}))}
				commit={async (value) =>
					isFit(value)
						? send(framingOperations(itemId, { fit: value }))
						: { ok: true }
				}
			/>
			{CROP_SIDES.map((side) => (
				<KeyframedNumberField
					key={side}
					itemId={itemId}
					property={`crop_${side}`}
					label={t(`ui.editor.clip.crop_${side}`)}
					stored={item.crop?.[side] ?? 0}
					step={0.01}
					{...registryLimits("clip.set_framing", "crop", side)}
					below={1 - (item.crop?.[OPPOSITE[side]] ?? 0)}
					staticOperations={(value) =>
						framingOperations(itemId, { crop: { [side]: value } })
					}
				/>
			))}
		</Section>
	);
}
