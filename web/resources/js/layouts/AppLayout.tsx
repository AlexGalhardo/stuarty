import { Link, usePage } from "@inertiajs/react";
import { Video } from "lucide-react";
import type { ReactNode } from "react";

import LocaleSwitcher from "@/components/LocaleSwitcher";
import ThemeToggle from "@/components/ThemeToggle";
import { TooltipProvider } from "@/components/ui/tooltip";
import { type I18nProps, I18nProvider, useT } from "@/lib/i18n";
import type { SharedProps } from "@/types";

// Every Inertia page renders inside this layout. It feeds the translation
// context from the page props, so useT works the same here and in the editor
// (SDD-012 section 16.3).
export default function AppLayout({ children }: { children: ReactNode }) {
	const { messages = {}, locale = "en" } = usePage<I18nProps>().props;
	return (
		<I18nProvider messages={messages} locale={locale}>
			<Shell>{children}</Shell>
		</I18nProvider>
	);
}

function Shell({ children }: { children: ReactNode }) {
	const t = useT();
	const page = usePage<SharedProps>();
	const message = page.flash?.success ?? page.flash?.status;

	return (
		<TooltipProvider>
			<header className="flex h-14 items-center gap-4 border-b px-4 md:px-6">
				<Link href="/projects" className="flex items-center gap-2">
					<span className="flex size-8 items-center justify-center rounded-lg bg-brand text-brand-foreground">
						<Video aria-hidden="true" className="size-4" />
					</span>
					<span className="text-lg font-semibold tracking-tight">
						{page.props.appName}
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
				</nav>
				<div className="ml-auto flex items-center gap-2">
					<LocaleSwitcher />
					<ThemeToggle />
				</div>
			</header>
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
