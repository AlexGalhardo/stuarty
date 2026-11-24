import { Link } from "@inertiajs/react";
import { ChevronRight, Menu, MessagesSquare } from "lucide-react";
import { Fragment, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { breadcrumbOf } from "./breadcrumb";
import { useShell } from "./ShellContext";

// The top bar of every dashboard page (SDD-012 section 23.4): the way to the navigation on a phone, the
// breadcrumb, the slot of the bell and the Chat button. It is the banner of the page.
export default function TopBar({
	url,
	drawer,
	onOpenMenu,
	menuOpen,
	children,
}: {
	url: string;
	// the sidebar is a drawer: the bar carries the button that opens it
	drawer: boolean;
	onOpenMenu: () => void;
	menuOpen: boolean;
	// the bell, between the breadcrumb and the Chat button
	children?: ReactNode;
}): ReactNode {
	const t = useT();
	const { chatOpen, openChat, chatButton } = useShell();
	const crumbs = breadcrumbOf(url);

	return (
		<header
			data-testid="topbar"
			className="sticky top-0 z-30 flex h-14 shrink-0 items-center gap-2 border-b bg-background px-3 md:px-6"
		>
			{drawer && (
				<Button
					type="button"
					variant="ghost"
					size="icon"
					aria-label={t("ui.shell.menu")}
					aria-expanded={menuOpen}
					onClick={onOpenMenu}
					data-testid="menu-toggle"
				>
					<Menu aria-hidden="true" />
				</Button>
			)}
			<nav
				aria-label={t("ui.shell.breadcrumb.label")}
				className="min-w-0 flex-1"
			>
				<ol className="flex items-center gap-1 text-sm text-muted-foreground">
					{crumbs.map((crumb, index) => (
						<Fragment key={crumb.label}>
							{index > 0 && (
								<li aria-hidden="true" className="shrink-0 max-sm:hidden">
									<ChevronRight className="size-4" />
								</li>
							)}
							<li
								className={cn(
									"min-w-0 truncate",
									// a phone keeps the page, the one thing the bar must say
									!crumb.current && "max-sm:hidden",
								)}
							>
								{crumb.current ? (
									<span
										aria-current="page"
										className="font-medium text-foreground"
									>
										{t(crumb.label)}
									</span>
								) : (
									<Link href={crumb.href} className="rounded hover:underline">
										{t(crumb.label)}
									</Link>
								)}
							</li>
						</Fragment>
					))}
				</ol>
			</nav>
			{children}
			<Button
				ref={chatButton}
				type="button"
				variant="outline"
				size="sm"
				aria-label={t("ui.chat.open")}
				aria-expanded={chatOpen}
				aria-controls="chat-panel"
				onClick={openChat}
				data-testid="chat-toggle"
				className={cn(chatOpen && "invisible")}
			>
				<MessagesSquare aria-hidden="true" />
				{t("ui.shell.chat")}
			</Button>
		</header>
	);
}
