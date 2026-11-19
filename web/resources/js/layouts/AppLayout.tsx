import { usePage } from "@inertiajs/react";
import { Video } from "lucide-react";
import { type ReactNode, useEffect } from "react";

import LocaleSwitcher from "@/components/LocaleSwitcher";
import ReleaseMark from "@/components/ReleaseMark";
import ThemeToggle from "@/components/ThemeToggle";
import { TooltipProvider } from "@/components/ui/tooltip";
import { type I18nProps, I18nProvider } from "@/lib/i18n";
import type { SharedProps } from "@/types";
import DashboardLayout from "./DashboardLayout";

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
	const page = usePage<SharedProps>();
	// The shared prop can arrive empty; the link keeps a name (axe link-name).
	const appName = page.props.appName || "Stuarty";

	// The login has no session and no sections to lead to: a plain header, no
	// sidebar. Every other page is drawn by the dashboard (SDD-012 section 23.2).
	if (page.component.startsWith("Auth/")) {
		return (
			<TooltipProvider>
				<header className="flex min-h-14 flex-wrap items-center gap-x-4 gap-y-1 border-b px-4 py-2 md:px-6">
					<span className="flex items-center gap-2">
						<span className="flex size-8 items-center justify-center rounded-lg bg-brand text-brand-foreground">
							<Video aria-hidden="true" className="size-4" />
						</span>
						<span className="text-lg font-semibold tracking-tight">
							{appName}
						</span>
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
