import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { type ColorChanges, colorOperations } from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import { registryRange } from "../fields/registryLimits";
import SliderField from "../fields/SliderField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

interface Row {
	key: keyof ColorChanges;
	// What the core uses when the item stores nothing.
	fallback: number;
}

const ROWS: readonly Row[] = [
	{ key: "brightness", fallback: 0 },
	{ key: "contrast", fallback: 1 },
	{ key: "saturation", fallback: 1 },
];

// The Colour section of SDD-012 section 12.1 for a clip: three sliders, each
// one `clip.set_color` sent when the slider is released.
export default function ColorSection({
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
		<Section title={t("ui.editor.clip.color_title")}>
			{ROWS.map((row) => (
				<SliderField
					key={row.key}
					label={t(`ui.editor.clip.${row.key}`)}
					stored={String(item[row.key] ?? row.fallback)}
					disabled={!canEdit}
					{...registryRange("clip.set_color", row.key)}
					step={0.01}
					commit={(value) =>
						send(colorOperations(itemId, { [row.key]: value }))
					}
				/>
			))}
		</Section>
	);
}
