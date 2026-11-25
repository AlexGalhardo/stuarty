import { Link } from "@inertiajs/react";
import {
	House,
	LogOut,
	PanelLeftClose,
	PanelLeftOpen,
	Settings,
	User,
} from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";

import ReleaseMark from "@/components/ReleaseMark";
import ThemeToggle from "@/components/ThemeToggle";
import { Button } from "@/components/ui/button";
import {
	Sheet,
	SheetContent,
	SheetDescription,
	SheetTitle,
} from "@/components/ui/sheet";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { useSession } from "@/lib/session";
import type { User as Account, LocalAccount } from "@/types";
import { createClient } from "../editor/api/client";
import { ApiError } from "../editor/api/errors";
import { HttpTransport } from "../editor/api/http";
import { activeNavItem, NAV_GROUPS, pathOf } from "./sections";
import {
	MEDIUM_QUERY,
	NARROW_QUERY,
	readChoice,
	type SidebarForm,
	type SidebarState,
	sidebarForm,
	toggled,
	writeSidebar,
} from "./sidebarState";

const ITEM =
	"flex h-8 items-center gap-2 rounded-md px-3 text-sm text-muted-foreground outline-offset-[-2px] hover:bg-muted hover:text-foreground aria-[current=page]:bg-brand-subtle aria-[current=page]:font-medium aria-[current=page]:text-brand-text";

// The form of the sidebar for this window: the stored choice, or the width. The first render is the full
// one on the server and in the browser alike; the choice and the width come once the page is mounted.
export function useSidebar(): {
	form: SidebarForm;
	toggle(): void;
} {
	const [chosen, setChosen] = useState<SidebarState | null>(null);
	const [narrow, setNarrow] = useState(false);
	const [medium, setMedium] = useState(false);

	useEffect(() => {
		setChosen(readChoice());
		const queries = [
			[window.matchMedia(NARROW_QUERY), setNarrow],
			[window.matchMedia(MEDIUM_QUERY), setMedium],
		] as const;
		const stops = queries.map(([query, set]) => {
			set(query.matches);
			const onChange = (event: MediaQueryListEvent) => set(event.matches);
			query.addEventListener("change", onChange);
			return () => query.removeEventListener("change", onChange);
		});
		return () => {
			for (const stop of stops) stop();
		};
	}, []);

	const form = sidebarForm(chosen, narrow, medium);
	return {
		form,
		toggle() {
			const next = toggled(form === "rail" ? "collapsed" : "expanded");
			setChosen(next);
			writeSidebar(next);
		},
	};
}

// An item with an icon and a label. In the rail, the label stays for the screen reader and a tooltip
// shows it; the link keeps its name.
function Item({
	href,
	current,
	icon,
	label,
	collapsed,
	testId,
	count = 0,
}: {
	href: string;
	current: boolean;
	icon: ReactNode;
	label: string;
	collapsed: boolean;
	testId?: string;
	// requests that wait for a person; a number the server reported, drawn as a word for a reader too
	count?: number;
}): ReactNode {
	const t = useT();
	const link = (
		<Link
			href={href}
			aria-current={current ? "page" : undefined}
			data-testid={testId}
			// min-w-8: an icon alone stays a 32 px target (WCAG 2.5.8, the dashboard minimum).
			className={cn(ITEM, collapsed && "relative min-w-8 justify-center px-0")}
		>
			{icon}
			<span className={cn(collapsed && "sr-only")}>{label}</span>
			{count > 0 && (
				<>
					<span
						aria-hidden="true"
						data-testid="nav-count"
						className={cn(
							"ms-auto min-w-5 rounded-full bg-warning-soft px-1.5 text-center text-xs font-medium text-warning-text tabular-nums",
							collapsed && "absolute -end-0.5 -top-0.5 ms-0",
						)}
					>
						{count}
					</span>
					<span className="sr-only">
						{t("ui.shell.waiting_count", { count })}
					</span>
				</>
			)}
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
	account,
	person,
	url,
	collapsed,
}: {
	user: Account | null;
	account: LocalAccount | null;
	person: boolean;
	url: string;
	collapsed: boolean;
}): ReactNode {
	const t = useT();
	const path = pathOf(url);

	// The studio is locked only when the server said so, or when this session had already ended: on any other
	// failure the page stays, because leaving it would say the studio is closed while it is open.
	async function logout(): Promise<void> {
		try {
			await createClient(new HttpTransport("")).accountLogout();
		} catch (error) {
			if (
				!(error instanceof ApiError) ||
				(error.code !== "E_LOCKED" && error.code !== "E_UNAUTHORIZED")
			)
				return;
		}
		window.location.assign("/account/signin");
	}

	const name = user?.email ?? account?.username ?? t("ui.nav.local");
	return (
		<section
			aria-label={t("ui.nav.profile")}
			className="flex flex-col gap-0.5 border-t border-border pt-2"
		>
			<Item
				href="/settings"
				current={path === "/settings"}
				icon={<Settings aria-hidden="true" className="size-4 shrink-0" />}
				label={t("ui.nav.settings")}
				collapsed={collapsed}
			/>
			{/* Only a person can lock the studio: a launch link is refused, and the button would seem to work. */}
			{account?.state === "unlocked" && person && (
				<Button
					type="button"
					variant="ghost"
					size="sm"
					className={cn(
						"h-8 justify-start px-3 text-sm font-normal text-muted-foreground",
						collapsed && "min-w-8 justify-center px-0",
					)}
					aria-label={collapsed ? t("ui.account.logout") : undefined}
					data-testid="account-logout"
					onClick={() => void logout()}
				>
					<LogOut aria-hidden="true" />
					<span className={cn(collapsed && "sr-only")}>
						{t("ui.account.logout")}
					</span>
				</Button>
			)}
			{user != null && (
				<Button
					type="button"
					variant="ghost"
					size="sm"
					className={cn(
						"h-8 justify-start px-3 text-sm font-normal text-muted-foreground",
						collapsed && "min-w-8 justify-center px-0",
					)}
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
			)}
			<p
				className={cn(
					"flex h-6 items-center gap-2 px-3 text-xs text-muted-foreground",
					collapsed && "justify-center px-0",
				)}
				data-testid="profile-name"
			>
				<User aria-hidden="true" className="size-3.5 shrink-0" />
				<span className={cn("truncate", collapsed && "sr-only")}>{name}</span>
			</p>
		</section>
	);
}

