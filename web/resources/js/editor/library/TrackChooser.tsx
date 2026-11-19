import { ChevronDown } from "lucide-react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useT } from "@/lib/i18n";
import { useActions } from "../commands/useActions";
import { useDocumentState } from "../state/EditorContext";
import { selectOrderedTrackIds } from "../state/selectors";
import { trackLabel } from "../ui/trackLabel";

// The destination of "Add to timeline": a track of the kind the asset needs,
// or a new one. Only the id is sent; the core places the clip at the end of
// the track (VV-092).
export default function TrackChooser({
	assetId,
	name,
	kind,
	disabled,
}: {
	assetId: string;
	name: string;
	kind: "audio" | "visual";
	disabled: boolean;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	const order = useDocumentState(selectOrderedTrackIds);
	const tracks = useDocumentState((s) => s.document?.tracks);
	const kindOf = (id: string): "audio" | "visual" =>
		tracks?.[id]?.kind === "audio" ? "audio" : "visual";
	const candidates = order.filter((id) => kindOf(id) === kind);
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					type="button"
					size="icon-sm"
					variant="outline"
					disabled={disabled}
					aria-label={t("ui.editor.library.choose_track", { name })}
				>
					<ChevronDown aria-hidden="true" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuLabel>
					{t("ui.editor.library.add_to_track")}
				</DropdownMenuLabel>
				{candidates.map((id) => {
					const given = tracks?.[id]?.name;
					return (
						<DropdownMenuItem
							key={id}
							data-track-id={id}
							onSelect={() => void actions.addToTimeline(assetId, id)}
						>
							{given === undefined || given === "" ? (
								trackLabel(id, order, kindOf)
							) : (
								<UntrustedText>{given}</UntrustedText>
							)}
						</DropdownMenuItem>
					);
				})}
				{candidates.length > 0 && <DropdownMenuSeparator />}
				<DropdownMenuItem
					onSelect={() => void actions.addToTimeline(assetId, null)}
				>
					{t("ui.editor.library.new_track")}
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
}
