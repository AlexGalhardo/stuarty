import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import {
	type TransformProperty,
	transformOperations,
} from "../../commands/builders";
import { keyframeValueAt } from "../../commands/units";
import { useCanEdit } from "../../commands/useActions";
import {
	useDocumentState,
	useEffectiveState,
	useUiState,
} from "../../state/EditorContext";
import type { EffectiveValues } from "../../state/effectiveStore";
import NumberField from "../fields/NumberField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

interface Row {
	property: TransformProperty;
	step: number;
	min?: number;
	max?: number;
}

const ROWS: readonly Row[] = [
	{ property: "x", step: 1 },
	{ property: "y", step: 1 },
	{ property: "scale", step: 0.05, min: 0.001, max: 20 },
	{ property: "rotation", step: 1 },
	{ property: "opacity", step: 0.05, min: 0, max: 1 },
];

const round3 = (value: number): string =>
	String(Math.round(value * 1000) / 1000);

// The value the read model shows for a property of one item at the playhead:
// the stored field (or its default) and, for an animated property, the last
// keyframe at or before the playhead. A display of what the core holds; the
// core owns the defaults and the interpolation.
interface ItemView {
	start: number;
	x?: number;
	y?: number;
	scale?: number;
	rotation?: number;
	opacity?: number;
	keyframes?: { readonly [k: string]: readonly { t: number; value: number }[] };
}

function shown(
	item: ItemView,
	property: TransformProperty,
	playhead: number,
	size: { width: number; height: number },
	effective: EffectiveValues | undefined,
): string {
	const keys = item.keyframes?.[property];
	if (keys !== undefined && keys.length > 0) {
		// The plan reports the effective scale and opacity; until its window
		// arrives the field shows nothing rather than a value worked out here.
		if (property === "scale" || property === "opacity") {
			return effective === undefined ? "" : round3(effective[property]);
		}
		return round3(keyframeValueAt(keys, item.start, playhead));
	}
	switch (property) {
		case "x":
			return round3(item.x ?? size.width / 2);
		case "y":
			return round3(item.y ?? size.height / 2);
		case "scale":
			return round3(item.scale ?? 1);
		case "rotation":
			return round3(item.rotation ?? 0);
		case "opacity":
			return round3(item.opacity ?? 1);
	}
}

// The Transform section of SDD-012 section 12.1: X, Y, Scale, Rotation and
// Opacity of one or several visual items. A commit sends only the changed field
// to every selected item, in one batch: `item.set_transform`, or `keyframe.set`
// at the playhead for a property that has keyframes. Values the items do not
// share show "mixed".
export default function TransformSection({
	itemIds,
}: {
	itemIds: readonly string[];
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const items = useDocumentState((s) => s.document?.items);
	const width = useDocumentState((s) => s.document?.settings.width);
	const height = useDocumentState((s) => s.document?.settings.height);
	const playhead = useUiState((s) => s.playheadFrame);
	const effective = useEffectiveState((s) => s.items);
	const targets = itemIds.flatMap((id) => {
		const item = items?.[id];
		return item === undefined ? [] : [item];
	});
	if (targets.length === 0 || width === undefined || height === undefined)
		return null;

	return (
		<Section title={t("ui.editor.inspector.transform_title")}>
			{ROWS.map((row) => {
				const values = targets.map((item) =>
					shown(
						item,
						row.property,
						playhead,
						{ width, height },
						effective[item.id],
					),
				);
				const same = values.every((value) => value === values[0]);
				const animated = targets.some(
					(item) => (item.keyframes?.[row.property]?.length ?? 0) > 0,
				);
				return (
					<NumberField
						key={row.property}
						label={t(`ui.editor.inspector.transform.${row.property}`)}
						stored={same ? (values[0] ?? "") : ""}
						placeholder={same ? undefined : t("ui.editor.inspector.mixed")}
						marker={animated ? t("ui.editor.inspector.keyframed") : undefined}
						step={row.step}
						min={row.min}
						max={row.max}
						disabled={!canEdit}
						commit={(value) =>
							send(
								transformOperations(
									targets.map((item) => ({
										id: item.id,
										animated: Object.keys(item.keyframes ?? {}),
									})),
									{ [row.property]: value },
									playhead,
								),
							)
						}
					/>
				);
			})}
		</Section>
	);
}
