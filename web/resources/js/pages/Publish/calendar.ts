// The layout of the calendar: which day a time falls on and which days a month or a week shows. It draws
// times; it decides nothing about them: the service owns every rule of a time, a zone and a missed slot.

export type CalendarView = "month" | "week";

// A day of the grid, named by its date in the zone of the person: YYYY-MM-DD.
export interface Day {
	key: string;
	inMonth: boolean;
}

export function browserZone(): string {
	return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

function pad(n: number, width = 2): string {
	return String(n).padStart(width, "0");
}

// The wall clock of an instant in a zone.
export interface Wall {
	day: string;
	// YYYY-MM-DDTHH:mm, the form the service reads as a local time
	local: string;
	hour: number;
	minute: number;
}

export function wallOf(iso: string, zone: string): Wall | null {
	const date = new Date(iso);
	if (Number.isNaN(date.getTime())) return null;
	let parts: Intl.DateTimeFormatPart[];
	try {
		parts = new Intl.DateTimeFormat("en-CA", {
			timeZone: zone,
			year: "numeric",
			month: "2-digit",
			day: "2-digit",
			hour: "2-digit",
			minute: "2-digit",
			hourCycle: "h23",
		}).formatToParts(date);
	} catch {
		return null;
	}
	const get = (type: string): string =>
		parts.find((p) => p.type === type)?.value ?? "00";
	const day = `${get("year")}-${get("month")}-${get("day")}`;
	return {
		day,
		local: `${day}T${get("hour")}:${get("minute")}`,
		hour: Number(get("hour")),
		minute: Number(get("minute")),
	};
}

function parseDay(key: string): Date {
	const [y, m, d] = key.split("-").map(Number);
	return new Date(Date.UTC(y ?? 1970, (m ?? 1) - 1, d ?? 1));
}

function keyOf(date: Date): string {
	return `${pad(date.getUTCFullYear(), 4)}-${pad(date.getUTCMonth() + 1)}-${pad(date.getUTCDate())}`;
}

export function addDays(key: string, n: number): string {
	const date = parseDay(key);
	date.setUTCDate(date.getUTCDate() + n);
	return keyOf(date);
}

export function addMonths(key: string, n: number): string {
	const date = parseDay(key);
	date.setUTCDate(1);
	date.setUTCMonth(date.getUTCMonth() + n);
	return keyOf(date);
}

// The wall time of a time that moves by days and hours: the date of the wall clock moves and the clock follows.
// The service reads the result in the zone and says what a change of the clock did to it.
export function shiftLocal(local: string, days: number, hours: number): string {
	const [datePart = "", timePart = "00:00"] = local.split("T");
	const [h = 0, m = 0] = timePart.split(":").map(Number);
	const date = parseDay(datePart);
	date.setUTCHours(h + hours, m, 0, 0);
	date.setUTCDate(date.getUTCDate() + days);
	return `${keyOf(date)}T${pad(date.getUTCHours())}:${pad(date.getUTCMinutes())}`;
}

// The local time of a day at the clock of another time: dropping a target on a day keeps its hour.
export function onDay(day: string, local: string): string {
	const time = local.split("T")[1] ?? "09:00";
	return `${day}T${time}`;
}

function weekStartOf(key: string, weekStart: 0 | 1): string {
	const date = parseDay(key);
	const back = (date.getUTCDay() - weekStart + 7) % 7;
	return addDays(key, -back);
}

export function weekDays(anchor: string, weekStart: 0 | 1): Day[] {
	const first = weekStartOf(anchor, weekStart);
	return Array.from({ length: 7 }, (_, i) => ({
		key: addDays(first, i),
		inMonth: true,
	}));
}

// Six weeks, so the grid does not change height from one month to the next.
export function monthDays(anchor: string, weekStart: 0 | 1): Day[] {
	const month = anchor.slice(0, 7);
	const first = weekStartOf(`${month}-01`, weekStart);
	return Array.from({ length: 42 }, (_, i) => {
		const key = addDays(first, i);
		return { key, inMonth: key.startsWith(month) };
	});
}

export function daysOf(
	view: CalendarView,
	anchor: string,
	weekStart: 0 | 1,
): Day[] {
	return view === "month"
		? monthDays(anchor, weekStart)
		: weekDays(anchor, weekStart);
}

// The range to ask the service for: the days of the grid in the zone of the person, widened by a day on each
// side, since a time near midnight falls on another day in another zone. The page keeps the targets whose day,
// in the zone of the person, is on the grid.
export function queryRange(days: readonly Day[]): { from: string; to: string } {
	const first = days[0]?.key ?? "1970-01-01";
	const last = days[days.length - 1]?.key ?? first;
	return {
		from: `${addDays(first, -1)}T00:00:00Z`,
		to: `${addDays(last, 2)}T00:00:00Z`,
	};
}

export interface Placed<T> {
	day: string;
	wall: Wall;
	item: T;
}

// Targets on the days of the grid, by the day of their time in the zone of the person, in the order of their time.
export function placeByDay<T>(
	items: readonly T[],
	at: (item: T) => string | null,
	zone: string,
): Map<string, Placed<T>[]> {
	const byDay = new Map<string, Placed<T>[]>();
	for (const item of items) {
		const iso = at(item);
		if (iso === null) continue;
		const wall = wallOf(iso, zone);
		if (wall === null) continue;
		const list = byDay.get(wall.day) ?? [];
		list.push({ day: wall.day, wall, item });
		byDay.set(wall.day, list);
	}
	for (const list of byDay.values()) {
		list.sort((a, b) => a.wall.local.localeCompare(b.wall.local));
	}
	return byDay;
}

export function today(zone: string): string {
	return wallOf(new Date().toISOString(), zone)?.day ?? "1970-01-01";
}
