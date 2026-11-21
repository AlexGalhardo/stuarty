import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";

// The reasons a field was refused, written under it (SDD-012 section 2.1.1). The reason is the code the server
// sent; the wording is the catalogue's, so the page computes nothing about the rule.
export default function FieldErrors({
	id,
	reasons,
}: {
	id: string;
	reasons: readonly string[];
}): ReactNode {
	const t = useT();
	if (reasons.length === 0) return null;
	return (
		<ul id={id} className="flex flex-col gap-0.5 text-sm text-destructive">
			{reasons.map((reason) => (
				<li key={reason}>{t(`ui.account.reason.${reason}`)}</li>
			))}
		</ul>
	);
}
