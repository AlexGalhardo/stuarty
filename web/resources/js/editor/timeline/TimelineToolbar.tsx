import {
	AlignHorizontalJustifyStart,
	Captions,
	Link,
	Magnet,
	Maximize2,
	MousePointer2,
	Scissors,
	Slice,
	Snowflake,
	StepBack,
	StepForward,
	Trash2,
	ZoomIn,
	ZoomOut,
} from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Slider } from "@/components/ui/slider";
import { useT } from "@/lib/i18n";
import { magnetTrackId } from "../commands/magnet";
import { formatTime } from "../commands/units";
import {
	useActions,
	useCanCut,
	useCanEdit,
	useHasSelection,
} from "../commands/useActions";
import { useShortcutLabel } from "../commands/useShortcutLabel";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../state/EditorContext";
import { readTimeDisplay } from "../state/prefs";
import { selectFps } from "../state/selectors";
import {
	DEFAULT_FPS,
	type Fps,
	sliderToZoom,
	ZOOM_SLIDER_MAX,
	ZOOM_SLIDER_MIN,
	zoomToSlider,
} from "./geometry";
import TimelineTable from "./TimelineTable";

// A text button keeps its label from the 2xl breakpoint up and collapses to its
// icon below it; the name stays for assistive technology (design system 4.3.1).
function Label({ children }: { children: ReactNode }): ReactNode {
	return <span className="max-2xl:sr-only">{children}</span>;
}

