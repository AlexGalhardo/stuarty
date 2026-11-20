import { type KeyboardEvent, type ReactNode, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { cn } from "@/lib/cn";
import { useLocale, useT } from "@/lib/i18n";
import {
	daysOf,
	onDay,
	placeByDay,
	shiftLocal,
	today,
	type CalendarView as View,
	type Wall,
} from "./calendar";
import { actionsOf } from "./publicationState";
import { TargetStateBadge } from "./TargetStateBadge";
import type { Publication, Target } from "./types";

interface Moving {
	targetId: string;
	local: string;
}

interface Entry {
	publication: Publication;
	target: Target;
}

// The month and the week of the calendar (SDD-020 section 30). A target is a button on the day of its time in the
// zone of the person. A person drags it to another day, or does the same with the keyboard: Space takes it, the
// arrow keys move it by a day (left and right) or an hour (up and down), Enter drops it and Escape lets it go. The
// page sends the new time and the zone; the service decides what it means.
export function CalendarView({
	view,
	anchor,
	weekStart,
	zone,
	publications,
	onOpen,
	onMove,
}: {
	view: View;
	anchor: string;
	weekStart: 0 | 1;
	zone: string;
	publications: readonly Publication[];
	onOpen(publicationId: string, targetId: string): void;
	onMove(publicationId: string, targetId: string, local: string): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const [moving, setMoving] = useState<Moving | null>(null);
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

	function onKey(
		event: KeyboardEvent<HTMLButtonElement>,
		entry: Entry,
		wall: Wall,
	): void {
		const { target } = entry;
		if (!actionsOf(target.state).edit) return;
		if (moving === null || moving.targetId !== target.id) {
			if (event.key === " ") {
				event.preventDefault();
				setMoving({ targetId: target.id, local: wall.local });
			}
			return;
		}
		const step: Record<string, [number, number]> = {
			ArrowLeft: [-1, 0],
			ArrowRight: [1, 0],
			ArrowUp: [0, -1],
			ArrowDown: [0, 1],
		};
		const delta = step[event.key];
		if (delta !== undefined) {
			event.preventDefault();
			setMoving({
				...moving,
				local: shiftLocal(moving.local, delta[0], delta[1]),
			});
		} else if (event.key === "Enter" || event.key === " ") {
			event.preventDefault();
			setMoving(null);
			if (moving.local !== wall.local) {
				onMove(entry.publication.id, target.id, moving.local);
			}
		} else if (event.key === "Escape") {
			event.preventDefault();
			setMoving(null);
		}
	}

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

	return (
		<div data-testid={`calendar-${view}`}>
			<div aria-live="polite" role="status" className="sr-only">
				{moving === null
					? ""
					: t("ui.publish.moving", { time: moving.local.replace("T", " ") })}
			</div>
			<ol
				className={cn(
					"grid grid-cols-1 gap-px overflow-hidden rounded-lg border bg-border sm:grid-cols-7",
				)}
			>
				{days.map((day) => {
					const list = placed.get(day.key) ?? [];
					const date = new Date(`${day.key}T00:00:00Z`);
					return (
						<li
							key={day.key}
							data-testid="calendar-day"
							data-day={day.key}
							className={cn(
								"min-h-24 bg-background p-2",
								!day.inMonth && "bg-muted/40 text-muted-foreground",
								view === "week" && "min-h-64",
							)}
							onDragOver={(event) => event.preventDefault()}
							onDrop={(event) => {
								event.preventDefault();
								onDrop(day.key, event.dataTransfer.getData("text/plain"));
							}}
						>
							<h3
								className={cn(
									"mb-1 text-xs font-medium",
									day.key === now && "text-primary",
								)}
								aria-label={dayName.format(date)}
							>
								<span className="sm:hidden">{dayName.format(date)}</span>
								<span className="hidden sm:inline">
									{view === "week" ? `${short.format(date)} ` : ""}
									{date.getUTCDate()}
								</span>
							</h3>
							<ul className="flex flex-col gap-1">
								{list.map(({ item, wall }) => {
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
													"flex w-full flex-col items-start gap-1 rounded-md border bg-card px-2 py-1 text-left text-xs hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring",
													picked && "ring-2 ring-ring",
												)}
												onClick={() => onOpen(publication.id, target.id)}
												onKeyDown={(event) => onKey(event, item, wall)}
												onBlur={() => picked && setMoving(null)}
												onDragStart={(event) =>
													event.dataTransfer.setData(
														"text/plain",
														`${publication.id}|${target.id}`,
													)
												}
											>
												<span className="font-medium tabular-nums">
													{picked
														? moving.local.slice(11)
														: wall.local.slice(11)}
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
								})}
							</ul>
						</li>
					);
				})}
			</ol>
		</div>
	);
}
