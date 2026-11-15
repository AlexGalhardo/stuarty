import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { useT } from "@/lib/i18n";
import { useDocumentState } from "../state/EditorContext";

// A track selected: its panel. Read only today; name, hidden, muted and the
// caption style are added by the track task.
export default function TrackPanel({
	trackId,
}: {
	trackId: string;
}): ReactNode {
	const t = useT();
	const track = useDocumentState((s) => s.document?.tracks[trackId]);
	if (track === undefined) return null;
	return (
		<dl className="grid grid-cols-[96px_1fr] gap-x-2 gap-y-1 text-sm">
			<dt className="text-xs">{t("ui.editor.inspector.name")}</dt>
			<dd>
				<UntrustedText>{track.name ?? track.id}</UntrustedText>
			</dd>
			<dt className="text-xs">{t("ui.editor.inspector.kind")}</dt>
			<dd>{t(`ui.editor.inspector.track_kind.${track.kind}`)}</dd>
		</dl>
	);
}