// What a sidebar holds, in the same order in its three forms: the mark, the navigation by group, the
// profile and the footer with the version, the mode and the controls.
function SidebarBody({
	url,
	appName,
	user,
	account,
	person,
	collapsed,
	pendingApprovals,
	onToggle,
}: {
	url: string;
	appName: string;
	user: Account | null;
	account: LocalAccount | null;
	person: boolean;
	collapsed: boolean;
	pendingApprovals: number;
	// not given in the drawer, which has no collapse button
	onToggle?: () => void;
}): ReactNode {
	const t = useT();
	const { mode } = useSession();
	const current = activeNavItem(url);
	const home = pathOf(url) === "/";

	return (
		// In the drawer the whole block scrolls, so a very short window (400 % zoom) keeps every item in reach
		<div
			className={cn(
				"flex min-h-0 flex-1 flex-col gap-2",
				onToggle === undefined && "overflow-y-auto",
			)}
		>
			<Link
				href="/"
				aria-label={t("ui.nav.home", { name: appName })}
				aria-current={home ? "page" : undefined}
				className={cn(
					"flex h-10 items-center gap-2 rounded-md px-1",
					collapsed && "justify-center",
				)}
			>
				{collapsed ? (
					<img
						src="/icon.png"
						loading="lazy"
						alt=""
						className="size-8 shrink-0"
					/>
				) : (
					<>
						<img
							src="/logo-wordmark-light.png"
							loading="lazy"
							alt=""
							className="h-7 w-auto dark:hidden"
						/>
						<img
							src="/logo-wordmark-dark.png"
							loading="lazy"
							alt=""
							className="hidden h-7 w-auto dark:block"
						/>
					</>
				)}
				<span className="sr-only">{appName}</span>
			</Link>
			<nav
				aria-label={t("ui.nav.label")}
				className={cn(
					onToggle === undefined
						? "flex-none"
						: "min-h-0 flex-1 overflow-y-auto",
				)}
			>
				<ul className="flex flex-col gap-0.5">
					<li>
						<Item
							href="/"
							current={home}
							icon={<House aria-hidden="true" className="size-4 shrink-0" />}
							label={t("ui.nav.dashboard_home")}
							collapsed={collapsed}
							testId="nav-home"
						/>
					</li>
					{NAV_GROUPS.map((group) => {
						const items = group.items.filter(
							(item) => item.networkOnly !== true || user != null,
						);
						return (
							<li key={group.id} data-testid={`nav-group-${group.id}`}>
								<p
									id={`nav-group-${group.id}`}
									className={cn(
										"mt-1 px-3 py-0.5 text-xs font-medium text-muted-foreground",
										collapsed && "sr-only",
									)}
								>
									{t(group.label)}
								</p>
								{collapsed && (
									<div
										aria-hidden="true"
										className="mx-2 mt-2 border-t border-border"
									/>
								)}
								<ul
									aria-labelledby={`nav-group-${group.id}`}
									className="flex flex-col gap-0.5"
								>
									{items.map(({ id, href, icon: Icon, label, counter }) => (
										<li key={id}>
											<Item
												href={href}
												current={current === id}
												icon={
													<Icon
														aria-hidden="true"
														className="size-4 shrink-0"
													/>
												}
												label={t(label)}
												collapsed={collapsed}
												testId={`nav-${id}`}
												count={counter === "approvals" ? pendingApprovals : 0}
											/>
										</li>
									))}
								</ul>
							</li>
						);
					})}
				</ul>
			</nav>
			<Profile
				user={user}
				account={account}
				person={person}
				url={url}
				collapsed={collapsed}
			/>
			<div
				className={cn(
					"flex items-center gap-1",
					collapsed ? "flex-col" : "justify-between",
				)}
			>
				<div
					data-testid="sidebar-version"
					className={cn(
						"flex items-center gap-2 px-1",
						collapsed && "flex-col gap-0",
					)}
				>
					<ReleaseMark inline />
					<span
						data-testid="sidebar-mode"
						className={cn(
							"text-xs text-muted-foreground",
							collapsed && "sr-only",
						)}
					>
						{t(`ui.shell.mode.${mode}`)}
					</span>
				</div>
				<div className={cn("flex items-center gap-1", collapsed && "flex-col")}>
					<ThemeToggle />
					{onToggle !== undefined && (
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									type="button"
									variant="ghost"
									size="icon"
									onClick={onToggle}
									aria-expanded={!collapsed}
									aria-label={
										collapsed ? t("ui.nav.expand") : t("ui.nav.collapse")
									}
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
					)}
				</div>
			</div>
		</div>
	);
}

