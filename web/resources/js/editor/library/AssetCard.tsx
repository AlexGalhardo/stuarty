import { ImageOff, MoreHorizontal, Music, Plus } from "lucide-react";
import { type ReactNode, useState } from "react";
import RenameAssetDialog from "@/components/RenameAssetDialog";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuTrigger,
} from "@/components/ui/context-menu";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import type { MediaEntry } from "../api/types";
import { thumbnailUrl } from "../api/urls";
import ErrorBody from "../commands/ErrorBody";
import { formatSeconds } from "../commands/units";
import { useActions, useCanEdit } from "../commands/useActions";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import { readTimeDisplay } from "../state/prefs";
import { errorHeadline } from "../ui/errorHeadline";
import AssetDetails from "./AssetDetails";
import TrackChooser from "./TrackChooser";
import { hasThumbnail } from "./thumbnail";

// One card of the library (SDD-012 section 13.2). "Add to timeline" is enabled
// only when the asset is ready and already in the read model, because the
// asset.updated event and the asset.add batch are not ordered.
export type AssetDragStart = (
	event: React.PointerEvent<HTMLElement>,
	entry: MediaEntry,
) => void;

export default function AssetCard({
	entry,
	onDragStart,
}: {
	entry: MediaEntry;
	onDragStart?: AssetDragStart;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const actions = useActions();
	const inDocument = useDocumentState(
		(s) => s.document?.assets[entry.asset_id] !== undefined,
	);
	const canEdit = useCanEdit();
	const canPlace = canEdit && entry.state === "ready" && inDocument;
	const trashed = entry.trashed;
	const draggable = canPlace && !trashed && onDragStart !== undefined;
	const [inspecting, setInspecting] = useState(false);
	const [renaming, setRenaming] = useState(false);
	// The hash of the file in the document: the library renames by it (SDD-011 section 14.8).
	const hash = useDocumentState(
		(s) => s.document?.assets[entry.asset_id]?.hash,
	);
	const canRename = canEdit && hash !== undefined && entry.state === "ready";
	const path = runtime.library.getState().paths[entry.name];
	const percent = Math.round(entry.progress * 100);

	return (
		<ContextMenu>
			<ContextMenuTrigger asChild>
				<li
					data-asset-id={entry.asset_id}
					data-state={entry.state}
					data-draggable={draggable}
					className={cn(
						"flex flex-col gap-2 rounded-md border border-ed-track-line p-2",
						draggable && "cursor-grab touch-none",
					)}
					onPointerDown={(event) => {
						if (
							!draggable ||
							event.button !== 0 ||
							(event.target as HTMLElement).closest("button, [role='menu']") !==
								null
						) {
							return;
						}
						onDragStart?.(event, entry);
					}}
				>
					<div className="flex gap-2">
						<div className="flex size-14 shrink-0 items-center justify-center overflow-hidden rounded bg-muted">
							{hasThumbnail(entry) ? (
								<img
									src={thumbnailUrl(runtime.projectId, entry.asset_id, 160)}
									alt=""
									draggable={false}
									className="size-full object-cover"
								/>
							) : entry.kind === "audio" ? (
								<Music
									aria-hidden="true"
									className="size-5 text-muted-foreground"
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
									`${formatSeconds(entry.duration_seconds, readTimeDisplay())} · `}
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
							<div
								className="h-full bg-primary"
								style={{ width: `${percent}%` }}
							/>
						</div>
					)}
					{entry.state === "failed" && entry.error !== undefined && (
						<div role="alert" className="text-xs text-destructive">
							<p className="font-medium">
								{errorHeadline(t, entry.error.code)}
							</p>
							<ErrorBody error={entry.error} />
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
					<div className="flex gap-1">
						{!trashed && (
							<Button
								type="button"
								size="sm"
								variant="outline"
								className="flex-1"
								disabled={!canPlace}
								onClick={() => void actions.addToTimeline(entry.asset_id)}
							>
								<Plus aria-hidden="true" />
								{t("ui.editor.library.add")}
							</Button>
						)}
						{!trashed && (
							<TrackChooser
								assetId={entry.asset_id}
								name={entry.name}
								kind={entry.kind === "audio" ? "audio" : "visual"}
								disabled={!canPlace}
							/>
						)}
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									type="button"
									size="icon-sm"
									variant="ghost"
									aria-label={t("ui.editor.library.menu", { name: entry.name })}
								>
									<MoreHorizontal aria-hidden="true" />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								<DropdownMenuItem
									disabled={entry.state !== "ready"}
									onSelect={() => setInspecting(true)}
								>
									{t("ui.editor.library.inspect")}
								</DropdownMenuItem>
								{!trashed && (
									<DropdownMenuItem
										disabled={!canRename}
										onSelect={() => setRenaming(true)}
									>
										{t("ui.editor.library.rename.action")}
									</DropdownMenuItem>
								)}
								{trashed ? (
									<DropdownMenuItem
										disabled={!canEdit}
										onSelect={() =>
											void runtime.setAssetTrashed(entry.asset_id, false)
										}
									>
										{t("ui.editor.library.restore")}
									</DropdownMenuItem>
								) : (
									<>
										<DropdownMenuItem
											disabled={!canEdit || !inDocument}
											onSelect={() => void actions.removeAsset(entry.asset_id)}
										>
											{t("ui.editor.library.remove")}
										</DropdownMenuItem>
										<DropdownMenuItem
											disabled={!canEdit}
											onSelect={() =>
												void runtime.setAssetTrashed(entry.asset_id, true)
											}
										>
											{t("ui.editor.library.trash")}
										</DropdownMenuItem>
									</>
								)}
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
					<AssetDetails
						assetId={entry.asset_id}
						name={entry.name}
						open={inspecting}
						onOpenChange={setInspecting}
					/>
					<RenameAssetDialog
						open={renaming}
						onOpenChange={setRenaming}
						name={entry.name}
						rename={(name) =>
							hash === undefined
								? Promise.resolve()
								: runtime.renameAsset(hash, name)
						}
					/>
				</li>
			</ContextMenuTrigger>
			<ContextMenuContent aria-label={t("ui.editor.library.rename.menu")}>
				<ContextMenuItem
					disabled={!canRename}
					onSelect={() => setRenaming(true)}
				>
					{t("ui.editor.library.rename.action")}
				</ContextMenuItem>
			</ContextMenuContent>
		</ContextMenu>
	);
}
