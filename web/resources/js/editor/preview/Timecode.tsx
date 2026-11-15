import type { ReactNode } from "react";

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
