// The one display formatter of every time in the editor (SDD-012 section 12.2).

export type TimeDisplay = "timecode" | "frames" | "seconds";

const pad = (n: number, width: number): string =>
	String(n).padStart(width, "0");

function timecodeOf(totalMs: number): string {
	const ms = totalMs % 1000;
	const whole = Math.floor(totalMs / 1000);
	const hours = Math.floor(whole / 3600);
	const minutes = Math.floor((whole % 3600) / 60);
	const clock = `${pad(minutes, 2)}:${pad(whole % 60, 2)}.${pad(ms, 3)}`;
	return hours > 0 ? `${pad(hours, 2)}:${clock}` : clock;
}

// Display only, always with a point (timecodes are not localized, section
// 16.4). `fps` is the project rational; the result is never parsed back.
export function formatTime(
	frame: number,
	fps: { num: number; den: number },
	mode: TimeDisplay,
): string {
	if (mode === "frames") return `${frame}f`;
	const ms = Math.round((frame * 1000 * fps.den) / fps.num);
	return mode === "seconds" ? `${(ms / 1000).toFixed(3)}s` : timecodeOf(ms);
}

// A duration the core gives in seconds (an asset's `duration_seconds`),
// formatted without a trip through frames.
export function formatSeconds(seconds: number, mode: TimeDisplay): string {
	const ms = Math.round(seconds * 1000);
	return mode === "timecode" ? timecodeOf(ms) : `${(ms / 1000).toFixed(3)}s`;
}
