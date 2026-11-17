import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import {
	type TransformProperty,
	transformOperations,
} from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import {
	useDocumentState,
	useEffectiveState,
	useUiState,
} from "../../state/EditorContext";
import type { EffectiveValues } from "../../state/effectiveStore";
import NumberField from "../fields/NumberField";
import { registryLimits } from "../fields/registryLimits";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

interface Row {
	property: TransformProperty;
	step: number;
	// A lower bound the registry does not carry (`scale` is above 0, no minimum).
	min?: number;
}

const ROWS: readonly Row[] = [
	{ property: "x", step: 1 },
	{ property: "y", step: 1 },
	{ property: "scale", step: 0.05, min: 0.001 },
	{ property: "rotation", step: 1 },
	{ property: "opacity", step: 0.05 },
];

// The values the core documents as clearing the stored property
// (`item.set_transform`: scale 1, rotation 0 and opacity 1). X and Y have no
// such value in the registry, so they have no reset button.
const RESET_VALUE: Partial<Record<TransformProperty, number>> = {
	scale: 1,
	rotation: 0,
	opacity: 1,
};

// The reset of one property, or null when the property has none: the value to
// send and whether the button is off, because the field already shows it or a
// keyframed property is reset through its keyframes.
export function resetOf(
	property: TransformProperty,
	values: readonly string[],
	animated: boolean,
): { value: number; disabled: boolean } | null {
	const value = RESET_VALUE[property];
	if (value === undefined) return null;
	const atDefault =
		values.length > 0 && values.every((shown) => shown === round3(value));
	return { value, disabled: animated || atDefault };
}

export const round3 = (value: number): string =>
	String(Math.round(value * 1000) / 1000);

// The value the read model shows for a property of one item at the playhead:
// the stored field (or its default) and, for an animated property, the value
// the plan reports. A display of what the core holds; the core owns the
// defaults and the interpolation.
export interface ItemView {
	x?: number;
	y?: number;
	scale?: number;
	rotation?: number;
	opacity?: number;
	keyframes?: { readonly [k: string]: readonly { t: number; value: number }[] };
}

export function shown(
	item: ItemView,
	property: TransformProperty,
	size: { width: number; height: number },
	effective: EffectiveValues | undefined,
): string {
	const keys = item.keyframes?.[property];
	if (keys !== undefined && keys.length > 0) {
		// Until the plan window arrives the field shows nothing rather than a
		// value worked out here.
		return effective === undefined ? "" : round3(effective[property]);
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
					shown(item, row.property, { width, height }, effective[item.id]),
				);
				const same = values.every((value) => value === values[0]);
				const limits = registryLimits("item.set_transform", row.property);
				const animated = targets.some(
					(item) => (item.keyframes?.[row.property]?.length ?? 0) > 0,
				);
				const reset = resetOf(row.property, values, animated);
				const label = t(`ui.editor.inspector.transform.${row.property}`);
				return (
					<NumberField
						key={row.property}
						label={label}
						stored={same ? (values[0] ?? "") : ""}
						placeholder={same ? undefined : t("ui.editor.inspector.mixed")}
						marker={animated ? t("ui.editor.inspector.keyframed") : undefined}
						reset={
							reset === null
								? undefined
								: {
										label: t("ui.editor.inspector.reset_field", {
											name: label,
										}),
										disabled: reset.disabled,
										onReset: () =>
											void send(
												transformOperations(
													targets.map((item) => ({
														id: item.id,
														animated: [],
													})),
													{ [row.property]: reset.value },
													playhead,
												),
											),
									}
						}
						step={row.step}
						min={row.min ?? limits.min}
						max={limits.max}
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
