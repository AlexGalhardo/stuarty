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

// The cost of a step, one amount for each currency: what was spent, then what the approvals estimate; a step with
// neither reads "Not reported". The amounts are the service's: this only writes them.
export function costWord(
	t: Translate,
	locale: string,
	step: WorkflowStep,
): string {
	const amounts = (list: WorkflowStep["cost"]): string =>
		(list ?? []).map((cost) => formatMoney(cost, locale)).join(" + ");
	const spent = amounts(step.spent);
	const estimate = amounts(step.cost);
	if (spent === "" && estimate === "") return t("ui.workflow.not_reported");
	if (spent === "") return estimate;
	if (estimate === "") return t("ui.workflow.spent", { amount: spent });
	return `${t("ui.workflow.spent", { amount: spent })} · ${t("ui.workflow.estimate", { amount: estimate })}`;
}

// The skill that leads a step: a name from the library, written as text.
export function skillWord(t: Translate, step: WorkflowStep): string {
	return step.skill === undefined || step.skill === ""
		? t("ui.workflow.no_skill")
		: step.skill;
}

// The title of a step by its id, for a place that has the id and no step (a card names the step the project is at).
export function stepTitle(t: Translate, id: string): string {
	return isOneOf(STEP_IDS, id) ? t(`ui.workflow.step.${id}.title`) : id;
}
