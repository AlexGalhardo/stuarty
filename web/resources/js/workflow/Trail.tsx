import type { ReactNode } from "react";

import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { TrailStep } from "./TrailStep";
import type { Workflow } from "./types";

// The path of a project, from its briefing to its publication, in the order and with the states the server gave.
// The list is the workflow as it came: nothing is added, hidden or reordered here.
export function Trail({
	workflow,
	className,
}: {
	workflow: Workflow;
	className?: string;
}): ReactNode {
	const t = useT();
	return (
		<ol
			data-slot="trail"
			data-kind={workflow.kind}
			aria-label={t("ui.workflow.label")}
			className={cn(
				"grid gap-x-4 gap-y-3 sm:grid-cols-[repeat(auto-fill,minmax(14rem,1fr))]",
				className,
			)}
		>
			{workflow.steps.map((step, index) => (
				<TrailStep key={step.id} step={step} number={index + 1} />
			))}
		</ol>
	);
}
