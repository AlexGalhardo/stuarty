import { formatMoney } from "@/dashboard/approval/money";
import type { Translate } from "@/lib/i18n";
import {
	isOneOf,
	STEP_ACTORS,
	STEP_IDS,
	STEP_STATES,
	type StepCost,
	type Workflow,
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

// Formats one list of server-reported amounts: zero reads "Free" ("Grátis") rather than "$0.00".
function formatStepAmounts(
	t: Translate,
	locale: string,
	list: readonly StepCost[] | undefined,
): string {
	if (list === undefined || list.length === 0) return "";
	const nonzero = list.filter((cost) => cost.micros !== 0);
	if (nonzero.length === 0) return t("ui.workflow.free");
	return nonzero.map((cost) => formatMoney(cost, locale)).join(" + ");
}

// The cost of a step, one amount for each currency: what was spent, then what the approvals estimate; a step with
// neither reads "Not reported". Zero cost reads "Free". The amounts are the service's: this only writes them.
export function costWord(
	t: Translate,
	locale: string,
	step: WorkflowStep,
): string {
	const spent = formatStepAmounts(t, locale, step.spent);
	const estimate = formatStepAmounts(t, locale, step.cost);
	if (spent === "" && estimate === "") return t("ui.workflow.not_reported");
	if (spent === "") return estimate;
	const spentFree = (step.spent ?? []).every((cost) => cost.micros === 0);
	const spentText = spentFree
		? spent
		: t("ui.workflow.spent", { amount: spent });
	if (estimate === "") return spentText;
	return `${spentText} · ${t("ui.workflow.estimate", { amount: estimate })}`;
}

// The estimated cost chip of the production flow, read directly from the server-computed approval cost on the
// workflow (core/project/workflow.go). No arithmetic is done here: zero or no reported approval cost reads "Free",
// and an undecided workflow with no approval step omits the chip.
export function estimatedCostWord(
	t: Translate,
	locale: string,
	workflow: Workflow,
): string | null {
	if (!workflow.steps.some((step) => step.id === "approval")) return null;
	const reported = workflow.steps.find(
		(step) => step.cost !== undefined && step.cost.length > 0,
	)?.cost;
	const amount =
		reported === undefined
			? t("ui.workflow.free")
			: formatStepAmounts(t, locale, reported);
	return t("ui.workflow.summary.estimated_cost", { amount });
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
