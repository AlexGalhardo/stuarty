import { type ReactNode, useEffect, useRef } from "react";
import { readTimeDisplay } from "../state/prefs";
import { formatTime } from "../ui/timeFormat";

interface TimecodeRate {
	num: number;
	den: number;
}

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
			{formatTime(frame, fps, readTimeDisplay())}
		</span>
	);
}

// The current time: a field that follows the playhead while it is not being typed in, and seeks to the
// time typed on Enter or on leaving it (VV-127). It writes the text of the input directly so playback
// does not re-render the preview for it (SDD-012 section 19.2 rule 1).
export function LiveTimecode({
	initial,
	fps,
	label,
	subscribe,
	parse,
	onSeek,
}: {
	initial: number;
	fps: TimecodeRate;
	label: string;
	subscribe: (cb: (frame: number) => void) => () => void;
	// The frame a typed text names, or null (`timeToFrame` of commands/units.ts, passed by the shell).
	parse: (text: string) => number | null;
	onSeek: (frame: number) => void;
}): ReactNode {
	const ref = useRef<HTMLInputElement>(null);
	const latest = useRef(initial);
	useEffect(
		() =>
			subscribe((frame) => {
				latest.current = frame;
				if (ref.current !== null && document.activeElement !== ref.current) {
					ref.current.value = formatTime(frame, fps, readTimeDisplay());
				}
			}),
		[subscribe, fps],
	);
	const commit = (input: HTMLInputElement): void => {
		const frame = parse(input.value);
		if (frame === null) {
			input.setAttribute("aria-invalid", "true");
			return;
		}
		input.removeAttribute("aria-invalid");
		onSeek(frame);
		input.value = formatTime(frame, fps, readTimeDisplay());
	};
	return (
		<input
			ref={ref}
			type="text"
			aria-label={label}
			defaultValue={formatTime(initial, fps, readTimeDisplay())}
			spellCheck={false}
			autoComplete="off"
			size={12}
			className="h-6 w-24 rounded bg-transparent px-1 text-xs [font-variant-numeric:tabular-nums] aria-invalid:ring-1 aria-invalid:ring-destructive"
			onKeyDown={(event) => {
				// The keys of the editor (space, J/K/L, arrows) are not for a field.
				event.stopPropagation();
				if (event.key === "Enter") {
					event.preventDefault();
					commit(event.currentTarget);
				} else if (event.key === "Escape") {
					event.currentTarget.value = formatTime(
						latest.current,
						fps,
						readTimeDisplay(),
					);
					event.currentTarget.removeAttribute("aria-invalid");
					event.currentTarget.blur();
				}
			}}
			onBlur={(event) => {
				if (
					event.currentTarget.value !==
					formatTime(latest.current, fps, readTimeDisplay())
				) {
					commit(event.currentTarget);
				}
			}}
		/>
	);
}
