import { useEffect } from "react";

// How often an open page asks whether the studio locked. The route needs no session and does not count as
// activity, so this never keeps the idle lock from firing (SDD-012 section 2.1.1).
export const LOCK_WATCH_MS = 15_000;

export interface LockWatchDeps {
	// The state the server reports, or null when it could not be read.
	readState(): Promise<string | null>;
	onLocked(): void;
	isVisible(): boolean;
	repeat(fn: () => void, ms: number): () => void;
	onVisible(fn: () => void): () => void;
}

// A page drawn before the lock stays on the screen until something asks the server: this takes the words of the
// person off it. A server that does not answer changes nothing.
export function startLockWatch(deps: LockWatchDeps): () => void {
	let stopped = false;
	const check = async (): Promise<void> => {
		if (stopped || !deps.isVisible()) return;
		const state = await deps.readState();
		if (stopped || state !== "locked") return;
		stopped = true;
		deps.onLocked();
	};
	const stopTimer = deps.repeat(() => void check(), LOCK_WATCH_MS);
	const stopVisible = deps.onVisible(() => void check());
	return () => {
		stopped = true;
		stopTimer();
		stopVisible();
	};
}

async function readAccountState(): Promise<string | null> {
	try {
		const response = await fetch("/auth/account", {
			credentials: "same-origin",
			cache: "no-store",
			headers: { Accept: "application/json" },
		});
		if (!response.ok) return null;
		const body = (await response.json()) as { state?: unknown };
		return typeof body.state === "string" ? body.state : null;
	} catch {
		return null;
	}
}

export function useLockWatch(enabled: boolean): void {
	useEffect(() => {
		if (!enabled) return;
		return startLockWatch({
			readState: readAccountState,
			onLocked: () => window.location.assign("/account/signin"),
			isVisible: () => document.visibilityState === "visible",
			repeat: (fn, ms) => {
				const id = window.setInterval(fn, ms);
				return () => window.clearInterval(id);
			},
			onVisible: (fn) => {
				document.addEventListener("visibilitychange", fn);
				return () => document.removeEventListener("visibilitychange", fn);
			},
		});
	}, [enabled]);
}
