// The workflow of a project as the service reports it (SDD-000 section 1, epic 78). Every value here is the
// server's: the kind, the steps, their states, who does them, their tools and their costs. The screens draw them
// and compute nothing.

export type ProjectKind = "undecided" | "text" | "video" | "image";

export const STEP_IDS = [
	"briefing",
	"choice",
	"script",
	"writing",
	"recording",
	"editing",
	"finishing",
	"review",
	"approval",
	"publication",
] as const;
export type StepId = (typeof STEP_IDS)[number];

export const STEP_STATES = [
	"done",
	"current",
	"waiting",
	"upcoming",
	"skipped",
	"unknown",
] as const;
export type StepState = (typeof STEP_STATES)[number];

export const STEP_ACTORS = ["person", "agent", "both"] as const;
export type StepActor = (typeof STEP_ACTORS)[number];

export interface StepCost {
	micros: number;
	currency: string;
}

export interface WorkflowStep {
	id: StepId;
	state: StepState;
	optional: boolean;
	actor: StepActor;
	tool?: string;
	// What the approvals of the step estimate.
	cost?: StepCost[];
	// The skill of the library that leads the step; the service leaves it out when the person has none reviewed.
	skill?: string;
	// What the finished jobs of the step cost, by currency; left out when nothing was spent.
	spent?: StepCost[];
}

export interface Workflow {
	kind: ProjectKind;
	steps: WorkflowStep[];
	current?: StepId;
	next?: StepId;
}

// What a list of projects says of the text a project holds (SDD-000 section 1, epic 78). The excerpt is untrusted.
export interface TextSummary {
	item_id: string;
	text_kind: string;
	words: number;
	reading_minutes: number;
	excerpt: string;
}

export function isOneOf<T extends string>(
	list: readonly T[],
	value: unknown,
): value is T {
	return (
		typeof value === "string" && (list as readonly string[]).includes(value)
	);
}
