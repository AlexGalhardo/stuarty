import { MoreVertical } from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { FieldLabel } from "@/components/FieldHelp";
import UntrustedText from "@/components/UntrustedText";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { useT } from "@/lib/i18n";
import type { ProjectSummary } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";

const NAME_MAX = 120;

// The menu of a project card, the same for a text and for a media project: Rename, Duplicate and Delete, with the
// dialogs of the first and the last. Rename is `ProjectUpdate`; Delete moves the project to the trash after a
// confirmation and can be undone from the trash. A refusal of the duplicate goes up to the card, which shows it.
export default function ProjectMenu({
	project,
	onTrash,
	onChanged,
	onFailure,
}: {
	project: ProjectSummary;
	onTrash(): void;
	onChanged(): void;
	onFailure(failure: Failure | null): void;
}): ReactNode {
	const t = useT();
	const [renaming, setRenaming] = useState(false);
	const [confirming, setConfirming] = useState(false);
	const [name, setName] = useState(project.name);
	const [busy, setBusy] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);

	// The copy is a new project with the brief, the texts and the timelines of this one (VV-225).
	const duplicate = async (): Promise<void> => {
		onFailure(null);
		try {
			await createClient(new HttpTransport("/api/v1")).duplicateProject(
				project.id,
			);
			onChanged();
		} catch (error) {
			onFailure(failureOf(error));
		}
	};

	const submitRename = async (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		const next = name.trim();
		if (next === "" || next === project.name) {
			setRenaming(false);
			return;
		}
		setBusy(true);
		setFailure(null);
		try {
			// The name of a project is its own since epic 56: ProjectUpdate changes it, with the rule of the name.
			await createClient(new HttpTransport("/api/v1")).updateProject(
				project.id,
				{ name: next },
			);
			setRenaming(false);
			onChanged();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	};

	return (
		<>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button type="button" variant="ghost" size="icon-sm">
						<MoreVertical aria-hidden="true" />
						<span className="sr-only">
							{t("ui.projects.card.menu")}{" "}
							<UntrustedText>{project.name}</UntrustedText>
						</span>
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end">
					<DropdownMenuItem
						onSelect={() => {
							setName(project.name);
							setFailure(null);
							setRenaming(true);
						}}
					>
						{t("ui.projects.card.rename")}
					</DropdownMenuItem>
					<DropdownMenuItem onSelect={() => void duplicate()}>
						{t("ui.projects.card.duplicate")}
					</DropdownMenuItem>
					<DropdownMenuItem
						variant="destructive"
						onSelect={() => setConfirming(true)}
					>
						{t("ui.projects.card.delete")}
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>

			<Dialog open={renaming} onOpenChange={setRenaming}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>{t("ui.projects.card.rename_title")}</DialogTitle>
					</DialogHeader>
					<form
						className="flex flex-col gap-3"
						onSubmit={(event) => void submitRename(event)}
					>
						<div className="flex flex-col gap-1">
							<FieldLabel
								htmlFor={`rename-${project.id}`}
								label={t("ui.projects.name")}
								help={t("ui.help.g2.project_rename_help")}
							/>
							<Input
								id={`rename-${project.id}`}
								placeholder={t("ui.help.g2.project_rename_ph")}
								value={name}
								maxLength={NAME_MAX}
								required
								autoComplete="off"
								onChange={(event) => setName(event.target.value)}
							/>
						</div>
						<FailureNote failure={failure} />
						<DialogFooter>
							<Button
								type="button"
								variant="outline"
								onClick={() => setRenaming(false)}
							>
								{t("ui.projects.card.cancel")}
							</Button>
							<Button type="submit" disabled={busy}>
								{t("ui.projects.card.save")}
							</Button>
						</DialogFooter>
					</form>
				</DialogContent>
			</Dialog>

			<AlertDialog open={confirming} onOpenChange={setConfirming}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							{t("ui.projects.card.delete_title")}
						</AlertDialogTitle>
						<AlertDialogDescription>
							{t("ui.projects.card.delete_body")}
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>
							{t("ui.projects.card.cancel")}
						</AlertDialogCancel>
						<AlertDialogAction variant="destructive" onClick={onTrash}>
							{t("ui.projects.trash.move")}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</>
	);
}
