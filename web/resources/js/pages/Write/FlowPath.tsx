import type { ReactNode } from "react";

import { NumberedStep, type StepState } from "@/components/NumberedStep";
import { useT } from "@/lib/i18n";
import type { ProjectFlow } from "./client";
import { pathSteps } from "./meta";

const STATE: Record<string, StepState> = {
	done: "done",
	current: "current",
	waiting: "current",
};

// The path of the project from its briefing to the end of its text, as the service computed it: each step with who
// does it (a person, the agent or both) and its state in a word. The page draws the list and decides no state.
export default function FlowPath({
	flow,
}: {
	flow: ProjectFlow | null;
}): ReactNode {
	const t = useT();
	const steps = pathSteps(flow);
	if (steps.length === 0) return null;
	return (
		<section
			aria-label={t("ui.write.flow.label")}
			data-testid="flow-path"
			className="rounded-lg border border-border bg-surface px-4 py-3"
		>
			<p className="mb-2 text-sm font-semibold text-text-muted">
				{t("ui.write.flow.label")}
			</p>
			<ol className="grid gap-x-4 gap-y-3 sm:grid-cols-2 xl:grid-cols-3">
				{steps.map((step, index) => (
					<NumberedStep
						key={step.id}
						number={index + 1}
						title={t(`ui.write.flow.step.${step.id}`)}
						description={`${t(`ui.write.flow.actor.${step.actor}`)}${step.optional ? ` · ${t("ui.write.flow.optional")}` : ""}`}
						state={STATE[step.state] ?? "todo"}
						stateLabel={t(`ui.write.flow.state.${step.state}`)}
					/>
				))}
			</ol>
		</section>
	);
}
