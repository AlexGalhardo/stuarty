import { Moon, Plus } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";

import { FieldLabel } from "@/components/FieldHelp";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/cn";
import { useLocale, useT } from "@/lib/i18n";
import type { SocialAccount } from "../../editor/api/types";
import { placeByDay, type Wall, wallOf } from "./calendar";
import type { CalendarApi } from "./calendarApi";
import {
	type CalendarPlan,
	isTheme,
	type SlotOccurrence,
	THEMES,
} from "./dayTypes";
import { actionsOf } from "./publicationState";
import { TargetStateBadge } from "./TargetStateBadge";
import { THEME_CLASSES, themeSurface } from "./theme";
import type { Publication, Target } from "./types";
import { useTargetMove } from "./useTargetMove";

export interface Adjustment {
	// the new wall time as text, read by the service in the zone of the target
	at?: string;
	connection_id?: string;
	description?: string;
}

interface Entry {
	publication: Publication;
	target: Target;
}

const HOURS = Array.from({ length: 24 }, (_, hour) => hour);

function pad(n: number): string {
	return String(n).padStart(2, "0");
}

// One day of the calendar (SDD-020 section 30A.6, epic 97): its hours as a column, the posts at their hours, the weekly
// slots that nothing fills, a note, a theme and the mark "no posts". A click on an empty hour opens the editor with that
// date and time filled in; a post can be dragged to another hour, moved with the keyboard as in the week, or changed in
// place (its time, its text and its account). It draws what the service answers and sends what the person does; which
// day is quiet and which post fills a slot are the service's.
export function DayView({
	date,
	zone,
	plan,
	publications,
	accounts,
	calendarApi,
	onOpen,
	onCreate,
	onMove,
	onAdjust,
	onChanged,
	onFail,
}: {
	date: string;
	zone: string;
	plan: CalendarPlan | null;
	publications: readonly Publication[];
	accounts: readonly SocialAccount[];
	calendarApi: CalendarApi;
	onOpen(publicationId: string, targetId: string): void;
	onCreate(local: string, network?: string, connectionId?: string): void;
	onMove(publicationId: string, targetId: string, local: string): void;
	// true when the service applied the change: the form closes only then, so a refusal stays where the person can fix it
	onAdjust(
		publicationId: string,
		targetId: string,
		change: Adjustment,
	): Promise<boolean>;
	onChanged(): void;
	onFail(error: unknown): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const { moving, cancel, onKey } = useTargetMove(onMove);
	const [adjusting, setAdjusting] = useState<string | null>(null);
	const entries: Entry[] = publications.flatMap((publication) =>
		publication.targets.map((target) => ({ publication, target })),
	);
	const placed = placeByDay(entries, (e) => e.target.scheduled_at, zone).get(
		date,
	);
	const empties = (plan?.slots ?? []).flatMap((slot) => {
		const wall = wallOf(slot.at, zone);
		return wall !== null && wall.day === date && slot.filled === undefined
			? [{ slot, wall }]
			: [];
	});
	const day = plan?.days.find((d) => d.date === date);
	const title = new Intl.DateTimeFormat(locale, {
		weekday: "long",
		day: "numeric",
		month: "long",
		year: "numeric",
		timeZone: "UTC",
	}).format(new Date(`${date}T00:00:00Z`));

	function dropOn(hour: number, data: string): void {
		const [publicationId, targetId] = data.split("|");
		const hit = placed?.find((p) => p.item.target.id === targetId);
		if (
			publicationId === undefined ||
			targetId === undefined ||
			hit === undefined
		)
			return;
		if (hit.wall.hour !== hour) {
			onMove(
				publicationId,
				targetId,
				`${date}T${pad(hour)}:${pad(hit.wall.minute)}`,
			);
		}
	}

	return (
		<div
			className={cn("grid gap-4 lg:grid-cols-[minmax(0,1fr)_18rem]")}
			data-testid="calendar-day-view"
			data-date={date}
		>
			<section aria-labelledby="day-heading" className="min-w-0">
				<div
					className={cn(
						"mb-3 flex flex-wrap items-center gap-2 rounded-lg border p-3",
						themeSurface(day?.theme),
					)}
					data-testid="day-header"
					data-theme={day?.theme ?? ""}
				>
					<h2 id="day-heading" className="text-lg font-medium">
						{title}
					</h2>
					{isTheme(day?.theme) && (
						<span className="text-xs font-medium">
							{t(`ui.publish.day.theme_${day.theme}`)}
						</span>
					)}
					{day?.quiet === true && (
						<span
							className="inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium"
							data-testid="day-quiet-badge"
						>
							<Moon aria-hidden="true" className="size-3" />
							{t("ui.publish.quiet.badge")}
						</span>
					)}
				</div>
				<div aria-live="polite" role="status" className="sr-only">
					{moving === null
						? ""
						: t("ui.publish.moving", { time: moving.local.replace("T", " ") })}
				</div>
				<ol
					className="flex flex-col divide-y rounded-lg border"
					data-testid="day-hours"
				>
					{HOURS.map((hour) => {
						const here = (placed ?? []).filter((p) => p.wall.hour === hour);
						const slots = empties.filter((e) => e.wall.hour === hour);
						const local = `${date}T${pad(hour)}:00`;
						return (
							<li
								key={hour}
								data-testid="day-hour"
								data-hour={hour}
								className="flex min-h-12 items-start gap-3 px-3 py-1.5"
								onDragOver={(event) => event.preventDefault()}
								onDrop={(event) => {
									event.preventDefault();
									dropOn(hour, event.dataTransfer.getData("text/plain"));
								}}
							>
								<span className="w-12 shrink-0 pt-1 text-xs tabular-nums text-muted-foreground">
									{pad(hour)}:00
								</span>
								<div className="flex min-w-0 flex-1 flex-col gap-1">
									{here.map(({ item, wall }) => (
										<DayChip
											key={item.target.id}
											entry={item}
											wall={wall}
											picked={moving?.targetId === item.target.id}
											pickedLocal={moving?.local}
											open={adjusting === item.target.id}
											accounts={accounts}
											onToggle={() =>
												setAdjusting((was) =>
													was === item.target.id ? null : item.target.id,
												)
											}
											onOpen={() => onOpen(item.publication.id, item.target.id)}
											onKey={(event) =>
												onKey(event, item.publication.id, item.target, wall)
											}
											onBlur={() =>
												moving?.targetId === item.target.id && cancel()
											}
											onAdjust={(change) =>
												onAdjust(
													item.publication.id,
													item.target.id,
													change,
												).then((applied) => {
													if (applied) setAdjusting(null);
												})
											}
										/>
									))}
									{slots.map(({ slot, wall }) => (
										<SlotButton
											key={`${slot.slot_id}-${slot.at}`}
											slot={slot}
											wall={wall}
											onClick={() =>
												onCreate(wall.local, slot.network, slot.connection_id)
											}
										/>
									))}
									{here.length === 0 && slots.length === 0 ? (
										<button
											type="button"
											className="w-full rounded-md px-2 py-1 text-left text-xs text-muted-foreground hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
											data-testid={`day-hour-add-${hour}`}
											onClick={() => onCreate(local)}
										>
											<span className="sr-only">
												{t("ui.publish.day.add_at", {
													time: `${pad(hour)}:00`,
												})}
											</span>
											<Plus aria-hidden="true" className="inline size-3" />
										</button>
									) : (
										<Button
											type="button"
											variant="ghost"
											size="icon-sm"
											className="self-start"
											aria-label={t("ui.publish.day.add_at", {
												time: `${pad(hour)}:00`,
											})}
											data-testid={`day-hour-add-${hour}`}
											onClick={() => onCreate(local)}
										>
											<Plus aria-hidden="true" />
										</Button>
									)}
								</div>
							</li>
						);
					})}
				</ol>
			</section>
			<DayPlan
				date={date}
				zone={zone}
				note={day?.note ?? ""}
				theme={day?.theme ?? ""}
				quiet={day?.quiet === true}
				calendarApi={calendarApi}
				onChanged={onChanged}
				onFail={onFail}
			/>
		</div>
	);
}

function SlotButton({
	slot,
	wall,
	onClick,
}: {
	slot: SlotOccurrence;
	wall: Wall;
	onClick(): void;
}): ReactNode {
	const t = useT();
	return (
		<button
			type="button"
			className="flex w-full items-center gap-2 rounded-md border border-dashed px-2 py-1 text-left text-xs hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
			data-testid="day-slot-empty"
			data-slot-id={slot.slot_id}
			onClick={onClick}
		>
			<span className="font-medium tabular-nums">{wall.local.slice(11)}</span>
			<span>{slot.network}</span>
			<span className="text-muted-foreground">
				{t("ui.publish.day.slot_empty")}
			</span>
		</button>
	);
}

function DayChip({
	entry,
	wall,
	picked,
	pickedLocal,
	open,
	accounts,
	onToggle,
	onOpen,
	onKey,
	onBlur,
	onAdjust,
}: {
	entry: Entry;
	wall: Wall;
	picked: boolean;
	pickedLocal: string | undefined;
	open: boolean;
	accounts: readonly SocialAccount[];
	onToggle(): void;
	onOpen(): void;
	onKey(event: React.KeyboardEvent<HTMLButtonElement>): void;
	onBlur(): void;
	onAdjust(change: Adjustment): Promise<void>;
}): ReactNode {
	const t = useT();
	const { publication, target } = entry;
	const editable = actionsOf(target.state).edit;
	return (
		<div className="flex flex-col gap-1">
			<div className="flex items-start gap-1">
				<button
					type="button"
					draggable={editable}
					data-testid="target-chip"
					data-target-id={target.id}
					data-state={target.state}
					aria-pressed={editable ? picked : undefined}
					aria-keyshortcuts={
						editable
							? "Space ArrowLeft ArrowRight ArrowUp ArrowDown Enter Escape"
							: undefined
					}
					className={cn(
						"flex min-w-0 flex-1 flex-col items-start gap-1 rounded-md border bg-card px-2 py-1 text-left text-xs hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring",
						picked && "ring-2 ring-ring",
					)}
					onClick={onOpen}
					onKeyDown={onKey}
					onBlur={onBlur}
					onDragStart={(event) =>
						event.dataTransfer.setData(
							"text/plain",
							`${publication.id}|${target.id}`,
						)
					}
				>
					<span className="font-medium tabular-nums">
						{picked && pickedLocal !== undefined
							? pickedLocal.slice(11)
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
				{editable && (
					<Button
						type="button"
						variant="outline"
						size="sm"
						aria-expanded={open}
						data-testid="day-adjust"
						onClick={onToggle}
					>
						{t("ui.publish.day.adjust")}
					</Button>
				)}
			</div>
			{open && editable && (
				<QuickEdit
					entry={entry}
					wall={wall}
					accounts={accounts}
					onSave={onAdjust}
				/>
			)}
		</div>
	);
}

// The three things of a post that a day changes in place (SDD-020 section 30A.6): its time, its text and its account.
// The change is sent as the person typed it and the day shows what the service answers.
function QuickEdit({
	entry,
	wall,
	accounts,
	onSave,
}: {
	entry: Entry;
	wall: Wall;
	accounts: readonly SocialAccount[];
	onSave(change: Adjustment): Promise<void>;
}): ReactNode {
	const t = useT();
	const { publication, target } = entry;
	const [time, setTime] = useState(wall.local.slice(11));
	const [text, setText] = useState(target.fields.description ?? "");
	const [account, setAccount] = useState(target.connection_id);
	const [busy, setBusy] = useState(false);
	const id = `adjust-${target.id}`;
	const changed =
		time !== wall.local.slice(11) ||
		text !== (target.fields.description ?? "") ||
		account !== target.connection_id;

	async function save(): Promise<void> {
		setBusy(true);
		try {
			await onSave({
				...(time !== wall.local.slice(11) ? { at: `${wall.day}T${time}` } : {}),
				...(account !== target.connection_id ? { connection_id: account } : {}),
				...(text !== (target.fields.description ?? "")
					? { description: text }
					: {}),
			});
		} finally {
			setBusy(false);
		}
	}

	return (
		<form
			className="flex flex-col gap-2 rounded-md border bg-muted/40 p-2 text-xs"
			data-testid="day-quick-edit"
			onSubmit={(event) => {
				event.preventDefault();
				void save();
			}}
		>
			<div className="grid gap-2 sm:grid-cols-2">
				<div className="flex flex-col gap-1">
					<FieldLabel
						htmlFor={`${id}-time`}
						label={t("ui.publish.day.time")}
						help={t("ui.help.g2.day_time_help")}
					/>
					<Input
						id={`${id}-time`}
						type="time"
						required
						value={time}
						onChange={(event) => setTime(event.target.value)}
					/>
				</div>
				<div className="flex flex-col gap-1">
					<Label htmlFor={`${id}-account`}>
						{t("ui.publish.editor.account")}
					</Label>
					<select
						id={`${id}-account`}
						className="h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2 text-sm"
						value={account}
						onChange={(event) => setAccount(event.target.value)}
					>
						{!accounts.some(
							(a) => a.connection_id === target.connection_id,
						) && <option value={target.connection_id}>{target.network}</option>}
						{accounts.map((a) => (
							<option key={a.connection_id} value={a.connection_id}>
								{a.label} ({a.network})
							</option>
						))}
					</select>
				</div>
			</div>
			<div className="flex flex-col gap-1">
				<FieldLabel
					htmlFor={`${id}-text`}
					label={t("ui.publish.day.text")}
					help={t("ui.help.g2.day_text_help")}
				/>
				<Textarea
					id={`${id}-text`}
					value={text}
					placeholder={publication.text || t("ui.help.g2.pub_caption_ph")}
					onChange={(event) => setText(event.target.value)}
				/>
			</div>
			<div>
				<Button
					type="submit"
					size="sm"
					disabled={busy || !changed}
					data-testid="day-adjust-save"
				>
					{t("ui.publish.day.adjust_save")}
				</Button>
			</div>
		</form>
	);
}

// The note, the theme and the mark of the day (SDD-020 section 30A.1). The note is saved when the field is left; the
// theme and the mark when they are chosen. Marking a day is a person's, and the service refuses anyone else.
function DayPlan({
	date,
	zone,
	note,
	theme,
	quiet,
	calendarApi,
	onChanged,
	onFail,
}: {
	date: string;
	zone: string;
	note: string;
	theme: string;
	quiet: boolean;
	calendarApi: CalendarApi;
	onChanged(): void;
	onFail(error: unknown): void;
}): ReactNode {
	const t = useT();
	const [text, setText] = useState(note);
	// The service's answer is the truth: a new date, or a note it holds now, replaces what was typed.
	// biome-ignore lint/correctness/useExhaustiveDependencies: date and note are the keys of the text
	useEffect(() => setText(note), [date, note]);

	function saved(promise: Promise<unknown>): void {
		promise.then(onChanged).catch(onFail);
	}

	return (
		<aside
			aria-label={t("ui.publish.day.plan")}
			className="flex flex-col gap-4 rounded-lg border p-3"
			data-testid="day-plan"
		>
			<div className="flex flex-col gap-1">
				<FieldLabel
					htmlFor="day-note"
					label={t("ui.publish.day.note")}
					help={t("ui.help.g2.day_note_help")}
				/>
				<Textarea
					id="day-note"
					placeholder={t("ui.help.g2.day_note_ph")}
					maxLength={2000}
					rows={5}
					value={text}
					data-testid="day-note"
					onChange={(event) => setText(event.target.value)}
					onBlur={() => {
						if (text !== note)
							saved(calendarApi.setDay(date, { zone, note: text }));
					}}
				/>
				<p className="text-xs text-muted-foreground">
					{t("ui.publish.day.note_help")}
				</p>
			</div>
			<fieldset className="flex flex-col gap-2">
				<legend className="text-sm font-medium">
					{t("ui.publish.day.theme")}
				</legend>
				<div className="flex flex-wrap gap-2">
					<label className="flex items-center gap-1 text-xs">
						<input
							type="radio"
							name="day-theme"
							className="accent-primary"
							checked={theme === ""}
							data-testid="day-theme-none"
							onChange={() =>
								saved(calendarApi.setDay(date, { zone, theme: "" }))
							}
						/>
						{t("ui.publish.day.theme_none")}
					</label>
					{THEMES.map((name) => (
						<label
							key={name}
							className={cn(
								"flex items-center gap-1 rounded-md px-1.5 py-0.5 text-xs",
								THEME_CLASSES[name].surface,
								theme === name && cn("ring-2", THEME_CLASSES[name].ring),
							)}
						>
							<input
								type="radio"
								name="day-theme"
								className="accent-primary"
								checked={theme === name}
								data-testid={`day-theme-${name}`}
								onChange={() =>
									saved(calendarApi.setDay(date, { zone, theme: name }))
								}
							/>
							{t(`ui.publish.day.theme_${name}`)}
						</label>
					))}
				</div>
			</fieldset>
			<div className="flex flex-col gap-1">
				<label className="flex items-center gap-2 text-sm">
					<input
						type="checkbox"
						role="switch"
						aria-checked={quiet}
						className="accent-primary"
						checked={quiet}
						aria-describedby="day-quiet-help"
						data-testid="day-quiet"
						onChange={(event) =>
							saved(calendarApi.setQuiet(date, zone, event.target.checked))
						}
					/>
					{t("ui.publish.quiet.switch")}
				</label>
				<p id="day-quiet-help" className="text-xs text-muted-foreground">
					{t("ui.publish.quiet.help")}
				</p>
			</div>
		</aside>
	);
}
