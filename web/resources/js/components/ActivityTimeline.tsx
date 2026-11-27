import { Link } from "@inertiajs/react";
import { Clock, RefreshCw } from "lucide-react";
import {
	type ReactNode,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { StatusBadge } from "@/components/StatusBadge";
import UntrustedText from "@/components/UntrustedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { createClient } from "@/editor/api/client";
import { HttpTransport } from "@/editor/api/http";
import type {
	ActivityActorKind,
	ActivityDomain,
	ActivityEntry,
} from "@/editor/api/types";
import { type Translate, useLocale, useT } from "@/lib/i18n";
import type { ProjectSummary } from "@/types";

const SELECT =
	"h-9 rounded-md border border-input bg-surface px-2.5 text-sm text-text outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50";

const ACTOR_FILTERS = ["all", "human", "agent", "system"] as const;
const DOMAIN_FILTERS = [
	"all",
	"project",
	"video",
	"image",
	"asset",
	"writing",
	"publish",
	"approval",
	"agent",
	"connection",
	"account",
	"workspace",
] as const;

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

// Converts an audit action like "project.create" or "approval.ceiling.set" into
// its catalogue key under `ui.history.action.*`.
export function actionLabel(t: Translate, action: string): string {
	const slug = action.toLowerCase().replace(/[^a-z0-9]+/g, "_");
	const key = `ui.history.action.${slug}`;
	const text = t(key);
	return text === key ? t("ui.history.action.unknown") : text;
}

function formatDayHeading(iso: string, locale: string): string {
	const date = new Date(iso);
	if (Number.isNaN(date.getTime())) return iso.slice(0, 10);
	return new Intl.DateTimeFormat(locale, {
		dateStyle: "long",
	}).format(date);
}

function formatTime(iso: string, locale: string): string {
	const date = new Date(iso);
	if (Number.isNaN(date.getTime())) return iso;
	return new Intl.DateTimeFormat(locale, {
		timeStyle: "short",
	}).format(date);
}

function actorBadgeStatus(
	kind: ActivityActorKind,
): "success" | "agent" | "neutral" {
	if (kind === "agent") return "agent";
	if (kind === "human") return "success";
	return "neutral";
}

function subjectHref(entry: ActivityEntry): string | undefined {
	if (entry.subject_id === undefined) return;
	const id = encodeURIComponent(entry.subject_id);
	if (entry.subject_kind === "video") return `/editor/${id}`;
	if (entry.subject_kind === "image") return `/edit-image/${id}`;
	if (entry.subject_kind === "text" && entry.project_id !== undefined) {
		return `/write?project=${encodeURIComponent(entry.project_id)}&text=${id}`;
	}
}

export interface ActivityTimelineProps {
	// When set, queries only this project's activity and hides the project filter.
	projectId?: string;
	// Optional pre-loaded entries for server-side or unit tests.
	initialEntries?: readonly ActivityEntry[];
}

export function ActivityTimeline({
	projectId,
	initialEntries,
}: ActivityTimelineProps): ReactNode {
	const t = useT();
	const locale = useLocale();
	const [entries, setEntries] = useState<ActivityEntry[]>(() =>
		initialEntries ? [...initialEntries] : [],
	);
	const [nextCursor, setNextCursor] = useState<string | null>(null);
	const [loading, setLoading] = useState(initialEntries === undefined);
	const [loadingMore, setLoadingMore] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [actorKind, setActorKind] = useState<"all" | ActivityActorKind>("all");
	const [domain, setDomain] = useState<"all" | ActivityDomain>("all");
	const [selectedProject, setSelectedProject] = useState<string>(
		projectId ?? "",
	);
	const [projects, setProjects] = useState<ProjectSummary[]>([]);
	const requestId = useRef(0);

	useEffect(() => {
		if (projectId !== undefined || initialEntries !== undefined) return;
		let cancelled = false;
		async function loadProjects() {
			const client = api();
			let cursor: string | undefined;
			do {
				const firstPage = cursor === undefined;
				const res = await client.listProjects({
					cursor,
					limit: 100,
					state: "all",
				});
				if (cancelled) return;
				setProjects((current) =>
					firstPage ? res.projects : [...current, ...res.projects],
				);
				cursor = res.next_cursor ?? undefined;
			} while (cursor !== undefined);
		}
		void loadProjects().catch(() => undefined);
		return () => {
			cancelled = true;
		};
	}, [projectId, initialEntries]);

	const fetchPage = useCallback(
		async (cursor?: string) => {
			const id = ++requestId.current;
			if (cursor === undefined) {
				setLoading(true);
				setEntries([]);
				setNextCursor(null);
			} else {
				setLoadingMore(true);
			}
			setFailure(null);
			try {
				const query = {
					actorKind,
					domain,
					cursor,
					limit: 50,
				};
				const res =
					projectId !== undefined
						? await api().listProjectActivity(projectId, query)
						: await api().listHistory({
								...query,
								projectId: selectedProject || undefined,
							});
				if (id !== requestId.current) return;
				setEntries((prev) =>
					cursor === undefined ? res.entries : [...prev, ...res.entries],
				);
				setNextCursor(res.next_cursor);
			} catch (error) {
				if (id === requestId.current) setFailure(failureOf(error));
			} finally {
				if (id === requestId.current) {
					setLoading(false);
					setLoadingMore(false);
				}
			}
		},
		[actorKind, domain, projectId, selectedProject],
	);

	useEffect(() => {
		if (initialEntries !== undefined) return;
		void fetchPage();
		return () => {
			requestId.current++;
		};
	}, [fetchPage, initialEntries]);

	const grouped = useMemo(() => {
		const groups: { day: string; items: ActivityEntry[] }[] = [];
		for (const entry of entries) {
			const day = formatDayHeading(entry.at, locale);
			const last = groups[groups.length - 1];
			if (last !== undefined && last.day === day) {
				last.items.push(entry);
			} else {
				groups.push({ day, items: [entry] });
			}
		}
		return groups;
	}, [entries, locale]);

	const filterIdPrefix =
		projectId !== undefined ? `project-history-${projectId}` : "global-history";

	return (
		<div
			className="flex flex-col gap-4"
			data-testid={
				projectId !== undefined
					? "project-activity-timeline"
					: "activity-timeline"
			}
		>
			<div className="flex flex-wrap items-end gap-3 rounded-lg border border-border bg-surface p-3">
				<div className="flex flex-col gap-1">
					<Label htmlFor={`${filterIdPrefix}-actor`} className="text-xs">
						{t("ui.history.filter.actor")}
					</Label>
					<select
						id={`${filterIdPrefix}-actor`}
						data-testid="history-filter-actor"
						className={SELECT}
						value={actorKind}
						onChange={(e) =>
							setActorKind(e.target.value as "all" | ActivityActorKind)
						}
					>
						{ACTOR_FILTERS.map((kind) => (
							<option key={kind} value={kind}>
								{t(`ui.history.actor.${kind}`)}
							</option>
						))}
					</select>
				</div>

				<div className="flex flex-col gap-1">
					<Label htmlFor={`${filterIdPrefix}-domain`} className="text-xs">
						{t("ui.history.filter.domain")}
					</Label>
					<select
						id={`${filterIdPrefix}-domain`}
						data-testid="history-filter-domain"
						className={SELECT}
						value={domain}
						onChange={(e) =>
							setDomain(e.target.value as "all" | ActivityDomain)
						}
					>
						{DOMAIN_FILTERS.map((d) => (
							<option key={d} value={d}>
								{t(`ui.history.domain.${d}`)}
							</option>
						))}
					</select>
				</div>

				{projectId === undefined && (
					<div className="flex flex-col gap-1">
						<Label htmlFor={`${filterIdPrefix}-project`} className="text-xs">
							{t("ui.history.filter.project")}
						</Label>
						<select
							id={`${filterIdPrefix}-project`}
							data-testid="history-filter-project"
							className={SELECT}
							value={selectedProject}
							onChange={(e) => setSelectedProject(e.target.value)}
						>
							<option value="">{t("ui.history.filter.all_projects")}</option>
							{projects.map((p) => (
								<option key={p.id} value={p.id}>
									{p.name}
								</option>
							))}
						</select>
					</div>
				)}

				<div className="ml-auto">
					<Button
						type="button"
						variant="outline"
						size="sm"
						disabled={loading}
						onClick={() => void fetchPage()}
					>
						<RefreshCw aria-hidden="true" className="size-3.5" />
						{t("ui.history.refresh")}
					</Button>
				</div>
			</div>

			<FailureNote failure={failure} />

			{loading ? (
				<p role="status" className="text-sm text-text-muted">
					{t("ui.history.loading")}
				</p>
			) : grouped.length === 0 ? (
				<div
					data-testid="history-empty"
					className="flex flex-col items-center gap-2 rounded-lg border border-dashed border-border p-8 text-center"
				>
					<Clock aria-hidden="true" className="size-6 text-text-muted" />
					<p className="text-sm font-medium">{t("ui.history.empty.title")}</p>
					<p className="text-sm text-text-muted">
						{t("ui.history.empty.hint")}
					</p>
				</div>
			) : (
				<div className="flex flex-col gap-5">
					{grouped.map((group) => (
						<section key={group.day} className="flex flex-col gap-2">
							<h3 className="text-xs font-semibold tracking-wider text-text-muted uppercase">
								{group.day}
							</h3>
							<ol className="flex flex-col gap-2">
								{group.items.map((entry) => (
									<li
										key={entry.id}
										data-testid="history-entry"
										data-actor-kind={entry.actor_kind}
										data-domain={entry.domain}
										className="flex flex-col gap-1.5 rounded-lg border border-border bg-surface p-3 text-sm"
									>
										<div className="flex flex-wrap items-center gap-2">
											<StatusBadge status={actorBadgeStatus(entry.actor_kind)}>
												{t(`ui.history.actor.${entry.actor_kind}`)}
											</StatusBadge>
											<Badge variant="outline">
												{t(`ui.history.domain.${entry.domain}`)}
											</Badge>
											<span className="font-medium text-text">
												{actionLabel(t, entry.action)}
											</span>
											<span className="ml-auto text-xs text-text-muted [font-variant-numeric:tabular-nums]">
												{formatTime(entry.at, locale)}
											</span>
										</div>

										<div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-text-muted">
											<span>
												{t("ui.history.by")}{" "}
												<UntrustedText>{entry.actor_label}</UntrustedText>
											</span>
											{projectId === undefined &&
												entry.project_id !== undefined && (
													<Link
														href={`/projects/${encodeURIComponent(entry.project_id)}`}
														className="font-medium text-accent-text underline-offset-2 hover:underline"
													>
														<UntrustedText>
															{entry.project_name || entry.project_id}
														</UntrustedText>
													</Link>
												)}
											{entry.revision !== undefined && (
												<a
													href={subjectHref(entry)}
													className="font-medium text-accent-text underline-offset-2 hover:underline"
												>
													{t("ui.history.revision", {
														revision: entry.revision,
													})}
												</a>
											)}
											{entry.revision === undefined &&
												entry.subject_id !== undefined && (
													<span className="font-mono text-text-subtle">
														<UntrustedText>{entry.subject_id}</UntrustedText>
													</span>
												)}
										</div>

										{entry.details !== undefined && (
											<details className="pt-1 text-xs text-text-muted">
												<summary className="cursor-pointer font-medium">
													{t("ui.history.details")}
												</summary>
												<pre className="mt-1 max-h-32 overflow-auto rounded bg-surface-sunken p-2 font-mono text-xs">
													<UntrustedText>
														{JSON.stringify(entry.details, null, 2)}
													</UntrustedText>
												</pre>
											</details>
										)}
									</li>
								))}
							</ol>
						</section>
					))}
				</div>
			)}
			{nextCursor !== null && (
				<div className="flex justify-center">
					<Button
						type="button"
						variant="outline"
						disabled={loadingMore}
						onClick={() => void fetchPage(nextCursor)}
					>
						{loadingMore ? t("ui.history.loading") : t("ui.history.load_more")}
					</Button>
				</div>
			)}
		</div>
	);
}
