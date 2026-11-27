import { ChevronLeft, ChevronRight } from "lucide-react";
import { type ReactNode, useState } from "react";

import { FieldLabel } from "@/components/FieldHelp";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/cn";
import { useLocale, useT } from "@/lib/i18n";
import { monthDays, today } from "./calendar";
import type { CalendarDay } from "./dayTypes";
import { themeSurface } from "./theme";

// A small month for the page header (SDD-020 section 30A.6): today is marked, the days that hold a note, a theme or a
// mark of "no posts" carry a sign, a click opens that day, and "Go to date" takes any date. It lays out dates; what a
// day holds is read from the plan.
export function MiniMonth({
	month,
	selected,
	weekStart,
	zone,
	days,
	onMonth,
	onPick,
}: {
	// any date of the month shown, YYYY-MM-DD
	month: string;
	// the date the page is on
	selected: string;
	weekStart: 0 | 1;
	zone: string;
	days: readonly CalendarDay[];
	onMonth(delta: -1 | 1): void;
	onPick(date: string): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const [go, setGo] = useState("");
	const now = today(zone);
	const held = new Map(days.map((d) => [d.date, d]));
	const heading = new Intl.DateTimeFormat(locale, {
		month: "long",
		year: "numeric",
		timeZone: "UTC",
	}).format(new Date(`${month}T00:00:00Z`));
	const weekday = new Intl.DateTimeFormat(locale, {
		weekday: "narrow",
		timeZone: "UTC",
	});
	const long = new Intl.DateTimeFormat(locale, {
		weekday: "long",
		day: "numeric",
		month: "long",
		timeZone: "UTC",
	});
	const grid = monthDays(month, weekStart);

	return (
		<section
			aria-label={t("ui.publish.mini.label")}
			className="w-56 rounded-lg border p-2 text-xs"
			data-testid="mini-month"
		>
			<div className="mb-1 flex items-center justify-between gap-1">
				<Button
					type="button"
					variant="ghost"
					size="icon-sm"
					aria-label={t("ui.publish.mini.previous")}
					onClick={() => onMonth(-1)}
				>
					<ChevronLeft aria-hidden="true" />
				</Button>
				<h3 className="font-medium" data-testid="mini-heading">
					{heading}
				</h3>
				<Button
					type="button"
					variant="ghost"
					size="icon-sm"
					aria-label={t("ui.publish.mini.next")}
					onClick={() => onMonth(1)}
				>
					<ChevronRight aria-hidden="true" />
				</Button>
			</div>
			<div
				className="grid grid-cols-7 text-center text-muted-foreground"
				aria-hidden="true"
			>
				{grid.slice(0, 7).map((d) => (
					<span key={d.key}>
						{weekday.format(new Date(`${d.key}T00:00:00Z`))}
					</span>
				))}
			</div>
			<div className="grid grid-cols-7 gap-px">
				{grid.map((d) => {
					const day = held.get(d.key);
					const date = new Date(`${d.key}T00:00:00Z`);
					const signs = [
						day?.quiet === true ? t("ui.publish.quiet.badge") : "",
						day?.note ? t("ui.publish.mini.has_note") : "",
					].filter((s) => s !== "");
					return (
						<button
							key={d.key}
							type="button"
							data-testid="mini-day"
							data-day={d.key}
							aria-label={[long.format(date), ...signs].join(", ")}
							aria-current={d.key === now ? "date" : undefined}
							aria-pressed={d.key === selected}
							className={cn(
								"relative h-7 rounded-md tabular-nums hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring",
								!d.inMonth && "text-muted-foreground",
								themeSurface(day?.theme),
								d.key === now && "font-semibold underline underline-offset-2",
								d.key === selected && "ring-2 ring-ring",
							)}
							onClick={() => onPick(d.key)}
						>
							{date.getUTCDate()}
							{(day?.quiet === true || day?.note) && (
								<span
									aria-hidden="true"
									className="absolute right-0.5 bottom-0.5 size-1 rounded-full bg-current"
								/>
							)}
						</button>
					);
				})}
			</div>
			<form
				className="mt-2 flex items-end gap-1"
				onSubmit={(event) => {
					event.preventDefault();
					if (go !== "") onPick(go);
				}}
			>
				<div className="flex min-w-0 flex-1 flex-col gap-1">
					<FieldLabel
						htmlFor="mini-go"
						label={t("ui.publish.mini.go_label")}
						help={t("ui.help.g2.mini_go_help")}
					/>
					<Input
						id="mini-go"
						type="date"
						value={go}
						data-testid="mini-go"
						onChange={(event) => setGo(event.target.value)}
					/>
				</div>
				<Button
					type="submit"
					size="sm"
					disabled={go === ""}
					data-testid="mini-go-submit"
				>
					{t("ui.publish.mini.go")}
				</Button>
			</form>
		</section>
	);
}
