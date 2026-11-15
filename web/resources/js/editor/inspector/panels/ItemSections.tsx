import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import {
	moveOperations,
	renameOperations,
	trimEndOperations,
} from "../../commands/builders";
import {
	displayDuration,
	displayEnd,
	displayInPoint,
	displayStart,
	fpsOf,
	type TimeDisplay,
} from "../../commands/units";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import TextField from "../fields/TextField";
import TimeCommitField from "../fields/TimeCommitField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";
import KeyframesSection from "./KeyframesSection";
import TransformSection from "./TransformSection";

// The user preference of the display mode lives in prefs.ts (not built yet).
const DISPLAY: TimeDisplay = "timecode";

function ReadOnlyRow({
	label,
	value,
}: {
	label: string;
	value: string;
}): ReactNode {
	return (
		<div className="grid grid-cols-[96px_1fr] items-baseline gap-x-2">
			<span className="text-xs">{label}</span>
			<span className="font-mono text-sm tabular-nums">{value}</span>
		</div>
	);
}

// The Name and Timing sections every item panel starts with (SDD-012 section
// 12.1): `item.rename`, `item.move {start}` and `item.trim {edge: "end", to}`.
// Duration and In point are read only and come from the units helpers; the
// panel does no arithmetic on times (rule R5).
export default function ItemSections({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	const rate = useDocumentState((s) => s.document?.settings.fps);
	const fps = fpsOf(rate);
	if (item === undefined) return null;
	const timed = {
		start: item.start,
		duration: item.duration,
		source_in: item.source_in,
	};
	const hasSource = item.type === "clip" || item.type === "audio";

	return (
		<>
			<Section title={t("ui.editor.inspector.name")}>
				<TextField
					label={t("ui.editor.inspector.name")}
					stored={item.name ?? ""}
					maxLength={120}
					disabled={!canEdit}
					commit={(name) => send(renameOperations(itemId, name))}
				/>
			</Section>
			<Section title={t("ui.editor.inspector.timing")}>
				<TimeCommitField
					label={t("ui.editor.inspector.start")}
					stored={displayStart(timed, fps, DISPLAY)}
					disabled={!canEdit}
					commit={(start) => send(moveOperations(itemId, start))}
				/>
				<TimeCommitField
					label={t("ui.editor.inspector.end")}
					stored={displayEnd(timed, fps, DISPLAY)}
					disabled={!canEdit}
					commit={(to) => send(trimEndOperations(itemId, to))}
				/>
				<ReadOnlyRow
					label={t("ui.editor.inspector.duration")}
					value={displayDuration(timed, fps, DISPLAY)}
				/>
				{hasSource && (
					<ReadOnlyRow
						label={t("ui.editor.inspector.in_point")}
						value={displayInPoint(timed, fps, DISPLAY)}
					/>
				)}
			</Section>
			{item.type !== "audio" && <TransformSection itemIds={[itemId]} />}
			<KeyframesSection itemId={itemId} />
		</>
	);
}
