import { Head, Link } from "@inertiajs/react";
import { FolderPlus } from "lucide-react";
import type { ReactNode } from "react";

import { EmptyState } from "@/components/EmptyState";
import { FailureNote } from "@/components/Failure";
import { ListRow } from "@/components/ListRow";
import { LoadingState } from "@/components/LoadingState";
import { Notice } from "@/components/Notice";
import { PageHeader } from "@/components/PageHeader";
import { SectionTitle } from "@/components/SectionTitle";
import { StatusBadge } from "@/components/StatusBadge";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { SECTIONS } from "@/dashboard/sections";
import { useT } from "@/lib/i18n";
import type { ProjectSummary } from "@/types";
import { FirstSteps } from "../../workflow/FirstSteps";
import { Trail } from "../../workflow/Trail";
import type { Workflow } from "../../workflow/types";
import { titleWord } from "../../workflow/words";
import { type HomeState, useHome } from "./useHome";

const NEW_PROJECT = "/projects?new=1";
const RECENT = 4;

// What waits for the person: the approvals the queue holds and the steps of the project in progress that the
// service says wait for them. Both are the service's words; the page only lists them.
function Waiting({
	workflow,
	approvals,
}: {
	workflow: Workflow | null;
	approvals: number | null;
}): ReactNode {
	const t = useT();
	const steps = (workflow?.steps ?? []).filter((s) => s.state === "waiting");
	const nothing = steps.length === 0 && (approvals ?? 0) === 0;
	return (
		<section
			aria-label={t("ui.dashboard.home.waiting.title")}
			data-testid="home-waiting"
			className="flex flex-col gap-3"
		>
			<SectionTitle level={3} title={t("ui.dashboard.home.waiting.title")} />
			{nothing ? (
				<p className="text-sm text-text-muted">
					{t("ui.dashboard.home.waiting.nothing")}
				</p>
			) : (
				<div className="flex flex-col gap-2">
					{(approvals ?? 0) > 0 ? (
						<Notice
							tone="warning"
							title={t("ui.dashboard.home.waiting.approvals", {
								count: approvals ?? 0,
							})}
							actions={
								<Button asChild variant="outline" size="sm">
									<Link href="/approvals">
										{t("ui.dashboard.home.waiting.review")}
									</Link>
								</Button>
							}
						/>
					) : null}
					{steps.map((step) => (
						<Notice
							key={step.id}
							tone="info"
							title={t("ui.dashboard.home.waiting.step", {
								step: titleWord(t, step),
							})}
						/>
					))}
				</div>
			)}
		</section>
	);
}

// The project in progress with the path the service drew for it and the step that comes next.
function InProgress({
	project,
	workflow,
}: {
	project: ProjectSummary;
	workflow: Workflow | null;
}): ReactNode {
	const t = useT();
	const next = workflow?.steps.find((s) => s.id === workflow.current);
	return (
		<section
			aria-labelledby="home-progress"
			data-testid="home-progress"
			className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-5"
		>
			<div className="flex flex-wrap items-start justify-between gap-3">
				<div className="min-w-0">
					<h2
						id="home-progress"
						className="text-sm font-semibold text-text-muted"
					>
						{t("ui.dashboard.home.progress.title")}
					</h2>
					<p className="mt-1 text-2xl font-semibold text-text">
						<UntrustedText>{project.name}</UntrustedText>
					</p>
				</div>
				<StatusBadge status="neutral">
					{t(`ui.projects.kind.${project.kind}`)}
				</StatusBadge>
			</div>
			{workflow === null ? (
				<p className="text-sm text-text-muted">
					{t("ui.dashboard.home.progress.no_workflow")}
				</p>
			) : (
				<>
					<Trail workflow={workflow} />
					<p className="text-base text-text" data-testid="home-next">
						{next === undefined
							? t("ui.dashboard.home.progress.no_next")
							: t("ui.dashboard.home.progress.next", {
									step: titleWord(t, next),
								})}
					</p>
				</>
			)}
			<div>
				<Button asChild>
					<Link href={`/projects/${project.id}`}>
						{t("ui.dashboard.home.progress.open")}
						<span className="sr-only">
							{" "}
							<UntrustedText>{project.name}</UntrustedText>
						</span>
					</Link>
				</Button>
			</div>
		</section>
	);
}

