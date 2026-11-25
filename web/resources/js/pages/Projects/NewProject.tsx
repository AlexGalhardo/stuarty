import { router } from "@inertiajs/react";
import { type ReactNode, useState } from "react";

import { type Failure, failureOf } from "@/components/Failure";
import { Segment } from "@/components/Segment";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { ProjectContextInput } from "../../editor/api/types";
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
import { emptyContext } from "./projectSchema";

const RADIO_CARD =
	"relative flex cursor-pointer flex-col items-center gap-1 rounded-lg text-center border border-border bg-surface p-4 text-text hover:bg-surface-hover has-[:checked]:border-accent has-[:checked]:bg-accent-soft has-[:focus-visible]:border-focus has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-focus";

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
	const [name, setName] = useState("");
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);
	// The project exists but its first item was refused: the person goes on from its page.
	const [created, setCreated] = useState<string | null>(null);

	const change = (next: boolean): void => {
		onOpenChange(next);
		// A project made without its first item is in the workspace: the list behind the dialog reads it again.
		if (!next && created !== null) router.reload({ only: ["projects"] });
		if (!next) {
			setStep("kind");
			setChoice(null);
			setName("");
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
						<div className="flex flex-col gap-1">
							<Label htmlFor="new-project-name">
								{t("ui.projects.new_project.name")}
							</Label>
							<Input
								id="new-project-name"
								value={name}
								onChange={(event) => setName(event.target.value)}
								placeholder={t("ui.projects.new_project.name_placeholder")}
							/>
						</div>
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
											<Icon aria-hidden="true" className="size-6 text-icon" />
											<span className="font-medium">
												{t(`ui.projects.new_project.choice.${c}.title`)}
											</span>
											<span className="text-sm text-text-muted">
												{t(`ui.projects.new_project.choice.${c}.hint`)}
											</span>
											{choice === c ? (
												<span className="mt-1 text-xs font-medium text-accent-text">
													{t("ui.projects.new_project.chosen")}
												</span>
											) : null}
										</label>
									);
								})}
							</div>
						</fieldset>
						{choice === "video" ? (
							<fieldset className="flex flex-col gap-2">
								<legend className="mb-1 text-sm font-medium text-text">
									{t("ui.projects.new_project.format")}
								</legend>
								<div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
									{SIZE_PRESETS.map((p) => (
										<label key={p} className={RADIO_CARD}>
											<input
												type="radio"
												name="new-project-format"
												value={p}
												checked={format === p}
												onChange={() => setFormat(p)}
												className="sr-only"
											/>
											<span className="font-medium">{p}</span>
											<span className="text-xs text-text-muted">
												{t(`ui.projects.presets.${p}`)}
											</span>
										</label>
									))}
								</div>
								<p className="text-sm text-text-muted">
									{t("ui.projects.new_project.format_hint")}
								</p>
							</fieldset>
						) : null}
						{choice === "text" ? (
							<div className="flex flex-col gap-2">
								<span className="text-sm font-medium text-text">
									{t("ui.projects.new_project.text_legend")}
								</span>
								<Segment
									label={t("ui.projects.new_project.text_legend")}
									value={textType}
									onChange={(next) => setTextType(next as TextType)}
									options={TEXT_TYPES.map((type) => ({
										value: type,
										label: t(`ui.write.kind.${type}`),
									}))}
								/>
								<p className="text-sm text-text-muted">
									{t("ui.projects.new_project.text_note")}
								</p>
							</div>
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
						initial={{ ...emptyContext(), name: name.trim() }}
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
