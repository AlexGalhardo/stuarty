import type { ReactNode } from "react";

import {
	type StepState as MarkState,
	NumberedStep,
} from "@/components/NumberedStep";
import { useT } from "@/lib/i18n";
import type { StepState, WorkflowStep } from "./types";
import { stateWord, titleWord } from "./words";

// How the numbered mark draws each state of the server. The mark only chooses a look: which step is done or
// current is the server's word. A step that waits for a person is drawn as the current one.
const MARK: Record<StepState, MarkState> = {
	done: "done",
	current: "current",
	waiting: "current",
	upcoming: "todo",
	skipped: "todo",
	unknown: "todo",
};

// One step of the trail, an item of its ordered list: the number, the title and the state in words.
export function TrailStep({
	step,
	number,
}: {
	step: WorkflowStep;
	number: number;
}): ReactNode {
	const t = useT();
	return (
		<NumberedStep
			number={number}
			title={titleWord(t, step)}
			state={MARK[step.state] ?? "todo"}
			stateLabel={stateWord(t, step)}
			className="min-w-0"
		/>
	);
}
