import { usePage } from "@inertiajs/react";
import { type ReactNode, useEffect } from "react";

import LocaleSwitcher from "@/components/LocaleSwitcher";
import ReleaseMark from "@/components/ReleaseMark";
import ThemeToggle from "@/components/ThemeToggle";
import { TooltipProvider } from "@/components/ui/tooltip";
import { type I18nProps, I18nProvider } from "@/lib/i18n";
import { SessionProvider } from "@/lib/session";
import type { SharedProps } from "@/types";
import DashboardLayout from "./DashboardLayout";

// Every Inertia page renders inside this layout. It feeds the translation
// context from the page props, so useT works the same here and in the editor
// (SDD-012 section 16.3).
export default function AppLayout({ children }: { children: ReactNode }) {
	const { messages = {}, locale = "en", locales } = usePage<I18nProps>().props;
	const { session } = usePage<SharedProps>().props;
	// The page prop is the language of the rendering; <html lang> follows it after
	// an Inertia visit or reload, which never replaces the root template (VV-169).
	useEffect(() => {
		document.documentElement.lang = locale;
	}, [locale]);
	return (
		<I18nProvider messages={messages} locale={locale} locales={locales}>
			<SessionProvider session={session}>
				<Shell>{children}</Shell>
			</SessionProvider>
		</I18nProvider>
	);
}

function Shell({ children }: { children: ReactNode }) {
	const page = usePage<SharedProps>();
	// The shared prop can arrive empty; the link keeps a name (axe link-name).
	const appName = page.props.appName || "Stuarty";

	// The login has no session and no sections to lead to: a plain header, no
	// sidebar. Every other page is drawn by the dashboard (SDD-012 section 23.2).
	if (page.component.startsWith("Auth/")) {
		return (
			<TooltipProvider>
				<header className="flex min-h-14 flex-wrap items-center gap-x-4 gap-y-1 border-b px-4 py-2 md:px-6">
					<span className="flex items-center">
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
						<span className="sr-only">{appName}</span>
					</span>
					<div className="ml-auto flex flex-wrap items-center gap-2">
						<LocaleSwitcher />
						<ThemeToggle />
					</div>
				</header>
				<ReleaseMark />
				<main className="flex-1 p-4 md:p-6">{children}</main>
			</TooltipProvider>
		);
	}

	return (
		<TooltipProvider>
			<DashboardLayout>{children}</DashboardLayout>
		</TooltipProvider>
	);
}