// The dashboard sidebar (SDD-012 section 23.4): the groups of sections, then the profile and the controls.
// It computes nothing: the current item comes from the url and the count from the approval queue. In the
// drawer form it draws no column; the drawer is a dialog that the menu button of the top bar opens.
export default function Sidebar({
	url,
	appName,
	user,
	account = null,
	person = false,
	form = "full",
	pendingApprovals = 0,
	onToggle,
	drawerOpen = false,
	onDrawerOpenChange,
}: {
	url: string;
	appName: string;
	user: Account | null;
	account?: LocalAccount | null;
	// Whether this session was opened with the password (the shared prop session.privileged).
	person?: boolean;
	form?: SidebarForm;
	pendingApprovals?: number;
	onToggle?: () => void;
	drawerOpen?: boolean;
	onDrawerOpenChange?: (open: boolean) => void;
}): ReactNode {
	const t = useT();
	const body = (collapsed: boolean, drawer: boolean): ReactNode => (
		<SidebarBody
			url={url}
			appName={appName}
			user={user}
			account={account}
			person={person}
			collapsed={collapsed}
			pendingApprovals={pendingApprovals}
			onToggle={drawer ? undefined : onToggle}
		/>
	);

	if (form === "drawer") {
		return (
			<Sheet
				open={drawerOpen}
				onOpenChange={(open) => onDrawerOpenChange?.(open)}
			>
				<SheetContent
					side="left"
					data-testid="sidebar-drawer"
					className="w-72 max-w-[85vw] gap-0 p-2 sm:max-w-72"
				>
					<SheetTitle className="sr-only">
						{t("ui.shell.drawer.title")}
					</SheetTitle>
					<SheetDescription className="sr-only">
						{t("ui.shell.drawer.description")}
					</SheetDescription>
					{body(false, true)}
				</SheetContent>
			</Sheet>
		);
	}

	const collapsed = form === "rail";
	// max-md:hidden: the first render does not know the width yet; on a phone the column would stand beside the
	// page, wider than the window, until the drawer form replaces it.
	return (
		<div
			data-testid="sidebar"
			data-state={collapsed ? "collapsed" : "expanded"}
			data-form={form}
			className={cn(
				"flex shrink-0 flex-col bg-surface-sunken p-2 max-md:hidden md:sticky md:top-0 md:h-dvh md:border-r",
				collapsed ? "w-14" : "w-60",
			)}
		>
			{body(collapsed, false)}
		</div>
	);
}
