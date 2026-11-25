import { usePage } from "@inertiajs/react";
import { type ReactNode, useEffect } from "react";

import LocaleSwitcher from "@/components/LocaleSwitcher";
import ReleaseMark from "@/components/ReleaseMark";
import ThemeToggle from "@/components/ThemeToggle";
import { TooltipProvider } from "@/components/ui/tooltip";
import { type I18nProps, I18nProvider } from "@/lib/i18n";
import { useLockWatch } from "@/lib/lockWatch";
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
	const accountPage =
		page.component.startsWith("Auth/") || page.component.startsWith("Account/");
	// A page drawn before an idle lock must not stay on the screen after it (ADR-0052). Only local mode has an account.
	useLockWatch(!accountPage && page.props.account !== undefined);

	// The login has no session and no sections to lead to: a plain header, no
	// sidebar. Every other page is drawn by the dashboard (SDD-012 section 23.2).
	// The pages before the studio opens, and the error page, are one centred card with no sidebar and no top bar
	// (the final design); the language and the theme sit in the corner. The sr-only name keeps the product named.
	if (accountPage || page.component === "Error") {
		return (
			<TooltipProvider>
				<div className="relative flex min-h-dvh flex-col">
					<span className="sr-only">{appName}</span>
					<div className="absolute end-4 top-3 z-10 flex flex-wrap items-center gap-2 md:end-6">
						<LocaleSwitcher />
						<ThemeToggle />
					</div>
					<ReleaseMark />
					<main className="flex flex-1 flex-col justify-center p-4 pt-16 md:p-6 md:pt-16">
						{children}
					</main>
				</div>
			</TooltipProvider>
		);
	}

	return (
		<TooltipProvider>
			<DashboardLayout>{children}</DashboardLayout>
		</TooltipProvider>
	);
}
