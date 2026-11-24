import { Head } from "@inertiajs/react";
import type { ReactNode } from "react";

import { PageHeader } from "@/components/PageHeader";
import { useT } from "@/lib/i18n";

// The frame the three libraries share (Video, Visual and Sound Assets): the window title, one h1 with its
// sentence and the page action, then whatever the library draws. It holds no data and no copy of its own.
// Nothing here assumes that a project has media (D-134): the words say a library is where files are kept.
export function LibraryFrame({
	navKey,
	description,
	actions,
	children,
}: {
	navKey: string;
	description: string;
	actions?: ReactNode;
	children: ReactNode;
}): ReactNode {
	const t = useT();
	const title = t(`ui.nav.section.${navKey}`);
	return (
		<>
			<Head title={title} />
			<div
				data-slot="asset-library"
				className="mx-auto flex max-w-6xl flex-col gap-6"
			>
				<PageHeader title={title} description={description} actions={actions} />
				{children}
			</div>
		</>
	);
}
