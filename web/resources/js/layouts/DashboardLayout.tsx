import { usePage } from "@inertiajs/react";
import type { ReactNode } from "react";

import Sidebar from "@/dashboard/Sidebar";
import type { SharedProps } from "@/types";
import ReleaseMark from "../components/ReleaseMark";

// The frame of every page except the editor and the login (SDD-012 section
// 23.2): the sidebar, the main region and the right dock, which stays empty
// until the agent chat of epic 14 takes it.
export default function DashboardLayout({
	children,
}: {
	children: ReactNode;
}): ReactNode {
	const page = usePage<SharedProps>();
	const message = page.flash?.success ?? page.flash?.status;
	// The shared prop can arrive empty; the link keeps a name (axe link-name).
	const appName = page.props.appName || "Stuarty";

	return (
		<div className="flex min-h-dvh max-md:flex-col">
			<Sidebar
				url={page.url}
				appName={appName}
				user={page.props.auth?.user ?? null}
			/>
			<ReleaseMark />
			<main className="min-w-0 flex-1 p-4 md:p-6">
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
			<div data-testid="right-dock" data-dock="right" />
		</div>
	);
}
