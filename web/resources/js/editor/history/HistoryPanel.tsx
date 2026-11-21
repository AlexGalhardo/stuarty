import { History, Redo2, Undo2 } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Skeleton } from "@/components/ui/skeleton";
import { type Translate, useLocale, useT } from "@/lib/i18n";
import ErrorBody from "../commands/ErrorBody";
import { useCanEdit } from "../commands/useActions";
import { actorLabel } from "../state/activityStore";
import { useStore } from "../state/createStore";
import { useRuntime } from "../state/EditorContext";
import type { HistoryRow } from "../state/historyStore";
import EmptyState from "../ui/EmptyState";
import { errorHeadline } from "../ui/errorHeadline";
import { formatRelative } from "../ui/format";
import CheckpointsSection from "./CheckpointsSection";
import DiffView from "./DiffView";
import RevertDialog, { type RevertTarget } from "./RevertDialog";

const TICK_MS = 30_000;

// The line under the revision: the description of the event when the row came
// from one, else the operations of the step as the page of the history lists
// them.
export function rowText(t: Translate, row: HistoryRow): string {
	if (row.description !== undefined && row.description !== "")
		return row.description;
	if (row.ops !== undefined && row.ops.length > 0) return row.ops.join(", ");
	return t("ui.editor.history.changes", { count: row.changes ?? 0 });
}

function Chip({ actor, own }: { actor: string; own: boolean }): ReactNode {
	const t = useT();
	return (
		<Badge
			variant={own ? "secondary" : "outline"}
			className={own ? undefined : "bg-agent/10 text-agent"}
		>
			{own ? t("ui.editor.history.you") : actorLabel(actor)}
		</Badge>
	);
}

