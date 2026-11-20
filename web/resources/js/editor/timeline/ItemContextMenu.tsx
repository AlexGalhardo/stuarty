import {
	Blend,
	ClipboardPaste,
	Copy,
	CopyPlus,
	Eye,
	EyeOff,
	Gauge,
	Layers,
	Link2,
	Link2Off,
	Maximize2,
	Merge,
	Music,
	Pencil,
	Repeat,
	Scissors,
	Snowflake,
	Split,
	StepBack,
	StepForward,
	Trash2,
} from "lucide-react";
import type { ReactNode } from "react";

import {
	ContextMenuItem,
	ContextMenuSeparator,
	ContextMenuShortcut,
} from "@/components/ui/context-menu";
import { useT } from "@/lib/i18n";
import { IS_MAC } from "../commands/keymap";
import { keyLabel, useKeymap } from "../commands/keymapStore";
import { useActions, useCanCut } from "../commands/useActions";
import { useCommandNames } from "../commands/useExposedCommands";
import { useMenuRunner } from "../commands/useMenuRunner";
import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../state/EditorContext";
import { openComposition } from "./openComposition";

// The key of a row of the cheat sheet, at the right of an entry.
function RowKey({ row }: { row: string }): ReactNode {
	const key = keyLabel(row, useKeymap(), IS_MAC);
	return key === null ? null : <ContextMenuShortcut>{key}</ContextMenuShortcut>;
}

