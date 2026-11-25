// The day of the publishing calendar as the service answers it (SDD-020 section 30A): the note, the theme and the quiet
// mark of a day, the recurring slots and the target that fills each one. The page draws these and decides none of
// them: whether a day is quiet, which slot a target fills and what a slot means on a date are the service's.

import type { Publication } from "./types";

// The names a day may take. Each is a family of the design system whose text-on-soft-surface pair `check:contrast`
// measures, so a theme adds no colour of its own.
export const THEMES = [
	"blue",
	"green",
	"amber",
	"red",
	"teal",
	"gray",
] as const;
export type Theme = (typeof THEMES)[number];

export function isTheme(value: unknown): value is Theme {
	return THEMES.includes(value as Theme);
}

export interface CalendarDay {
	// the local date, 2026-12-25, and the zone the day was planned in
	date: string;
	zone: string;
	note?: string;
	theme?: string;
	quiet?: boolean;
	updated_at: string;
}

export const WEEKDAYS = [
	"mon",
	"tue",
	"wed",
	"thu",
	"fri",
	"sat",
	"sun",
] as const;

export interface SlotRule {
	id: string;
	network: string;
	weekday: string;
	// HH:MM on the wall clock of zone
	time: string;
	zone: string;
	connection_id?: string;
}

export interface SlotOccurrence {
	slot_id: string;
	network: string;
	connection_id?: string;
	// the wall time of the slot, in zone, and the instant in UTC
	date: string;
	time: string;
	zone: string;
	at: string;
	// "gap" or "ambiguous" when the clock changed over that wall time
	adjustment?: string;
	filled?: { publication_id: string; target_id: string };
}

export interface CalendarPlan {
	from: string;
	to: string;
	zone: string;
	days: CalendarDay[];
	rules: SlotRule[];
	slots: SlotOccurrence[];
	publications: Publication[];
	total: number;
}

export interface DayInput {
	zone?: string;
	note?: string;
	theme?: string;
}

export interface SlotInput {
	id?: string;
	network: string;
	weekday: string;
	time: string;
	zone?: string;
	connection_id?: string;
}

export interface DuplicateInput {
	target_id?: string;
	at: string;
	zone?: string;
}
