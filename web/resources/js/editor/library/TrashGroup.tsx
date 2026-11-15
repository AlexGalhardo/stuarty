import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import type { MediaEntry } from "../api/types";
import AssetCard from "./AssetCard";

// Trashed assets sit in a collapsed group, each with "Restore" in its menu
// (SDD-012 section 13.2).
export default function TrashGroup({
	entries,
}: {
	entries: readonly MediaEntry[];
}): ReactNode {
	const t = useT();
	if (entries.length === 0) return null;
	return (
		<details data-testid="trash-group" className="flex flex-col gap-2">
			<summary className="cursor-pointer text-xs font-medium text-muted-foreground">
				{t("ui.editor.library.trash_group", { count: entries.length })}
			</summary>
			<ul className="mt-2 flex flex-col gap-2">
				{entries.map((entry) => (
					<AssetCard key={entry.asset_id} entry={entry} />
				))}
			</ul>
		</details>
	);
}
