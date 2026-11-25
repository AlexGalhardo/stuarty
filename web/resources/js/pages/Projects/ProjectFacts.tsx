import { CalendarClock, Route } from "lucide-react";
import type { ReactNode } from "react";

import { formatDateTime } from "@/lib/format";
import { useT } from "@/lib/i18n";
import type { ProjectSummary } from "@/types";
import { stepTitle } from "../../workflow/words";

// What a card says of where the project is: the step it stands at and when its next post is due, both as the
// service reported them. Nothing is worked out here: a project with no plan has neither line.
export default function ProjectFacts({
	project,
	locale,
}: {
	project: ProjectSummary;
	locale: string;
}): ReactNode {
	const t = useT();
	const step = project.current_step;
	const next = project.next_post_at;
	if (!step && !next) return null;
	return (
		<ul className="flex flex-col gap-0.5 text-xs text-text-muted">
			{step ? (
				<li className="flex items-center gap-1" data-testid="card-step">
					<Route aria-hidden="true" className="size-3 shrink-0" />
					{t("ui.projects.card.now", { step: stepTitle(t, step) })}
				</li>
			) : null}
			{next ? (
				<li className="flex items-center gap-1" data-testid="card-next-post">
					<CalendarClock aria-hidden="true" className="size-3 shrink-0" />
					{t("ui.projects.card.next_post", {
						date: formatDateTime(next, locale),
					})}
				</li>
			) : null}
		</ul>
	);
}
