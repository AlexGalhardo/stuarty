import { ScanFace } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { useDocumentState, useRuntime } from "../../state/EditorContext";
import Section from "../Section";

// The Vertical cuts section of a clip (SDD-024 section 8): the way into the vertical cuts screen. The face model, the
// search for the faces, the proposed cuts and their acceptance are on that screen (stages/CutsStage and CutsPanels),
// where the proposals are cards on the stage and the review is at the right.
export default function VerticalCuts({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const isVideo = useDocumentState((s) => {
		const asset = s.document?.items[itemId]?.asset;
		return asset !== undefined && s.document?.assets[asset]?.kind === "video";
	});
	if (!isVideo) return null;
	return (
		<Section title={t("ui.editor.vertical_cuts.title")} icon={ScanFace}>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.vertical_cuts.hint")}
			</p>
			<div>
				<Button
					type="button"
					size="sm"
					data-testid="open-cuts-stage"
					onClick={() =>
						runtime.uiWriter.showStage({ kind: "cuts", item: itemId })
					}
				>
					{t("ui.editor.vertical_cuts.open")}
				</Button>
			</div>
		</Section>
	);
}
