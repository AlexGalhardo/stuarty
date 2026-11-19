import type { Money } from "../../editor/api/types";

// An amount of the approval queue is an integer in millionths of the currency unit (SDD-015
// section 3). The page shows it and reads what a person types; it adds, compares and decides
// nothing: every ceiling and every sum is the service's.

const MICROS = 1_000_000;

// "$1.50" in the locale of the person. A currency Intl does not know is shown as its code.
export function formatMoney(money: Money, locale: string): string {
	const units = money.micros / MICROS;
	try {
		return new Intl.NumberFormat(locale, {
			style: "currency",
			currency: money.currency,
			maximumFractionDigits: 6,
			minimumFractionDigits: 2,
		}).format(units);
	} catch {
		return `${units} ${money.currency}`;
	}
}

// The amount a person typed ("5", "0.5", "1,25") as micros, or null when it is not an amount of at
// most six decimals. It is text handling: no floating point, so 0.1 is exactly 100000.
export function parseMicros(text: string): number | null {
	const trimmed = text.trim().replace(",", ".");
	const match = /^(\d{1,12})(?:\.(\d{0,6}))?$/.exec(trimmed);
	if (match === null) return null;
	const whole = match[1] ?? "0";
	const fraction = (match[2] ?? "").padEnd(6, "0");
	return Number(whole) * MICROS + Number(fraction);
}

// The inverse of parseMicros for a form field: 1500000 is "1.5".
export function microsToInput(micros: number): string {
	const whole = Math.trunc(micros / MICROS);
	const fraction = String(micros % MICROS)
		.padStart(6, "0")
		.replace(/0+$/, "");
	return fraction === "" ? String(whole) : `${whole}.${fraction}`;
}
