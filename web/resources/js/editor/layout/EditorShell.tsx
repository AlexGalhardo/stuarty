import { type ReactNode, useCallback, useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { useLocale, useT } from "@/lib/i18n";
import LiveRegion from "../a11y/LiveRegion";
import { rowFor } from "../commands/keymapStore";
import { useSurfaceRequest } from "../commands/surface";
import { fpsOf, localizeDecimalInput, timeToFrame } from "../commands/units";
import { useShellShortcuts } from "../commands/useShellShortcuts";
import FindBar from "../find/FindBar";
import UndoRedo from "../history/UndoRedo";
import Inspector from "../inspector/Inspector";
import Preview from "../preview/Preview";
import RecordingBanner from "../record/RecordingBanner";
import CutsStage from "../stages/CutsStage";
import { CutsScope } from "../stages/useCuts";
import { TranscriptScope } from "../stages/useTranscript";
import VoiceStage from "../stages/VoiceStage";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../state/EditorContext";
import { selectFps } from "../state/selectors";
import AddCaptionDialog from "../timeline/AddCaptionDialog";
import { useCanvasPort } from "../timeline/gestures/transformDrag";
import Notices from "../timeline/Notices";
import RepeatDialog from "../timeline/RepeatDialog";
import Timeline from "../timeline/Timeline";
import { useShortcuts } from "../timeline/useShortcuts";
import { FailureView } from "../ui/FailureView";
import HistorySidebar from "./HistorySidebar";
import InUseHost from "./InUseHost";
import LeftPanel from "./LeftPanel";
import PlayerRatioSelect from "./PlayerRatioSelect";
import TopBar from "./TopBar";
import TrashedBanner from "./TrashedBanner";
import Tour from "./tour/Tour";
import { useDesktopMenu } from "./useDesktopMenu";
import { usePanelShortcuts } from "./usePanelShortcuts";

// The material region is a rail of RAIL_PX with the icon of every tool, and beside it a panel of LIBRARY_PX
// (final design 05: 44 and 240); the inspector is INSPECTOR_PX wide.
const RAIL_PX = 44;
const LIBRARY_PX = 240;
const INSPECTOR_PX = 340;

// Panel sizes by viewport width (SDD-012 section 6.3); below 1280 the drawers
// of the later phases are not built yet, so the compact sizes apply.
export interface PanelSizes {
	library: number;
	history: number;
	inspector: number;
	timeline: number;
}

// The preview keeps this much beside the open panels.
const PREVIEW_MIN_PX = 360;

// The history sidebar takes no room collapsed (its button is on the rail) and this wide open; the
// library gives way to it before the preview does.
const HISTORY_OPEN_PX = 320;

// When the two sidebars do not both fit (below 1280 px, where the editor is
// tolerated, SDD-012 section 6.3) each keeps this much, so neither is a sliver.
const SIDEBAR_MIN_PX = 192;

export function panelSizes(
	viewportWidth: number,
	historyOpen = false,
	wideLeft = false,
): PanelSizes {
	const wide = viewportWidth >= 1600;
	const inspector = viewportWidth >= 1280 ? INSPECTOR_PX : 280;
	const room = viewportWidth - inspector - PREVIEW_MIN_PX;
	const history = historyOpen
		? Math.min(HISTORY_OPEN_PX, Math.max(SIDEBAR_MIN_PX, room - SIDEBAR_MIN_PX))
		: 0;
	// The edit by text screen gives the transcript every pixel the preview and the inspector can spare.
	const libraryMax = wideLeft ? room : RAIL_PX + LIBRARY_PX;
	const library = Math.min(libraryMax, Math.max(RAIL_PX, room - history));
	return { library, history, inspector, timeline: wide ? 300 : 240 };
}

// The keys of the player (play, pause, shuttle) are rows of the registry.
const previewShortcutRow = (event: KeyboardEvent): string | null =>
	rowFor(event, ["preview"]);

const MIN_SUPPORTED_WIDTH = 1024;

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
	sceneBar = null,
	standalone = false,
}: {
	onLocaleChange(locale: string): void;
	// The scene bar of the agent, under the timeline; the timeline keeps its stored size and the preview gives way.
	sceneBar?: ReactNode;
	standalone?: boolean;
}): ReactNode {
	const t = useT();
	const canvasPort = useCanvasPort();
	const status = useDocumentState((s) => s.status);
	const locale = useLocale();
	const fps = useDocumentState(selectFps);
	const loadError = useDocumentState((s) => s.loadError);
	const viewport = useViewportWidth();
	const panels = useUiState((s) => s.panels);
	const runtime = useRuntime();
	const stage = useUiState((s) => s.stage);
	const stageItemGone = useDocumentState(
		(s) => stage !== null && s.document?.items[stage.item] === undefined,
	);
	// A stage works on one item: when the item is gone the editor is the ordinary one again.
	useEffect(() => {
		if (stageItemGone && status !== "loading") runtime.uiWriter.showStage(null);
	}, [stageItemGone, status, runtime]);
	const full = panelSizes(
		viewport,
		panels.history,
		stage?.kind === "transcript",
	);
	// The edit by text and the vertical cuts screens use the whole height: the timeline is out of sight (final design 26, 27).
	const fullHeight = stage?.kind === "transcript" || stage?.kind === "cuts";
	const sizes = {
		...full,
		timeline: fullHeight ? 0 : full.timeline,
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
		<TranscriptScope item={stage?.kind === "transcript" ? stage.item : null}>
			<CutsScope item={stage?.kind === "cuts" ? stage.item : null}>
				<div
					className="grid h-dvh min-h-0 bg-ed-bg text-foreground"
					style={{
						gridTemplateRows: `auto minmax(0, 1fr) ${sizes.timeline}px${sceneBar === null ? "" : " auto"}`,
						gridTemplateColumns: `${sizes.library}px ${sizes.history}px minmax(0, 1fr) ${sizes.inspector}px`,
					}}
				>
					<div className="col-span-full">
						<TopBar
							onLocaleChange={onLocaleChange}
							agent={sceneBar !== null}
							standalone={standalone}
						/>
						<TrashedBanner />
						<RecordingBanner />
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
					<HistorySidebar />
					<main
						aria-label={t("ui.editor.panels.preview")}
						data-landmark="3"
						className="flex min-h-0 min-w-0 flex-col bg-ed-preview-surround p-3 text-foreground"
					>
						{status === "loading" ? (
							<p role="status" className="text-sm text-muted-foreground">
								{t("ui.editor.loading")}
							</p>
						) : stage?.kind === "cuts" ? (
							<CutsStage onClose={() => runtime.uiWriter.showStage(null)} />
						) : stage?.kind === "voice" ? (
							<VoiceStage
								itemId={stage.item}
								onClose={() => runtime.uiWriter.showStage(null)}
							/>
						) : (
							<Preview
								port={canvasPort}
								parseTime={(text) =>
									timeToFrame(localizeDecimalInput(text, locale), fpsOf(fps))
								}
								trailing={<PlayerRatioSelect />}
								shortcutRow={previewShortcutRow}
							/>
						)}
					</main>
					{panels.inspector ? <Inspector /> : <div />}
					<div
						className={`col-span-full min-h-0 border-t border-ed-track-line ${fullHeight ? "invisible overflow-hidden border-t-0" : ""}`}
					>
						<Timeline
							onAddCaption={() => setCaptionOpen(true)}
							undoRedo={<UndoRedo />}
						/>
					</div>
					{sceneBar !== null && <div className="col-span-full">{sceneBar}</div>}
					<AddCaptionDialog open={captionOpen} onOpenChange={setCaptionOpen} />
					<Notices />
					<FindBar />
					<LiveRegion />
					<InUseHost />
					<RepeatDialog />
					<Tour />
				</div>
			</CutsScope>
		</TranscriptScope>
	);
}
