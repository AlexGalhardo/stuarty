import type { ReactNode } from "react";

import { StatusBadge } from "@/components/StatusBadge";
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import type { TextSummary } from "../../workflow/types";

// The facts of the head of a production: how long the text is, how long it takes to read and when the next post is
// due. They come with the page props and are drawn as the service gave them: no word count, no reading time and no
// date is worked out here.
export default function FlowFacts({
	text,
	next,
}: {
	text?: TextSummary | null;
	next?: string | null;
}): ReactNode {
	const t = useT();
	const locale = useLocale();

	if (!text && !next) return null;
	return (
		<>
			{text ? (
				<>
					<span data-testid="flow-words">
						<StatusBadge status="neutral">
							{t("ui.projects.card.words", { count: text.words })}
						</StatusBadge>
					</span>
					<span data-testid="flow-reading">
						<StatusBadge status="neutral">
							{t("ui.projects.card.reading", { count: text.reading_minutes })}
						</StatusBadge>
					</span>
				</>
			) : null}
			{next ? (
				<span data-testid="flow-next-post">
					<StatusBadge status="info">
						{t("ui.projects.card.next_post", {
							date: formatDateTime(next, locale),
						})}
					</StatusBadge>
				</span>
			) : null}
		</>
	);
}
