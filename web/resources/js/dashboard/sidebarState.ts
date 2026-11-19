// The sidebar is expanded or collapsed to icons. The choice is remembered in
// localStorage.sidebar; a narrow window collapses it whatever was chosen, and
// the stored choice is kept for the next wide window (SDD-012 section 23.2).
export type SidebarState = "expanded" | "collapsed";

export const SIDEBAR_KEY = "sidebar";

// NARROW_QUERY is the MOBILE_BREAKPOINT of hooks/use-mobile.ts (768 px).
export const NARROW_QUERY = "(max-width: 767px)";

export function parseSidebar(raw: string | null | undefined): SidebarState {
	return raw === "collapsed" ? "collapsed" : "expanded";
}

export function toggled(state: SidebarState): SidebarState {
	return state === "collapsed" ? "expanded" : "collapsed";
}

export function effectiveSidebar(
	chosen: SidebarState,
	narrow: boolean,
): SidebarState {
	return narrow ? "collapsed" : chosen;
}

export function readSidebar(): SidebarState {
	try {
		return parseSidebar(localStorage.getItem(SIDEBAR_KEY));
	} catch {
		return "expanded";
	}
}

export function writeSidebar(state: SidebarState): void {
	try {
		localStorage.setItem(SIDEBAR_KEY, state);
	} catch {
		// Storage can be blocked (private mode): the choice lasts for this page only.
	}
}
