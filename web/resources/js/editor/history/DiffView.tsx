import { type ReactNode, useEffect, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Skeleton } from "@/components/ui/skeleton";
import { useT } from "@/lib/i18n";
import { ApiError } from "../api/errors";
import type { HistoryDiffResult } from "../api/types";
import ErrorBody from "../commands/ErrorBody";
import { useRuntime } from "../state/EditorContext";

type State =
	| { status: "loading" }
	| { status: "ready"; result: HistoryDiffResult }
	| { status: "failed"; error: ApiError | null };

// What changed between two steps of the history, in the words of the core, most
// significant first (RF-121). The view draws the lines it is given: it counts and
// classifies nothing.
export default function DiffView({
	from,
	until,
}: {
	from: number;
	until: number;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const [state, setState] = useState<State>({ status: "loading" });

	useEffect(() => {
		let live = true;
		setState({ status: "loading" });
		runtime.client
			.getHistoryDiff(runtime.projectId, { from, until })
			.then((result) => {
				if (live) setState({ status: "ready", result });
			})
			.catch((error: unknown) => {
				if (live)
					setState({
						status: "failed",
						error: error instanceof ApiError ? error : null,
					});
			});
		return () => {
			live = false;
		};
	}, [runtime, from, until]);

	return (
		<section
			aria-label={t("ui.editor.history.diff_title", { from, to: until })}
			className="flex flex-col gap-1 border-b border-border pb-2"
		>
			<h3 className="text-sm font-semibold">
				{t("ui.editor.history.diff_title", { from, to: until })}
			</h3>
			{state.status === "loading" && (
				<div role="status" className="flex flex-col gap-1">
					<Skeleton className="h-4" />
					<Skeleton className="h-4" />
				</div>
			)}
			{state.status === "failed" && (
				<div role="alert" className="flex flex-col gap-1 text-sm">
					{state.error === null ? (
						<p>{t("ui.editor.history.diff_failed")}</p>
					) : (
						<ErrorBody error={state.error} />
					)}
				</div>
			)}
			{state.status === "ready" && state.result.entries.length === 0 && (
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.history.diff_empty")}
				</p>
			)}
			{state.status === "ready" && state.result.entries.length > 0 && (
				<ul
					lang="en"
					data-testid="history-diff"
					className="max-h-48 list-none overflow-auto font-mono text-xs"
				>
					{state.result.entries.map((entry, i) => (
						// biome-ignore lint/suspicious/noArrayIndexKey: the lines of one diff never reorder
						<li key={i} data-verb={entry.verb}>
							<UntrustedText>{entry.line}</UntrustedText>
						</li>
					))}
				</ul>
			)}
		</section>
	);
}
