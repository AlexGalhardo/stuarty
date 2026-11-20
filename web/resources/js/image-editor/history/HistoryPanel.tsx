import { type ReactNode, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import { useLocale, useT } from "@/lib/i18n";
import { formatRelative } from "../../editor/ui/format";
import type { ImageHistoryEntry } from "../api/types";
import { useImage } from "../state/context";
import CheckpointsSection from "./CheckpointsSection";
import DiffView from "./DiffView";
import RevertDialog, { type RevertTarget } from "./RevertDialog";

// The recent operations of the document with who made them (SDD-012 section 24.9): the actor is a text label
// ("You" or the name of the key or agent), never a colour alone; an undone step says so in words. Since epic 50
// the panel also names versions, shows what changed since a revision and goes back to one, with the dry run of
// the core as the confirmation.
export default function HistoryPanel(): ReactNode {
	const t = useT();
	const history = useImage((s) => s.history);
	const head = useImage((s) => s.doc?.revision ?? 0);
	const [target, setTarget] = useState<RevertTarget | null>(null);
	const [compare, setCompare] = useState<number | null>(null);

	return (
		<section
			aria-labelledby="image-history-title"
			data-testid="image-history"
			className="flex h-72 shrink-0 flex-col border-t"
		>
			<h2
				id="image-history-title"
				className="border-b px-3 py-2 text-sm font-semibold"
			>
				{t("ui.image.history.title")}
			</h2>
			<div className="min-h-0 flex-1 overflow-y-auto">
				<CheckpointsSection onRevert={setTarget} />
				{compare !== null && (
					<DiffView from={compare} onClose={() => setCompare(null)} />
				)}
				{history.length === 0 ? (
					<p className="p-3 text-sm text-muted-foreground">
						{t("ui.image.history.empty")}
					</p>
				) : (
					<ol>
						{history.map((entry) => (
							<Entry
								key={entry.revision}
								entry={entry}
								current={entry.revision === head}
								onRevert={() => setTarget({ revision: entry.revision })}
								onCompare={() => setCompare(entry.revision)}
							/>
						))}
					</ol>
				)}
			</div>
			<RevertDialog target={target} onClose={() => setTarget(null)} />
		</section>
	);
}

// The line under the revision: the description of the core, as received. Only an edit row whose operations the core
// no longer knows has none; it shows the names of those operations (SDD-012 section 24.9).
export function entryText(entry: ImageHistoryEntry): string {
	if (entry.description !== undefined && entry.description !== "")
		return entry.description;
	return entry.ops.join(", ");
}

function Entry({
	entry,
	current,
	onRevert,
	onCompare,
}: {
	entry: ImageHistoryEntry;
	// The newest revision: going back to it changes nothing, and nothing has changed since it.
	current: boolean;
	onRevert(): void;
	onCompare(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const undone = entry.undone_by !== undefined;
	const what = entryText(entry);

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
			{!current && (
				<span className="flex gap-1">
					<Button
						type="button"
						size="sm"
						variant="ghost"
						data-testid="image-history-compare"
						aria-label={t("ui.image.history.compare", {
							revision: entry.revision,
						})}
						onClick={onCompare}
					>
						{t("ui.image.history.compare", { revision: entry.revision })}
					</Button>
					<Button
						type="button"
						size="sm"
						variant="ghost"
						data-testid="image-history-revert"
						aria-label={t("ui.image.history.revert_row", {
							revision: entry.revision,
						})}
						onClick={onRevert}
					>
						{t("ui.image.history.revert_here")}
					</Button>
				</span>
			)}
		</li>
	);
}
