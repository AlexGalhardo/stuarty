import { Link } from "@inertiajs/react";
import {
	LogOut,
	PanelLeftClose,
	PanelLeftOpen,
	Settings,
	User,
	Video,
} from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";

import LocaleSwitcher from "@/components/LocaleSwitcher";
import ThemeToggle from "@/components/ThemeToggle";
import { Button } from "@/components/ui/button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import type { User as Account } from "@/types";
import { createClient } from "../editor/api/client";
import { HttpTransport } from "../editor/api/http";
import { activeSection, pathOf, SECTIONS } from "./sections";
import {
	effectiveSidebar,
	NARROW_QUERY,
	readSidebar,
	type SidebarState,
	toggled,
	writeSidebar,
} from "./sidebarState";

const ITEM =
	"flex h-9 items-center gap-2 rounded-md px-3 text-sm text-muted-foreground outline-offset-[-2px] hover:bg-muted hover:text-foreground aria-[current=page]:bg-muted aria-[current=page]:font-medium aria-[current=page]:text-foreground";

// The chosen state of the sidebar, and whether the window is narrow. The first
// render is the expanded one on the server and in the browser alike; the choice
// comes from storage once the page is mounted.
export function useSidebar(): {
	state: SidebarState;
	toggle(): void;
} {
	const [chosen, setChosen] = useState<SidebarState>("expanded");
	const [narrow, setNarrow] = useState(false);

	useEffect(() => {
		setChosen(readSidebar());
		const query = window.matchMedia(NARROW_QUERY);
		setNarrow(query.matches);
		const onChange = (event: MediaQueryListEvent) => setNarrow(event.matches);
		query.addEventListener("change", onChange);
		return () => query.removeEventListener("change", onChange);
	}, []);

	const state = effectiveSidebar(chosen, narrow);
	return {
		state,
		toggle() {
			const next = toggled(state);
			setChosen(next);
			writeSidebar(next);
		},
	};
}

// An item with an icon and a label. Collapsed, the label stays for the screen
// reader and a tooltip shows it; the link keeps its name.
function Item({
	href,
	current,
	icon,
	label,
	collapsed,
	testId,
}: {
	href: string;
	current: boolean;
	icon: ReactNode;
	label: string;
	collapsed: boolean;
	testId?: string;
}): ReactNode {
	const link = (
		<Link
			href={href}
			aria-current={current ? "page" : undefined}
			data-testid={testId}
			className={cn(ITEM, collapsed && "justify-center px-0")}
		>
			{icon}
			<span className={cn(collapsed && "sr-only")}>{label}</span>
		</Link>
	);
	if (!collapsed) return link;
	return (
		<Tooltip>
			<TooltipTrigger asChild>{link}</TooltipTrigger>
			<TooltipContent side="right">{label}</TooltipContent>
		</Tooltip>
	);
}

// The profile block: the signed-in email in network mode, "Local" in local
// mode, a link to settings and, where there is a session, the keys and the end
// of it. POST /auth/logout clears the cookie; the next page is the login.
function Profile({
	user,
	url,
	collapsed,
}: {
	user: Account | null;
	url: string;
	collapsed: boolean;
}): ReactNode {
	const t = useT();
	const path = pathOf(url);
	const name = user?.email ?? t("ui.nav.local");
	return (
		<section
			aria-label={t("ui.nav.profile")}
			className="flex flex-col gap-1 max-md:flex-row max-md:items-center"
		>
			<p
				className={cn(
					"flex h-9 items-center gap-2 px-3 text-sm",
					collapsed && "justify-center px-0",
				)}
				data-testid="profile-name"
			>
				<User aria-hidden="true" className="size-4 shrink-0" />
				<span className={cn("truncate", collapsed && "sr-only")}>{name}</span>
			</p>
			<Item
				href="/settings"
				current={path === "/settings"}
				icon={<Settings aria-hidden="true" className="size-4 shrink-0" />}
				label={t("ui.nav.settings")}
				collapsed={collapsed}
			/>
			{user != null && (
				<>
					<Item
						href="/settings/keys"
						current={path.startsWith("/settings/keys")}
						icon={<span aria-hidden="true" className="size-4 shrink-0" />}
						label={t("ui.keys.nav")}
						collapsed={collapsed}
					/>
					<Button
						type="button"
						variant="ghost"
						size="sm"
						className={cn("justify-start", collapsed && "justify-center")}
						aria-label={collapsed ? t("ui.login.logout") : undefined}
						onClick={() =>
							void createClient(new HttpTransport(""))
								.logout()
								.catch(() => undefined)
								.then(() => window.location.assign("/login"))
						}
					>
						<LogOut aria-hidden="true" />
						<span className={cn(collapsed && "sr-only")}>
							{t("ui.login.logout")}
						</span>
					</Button>
				</>
			)}
		</section>
	);
}

