import { ImageOff, Plus } from "lucide-react";
import type { ReactNode } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { useLocale, useT } from "@/lib/i18n";
import type { MediaEntry } from "../api/types";
import { thumbnailUrl } from "../api/urls";
import { useActions, useCanEdit } from "../commands/useActions";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import { errorHeadline } from "../ui/errorHeadline";
import { formatSeconds } from "../ui/format";

// One card of the library (SDD-012 section 13.2). "Add to timeline" is enabled
// only when the asset is ready and already in the read model, because the
// asset.updated event and the asset.add batch are not ordered.
export default function AssetCard({ entry }: { entry: MediaEntry }): ReactNode {
	const t = useT();
	const locale = useLocale();
	const runtime = useRuntime();
	const actions = useActions();
	const inDocument = useDocumentState(
		(s) => s.document?.assets[entry.asset_id] !== undefined,
	);
	const canEdit = useCanEdit();
	const canPlace =
		canEdit && entry.state === "ready" && inDocument && entry.kind !== "audio";
	const path = runtime.library.getState().paths[entry.name];
	const percent = Math.round(entry.progress * 100);

	return (
		<li
			className="flex flex-col gap-2 rounded-md border border-ed-track-line p-2"
			data-asset-id={entry.asset_id}
			data-state={entry.state}
		>
			<div className="flex gap-2">
				<div className="flex size-14 shrink-0 items-center justify-center overflow-hidden rounded bg-muted">
					{entry.state === "ready" ? (
						<img
							src={thumbnailUrl(runtime.projectId, entry.asset_id, 160)}
							alt=""
							className="size-full object-cover"
						/>
					) : (
						<ImageOff
							aria-hidden="true"
							className="size-4 text-muted-foreground"
						/>
					)}
				</div>
				<div className="min-w-0 flex-1 text-xs">
					<p className="truncate font-medium">
						<UntrustedText>{entry.name}</UntrustedText>
					</p>
					<p className="text-muted-foreground [font-variant-numeric:tabular-nums]">
						{entry.duration_seconds !== null &&
							`${formatSeconds(entry.duration_seconds, locale)} · `}
						{entry.width !== undefined &&
							entry.height !== undefined &&
							`${entry.width}×${entry.height} · `}
						{t(`ui.editor.library.state.${entry.state}`)}
					</p>
				</div>
			</div>
			{entry.state === "ingesting" && (
				<div
					role="progressbar"
					aria-label={t("ui.editor.library.progress")}
					aria-valuemin={0}
					aria-valuemax={100}
					aria-valuenow={percent}
					className="h-1 w-full overflow-hidden rounded-full bg-muted"
				>
					<div className="h-full bg-primary" style={{ width: `${percent}%` }} />
				</div>
			)}
			{entry.state === "failed" && entry.error !== undefined && (
				<div role="alert" className="text-xs text-destructive">
					<p className="font-medium">{errorHeadline(t, entry.error.code)}</p>
					<p lang="en">
						<UntrustedText>{entry.error.message}</UntrustedText>
					</p>
					{entry.error.hint !== undefined && (
						<p lang="en" className="text-muted-foreground">
							<UntrustedText>{entry.error.hint}</UntrustedText>
						</p>
					)}
					<div className="mt-1 flex gap-1">
						{path !== undefined && (
							<Button
								type="button"
								size="sm"
								variant="outline"
								onClick={() => void runtime.importPaths([path])}
							>
								{t("ui.editor.library.retry")}
							</Button>
						)}
						<Button
							type="button"
							size="sm"
							variant="ghost"
							onClick={() => runtime.libraryWriter.dismiss(entry.asset_id)}
						>
							{t("ui.editor.library.dismiss")}
						</Button>
					</div>
				</div>
			)}
			<Button
				type="button"
				size="sm"
				variant="outline"
				disabled={!canPlace}
				onClick={() => void actions.addToTimeline(entry.asset_id)}
			>
				<Plus aria-hidden="true" />
				{t("ui.editor.library.add")}
			</Button>
		</li>
	);
}
