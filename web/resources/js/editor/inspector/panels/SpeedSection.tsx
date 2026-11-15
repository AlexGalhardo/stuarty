import { type ReactNode, useState } from "react";

import { useT } from "@/lib/i18n";
import { speedOperations } from "../../commands/builders";
import { speedText } from "../../commands/units";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import NumberField from "../fields/NumberField";
import ToggleField from "../fields/ToggleField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

// The Speed section of SDD-012 section 12.1, for a clip only: the speed with an
// `x` suffix and the choice to move the later items of the track with it. The
// toggle is a choice for the next change, not a command of its own.
export default function SpeedSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const [ripple, setRipple] = useState(false);
	const speed = useDocumentState((s) => s.document?.items[itemId]?.speed);
	const found = useDocumentState(
		(s) => s.document?.items[itemId] !== undefined,
	);
	if (!found) return null;
	return (
		<Section title={t("ui.editor.clip.speed_title")}>
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
		</Section>
	);
}
