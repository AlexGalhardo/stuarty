import { type ReactNode, useState } from "react";

import { useT } from "@/lib/i18n";
import {
	colorOperations,
	colorResetChanges,
	WHEEL_NAMES,
	type WheelName,
} from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import { registryRange } from "../fields/registryLimits";
import SliderField from "../fields/SliderField";
import { useFieldCommit } from "../useFieldCommit";
import ColorGroup from "./ColorGroup";

const CHANNELS = [
	{ key: "hue", step: 1 },
	{ key: "amount", step: 0.01 },
	{ key: "luma", step: 0.01 },
] as const;

// The colour wheels of shadows, midtones and highlights (SDD-012 section 12.1):
// the angle of the tint (hue), how much of it (amount) and the lift or drop of
// the brightness (luma), three sliders per wheel, so every control works from
// the keyboard. Each slider sends the whole wheel in `clip.set_color {wheels}`;
// a wheel with amount and luma at 0 is neutral and the core drops it.
export default function WheelControls({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	// The core drops a neutral wheel (amount 0, luma 0), hue included, so the
	// angle picked on such a wheel waits here until the wheel has a tint to keep.
	const [pendingHue, setPendingHue] = useState<
		Partial<Record<WheelName, number>>
	>({});
	if (item === undefined) return null;
	const stored = WHEEL_NAMES.filter(
		(name) => item.wheels?.[name] !== undefined,
	);
	return (
		<ColorGroup
			itemId={itemId}
			title={t("ui.editor.clip.wheels_title")}
			resetLabel={t("ui.editor.clip.color_reset")}
			canReset={stored.length > 0}
			reset={() =>
				colorResetChanges("wheels", { hsl: [], wheels: stored, curves: [] })
			}
		>
			{WHEEL_NAMES.map((name: WheelName) => {
				const wheel = {
					hue: item.wheels?.[name]?.hue ?? pendingHue[name] ?? 0,
					amount: item.wheels?.[name]?.amount ?? 0,
					luma: item.wheels?.[name]?.luma ?? 0,
				};
				const label = t(`ui.editor.clip.wheel_name.${name}`);
				return (
					<fieldset key={name} className="flex flex-col gap-1">
						<legend className="text-xs font-semibold">{label}</legend>
						{CHANNELS.map(({ key, step }) => (
							<SliderField
								key={key}
								label={t(`ui.editor.clip.wheel_${key}`)}
								stored={String(wheel[key])}
								disabled={!canEdit}
								{...registryRange("clip.set_color", "wheels", name, key)}
								step={step}
								commit={(value) => {
									const next = { ...wheel, [key]: value };
									setPendingHue((held) => ({ ...held, [name]: next.hue }));
									const neutral = next.amount === 0 && next.luma === 0;
									if (neutral && item.wheels?.[name] === undefined) {
										return Promise.resolve({ ok: true });
									}
									return send(
										colorOperations(itemId, { wheels: { [name]: next } }),
									);
								}}
							/>
						))}
					</fieldset>
				);
			})}
		</ColorGroup>
	);
}
