import { useEffect } from "react";

import { useActions } from "../commands/useActions";

function isEditable(target: EventTarget | null): boolean {
	if (!(target instanceof HTMLElement)) return false;
	return (
		target.isContentEditable ||
		["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
	);
}

// The keys of the Prototype (SDD-012 sections 8.2 and 15): S splits, [ and ]
// trim to the playhead, C opens Add caption, Ctrl+Z undoes, Ctrl+Shift+Z and
// Ctrl+Y redo. Typing in a field never triggers them.
export function useShortcuts(openCaption: () => void): void {
	const actions = useActions();
	useEffect(() => {
		const onKey = (event: KeyboardEvent): void => {
			if (isEditable(event.target) || event.altKey) return;
			const mod = event.ctrlKey || event.metaKey;
			const key = event.key.toLowerCase();
			if (mod) {
				if (key === "z" && !event.shiftKey) void actions.undo();
				else if ((key === "z" && event.shiftKey) || key === "y") {
					void actions.redo();
				} else return;
				event.preventDefault();
				return;
			}
			if (key === "s") void actions.splitAtPlayhead();
			else if (key === "[") void actions.trimToPlayhead("start");
			else if (key === "]") void actions.trimToPlayhead("end");
			else if (key === "c") openCaption();
			else return;
			event.preventDefault();
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, [actions, openCaption]);
}
