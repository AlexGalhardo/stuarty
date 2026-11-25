import { MousePointer2, MousePointerClick, ZoomIn } from "lucide-react";
import { type ReactNode, useState } from "react";

import { Segment } from "@/components/Segment";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import {
	autoZoomOperations,
	CURSOR_STYLES,
	type CursorStyle,
	clickHighlightOperations,
	cursorOperations,
	HIGHLIGHT_STYLES,
	type HighlightStyle,
} from "../../commands/builders";
import { useActions, useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import NumberField from "../fields/NumberField";
import SelectField from "../fields/SelectField";
import Section, { ToolLine } from "../Section";

const isCursorStyle = (value: string): value is CursorStyle =>
	(CURSOR_STYLES as readonly string[]).includes(value);

const isHighlightStyle = (value: string): value is HighlightStyle =>
	(HIGHLIGHT_STYLES as readonly string[]).includes(value);

// The screen recording sections (SDD-025 section 17, SDD-002 C.93 to C.95): the zoom on the clicks, the smooth
// cursor and the click highlight of a screen recording made with pointer events. Each button sends one command;
// the service reads the pointer events, the core plans and draws, and answers with a refusal when the clip is not
// a screen recording that has them. The options are the core's defaults until the person changes them. The zoom
// is the inspector section of the clip; the cursor and the highlight are the "Cursor and clicks" panel of the
// Effects tab (library/CursorClicksPanel).
function useRun(): {
	busy: boolean;
	canEdit: boolean;
	run(
		operations: Parameters<ReturnType<typeof useActions>["runOperations"]>[0],
	): Promise<void>;
} {
	const actions = useActions();
	const canEdit = useCanEdit();
	const [busy, setBusy] = useState(false);
	const run = async (
		operations: Parameters<typeof actions.runOperations>[0],
	): Promise<void> => {
		setBusy(true);
		try {
			await actions.runOperations(operations);
		} finally {
			setBusy(false);
		}
	};
	return { busy, canEdit, run };
}

export function ScreenCursorSection({ itemId }: { itemId: string }): ReactNode {
	const t = useT();
	const { busy, canEdit, run } = useRun();
	const exposed = useCommandsExposed("recording.set_cursor");
	const [cursorStyle, setCursorStyle] = useState<CursorStyle>("dot");
	const [smoothing, setSmoothing] = useState<number | undefined>(undefined);
	if (!exposed) return null;
	return (
		<Section
			title={t("ui.editor.screen_take.cursor_title")}
			name="screen-cursor"
			icon={MousePointer2}
		>
			<ToolLine name="recording.set_cursor" />
			<SelectField
				label={t("ui.editor.screen_take.cursor_style")}
				stored={cursorStyle}
				disabled={!canEdit}
				options={[
					{ value: "dot", label: t("ui.editor.screen_take.cursor_dot") },
					{ value: "ring", label: t("ui.editor.screen_take.cursor_ring") },
				]}
				commit={async (value) => {
					if (isCursorStyle(value)) setCursorStyle(value);
					return { ok: true };
				}}
			/>
			<NumberField
				label={t("ui.editor.screen_take.cursor_smoothing")}
				stored={smoothing === undefined ? "" : String(smoothing)}
				disabled={!canEdit}
				step={50}
				min={0}
				max={2000}
				suffix="ms"
				placeholder={t("ui.editor.screen_take.default")}
				commit={async (value) => {
					setSmoothing(value);
					return { ok: true };
				}}
			/>
			<div className="flex gap-2">
				<Button
					type="button"
					variant="outline"
					size="sm"
					disabled={!canEdit || busy}
					onClick={() =>
						run(
							cursorOperations(itemId, {
								visible: true,
								style: cursorStyle,
								smoothingMs: smoothing,
							}),
						)
					}
				>
					{t("ui.editor.screen_take.cursor_apply")}
				</Button>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					disabled={!canEdit || busy}
					onClick={() => run(cursorOperations(itemId, { visible: false }))}
				>
					{t("ui.editor.screen_take.cursor_remove")}
				</Button>
			</div>
		</Section>
	);
}

export function ScreenHighlightSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const { busy, canEdit, run } = useRun();
	const exposed = useCommandsExposed("recording.set_click_highlight");
	const [highlightStyle, setHighlightStyle] = useState<HighlightStyle>("ring");
	const [duration, setDuration] = useState<number | undefined>(undefined);
	if (!exposed) return null;
	return (
		<Section
			title={t("ui.editor.screen_take.highlight_title")}
			name="screen-highlight"
			icon={MousePointerClick}
		>
			<ToolLine name="recording.set_click_highlight" />
			<Segment
				label={t("ui.editor.screen_take.highlight_style")}
				value={highlightStyle}
				onChange={(value) => {
					if (isHighlightStyle(value)) setHighlightStyle(value);
				}}
				options={[
					{ value: "ring", label: t("ui.editor.screen_take.highlight_ring") },
					{ value: "wave", label: t("ui.editor.screen_take.highlight_wave") },
					{ value: "pulse", label: t("ui.editor.screen_take.highlight_pulse") },
					{ value: "none", label: t("ui.editor.screen_take.highlight_none") },
				]}
				className="w-full *:flex-1"
			/>
			<NumberField
				label={t("ui.editor.screen_take.highlight_duration")}
				stored={duration === undefined ? "" : String(duration)}
				disabled={!canEdit}
				step={100}
				min={200}
				max={2000}
				suffix="ms"
				placeholder={t("ui.editor.screen_take.default")}
				commit={async (value) => {
					setDuration(value);
					return { ok: true };
				}}
			/>
			<Button
				type="button"
				variant="outline"
				size="sm"
				disabled={!canEdit || busy}
				onClick={() =>
					run(
						clickHighlightOperations(itemId, {
							style: highlightStyle,
							durationMs: duration,
						}),
					)
				}
			>
				{highlightStyle === "none"
					? t("ui.editor.screen_take.highlight_remove")
					: t("ui.editor.screen_take.highlight_apply")}
			</Button>
		</Section>
	);
}

// The "Zoom on click" section of a clip, in the Adjust tab of the inspector.
export default function ScreenTakeSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const { busy, canEdit, run } = useRun();
	const exposed = useCommandsExposed("recording.auto_zoom");
	const [level, setLevel] = useState<number | undefined>(undefined);
	if (!exposed) return null;
	return (
		<Section
			title={t("ui.editor.screen_take.title")}
			name="screen-take"
			icon={ZoomIn}
		>
			<ToolLine name="recording.auto_zoom" />
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.screen_take.hint")}
			</p>
			<NumberField
				label={t("ui.editor.screen_take.zoom_level")}
				stored={level === undefined ? "" : String(level)}
				disabled={!canEdit}
				step={0.1}
				min={1.1}
				max={3}
				suffix="x"
				placeholder={t("ui.editor.screen_take.default")}
				commit={async (value) => {
					setLevel(value);
					return { ok: true };
				}}
			/>
			<Button
				type="button"
				variant="outline"
				size="sm"
				disabled={!canEdit || busy}
				onClick={() => run(autoZoomOperations(itemId, { level }))}
			>
				{t("ui.editor.screen_take.zoom_apply")}
			</Button>
		</Section>
	);
}
