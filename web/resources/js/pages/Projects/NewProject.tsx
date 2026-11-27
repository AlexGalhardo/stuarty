import { type ReactNode, useState } from "react";

import { FieldLabel } from "@/components/FieldHelp";
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
import { useT } from "@/lib/i18n";
import {
	CHOICE_ICON,
	CHOICES,
	type Choice,
	SIZE_PRESETS,
	TEXT_TYPES,
	type TextType,
} from "./kinds";

export interface NewProjectPick {
	choice: Choice;
	textType: TextType;
	format: string;
	name: string;
}

const RADIO_CARD =
	"relative flex cursor-pointer flex-col items-center gap-1 rounded-lg text-center border border-border bg-surface p-4 text-text hover:bg-surface-hover has-[:checked]:border-accent has-[:checked]:bg-accent-soft has-[:focus-visible]:border-focus has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-focus";

// The first decision of a project, in four equal cards: a text, a video, an image, or later. A format is asked
// only for a video and a type only for a text. "Continue to the briefing" opens the briefing in the same dialog,
// and the project is created at its end, with the first item the choice calls for (createPlan). The service says
// every rule; a refused field comes back under it.
export default function NewProject({
	open,
	onOpenChange,
	onContinue,
}: {
	open: boolean;
	onOpenChange(open: boolean): void;
	onContinue(pick: NewProjectPick): void;
}): ReactNode {
	const t = useT();
	const [choice, setChoice] = useState<Choice | null>(null);
	const [textType, setTextType] = useState<TextType>("post");
	const [format, setFormat] = useState<string>(SIZE_PRESETS[0]);
	const [name, setName] = useState("");

	const change = (next: boolean): void => {
		onOpenChange(next);
		if (!next) {
			setChoice(null);
			setName("");
		}
	};

	return (
		<Dialog open={open} onOpenChange={change}>
			<DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
				<DialogHeader>
					<DialogTitle>{t("ui.projects.new_project.title")}</DialogTitle>
					<DialogDescription>
						{t("ui.projects.new_project.description")}
					</DialogDescription>
				</DialogHeader>
				<form
					className="flex flex-col gap-5"
					onSubmit={(event) => {
						event.preventDefault();
						if (choice !== null) {
							onContinue({ choice, textType, format, name: name.trim() });
							onOpenChange(false);
						}
					}}
				>
					<div className="flex flex-col gap-1">
						<FieldLabel
							htmlFor="new-project-name"
							label={t("ui.projects.new_project.name")}
							help={t("ui.help.g2.new_project_name_help")}
						/>
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
			</DialogContent>
		</Dialog>
	);
}
