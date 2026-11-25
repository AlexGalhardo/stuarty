// Dates use Intl with the active locale (SDD-012 section 16.4). A size is never formatted here: the service writes
// it with the one rule of core/asset (SDD-011 section 14.9) and the page draws it.
export function formatDateTime(iso: string, locale: string): string {
	const date = new Date(iso);
	if (Number.isNaN(date.getTime())) return iso;
	return new Intl.DateTimeFormat(locale, {
		dateStyle: "medium",
		timeStyle: "short",
	}).format(date);
}

// The day only, for a card that tells two files of the same name apart.
export function formatDate(iso: string, locale: string): string {
	const date = new Date(iso);
	if (Number.isNaN(date.getTime())) return iso;
	return new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(date);
}

// How long ago a moment was, in the largest unit that fits ("4 min ago"), for the rows that wait for a person. It
// only words a distance of time: the moment itself is the service's.
export function formatAgo(
	iso: string,
	locale: string,
	now: Date = new Date(),
): string {
	const then = new Date(iso);
	if (Number.isNaN(then.getTime())) return iso;
	const seconds = Math.max(
		0,
		Math.round((now.getTime() - then.getTime()) / 1000),
	);
	const rtf = new Intl.RelativeTimeFormat(locale, {
		numeric: "always",
		style: "narrow",
	});
	// -0 reads "0m ago": a plain 0 would read "in 0m".
	if (seconds < 60) return rtf.format(-0, "minute");
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return rtf.format(-minutes, "minute");
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return rtf.format(-hours, "hour");
	return rtf.format(-Math.floor(hours / 24), "day");
}