// The entries of the item menu (SDD-012 section 8.2). They act on the
// selection; the row selects the item under the pointer first when it was not
// part of it.
export default function ItemContextMenuItems({
	itemId,
	word = null,
}: {
	itemId: string;
	// The caption word under the pointer, if the menu was opened on one.
	word?: number | null;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	const runtime = useRuntime();
	const run = useMenuRunner();
	const names = useCommandNames();
	const canCut = useCanCut();
	const selection = useUiState((s) => s.selection.items);
	const isCaption = useDocumentState(
		(s) => s.document?.items[itemId]?.type === "caption",
	);
	const off = useDocumentState(
		(s) => s.document?.items[itemId]?.disabled === true,
	);
	// Merge takes exactly two captions; the core judges whether they share a track.
	const captionPair = useDocumentState(
		(s) =>
			selection.length === 2 &&
			selection.every((id) => s.document?.items[id]?.type === "caption"),
	);
	// Crossfade takes exactly two visual items; the core judges whether they touch.
	const visualPair = useDocumentState(
		(s) =>
			selection.length === 2 &&
			selection.every((id) => {
				const type = s.document?.items[id]?.type;
				return type !== undefined && type !== "audio";
			}),
	);
	// Link takes two or more unlinked items; unlink acts on any linked one. The
	// core judges the rest (captions follow a clip with `linked_item`).
	const anyLinked = useDocumentState((s) =>
		selection.some((id) => s.document?.items[id]?.link !== undefined),
	);
	const linkable = useDocumentState(
		(s) =>
			selection.length >= 2 &&
			selection.every((id) => {
				const item = s.document?.items[id];
				return (
					item !== undefined &&
					item.link === undefined &&
					item.type !== "caption"
				);
			}),
	);
	// Extract audio is for a clip of a video asset whose sound is still on it.
	const extractable = useDocumentState((s) => {
		const item = s.document?.items[itemId];
		const asset = item?.asset;
		return (
			item?.type === "clip" &&
			item.audio_muted !== true &&
			asset !== undefined &&
			s.document?.assets[asset]?.kind === "video"
		);
	});
	// Freeze frame is for a clip of a video asset.
	const freezable = useDocumentState((s) => {
		const item = s.document?.items[itemId];
		const asset = item?.asset;
		return (
			item?.type === "clip" &&
			asset !== undefined &&
			s.document?.assets[asset]?.kind === "video"
		);
	});
	// A composition opens the timeline it uses, and expands when it only places it.
	const compositionTimeline = useDocumentState((s) => {
		const item = s.document?.items[itemId];
		return item?.type === "composition" ? item.timeline : undefined;
	});
	return (
		<>
			{compositionTimeline !== undefined && (
				<>
					<ContextMenuItem
						onSelect={() =>
							openComposition(runtime.projectId, compositionTimeline)
						}
					>
						<Layers aria-hidden="true" />
						{t("ui.editor.item.composition_open")}
					</ContextMenuItem>
					<ContextMenuItem
						onSelect={() => void actions.expandComposition(itemId)}
					>
						<Maximize2 aria-hidden="true" />
						{t("ui.editor.edit.expand_composition")}
					</ContextMenuItem>
				</>
			)}
			<ContextMenuItem onSelect={() => void actions.createComposition()}>
				<Layers aria-hidden="true" />
				{t("ui.editor.edit.create_composition")}
			</ContextMenuItem>
			{isCaption && word !== null && (
				<ContextMenuItem
					disabled={word < 1}
					onSelect={() => void actions.splitCaption(itemId, word)}
				>
					<Split aria-hidden="true" />
					{t("ui.editor.edit.split_caption")}
				</ContextMenuItem>
			)}
			{captionPair && (
				<ContextMenuItem onSelect={() => void actions.mergeCaptions(selection)}>
					<Merge aria-hidden="true" />
					{t("ui.editor.edit.merge_captions")}
				</ContextMenuItem>
			)}
			<ContextMenuItem
				disabled={!canCut}
				onSelect={() => void actions.splitAtPlayhead()}
			>
				<Scissors aria-hidden="true" />
				{t("ui.editor.edit.split")}
				<RowKey row="split" />
			</ContextMenuItem>
			<ContextMenuItem
				disabled={!canCut}
				onSelect={() => void run("trimToPlayhead.start")}
			>
				<StepForward aria-hidden="true" />
				{t("ui.editor.edit.trim_start")}
				<RowKey row="trim_start" />
			</ContextMenuItem>
			<ContextMenuItem
				disabled={!canCut}
				onSelect={() => void run("trimToPlayhead.end")}
			>
				<StepBack aria-hidden="true" />
				{t("ui.editor.edit.trim_end")}
				<RowKey row="trim_end" />
			</ContextMenuItem>
			{names.has("clip.set_speed") && (
				<ContextMenuItem
					onSelect={() => void run("surface.open_inspector_tab.speed")}
				>
					<Gauge aria-hidden="true" />
					{t("ui.editor.menu.speed")}
				</ContextMenuItem>
			)}
			{freezable && (
				<ContextMenuItem
					disabled={!canCut}
					onSelect={() => void actions.freezeFrame()}
				>
					<Snowflake aria-hidden="true" />
					{t("ui.editor.edit.freeze_frame")}
				</ContextMenuItem>
			)}
			{visualPair && (
				<ContextMenuItem onSelect={() => void actions.crossfadeSelection()}>
					<Blend aria-hidden="true" />
					{t("ui.editor.edit.crossfade")}
				</ContextMenuItem>
			)}
			<ContextMenuItem onSelect={() => actions.copyAttributes()}>
				<Copy aria-hidden="true" />
				{t("ui.editor.edit.copy_attributes")}
			</ContextMenuItem>
			<ContextMenuItem onSelect={() => void actions.pasteAttributes()}>
				<ClipboardPaste aria-hidden="true" />
				{t("ui.editor.edit.paste_attributes")}
			</ContextMenuItem>
			{linkable && (
				<ContextMenuItem onSelect={() => void actions.linkSelection()}>
					<Link2 aria-hidden="true" />
					{t("ui.editor.edit.link")}
				</ContextMenuItem>
			)}
			{anyLinked && (
				<ContextMenuItem onSelect={() => void actions.unlinkSelection()}>
					<Link2Off aria-hidden="true" />
					{t("ui.editor.edit.unlink")}
				</ContextMenuItem>
			)}
			{extractable && (
				<ContextMenuItem onSelect={() => void actions.extractAudio()}>
					<Music aria-hidden="true" />
					{t("ui.editor.edit.extract_audio")}
				</ContextMenuItem>
			)}
			<ContextMenuItem onSelect={() => void run("surface.copy")}>
				<Copy aria-hidden="true" />
				{t("ui.editor.menu.copy")}
				<RowKey row="copy" />
			</ContextMenuItem>
			<ContextMenuItem onSelect={() => void run("surface.cut")}>
				<Scissors aria-hidden="true" />
				{t("ui.editor.menu.cut")}
				<RowKey row="cut" />
			</ContextMenuItem>
			<ContextMenuItem onSelect={() => void run("surface.paste")}>
				<ClipboardPaste aria-hidden="true" />
				{t("ui.editor.menu.paste")}
				<RowKey row="paste" />
			</ContextMenuItem>
			<ContextMenuItem onSelect={() => void run("duplicateSelection")}>
				<CopyPlus aria-hidden="true" />
				{t("ui.editor.edit.duplicate")}
				<RowKey row="duplicate" />
			</ContextMenuItem>
			<ContextMenuItem onSelect={() => runtime.uiWriter.startRepeat(itemId)}>
				<Repeat aria-hidden="true" />
				{t("ui.editor.edit.repeat")}
			</ContextMenuItem>
			<ContextMenuItem onSelect={() => void actions.toggleEnabled()}>
				{off ? <Eye aria-hidden="true" /> : <EyeOff aria-hidden="true" />}
				{t(off ? "ui.editor.edit.enable" : "ui.editor.edit.disable")}
			</ContextMenuItem>
			<ContextMenuItem onSelect={() => runtime.uiWriter.startRename(itemId)}>
				<Pencil aria-hidden="true" />
				{t("ui.editor.edit.rename")}
			</ContextMenuItem>
			<ContextMenuSeparator />
			<ContextMenuItem
				variant="destructive"
				onSelect={() => void actions.deleteSelection()}
			>
				<Trash2 aria-hidden="true" />
				{t("ui.editor.edit.delete")}
				<RowKey row="delete" />
			</ContextMenuItem>
			<ContextMenuItem
				variant="destructive"
				onSelect={() => void actions.deleteSelection("track")}
			>
				<Trash2 aria-hidden="true" />
				{t("ui.editor.edit.ripple_delete")}
			</ContextMenuItem>
			<ContextMenuItem
				variant="destructive"
				onSelect={() => void actions.deleteSelection("all")}
			>
				<Trash2 aria-hidden="true" />
				{t("ui.editor.edit.ripple_delete_all")}
			</ContextMenuItem>
		</>
	);
}