// The dashboard sidebar (SDD-012 section 23.2): the eleven sections, then the
// profile and the controls. It computes nothing: the current section comes from
// the url.
export default function Sidebar({
	url,
	appName,
	user,
}: {
	url: string;
	appName: string;
	user: Account | null;
}): ReactNode {
	const t = useT();
	const { state, toggle } = useSidebar();
	const collapsed = state === "collapsed";
	const current = activeSection(url);

	return (
		<div
			data-testid="sidebar"
			data-state={state}
			className={cn(
				"flex shrink-0 flex-col gap-2 bg-background p-2",
				// Wide: a column fixed to the window. Narrow: a strip above the page, so
				// the page keeps the whole width (WCAG 1.4.10 reflow at 320 px).
				"md:sticky md:top-0 md:h-dvh md:border-r md:pb-8",
				"max-md:w-full max-md:flex-row max-md:flex-wrap max-md:items-center max-md:border-b",
				collapsed ? "md:w-14" : "md:w-64",
			)}
		>
			<Link
				href="/"
				aria-label={t("ui.nav.home", { name: appName })}
				aria-current={pathOf(url) === "/" ? "page" : undefined}
				className={cn(
					"flex h-10 items-center gap-2 rounded-md px-1",
					collapsed && "justify-center",
				)}
			>
				<span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-brand text-brand-foreground">
					<Video aria-hidden="true" className="size-4" />
				</span>
				<span
					className={cn(
						"truncate text-lg font-semibold tracking-tight",
						collapsed && "sr-only",
					)}
				>
					{appName}
				</span>
			</Link>
			<nav
				aria-label={t("ui.nav.label")}
				className="min-h-0 flex-1 overflow-y-auto max-md:order-last max-md:w-full max-md:flex-none max-md:overflow-x-auto max-md:overflow-y-hidden"
			>
				<ul className="flex flex-col gap-0.5 max-md:flex-row">
					{SECTIONS.map(({ key, path, icon: Icon }) => (
						<li key={key}>
							<Item
								href={path}
								current={current === key}
								icon={<Icon aria-hidden="true" className="size-4 shrink-0" />}
								label={t(`ui.nav.section.${key}`)}
								collapsed={collapsed}
								testId={`nav-${key}`}
							/>
						</li>
					))}
				</ul>
			</nav>
			<Profile user={user} url={url} collapsed={collapsed} />
			<div
				className={cn(
					"flex items-center gap-1",
					collapsed ? "flex-col max-md:flex-row" : "justify-between",
				)}
			>
				<LocaleSwitcher
					className={cn(
						collapsed &&
							"w-10 justify-center px-1 [&>[data-slot=select-value]]:sr-only",
					)}
				/>
				<ThemeToggle />
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							type="button"
							variant="ghost"
							size="icon"
							onClick={toggle}
							aria-expanded={!collapsed}
							aria-label={collapsed ? t("ui.nav.expand") : t("ui.nav.collapse")}
							data-testid="sidebar-toggle"
						>
							{collapsed ? (
								<PanelLeftOpen aria-hidden="true" />
							) : (
								<PanelLeftClose aria-hidden="true" />
							)}
						</Button>
					</TooltipTrigger>
					<TooltipContent side="right">
						{collapsed ? t("ui.nav.expand") : t("ui.nav.collapse")}
					</TooltipContent>
				</Tooltip>
			</div>
		</div>
	);
}
