import {
	type KeyboardEvent,
	type PointerEvent,
	type ReactNode,
	useRef,
} from "react";

import { cn } from "@/lib/cn";

const STEP_PX = 4;
const BIG_STEP_PX = 16;

// A horizontal separator that resizes the element above it (the WAI-ARIA window
// splitter pattern): drag with the pointer, Up and Down with the keyboard (Shift
// for larger steps), Home and End for the bounds, a double click or Enter for
// the default. It computes a size for the screen only, never an edit.
export default function Splitter({
	label,
	value,
	min,
	max,
	defaultValue,
	onChange,
	className,
}: {
	label: string;
	value: number;
	min: number;
	max: number;
	defaultValue: number;
	onChange(px: number): void;
	className?: string;
}): ReactNode {
	const drag = useRef<{ startY: number; startValue: number } | null>(null);
	const stop = (event: { stopPropagation(): void }): void =>
		event.stopPropagation();
	const clamp = (px: number): number => Math.min(Math.max(px, min), max);

	const onKeyDown = (event: KeyboardEvent<HTMLDivElement>): void => {
		const step = event.shiftKey ? BIG_STEP_PX : STEP_PX;
		let next: number | null = null;
		if (event.key === "ArrowUp") next = value - step;
		else if (event.key === "ArrowDown") next = value + step;
		else if (event.key === "Home") next = min;
		else if (event.key === "End") next = max;
		else if (event.key === "Enter") next = defaultValue;
		if (next === null) return;
		event.preventDefault();
		event.stopPropagation();
		onChange(clamp(next));
	};

	const onPointerDown = (event: PointerEvent<HTMLDivElement>): void => {
		stop(event);
		if (event.button !== 0) return;
		event.currentTarget.setPointerCapture(event.pointerId);
		drag.current = { startY: event.clientY, startValue: value };
	};

	return (
		// biome-ignore lint/a11y/useSemanticElements: a focusable separator is the splitter pattern; <hr> cannot take focus or a value.
		<div
			role="separator"
			tabIndex={0}
			aria-orientation="horizontal"
			aria-label={label}
			aria-valuenow={value}
			aria-valuemin={min}
			aria-valuemax={max}
			data-splitter=""
			className={cn(
				"absolute inset-x-0 bottom-0 z-10 h-2 cursor-row-resize touch-none hover:bg-ed-handle/60 focus-visible:bg-ed-handle/60",
				className,
			)}
			onClick={stop}
			onDoubleClick={(event) => {
				stop(event);
				onChange(defaultValue);
			}}
			onKeyDown={onKeyDown}
			onPointerDown={onPointerDown}
			onPointerMove={(event) => {
				const start = drag.current;
				if (start === null) return;
				onChange(clamp(start.startValue + event.clientY - start.startY));
			}}
			onPointerUp={(event) => {
				drag.current = null;
				event.currentTarget.releasePointerCapture(event.pointerId);
			}}
			onPointerCancel={() => {
				drag.current = null;
			}}
		/>
	);
}
