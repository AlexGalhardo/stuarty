import { CornerUpLeft } from "lucide-react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { useT } from "@/lib/i18n";
import { useProductionTimelines } from "../state/timelines";

const TIMELINE_ID = /^prj_[0-9a-hjkmnp-tv-z]{10}$/;

// The way back from a timeline opened out of a composition: `from` in the
// address is the timeline that held the composition (see openComposition). Only
// an identifier of that form is linked, and it names the timeline when the
// production lists it.
export default function CompositionBreadcrumb(): ReactNode {
	const t = useT();
	const names = useProductionTimelines();
	// The page is also rendered on the server, where there is no address to read.
	const from =
		typeof window === "undefined"
			? null
			: new URLSearchParams(window.location.search).get("from");
	if (from === null || !TIMELINE_ID.test(from)) return null;
	const name = names.get(from) ?? from;
	return (
		<a
			href={`/editor/${from}`}
			data-breadcrumb="composition"
			className="flex max-w-56 items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
		>
			<CornerUpLeft aria-hidden="true" className="size-3.5 shrink-0" />
			<UntrustedText className="truncate">
				{t("ui.editor.item.from_breadcrumb", { name })}
			</UntrustedText>
		</a>
	);
}
