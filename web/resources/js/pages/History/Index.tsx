import { Head } from "@inertiajs/react";
import type { ReactNode } from "react";

import { ActivityTimeline } from "@/components/ActivityTimeline";
import { PageHeader } from "@/components/PageHeader";
import { useT } from "@/lib/i18n";

// Global Activity History page (/history, epic 111): displays recent actions
// performed by the user and by AI agents across all accessible projects.
export default function HistoryIndex(): ReactNode {
	const t = useT();
	return (
		<>
			<Head title={t("ui.history.title")} />
			<div className="mx-auto flex max-w-5xl flex-col gap-6">
				<PageHeader
					title={t("ui.history.title")}
					description={t("ui.history.intro")}
				/>
				<ActivityTimeline />
			</div>
		</>
	);
}
