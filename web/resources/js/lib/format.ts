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
