import { MoveHorizontal } from "lucide-react";
import type { ReactNode } from "react";

import { ContextMenuItem } from "@/components/ui/context-menu";
import { useT } from "@/lib/i18n";
import { useActions } from "../commands/useActions";

// The menu of an empty place on a track: "Close gap" at the frame under the
// pointer. Whether there is a gap to close is the core's to say.
export default function GapContextMenuItems({
	trackId,
	frame,
}: {
	trackId: string;
	frame: number;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	return (
		<ContextMenuItem onSelect={() => void actions.closeGap(trackId, frame)}>
			<MoveHorizontal aria-hidden="true" />
			{t("ui.editor.edit.close_gap")}
		</ContextMenuItem>
	);
}
