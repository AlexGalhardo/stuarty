import { Head, Link, usePage } from "@inertiajs/react";
import {
	ArrowRight,
	Bot,
	CalendarDays,
	FileText,
	Film,
	FolderOpen,
	FolderPlus,
	ImageIcon,
	Images,
	type LucideIcon,
	PenLine,
	Plug,
	Plus,
	ShieldCheck,
} from "lucide-react";
import type { ReactNode } from "react";

import { EmptyState } from "@/components/EmptyState";
import { LoadError } from "@/components/LoadError";
import { LoadingState } from "@/components/LoadingState";
import { PageHeader, PageHeaderAction } from "@/components/PageHeader";
import { SectionTitle } from "@/components/SectionTitle";
import { StatusBadge } from "@/components/StatusBadge";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { kindIcon as requestIcon } from "@/dashboard/approval/kindIcon";
import { formatMoney } from "@/dashboard/approval/money";
import { formatAgo } from "@/lib/format";
import { type Translate, useLocale, useT } from "@/lib/i18n";
import type { ProjectSummary, SharedProps } from "@/types";
import type { InboxRow } from "../../editor/api/types";
import { useShell } from "../../shell/ShellContext";
import { continueTarget } from "../../workflow/continueTarget";
import { FirstSteps } from "../../workflow/FirstSteps";
import { HomeTrail } from "../../workflow/HomeTrail";
import type { Workflow } from "../../workflow/types";
import { hintWord, titleWord } from "../../workflow/words";
import { dayPart, greetingName } from "./greeting";
import { type HomeState, useHome } from "./useHome";

const NEW_PROJECT = "/projects?new=1";
const OTHERS = 3;
const WAITING_ROWS = 3;

// The six areas of "Your studio": where each kind of work lives. They link to pages that exist; none of them says
// anything about the person's data.
const AREAS: readonly { key: string; href: string; icon: LucideIcon }[] = [
	{ key: "projects", href: "/projects", icon: FolderOpen },
	{ key: "writing", href: "/write", icon: PenLine },
	{ key: "assets", href: "/video-assets", icon: Images },
	{ key: "agent", href: "/skills", icon: Bot },
	{ key: "publish", href: "/publish", icon: CalendarDays },
	{ key: "connections", href: "/connections", icon: Plug },
];

function Studio(): ReactNode {
	const t = useT();
	return (
		<section
			aria-label={t("ui.dashboard.home.studio.title")}
			data-testid="home-studio"
			className="flex flex-col gap-3"
		>
			<SectionTitle level={3} title={t("ui.dashboard.home.studio.title")} />
			<ul className="grid gap-4 sm:grid-cols-2">
				{AREAS.map(({ key, href, icon: Icon }) => (
					<li key={key}>
						<Link
							href={href}
							data-testid={`home-area-${key}`}
							className="flex h-full items-center gap-3 rounded-lg border border-border bg-surface p-4 text-text hover:bg-surface-hover"
						>
							<span
								aria-hidden="true"
								className="flex size-10 shrink-0 items-center justify-center rounded-md bg-accent-soft text-accent-text"
							>
								<Icon className="size-5" />
							</span>
							<span className="min-w-0">
								<span className="block font-medium">
									{t(`ui.dashboard.home.studio.${key}.title`)}
								</span>
								<span className="block text-sm text-text-muted">
									{t(`ui.dashboard.home.studio.${key}.body`)}
								</span>
							</span>
						</Link>
					</li>
				))}
			</ul>
		</section>
	);
}

