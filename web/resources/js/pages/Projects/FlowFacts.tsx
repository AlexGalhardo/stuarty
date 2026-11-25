import type { ReactNode } from "react";

import { StatusBadge } from "@/components/StatusBadge";
import { useT } from "@/lib/i18n";
import type { TextSummary } from "../../workflow/types";

// The text facts of the production summary strip: how long the text is and how long it takes to read.
// They come with the page props and are drawn as the service gave them: no word count and no reading time
// is worked out here.
export default function FlowFacts({
	text,
}: {
	text?: TextSummary | null;
}): ReactNode {
	const t = useT();

	if (!text) return null;
	return (
		<>
			<li data-testid="flow-words">
				<StatusBadge status="neutral" className="px-3 py-1 text-sm">
					{t("ui.projects.card.words", { count: text.words })}
				</StatusBadge>
			</li>
			<li data-testid="flow-reading">
				<StatusBadge status="neutral" className="px-3 py-1 text-sm">
					{t("ui.projects.card.reading", { count: text.reading_minutes })}
				</StatusBadge>
			</li>
		</>
	);
}
