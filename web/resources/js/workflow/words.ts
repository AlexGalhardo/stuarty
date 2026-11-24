import { formatMoney } from "@/dashboard/approval/money";
import type { Translate } from "@/lib/i18n";
import {
	isOneOf,
	STEP_ACTORS,
	STEP_IDS,
	STEP_STATES,
	type WorkflowStep,
} from "./types";

// The operations a step names that the page has words for. Another name is the server's and is shown as it came.
const TOOLS = [
	"BriefingAnswer",
	"ProjectItemAdd",
	"TextWrite",
	"MediaImport",
	"Edit",
	"JobStart",
	"ApprovalDecide",
	"PublicationCreate",
] as const;

// A value the server did not report, or one this page does not know, reads the same way: "Not reported".
export function stateWord(t: Translate, step: WorkflowStep): string {
	return isOneOf(STEP_STATES, step.state)
		? t(`ui.workflow.state.${step.state}`)
		: t("ui.workflow.not_reported");
}

export function actorWord(t: Translate, step: WorkflowStep): string {
	return isOneOf(STEP_ACTORS, step.actor)
		? t(`ui.workflow.actor.${step.actor}`)
		: t("ui.workflow.not_reported");
}

export function toolWord(t: Translate, step: WorkflowStep): string {
	if (step.tool === undefined || step.tool === "") {
		return t("ui.workflow.not_reported");
	}
	return isOneOf(TOOLS, step.tool)
		? t(`ui.workflow.tool_name.${step.tool}`)
		: step.tool;
}

export function titleWord(t: Translate, step: WorkflowStep): string {
	return isOneOf(STEP_IDS, step.id)
		? t(`ui.workflow.step.${step.id}.title`)
		: String(step.id);
}

export function hintWord(t: Translate, step: WorkflowStep): string | undefined {
	return isOneOf(STEP_IDS, step.id)
		? t(`ui.workflow.step.${step.id}.hint`)
		: undefined;
}

// The cost of a step, one amount for each currency; a step with none reads "Not reported".
export function costWord(
	t: Translate,
	locale: string,
	step: WorkflowStep,
): string {
	if (step.cost === undefined || step.cost.length === 0) {
		return t("ui.workflow.not_reported");
	}
	return step.cost.map((cost) => formatMoney(cost, locale)).join(" + ");
}