// One request that waits for the person: what will happen, where it goes and what it costs, as the service
// worded it. The answer is given in Approvals (or on the chat, for a permission), never here.
function WaitingRow({
	row,
	project,
}: {
	row: InboxRow;
	// the name of the project of the request, when the list the home read holds it
	project: string | null;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const Icon = requestIcon(row.tool);
	// An approval has no plain-description key and its summary is the server's English sentence: the home says what
	// kind of request it is in its own words, and the project and the cost go on the line under it.
	const tool = row.tool.toLowerCase();
	const what =
		row.action_key !== ""
			? t(row.action_key)
			: row.source === "approval"
				? t(
						`ui.dashboard.home.waiting.action.${
							tool.includes("publish")
								? "publish"
								: tool.includes("generate")
									? "generate"
									: "approval"
						}`,
					)
				: row.summary !== ""
					? row.summary
					: row.tool;
	const cost =
		row.cost === null
			? t("ui.shell.notify.no_cost")
			: formatMoney(row.cost, locale);
	// The inbox names the destination of an approval by ids, which tell a person nothing: the home says the project
	// by its name when it has it, and what the agent said for a permission.
	const where = project ?? row.item;
	const meta = [where, cost].filter((part) => part !== "").join(", ");
	return (
		<li
			data-testid="home-waiting-row"
			data-source={row.source}
			className="flex items-center gap-3 py-3 first:pt-0"
		>
			<Icon aria-hidden="true" className="size-5 shrink-0 text-accent-text" />
			<div className="min-w-0 flex-1">
				<p className="truncate text-sm font-medium text-text">
					<UntrustedText>{what}</UntrustedText>
				</p>
				<p className="truncate text-xs text-text-subtle">
					<UntrustedText>{meta}</UntrustedText>
				</p>
			</div>
			<StatusBadge status="waiting" className="max-md:hidden">
				{t("ui.dashboard.home.waiting.badge")}
			</StatusBadge>
			<span className="shrink-0 text-xs text-text-subtle md:hidden">
				{formatAgo(row.created_at, locale)}
			</span>
		</li>
	);
}

// What waits for the person, as the one list the service builds from the approval queue and the agent
// permissions (the inbox of the shell, epic 77). The page counts nothing but the rows it is given.
export function Waiting({
	rows,
	projects,
}: {
	rows: readonly InboxRow[] | null;
	projects: readonly ProjectSummary[];
}): ReactNode {
	const t = useT();
	const waiting = (rows ?? []).filter((row) => row.state === "waiting");
	return (
		<section
			aria-label={t("ui.dashboard.home.waiting.title")}
			data-testid="home-waiting"
			className="flex flex-col gap-3"
		>
			<SectionTitle
				level={3}
				title={t("ui.dashboard.home.waiting.title")}
				actions={
					<Button asChild variant="ghost" size="sm">
						<Link href="/approvals">
							<ArrowRight aria-hidden="true" />
							{t("ui.dashboard.home.waiting.open_queue")}
						</Link>
					</Button>
				}
			/>
			<div className="rounded-lg border border-border bg-surface p-4">
				{waiting.length === 0 ? (
					<p className="text-sm text-text-muted">
						{t("ui.dashboard.home.waiting.nothing")}
					</p>
				) : (
					<>
						<ul className="divide-y divide-border">
							{waiting.slice(0, WAITING_ROWS).map((row) => (
								<WaitingRow
									key={row.id}
									row={row}
									project={
										projects.find((p) => p.id === row.project_id)?.name ?? null
									}
								/>
							))}
						</ul>
						{waiting.length > WAITING_ROWS ? (
							<p className="mt-3 text-sm text-text-muted">
								{t("ui.dashboard.home.waiting.more", {
									count: waiting.length - WAITING_ROWS,
								})}
							</p>
						) : null}
						<p className="mt-3 flex items-center gap-2 text-sm text-text-subtle">
							<ShieldCheck aria-hidden="true" className="size-4 shrink-0" />
							{t("ui.dashboard.home.waiting.promise")}
						</p>
					</>
				)}
			</div>
		</section>
	);
}

// The project in progress, with the path the service drew for it and the one button that goes on from the step
// the service says is current.
function InProgress({
	project,
	workflow,
}: {
	project: ProjectSummary;
	workflow: Workflow | null;
}): ReactNode {
	const t = useT();
	const step = workflow?.steps.find((s) => s.id === workflow.current);
	const target =
		step === undefined
			? null
			: continueTarget(step.id, project.id, project.text_summary?.item_id);
	return (
		<section
			aria-labelledby="home-progress"
			data-testid="home-progress"
			className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-5"
		>
			<div className="flex flex-wrap items-start justify-between gap-3 max-md:contents">
				<div className="min-w-0 flex-1 md:basis-64 max-md:order-1 max-md:flex-none">
					<h2
						id="home-progress"
						className="text-sm font-medium text-accent-text max-md:sr-only"
					>
						{t("ui.dashboard.home.progress.title")}
					</h2>
					<p className="text-xl font-semibold [overflow-wrap:anywhere] text-text md:text-2xl">
						<UntrustedText>{project.name}</UntrustedText>
					</p>
					{step === undefined ? null : (
						<p
							className="mt-1 text-base text-text-muted max-md:hidden"
							data-testid="home-at"
						>
							{t("ui.dashboard.home.progress.at", {
								step: titleWord(t, step),
								hint: hintWord(t, step) ?? "",
							})}
						</p>
					)}
				</div>
				<div className="flex flex-wrap items-center gap-2 max-md:order-3 max-md:w-full max-md:flex-col-reverse max-md:items-stretch">
					<Button asChild variant="ghost" className="max-md:hidden">
						<Link href={`/projects/${project.id}`}>
							{t("ui.dashboard.home.progress.open")}
							<span className="sr-only">
								{" "}
								<UntrustedText>{project.name}</UntrustedText>
							</span>
						</Link>
					</Button>
					{step !== undefined && target !== null ? (
						<Button asChild size="lg" data-testid="home-continue">
							<Link href={target}>
								<ArrowRight aria-hidden="true" />
								{t(`ui.dashboard.home.continue.${step.id}`)}
							</Link>
						</Button>
					) : (
						<Button asChild size="lg">
							<Link href={`/projects/${project.id}`}>
								{t("ui.dashboard.home.progress.open")}
								<span className="sr-only">
									{" "}
									<UntrustedText>{project.name}</UntrustedText>
								</span>
							</Link>
						</Button>
					)}
				</div>
			</div>
			{workflow === null ? (
				<p className="text-sm text-text-muted">
					{t("ui.dashboard.home.progress.no_workflow")}
				</p>
			) : (
				<div className="max-md:order-2">
					<HomeTrail workflow={workflow} />
				</div>
			)}
		</section>
	);
}

function kindIcon(project: ProjectSummary): LucideIcon {
	return project.kind === "text"
		? FileText
		: project.kind === "image"
			? ImageIcon
			: Film;
}

// What the service counted for a project, in a few words: a text says its kind and its words, any other project
// says what it is.
function projectMeta(t: Translate, project: ProjectSummary): string {
	const summary = project.text_summary;
	if (project.kind === "text" && summary !== undefined) {
		const kind = ["script", "copy", "caption", "post"].includes(
			summary.text_kind,
		)
			? t(`ui.write.kind.${summary.text_kind}`)
			: t("ui.projects.kind.text");
		return `${kind}, ${t("ui.write.meta.words", { count: summary.words })}`;
	}
	return t(`ui.projects.kind.${project.kind}`);
}

// The other projects the service lists, each with the step it is at.
function Others({
	projects,
	workflows,
}: {
	projects: ProjectSummary[];
	workflows: Record<string, Workflow | null>;
}): ReactNode {
	const t = useT();
	if (projects.length === 0) return null;
	return (
		<section
			aria-label={t("ui.dashboard.home.recent.title")}
			data-testid="home-others"
			className="flex flex-col gap-3"
		>
			<ul className="flex flex-col gap-3">
				{projects.map((project) => {
					const Icon = kindIcon(project);
					const workflow = workflows[project.id] ?? null;
					const step = workflow?.steps.find((s) => s.id === workflow.current);
					return (
						<li
							key={project.id}
							className="flex items-center gap-3 rounded-lg border border-border bg-surface p-4"
						>
							<Icon
								aria-hidden="true"
								className="size-5 shrink-0 text-accent-text"
							/>
							<div className="min-w-0 flex-1">
								<p className="truncate text-sm font-medium text-text">
									<Link
										href={`/projects/${project.id}`}
										className="hover:underline"
									>
										<UntrustedText>{project.name}</UntrustedText>
									</Link>
								</p>
								<p className="truncate text-xs text-text-subtle">
									{projectMeta(t, project)}
								</p>
							</div>
							{step === undefined ? null : (
								<StatusBadge status="info">{titleWord(t, step)}</StatusBadge>
							)}
						</li>
					);
				})}
			</ul>
		</section>
	);
}

// The one line under the greeting: how many productions are at a step and how many requests wait for a yes.
// Both counts are the service's rows and workflows; with neither, the page says what the product is for.
export function summaryLine(
	t: Translate,
	inProgress: number,
	requests: number,
): string {
	const progress =
		inProgress > 0
			? t("ui.dashboard.home.summary.progress", { count: inProgress })
			: "";
	const asked =
		requests > 0
			? t("ui.dashboard.home.summary.requests", { count: requests })
			: "";
	if (progress !== "" && asked !== "") {
		return t("ui.dashboard.home.summary.both", { progress, requests: asked });
	}
	if (progress !== "" || asked !== "")
		return t("ui.dashboard.home.summary.only", { part: progress + asked });
	return t("ui.dashboard.home.lead");
}

export function greetingLine(
	t: Translate,
	hour: number,
	name: string | null,
): string {
	const part = dayPart(hour);
	return name === null
		? t(`ui.dashboard.home.greeting.${part}`)
		: t(`ui.dashboard.home.greeting.${part}_named`, { name });
}

export function Body({
	home,
	rows,
}: {
	home: Exclude<HomeState, { state: "loading" } | { state: "failed" }>;
	rows: readonly InboxRow[] | null;
}): ReactNode {
	const first = home.projects[0];
	const others = home.projects.slice(1, 1 + OTHERS);
	return (
		<div className="flex flex-col gap-6">
			{first === undefined ? null : (
				<InProgress
					project={first}
					workflow={home.workflows[first.id] ?? null}
				/>
			)}
			<div className="grid grid-cols-[minmax(0,1fr)] gap-6 lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
				<div className="max-md:hidden">
					<Studio />
				</div>
				<div className="flex flex-col gap-4">
					<Waiting rows={rows} projects={home.projects} />
					<Others projects={others} workflows={home.workflows} />
				</div>
			</div>
		</div>
	);
}

// What the page draws, from what it is given: the name, the local hour, what the service said of the projects and
// what waits. Kept apart from the reads so that a test can draw it.
export function HomeView({
	home,
	rows,
	name,
	hour,
	onRetry,
}: {
	home: HomeState;
	rows: readonly InboxRow[] | null;
	name: string | null;
	hour: number;
	onRetry?: () => void;
}): ReactNode {
	const t = useT();
	const ready = home.state === "ready" ? home : null;
	const inProgress =
		ready === null
			? 0
			: Object.values(ready.workflows).filter((w) => w?.current !== undefined)
					.length;
	const requests = (rows ?? []).filter((row) => row.state === "waiting").length;
	const empty = ready !== null && ready.projects.length === 0;

	return (
		<div className="mx-auto flex max-w-6xl flex-col gap-6">
			<PageHeader
				title={greetingLine(t, hour, name)}
				description={
					ready === null
						? t("ui.dashboard.home.lead")
						: summaryLine(t, inProgress, requests)
				}
				actions={
					<Button asChild variant="outline" size="lg" className="max-md:hidden">
						<Link href={NEW_PROJECT}>
							<Plus aria-hidden="true" />
							{t("ui.dashboard.home.new_project")}
						</Link>
					</Button>
				}
			/>
			{home.state === "loading" ? (
				<LoadingState message={t("ui.dashboard.home.loading")} />
			) : home.state === "failed" ? (
				<LoadError failure={home.failure} onRetry={onRetry} />
			) : empty ? (
				<>
					<EmptyState
						icon={FolderPlus}
						title={t("ui.dashboard.home.empty.title")}
						description={t("ui.dashboard.home.empty.description")}
					>
						<Button asChild>
							<Link href={NEW_PROJECT}>
								{t("ui.dashboard.home.new_project")}
							</Link>
						</Button>
					</EmptyState>
					<FirstSteps />
					<div className="grid grid-cols-[minmax(0,1fr)] gap-6 lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
						<Studio />
						<Waiting rows={rows} projects={home.projects} />
					</div>
				</>
			) : (
				<Body home={home} rows={rows} />
			)}
		</div>
	);
}

// The home of the application: a greeting, the project in progress with its path, the areas of the studio and what
// waits for the person. Every part is what the service says (SDD-012 section 1): the page draws the projects, their
// workflows and the inbox, and the greeting only words the local hour and the name of the account.
export default function Home(): ReactNode {
	const { retry, ...home } = useHome();
	const { inbox } = useShell();
	const t = useT();
	const account = usePage<SharedProps>().props.account;
	return (
		<>
			<Head title={t("ui.dashboard.home.title")} />
			<HomeView
				home={home}
				rows={inbox.rows}
				name={greetingName(account)}
				hour={new Date().getHours()}
				onRetry={retry}
			/>
		</>
	);
}
