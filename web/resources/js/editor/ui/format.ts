// A message that starts with {actor} starts with a lower-case word when the
// actor is the localized fallback ("alguém"): the sentence gets its capital.
export function capitalizeFirst(text: string): string {
	return text.charAt(0).toLocaleUpperCase() + text.slice(1);
}

// One decimal place in the active locale, for sizes such as "18,4".
export function formatDecimal(value: number, locale: string): string {
	return new Intl.NumberFormat(locale, {
		minimumFractionDigits: 1,
		maximumFractionDigits: 1,
	}).format(value);
}

const UNITS: readonly [Intl.RelativeTimeFormatUnit, number][] = [
	["day", 86_400_000],
	["hour", 3_600_000],
	["minute", 60_000],
	["second", 1000],
];

// "2 min ago", in the active locale, from an ISO instant (SDD-012 section 15).
// Display only.
export function formatRelative(
	iso: string,
	nowMs: number,
	locale: string,
): string {
	const at = Date.parse(iso);
	const formatter = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
	if (Number.isNaN(at)) return "";
	const delta = at - nowMs;
	for (const [unit, size] of UNITS) {
		if (Math.abs(delta) >= size || unit === "second") {
			return formatter.format(Math.round(delta / size), unit);
		}
	}
	return "";
}
