import { Link } from "@inertiajs/react";
import { Film, ImageIcon } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import { type Failure, FailureNote } from "@/components/Failure";
import { StatusBadge } from "@/components/StatusBadge";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { formatDateTime } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { createSlotQueue, type Release, whileNear } from "@/lib/slotQueue";
import type { ProjectSummary } from "@/types";
import { frameUrl } from "../../editor/api/urls";
import { BundleExport } from "./Bundles";
import ProjectMenu from "./ProjectMenu";

const THUMBNAIL_WIDTH = 270;

// The frame endpoint refuses a credential with more than two frames in flight
// (429, SDD-007 section 4.4), so every card draws its picture through this
// queue, and only while it is near the viewport.
const thumbnails = createSlotQueue(2);

// The picture of a card: the frame at the start, as the player draws it. A
// project with nothing on its timeline has no picture, and a frame that cannot
// be read leaves the placeholder, which follows the kind of the project.
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

	const Placeholder = project.kind === "image" ? ImageIcon : Film;

	return (
		<div
			ref={box}
			aria-hidden="true"
			className="flex h-36 items-center justify-center bg-surface-hover text-icon"
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
				<Placeholder aria-hidden="true" className="size-8" />
			)}
		</div>
	);
}

// One project of the grid that is not a text (SDD-012 section 2.1): the picture, the name, its kind, the size and
// the last edit, the way into the editor, and a menu. A project that is not decided yet has no picture box, since
// it is not known to be a video or an image. Project actions are the project endpoints of the service, not
// timeline commands.
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
	const [failure, setFailure] = useState<Failure | null>(null);
	const hasPicture = project.kind !== "undecided";

	return (
		<li
			data-project-card={project.id}
			data-kind={project.kind}
			className="flex flex-col overflow-hidden rounded-lg border border-border bg-surface text-text"
		>
			{hasPicture ? <Thumbnail project={project} /> : null}
			<div className="flex flex-1 flex-col gap-3 p-3">
				<div className="flex items-start gap-2">
					<div className="min-w-0 flex-1">
						<p className="truncate font-medium">
							<UntrustedText>{project.name}</UntrustedText>
						</p>
						<StatusBadge status="neutral" className="my-1">
							{t(`ui.projects.kind.${project.kind}`)}
						</StatusBadge>
						{project.timelines > 0 ? (
							<>
								<p className="text-xs text-text-muted">
									{t("ui.projects.summary", {
										width: project.width,
										height: project.height,
										fps: project.fps,
									})}
								</p>
								<p className="text-xs text-text-muted">
									{t("ui.projects.card.timelines", {
										count: project.timelines,
									})}
								</p>
							</>
						) : null}
						<p className="text-xs text-text-muted">
							{t("ui.projects.updated", {
								date: formatDateTime(project.updated_at, locale),
							})}
						</p>
					</div>
					<ProjectMenu
						project={project}
						onTrash={onTrash}
						onChanged={onChanged}
						onFailure={setFailure}
					/>
				</div>
				{/* The editor is another document, so this is a plain link, not an Inertia visit (SDD-012 section 2.2).
				    It opens the first active timeline, the one the card draws: the timeline with the id of the project
				    may be in the trash of the project. A project with no timeline opens its page, where a medium is added. */}
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
		</li>
	);
}
