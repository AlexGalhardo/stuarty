import { ZoomIn } from "lucide-react";
import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import type { ClipZoom } from "../api/types";
import { frameToContentX } from "./geometry";

// The height of the zoom lane: lower than a track, as the final design draws it (frame 25).
export const ZOOM_LANE_PX = 32;

// The name of the lane, Z1, at the left of the timeline, in the place a track header takes.
export function ZoomLaneHeader(): ReactNode {
	const t = useT();
	return (
		// biome-ignore lint/a11y/useSemanticElements: the row sits in a CSS-positioned grid (SDD-012 section 7.1).
		<div role="row" tabIndex={-1} className="contents">
			{/* biome-ignore lint/a11y/useSemanticElements: a rowheader has to sit in a row; the row adds no box. */}
			<div
				role="rowheader"
				tabIndex={-1}
				style={{ height: ZOOM_LANE_PX }}
				className="flex items-center gap-2 border-b border-ed-track-line px-2 text-sm text-muted-foreground"
			>
				<ZoomIn aria-hidden="true" className="size-4 shrink-0" />
				<span className="truncate">{t("ui.editor.zoom_lane.name")}</span>
			</div>
		</div>
	);
}

// The zoom lane: one numbered box for each zoom of the take, from the frame the view starts to move in to the frame
// it is back. It draws what the service read from the keyframes (`media_inspect`, section `zooms`) and sends
// nothing.
export function ZoomLane({
	zooms,
	ppf,
}: {
	zooms: readonly ClipZoom[];
	ppf: number;
}): ReactNode {
	const t = useT();
	return (
		// biome-ignore lint/a11y/useSemanticElements: the row sits in a CSS-positioned grid (SDD-012 section 7.1).
		<div
			role="row"
			tabIndex={-1}
			aria-label={t("ui.editor.zoom_lane.label", { count: zooms.length })}
			data-testid="zoom-lane"
			style={{ height: ZOOM_LANE_PX }}
			className="relative border-b border-ed-track-line"
		>
			{zooms.map((zoom) => (
				// biome-ignore lint/a11y/useSemanticElements: a cell of a CSS-positioned grid (SDD-012 section 7.1).
				<div
					key={`${zoom.item_id}-${zoom.n}`}
					role="gridcell"
					tabIndex={-1}
					data-zoom={zoom.n}
					title={t("ui.editor.zoom_panel.zoom", { n: zoom.n })}
					className="absolute top-1 flex h-6 items-center justify-center rounded-md border border-accent bg-surface-selected text-xs text-foreground tabular-nums"
					style={{
						left: frameToContentX(zoom.start.frame, ppf),
						width: Math.max(
							16,
							frameToContentX(zoom.end.frame, ppf) -
								frameToContentX(zoom.start.frame, ppf),
						),
					}}
				>
					{zoom.n}
				</div>
			))}
		</div>
	);
}
