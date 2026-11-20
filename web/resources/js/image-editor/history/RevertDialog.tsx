import { type ReactNode, useEffect, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import {
	AlertDialog,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useT } from "@/lib/i18n";
import { ApiError } from "../../editor/api/errors";
import ErrorBody from "../../editor/commands/ErrorBody";
import type { ImageRevertResult } from "../api/types";
import { useImage, useImageApi, useImageStore } from "../state/context";

// Where a revert goes back to: a revision of the history, or a named version.
export type RevertTarget = { revision: number } | { checkpoint: string };

type Preview =
	| { status: "loading" }
	| { status: "ready"; result: ImageRevertResult }
	| { status: "failed"; error: ApiError };

// "Revert to here" with the dry run as the confirmation (SDD-012 section 24, epic 50): the core says what would
// change, in words, before anything is written. The revert is a new step that the person can undo, held to the
// revision the preview was made on, so a change of another actor in between is a conflict and not lost.
export default function RevertDialog({
	target,
	onClose,
}: {
	target: RevertTarget | null;
	onClose(): void;
}): ReactNode {
	const t = useT();
	const api = useImageApi();
	const store = useImageStore();
	const imageId = useImage((s) => s.imageId);
	const [preview, setPreview] = useState<Preview>({ status: "loading" });
	const [busy, setBusy] = useState(false);
	const [failure, setFailure] = useState<ApiError | null>(null);

	useEffect(() => {
		if (target === null) return;
		let live = true;
		setPreview({ status: "loading" });
		setFailure(null);
		api
			.revert(imageId, { ...target, dry_run: true })
			.then((result) => {
				if (live) setPreview({ status: "ready", result });
			})
			.catch((error: unknown) => {
				if (live && error instanceof ApiError)
					setPreview({ status: "failed", error });
			});
		return () => {
			live = false;
		};
	}, [target, api, imageId]);

	const confirm = (): void => {
		if (target === null || preview.status !== "ready") return;
		setBusy(true);
		setFailure(null);
		api
			.revert(imageId, {
				...target,
				base_revision: preview.result.base_revision,
			})
			.then(async () => {
				await store.getState().reload();
				setBusy(false);
				onClose();
			})
			.catch((error: unknown) => {
				setBusy(false);
				if (error instanceof ApiError) setFailure(error);
			});
	};

	const same = preview.status === "ready" && preview.result.same;
	const title =
		target === null
			? ""
			: "checkpoint" in target
				? t("ui.image.history.revert_title_named", { name: target.checkpoint })
				: t("ui.image.history.revert_title", { revision: target.revision });
	const lines =
		preview.status === "ready" && preview.result.description !== ""
			? preview.result.description.split("\n")
			: [];

	return (
		<AlertDialog
			open={target !== null}
			onOpenChange={(open) => {
				if (!open && !busy) onClose();
			}}
		>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogTitle>
						<UntrustedText>{title}</UntrustedText>
					</AlertDialogTitle>
					<AlertDialogDescription>
						{t("ui.image.history.revert_body")}
					</AlertDialogDescription>
				</AlertDialogHeader>
				<div aria-live="polite" className="flex flex-col gap-2 text-sm">
					{preview.status === "loading" && (
						<div role="status" className="flex flex-col gap-1">
							<span className="sr-only">
								{t("ui.image.history.revert_loading")}
							</span>
							<Skeleton className="h-4" />
							<Skeleton className="h-4" />
						</div>
					)}
					{preview.status === "failed" && (
						<div role="alert" className="flex flex-col gap-1">
							<ErrorBody error={preview.error} />
						</div>
					)}
					{same && <p>{t("ui.image.history.revert_same")}</p>}
					{preview.status === "ready" && !same && (
						<>
							<p className="font-medium">
								{t("ui.image.history.revert_changes")}
							</p>
							<ul
								lang="en"
								data-testid="image-revert-preview"
								className="max-h-48 list-none overflow-auto rounded-md border border-border p-2 font-mono text-xs"
							>
								{lines.map((line, i) => (
									// biome-ignore lint/suspicious/noArrayIndexKey: lines of one preview never reorder
									<li key={i}>
										<UntrustedText>{line}</UntrustedText>
									</li>
								))}
							</ul>
						</>
					)}
					{failure !== null && (
						<div role="alert" className="flex flex-col gap-1">
							<ErrorBody error={failure} />
						</div>
					)}
				</div>
				<AlertDialogFooter>
					<AlertDialogCancel disabled={busy}>
						{t("ui.image.history.revert_cancel")}
					</AlertDialogCancel>
					<Button
						type="button"
						variant="destructive"
						data-testid="image-revert-confirm"
						aria-busy={busy}
						disabled={busy || preview.status !== "ready" || same}
						onClick={confirm}
					>
						{t("ui.image.history.revert_confirm")}
					</Button>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	);
}
