import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { cn } from "@/lib/cn";
import { useLocale, useT } from "@/lib/i18n";
import { formatRelative } from "../../editor/ui/format";
import type { ImageHistoryEntry } from "../api/types";
import { useImage } from "../state/context";

// The recent operations of the document with who made them (SDD-012 section 24.9): the actor is a text label
// ("You" or the name of the key or agent), never a colour alone; an undone step says so in words.
export default function HistoryPanel(): ReactNode {
	const t = useT();
	const history = useImage((s) => s.history);

	return (
		<section
			aria-labelledby="image-history-title"
			data-testid="image-history"
			className="flex h-52 shrink-0 flex-col border-t"
		>
			<h2
				id="image-history-title"
				className="border-b px-3 py-2 text-sm font-semibold"
			>
				{t("ui.image.history.title")}
			</h2>
			{history.length === 0 ? (
				<p className="p-3 text-sm text-muted-foreground">
					{t("ui.image.history.empty")}
				</p>
			) : (
				<ol className="min-h-0 flex-1 overflow-y-auto">
					{history.map((entry) => (
						<Entry key={entry.revision} entry={entry} />
					))}
				</ol>
			)}
		</section>
	);
}

function Entry({ entry }: { entry: ImageHistoryEntry }): ReactNode {
	const t = useT();
	const locale = useLocale();
	const undone = entry.undone_by !== undefined;
	const what =
		entry.kind === "undo"
			? t("ui.image.history.undo_of", { revision: entry.undo_of ?? 0 })
			: entry.kind === "redo"
				? t("ui.image.history.redo_of", { revision: entry.undo_of ?? 0 })
				: entry.ops.join(", ");

	return (
		<li
			data-testid="image-history-entry"
			data-actor={entry.actor}
			className={cn(
				"flex flex-col gap-0.5 border-b border-l-4 px-3 py-1.5 text-xs",
				entry.mine ? "border-l-ed-handle" : "border-l-agent",
			)}
		>
			<span className="flex items-center gap-2">
				<span className="tabular-nums text-muted-foreground">
					{t("ui.image.history.revision", { revision: entry.revision })}
				</span>
				<span className="font-medium" data-testid="image-history-actor">
					{entry.mine ? (
						t("ui.image.history.you")
					) : (
						<UntrustedText>{entry.actor}</UntrustedText>
					)}
				</span>
				<span className="ml-auto text-muted-foreground">
					{formatRelative(entry.at, Date.now(), locale)}
				</span>
			</span>
			<span className={cn("break-words font-mono", undone && "line-through")}>
				{what}
			</span>
			{undone && (
				<span className="text-muted-foreground">
					{t("ui.image.history.undone", { revision: entry.undone_by ?? 0 })}
				</span>
			)}
			{entry.skipped !== undefined && entry.skipped > 0 && (
				<span className="text-muted-foreground">
					{t("ui.image.history.skipped", { count: entry.skipped })}
				</span>
			)}
		</li>
	);
}
