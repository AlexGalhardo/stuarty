import { router } from "@inertiajs/react";
import { useCallback, useEffect, useState } from "react";

import { isDocumentNavigation } from "../workflow/continueTarget";
import type { Crumb } from "./breadcrumb";

export const NAV_HISTORY_KEY = "stuarty.nav.history";
export const MAX_NAV_HISTORY = 50;

export interface HistoryStorage {
	getItem(key: string): string | null;
	setItem(key: string, value: string): void;
}

function browserStorage(): HistoryStorage | null {
	if (typeof window === "undefined") return null;
	try {
		return window.sessionStorage;
	} catch {
		return null;
	}
}

export function normalizeUrl(url: string): string {
	const trimmed = url.trim();
	return trimmed === "" ? "/" : trimmed;
}

export function readNavHistory(storage: HistoryStorage | null): string[] {
	if (storage === null) return [];
	try {
		const raw = storage.getItem(NAV_HISTORY_KEY);
		if (raw === null) return [];
		const parsed: unknown = JSON.parse(raw);
		if (!Array.isArray(parsed)) return [];
		return parsed.filter(
			(entry): entry is string => typeof entry === "string" && entry !== "",
		);
	} catch {
		return [];
	}
}

function writeNavHistory(
	storage: HistoryStorage | null,
	entries: string[],
): void {
	if (storage === null) return;
	try {
		storage.setItem(NAV_HISTORY_KEY, JSON.stringify(entries));
	} catch {
		// Private browsing or full storage must never break navigation.
	}
}

// Appends the current URL to the in-app navigation stack, deduplicating consecutive
// visits and capping the history length.
export function recordNavPath(
	storage: HistoryStorage | null,
	url: string,
): string[] {
	const current = normalizeUrl(url);
	const list = readNavHistory(storage);
	if (list[list.length - 1] === current) return list;
	const next = [...list, current].slice(-MAX_NAV_HISTORY);
	writeNavHistory(storage, next);
	return next;
}

// Computes the parent route to fall back to when a person lands directly on a deep URL.
export function fallbackBackHref(
	url: string,
	crumbs: readonly Crumb[],
): string | null {
	const path = normalizeUrl(url).split(/[?#]/)[0] ?? "/";
	if (path === "/") return null;
	const last = crumbs[crumbs.length - 1];
	if (last !== undefined && last.href !== path) return last.href;
	for (let i = crumbs.length - 2; i >= 0; i -= 1) {
		const candidate = crumbs[i];
		if (candidate !== undefined && candidate.href !== path) {
			return candidate.href;
		}
	}
	return "/";
}

// Reads the previous target without mutating the stored history stack.
export function peekBackTarget(
	storage: HistoryStorage | null,
	currentUrl: string,
	fallback: string | null,
): string | null {
	const current = normalizeUrl(currentUrl);
	const list = readNavHistory(storage);
	const trimmed = list[list.length - 1] === current ? list.slice(0, -1) : list;
	const prev = trimmed[trimmed.length - 1];
	if (prev !== undefined && prev !== current) return prev;
	return fallback !== null && fallback !== current ? fallback : null;
}

// Pops the current and previous entry from the stack so navigating back does not
// immediately bounce forward again.
export function popBackTarget(
	storage: HistoryStorage | null,
	currentUrl: string,
	fallback: string | null,
): string | null {
	const current = normalizeUrl(currentUrl);
	const list = readNavHistory(storage);
	const trimmed = list[list.length - 1] === current ? list.slice(0, -1) : list;
	const prev = trimmed[trimmed.length - 1];
	if (prev !== undefined && prev !== current) {
		writeNavHistory(storage, trimmed);
		return prev;
	}
	return fallback !== null && fallback !== current ? fallback : null;
}

export function useBackNavigation(
	url: string,
	fallback: string | null,
	mode: "inertia" | "document" = "inertia",
): { canGoBack: boolean; target: string | null; goBack: () => void } {
	const [target, setTarget] = useState<string | null>(() =>
		peekBackTarget(browserStorage(), url, fallback),
	);

	useEffect(() => {
		const storage = browserStorage();
		recordNavPath(storage, url);
		setTarget(peekBackTarget(storage, url, fallback));
	}, [url, fallback]);

	const goBack = useCallback(() => {
		const storage = browserStorage();
		const dest = popBackTarget(storage, url, fallback);
		if (dest === null) return;
		if (mode === "document" || isDocumentNavigation(dest)) {
			window.location.assign(dest);
			return;
		}
		router.visit(dest);
	}, [url, fallback, mode]);

	return { canGoBack: target !== null, target, goBack };
}
