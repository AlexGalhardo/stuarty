import { FileText } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { useDocumentState, useRuntime } from "../../state/EditorContext";
import Section from "../Section";

// The Transcript section of the Captions tab (SDD-023 section 6): the way into the edit by text screen. The words,
// the selection and the review of the filler words are on that screen (stages/TranscriptStage and
// TranscriptReview): the transcript takes the wide region beside the preview and the review is the inspector.
export default function TranscriptPanel({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const hasAudio = useDocumentState((s) => {
		const asset = s.document?.items[itemId]?.asset;
		const found = asset === undefined ? undefined : s.document?.assets[asset];
		return found?.kind === "video" && found.has_audio !== false;
	});
	if (!hasAudio) return null;
	return (
		<Section
			title={t("ui.editor.transcript.title")}
			name="transcript"
			icon={FileText}
		>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.transcript_stage.launch_hint")}
			</p>
			<Button
				type="button"
				size="sm"
				data-testid="open-transcript-stage"
				onClick={() =>
					runtime.uiWriter.showStage({ kind: "transcript", item: itemId })
				}
			>
				{t("ui.editor.transcript_stage.launch")}
			</Button>
		</Section>
	);
}
