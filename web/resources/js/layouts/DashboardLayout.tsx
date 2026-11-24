import { usePage } from "@inertiajs/react";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { useApprovalQueue } from "@/dashboard/approval/useApprovalQueue";
import ChatDock from "@/dashboard/chat/ChatDock";
import { useChat } from "@/dashboard/chat/useChat";
import Sidebar, { useSidebar } from "@/dashboard/Sidebar";
import { useT } from "@/lib/i18n";
import type { SharedProps } from "@/types";
import ConnectedSceneBar from "../shell/ConnectedSceneBar";
import NotificationBell from "../shell/NotificationBell";
import { projectInUrl, ShellProvider, useShell } from "../shell/ShellContext";
import TopBar from "../shell/TopBar";

// The path of the page the person was on: a change of page moves the focus to the main region, so a
// reader that does not reload the page is told that the page changed (WCAG 2.4.3, SDD-012 section 18.10).
let lastPath: string | null = null;

// The frame of every page except the editor and the login (SDD-012 sections
// 23.2 and 23.4): the sidebar, the top bar, the main region and the right dock,
// which holds the panel of the agent chat (SDD-017). The first focusable
// element is a link that skips the navigation.
export default function DashboardLayout({
	children,
}: {
	children: ReactNode;
}): ReactNode {
	const page = usePage<SharedProps>();
	// One read of the agent session for the whole frame: the scene bar and the panel show the same one.
	const chat = useChat(true, projectInUrl(page.url));
	const approvals = useApprovalQueue();
	return (
		<ShellProvider chat={chat} approvals={approvals}>
			<Frame>{children}</Frame>
		</ShellProvider>
	);
}

function Frame({ children }: { children: ReactNode }): ReactNode {
	const t = useT();
	const page = usePage<SharedProps>();
	const main = useRef<HTMLElement>(null);
	const sidebar = useSidebar();
	const { approvals } = useShell();
	const [menuOpen, setMenuOpen] = useState(false);
	const path = page.url.split(/[?#]/)[0] ?? page.url;
	useEffect(() => {
		if (lastPath !== null && lastPath !== path) main.current?.focus();
		lastPath = path;
		// following a link of the drawer is leaving it
		setMenuOpen(false);
	}, [path]);
	// The image editor keeps the default text size; every other page of the
	// dashboard reads larger (D-123, app.css).
	const scaled = page.component !== "EditImage/Show";
	useEffect(() => {
		const root = document.documentElement;
		if (scaled) root.dataset.textScale = "dashboard";
		else delete root.dataset.textScale;
		return () => {
			delete root.dataset.textScale;
		};
	}, [scaled]);
	const message = page.flash?.success ?? page.flash?.status;
	// The shared prop can arrive empty; the link keeps a name (axe link-name).
	const appName = page.props.appName || "Stuarty";

	return (
		<div className="flex min-h-dvh">
			{/* biome-ignore lint/a11y/useValidAnchor: a skip link is a real link to a place of the page; the click adds the focus move that Safari does not make on its own. */}
			<a
				href="#main"
				className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-background focus:px-3 focus:py-2 focus:text-sm focus:ring-2 focus:ring-ring"
				onClick={() => main.current?.focus()}
			>
				{t("ui.nav.skip")}
			</a>
			<Sidebar
				form={sidebar.form}
				onToggle={sidebar.toggle}
				url={page.url}
				appName={appName}
				user={page.props.auth?.user ?? null}
				account={page.props.account ?? null}
				person={page.props.session?.privileged ?? false}
				pendingApprovals={approvals.requests?.length ?? 0}
				drawerOpen={menuOpen}
				onDrawerOpenChange={setMenuOpen}
			/>
			<div className="flex min-w-0 flex-1 flex-col">
				<TopBar
					url={page.url}
					drawer={sidebar.form === "drawer"}
					menuOpen={menuOpen}
					onOpenMenu={() => setMenuOpen(true)}
				>
					<NotificationBell />
				</TopBar>
				<main
					ref={main}
					id="main"
					tabIndex={-1}
					className="min-w-0 flex-1 p-4 md:p-6"
				>
					{typeof message === "string" && (
						<p
							role="status"
							className="mb-4 rounded-lg border bg-muted px-4 py-3 text-sm"
						>
							{message}
						</p>
					)}
					{children}
				</main>
				<ConnectedSceneBar />
			</div>
			<div data-testid="right-dock" data-dock="right">
				<ChatDock />
			</div>
		</div>
	);
}
