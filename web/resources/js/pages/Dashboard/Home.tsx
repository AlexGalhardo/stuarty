import { Head, Link } from "@inertiajs/react";
import type { ReactNode } from "react";

import { SECTIONS } from "@/dashboard/sections";
import { useT } from "@/lib/i18n";

// The home of the application (RF-310): a card for each section, saying what it
// holds. The sidebar is the navigation; this page is where a person lands.
export default function Home(): ReactNode {
	const t = useT();

	return (
		<>
			<Head title={t("ui.dashboard.home.title")} />
			<h1 className="text-2xl font-semibold tracking-tight">
				{t("ui.dashboard.home.title")}
			</h1>
			<p className="mt-1 text-muted-foreground">
				{t("ui.dashboard.home.lead")}
			</p>
			<ul className="mt-6 grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-4">
				{SECTIONS.map(({ key, path, icon: Icon }) => (
					<li key={key}>
						<Link
							href={path}
							data-testid={`home-${key}`}
							className="flex h-full flex-col gap-2 rounded-xl border bg-card p-4 text-card-foreground hover:bg-muted"
						>
							<span className="flex items-center gap-2 font-medium">
								<Icon aria-hidden="true" className="size-4" />
								{t(`ui.nav.section.${key}`)}
							</span>
							<span className="text-sm text-muted-foreground">
								{t(`ui.dashboard.section.${key}.holds`)}
							</span>
						</Link>
					</li>
				))}
			</ul>
		</>
	);
}
