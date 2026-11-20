import { type ReactNode, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { useT } from "@/lib/i18n";
import { useDocumentState } from "../../state/EditorContext";
import { selectItem } from "../../state/selectors";
import { useProductionTimelines } from "../../state/timelines";

// The label of a composition item: a small picture of the first frame of the
// timeline it uses and that timeline's name, or the name of the item when it
// has one (SDD-012 section 7.4). The picture is asked for without a revision,
// so the server answers with the current frame of the nested timeline.
export default function CompositionItem({ id }: { id: string }): ReactNode {
	const t = useT();
	const item = useDocumentState((s) => selectItem(s, id));
	const names = useProductionTimelines();
	const [pictureFailed, setPictureFailed] = useState(false);
	if (item === undefined || item.timeline === undefined) return null;
	const title = names.get(item.timeline);
	const label =
		item.name ??
		title ??
		(names.size > 0
			? t("ui.editor.item.composition_missing")
			: t("ui.editor.item.composition"));
	const target = encodeURIComponent(item.timeline);
	return (
		<>
			{!pictureFailed && title !== undefined && (
				<img
					aria-hidden="true"
					alt=""
					loading="lazy"
					className="relative z-10 mr-1 h-full w-auto rounded-sm object-cover"
					src={`/projects/${target}/frame?at=0f&width=96&timeline_id=${target}`}
					onError={() => setPictureFailed(true)}
				/>
			)}
			<UntrustedText
				// The cell already carries the name; the text would be read twice.
				aria-hidden="true"
				className="relative z-10 truncate rounded bg-background/70 px-1"
			>
				{label}
			</UntrustedText>
		</>
	);
}
