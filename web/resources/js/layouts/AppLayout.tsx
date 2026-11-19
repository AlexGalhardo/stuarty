import { Link, usePage } from "@inertiajs/react";
import { Video } from "lucide-react";
import { type ReactNode, useEffect } from "react";

import LocaleSwitcher from "@/components/LocaleSwitcher";
import ReleaseMark from "@/components/ReleaseMark";
import ThemeToggle from "@/components/ThemeToggle";
import { Button } from "@/components/ui/button";
import { TooltipProvider } from "@/components/ui/tooltip";
import { type I18nProps, I18nProvider, useT } from "@/lib/i18n";
import type { SharedProps } from "@/types";
import { createClient } from "../editor/api/client";
import { HttpTransport } from "../editor/api/http";

// Every Inertia page renders inside this layout. It feeds the translation
// context from the page props, so useT works the same here and in the editor
// (SDD-012 section 16.3).
export default function AppLayout({ children }: { children: ReactNode }) {
	const { messages = {}, locale = "en", locales } = usePage<I18nProps>().props;
	// The page prop is the language of the rendering; <html lang> follows it after
	// an Inertia visit or reload, which never replaces the root template (VV-169).
	useEffect(() => {
		document.documentElement.lang = locale;
	}, [locale]);
	return (
		<I18nProvider messages={messages} locale={locale} locales={locales}>
			<Shell>{children}</Shell>
		</I18nProvider>
	);
}

function Shell({ children }: { children: ReactNode }) {
	const t = useT();
	const page = usePage<SharedProps>();
	const message = page.flash?.success ?? page.flash?.status;
	// The shared prop can arrive empty; the link keeps a name (axe link-name).
	const appName = page.props.appName || "Stuarty";

	return (
		<TooltipProvider>
			<header className="flex min-h-14 flex-wrap items-center gap-x-4 gap-y-1 border-b px-4 py-2 md:px-6">
				<Link
					href="/projects"
					aria-label={t("ui.nav.home", { name: appName })}
					className="flex items-center gap-2"
				>
					<span className="flex size-8 items-center justify-center rounded-lg bg-brand text-brand-foreground">
						<Video aria-hidden="true" className="size-4" />
					</span>
					<span className="text-lg font-semibold tracking-tight">
						{appName}
					</span>
				</Link>
				<nav aria-label={t("ui.nav.label")} className="text-sm">
					<Link
						href="/projects"
						aria-current={page.url.startsWith("/projects") ? "page" : undefined}
						className="rounded-md px-2 py-1 hover:bg-muted aria-[current=page]:font-medium"
					>
						{t("ui.nav.projects")}
					</Link>
					<Link
						href="/settings"
						aria-current={page.url.startsWith("/settings") ? "page" : undefined}
						className="rounded-md px-2 py-1 hover:bg-muted aria-[current=page]:font-medium"
					>
						{t("ui.nav.settings")}
					</Link>
				</nav>
				<div className="ml-auto flex flex-wrap items-center gap-2">
					<LocaleSwitcher />
					<ThemeToggle />
					{page.props.auth?.user != null && <Account />}
				</div>
			</header>
			<ReleaseMark />
			<main className="flex-1 p-4 md:p-6">
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
		</TooltipProvider>
	);
}

// Network mode only (the shared prop `auth.user` is set): the keys page and the
// end of the session. `POST /auth/logout` clears the cookie; the next page is
// the login.
function Account(): ReactNode {
	const t = useT();
	const page = usePage<SharedProps>();
	return (
		<>
			<Link
				href="/settings/keys"
				aria-current={
					page.url.startsWith("/settings/keys") ? "page" : undefined
				}
				className="rounded-md px-2 py-1 text-sm hover:bg-muted aria-[current=page]:font-medium"
			>
				{t("ui.keys.nav")}
			</Link>
			<Button
				type="button"
				variant="ghost"
				size="sm"
				onClick={() =>
					void createClient(new HttpTransport(""))
						.logout()
						.catch(() => undefined)
						.then(() => window.location.assign("/login"))
				}
			>
				{t("ui.login.logout")}
			</Button>
		</>
	);
}