function Row({
	row,
	own,
	now,
	expanded,
	onToggle,
	picked,
	onPick,
	onRevert,
}: {
	row: HistoryRow;
	own: boolean;
	now: number;
	expanded: boolean;
	onToggle(): void;
	// The step is one of the two being compared; a step the core has not listed yet
	// (provisional) is not offered.
	picked: boolean;
	onPick(): void;
	// Absent on the newest step: going back to it changes nothing.
	onRevert?(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const text = rowText(t, row);
	const first = text.split("\n")[0] ?? "";
	const Icon = row.kind === "undo" ? Undo2 : row.kind === "redo" ? Redo2 : null;
	return (
		<li
			data-revision={row.revision}
			data-actor={row.actor}
			className="flex flex-wrap items-start gap-1"
		>
			{!row.provisional && (
				<Checkbox
					className="mt-2.5"
					checked={picked}
					aria-label={t("ui.editor.history.compare", {
						revision: row.revision,
					})}
					onCheckedChange={onPick}
				/>
			)}
			<button
				type="button"
				aria-expanded={expanded}
				className="grid min-w-36 flex-1 grid-cols-[auto_1fr_auto] items-center gap-x-2 gap-y-0.5 px-1 py-1.5 text-left hover:bg-accent"
				onClick={onToggle}
			>
				<span className="font-mono text-xs text-muted-foreground tabular-nums">
					{t("ui.editor.history.revision", { revision: row.revision })}
				</span>
				<span className="flex items-center gap-1">
					{Icon !== null && <Icon aria-hidden="true" className="size-3" />}
					<Chip actor={row.actor} own={own} />
				</span>
				<span className="text-xs text-muted-foreground tabular-nums">
					{formatRelative(row.at, now, locale)}
				</span>
				<span
					lang="en"
					data-undone={row.undone ? "true" : undefined}
					className={
						row.undone
							? "col-span-3 text-sm line-through"
							: "col-span-3 text-sm"
					}
				>
					{expanded ? (
						<span className="whitespace-pre-wrap">
							<UntrustedText>{text}</UntrustedText>
						</span>
					) : (
						<span className="block truncate">
							<UntrustedText>{first}</UntrustedText>
						</span>
					)}
				</span>
				{row.undone && (
					<Badge variant="outline" className="col-span-3 w-fit">
						{t("ui.editor.history.undone")}
					</Badge>
				)}
			</button>
			{onRevert !== undefined && !row.provisional && (
				<Button
					type="button"
					size="sm"
					variant="ghost"
					className="mt-1"
					aria-label={t("ui.editor.history.revert_row", {
						revision: row.revision,
					})}
					onClick={onRevert}
				>
					{t("ui.editor.history.revert_here")}
				</Button>
			)}
		</li>
	);
}

// The History tab of SDD-012 section 15 and docs/design/alpha-editor.md
// section 4.2: newest first, 50 rows per page with "Load more", the actor of
// each step, undone steps struck through. New rows come from the events; the
// refetch that follows replaces them with the page of the core.
export default function HistoryPanel(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const state = useStore(runtime.history, (s) => s);
	const [expanded, setExpanded] = useState<ReadonlySet<number>>(new Set());
	const [now, setNow] = useState(() => Date.now());
	const canEdit = useCanEdit();
	// The two steps picked to compare, oldest pick first; a third pick replaces
	// the oldest one.
	const [picked, setPicked] = useState<readonly number[]>([]);
	const [revertTarget, setRevertTarget] = useState<RevertTarget | null>(null);
	const head = state.rows[0]?.revision ?? 0;

	const pick = (revision: number): void =>
		setPicked((was) =>
			was.includes(revision)
				? was.filter((r) => r !== revision)
				: [...was, revision].slice(-2),
		);

	useEffect(() => {
		const id = setInterval(() => setNow(Date.now()), TICK_MS);
		return () => clearInterval(id);
	}, []);

	const toggle = (revision: number): void =>
		setExpanded((was) => {
			const next = new Set(was);
			if (!next.delete(revision)) next.add(revision);
			return next;
		});

	return (
		<section
			aria-label={t("ui.editor.history.title")}
			aria-busy={state.status === "loading"}
			className="flex min-h-0 min-w-0 flex-col gap-2 overflow-auto"
		>
			<CheckpointsSection head={head} onRevert={setRevertTarget} />
			{picked.length === 2 ? (
				<DiffView from={Math.min(...picked)} until={Math.max(...picked)} />
			) : (
				state.rows.length > 1 && (
					<p className="text-xs text-muted-foreground">
						{t("ui.editor.history.diff_hint")}
					</p>
				)
			)}
			<RevertDialog
				target={revertTarget}
				onClose={() => setRevertTarget(null)}
			/>
			{state.status === "loading" && (
				<div className="flex flex-col gap-1">
					<Skeleton className="h-12" />
					<Skeleton className="h-12" />
					<Skeleton className="h-12" />
					<Skeleton className="h-12" />
				</div>
			)}
			{state.status === "failed" && state.error !== null && (
				<div role="alert" className="flex flex-col items-start gap-2 text-sm">
					<p className="font-semibold">{errorHeadline(t, state.error.code)}</p>
					<ErrorBody error={state.error} />
					<Button
						type="button"
						size="sm"
						variant="outline"
						onClick={() => void runtime.historyControls.refresh()}
					>
						{t("ui.editor.history.retry")}
					</Button>
				</div>
			)}
			{state.status === "ready" && state.rows.length === 0 && (
				<EmptyState
					icon={History}
					title={t("ui.editor.history.empty")}
					hint={t("ui.editor.history.empty_hint")}
				/>
			)}
			{state.rows.length > 0 && (
				<ul className="flex flex-col divide-y divide-border">
					{state.rows.map((row) => (
						<Row
							key={row.revision}
							row={row}
							own={row.actor === runtime.ownActor}
							now={now}
							expanded={expanded.has(row.revision)}
							onToggle={() => toggle(row.revision)}
							picked={picked.includes(row.revision)}
							onPick={() => pick(row.revision)}
							onRevert={
								canEdit && row.revision !== head
									? () => setRevertTarget({ revision: row.revision })
									: undefined
							}
						/>
					))}
				</ul>
			)}
			{state.rows.length > 0 &&
				(state.nextCursor !== null ? (
					<Button
						type="button"
						variant="secondary"
						aria-busy={state.loadingMore}
						onClick={() => void runtime.historyControls.loadMore()}
					>
						{t("ui.editor.history.load_more")}
					</Button>
				) : (
					<p className="text-xs text-muted-foreground">
						{t("ui.editor.history.start")}
					</p>
				))}
		</section>
	);
}
