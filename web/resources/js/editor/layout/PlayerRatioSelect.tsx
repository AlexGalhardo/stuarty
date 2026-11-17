import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { reframeOperations, SIZE_PRESETS } from "../commands/builders";
import { useActions, useCanEdit } from "../commands/useActions";
import { useDocumentState } from "../state/EditorContext";

const CUSTOM = "custom";

// The aspect ratio selector of the player row (SDD-012 section 6.4): the size
// presets of RF-002, sent as the same `settings.reframe` the project panel
// sends. It shows the size the read model holds, which is what the core applied.
export default function PlayerRatioSelect(): ReactNode {
	const t = useT();
	const actions = useActions();
	const canEdit = useCanEdit();
	const width = useDocumentState((s) => s.document?.settings?.width);
	const height = useDocumentState((s) => s.document?.settings?.height);
	if (width === undefined || height === undefined) return null;
	const current =
		SIZE_PRESETS.find((p) => p.width === width && p.height === height)?.id ??
		CUSTOM;
	return (
		<select
			value={current}
			disabled={!canEdit}
			aria-label={t("ui.editor.preview.ratio")}
			className="h-7 rounded-md border border-input bg-transparent px-2 text-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
			onChange={(event) => {
				const chosen = SIZE_PRESETS.find((p) => p.id === event.target.value);
				if (chosen !== undefined) {
					void actions.commitField(reframeOperations(chosen.id));
				}
			}}
		>
			{SIZE_PRESETS.map((p) => (
				<option key={p.id} value={p.id}>
					{p.id}
				</option>
			))}
			{current === CUSTOM && (
				<option value={CUSTOM}>{t("ui.editor.project.custom")}</option>
			)}
		</select>
	);
}
