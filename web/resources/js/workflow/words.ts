import type { Translate } from "@/lib/i18n";
import {
	isOneOf,
	STEP_ACTORS,
	STEP_IDS,
	STEP_STATES,
	type StepCost,
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

// One amount as the person reads it: the server gives millionths of a unit and a currency code, and the number is
// only formatted for the locale here. A code Intl does not know is shown after the number.
function amountWord(cost: StepCost, locale: string): string {
	const amount = cost.micros / 1_000_000;
	try {
		return new Intl.NumberFormat(locale, {
			style: "currency",
			currency: cost.currency,
		}).format(amount);
	} catch {
		return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 6 }).format(amount)} ${cost.currency}`;
	}
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
	return step.cost.map((cost) => amountWord(cost, locale)).join(" + ");
}
