import { type ReactNode, useEffect, useRef } from "react";

import { formatTimecode, type TimecodeRate } from "../ui/timecode";

// Geist with tabular numbers, not a monospace face (SDD-012 section 11.5).
export default function Timecode({
	frame,
	fps,
	label,
}: {
	frame: number;
	fps: TimecodeRate;
	label: string;
}): ReactNode {
	return (
		<span
			role="timer"
			aria-label={label}
			className="text-xs [font-variant-numeric:tabular-nums]"
		>
			{formatTimecode(frame, fps)}
		</span>
	);
}

// Writes the text of the span directly so playback does not re-render the
// preview for it (SDD-012 section 19.2 rule 1).
export function LiveTimecode({
	initial,
	fps,
	label,
	subscribe,
}: {
	initial: number;
	fps: TimecodeRate;
	label: string;
	subscribe: (cb: (frame: number) => void) => () => void;
}): ReactNode {
	const ref = useRef<HTMLSpanElement>(null);
	useEffect(
		() =>
			subscribe((frame) => {
				if (ref.current !== null) {
					ref.current.textContent = formatTimecode(frame, fps);
				}
			}),
		[subscribe, fps],
	);
	return (
		<span
			ref={ref}
			role="timer"
			aria-label={label}
			className="text-xs [font-variant-numeric:tabular-nums]"
		>
			{formatTimecode(initial, fps)}
		</span>
	);
}
