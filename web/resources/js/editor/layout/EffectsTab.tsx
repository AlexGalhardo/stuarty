import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { useCommandsExposed } from "../commands/useExposedCommands";
import {
	ScreenCursorSection,
	ScreenHighlightSection,
} from "../inspector/panels/ScreenTakeSection";
import EffectsPanel from "../library/EffectsPanel";

// The cursor and the click highlight of a screen recording (SDD-002 C.94, C.95): the final design puts them in
// the Effects tab of the rail, over the effect cards, for the selected clip. The zoom on the clicks is the
// Adjust tab of the inspector.
function CursorClicks({ itemId }: { itemId: string }): ReactNode {
	const t = useT();
	const cursor = useCommandsExposed("recording.set_cursor");
	const highlight = useCommandsExposed("recording.set_click_highlight");
	if (!cursor && !highlight) return null;
	return (
		<div className="flex flex-col gap-2" data-testid="cursor-clicks">
			<h3 className="text-xs font-semibold text-muted-foreground">
				{t("ui.editor.panels.cursor_clicks")}
			</h3>
			<ScreenCursorSection itemId={itemId} />
			<ScreenHighlightSection itemId={itemId} />
		</div>
	);
}

export default function EffectsTab(): ReactNode {
	return <EffectsPanel lead={(itemId) => <CursorClicks itemId={itemId} />} />;
}
