import { z } from "zod";

import { PROJECT_SCHEMA } from "@/generated/project-schema";
import type { ProjectContextInput } from "../../editor/api/types";

// zod compiles a faster parser with new Function, which the content security policy of the app refuses (script-src
// has no unsafe-eval): the schemas run in the interpreter.
z.config({ jitless: true });

// The zod schema of the context of a project. It is built from the JSON Schema that `task codegen` exports from
// core/project (generated/project-schema.ts), so the form and the core never drift: the page decides nothing, it
// shows early what the core will say (SDD-012 section 1; epic 56).

interface TextRule {
	readonly minLength: number;
	readonly maxLength: number;
	readonly pattern?: string;
}

function text(rule: TextRule): z.ZodString {
	let s = z.string().min(rule.minLength).max(rule.maxLength);
	if (rule.pattern !== undefined) {
		s = s.regex(new RegExp(rule.pattern, "u"));
	}
	return s;
}

const props = PROJECT_SCHEMA.properties;
const goalProps = props.goal.properties;

const DATE_SHAPE = new RegExp(goalProps.due.pattern, "u");

function realDate(value: string): boolean {
	const parsed = new Date(`${value}T00:00:00Z`);
	return (
		!Number.isNaN(parsed.getTime()) &&
		parsed.toISOString().slice(0, 10) === value
	);
}

export const goalKeys = [
	"specific",
	"measurable",
	"achievable",
	"relevant",
	"time_bound",
] as const;

export type GoalKey = (typeof goalKeys)[number];

export const RULES = {
	name: props.name,
	description: props.description,
	goal: {
		specific: goalProps.specific,
		measurable: goalProps.measurable,
		achievable: goalProps.achievable,
		relevant: goalProps.relevant,
		time_bound: goalProps.time_bound,
	},
} as const;

export const contextSchema = z.object({
	name: text(props.name),
	description: text(props.description),
	goal: z.object({
		specific: text(goalProps.specific),
		measurable: text(goalProps.measurable),
		achievable: text(goalProps.achievable),
		relevant: text(goalProps.relevant),
		time_bound: text(goalProps.time_bound),
		due: z.string().regex(DATE_SHAPE).refine(realDate, { message: "date" }),
	}),
});

// A field error is a key of the catalogue and its parameters; the page translates it.
export interface FieldError {
	key:
		| "ui.projects.errors.too_short"
		| "ui.projects.errors.too_long"
		| "ui.projects.errors.name_pattern"
		| "ui.projects.errors.date"
		| "ui.projects.errors.date_past";
	params: Record<string, number>;
}

function errorOf(issue: z.core.$ZodIssue, path: string): FieldError {
	switch (issue.code) {
		case "too_small":
			return {
				key: "ui.projects.errors.too_short",
				params: { min: Number(issue.minimum) },
			};
		case "too_big":
			return {
				key: "ui.projects.errors.too_long",
				params: { max: Number(issue.maximum) },
			};
		case "invalid_format":
			return {
				key:
					path === "name"
						? "ui.projects.errors.name_pattern"
						: "ui.projects.errors.date",
				params: {},
			};
		default:
			return { key: "ui.projects.errors.date", params: {} };
	}
}

// today is the day the form was opened, YYYY-MM-DD; a due date before it is refused when the date is new.
export function validate(
	values: ProjectContextInput,
	options: { today: string; checkDue: boolean },
): Record<string, FieldError> {
	const out: Record<string, FieldError> = {};
	const parsed = contextSchema.safeParse(values);
	if (!parsed.success) {
		for (const issue of parsed.error.issues) {
			const path = issue.path.join(".");
			// A short value breaks the length and the pattern: the first one says it.
			if (!(path in out)) {
				out[path] = errorOf(issue, path);
			}
		}
	}
	const due = values.goal.due;
	if (options.checkDue && !("goal.due" in out) && due < options.today) {
		out["goal.due"] = { key: "ui.projects.errors.date_past", params: {} };
	}
	return out;
}

export function emptyContext(): ProjectContextInput {
	return {
		name: "",
		description: "",
		goal: {
			specific: "",
			measurable: "",
			achievable: "",
			relevant: "",
			time_bound: "",
			due: "",
		},
	};
}
