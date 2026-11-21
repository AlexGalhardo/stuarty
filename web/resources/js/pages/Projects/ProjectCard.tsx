import { Link } from "@inertiajs/react";
import { Film, MoreVertical } from "lucide-react";
import {
	type FormEvent,
	type ReactNode,
	useEffect,
	useRef,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
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
import { Label } from "@/components/ui/label";
import { formatDateTime } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { createSlotQueue, type Release, whileNear } from "@/lib/slotQueue";
import type { ProjectSummary } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import { frameUrl } from "../../editor/api/urls";
import { BundleExport } from "./Bundles";

const NAME_MAX = 120;
const THUMBNAIL_WIDTH = 270;

// The frame endpoint refuses a credential with more than two frames in flight
// (429, SDD-007 section 4.4), so every card draws its picture through this
// queue, and only while it is near the viewport.
const thumbnails = createSlotQueue(2);

// The picture of a card: the frame at the start, as the player draws it. A
// project with nothing on its timeline has no picture, and a frame that cannot
// be read leaves the placeholder.
function Thumbnail({ project }: { project: ProjectSummary }): ReactNode {
	const [failed, setFailed] = useState(false);
	const [src, setSrc] = useState<string | null>(null);
	const box = useRef<HTMLDivElement>(null);
	const release = useRef<Release | null>(null);
	const wanted = project.items > 0 && !failed;
	const url = frameUrl(
		project.timeline_id,
		0,
		THUMBNAIL_WIDTH,
		project.revision,
	);

	useEffect(() => {
		release.current = null;
		setSrc(null);
		const element = box.current;
		if (!wanted || !element) return;

		const ticket = whileNear(thumbnails, (done) => {
			release.current = done;
			setSrc(url);
		});
		// The last record of a batch is the current state of the element.
		const observer = new IntersectionObserver(
			(entries) => {
				const last = entries.at(-1);
				if (last) ticket.near(last.isIntersecting);
			},
			{ rootMargin: "200px" },
		);
		observer.observe(element);

		return () => {
			observer.disconnect();
			ticket.stop();
			release.current = null;
		};
	}, [wanted, url]);

	const settle = (): void => {
		release.current?.();
		release.current = null;
	};

	return (
		<div
			ref={box}
			aria-hidden="true"
			className="flex h-36 items-center justify-center bg-muted text-muted-foreground"
		>
			{wanted ? (
				src && (
					<img
						src={src}
						alt=""
						className="max-h-full max-w-full object-contain"
						onLoad={settle}
						onError={() => {
							settle();
							setFailed(true);
						}}
					/>
				)
			) : (
				<Film aria-hidden="true" className="size-8" />
			)}
		</div>
	);
}

// One project of the grid (SDD-012 section 2.1): the picture, the name, the size
// and the last edit, the way into the editor, and a menu with Rename and Delete.
// Rename is `ProjectUpdate`; Delete moves the project to the trash
// after a confirmation and can be undone from the trash. Project actions are
// the project endpoints of the service, not timeline commands.
export default function ProjectCard({
	project,
	locale,
	onTrash,
	onChanged,
}: {
	project: ProjectSummary;
	locale: string;
	onTrash(): void;
	onChanged(): void;
}): ReactNode {
	const t = useT();
	const [renaming, setRenaming] = useState(false);
	const [confirming, setConfirming] = useState(false);
	const [name, setName] = useState(project.name);
	const [busy, setBusy] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);

	// The copy is a new project with the brief, the texts and the timelines of this one (VV-225).
	const duplicate = async (): Promise<void> => {
		setFailure(null);
		try {
			await createClient(new HttpTransport("/api/v1")).duplicateProject(
				project.id,
			);
			onChanged();
		} catch (error) {
			setFailure(failureOf(error));
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
		<li
			data-project-card={project.id}
			className="flex flex-col overflow-hidden rounded-xl border bg-card text-card-foreground"
		>
			<Thumbnail project={project} />
			<div className="flex flex-1 flex-col gap-3 p-3">
				<div className="flex items-start gap-2">
					<div className="min-w-0 flex-1">
						<p className="truncate font-medium">
							<UntrustedText>{project.name}</UntrustedText>
						</p>
						<p className="text-xs text-muted-foreground">
							{t("ui.projects.summary", {
								width: project.width,
								height: project.height,
								fps: project.fps,
							})}
						</p>
						<p className="text-xs text-muted-foreground">
							{t("ui.projects.card.timelines", { count: project.timelines })}
						</p>
						<p className="text-xs text-muted-foreground">
							{t("ui.projects.updated", {
								date: formatDateTime(project.updated_at, locale),
							})}
						</p>
					</div>
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
				</div>
				{/* The editor is another document, so this is a plain link, not an Inertia visit (SDD-012 section 2.2).
				    It opens the first active timeline, the one the card draws: the timeline with the id of the project
				    may be in the trash of the project. A project with no timeline opens its page, where a video is added. */}
				<div className="mt-auto flex flex-wrap gap-2">
					<Button asChild variant="outline">
						<a
							href={
								project.timeline_id
									? `/editor/${project.timeline_id}`
									: `/projects/${project.id}`
							}
						>
							{t("ui.projects.open")}
							<span className="sr-only">
								{" "}
								<UntrustedText>{project.name}</UntrustedText>
							</span>
						</a>
					</Button>
					<Button asChild variant="outline">
						<Link href={`/projects/${project.id}`}>
							{t("ui.projects.card.details")}
							<span className="sr-only">
								{" "}
								<UntrustedText>{project.name}</UntrustedText>
							</span>
						</Link>
					</Button>
					<BundleExport projectId={project.id} />
				</div>
				<FailureNote failure={failure} />
			</div>

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
							<Label htmlFor={`rename-${project.id}`}>
								{t("ui.projects.name")}
							</Label>
							<Input
								id={`rename-${project.id}`}
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
		</li>
	);
}
