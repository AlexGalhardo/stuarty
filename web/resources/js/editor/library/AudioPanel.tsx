import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import {
	DEFAULT_ATTACK,
	DEFAULT_GAIN_DB,
	DEFAULT_RELEASE,
	duckOperations,
	markBeatsOperations,
} from "../commands/builders";
import { useActions } from "../commands/useActions";
import { useCommandNames } from "../commands/useExposedCommands";
import { useStore } from "../state/createStore";
import { useRuntime, useUiState } from "../state/EditorContext";
import AssetCard, { type AssetDragStart } from "./AssetCard";
import PanelFrame from "./PanelFrame";
import { useApplyTarget } from "./useApplyTarget";

// The Audio tab of the material rail (SDD-012 section 6.4): the audio assets of
// the library, then the two audio actions of the selection. Each button sends
// one command; the core analyses the audio and resolves the ranges.
export default function AudioPanel({
	onAssetDragStart,
}: {
	onAssetDragStart?: AssetDragStart;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	const runtime = useRuntime();
	const names = useCommandNames();
	const entries = useStore(runtime.library, (s) => s.entries);
	const dismissed = useStore(runtime.library, (s) => s.dismissed);
	const audio = entries.filter(
		(entry) =>
			entry.kind === "audio" &&
			!entry.trashed &&
			!dismissed.includes(entry.asset_id),
	);
	// Ducking: the first selected item is the music, the others are speech.
	const selection = useUiState((s) => s.selection.items);
	const playable = useApplyTarget(["clip", "audio"]);
	const music = useApplyTarget(["audio"]);
	const canDuck =
		playable.canApply &&
		playable.ids.length >= 2 &&
		playable.ids[0] === selection[0] &&
		music.ids.includes(selection[0] ?? "");
	const canBeats = playable.canApply && playable.ids.length === 1;
	const duckWhy = canDuck ? undefined : "audio-duck-why";
	const beatsWhy = canBeats ? undefined : "audio-beats-why";
	return (
		<PanelFrame title={t("ui.editor.panels.audio")}>
			{audio.length === 0 ? (
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.library.audio_empty")}
				</p>
			) : (
				<ul className="flex flex-col gap-2">
					{audio.map((entry) => (
						<AssetCard
							key={entry.asset_id}
							entry={entry}
							onDragStart={onAssetDragStart}
						/>
					))}
				</ul>
			)}
			<h3 className="text-sm font-medium">
				{t("ui.editor.library.for_selection")}
			</h3>
			{names.has("audio.duck") && (
				<>
					<Button
						type="button"
						variant="secondary"
						size="sm"
						aria-disabled={!canDuck}
						aria-describedby={duckWhy}
						onClick={() => {
							if (!canDuck) return;
							const [musicId, ...speech] = playable.ids;
							if (musicId === undefined) return;
							void actions.runOperations(
								duckOperations(musicId, {
									speechItemIds: speech,
									gainDb: DEFAULT_GAIN_DB,
									attack: DEFAULT_ATTACK,
									release: DEFAULT_RELEASE,
								}),
							);
						}}
					>
						{t("ui.editor.library.duck")}
					</Button>
					{!canDuck && (
						<p id="audio-duck-why" className="text-xs text-muted-foreground">
							{t("ui.editor.library.duck_why")}
						</p>
					)}
				</>
			)}
			{names.has("audio.mark_beats") && (
				<>
					<Button
						type="button"
						variant="secondary"
						size="sm"
						aria-disabled={!canBeats}
						aria-describedby={beatsWhy}
						onClick={() => {
							const [id] = playable.ids;
							if (!canBeats || id === undefined) return;
							void actions.runOperations(
								markBeatsOperations(id, {
									every: 1,
									label: "",
									replace: true,
								}),
							);
						}}
					>
						{t("ui.editor.library.mark_beats")}
					</Button>
					{!canBeats && (
						<p id="audio-beats-why" className="text-xs text-muted-foreground">
							{t("ui.editor.library.mark_beats_why")}
						</p>
					)}
				</>
			)}
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.library.audio_note")}
			</p>
		</PanelFrame>
	);
}
