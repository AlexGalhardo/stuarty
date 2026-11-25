import { Moon } from "lucide-react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { cn } from "@/lib/cn";
import { useLocale, useT } from "@/lib/i18n";
import {
	daysOf,
	onDay,
	placeByDay,
	today,
	type CalendarView as View,
	type Wall,
} from "./calendar";
import type { CalendarDay } from "./dayTypes";
import { actionsOf } from "./publicationState";
import { TargetStateBadge } from "./TargetStateBadge";
import { stateEdge, themeSurface } from "./theme";
import type { Publication } from "./types";
import { useTargetMove } from "./useTargetMove";

interface Entry {
	publication: Publication;
	target: Publication["targets"][number];
}

// The month and the week of the calendar (SDD-020 section 30). A target is a button on the day of its time in the
// zone of the person. A person drags it to another day, or does the same with the keyboard: Space takes it, the
// arrow keys move it by a day (left and right) or an hour (up and down), Enter drops it and Escape lets it go. The
// page sends the new time and the zone; the service decides what it means. The number of a day opens that day
// (epic 97), and a day that holds a theme, a note or the mark "no posts" shows it, as the service answered it.
export function CalendarView({
	view,
	anchor,
	weekStart,
	zone,
	publications,
	marks,
	onOpen,
	onOpenDay,
	onMove,
}: {
	view: Exclude<View, "day">;
	anchor: string;
	weekStart: 0 | 1;
	zone: string;
	publications: readonly Publication[];
	// the days of the range that hold a note, a theme or a mark, by date
	marks?: ReadonlyMap<string, CalendarDay>;
	onOpen(publicationId: string, targetId: string): void;
	onOpenDay?(date: string): void;
	onMove(publicationId: string, targetId: string, local: string): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const { moving, cancel, onKey } = useTargetMove(onMove);
	const entries: Entry[] = publications.flatMap((publication) =>
		publication.targets.map((target) => ({ publication, target })),
	);
	const placed = placeByDay(entries, (e) => e.target.scheduled_at, zone);
	const days = daysOf(view, anchor, weekStart);
	const now = today(zone);
	const dayName = new Intl.DateTimeFormat(locale, {
		weekday: "long",
		day: "numeric",
		month: "long",
		timeZone: "UTC",
	});
	const short = new Intl.DateTimeFormat(locale, {
		weekday: "short",
		timeZone: "UTC",
	});

	function onDrop(day: string, data: string): void {
		const [publicationId, targetId] = data.split("|");
		if (publicationId === undefined || targetId === undefined) return;
		for (const list of placed.values()) {
			const hit = list.find((p) => p.item.target.id === targetId);
			if (hit !== undefined) {
				if (hit.day !== day)
					onMove(publicationId, targetId, onDay(day, hit.wall.local));
				return;
			}
		}
	}

	function chip(item: Entry, wall: Wall): ReactNode {
		const { publication, target } = item;
		const draggable = actionsOf(target.state).edit;
		const picked = moving?.targetId === target.id;
		return (
			<li key={target.id}>
				<button
					type="button"
					draggable={draggable}
					data-testid="target-chip"
					data-target-id={target.id}
					data-state={target.state}
					aria-pressed={draggable ? picked : undefined}
					aria-keyshortcuts={
						draggable
							? "Space ArrowLeft ArrowRight ArrowUp ArrowDown Enter Escape"
							: undefined
					}
					className={cn(
						"flex w-full flex-col items-start gap-1 rounded-md border border-s-4 bg-card px-2 py-1 text-left text-xs hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring",
						stateEdge(target.state),
						picked && "ring-2 ring-ring",
					)}
					onClick={() => onOpen(publication.id, target.id)}
					onKeyDown={(event) => onKey(event, publication.id, target, wall)}
					onBlur={() => picked && cancel()}
					onDragStart={(event) =>
						event.dataTransfer.setData(
							"text/plain",
							`${publication.id}|${target.id}`,
						)
					}
				>
					<span className="font-medium tabular-nums">
						{picked ? moving.local.slice(11) : wall.local.slice(11)}
						<span className="font-normal text-muted-foreground">
							{" "}
							{target.network}
						</span>
					</span>
					<span className="max-w-full truncate">
						<UntrustedText>
							{target.fields.title || publication.title}
						</UntrustedText>
					</span>
					<TargetStateBadge state={target.state} />
				</button>
			</li>
		);
	}

	// The week as a grid of hours (SDD-020 section 30): a row for each hour from the earliest of 08:00 and the first
	// post to the latest of 18:00 and the last. A day is one column and the hours are its rows, aligned across the
	// days by a subgrid.
	function weekGrid(): ReactNode {
		const hoursUsed = [...placed.values()].flatMap((list) =>
			list.map((p) => p.wall.hour),
		);
		const first = Math.min(8, ...hoursUsed);
		const last = Math.max(18, ...hoursUsed);
		const hours = Array.from({ length: last - first + 1 }, (_, i) => first + i);
		const rows = hours.length + 1;
		const pad = (n: number): string => String(n).padStart(2, "0");
		return (
			<div className="overflow-x-auto rounded-lg border">
				<ol
					className="grid min-w-[46rem] grid-cols-[3.5rem_repeat(7,minmax(0,1fr))] gap-px bg-border"
					style={{
						gridTemplateRows: `auto repeat(${hours.length}, minmax(3.25rem, auto))`,
					}}
				>
					<li
						aria-hidden="true"
						className="grid bg-surface-sunken text-xs text-text-subtle"
						style={{ gridRow: `1 / span ${rows}`, gridTemplateRows: "subgrid" }}
					>
						<span />
						{hours.map((h) => (
							<span key={h} className="px-2 pt-1 tabular-nums">
								{pad(h)}:00
							</span>
						))}
					</li>
					{days.map((day) => {
						const list = placed.get(day.key) ?? [];
						const date = new Date(`${day.key}T00:00:00Z`);
						const mark = marks?.get(day.key);
						const label = `${short.format(date)} ${date.getUTCDate()}`;
						return (
							<li
								key={day.key}
								data-testid="calendar-day"
								data-day={day.key}
								data-theme={mark?.theme ?? undefined}
								data-quiet={mark?.quiet === true ? "" : undefined}
								className={cn(
									"grid",
									themeSurface(mark?.theme) || "bg-background",
									day.key === now && "bg-accent-soft",
								)}
								style={{
									gridRow: `1 / span ${rows}`,
									gridTemplateRows: "subgrid",
								}}
								onDragOver={(event) => event.preventDefault()}
								onDrop={(event) => {
									event.preventDefault();
									onDrop(day.key, event.dataTransfer.getData("text/plain"));
								}}
							>
								<h3
									className={cn(
										"flex flex-wrap items-center gap-1 px-2 py-2 text-xs font-medium",
										day.key === now && "text-accent-text",
									)}
									aria-label={dayName.format(date)}
								>
									{onOpenDay === undefined ? (
										label
									) : (
										<button
											type="button"
											className="rounded-sm underline-offset-2 hover:underline focus-visible:outline-2 focus-visible:outline-ring"
											data-testid="calendar-day-open"
											aria-label={t("ui.publish.day.open", {
												day: dayName.format(date),
											})}
											onClick={() => onOpenDay(day.key)}
										>
											{label}
										</button>
									)}
									{mark?.quiet === true && (
										<span
											role="img"
											aria-label={t("ui.publish.quiet.badge")}
											title={t("ui.publish.quiet.badge")}
											data-testid="calendar-day-quiet"
										>
											<Moon aria-hidden="true" className="size-3" />
										</span>
									)}
									{mark?.note ? (
										<span
											className="line-clamp-1 font-normal"
											data-testid="calendar-day-note"
										>
											<UntrustedText>{mark.note}</UntrustedText>
										</span>
									) : null}
								</h3>
								{hours.map((h) => (
									<ul
										key={h}
										className="flex min-w-0 flex-col gap-1 border-t border-border px-1 py-1"
									>
										{list
											.filter((p) => p.wall.hour === h)
											.map(({ item, wall }) => chip(item, wall))}
									</ul>
								))}
							</li>
						);
					})}
				</ol>
			</div>
		);
	}

	return (
		<div data-testid={`calendar-${view}`}>
			<div aria-live="polite" role="status" className="sr-only">
				{moving === null
					? ""
					: t("ui.publish.moving", { time: moving.local.replace("T", " ") })}
			</div>
			{view === "month" && (
				<div
					aria-hidden="true"
					className="hidden grid-cols-7 rounded-t-lg border border-b-0 bg-surface-sunken px-2 py-2 text-sm font-medium text-text-muted sm:grid"
				>
					{days.slice(0, 7).map((day) => (
						<span key={day.key}>
							{short.format(new Date(`${day.key}T00:00:00Z`))}
						</span>
					))}
				</div>
			)}
			{view === "week" ? (
				weekGrid()
			) : (
				<ol
					className={cn(
						"grid grid-cols-1 gap-px overflow-hidden rounded-lg border bg-border sm:grid-cols-7",
						view === "month" && "sm:rounded-t-none",
					)}
				>
					{days.map((day) => {
						const list = placed.get(day.key) ?? [];
						const date = new Date(`${day.key}T00:00:00Z`);
						const mark = marks?.get(day.key);
						const label = (
							<>
								<span className="sm:hidden">{dayName.format(date)}</span>
								<span className="hidden sm:inline">{date.getUTCDate()}</span>
							</>
						);
						return (
							<li
								key={day.key}
								data-testid="calendar-day"
								data-day={day.key}
								data-theme={mark?.theme ?? undefined}
								data-quiet={mark?.quiet === true ? "" : undefined}
								className={cn(
									"min-h-24 p-2",
									themeSurface(mark?.theme) || "bg-background",
									!day.inMonth && "bg-muted/40 text-muted-foreground",
									day.key === now && "bg-accent-soft",
								)}
								onDragOver={(event) => event.preventDefault()}
								onDrop={(event) => {
									event.preventDefault();
									onDrop(day.key, event.dataTransfer.getData("text/plain"));
								}}
							>
								<h3
									className={cn(
										"mb-1 flex items-center gap-1 text-xs font-medium",
										day.key === now && "text-accent-text",
									)}
									aria-label={dayName.format(date)}
								>
									{onOpenDay === undefined ? (
										label
									) : (
										<button
											type="button"
											className="rounded-sm underline-offset-2 hover:underline focus-visible:outline-2 focus-visible:outline-ring"
											data-testid="calendar-day-open"
											aria-label={t("ui.publish.day.open", {
												day: dayName.format(date),
											})}
											onClick={() => onOpenDay(day.key)}
										>
											{label}
										</button>
									)}
									{mark?.quiet === true && (
										<span
											role="img"
											aria-label={t("ui.publish.quiet.badge")}
											title={t("ui.publish.quiet.badge")}
											data-testid="calendar-day-quiet"
										>
											<Moon aria-hidden="true" className="size-3" />
										</span>
									)}
								</h3>
								<ul className="flex flex-col gap-1">
									{list.map(({ item, wall }) => chip(item, wall))}
								</ul>
							</li>
						);
					})}
				</ol>
			)}
		</div>
	);
}
