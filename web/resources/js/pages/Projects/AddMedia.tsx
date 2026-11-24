import { type FormEvent, type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
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
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import { SELECT_CLASS } from "../Settings/SettingsSection";
import { SIZE_PRESETS } from "./kinds";

const TITLE_MAX = 120;

// "Add media" of a project that began as something else, a text for example: it adds the video, with its format,
// as the first timeline of the project. The service then adds the steps of a video to the workflow and keeps the
// finished ones as they are; the page only asks it to read again (onAdded).
export default function AddMedia({
	projectId,
	projectName,
	onAdded,
}: {
	projectId: string;
	projectName: string;
	onAdded(): void;
}): ReactNode {
	const t = useT();
	const [open, setOpen] = useState(false);
	const [title, setTitle] = useState("");
	const [format, setFormat] = useState<string>(SIZE_PRESETS[0]);
	const [busy, setBusy] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);

	const submit = async (event: FormEvent<HTMLFormElement>): Promise<void> => {
		event.preventDefault();
		setBusy(true);
		setFailure(null);
		try {
			await createClient(new HttpTransport("/api/v1")).addProjectItem(
				projectId,
				{
					kind: "timeline",
					title:
						title.trim() === ""
							? projectName.slice(0, TITLE_MAX)
							: title.trim(),
					preset: format,
				},
			);
			setOpen(false);
			setTitle("");
			onAdded();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	};

	return (
		<>
			<Button type="button" variant="outline" onClick={() => setOpen(true)}>
				{t("ui.projects.add_media.open")}
			</Button>
			<Dialog
				open={open}
				onOpenChange={(next) => {
					setOpen(next);
					setFailure(null);
				}}
			>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>{t("ui.projects.add_media.title")}</DialogTitle>
						<DialogDescription>
							{t("ui.projects.add_media.description")}
						</DialogDescription>
					</DialogHeader>
					<form
						className="flex flex-col gap-3"
						onSubmit={(event) => void submit(event)}
					>
						<div className="flex flex-col gap-1">
							<Label htmlFor="add-media-title">
								{t("ui.projects.add_media.name")}
							</Label>
							<Input
								id="add-media-title"
								value={title}
								maxLength={TITLE_MAX}
								placeholder={projectName}
								autoComplete="off"
								onChange={(event) => setTitle(event.target.value)}
							/>
						</div>
						<div className="flex flex-col gap-1">
							<Label htmlFor="add-media-format">
								{t("ui.projects.new_project.format")}
							</Label>
							<select
								id="add-media-format"
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
						</div>
						<FailureNote failure={failure} />
						<DialogFooter>
							<Button
								type="button"
								variant="outline"
								onClick={() => setOpen(false)}
							>
								{t("ui.projects.card.cancel")}
							</Button>
							<Button type="submit" disabled={busy}>
								{t("ui.projects.add_media.submit")}
							</Button>
						</DialogFooter>
					</form>
				</DialogContent>
			</Dialog>
		</>
	);
}
