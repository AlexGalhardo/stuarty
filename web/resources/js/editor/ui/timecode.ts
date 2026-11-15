export interface TimecodeRate {
	num: number;
	den: number;
}

const pad2 = (n: number): string => String(n).padStart(2, "0");

// mm:ss:ff for display only. The whole frames per second are the nominal rate
// (29.97 counts 30), the convention of the ruler.
export function formatTimecode(frame: number, fps: TimecodeRate): string {
	const perSecond = Math.max(1, Math.round(fps.num / fps.den));
	const whole = Math.floor(frame / perSecond);
	return `${pad2(Math.floor(whole / 60))}:${pad2(whole % 60)}:${pad2(frame % perSecond)}`;
}
