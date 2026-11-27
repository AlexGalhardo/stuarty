import { Link } from "@inertiajs/react";
import { FolderPlus } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
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
import { useT } from "@/lib/i18n";
import { useImage, useImageApi } from "../state/context";

type ProjectOption = { id: string; name: string };

// Optional standalone-to-project bridge in /edit-image/{id} (epic 105):
// exports the current image as PNG and attaches it into a chosen project's media library.
export function AttachProjectTool(): ReactNode {
	const t = useT();
	const doc = useImage((s) => s.doc);
	const [open, setOpen] = useState(false);

	return (
		<>
			<Button
				type="button"
				variant="outline"
				size="sm"
				className="h-8 px-3 font-semibold"
				disabled={doc === null}
				onClick={() => setOpen(true)}
				data-testid="image-attach-project"
			>
				<FolderPlus aria-hidden="true" className="size-4" />
				{t("ui.image.attach.open")}
			</Button>
			{open && <AttachProjectDialog onClose={() => setOpen(false)} />}
		</>
	);
}

function AttachProjectDialog({ onClose }: { onClose(): void }): ReactNode {
	const t = useT();
	const api = useImageApi();
	const doc = useImage((s) => s.doc);
	const [projects, setProjects] = useState<ProjectOption[] | null>(null);
	const [selected, setSelected] = useState("");
	const [busy, setBusy] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [attachedTo, setAttachedTo] = useState<ProjectOption | null>(null);

	useEffect(() => {
		let live = true;
		api
			.listProjects()
			.then((res) => {
				if (!live) return;
				setProjects(res.projects);
				if (res.projects.length > 0) setSelected(res.projects[0].id);
			})
			.catch((error: unknown) => {
				if (live) setFailure(failureOf(error));
			});
		return () => {
			live = false;
		};
	}, [api]);

	if (doc === null) return null;
	const imageId = doc.image_id;

	async function attach(): Promise<void> {
		if (!selected) return;
		setBusy(true);
		setFailure(null);
		setAttachedTo(null);
		try {
			const exported = await api.exportImage(imageId, { format: "png" });
			await api.attachExportToProject(selected, exported.path);
			const target = projects?.find((p) => p.id === selected) ?? {
				id: selected,
				name: selected,
			};
			setAttachedTo(target);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusy(false);
		}
	}

	return (
		<Dialog open onOpenChange={(next) => !next && onClose()}>
			<DialogContent data-testid="image-attach-dialog">
				<DialogHeader>
					<DialogTitle>{t("ui.image.attach.title")}</DialogTitle>
					<DialogDescription>{t("ui.image.attach.intro")}</DialogDescription>
				</DialogHeader>

				{projects === null && failure === null && (
					<p className="text-sm text-muted-foreground">
						{t("ui.image.attach.loading")}
					</p>
				)}

				{projects !== null && projects.length === 0 && (
					<p className="text-sm text-muted-foreground">
						{t("ui.image.attach.empty")}
					</p>
				)}

				{projects !== null && projects.length > 0 && (
					<div className="flex flex-col gap-1.5">
						<Label htmlFor="image-attach-select">
							{t("ui.image.attach.project_label")}
						</Label>
						<select
							id="image-attach-select"
							data-testid="image-attach-select"
							value={selected}
							onChange={(event) => setSelected(event.target.value)}
							className="h-9 rounded-lg border border-input bg-background px-2 text-sm"
						>
							{projects.map((p) => (
								<option key={p.id} value={p.id}>
									{p.name}
								</option>
							))}
						</select>
					</div>
				)}

				<FailureNote failure={failure} />

				{attachedTo !== null && (
					<div
						data-testid="image-attach-done"
						className="flex items-center justify-between gap-2 rounded-md border border-border bg-surface p-3 text-sm"
					>
						<span>
							{t("ui.image.attach.done")}{" "}
							<strong>
								<UntrustedText>{attachedTo.name}</UntrustedText>
							</strong>
						</span>
						<Button asChild variant="outline" size="sm">
							<Link href={`/projects/${encodeURIComponent(attachedTo.id)}`}>
								{t("ui.image.attach.open_project")}
							</Link>
						</Button>
					</div>
				)}

				<DialogFooter>
					<Button type="button" variant="outline" onClick={onClose}>
						{t("ui.image.export.close")}
					</Button>
					<Button
						type="button"
						disabled={busy || !selected}
						data-testid="image-attach-submit"
						onClick={() => void attach()}
					>
						{t(busy ? "ui.image.attach.working" : "ui.image.attach.submit")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
