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
