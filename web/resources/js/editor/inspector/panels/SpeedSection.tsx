import { type ReactNode, useState } from "react";

import { useT } from "@/lib/i18n";
import { reverseOperations, speedOperations } from "../../commands/builders";
import { speedText } from "../../commands/units";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import NumberField from "../fields/NumberField";
import ToggleField from "../fields/ToggleField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";
import SpeedCurve from "./SpeedCurve";

// The Speed section of SDD-012 section 12.1, for a clip only: the speed with an
// `x` suffix and the choice to move the later items of the track with it, the
// Reverse toggle (`clip.set_speed` with `reverse`) and the speed curve. The
// ripple toggle is a choice for the next change, not a command of its own.
export default function SpeedSection({
	itemId,
	constantOnly = false,
}: {
	itemId: string;
	// A composition item has a constant speed only (epic 53): no Reverse, no curve.
	constantOnly?: boolean;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const [ripple, setRipple] = useState(false);
	const item = useDocumentState((s) => s.document?.items[itemId]);
	if (item === undefined) return null;
	const speed = item.speed;
	return (
		<Section name="speed" title={t("ui.editor.clip.speed_title")}>
			<NumberField
				label={t("ui.editor.clip.speed")}
				stored={speedText(speed)}
				disabled={!canEdit}
				step={0.25}
				min={0.25}
				max={4}
				suffix="x"
				commit={(value) => send(speedOperations(itemId, value, ripple))}
			/>
			<ToggleField
				label={t("ui.editor.clip.ripple")}
				stored={ripple}
				disabled={!canEdit}
				commit={async (next) => {
					setRipple(next);
					return { ok: true };
				}}
			/>
			{!constantOnly && (
				<>
					<ToggleField
						label={t("ui.editor.clip.reverse")}
						stored={item.reverse === true}
						disabled={!canEdit}
						commit={(next) => send(reverseOperations(itemId, next))}
					/>
					<SpeedCurve
						itemId={itemId}
						curve={item.speed_curve}
						speed={speed}
						ripple={ripple}
					/>
				</>
			)}
		</Section>
	);
}
