import { Head, Link } from "@inertiajs/react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { isSectionKey, SECTIONS } from "@/dashboard/sections";
import { useT } from "@/lib/i18n";

// The page of a section whose epic is not done yet (SDD-012 section 23.3): it
// says what the section will hold and where to go meanwhile, never a blank
// page. The controller sends only the key of the section.
export default function Section({ section }: { section: string }): ReactNode {
	const t = useT();
	const known = isSectionKey(section) ? section : null;
	const entry = SECTIONS.find((s) => s.key === known);
	if (known === null || entry === undefined) return null;
	const Icon = entry.icon;
	const title = t(`ui.nav.section.${known}`);

	return (
		<>
			<Head title={title} />
			<section
				aria-labelledby="section-title"
				className="mx-auto flex max-w-xl flex-col items-center gap-3 py-16 text-center"
				data-testid={`empty-${known}`}
			>
				<span className="flex size-12 items-center justify-center rounded-xl bg-muted text-muted-foreground">
					<Icon aria-hidden="true" className="size-6" />
				</span>
				<h1
					id="section-title"
					className="text-2xl font-semibold tracking-tight"
				>
					{title}
				</h1>
				<p>{t(`ui.dashboard.section.${known}.holds`)}</p>
				<p className="text-sm text-muted-foreground">
					{known === "edit-video"
						? t("ui.dashboard.empty.via_projects")
						: t("ui.dashboard.empty.status")}
				</p>
				<Button asChild variant="outline">
					<Link href="/projects">{t("ui.dashboard.empty.goto")}</Link>
				</Button>
			</section>
		</>
	);
}
