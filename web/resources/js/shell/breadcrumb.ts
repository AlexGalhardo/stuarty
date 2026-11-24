import { activeNavItem, NAV_GROUPS, pathOf } from "../dashboard/sections";

// One step of the trail above a page. `label` is a translation key: the shell never writes a name of its
// own, so a page that is not in the sidebar has no step it did not earn.
export interface Crumb {
	label: string;
	href: string;
	current: boolean;
}

const STUDIO = "ui.shell.breadcrumb.studio";

// The trail of a url: the studio, then the sidebar item that holds the path (or Home, or Settings). The
// last step is the current page; a page below an item (a project, an asset) belongs to its item, so the
// item is the last step.
export function breadcrumbOf(url: string): Crumb[] {
	const path = pathOf(url);
	if (path === "/") {
		return [
			{ label: STUDIO, href: "/", current: false },
			{ label: "ui.nav.dashboard_home", href: "/", current: true },
		];
	}
	const id = activeNavItem(url);
	const item = NAV_GROUPS.flatMap((group) => group.items).find(
		(candidate) => candidate.id === id,
	);
	if (item !== undefined) {
		return [
			{ label: STUDIO, href: "/", current: false },
			{ label: item.label, href: item.href, current: true },
		];
	}
	if (path === "/settings" || path.startsWith("/settings/")) {
		return [
			{ label: STUDIO, href: "/", current: false },
			{ label: "ui.nav.settings", href: "/settings", current: true },
		];
	}
	return [{ label: STUDIO, href: "/", current: true }];
}
