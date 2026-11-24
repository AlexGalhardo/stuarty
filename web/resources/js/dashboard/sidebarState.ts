// The sidebar has three forms (SDD-012 section 23.4): full, an icon rail and, on a phone, a drawer. The
// choice between full and rail is remembered in localStorage.sidebar; with none stored, the rail is the
// form from 768 to 1280 px and the full form is the one above it.
export type SidebarState = "expanded" | "collapsed";
export type SidebarForm = "full" | "rail" | "drawer";

export const SIDEBAR_KEY = "sidebar";

// NARROW_QUERY is the MOBILE_BREAKPOINT of hooks/use-mobile.ts (768 px).
export const NARROW_QUERY = "(max-width: 767px)";
// MEDIUM_QUERY: the window where the rail is the default.
export const MEDIUM_QUERY = "(max-width: 1280px)";

export function parseSidebar(raw: string | null | undefined): SidebarState {
	return raw === "collapsed" ? "collapsed" : "expanded";
}

// The stored choice, or null when the person never made one.
export function parseChoice(
	raw: string | null | undefined,
): SidebarState | null {
	return raw === "collapsed" || raw === "expanded" ? raw : null;
}

export function toggled(state: SidebarState): SidebarState {
	return state === "collapsed" ? "expanded" : "collapsed";
}

export function sidebarForm(
	chosen: SidebarState | null,
	narrow: boolean,
	medium: boolean,
): SidebarForm {
	if (narrow) return "drawer";
	if (chosen === "collapsed") return "rail";
	if (chosen === "expanded") return "full";
	return medium ? "rail" : "full";
}

export function readChoice(): SidebarState | null {
	try {
		return parseChoice(localStorage.getItem(SIDEBAR_KEY));
	} catch {
		return null;
	}
}

export function writeSidebar(state: SidebarState): void {
	try {
		localStorage.setItem(SIDEBAR_KEY, state);
	} catch {
		// Storage can be blocked (private mode): the choice lasts for this page only.
	}
}
