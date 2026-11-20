import { usePage } from "@inertiajs/react";
import { type ReactNode, useEffect, useRef } from "react";

import ChatDock from "@/dashboard/chat/ChatDock";
import Sidebar from "@/dashboard/Sidebar";
import { useT } from "@/lib/i18n";
import type { SharedProps } from "@/types";
import ReleaseMark from "../components/ReleaseMark";

// The path of the page the person was on: a change of page moves the focus to the main region, so a
// reader that does not reload the page is told that the page changed (WCAG 2.4.3, SDD-012 section 18.10).
let lastPath: string | null = null;

// The frame of every page except the editor and the login (SDD-012 section
// 23.2): the sidebar, the main region and the right dock, which holds the
// panel of the agent chat (SDD-017) behind one button. The first focusable
// element is a link that skips the sidebar.
export default function DashboardLayout({
	children,
}: {
	children: ReactNode;
}): ReactNode {
	const t = useT();
	const page = usePage<SharedProps>();
	const main = useRef<HTMLElement>(null);
	const path = page.url.split(/[?#]/)[0] ?? page.url;
	useEffect(() => {
		if (lastPath !== null && lastPath !== path) main.current?.focus();
		lastPath = path;
	}, [path]);
	const message = page.flash?.success ?? page.flash?.status;
	// The shared prop can arrive empty; the link keeps a name (axe link-name).
	const appName = page.props.appName || "Stuarty";

	return (
		<div className="flex min-h-dvh max-md:flex-col">
			{/* biome-ignore lint/a11y/useValidAnchor: a skip link is a real link to a place of the page; the click adds the focus move that Safari does not make on its own. */}
			<a
				href="#main"
				className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-background focus:px-3 focus:py-2 focus:text-sm focus:ring-2 focus:ring-ring"
				onClick={() => main.current?.focus()}
			>
				{t("ui.nav.skip")}
			</a>
			<Sidebar
				url={page.url}
				appName={appName}
				user={page.props.auth?.user ?? null}
			/>
			<ReleaseMark />
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
			<div data-testid="right-dock" data-dock="right">
				<ChatDock />
			</div>
		</div>
	);
}
