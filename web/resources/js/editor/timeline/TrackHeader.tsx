import {
	AudioLines,
	Eye,
	EyeOff,
	GripVertical,
	MoreHorizontal,
	Video,
	Volume2,
	VolumeX,
} from "lucide-react";
import { type ReactNode, useRef, useState } from "react";
import { createPortal } from "react-dom";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { useT } from "@/lib/i18n";
import { useActions, useCanEdit } from "../commands/useActions";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import { DEFAULT_ROWS, ROW_BOUNDS } from "../state/prefs";
import {
	selectOrderedTrackIds,
	selectTrack,
	selectTrackItemIds,
} from "../state/selectors";
import ConfirmTrackRemove from "../ui/ConfirmTrackRemove";
import Splitter from "../ui/Splitter";
import { beginReorder, type Ghost } from "./gestures/reorder";
import { useRowHeight } from "./TrackRow";

// The inline editor of the name: Enter or leaving the field sends
// `track.update`, Esc leaves it alone (SDD-012 section 7.4).
function RenameField({
	trackId,
	initial,
	onDone,
}: {
	trackId: string;
	initial: string;
	onDone(): void;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	const finished = useRef(false);
	const finish = (value: string | null): void => {
		if (finished.current) return;
		finished.current = true;
		if (value !== null && value !== initial)
			void actions.updateTrack(trackId, { name: value });
		onDone();
	};
	return (
		<Input
			autoFocus
			aria-label={t("ui.editor.track.name_label")}
			defaultValue={initial}
			maxLength={60}
			className="h-6 min-w-0 flex-1 px-1 text-xs"
			onFocus={(event) => event.currentTarget.select()}
			onPointerDown={(event) => event.stopPropagation()}
			onClick={(event) => event.stopPropagation()}
			onKeyDown={(event) => {
				event.stopPropagation();
				if (event.key === "Enter") finish(event.currentTarget.value);
				else if (event.key === "Escape") finish(null);
			}}
			onBlur={(event) => finish(event.currentTarget.value)}
		/>
	);
}

// The header of a track (SDD-012 section 7.4): a handle to reorder, the name
// (double click renames), hide and mute toggles and a menu. Every control sends
// one request and the header changes when the read model does.
export default function TrackHeader({ id }: { id: string }): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const actions = useActions();
	const canEdit = useCanEdit();
	const track = useDocumentState((s) => selectTrack(s, id));
	const kind = track?.kind === "audio" ? "audio" : "visual";
	const rowPx = useRowHeight(kind);
	const order = useDocumentState(selectOrderedTrackIds);
	const itemCount = useDocumentState((s) => selectTrackItemIds(s, id).length);
	const [renaming, setRenaming] = useState(false);
	const [confirming, setConfirming] = useState(false);
	const [ghost, setGhost] = useState<Ghost | null>(null);
	if (track === undefined) return null;
	const Icon = track.kind === "audio" ? AudioLines : Video;
	const name = track.name === undefined || track.name === "" ? id : track.name;
	const at = order.indexOf(id);
	const above = order[at - 1];
	const below = order[at + 1];
	const hidden = track.hidden === true;
	const muted = track.muted === true;
	const stop = (event: React.SyntheticEvent): void => event.stopPropagation();

	return (
		// biome-ignore lint/a11y/useSemanticElements: the header sits in a CSS-positioned grid (SDD-012 section 7.1).
		<div
			role="rowheader"
			tabIndex={-1}
			data-header-for={id}
			onClick={() => runtime.uiWriter.selectTrack(id)}
			onKeyDown={(event) => {
				if (event.key !== "Enter" || event.target !== event.currentTarget)
					return;
				event.preventDefault();
				runtime.uiWriter.selectTrack(id);
			}}
			style={{ height: rowPx }}
			className="ed-motion relative flex items-center gap-1 border-b border-ed-track-line px-1 text-xs"
		>
			<button
				type="button"
				data-track-handle={id}
				disabled={!canEdit}
				aria-label={t("ui.editor.track.drag", { name })}
				className="flex h-6 w-4 shrink-0 cursor-grab touch-none items-center justify-center text-muted-foreground disabled:cursor-default disabled:opacity-50"
				onClick={stop}
				onPointerDown={(event) => {
					if (event.button !== 0 || !canEdit) return;
					stop(event);
					beginReorder({
						event,
						trackId: id,
						onGhost: setGhost,
						send: (trackId, placement) =>
							void actions.moveTrack(trackId, placement),
					});
				}}
			>
				<GripVertical aria-hidden="true" className="size-4" />
			</button>
			<Icon
				aria-hidden="true"
				className="size-4 shrink-0 text-muted-foreground"
			/>
			<span className="sr-only">
				{track.kind === "audio"
					? t("ui.editor.timeline.track_audio")
					: t("ui.editor.timeline.track_visual")}
			</span>
			{renaming ? (
				<RenameField
					trackId={id}
					initial={track.name ?? ""}
					onDone={() => setRenaming(false)}
				/>
			) : (
				// biome-ignore lint/a11y/noStaticElementInteractions: the keyboard renames from the menu entry "Rename".
				<span
					className="min-w-0 flex-1 truncate font-medium"
					onDoubleClick={() => canEdit && setRenaming(true)}
				>
					<UntrustedText>{name}</UntrustedText>
				</span>
			)}
			{track.kind === "visual" && (
				<Button
					type="button"
					variant="ghost"
					size="icon-sm"
					className="size-6"
					disabled={!canEdit}
					aria-pressed={hidden}
					aria-label={t(
						hidden ? "ui.editor.track.show" : "ui.editor.track.hide",
					)}
					onClick={(event) => {
						stop(event);
						void actions.updateTrack(id, { hidden: !hidden });
					}}
				>
					{hidden ? <EyeOff aria-hidden="true" /> : <Eye aria-hidden="true" />}
				</Button>
			)}
			<Button
				type="button"
				variant="ghost"
				size="icon-sm"
				className="size-6"
				disabled={!canEdit}
				aria-pressed={muted}
				aria-label={t(
					muted ? "ui.editor.track.unmute" : "ui.editor.track.mute",
				)}
				onClick={(event) => {
					stop(event);
					void actions.updateTrack(id, { muted: !muted });
				}}
			>
				{muted ? (
					<VolumeX aria-hidden="true" />
				) : (
					<Volume2 aria-hidden="true" />
				)}
			</Button>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						type="button"
						variant="ghost"
						size="icon-sm"
						className="size-6"
						disabled={!canEdit}
						aria-label={t("ui.editor.track.menu", { name })}
						onClick={stop}
					>
						<MoreHorizontal aria-hidden="true" />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="start">
					<DropdownMenuItem onSelect={() => setRenaming(true)}>
						{t("ui.editor.track.rename")}
					</DropdownMenuItem>
					<DropdownMenuSeparator />
					<DropdownMenuItem
						onSelect={() => void actions.addTrack("visual", { above: id })}
					>
						{t("ui.editor.track.add_visual_above")}
					</DropdownMenuItem>
					<DropdownMenuItem
						onSelect={() => void actions.addTrack("visual", { below: id })}
					>
						{t("ui.editor.track.add_visual_below")}
					</DropdownMenuItem>
					<DropdownMenuItem
						onSelect={() => void actions.addTrack("audio", { above: id })}
					>
						{t("ui.editor.track.add_audio_above")}
					</DropdownMenuItem>
					<DropdownMenuItem
						onSelect={() => void actions.addTrack("audio", { below: id })}
					>
						{t("ui.editor.track.add_audio_below")}
					</DropdownMenuItem>
					<DropdownMenuSeparator />
					<DropdownMenuItem
						disabled={above === undefined}
						onSelect={() =>
							above !== undefined && void actions.moveTrack(id, { above })
						}
					>
						{t("ui.editor.track.move_up")}
					</DropdownMenuItem>
					<DropdownMenuItem
						disabled={below === undefined}
						onSelect={() =>
							below !== undefined && void actions.moveTrack(id, { below })
						}
					>
						{t("ui.editor.track.move_down")}
					</DropdownMenuItem>
					<DropdownMenuSeparator />
					<DropdownMenuItem
						variant="destructive"
						onSelect={async () => {
							const outcome = await actions.removeTrack(id, false);
							if (
								outcome !== null &&
								!outcome.ok &&
								outcome.error.code === "E_NOT_EMPTY"
							) {
								setConfirming(true);
							}
						}}
					>
						{t("ui.editor.track.remove")}
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
			<ConfirmTrackRemove
				count={itemCount}
				open={confirming}
				onOpenChange={setConfirming}
				onConfirm={() => void actions.removeTrack(id, true)}
			/>
			<Splitter
				label={t(
					kind === "audio"
						? "ui.editor.track.row_height_audio"
						: "ui.editor.track.row_height_visual",
				)}
				value={rowPx}
				min={ROW_BOUNDS.min}
				max={ROW_BOUNDS.max}
				defaultValue={DEFAULT_ROWS[kind]}
				onChange={(px) => runtime.uiWriter.setRowPx(kind, px)}
			/>
			{ghost !== null &&
				createPortal(
					<div
						aria-hidden="true"
						data-testid="track-ghost"
						className="pointer-events-none fixed z-50 rounded-md border-2 border-ed-handle bg-ed-ghost"
						style={ghost}
					/>,
					document.body,
				)}
		</div>
	);
}
