import { CircleHelp } from "lucide-react";
import { type FormEvent, useId, useState } from "react";

import { type Failure, FailureNote } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "@/components/ui/tooltip";
import { useT } from "@/lib/i18n";
import type { ProjectContextInput } from "../../editor/api/types";
import {
	charCount,
	type FieldError,
	type GoalKey,
	goalKeys,
	RULES,
	validate,
} from "./projectSchema";

type FieldId = "name" | "description" | GoalKey | "due";

const FIELDS: readonly FieldId[] = ["name", "description", ...goalKeys, "due"];

function pathOf(field: FieldId): string {
	return field === "name" || field === "description" ? field : `goal.${field}`;
}

function fieldValue(values: ProjectContextInput, field: FieldId): string {
	if (field === "name" || field === "description") {
		return values[field];
	}
	return values.goal[field === "due" ? "due" : field];
}

function withValue(
	values: ProjectContextInput,
	field: FieldId,
	value: string,
): ProjectContextInput {
	if (field === "name" || field === "description") {
		return { ...values, [field]: value };
	}
	return { ...values, goal: { ...values.goal, [field]: value } };
}

// today is the local day, YYYY-MM-DD, the day the form opened.
function today(): string {
	const now = new Date();
	const month = String(now.getMonth() + 1).padStart(2, "0");
	const day = String(now.getDate()).padStart(2, "0");
	return `${now.getFullYear()}-${month}-${day}`;
}

interface Props {
	initial: ProjectContextInput;
	submitLabel: string;
	busy: boolean;
	// An edit of a project that has a due date accepts it again when it did not change.
	keepDue?: string;
	// What the core answered to the last submit: under the field it names, or above the buttons.
	failure?: Failure | null;
	onEdited?: () => void;
	onSubmit: (values: ProjectContextInput) => void;
	onCancel?: () => void;
}

// The form of the context of a project: name, description and the five parts of the SMART goal with its due date.
// Every field has a tooltip that says what to write and why, and a placeholder with a worked example. The button is
// enabled only when every field is valid, and each error shows under its field. The rules come from the schema
// that the core exports (projectSchema.ts); the core checks them again (SDD-012 section 2.1, epic 56).
export default function ProjectContextForm({
	initial,
	submitLabel,
	busy,
	keepDue,
	failure = null,
	onEdited,
	onSubmit,
	onCancel,
}: Props) {
	const t = useT();
	const prefix = useId();
	const [values, setValues] = useState<ProjectContextInput>(initial);
	// A field that opens with a value shows its error at once: the name of a project made before epic 56 may break
	// the rule of today, and the button would wait with no word of why.
	const [touched, setTouched] = useState<ReadonlySet<FieldId>>(
		() => new Set(FIELDS.filter((field) => fieldValue(initial, field) !== "")),
	);
	const [day] = useState(today);

	const errors: Record<string, FieldError> = validate(values, {
		today: day,
		checkDue: values.goal.due !== (keepDue ?? ""),
	});
	const valid = Object.keys(errors).length === 0;
	const aboutAField = FIELDS.some((field) => pathOf(field) === failure?.path);

	const touch = (field: FieldId): void =>
		setTouched((was) => new Set(was).add(field));

	const submit = (event: FormEvent<HTMLFormElement>): void => {
		event.preventDefault();
		if (valid) {
			onSubmit(values);
		}
	};

	const rule = (field: FieldId): { min?: number; max?: number } => {
		if (field === "name") {
			return { min: RULES.name.minLength, max: RULES.name.maxLength };
		}
		if (field === "description") {
			return {
				min: RULES.description.minLength,
				max: RULES.description.maxLength,
			};
		}
		if (field === "due") {
			return {};
		}
		return {
			min: RULES.goal[field].minLength,
			max: RULES.goal[field].maxLength,
		};
	};

	return (
		<TooltipProvider>
			<form
				onSubmit={submit}
				noValidate
				className="flex flex-col gap-5 rounded-xl border bg-card p-5 text-card-foreground"
			>
				{FIELDS.map((field) => {
					const id = `${prefix}-${field}`;
					const error = touched.has(field) ? errors[pathOf(field)] : undefined;
					const refused =
						!error && failure?.path === pathOf(field) ? failure : null;
					const label = t(`ui.projects.context.${field}`);
					const common = {
						id,
						name: field,
						value: fieldValue(values, field),
						autoComplete: "off",
						placeholder: t(`ui.projects.example.${field}`),
						"aria-invalid": error || refused ? true : undefined,
						"aria-describedby": error || refused ? `${id}-error` : undefined,
						onBlur: () => touch(field),
						onChange: (
							e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
						) => {
							touch(field);
							onEdited?.();
							setValues((was) => withValue(was, field, e.target.value));
						},
					};
					const { min, max } = rule(field);

					return (
						<div key={field} className="flex flex-col gap-2">
							<div className="flex items-center gap-1">
								<Label id={`${id}-label`} htmlFor={id}>
									{label}
								</Label>
								<Tooltip>
									<TooltipTrigger
										type="button"
										aria-label={t("ui.projects.help_for", { field: label })}
										className="rounded-full text-muted-foreground hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50"
									>
										<CircleHelp className="size-4" aria-hidden="true" />
									</TooltipTrigger>
									<TooltipContent className="max-w-xs">
										{t(`ui.projects.help.${field}`)}
									</TooltipContent>
								</Tooltip>
							</div>
							{field === "description" ? (
								<Textarea rows={4} {...common} />
							) : (
								<Input type={field === "due" ? "date" : "text"} {...common} />
							)}
							{min !== undefined && max !== undefined && (
								<p className="text-xs text-muted-foreground">
									{t("ui.projects.counter", {
										count: charCount(fieldValue(values, field)),
										min,
										max,
									})}
								</p>
							)}
							{error && (
								<p
									id={`${id}-error`}
									role="alert"
									className="text-sm text-destructive"
								>
									{t(error.key, error.params)}
								</p>
							)}
							<FailureNote failure={refused} id={`${id}-error`} />
						</div>
					);
				})}
				{!aboutAField && <FailureNote failure={failure} />}
				<div className="flex gap-2">
					<Button type="submit" disabled={!valid || busy}>
						{submitLabel}
					</Button>
					{onCancel && (
						<Button type="button" variant="outline" onClick={onCancel}>
							{t("ui.projects.card.cancel")}
						</Button>
					)}
				</div>
			</form>
		</TooltipProvider>
	);
}
