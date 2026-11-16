import { useEffect } from "react";

// The forms of the projects page each show their own alert; submitting one
// clears the alerts of the others, so an old error does not stay on screen.
const listeners = new Set<(except: string) => void>();

export function clearOtherAlerts(owner: string): void {
	for (const listener of listeners) listener(owner);
}

export function onOtherSubmit(owner: string, clear: () => void): () => void {
	const listener = (except: string): void => {
		if (except !== owner) clear();
	};
	listeners.add(listener);
	return () => {
		listeners.delete(listener);
	};
}

export function useClearAlertOnOtherSubmit(
	owner: string,
	clear: () => void,
): void {
	// biome-ignore lint/correctness/useExhaustiveDependencies: `clear` only calls a state setter, which is stable.
	useEffect(() => onOtherSubmit(owner, clear), [owner]);
}
