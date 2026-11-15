import { type ReactNode, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { silenceOperations } from "../../commands/builders";
import { useActions, useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import TimeCommitField from "../fields/TimeCommitField";
import ToggleField from "../fields/ToggleField";
import Section from "../Section";

// What the core uses when a request names no minimum or padding (SDD-002 C.27);
// the fields start there and the choices are sent with the button.
const DEFAULT_MIN_SILENCE = "0.4s";
const DEFAULT_PADDING = "0.1s";

// The Silences section of SDD-012 section 12.1 for a clip with audio: the
// choices and one button. The service measures the asset and resolves the
// ranges; the browser sends only the choices. The result is one undo step.
export default function SilencesSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	const canEdit = useCanEdit();
	const [minSilence, setMinSilence] = useState(DEFAULT_MIN_SILENCE);
	const [padding, setPadding] = useState(DEFAULT_PADDING);
	const [ripple, setRipple] = useState(true);
	const [busy, setBusy] = useState(false);
	const hasAudio = useDocumentState((s) => {
		const asset = s.document?.items[itemId]?.asset;
		return asset === undefined
			? false
			: s.document?.assets[asset]?.has_audio === true;
	});
	if (!hasAudio) return null;
	return (
		<Section title={t("ui.editor.clip.silences_title")}>
			<TimeCommitField
				label={t("ui.editor.clip.min_silence")}
				stored={minSilence}
				disabled={!canEdit}
				commit={async (value) => {
					setMinSilence(value);
					return { ok: true };
				}}
			/>
			<TimeCommitField
				label={t("ui.editor.clip.padding")}
				stored={padding}
				disabled={!canEdit}
				commit={async (value) => {
					setPadding(value);
					return { ok: true };
				}}
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
			<Button
				type="button"
				variant="outline"
				size="sm"
				disabled={!canEdit || busy}
				onClick={async () => {
					setBusy(true);
					try {
						await actions.runOperations(
							silenceOperations(itemId, { minSilence, padding, ripple }),
						);
					} finally {
						setBusy(false);
					}
				}}
			>
				{t("ui.editor.clip.remove_silences")}
			</Button>
		</Section>
	);
}
