import { ZoomIn } from "lucide-react";
import { type ReactNode, useState } from "react";

import { Switch } from "@/components/ui/switch";
import { useT } from "@/lib/i18n";
import { autoZoomOperations, zoomClearOperations } from "../commands/builders";
import { formatTime, fpsOf } from "../commands/units";
import { useActions, useCanEdit } from "../commands/useActions";
import { useCommandsExposed } from "../commands/useExposedCommands";
import Section, { ToolLine } from "../inspector/Section";
import { useDocumentState } from "../state/EditorContext";
import { selectFps } from "../state/selectors";
import { useZooms } from "./useZooms";

// The panel "Zoom on click" of the finishing screen (final design 25): the switch that makes the zooms from the
// clicks of the take (`recording.auto_zoom`, or the three keyframe.clear that put the clip back at rest), and the
// list of the zooms the clip holds as the service reads them. The count and the times are the service's.
export default function ZoomPanel({ itemId }: { itemId: string }): ReactNode {
	const t = useT();
	const actions = useActions();
	const canEdit = useCanEdit();
	const exposed = useCommandsExposed("recording.auto_zoom");
	const fps = useDocumentState(selectFps);
	const assetId = useDocumentState((s) => s.document?.items[itemId]?.asset);
	const zooms = useZooms(assetId === undefined ? [] : [assetId]).filter(
		(z) => z.item_id === itemId,
	);
	const [busy, setBusy] = useState(false);
	if (!exposed) return null;
	const on = zooms.length > 0;
	const toggle = async (next: boolean): Promise<void> => {
		setBusy(true);
		try {
			await actions.runOperations(
				next ? autoZoomOperations(itemId, {}) : zoomClearOperations(itemId),
			);
		} finally {
			setBusy(false);
		}
	};
	return (
		<>
			<Section
				title={t("ui.editor.zoom_panel.auto")}
				name="zoom-auto"
				icon={ZoomIn}
			>
				<ToolLine name="recording.auto_zoom" />
				<div className="flex items-center justify-between gap-2 text-sm">
					<label htmlFor="zoom-use-clicks">
						{t("ui.editor.zoom_panel.use_clicks")}
					</label>
					<span className="flex items-center gap-2">
						<Switch
							id="zoom-use-clicks"
							checked={on}
							disabled={!canEdit || busy}
							onCheckedChange={(next) => void toggle(next)}
						/>
						<span className="w-12 text-sm">
							{on
								? t("ui.editor.zoom_panel.on")
								: t("ui.editor.zoom_panel.off")}
						</span>
					</span>
				</div>
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.zoom_panel.hint")}
				</p>
			</Section>
			<Section
				title={t("ui.editor.zoom_panel.list", { count: zooms.length })}
				name="zoom-list"
			>
				{zooms.length === 0 ? (
					<p className="text-xs text-muted-foreground">
						{t("ui.editor.zoom_panel.none")}
					</p>
				) : (
					<ol className="flex flex-col gap-1.5" data-testid="zoom-list">
						{zooms.map((zoom) => (
							<li
								key={`${zoom.item_id}-${zoom.n}`}
								className="flex items-center justify-between gap-2 text-sm"
							>
								<span className="text-muted-foreground">
									{t("ui.editor.zoom_panel.zoom", { n: zoom.n })}
								</span>
								<span className="w-28 rounded-md border border-border bg-surface-sunken px-2 py-1 font-mono text-xs tabular-nums">
									{formatTime(zoom.start.frame, fpsOf(fps), "timecode")}
								</span>
							</li>
						))}
					</ol>
				)}
			</Section>
		</>
	);
}