// The timeline toolbar of SDD-012 section 6.4: the edit tools at the left, the
// modes and zoom at the right. Every button is the same action as its key; the
// toolbar computes no edit result.
export default function TimelineToolbar({
	undoRedo,
	onAddCaption,
	onZoomStep,
	onZoomTo,
	onFit,
	ppf,
}: {
	// The Undo and Redo buttons, passed in by the shell (they live in history/).
	undoRedo: ReactNode;
	onAddCaption(): void;
	onZoomStep(direction: 1 | -1): void;
	onZoomTo(ppf: number): void;
	onFit(): void;
	ppf: number;
}): ReactNode {
	const t = useT();
	const tip = useShortcutLabel();
	const runtime = useRuntime();
	const actions = useActions();
	const canEdit = useCanEdit();
	const canCut = useCanCut();
	const hasSelection = useHasSelection();
	const fps: Fps = useDocumentState(selectFps) ?? DEFAULT_FPS;
	const playhead = useUiState((s) => s.playheadFrame);
	const blade = useUiState((s) => s.tool === "blade");
	const snapEnabled = useUiState((s) => s.timeline.snapEnabled);
	const selection = useUiState((s) => s.selection);
	const focusedItem = useUiState((s) => s.focusedItem);
	const magnetOn = useDocumentState((s) => {
		const id = magnetTrackId(s, { selection, focusedItem });
		return id !== null && s.document?.tracks[id]?.magnet === true;
	});
	const linked = useDocumentState((s) =>
		selection.items.some((id) => s.document?.items[id]?.link !== undefined),
	);

	return (
		<div className="flex h-10 shrink-0 items-center gap-2 border-b border-ed-track-line bg-ed-panel px-3">
			{/* biome-ignore lint/a11y/useSemanticElements: a fieldset would draw a border and legend around buttons in a toolbar. */}
			<div
				role="group"
				aria-label={t("ui.editor.timeline.group_tools")}
				className="flex items-center gap-1"
			>
				<Button
					type="button"
					variant={blade ? "ghost" : "secondary"}
					size="sm"
					aria-pressed={!blade}
					title={t("ui.editor.timeline.select_hint")}
					onClick={() => runtime.uiWriter.setTool("select")}
				>
					<MousePointer2 aria-hidden="true" />
					<Label>{t("ui.editor.timeline.select")}</Label>
				</Button>
				<Button
					type="button"
					variant={blade ? "secondary" : "ghost"}
					size="sm"
					aria-pressed={blade}
					title={t("ui.editor.timeline.blade_hint")}
					onClick={() => runtime.uiWriter.setTool(blade ? "select" : "blade")}
				>
					<Slice aria-hidden="true" />
					<Label>{t("ui.editor.timeline.blade")}</Label>
				</Button>
				{undoRedo}
				<Button
					type="button"
					variant="ghost"
					size="sm"
					disabled={!canCut}
					title={tip(t("ui.editor.edit.split"), "split")}
					onClick={() => void actions.splitAtPlayhead()}
				>
					<Scissors aria-hidden="true" />
					<Label>{t("ui.editor.edit.split")}</Label>
				</Button>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					disabled={!canCut}
					title={tip(t("ui.editor.edit.trim_start"), "trim_start")}
					onClick={() => void actions.trimToPlayhead("start")}
				>
					<StepForward aria-hidden="true" />
					<Label>{t("ui.editor.edit.trim_start")}</Label>
				</Button>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					disabled={!canCut}
					title={tip(t("ui.editor.edit.trim_end"), "trim_end")}
					onClick={() => void actions.trimToPlayhead("end")}
				>
					<StepBack aria-hidden="true" />
					<Label>{t("ui.editor.edit.trim_end")}</Label>
				</Button>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					disabled={!canCut}
					onClick={() => void actions.freezeFrame()}
				>
					<Snowflake aria-hidden="true" />
					<Label>{t("ui.editor.edit.freeze_frame")}</Label>
				</Button>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					disabled={!hasSelection}
					title={tip(t("ui.editor.edit.delete"), "delete")}
					onClick={() => void actions.deleteSelection()}
				>
					<Trash2 aria-hidden="true" />
					<Label>{t("ui.editor.edit.delete")}</Label>
				</Button>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					disabled={!canEdit}
					title={tip(t("ui.editor.edit.caption"), "add_caption")}
					onClick={onAddCaption}
				>
					<Captions aria-hidden="true" />
					<Label>{t("ui.editor.edit.caption")}</Label>
				</Button>
				<span className="ml-2 font-mono text-xs text-muted-foreground tabular-nums">
					{formatTime(playhead, fps, readTimeDisplay())}
				</span>
			</div>
			{/* biome-ignore lint/a11y/useSemanticElements: a fieldset would draw a border and legend around buttons in a toolbar. */}
			<div
				role="group"
				aria-label={t("ui.editor.timeline.group_modes")}
				className="ml-auto flex items-center gap-1"
			>
				<Button
					type="button"
					variant={snapEnabled ? "secondary" : "ghost"}
					size="sm"
					aria-pressed={snapEnabled}
					title={tip(t("ui.editor.timeline.snap"), "toggle_snap")}
					onClick={runtime.uiWriter.toggleSnap}
				>
					<Magnet aria-hidden="true" />
					<Label>{t("ui.editor.timeline.snap")}</Label>
				</Button>
				<Button
					type="button"
					variant={magnetOn ? "secondary" : "ghost"}
					size="sm"
					aria-pressed={magnetOn}
					title={t("ui.editor.timeline.magnet_hint")}
					disabled={!canEdit}
					onClick={() => void actions.toggleMagnet()}
				>
					<AlignHorizontalJustifyStart aria-hidden="true" />
					<Label>{t("ui.editor.timeline.magnet")}</Label>
				</Button>
				<Button
					type="button"
					variant={linked ? "secondary" : "ghost"}
					size="sm"
					aria-pressed={linked}
					disabled={!hasSelection}
					title={tip(t("ui.editor.timeline.link_hint"), "link")}
					onClick={() => void actions.toggleLink()}
				>
					<Link aria-hidden="true" />
					<Label>{t("ui.editor.timeline.link")}</Label>
				</Button>
				<TimelineTable />
				<Button
					type="button"
					variant="ghost"
					size="icon-sm"
					aria-label={t("ui.editor.timeline.zoom_out")}
					title={tip(t("ui.editor.timeline.zoom_out"), "zoom_out")}
					onClick={() => onZoomStep(-1)}
				>
					<ZoomOut aria-hidden="true" />
				</Button>
				<Slider
					className="w-24"
					aria-label={t("ui.editor.timeline.zoom")}
					min={ZOOM_SLIDER_MIN}
					max={ZOOM_SLIDER_MAX}
					step={0.25}
					value={[zoomToSlider(ppf)]}
					onValueChange={([value]) => onZoomTo(sliderToZoom(value))}
				/>
				<Button
					type="button"
					variant="ghost"
					size="icon-sm"
					aria-label={t("ui.editor.timeline.zoom_in")}
					title={tip(t("ui.editor.timeline.zoom_in"), "zoom_in")}
					onClick={() => onZoomStep(1)}
				>
					<ZoomIn aria-hidden="true" />
				</Button>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					title={tip(t("ui.editor.timeline.fit"), "zoom_fit")}
					onClick={onFit}
				>
					<Maximize2 aria-hidden="true" />
					<Label>{t("ui.editor.timeline.fit")}</Label>
				</Button>
			</div>
		</div>
	);
}
