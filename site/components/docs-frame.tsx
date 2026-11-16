import { Sidebar } from "@/components/sidebar";
import { getNavGroups } from "@/lib/content";
import { ui } from "@/lib/ui-text";

/** Sidebar + content columns of every /docs page. */
export function DocsFrame({
	pathname,
	children,
}: {
	pathname: string;
	children: React.ReactNode;
}): React.ReactElement {
	return (
		<div className="mx-auto flex max-w-[90rem] px-4 sm:px-6">
			<aside
				aria-label={ui.documentation}
				className="sticky top-(--header-h) hidden h-[calc(100dvh-var(--header-h))] w-64 shrink-0 overflow-y-auto py-8 pr-6 scroll-slim lg:block"
			>
				<Sidebar groups={getNavGroups()} pathname={pathname} />
			</aside>
			<div className="min-w-0 flex-1">{children}</div>
		</div>
	);
}
