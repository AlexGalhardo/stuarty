import { FileImage, Film, Palette } from "lucide-react";
import type { ComponentType, ReactNode } from "react";

import RenameMenu from "@/components/RenameMenu";
import UntrustedText from "@/components/UntrustedText";
import { cn } from "@/lib/cn";
import { sizeText } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { createClient } from "../editor/api/client";
import { HttpTransport } from "../editor/api/http";
import type { LibraryAsset } from "../editor/api/types";
import { thumbnailUrl } from "../editor/api/urls";
import { formatSeconds } from "../editor/ui/timeFormat";
import { OriginBadge, ReusableBadge, TagChip } from "./badges";

const POSTER_WIDTH = 320;

const KIND_ICON: Record<string, ComponentType<{ className?: string }>> = {
	image: FileImage,
	video: Film,
	lut: Palette,
};

function hasPicture(asset: LibraryAsset): boolean {
	return (
		asset.poster !== undefined &&
		(asset.kind === "image" || asset.kind === "video")
	);
}

// One file of the video and visual libraries: its picture, the name, where it came from, whether it is
// reusable, its size and its tags. Everything is what the service listed; the card decides nothing. Its
// poster stays a plain button so the file opens with Enter, and the rename menu is the one of epic 60.
export function MediaCard({
	asset,
	selected,
	onOpen,
	onRenamed,
}: {
	asset: LibraryAsset;
	selected: boolean;
	onOpen(): void;
	// The file was renamed: the page reads the list again, which the service orders by name.
	onRenamed?(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const Fallback = KIND_ICON[asset.kind] ?? FileImage;
	return (
		<RenameMenu
			name={asset.name}
			rename={async (name) => {
				await createClient(new HttpTransport("/api/v1")).renameAsset(
					asset.hash,
					name,
				);
				onRenamed?.();
			}}
			returnFocus={() =>
				document.querySelector<HTMLElement>(
					`button[data-asset-hash="${CSS.escape(asset.hash)}"]`,
				)
			}
		>
			<li>
				<button
					type="button"
					onClick={onOpen}
					aria-pressed={selected}
					data-asset-hash={asset.hash}
					className={cn(
						"flex w-full flex-col gap-2 rounded-lg border border-border bg-surface p-2 text-left text-text transition-colors hover:bg-surface-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus",
						"aria-pressed:border-accent aria-pressed:bg-accent-soft",
					)}
				>
					<span className="relative flex aspect-video items-center justify-center overflow-hidden rounded-md bg-surface-hover text-sm text-text-muted">
						{hasPicture(asset) && asset.poster !== undefined ? (
							<img
								src={thumbnailUrl(
									asset.poster.timeline_id,
									asset.poster.asset_id,
									POSTER_WIDTH,
								)}
								alt=""
								loading="lazy"
								className="size-full object-cover"
							/>
						) : (
							<span className="flex flex-col items-center gap-1">
								<Fallback aria-hidden="true" className="size-6" />
								{t(`ui.assets.kind.${asset.kind}`)}
							</span>
						)}
						{asset.duration_ms !== undefined && (
							<span
								data-testid="media-duration"
								className="absolute bottom-1 left-1 rounded border border-border bg-surface px-1.5 py-0.5 font-mono text-xs text-text tabular-nums"
							>
								{formatSeconds(asset.duration_ms / 1000, "timecode")}
							</span>
						)}
					</span>
					<span className="truncate text-sm font-medium">
						<UntrustedText>{asset.name}</UntrustedText>
					</span>
					<span className="flex flex-wrap items-center gap-1">
						<OriginBadge kind={asset.origin?.kind} />
						{asset.reusable === true && <ReusableBadge />}
						<span className="ml-auto text-xs text-text-muted">
							{sizeText(asset.bytes, locale)}
						</span>
					</span>
					{(asset.tags ?? []).length > 0 && (
						<span className="flex flex-wrap gap-1">
							{(asset.tags ?? []).map((tag) => (
								<TagChip key={tag} tag={tag} />
							))}
						</span>
					)}
				</button>
			</li>
		</RenameMenu>
	);
}
