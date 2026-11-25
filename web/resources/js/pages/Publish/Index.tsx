import { Head } from "@inertiajs/react";
import { Check, ChevronLeft, ChevronRight, Plus } from "lucide-react";
import { type ReactNode, useEffect, useMemo, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { PageHeader } from "@/components/PageHeader";
import { StatusBadge } from "@/components/StatusBadge";
import { Button } from "@/components/ui/button";
import { useLocale, useT } from "@/lib/i18n";
import type { ProjectSummary } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type {
	Connection,
	SocialAccount,
	SocialNetwork,
} from "../../editor/api/types";
import { CalendarView } from "./CalendarView";
import {
	addDays,
	addMonths,
	browserZone,
	daysOf,
	queryRange,
	today,
	type CalendarView as View,
} from "./calendar";
import { ListView } from "./ListView";
import { PublicationEditor } from "./PublicationEditor";
import { type ListFilter, TARGET_STATES } from "./types";
import { usePublications } from "./usePublications";

type Mode = View | "list";
const MODES: readonly Mode[] = ["month", "week", "list"];

// The week starts on Monday, as in the final design, in both languages.
const weekStart = 1;

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

// The publishing calendar (ADR-0046, SDD-020 section 30): which piece goes to which account and when, in a month,
// a week or a list. It draws what the service answers and sends what the person does; the approval of a post is
// in the Approvals page, and nothing is sent before it.
export default function Publish(): ReactNode {
	const t = useT();
	const locale = useLocale();
	const zone = useMemo(() => browserZone(), []);
	const [mode, setMode] = useState<Mode>("month");
	const [anchor, setAnchor] = useState(() => today(browserZone()));
	const [state, setState] = useState("");
	const [network, setNetwork] = useState("");
	const [projects, setProjects] = useState<ProjectSummary[]>([]);
	const [accounts, setAccounts] = useState<SocialAccount[]>([]);
	const [networks, setNetworks] = useState<SocialNetwork[]>([]);
	const [blogs, setBlogs] = useState<Connection[]>([]);
	const [editing, setEditing] = useState<{
		id: string | null;
		target?: string;
	} | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);

	const filter: ListFilter = useMemo(() => {
		const base: ListFilter = {
			...(state === "" ? {} : { state }),
			...(network === "" ? {} : { network }),
			limit: 500,
		};
		if (mode === "list") return base;
		return { ...base, ...queryRange(daysOf(mode, anchor, weekStart)) };
	}, [mode, anchor, state, network]);

	const feed = usePublications(filter);

	useEffect(() => {
		const client = api();
		client
			.listActiveProjects()
			.then((res) => setProjects(res.projects))
			.catch((error: unknown) => setFailure(failureOf(error)));
		client
			.listSocialAccounts()
			.then((res) => setAccounts(res.accounts))
			.catch((error: unknown) => setFailure(failureOf(error)));
		client
			.listSocialNetworks()
			.then((res) => setNetworks(res.networks))
			.catch((error: unknown) => setFailure(failureOf(error)));
		// The blog destinations are connections of their own kinds (SDD-020 part 5); a build or a key that may not
		// read connections leaves the list empty and a post has nowhere to go.
		client
			.listConnections()
			.then((res) =>
				setBlogs(res.connections.filter((c) => c.kind.startsWith("blog_"))),
			)
			.catch(() => setBlogs([]));
	}, []);

	const publications = feed.publications ?? [];
	// How many targets of the list the service sent wait for a yes or failed: the chips next to the view only count them.
	const targets = publications.flatMap((p) => p.targets);
	const waiting = targets.filter((x) => x.state === "waiting_approval").length;
	const failed = targets.filter((x) => x.state === "failed").length;
	const opened =
		editing?.id == null
			? null
			: (publications.find((p) => p.id === editing.id) ?? null);

	function step(direction: -1 | 1): void {
		setAnchor((was) =>
			mode === "month"
				? addMonths(was, direction)
				: addDays(was, 7 * direction),
		);
	}

	const title = new Intl.DateTimeFormat(locale, {
		month: "long",
		year: "numeric",
		timeZone: "UTC",
	}).format(new Date(`${anchor}T00:00:00Z`));

	return (
		<>
			<Head title={t("ui.publish.title")} />
			<div className="mx-auto flex max-w-6xl flex-col gap-6">
				<PageHeader
					title={t("ui.publish.title")}
					description={t("ui.publish.intro")}
					actions={
						<Button
							type="button"
							size="lg"
							onClick={() => setEditing({ id: null })}
							data-testid="publish-new"
						>
							<Plus aria-hidden="true" />
							{t("ui.publish.new")}
						</Button>
					}
				/>

				<div className="flex flex-wrap items-center gap-3">
					{mode !== "list" && (
						<div className="flex flex-wrap items-center gap-2">
							<Button
								type="button"
								variant="outline"
								size="icon-lg"
								aria-label={t("ui.publish.previous")}
								onClick={() => step(-1)}
							>
								<ChevronLeft aria-hidden="true" />
							</Button>
							<h2
								className="text-center text-xl font-semibold sm:min-w-36"
								data-testid="publish-range"
							>
								{title}
							</h2>
							<Button
								type="button"
								variant="outline"
								size="icon-lg"
								aria-label={t("ui.publish.next")}
								onClick={() => step(1)}
							>
								<ChevronRight aria-hidden="true" />
							</Button>
							<Button
								type="button"
								variant="ghost"
								onClick={() => setAnchor(today(zone))}
							>
								{t("ui.publish.today")}
							</Button>
						</div>
					)}
					<div className="ml-auto flex flex-wrap items-center gap-2">
						{waiting > 0 && (
							<StatusBadge status="waiting" className="px-3 py-1 text-sm">
								{t("ui.publish.count_waiting", { count: waiting })}
							</StatusBadge>
						)}
						{failed > 0 && (
							<StatusBadge status="danger" className="px-3 py-1 text-sm">
								{t("ui.publish.count_failed", { count: failed })}
							</StatusBadge>
						)}
						<fieldset className="inline-flex overflow-hidden rounded-lg border border-border-strong p-1">
							<legend className="sr-only">{t("ui.publish.view")}</legend>
							{MODES.map((m) => (
								<Button
									key={m}
									type="button"
									variant={mode === m ? "secondary" : "ghost"}
									aria-pressed={mode === m}
									onClick={() => setMode(m)}
									data-testid={`publish-mode-${m}`}
								>
									{mode === m ? <Check aria-hidden="true" /> : null}
									{t(`ui.publish.view_${m}`)}
								</Button>
							))}
						</fieldset>
					</div>
				</div>

				<div className="flex flex-wrap items-center gap-2 text-sm">
					<label className="flex items-center gap-2">
						{t("ui.publish.filter.state")}
						<select
							className="h-8 rounded-lg border border-input bg-transparent px-2 text-sm"
							value={state}
							onChange={(event) => setState(event.target.value)}
							data-testid="publish-filter-state"
						>
							<option value="">{t("ui.publish.filter.all")}</option>
							{TARGET_STATES.map((s) => (
								<option key={s} value={s}>
									{t(`ui.publish.state.${s}`)}
								</option>
							))}
						</select>
					</label>
					<label className="flex items-center gap-2">
						{t("ui.publish.filter.network")}
						<select
							className="h-8 rounded-lg border border-input bg-transparent px-2 text-sm"
							value={network}
							onChange={(event) => setNetwork(event.target.value)}
							data-testid="publish-filter-network"
						>
							<option value="">{t("ui.publish.filter.all")}</option>
							{networks.map((n) => (
								<option key={n.network} value={n.network}>
									{n.name}
								</option>
							))}
						</select>
					</label>
					<span className="text-xs text-muted-foreground" aria-live="polite">
						{feed.live ? t("ui.publish.live") : ""}
					</span>
				</div>

				<FailureNote failure={failure ?? feed.failure} />
				{feed.publications === null && feed.failure === null && (
					<p
						role="status"
						className="text-sm text-muted-foreground"
						data-testid="publish-loading"
					>
						{t("ui.publish.loading")}
					</p>
				)}

				{mode === "list" ? (
					<ListView
						publications={feed.publications}
						onOpen={(id, target) => setEditing({ id, target })}
					/>
				) : (
					<CalendarView
						view={mode}
						anchor={anchor}
						weekStart={weekStart}
						zone={zone}
						publications={publications}
						onOpen={(id, target) => setEditing({ id, target })}
						onMove={(id, target, local) =>
							void feed.api
								.schedule(id, { target_id: target, at: local, zone })
								.then(() => feed.reload())
								.catch((error: unknown) => feed.fail(error))
						}
					/>
				)}
			</div>
			{editing !== null && (editing.id === null || opened !== null) && (
				<PublicationEditor
					key={editing.id ?? "new"}
					publication={opened}
					projects={projects}
					accounts={accounts}
					networks={networks}
					blogs={blogs}
					api={feed.api}
					onClose={() => setEditing(null)}
					onChanged={() => void feed.reload()}
				/>
			)}
		</>
	);
}
