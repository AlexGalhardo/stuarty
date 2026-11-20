import { type ReactNode, useEffect, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useT } from "@/lib/i18n";
import { ApiError } from "../../editor/api/errors";
import ErrorBody from "../../editor/commands/ErrorBody";
import type { ImageDiffResult } from "../api/types";
import { useImage, useImageApi } from "../state/context";

type State =
	| { status: "loading" }
	| { status: "ready"; result: ImageDiffResult }
	| { status: "failed"; error: ApiError | null };

// What changed between a revision and the one on screen, in the words of the core, most significant first. The
// view draws the lines it is given: it counts and classifies nothing.
export default function DiffView({
	from,
	onClose,
}: {
	from: number;
	onClose(): void;
}): ReactNode {
	const t = useT();
	const api = useImageApi();
	const imageId = useImage((s) => s.imageId);
	const revision = useImage((s) => s.doc?.revision ?? 0);
	const [state, setState] = useState<State>({ status: "loading" });

	// biome-ignore lint/correctness/useExhaustiveDependencies: `revision` makes a new step read the diff again
	useEffect(() => {
		let live = true;
		setState({ status: "loading" });
		api
			.diff(imageId, from)
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
	}, [api, imageId, from, revision]);

	const title = t("ui.image.history.diff_title", {
		from,
		to: state.status === "ready" ? state.result.to : revision,
	});

	return (
		<section
			aria-label={title}
			data-testid="image-diff"
			className="flex flex-col gap-1 border-b border-border px-3 py-2"
		>
			<div className="flex items-center gap-2">
				<h3 className="flex-1 text-sm font-semibold">{title}</h3>
				<Button type="button" size="sm" variant="ghost" onClick={onClose}>
					{t("ui.image.history.diff_close")}
				</Button>
			</div>
			{state.status === "loading" && (
				<div role="status" className="flex flex-col gap-1">
					<Skeleton className="h-4" />
					<Skeleton className="h-4" />
				</div>
			)}
			{state.status === "failed" && (
				<div role="alert" className="flex flex-col gap-1 text-sm">
					{state.error === null ? (
						<p>{t("ui.image.history.diff_failed")}</p>
					) : (
						<ErrorBody error={state.error} />
					)}
				</div>
			)}
			{state.status === "ready" && state.result.entries.length === 0 && (
				<p className="text-xs text-muted-foreground">
					{t("ui.image.history.diff_empty")}
				</p>
			)}
			{state.status === "ready" && state.result.entries.length > 0 && (
				<ul
					lang="en"
					data-testid="image-diff-lines"
					className="max-h-40 list-none overflow-auto font-mono text-xs"
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
