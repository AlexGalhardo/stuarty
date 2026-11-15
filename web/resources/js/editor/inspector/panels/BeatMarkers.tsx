import { type ReactNode, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { markBeatsOperations } from "../../commands/builders";
import { useActions, useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import { useDocumentState } from "../../state/EditorContext";
import NumberField from "../fields/NumberField";
import ToggleField from "../fields/ToggleField";
import Section from "../Section";

// The Beat markers section of an audio item (SDD-012 section 12.1, BETA): which
// beats to mark and one button that sends `audio.mark_beats`. The beats come
// from the analysis of the asset, supplied by the service; the markers the core
// writes are drawn as ticks on the item (`MarkerTicks`).
export default function BeatMarkers({ itemId }: { itemId: string }): ReactNode {
	const t = useT();
	const actions = useActions();
	const canEdit = useCanEdit();
	const exposed = useCommandsExposed("audio.mark_beats");
	const count = useDocumentState(
		(s) => s.document?.items[itemId]?.markers?.length ?? 0,
	);
	const [every, setEvery] = useState(1);
	const [label, setLabel] = useState("");
	const [replace, setReplace] = useState(true);
	const [busy, setBusy] = useState(false);
	if (!exposed) return null;
	return (
		<Section title={t("ui.editor.audio.beats_title")}>
			<NumberField
				label={t("ui.editor.audio.beats_every")}
				stored={String(every)}
				disabled={!canEdit}
				step={1}
				min={1}
				max={64}
				commit={async (value) => {
					setEvery(Math.round(value));
					return { ok: true };
				}}
			/>
			<div className="flex flex-col gap-1">
				<Label htmlFor={`beats-label-${itemId}`} className="text-xs">
					{t("ui.editor.audio.beats_label")}
				</Label>
				<Input
					id={`beats-label-${itemId}`}
					value={label}
					maxLength={60}
					disabled={!canEdit}
					className="h-8"
					onChange={(event) => setLabel(event.target.value)}
				/>
			</div>
			<ToggleField
				label={t("ui.editor.audio.beats_replace")}
				stored={replace}
				disabled={!canEdit}
				commit={async (next) => {
					setReplace(next);
					return { ok: true };
				}}
			/>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.audio.beats_count", { count })}
			</p>
			<Button
				type="button"
				variant="outline"
				size="sm"
				disabled={!canEdit || busy}
				onClick={async () => {
					setBusy(true);
					try {
						await actions.runOperations(
							markBeatsOperations(itemId, { every, label, replace }),
						);
					} finally {
						setBusy(false);
					}
				}}
			>
				{t("ui.editor.audio.beats_apply")}
			</Button>
		</Section>
	);
}
