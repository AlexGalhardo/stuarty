import {
	AudioLines,
	AudioWaveform,
	Captions,
	Film,
	Layers,
	ListChecks,
	type LucideIcon,
	Shapes,
	SlidersHorizontal,
	Type,
	ZoomIn,
} from "lucide-react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { useT } from "@/lib/i18n";
import { useCommandsExposed } from "../commands/useExposedCommands";
import CaptionsInspector from "../stages/CaptionsInspector";
import TranscriptReview from "../stages/TranscriptReview";
import VoiceInspector from "../stages/VoiceInspector";
import ZoomPanel from "../stages/ZoomPanel";
import { useDocumentState, useUiState } from "../state/EditorContext";
import { selectOrderedTrackIds } from "../state/selectors";
import { trackLabel } from "../ui/trackLabel";
import ProjectPanel from "./ProjectPanel";
import AudioPanel from "./panels/AudioPanel";
import CaptionPanel from "./panels/CaptionPanel";
import ClipPanel from "./panels/ClipPanel";
import CompositionPanel from "./panels/CompositionPanel";
import ShapePanel from "./panels/ShapePanel";
import TemplateSection from "./panels/TemplateSection";
import TextPanel from "./panels/TextPanel";
import TransformSection from "./panels/TransformSection";
import TrackPanel from "./TrackPanel";

const ITEM_ICON: Record<string, LucideIcon> = {
	clip: Film,
	audio: AudioLines,
	text: Type,
	caption: Captions,
	shape: Shapes,
	composition: Layers,
};

// The short name of the track an item sits on (V1, A2, or the name the person gave it), at the right of the
// header of the panel, as the final design draws it.
function TrackBadge({ trackId }: { trackId: string | undefined }): ReactNode {
	const label = useDocumentState((s) => {
		const track =
			trackId === undefined ? undefined : s.document?.tracks[trackId];
		if (trackId === undefined || track === undefined) return null;
		return trackLabel(trackId, selectOrderedTrackIds(s), (id) =>
			s.document?.tracks[id]?.kind === "audio" ? "audio" : "visual",
		);
	});
	if (label === null) return null;
	return (
		<span className="ml-auto shrink-0 font-mono text-xs text-muted-foreground tabular-nums">
			{label}
		</span>
	);
}

function ItemPanel({ itemId }: { itemId: string }): ReactNode {
	const type = useDocumentState((s) => s.document?.items[itemId]?.type);
	switch (type) {
		case "clip":
			return <ClipPanel itemId={itemId} />;
		case "audio":
			return <AudioPanel itemId={itemId} />;
		case "text":
			return <TextPanel itemId={itemId} />;
		case "caption":
			return <CaptionPanel itemId={itemId} />;
		case "shape":
			return <ShapePanel itemId={itemId} />;
		case "composition":
			return <CompositionPanel itemId={itemId} />;
		default:
			return null;
	}
}

// The inspector of SDD-012 section 12.1: one panel by selection. Nothing
// selected is the project panel, a track its panel, one item the panel of its
// type, several items only what they share.
export default function Inspector(): ReactNode {
	const t = useT();
	const selected = useUiState((s) => s.selection.items);
	const trackId = useUiState((s) => s.selection.track);
	const track = useDocumentState((s) =>
		trackId === null ? undefined : s.document?.tracks[trackId],
	);
	const allVisual = useDocumentState((s) =>
		selected.every((id) => {
			const type = s.document?.items[id]?.type;
			return type !== undefined && type !== "audio";
		}),
	);
	const only = selected.length === 1 ? selected[0] : undefined;
	const item = useDocumentState((s) =>
		only === undefined ? undefined : s.document?.items[only],
	);

	// The finishing screen (final design 25): with the Effects tab of the rail open, the inspector of a clip is the
	// panel of the zoom on the clicks.
	const effectsTab = useUiState(
		(s) => s.activePanelTab === "effects" && s.panels.left,
	);
	const zoomExposed = useCommandsExposed("recording.auto_zoom");
	const stage = useUiState((s) => s.stage);
	const captionsTab = useUiState(
		(s) => s.activePanelTab === "captions" && s.panels.left,
	);

	let title: ReactNode;
	let Icon: LucideIcon = SlidersHorizontal;
	let body: ReactNode;
	if (stage?.kind === "transcript") {
		Icon = ListChecks;
		title = t("ui.editor.transcript_review.title");
		body = <TranscriptReview />;
	} else if (stage?.kind === "voice") {
		Icon = AudioWaveform;
		title = t("ui.editor.voice_stage.title");
		body = <VoiceInspector itemId={stage.item} />;
	} else if (captionsTab && selected.length <= 1) {
		Icon = Captions;
		title = t("ui.editor.panels.captions");
		body = <CaptionsInspector />;
	} else if (
		selected.length === 1 &&
		item?.type === "clip" &&
		effectsTab &&
		zoomExposed &&
		only !== undefined
	) {
		Icon = ZoomIn;
		title = t("ui.editor.zoom_panel.title");
		body = <ZoomPanel itemId={only} />;
	} else if (selected.length > 1) {
		title = t("ui.editor.inspector.items", { count: selected.length });
		body = (
			<>
				<p className="text-sm text-muted-foreground">
					{t("ui.editor.inspector.multi_hint")}
				</p>
				{allVisual && <TransformSection itemIds={selected} />}
			</>
		);
	} else if (only !== undefined && item !== undefined) {
		Icon = ITEM_ICON[item.type] ?? SlidersHorizontal;
		title = (
			<>
				{t(`ui.editor.item.${item.type}`)}
				{item.name !== undefined && item.name !== "" && (
					<>
						{" · "}
						<UntrustedText>{item.name}</UntrustedText>
					</>
				)}
			</>
		);
		body = (
			<>
				<ItemPanel itemId={only} />
				{item.type !== "shape" && <TemplateSection key={only} itemId={only} />}
			</>
		);
	} else if (trackId !== null && track !== undefined) {
		title = t("ui.editor.inspector.track");
		body = <TrackPanel trackId={trackId} />;
	} else {
		title = t("ui.editor.inspector.project");
		body = <ProjectPanel />;
	}

	return (
		<aside
			aria-label={t("ui.editor.panels.inspector")}
			data-region="inspector"
			data-landmark="5"
			tabIndex={-1}
			className="flex min-h-0 min-w-0 flex-col gap-3 overflow-auto bg-ed-panel p-3"
		>
			<h2 className="sticky -top-3 z-10 -mx-3 -mt-3 flex shrink-0 items-center gap-2 border-b border-border bg-ed-panel px-3 py-2 text-sm font-semibold">
				<Icon
					aria-hidden="true"
					className="size-4 shrink-0 text-muted-foreground"
				/>
				<span className="min-w-0 truncate">{title}</span>
				{item !== undefined && selected.length === 1 && (
					<TrackBadge trackId={item.track} />
				)}
			</h2>
			{body}
		</aside>
	);
}
