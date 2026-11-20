import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { useT } from "@/lib/i18n";
import { useDocumentState, useUiState } from "../state/EditorContext";
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

	let title: ReactNode;
	let body: ReactNode;
	if (selected.length > 1) {
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
			<h2 className="shrink-0 truncate text-sm font-semibold">{title}</h2>
			{body}
		</aside>
	);
}
