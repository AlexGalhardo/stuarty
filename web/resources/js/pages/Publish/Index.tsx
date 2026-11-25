import { Head } from "@inertiajs/react";
import {
	CalendarClock,
	Check,
	ChevronLeft,
	ChevronRight,
	Download,
	Plus,
} from "lucide-react";
import { type ReactNode, useEffect, useMemo, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { FilterBar } from "@/components/FilterBar";
import { GuideStrip } from "@/components/GuideStrip";
import { NumberedStep } from "@/components/NumberedStep";
import { OptionSelect } from "@/components/OptionSelect";
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
	dateRange,
	daysOf,
	monthDays,
	queryRange,
	today,
	type CalendarView as View,
	wallOf,
} from "./calendar";
import { exportHref } from "./calendarApi";
import { type Adjustment, DayView } from "./DayView";
import type { CalendarDay } from "./dayTypes";
import { ListView } from "./ListView";
import { MiniMonth } from "./MiniMonth";
import { type EditorInitial, PublicationEditor } from "./PublicationEditor";
import { SlotsDialog } from "./SlotsDialog";
import { type ListFilter, type Publication, TARGET_STATES } from "./types";
import { useCalendarPlan } from "./useCalendarPlan";
import { usePublications } from "./usePublications";
import { QuietDeclined, useQuietConfirm } from "./useQuietConfirm";

type Mode = View | "list";
const MODES: readonly Mode[] = ["day", "week", "month", "list"];

// The week starts on Monday, as in the final design, in both languages.
const weekStart = 1;

// The value of "All" in a filter: the select has no empty option.
const ALL = "all";

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

