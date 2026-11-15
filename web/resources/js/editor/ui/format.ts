function decimalSeparator(locale: string): string {
	return (
		new Intl.NumberFormat(locale)
			.formatToParts(1.1)
			.find((part) => part.type === "decimal")?.value ?? "."
	);
}

// Display only: the core's `duration_seconds` as m:ss.t, with the decimal
// separator of the active locale (a comma in pt-BR, D-030). The browser does
// not derive a duration from frame counts and rates (SDD-012 section 13.2).
export function formatSeconds(seconds: number, locale: string): string {
	const tenths = Math.round(seconds * 10);
	const whole = Math.floor(tenths / 10);
	const minutes = Math.floor(whole / 60);
	const rest = String(whole % 60).padStart(2, "0");
	return `${minutes}:${rest}${decimalSeparator(locale)}${tenths % 10}`;
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
