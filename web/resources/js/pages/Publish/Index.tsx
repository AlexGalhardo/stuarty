import { Head } from "@inertiajs/react";
import { ChevronLeft, ChevronRight, Plus } from "lucide-react";
import { type ReactNode, useEffect, useMemo, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { Button } from "@/components/ui/button";
import { useLocale, useT } from "@/lib/i18n";
import type { ProjectSummary } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { SocialAccount, SocialNetwork } from "../../editor/api/types";
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

function api() {
	return createClient(new HttpTransport("/api/v1"));
}

// The publishing calendar (ADR-0046, SDD-020 section 22): which piece goes to which account and when, in a month,
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
	const [editing, setEditing] = useState<{
		id: string | null;
		target?: string;
	} | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	// Sunday in the languages that start the week there, Monday in the rest.
	const weekStart = locale.startsWith("en") ? 0 : 1;

	const filter: ListFilter = useMemo(() => {
		const base: ListFilter = {
			...(state === "" ? {} : { state }),
			...(network === "" ? {} : { network }),
			limit: 500,
		};
		if (mode === "list") return base;
		return { ...base, ...queryRange(daysOf(mode, anchor, weekStart)) };
	}, [mode, anchor, weekStart, state, network]);

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
	}, []);

	const publications = feed.publications ?? [];
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
				<div className="flex flex-wrap items-start justify-between gap-3">
					<div>
						<h1 className="text-2xl font-semibold">{t("ui.publish.title")}</h1>
						<p className="mt-1 text-sm text-muted-foreground">
							{t("ui.publish.intro")}
						</p>
					</div>
					<Button
						type="button"
						onClick={() => setEditing({ id: null })}
						data-testid="publish-new"
					>
						<Plus aria-hidden="true" />
						{t("ui.publish.new")}
					</Button>
				</div>

				<div className="flex flex-wrap items-center gap-3">
					<fieldset className="inline-flex overflow-hidden rounded-lg border">
						<legend className="sr-only">{t("ui.publish.view")}</legend>
						{MODES.map((m) => (
							<Button
								key={m}
								type="button"
								variant={mode === m ? "default" : "ghost"}
								size="sm"
								aria-pressed={mode === m}
								onClick={() => setMode(m)}
								data-testid={`publish-mode-${m}`}
							>
								{t(`ui.publish.view_${m}`)}
							</Button>
						))}
					</fieldset>
					{mode !== "list" && (
						<div className="flex items-center gap-1">
							<Button
								type="button"
								variant="outline"
								size="icon-sm"
								aria-label={t("ui.publish.previous")}
								onClick={() => step(-1)}
							>
								<ChevronLeft aria-hidden="true" />
							</Button>
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={() => setAnchor(today(zone))}
							>
								{t("ui.publish.today")}
							</Button>
							<Button
								type="button"
								variant="outline"
								size="icon-sm"
								aria-label={t("ui.publish.next")}
								onClick={() => step(1)}
							>
								<ChevronRight aria-hidden="true" />
							</Button>
							<h2
								className="ml-2 text-lg font-medium"
								data-testid="publish-range"
							>
								{title}
							</h2>
						</div>
					)}
					<div className="ml-auto flex flex-wrap items-center gap-2 text-sm">
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
				</div>

				<FailureNote failure={failure ?? feed.failure} />

				{mode === "list" ? (
					<ListView
						publications={publications}
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
					api={feed.api}
					onClose={() => setEditing(null)}
					onChanged={() => void feed.reload()}
				/>
			)}
		</>
	);
}
