import { type KeyboardEvent, useState } from "react";

import { shiftLocal, type Wall } from "./calendar";
import { actionsOf } from "./publicationState";
import type { Target } from "./types";

export interface Moving {
	targetId: string;
	local: string;
}

// The keyboard move of a target (SDD-020 section 30): Space takes it, the arrow keys move it by a day (left and right)
// or an hour (up and down), Enter drops it and Escape lets it go. Only the new wall time is computed here, as text the
// service reads in the zone; what the time means is the service's. The same keys work on a chip of the month, of the
// week and of the day.
export function useTargetMove(
	onMove: (publicationId: string, targetId: string, local: string) => void,
): {
	moving: Moving | null;
	cancel(): void;
	onKey(
		event: KeyboardEvent<HTMLElement>,
		publicationId: string,
		target: Target,
		wall: Wall,
	): void;
} {
	const [moving, setMoving] = useState<Moving | null>(null);

	function onKey(
		event: KeyboardEvent<HTMLElement>,
		publicationId: string,
		target: Target,
		wall: Wall,
	): void {
		if (!actionsOf(target.state).edit) return;
		if (moving === null || moving.targetId !== target.id) {
			if (event.key === " ") {
				event.preventDefault();
				setMoving({ targetId: target.id, local: wall.local });
			}
			return;
		}
		const step: Record<string, [number, number]> = {
			ArrowLeft: [-1, 0],
			ArrowRight: [1, 0],
			ArrowUp: [0, -1],
			ArrowDown: [0, 1],
		};
		const delta = step[event.key];
		if (delta !== undefined) {
			event.preventDefault();
			setMoving({
				...moving,
				local: shiftLocal(moving.local, delta[0], delta[1]),
			});
		} else if (event.key === "Enter" || event.key === " ") {
			event.preventDefault();
			setMoving(null);
			if (moving.local !== wall.local) {
				onMove(publicationId, target.id, moving.local);
			}
		} else if (event.key === "Escape") {
			event.preventDefault();
			setMoving(null);
		}
	}

	return { moving, cancel: () => setMoving(null), onKey };
}