// The publishing calendar (ADR-0046, SDD-020 sections 30 and 30A): which piece goes to which account and when, in a
// day, a week, a month or a list. It draws what the service answers and sends what the person does; the approval of a
// post is in the Approvals page, and nothing is sent before it. A day is opened from a day of the month or the week, from
// the small month of the header or by a date typed there.
export default function Publish(): ReactNode {
	const t = useT();
	const locale = useLocale();
	const zone = useMemo(() => browserZone(), []);
	const [mode, setMode] = useState<Mode>("month");
	const [anchor, setAnchor] = useState(() => today(browserZone()));
	const [mini, setMini] = useState(anchor);
	const [state, setState] = useState("");
	const [network, setNetwork] = useState("");
	const [projects, setProjects] = useState<ProjectSummary[]>([]);
	const [accounts, setAccounts] = useState<SocialAccount[]>([]);
	const [networks, setNetworks] = useState<SocialNetwork[]>([]);
	const [blogs, setBlogs] = useState<Connection[]>([]);
	const [editing, setEditing] = useState<{
		id: string | null;
		target?: string;
		initial?: EditorInitial;
	} | null>(null);
	const [slotsOpen, setSlotsOpen] = useState(false);
	const [failure, setFailure] = useState<Failure | null>(null);
	const quiet = useQuietConfirm();
	const gridView: Exclude<View, "day"> = mode === "week" ? "week" : "month";
	const days = useMemo(
		() => daysOf(mode === "list" ? "month" : mode, anchor, weekStart),
		[mode, anchor],
	);

	const filter: ListFilter = useMemo(() => {
		const base: ListFilter = {
			...(state === "" ? {} : { state }),
			...(network === "" ? {} : { network }),
			limit: 500,
		};
		if (mode === "list") return base;
		return { ...base, ...queryRange(days) };
	}, [mode, days, state, network]);

	const feed = usePublications(filter);
	const publications = feed.publications ?? [];

	// What a publication last looked like, so the slots that a post fills are read again when it changes.
	const refresh = publications.map((p) => `${p.id}@${p.updated_at}`).join(",");
	const view = useCalendarPlan({ ...dateRange(days), zone }, refresh);
	const miniPlan = useCalendarPlan(
		{ ...dateRange(monthDays(mini, weekStart)), zone },
		refresh,
	);
	const marks = useMemo(
		() =>
			new Map<string, CalendarDay>(
				(view.plan?.days ?? []).map((d) => [d.date, d]),
			),
		[view.plan],
	);

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

	// How many targets of the list the service sent wait for a yes or failed: the chips next to the view only count them.
	const targets = publications.flatMap((p) => p.targets);
	// The name the person gave to each destination, by connection id, for the list.
	const accountNames: Record<string, string> = {
		...Object.fromEntries(blogs.map((c) => [c.id, c.label])),
		...Object.fromEntries(accounts.map((a) => [a.connection_id, a.label])),
	};
	const waiting = targets.filter((x) => x.state === "waiting_approval").length;
	const failed = targets.filter((x) => x.state === "failed").length;

	// The small month follows the date the page is on.
	useEffect(() => setMini(anchor), [anchor]);

	const opened =
		editing?.id == null
			? null
			: (publications.find((p) => p.id === editing.id) ?? null);

	function step(direction: -1 | 1): void {
		setAnchor((was) =>
			mode === "month"
				? addMonths(was, direction)
				: addDays(was, (mode === "day" ? 1 : 7) * direction),
		);
	}

	function openDay(date: string): void {
		setAnchor(date);
		setMode("day");
	}

	function reloadAll(): void {
		void feed.reload();
		void view.reload();
		void miniPlan.reload();
	}

	// Scheduling on a quiet day is the person's, who is asked first (D-146); a no leaves everything as it was.
	function move(id: string, target: string, local: string): void {
		void quiet
			.run((confirm) =>
				feed.api.schedule(id, {
					target_id: target,
					at: local,
					zone,
					...(confirm ? { confirm_quiet_day: true } : {}),
				}),
			)
			.then(() => reloadAll())
			.catch((error: unknown) => {
				if (!(error instanceof QuietDeclined)) feed.fail(error);
			});
	}

	// The change of a post made in place in the day: its time, its account or its text, sent to the service as an edit.
	async function adjust(
		id: string,
		targetId: string,
		change: Adjustment,
	): Promise<boolean> {
		const target = publications
			.find((p) => p.id === id)
			?.targets.find((x) => x.id === targetId);
		if (target === undefined) return false;
		try {
			await quiet.run((confirm) =>
				feed.api.update(id, {
					targets: [
						{
							id: targetId,
							...(change.connection_id === undefined
								? {}
								: { connection_id: change.connection_id }),
							...(change.at === undefined ? {} : { at: change.at, zone }),
							...(change.description === undefined
								? {}
								: {
										fields: {
											...target.fields,
											description: change.description,
										},
									}),
						},
					],
					...(confirm ? { confirm_quiet_day: true } : {}),
				}),
			);
			reloadAll();
			return true;
		} catch (error) {
			if (!(error instanceof QuietDeclined)) feed.fail(error);
			return false;
		}
	}

	function duplicated(copy: Publication): void {
		const at = copy.targets[0]?.scheduled_at;
		const day = at == null ? null : wallOf(at, zone)?.day;
		if (day != null) setAnchor(day);
		setEditing({ id: copy.id });
	}

	const title = new Intl.DateTimeFormat(
		locale,
		mode === "day"
			? {
					weekday: "long",
					day: "numeric",
					month: "long",
					year: "numeric",
					timeZone: "UTC",
				}
			: { month: "long", year: "numeric", timeZone: "UTC" },
	).format(new Date(`${anchor}T00:00:00Z`));

	return (
		<>
			<Head title={t("ui.publish.title")} />
			<div className="mx-auto flex max-w-6xl flex-col gap-6">
				<PageHeader
					title={t("ui.publish.title")}
					description={t("ui.publish.intro")}
					actions={
						<>
							<Button
								type="button"
								variant="outline"
								size="lg"
								onClick={() => setSlotsOpen(true)}
								data-testid="publish-slots"
							>
								<CalendarClock aria-hidden="true" />
								{t("ui.publish.slots.open")}
							</Button>
							<Button asChild variant="outline" size="lg">
								<a
									href={exportHref({ ...dateRange(days), zone })}
									download
									data-testid="publish-export"
								>
									<Download aria-hidden="true" />
									{t("ui.publish.export")}
								</a>
							</Button>
							<Button
								type="button"
								size="lg"
								onClick={() => setEditing({ id: null })}
								data-testid="publish-new"
							>
								<Plus aria-hidden="true" />
								{t("ui.publish.new")}
							</Button>
						</>
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
								data-testid="publish-today"
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

				<FilterBar label={t("ui.publish.filters")}>
					<span className="text-sm text-text-muted">
						{t("ui.publish.filter.state")}
					</span>
					<OptionSelect
						size="sm"
						label={t("ui.publish.filter.state")}
						value={state === "" ? ALL : state}
						onChange={(v) => setState(v === ALL ? "" : v)}
						options={[
							{ value: ALL, label: t("ui.publish.filter.all") },
							...TARGET_STATES.map((s) => ({
								value: s,
								label: t(`ui.publish.state.${s}`),
							})),
						]}
					/>
					<span className="text-sm text-text-muted">
						{t("ui.publish.filter.network")}
					</span>
					<OptionSelect
						size="sm"
						label={t("ui.publish.filter.network")}
						value={network === "" ? ALL : network}
						onChange={(v) => setNetwork(v === ALL ? "" : v)}
						options={[
							{ value: ALL, label: t("ui.publish.filter.all") },
							...networks.map((n) => ({ value: n.network, label: n.name })),
						]}
					/>
					<span className="text-xs text-muted-foreground" aria-live="polite">
						{feed.live ? t("ui.publish.live") : ""}
					</span>
				</FilterBar>

				<div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_17rem]">
					<div className="flex min-w-0 flex-col gap-4">
						<FailureNote failure={failure ?? feed.failure ?? view.failure} />
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
								accounts={accountNames}
								onOpen={(id, target) => setEditing({ id, target })}
							/>
						) : mode === "day" ? (
							<DayView
								date={anchor}
								zone={zone}
								plan={view.plan}
								publications={publications}
								accounts={accounts}
								calendarApi={view.api}
								onOpen={(id, target) => setEditing({ id, target })}
								onCreate={(local, net, connection) =>
									setEditing({
										id: null,
										initial: {
											at: local,
											...(net === undefined ? {} : { network: net }),
											...(connection === undefined ? {} : { connection }),
										},
									})
								}
								onMove={move}
								onAdjust={adjust}
								onChanged={reloadAll}
								onFail={view.fail}
							/>
						) : (
							<CalendarView
								view={gridView}
								anchor={anchor}
								weekStart={weekStart}
								zone={zone}
								publications={publications}
								marks={marks}
								accountNames={accountNames}
								onOpen={(id, target) => setEditing({ id, target })}
								onOpenDay={openDay}
								onMove={move}
							/>
						)}
					</div>
					<aside className="flex flex-col gap-4">
						<MiniMonth
							month={mini}
							selected={anchor}
							weekStart={weekStart}
							zone={zone}
							days={miniPlan.plan?.days ?? []}
							onMonth={(direction) =>
								setMini((was) => addMonths(was, direction))
							}
							onPick={openDay}
						/>
						<GuideStrip
							className="[&_ol]:sm:grid-cols-1"
							title={t("ui.publish.guide.title")}
						>
							<NumberedStep
								number={1}
								state={waiting > 0 ? "done" : "current"}
								stateLabel={t("ui.publish.guide.done")}
								title={t("ui.publish.guide.plan")}
								description={t("ui.publish.guide.plan_body")}
							/>
							<NumberedStep
								number={2}
								state={waiting > 0 ? "current" : "todo"}
								stateLabel={t("ui.publish.guide.done")}
								title={t("ui.publish.guide.approve")}
								description={t("ui.publish.guide.approve_body")}
							/>
							<NumberedStep
								number={3}
								state="todo"
								stateLabel={t("ui.publish.guide.done")}
								title={t("ui.publish.guide.out")}
								description={t("ui.publish.guide.out_body")}
							/>
						</GuideStrip>
					</aside>
				</div>
			</div>
			{editing !== null && (editing.id === null || opened !== null) && (
				<PublicationEditor
					key={editing.id ?? `new-${editing.initial?.at ?? ""}`}
					publication={opened}
					initial={editing.initial}
					projects={projects}
					accounts={accounts}
					networks={networks}
					blogs={blogs}
					api={feed.api}
					calendarApi={view.api}
					onClose={() => setEditing(null)}
					onChanged={reloadAll}
					onDuplicated={duplicated}
				/>
			)}
			{slotsOpen && (
				<SlotsDialog
					rules={view.plan?.rules ?? []}
					networks={networks}
					accounts={accounts}
					zone={zone}
					calendarApi={view.api}
					onChanged={reloadAll}
					onClose={() => setSlotsOpen(false)}
				/>
			)}
			{quiet.dialog}
		</>
	);
}
