import { Trash2 } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import {
	markerRemoveOperations,
	markerSetOperations,
} from "../../commands/builders";
import { formatTime, fpsOf } from "../../commands/units";
import { useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import { useDocumentState } from "../../state/EditorContext";
import { readTimeDisplay } from "../../state/prefs";
import TextField from "../fields/TextField";
import TimeCommitField from "../fields/TimeCommitField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

// The markers of the timeline, or of one item when `itemId` is given: the label
// and the position of each (`marker.set`) and a remove button (`marker.remove`).
// The list is what the core stores; nothing is shown when there is no marker.
export default function MarkersSection({
	itemId,
}: {
	itemId?: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const exposed = useCommandsExposed("marker.set");
	const rate = useDocumentState((s) => s.document?.settings.fps);
	const markers = useDocumentState((s) =>
		itemId === undefined
			? s.document?.markers
			: s.document?.items[itemId]?.markers,
	);
	if (!exposed || markers === undefined || markers.length === 0) return null;
	const fps = fpsOf(rate);
	return (
		<Section title={t("ui.editor.markers.title", { count: markers.length })}>
			<ul className="flex max-h-64 flex-col gap-2 overflow-y-auto">
				{markers.map((marker) => (
					<li
						key={marker.frame}
						data-marker-row={marker.frame}
						className="flex flex-col gap-1 rounded-md border p-2"
					>
						<TimeCommitField
							label={t("ui.editor.markers.position")}
							stored={formatTime(marker.frame, fps, readTimeDisplay())}
							disabled={!canEdit}
							commit={(to) =>
								send(markerSetOperations(marker.frame, { to }, itemId))
							}
						/>
						<TextField
							label={t("ui.editor.markers.label")}
							stored={marker.label ?? ""}
							maxLength={60}
							disabled={!canEdit}
							commit={(label) =>
								send(markerSetOperations(marker.frame, { label }, itemId))
							}
						/>
						<Button
							type="button"
							variant="ghost"
							size="sm"
							disabled={!canEdit}
							onClick={() =>
								void send(markerRemoveOperations(marker.frame, itemId))
							}
						>
							<Trash2 aria-hidden="true" />
							{t("ui.editor.markers.remove")}
						</Button>
					</li>
				))}
			</ul>
		</Section>
	);
}
