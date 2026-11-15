// Display only: the core's `duration_seconds` as m:ss.t (SDD-012 section 13.2).
// The browser does not derive a duration from frame counts and rates.
export function formatSeconds(seconds: number): string {
	const tenths = Math.round(seconds * 10);
	const whole = Math.floor(tenths / 10);
	const minutes = Math.floor(whole / 60);
	const rest = String(whole % 60).padStart(2, "0");
	return `${minutes}:${rest}.${tenths % 10}`;
}