function Recent({ projects }: { projects: ProjectSummary[] }): ReactNode {
	const t = useT();
	return (
		<section
			aria-label={t("ui.dashboard.home.recent.title")}
			className="flex flex-col gap-3"
		>
			<SectionTitle
				level={3}
				title={t("ui.dashboard.home.recent.title")}
				actions={
					<Button asChild variant="ghost" size="sm">
						<Link href="/projects">{t("ui.dashboard.home.recent.all")}</Link>
					</Button>
				}
			/>
			<ul className="divide-y divide-border rounded-lg border border-border bg-surface">
				{projects.slice(0, RECENT).map((project) => (
					<ListRow
						key={project.id}
						title={
							<Link href={`/projects/${project.id}`} className="underline">
								<UntrustedText>{project.name}</UntrustedText>
							</Link>
						}
						meta={t(`ui.projects.kind.${project.kind}`)}
					/>
				))}
			</ul>
		</section>
	);
}

function Sections(): ReactNode {
	const t = useT();
	return (
		<section
			aria-label={t("ui.dashboard.home.sections.title")}
			className="flex flex-col gap-3"
		>
			<SectionTitle
				level={3}
				title={t("ui.dashboard.home.sections.title")}
				description={t("ui.dashboard.home.sections.description")}
			/>
			<ul className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,15rem),1fr))] gap-4">
				{SECTIONS.map(({ key, path, icon: Icon }) => (
					<li key={key}>
						<Link
							href={path}
							data-testid={`home-${key}`}
							className="flex h-full flex-col gap-2 rounded-lg border border-border bg-surface p-4 text-text hover:bg-surface-hover"
						>
							<span className="flex items-center gap-2 font-medium">
								<Icon aria-hidden="true" className="size-4 text-icon" />
								{t(`ui.nav.section.${key}`)}
							</span>
							<span className="text-sm text-text-muted">
								{t(`ui.dashboard.section.${key}.holds`)}
							</span>
						</Link>
					</li>
				))}
			</ul>
		</section>
	);
}

function Body({ home }: { home: HomeState }): ReactNode {
	const t = useT();
	if (home.state === "loading") {
		return <LoadingState message={t("ui.dashboard.home.loading")} />;
	}
	if (home.state === "failed") {
		return <FailureNote failure={home.failure} />;
	}
	const first = home.projects[0];
	if (first === undefined) {
		return (
			<EmptyState
				icon={FolderPlus}
				title={t("ui.dashboard.home.empty.title")}
				description={t("ui.dashboard.home.empty.description")}
			>
				<Button asChild>
					<Link href={NEW_PROJECT}>{t("ui.dashboard.home.new_project")}</Link>
				</Button>
			</EmptyState>
		);
	}
	return (
		<div className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
			<InProgress project={first} workflow={home.workflow} />
			<Waiting workflow={home.workflow} approvals={home.approvals} />
			<div className="lg:col-span-2">
				<Recent projects={home.projects} />
			</div>
		</div>
	);
}

// The home of the application: what to do next first, then the projects, then the sections. Every part is what
// the service says (SDD-012 section 1): the page draws the projects, their workflow and the approvals that wait.
export default function Home(): ReactNode {
	const t = useT();
	const home = useHome();
	const empty = home.state === "ready" && home.projects.length === 0;

	return (
		<>
			<Head title={t("ui.dashboard.home.title")} />
			<div className="mx-auto flex max-w-6xl flex-col gap-8">
				<PageHeader
					title={t("ui.dashboard.home.title")}
					description={t("ui.dashboard.home.lead")}
					actions={
						<Button asChild>
							<Link href={NEW_PROJECT}>
								{t("ui.dashboard.home.new_project")}
							</Link>
						</Button>
					}
				/>
				<Body home={home} />
				{empty ? <FirstSteps /> : null}
				<Sections />
			</div>
		</>
	);
}
