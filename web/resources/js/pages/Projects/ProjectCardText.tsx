import { Link } from "@inertiajs/react";
import { Clock, FileText } from "lucide-react";
import { type ReactNode, useState } from "react";

import { type Failure, FailureNote } from "@/components/Failure";
import { StatusBadge } from "@/components/StatusBadge";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { formatDateTime } from "@/lib/format";
import { useT } from "@/lib/i18n";
import type { ProjectSummary } from "@/types";
import ProjectFacts from "./ProjectFacts";
import ProjectMenu from "./ProjectMenu";

const TEXT_KINDS = ["script", "copy", "caption", "post"] as const;

// The card of a project that is a text: its excerpt, its type, its words and its reading time, all as the service
// counted them, and no picture box, so that a text looks complete without a thumbnail. A text offers no export of
// a bundle: there is no media to bring along.
export default function ProjectCardText({
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
	const summary = project.text_summary;
	const kind = TEXT_KINDS.find((k) => k === summary?.text_kind);
	const write =
		summary === undefined
			? `/projects/${project.id}`
			: `/write?project=${encodeURIComponent(project.id)}&text=${encodeURIComponent(summary.item_id)}`;

	return (
		<li
			data-project-card={project.id}
			data-kind="text"
			className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-4 text-text"
		>
			<div className="flex items-start gap-2">
				<span
					aria-hidden="true"
					className="flex size-9 shrink-0 items-center justify-center rounded-md bg-accent-soft text-accent-text"
				>
					<FileText className="size-5" />
				</span>
				<div className="min-w-0 flex-1">
					<p className="truncate font-semibold">
						<UntrustedText>{project.name}</UntrustedText>
					</p>
					<div className="mt-1 flex flex-wrap gap-1">
						<StatusBadge status="neutral">
							{t("ui.projects.kind.text")}
						</StatusBadge>
						{kind !== undefined ? (
							<StatusBadge status="info">
								{t(`ui.write.kind.${kind}`)}
							</StatusBadge>
						) : null}
					</div>
				</div>
				<ProjectMenu
					project={project}
					onTrash={onTrash}
					onChanged={onChanged}
					onFailure={setFailure}
				/>
			</div>
			{summary === undefined || summary.excerpt === "" ? (
				<p className="text-sm text-text-muted" data-testid="excerpt-empty">
					{t("ui.projects.card.no_text")}
				</p>
			) : (
				<p
					className="line-clamp-3 text-sm text-text-muted"
					data-testid="excerpt"
				>
					<UntrustedText>{summary.excerpt}</UntrustedText>
				</p>
			)}
			<p className="flex flex-wrap items-center gap-x-3 text-xs text-text-muted">
				{summary === undefined ? null : (
					<>
						<span>{t("ui.projects.card.words", { count: summary.words })}</span>
						<span className="flex items-center gap-1">
							<Clock aria-hidden="true" className="size-3" />
							{t("ui.projects.card.reading", {
								count: summary.reading_minutes,
							})}
						</span>
					</>
				)}
				<span>
					{t("ui.projects.updated", {
						date: formatDateTime(project.updated_at, locale),
					})}
				</span>
			</p>
			<ProjectFacts project={project} locale={locale} />
			<div className="mt-auto flex flex-wrap gap-2">
				<Button asChild variant="ghost" size="sm">
					{summary === undefined ? (
						<Link href={write}>
							{t("ui.projects.open")}
							<span className="sr-only">
								{" "}
								<UntrustedText>{project.name}</UntrustedText>
							</span>
						</Link>
					) : (
						<a href={write}>
							{t("ui.projects.card.write")}
							<span className="sr-only">
								{" "}
								<UntrustedText>{project.name}</UntrustedText>
							</span>
						</a>
					)}
				</Button>
				<Button asChild variant="ghost" size="sm">
					<Link href={`/projects/${project.id}`}>
						{t("ui.projects.card.details")}
						<span className="sr-only">
							{" "}
							<UntrustedText>{project.name}</UntrustedText>
						</span>
					</Link>
				</Button>
			</div>
			<FailureNote failure={failure} />
		</li>
	);
}
