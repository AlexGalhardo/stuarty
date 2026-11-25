import { activeNavItem, NAV_GROUPS, pathOf } from "../dashboard/sections";

// One step of the trail above a page. `label` is a translation key: the shell never writes a name of its
// own, so a page that is not in the sidebar has no step it did not earn.
export interface Crumb {
	label: string;
	href: string;
	current: boolean;
}

const STUDIO = "ui.shell.breadcrumb.studio";
const ACCOUNT = "ui.shell.breadcrumb.account";

// The trail of a url: the group of the sidebar that holds the path (the studio for the Home, the account for
// Settings), then the item itself. The last step is the current page; a page below an item (a project, an
// asset) belongs to its item, so the item is the last step. A group has no page: its step links to the
// first item of the group.
export function breadcrumbOf(url: string): Crumb[] {
	const path = pathOf(url);
	if (path === "/") {
		return [
			{ label: STUDIO, href: "/", current: false },
			{ label: "ui.nav.dashboard_home", href: "/", current: true },
		];
	}
	const id = activeNavItem(url);
	const group = NAV_GROUPS.find((candidate) =>
		candidate.items.some((item) => item.id === id),
	);
	const item = group?.items.find((candidate) => candidate.id === id);
	if (group !== undefined && item !== undefined) {
		return [
			{
				label: group.label,
				href: group.items[0]?.href ?? "/",
				current: false,
			},
			{ label: item.label, href: item.href, current: true },
		];
	}
	if (path === "/settings" || path.startsWith("/settings/")) {
		return [
			{ label: ACCOUNT, href: "/settings", current: false },
			{ label: "ui.nav.settings", href: "/settings", current: true },
		];
	}
	return [{ label: STUDIO, href: "/", current: true }];
}
