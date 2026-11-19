import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import {
	enabledOperations,
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
} from "../../commands/units";
import { useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import { useDocumentState } from "../../state/EditorContext";
import { readTimeDisplay } from "../../state/prefs";
import TextField from "../fields/TextField";
import TimeCommitField from "../fields/TimeCommitField";
import ToggleField from "../fields/ToggleField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";
import AnimationSection from "./AnimationSection";
import BlendSection from "./BlendSection";
import FlipSection from "./FlipSection";
import KeyframesSection from "./KeyframesSection";
import MarkersSection from "./MarkersSection";
import MaskSection from "./MaskSection";
import TransformSection from "./TransformSection";
import TransitionsSection from "./TransitionsSection";

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
export function ItemBasics({ itemId }: { itemId: string }): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	const rate = useDocumentState((s) => s.document?.settings.fps);
	const fps = fpsOf(rate);
	const enabledExposed = useCommandsExposed("item.set_enabled");
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
				{enabledExposed && (
					<ToggleField
						label={t("ui.editor.inspector.enabled")}
						stored={item.disabled !== true}
						disabled={!canEdit}
						commit={(enabled) => send(enabledOperations(itemId, enabled))}
					/>
				)}
			</Section>
			<Section title={t("ui.editor.inspector.timing")}>
				<TimeCommitField
					label={t("ui.editor.inspector.start")}
					stored={displayStart(timed, fps, readTimeDisplay())}
					disabled={!canEdit}
					commit={(start) => send(moveOperations(itemId, start))}
				/>
				<TimeCommitField
					label={t("ui.editor.inspector.end")}
					stored={displayEnd(timed, fps, readTimeDisplay())}
					disabled={!canEdit}
					commit={(to) => send(trimEndOperations(itemId, to))}
				/>
				<ReadOnlyRow
					label={t("ui.editor.inspector.duration")}
					value={displayDuration(timed, fps, readTimeDisplay())}
				/>
				{hasSource && (
					<ReadOnlyRow
						label={t("ui.editor.inspector.in_point")}
						value={displayInPoint(timed, fps, readTimeDisplay())}
					/>
				)}
			</Section>
		</>
	);
}

// Transform, flip, blend, mask and transitions: how the item sits on the canvas
// and enters and leaves it. A sound has none of them.
export function ItemVisual({ itemId }: { itemId: string }): ReactNode {
	const type = useDocumentState((s) => s.document?.items[itemId]?.type);
	if (type === undefined || type === "audio") return null;
	return (
		<>
			<TransformSection itemIds={[itemId]} />
			<FlipSection itemId={itemId} />
			<BlendSection itemId={itemId} />
			<MaskSection itemId={itemId} />
			<TransitionsSection itemId={itemId} />
		</>
	);
}

// The animation presets of the item (`text.apply_preset`).
export function ItemAnimation({ itemId }: { itemId: string }): ReactNode {
	return <AnimationSection itemId={itemId} />;
}

// Keyframes and markers.
export function ItemKeyframes({ itemId }: { itemId: string }): ReactNode {
	return (
		<>
			<KeyframesSection itemId={itemId} />
			<MarkersSection itemId={itemId} />
		</>
	);
}
