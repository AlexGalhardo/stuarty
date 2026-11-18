import { type ReactNode, useCallback, useEffect, useState } from "react";

import ReleaseMark from "@/components/ReleaseMark";
import { Button } from "@/components/ui/button";
import { useLocale, useT } from "@/lib/i18n";
import LiveRegion from "../a11y/LiveRegion";
import { useSurfaceRequest } from "../commands/surface";
import { fpsOf, localizeDecimalInput, timeToFrame } from "../commands/units";
import { useShellShortcuts } from "../commands/useShellShortcuts";
import FindBar from "../find/FindBar";
import UndoRedo from "../history/UndoRedo";
import Inspector from "../inspector/Inspector";
import Preview from "../preview/Preview";
import { useDocumentState, useUiState } from "../state/EditorContext";
import { selectFps } from "../state/selectors";
import EditToolbar from "../timeline/EditToolbar";
import { useCanvasPort } from "../timeline/gestures/transformDrag";
import Notices from "../timeline/Notices";
import RepeatDialog from "../timeline/RepeatDialog";
import Timeline from "../timeline/Timeline";
import { useShortcuts } from "../timeline/useShortcuts";
import { FailureView } from "../ui/FailureView";
import FirstRunTips from "./FirstRunTips";
import InUseHost from "./InUseHost";
import LeftPanel from "./LeftPanel";
import PlayerRatioSelect from "./PlayerRatioSelect";
import TopBar from "./TopBar";
import TrashedBanner from "./TrashedBanner";
import { useDesktopMenu } from "./useDesktopMenu";
import { usePanelShortcuts } from "./usePanelShortcuts";

// Panel sizes by viewport width (SDD-012 section 6.3); below 1280 the drawers
// of the later phases are not built yet, so the compact sizes apply.
export interface PanelSizes {
	library: number;
	inspector: number;
	timeline: number;
}

export function panelSizes(viewportWidth: number): PanelSizes {
	if (viewportWidth >= 1600)
		return { library: 320, inspector: 360, timeline: 300 };
	return { library: 240, inspector: 280, timeline: 240 };
}

const MIN_SUPPORTED_WIDTH = 1024;
// The collapsed material region keeps its tab rail (docs/design-system.md 3.4.4).
const RAIL_PX = 40;

function useViewportWidth(): number {
	const [width, setWidth] = useState(() => window.innerWidth);
	useEffect(() => {
		const onResize = (): void => setWidth(window.innerWidth);
		window.addEventListener("resize", onResize);
		return () => window.removeEventListener("resize", onResize);
	}, []);
	return width;
}

// The grid of SDD-012 section 6.1: top bar, library, preview, inspector and
// the timeline across the bottom. The library, the preview and the inspector
// are filled by their own tasks; here they are labelled regions.
export default function EditorShell({
	onLocaleChange,
}: {
	onLocaleChange(locale: string): void;
}): ReactNode {
	const t = useT();
	const canvasPort = useCanvasPort();
	const status = useDocumentState((s) => s.status);
	const locale = useLocale();
	const fps = useDocumentState(selectFps);
	const loadError = useDocumentState((s) => s.loadError);
	const viewport = useViewportWidth();
	const panels = useUiState((s) => s.panels);
	const full = panelSizes(viewport);
	const sizes = {
		...full,
		library: panels.left ? full.library : RAIL_PX,
		inspector: panels.inspector ? full.inspector : 0,
	};
	usePanelShortcuts();
	useShellShortcuts();
	const [captionOpen, setCaptionOpen] = useState(false);
	useShortcuts(() => setCaptionOpen(true));
	useDesktopMenu();
	const openCaption = useCallback(() => setCaptionOpen(true), []);
	useSurfaceRequest("add_caption", openCaption);

	if (viewport < MIN_SUPPORTED_WIDTH) {
		return (
			<main className="flex h-dvh flex-col items-center justify-center gap-4 p-6 text-center">
				<p>{t("ui.editor.unsupported_size")}</p>
				<Button asChild variant="outline">
					<a href="/projects">{t("ui.editor.back")}</a>
				</Button>
			</main>
		);
	}

	return (
		<div
			className="grid h-dvh min-h-0 bg-ed-bg text-foreground"
			style={{
				gridTemplateRows: `auto minmax(0, 1fr) ${sizes.timeline}px`,
				gridTemplateColumns: `${sizes.library}px minmax(0, 1fr) ${sizes.inspector}px`,
			}}
		>
			<div className="col-span-full">
				<TopBar onLocaleChange={onLocaleChange} />
				<TrashedBanner />
				{/* A landmark, so the toolbar is not content outside regions (axe region). */}
				<section
					aria-label={t("ui.editor.edit.label")}
					className="border-b border-ed-track-line bg-ed-panel px-3 py-1"
				>
					<EditToolbar
						captionOpen={captionOpen}
						onCaptionOpenChange={setCaptionOpen}
					/>
				</section>
				<FirstRunTips />
				{status === "failed" && (
					<div
						role="alert"
						className="col-span-full flex items-center gap-3 border-b border-destructive bg-ed-panel px-3 py-2 text-sm text-destructive"
					>
						{loadError === null ? (
							t("ui.editor.banner.failed")
						) : (
							<div className="flex flex-col">
								<span>{t("ui.editor.banner.load_failed")}</span>
								<FailureView failure={loadError} />
							</div>
						)}
						<Button
							type="button"
							size="sm"
							variant="outline"
							onClick={() => location.reload()}
						>
							{t("ui.editor.banner.reload")}
						</Button>
					</div>
				)}
			</div>
			<LeftPanel />
			<main
				aria-label={t("ui.editor.panels.preview")}
				data-landmark="3"
				className="flex min-h-0 min-w-0 flex-col bg-ed-preview-surround p-3 text-foreground"
			>
				{status === "loading" ? (
					<p role="status" className="text-sm text-muted-foreground">
						{t("ui.editor.loading")}
					</p>
				) : (
					<Preview
						port={canvasPort}
						parseTime={(text) =>
							timeToFrame(localizeDecimalInput(text, locale), fpsOf(fps))
						}
						trailing={<PlayerRatioSelect />}
					/>
				)}
			</main>
			{panels.inspector ? <Inspector /> : <div />}
			<div className="col-span-full min-h-0 border-t border-ed-track-line">
				<Timeline
					onAddCaption={() => setCaptionOpen(true)}
					undoRedo={<UndoRedo />}
				/>
			</div>
			<ReleaseMark />
			<Notices />
			<FindBar />
			<LiveRegion />
			<InUseHost />
			<RepeatDialog />
		</div>
	);
}
