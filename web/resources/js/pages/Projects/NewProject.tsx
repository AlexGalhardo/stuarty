import { type ReactNode, useState } from "react";

import { type Failure, failureOf } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { ProjectContextInput } from "../../editor/api/types";
import { SELECT_CLASS } from "../Settings/SettingsSection";
import {
	CHOICE_ICON,
	CHOICES,
	type Choice,
	createPlan,
	SIZE_PRESETS,
	TEXT_TYPES,
	type TextType,
} from "./kinds";
import NewProjectBriefing from "./NewProjectBriefing";

const RADIO_CARD =
	"flex cursor-pointer flex-col gap-1 rounded-lg border border-border bg-surface p-4 text-text hover:bg-surface-hover has-[:checked]:border-accent has-[:checked]:bg-accent-soft has-[:focus-visible]:border-focus has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-focus";

// The first decision of a project, in four equal cards: a text, a video, an image, or later. A format is asked
// only for a video and a type only for a text. "Continue to the briefing" opens the briefing in the same dialog,
// and the project is created at its end, with the first item the choice calls for (createPlan). The service says
// every rule; a refused field comes back under it.
export default function NewProject({
	open,
	onOpenChange,
}: {
	open: boolean;
	onOpenChange(open: boolean): void;
}): ReactNode {
	const t = useT();
	const [step, setStep] = useState<"kind" | "briefing">("kind");
	const [choice, setChoice] = useState<Choice | null>(null);
	const [textType, setTextType] = useState<TextType>("post");
	const [format, setFormat] = useState<string>(SIZE_PRESETS[0]);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);
	// The project exists but its first item was refused: the person goes on from its page.
	const [created, setCreated] = useState<string | null>(null);

	const change = (next: boolean): void => {
		onOpenChange(next);
		if (!next) {
			setStep("kind");
			setChoice(null);
			setFailure(null);
			setCreated(null);
		}
	};

	const submit = async (values: ProjectContextInput): Promise<void> => {
		if (choice === null) return;
		setFailure(null);
		setBusy(true);
		const plan = createPlan({ choice, textType, format }, values.name);
		const api = createClient(new HttpTransport("/api/v1"));
		let id: string;
		try {
			const result = await api.createProject(
				plan.kind === undefined ? values : { ...values, kind: plan.kind },
			);
			id = result.project_id;
		} catch (error) {
			setFailure(failureOf(error));
			setBusy(false);
			return;
		}
		if (plan.item !== undefined) {
			try {
				await api.addProjectItem(id, plan.item);
			} catch (error) {
				setFailure(failureOf(error));
				setCreated(id);
				setBusy(false);
				return;
			}
		}
		location.assign(`/projects/${id}`);
	};

	return (
		<Dialog open={open} onOpenChange={change}>
			<DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
				<DialogHeader>
					<DialogTitle>{t("ui.projects.new_project.title")}</DialogTitle>
					<DialogDescription>
						{step === "kind"
							? t("ui.projects.new_project.description")
							: t("ui.projects.new_project.step_two")}
					</DialogDescription>
				</DialogHeader>
				{step === "kind" ? (
					<form
						className="flex flex-col gap-5"
						onSubmit={(event) => {
							event.preventDefault();
							if (choice !== null) setStep("briefing");
						}}
					>
						<fieldset className="flex flex-col gap-2">
							<legend className="mb-2 text-sm font-medium text-text">
								{t("ui.projects.new_project.kind_legend")}
							</legend>
							<div className="grid grid-cols-1 gap-3 min-[420px]:grid-cols-2 sm:grid-cols-4">
								{CHOICES.map((c) => {
									const Icon = CHOICE_ICON[c];
									return (
										<label key={c} className={RADIO_CARD} data-choice={c}>
											<input
												type="radio"
												name="new-project-kind"
												value={c}
												checked={choice === c}
												onChange={() => setChoice(c)}
												className="sr-only"
											/>
											<Icon aria-hidden="true" className="size-5 text-icon" />
											<span className="font-medium">
												{t(`ui.projects.new_project.choice.${c}.title`)}
											</span>
											<span className="text-sm text-text-muted">
												{t(`ui.projects.new_project.choice.${c}.hint`)}
											</span>
										</label>
									);
								})}
							</div>
						</fieldset>
						{choice === "video" ? (
							<div className="flex flex-col gap-1">
								<Label htmlFor="new-project-format">
									{t("ui.projects.new_project.format")}
								</Label>
								<select
									id="new-project-format"
									value={format}
									onChange={(event) => setFormat(event.target.value)}
									className={cn(SELECT_CLASS, "w-fit")}
								>
									{SIZE_PRESETS.map((p) => (
										<option key={p} value={p}>
											{p}
										</option>
									))}
								</select>
								<p className="text-sm text-text-muted">
									{t("ui.projects.new_project.format_hint")}
								</p>
							</div>
						) : null}
						{choice === "text" ? (
							<fieldset className="flex flex-col gap-2">
								<legend className="mb-1 text-sm font-medium text-text">
									{t("ui.projects.new_project.text_legend")}
								</legend>
								<div className="flex flex-wrap gap-x-5 gap-y-2">
									{TEXT_TYPES.map((type) => (
										<label
											key={type}
											className="flex items-center gap-2 text-sm"
										>
											<input
												type="radio"
												name="new-project-text-type"
												value={type}
												checked={textType === type}
												onChange={() => setTextType(type)}
												className="accent-accent"
											/>
											{t(`ui.write.kind.${type}`)}
										</label>
									))}
								</div>
							</fieldset>
						) : null}
						<DialogFooter>
							<Button
								type="button"
								variant="outline"
								onClick={() => change(false)}
							>
								{t("ui.projects.new_project.cancel")}
							</Button>
							<Button type="submit" disabled={choice === null}>
								{t("ui.projects.new_project.continue")}
							</Button>
						</DialogFooter>
					</form>
				) : (
					<NewProjectBriefing
						kind={choice ?? "undecided"}
						busy={busy}
						failure={failure}
						created={created}
						onEdited={() => setFailure(null)}
						onSubmit={(values) => void submit(values)}
						onBack={() => setStep("kind")}
					/>
				)}
			</DialogContent>
		</Dialog>
	);
}
