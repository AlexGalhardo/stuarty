import { FolderPlus } from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { FieldLabel } from "@/components/FieldHelp";
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
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import { useRuntime } from "../state/EditorContext";
import type { EditorRuntime } from "../state/runtime";

function defaultDueDate(): string {
	const d = new Date();
	d.setDate(d.getDate() + 30);
	const m = String(d.getMonth() + 1).padStart(2, "0");
	const day = String(d.getDate()).padStart(2, "0");
	return `${d.getFullYear()}-${m}-${day}`;
}

export default function PromoteProjectDialog(
	props: PromoteDialogProps,
): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	return (
		<PromoteProjectForm
			{...props}
			promote={async (input) => {
				const result = await runtime.promoteStandalone(input);
				runtime.noticeWriter.push({
					kind: "info",
					message: t("ui.editor.promote.success", { name: result.name }),
				});
				return result;
			}}
		/>
	);
}

interface PromoteDialogProps {
	open: boolean;
	initialName: string;
	onOpenChange(open: boolean): void;
	onPromoted(projectId: string, name: string): void;
}

export function PromoteProjectForm({
	open,
	initialName,
	onOpenChange,
	onPromoted,
	promote,
}: PromoteDialogProps & {
	promote: EditorRuntime["promoteStandalone"];
}): ReactNode {
	const t = useT();
	const [name, setName] = useState(initialName.slice(0, 32));
	const [description, setDescription] = useState("");
	const [specific, setSpecific] = useState("");
	const [measurable, setMeasurable] = useState("");
	const [achievable, setAchievable] = useState("");
	const [relevant, setRelevant] = useState("");
	const [timeBound, setTimeBound] = useState("");
	const [due, setDue] = useState(defaultDueDate);
	const [busy, setBusy] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);

	async function handleSubmit(
		event: FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		try {
			const res = await promote({
				name: name.trim(),
				description: description.trim(),
				goal: {
					specific: specific.trim(),
					measurable: measurable.trim(),
					achievable: achievable.trim(),
					relevant: relevant.trim(),
					time_bound: timeBound.trim(),
					due: due.trim(),
				},
				kind: "video",
			});
			onPromoted(res.project_id, res.name);
			onOpenChange(false);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				className="max-h-[85vh] overflow-y-auto sm:max-w-xl"
				data-testid="promote-project-dialog"
			>
				<DialogHeader>
					<DialogTitle className="flex items-center gap-2">
						<FolderPlus aria-hidden="true" className="size-4" />
						{t("ui.editor.promote.title")}
					</DialogTitle>
					<DialogDescription>{t("ui.editor.promote.intro")}</DialogDescription>
				</DialogHeader>
				<form
					onSubmit={(event) => void handleSubmit(event)}
					className="flex flex-col gap-3"
				>
					<div className="flex flex-col gap-1">
						<FieldLabel
							htmlFor="promote-name"
							label={t("ui.projects.context.name")}
							help={t("ui.projects.help.name")}
						/>
						<Input
							id="promote-name"
							value={name}
							onChange={(e) => setName(e.target.value)}
							placeholder={t("ui.projects.example.name")}
							maxLength={32}
							required
						/>
					</div>
					<div className="flex flex-col gap-1">
						<FieldLabel
							htmlFor="promote-description"
							label={t("ui.projects.context.description")}
							help={t("ui.projects.help.description")}
						/>
						<Textarea
							id="promote-description"
							rows={3}
							value={description}
							onChange={(e) => setDescription(e.target.value)}
							placeholder={t("ui.projects.example.description")}
							required
						/>
					</div>
					<div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
						<div className="flex flex-col gap-1">
							<FieldLabel
								htmlFor="promote-specific"
								label={t("ui.projects.context.specific")}
								help={t("ui.projects.help.specific")}
							/>
							<Input
								id="promote-specific"
								value={specific}
								onChange={(e) => setSpecific(e.target.value)}
								placeholder={t("ui.projects.example.specific")}
								required
							/>
						</div>
						<div className="flex flex-col gap-1">
							<FieldLabel
								htmlFor="promote-measurable"
								label={t("ui.projects.context.measurable")}
								help={t("ui.projects.help.measurable")}
							/>
							<Input
								id="promote-measurable"
								value={measurable}
								onChange={(e) => setMeasurable(e.target.value)}
								placeholder={t("ui.projects.example.measurable")}
								required
							/>
						</div>
						<div className="flex flex-col gap-1">
							<FieldLabel
								htmlFor="promote-achievable"
								label={t("ui.projects.context.achievable")}
								help={t("ui.projects.help.achievable")}
							/>
							<Input
								id="promote-achievable"
								value={achievable}
								onChange={(e) => setAchievable(e.target.value)}
								placeholder={t("ui.projects.example.achievable")}
								required
							/>
						</div>
						<div className="flex flex-col gap-1">
							<FieldLabel
								htmlFor="promote-relevant"
								label={t("ui.projects.context.relevant")}
								help={t("ui.projects.help.relevant")}
							/>
							<Input
								id="promote-relevant"
								value={relevant}
								onChange={(e) => setRelevant(e.target.value)}
								placeholder={t("ui.projects.example.relevant")}
								required
							/>
						</div>
						<div className="flex flex-col gap-1">
							<FieldLabel
								htmlFor="promote-time-bound"
								label={t("ui.projects.context.time_bound")}
								help={t("ui.projects.help.time_bound")}
							/>
							<Input
								id="promote-time-bound"
								value={timeBound}
								onChange={(e) => setTimeBound(e.target.value)}
								placeholder={t("ui.projects.example.time_bound")}
								required
							/>
						</div>
						<div className="flex flex-col gap-1">
							<FieldLabel
								htmlFor="promote-due"
								label={t("ui.projects.context.due")}
								help={t("ui.projects.help.due")}
							/>
							<Input
								id="promote-due"
								type="date"
								value={due}
								onChange={(e) => setDue(e.target.value)}
								placeholder={t("ui.projects.example.due")}
								required
							/>
						</div>
					</div>
					<FailureNote failure={failure} />
					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							onClick={() => onOpenChange(false)}
							disabled={busy}
						>
							{t("ui.projects.new_project.cancel")}
						</Button>
						<Button
							type="submit"
							disabled={busy}
							data-testid="promote-project-submit"
						>
							{busy
								? t("ui.editor.promote.saving")
								: t("ui.editor.promote.submit")}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
}
