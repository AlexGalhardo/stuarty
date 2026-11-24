import { PenLine } from "lucide-react";
import type { ReactNode } from "react";

import { EmptyState } from "@/components/EmptyState";
import { ListRow } from "@/components/ListRow";
import { LoadingState } from "@/components/LoadingState";
import { Notice } from "@/components/Notice";
import UntrustedText from "@/components/UntrustedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import type { TextItem } from "./client";

export type Loaded =
	| { state: "loading" }
	| { state: "failed" }
	| { state: "ready"; texts: readonly TextItem[]; next?: string };

// The list of texts, with the four states a list has: loading, failed (with a way to try again), empty, and empty
// because of the filters. A title is text a person or an agent wrote, so it is drawn isolated and never as markup.
export default function TextList({
	loaded,
	currentId,
	filtered,
	onPick,
	onRetry,
	onMore,
}: {
	loaded: Loaded;
	currentId: string | null;
	filtered: boolean;
	onPick: (text: TextItem) => void;
	onRetry: () => void;
	onMore: () => void;
}): ReactNode {
	const t = useT();
	return (
		<nav
			aria-label={t("ui.write.list.label")}
			className="flex min-w-0 flex-col gap-2"
		>
			{loaded.state === "loading" && (
				<LoadingState message={t("ui.write.list.loading")} />
			)}
			{loaded.state === "failed" && (
				<Notice
					tone="danger"
					role="alert"
					title={t("ui.write.list.failed")}
					actions={
						<Button type="button" variant="outline" size="sm" onClick={onRetry}>
							{t("ui.write.list.retry")}
						</Button>
					}
				/>
			)}
			{loaded.state === "ready" && loaded.texts.length === 0 && (
				<EmptyState
					icon={PenLine}
					level={3}
					title={t(filtered ? "ui.write.list.no_match" : "ui.write.list.empty")}
					description={t(
						filtered
							? "ui.write.list.no_match_hint"
							: "ui.write.list.empty_hint",
					)}
				/>
			)}
			{loaded.state === "ready" && loaded.texts.length > 0 && (
				<ul className="flex flex-col divide-y divide-border rounded-lg border border-border bg-surface">
					{loaded.texts.map((text) => (
						<ListRow
							key={text.id}
							selected={currentId === text.id}
							title={
								<button
									type="button"
									data-text-id={text.id}
									onClick={() => onPick(text)}
									className="flex w-full min-w-0 items-center gap-2 text-left"
								>
									<span className="min-w-0 flex-1 truncate">
										<UntrustedText>{text.title}</UntrustedText>
									</span>
									<Badge variant="outline">
										{t(`ui.write.kind.${text.text_kind}`)}
									</Badge>
								</button>
							}
							description={t("ui.write.list.row", {
								revision: String(text.revision),
								words: String(text.words),
							})}
						/>
					))}
				</ul>
			)}
			{loaded.state === "ready" && loaded.next !== undefined && (
				<Button type="button" variant="outline" onClick={onMore}>
					{t("ui.write.list.more")}
				</Button>
			)}
		</nav>
	);
}
