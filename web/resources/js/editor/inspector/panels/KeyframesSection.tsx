import type { ReactNode } from "react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import {
	type AnyKeyframeProperty,
	COLOUR_FALLBACK,
	type ColourKeyframeProperty,
	containsInclusive,
	type FramingKeyframeProperty,
	framingStored,
	KEYFRAME_COLOUR_PROPERTIES,
	KEYFRAME_CROP_PROPERTIES,
	KEYFRAME_EASINGS,
	KEYFRAME_FRAMING_PROPERTIES,
	KEYFRAME_MASK_PROPERTIES,
	KEYFRAME_NAMED_EASINGS,
	KEYFRAME_PROPERTIES,
	type KeyframeEasing,
	keyframeClearOperations,
	keyframeEasingOperations,
	keyframeFrame,
	keyframeRemoveOperations,
	keyframeSetOperations,
	type NamedKeyframeEasing,
} from "../../commands/builders";
import { formatTime, fpsOf } from "../../commands/units";
import { useCanEdit } from "../../commands/useActions";
import {
	useDocumentState,
	useEffectiveState,
	useRuntime,
	useUiState,
} from "../../state/EditorContext";
import { readTimeDisplay } from "../../state/prefs";
import type { CommitResult, FieldSnapshot } from "../fields/commitField";
import { FieldMessage } from "../fields/FieldFrame";
import { useCommitField } from "../fields/useCommitField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";
import BezierEditor from "./BezierEditor";
import { curveOf } from "./bezierModel";
import { round3, shown } from "./TransformSection";

const SELECT_CLASS =
	"h-8 min-w-0 rounded-lg border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30";

// The properties a keyframe may animate on an item of each type.
// The mask ones need the item to have a mask, the crop ones are for clips.
function propertiesOf(
	type: string,
	hasMask: boolean,
): readonly AnyKeyframeProperty[] {
	if (type === "audio") return ["volume"];
	const mask = hasMask ? KEYFRAME_MASK_PROPERTIES : [];
	if (type === "clip") {
		return [
			...KEYFRAME_PROPERTIES,
			...KEYFRAME_COLOUR_PROPERTIES,
			...mask,
			...KEYFRAME_CROP_PROPERTIES,
		];
	}
	return [
		...KEYFRAME_PROPERTIES.filter(
			(property) => property !== "volume" || type === "composition",
		),
		...mask,
	];
}

const ALL_PROPERTIES: readonly AnyKeyframeProperty[] = [
	...KEYFRAME_PROPERTIES,
	...KEYFRAME_COLOUR_PROPERTIES,
	...KEYFRAME_FRAMING_PROPERTIES,
];

const isProperty = (name: string): name is AnyKeyframeProperty =>
	(ALL_PROPERTIES as readonly string[]).includes(name);

const isColour = (name: AnyKeyframeProperty): name is ColourKeyframeProperty =>
	(KEYFRAME_COLOUR_PROPERTIES as readonly string[]).includes(name);

const isFraming = (
	name: AnyKeyframeProperty,
): name is FramingKeyframeProperty =>
	(KEYFRAME_FRAMING_PROPERTIES as readonly string[]).includes(name);

const isEasing = (name: string): name is KeyframeEasing =>
	(KEYFRAME_EASINGS as readonly string[]).includes(name);

const isNamedEasing = (name: string): name is NamedKeyframeEasing =>
	(KEYFRAME_NAMED_EASINGS as readonly string[]).includes(name);

// The value cell of a keyframe row: Enter or blur sends `keyframe.set` for the
// same position with the new value (SDD-012 section 12.3).
function ValueCell({
	label,
	stored,
	commit,
	disabled,
}: {
	label: string;
	stored: string;
	commit(draft: string): Promise<CommitResult>;
	disabled: boolean;
}): ReactNode {
	const binding = useCommitField(stored, commit);
	return (
		<div className="flex min-w-20 flex-1 flex-col gap-0.5">
			<Input
				{...binding.input}
				aria-label={label}
				disabled={disabled}
				autoComplete="off"
				spellCheck={false}
				className="h-7 min-w-20 px-1 font-mono text-xs"
			/>
			<FieldMessage id={`${label}-message`} state={binding} />
		</div>
	);
}

// The Keyframes section of SDD-012 section 12.1: for each animated property the
// keyframes the read model holds (time, value, easing), "Add at playhead" and
// the removals. Nothing here works out a value between two keyframes: the
// field of the Transform section shows the plan's.
export default function KeyframesSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	const rate = useDocumentState((s) => s.document?.settings.fps);
	const width = useDocumentState((s) => s.document?.settings.width);
	const height = useDocumentState((s) => s.document?.settings.height);
	const playhead = useUiState((s) => s.playheadFrame);
	const picked = useUiState((s) => s.selection.keyframe);
	const effective = useEffectiveState((s) => s.items[itemId]);
	const [property, setProperty] = useState<AnyKeyframeProperty | null>(null);
	const [valueText, setValueText] = useState("");
	const [easing, setEasing] = useState<NamedKeyframeEasing>("linear");
	// The row whose curve editor is open: "<property>:<t>".
	const [curveOpen, setCurveOpen] = useState<string | null>(null);
	// The refusal of the last "Add at playhead": the core's message and hint.
	const [addError, setAddError] = useState<FieldSnapshot["error"]>(null);
	if (item === undefined || width === undefined || height === undefined) {
		return null;
	}

	const fps = fpsOf(rate);
	const offered = propertiesOf(item.type, item.mask !== undefined);
	const target =
		property !== null && offered.includes(property)
			? property
			: (offered[0] as AnyKeyframeProperty);
	const animated = ALL_PROPERTIES.filter(
		(name) => (item.keyframes?.[name]?.length ?? 0) > 0,
	);
	const inside = containsInclusive(
		{ id: itemId, start: item.start, duration: item.duration },
		playhead,
	);

	// With no value typed, the keyframe takes the value the field shows now:
	// the stored one, or the plan's for a property already animated.
	const framed = isFraming(target) ? framingStored(item, target) : undefined;
	const current = isColour(target)
		? round3(item[target] ?? COLOUR_FALLBACK[target])
		: isFraming(target)
			? framed === undefined
				? ""
				: round3(framed)
			: target === "volume"
				? item.volume === undefined
					? ""
					: round3(item.volume)
				: shown(item, target, { width, height }, effective);
	const raw = valueText.trim() === "" ? current : valueText.trim();
	const value = raw === "" ? Number.NaN : Number(raw);
	const canAdd = canEdit && inside && Number.isFinite(value);

	return (
		<Section name="keyframes" title={t("ui.editor.keyframes.title")}>
			<div className="grid grid-cols-[96px_1fr] items-center gap-x-2 gap-y-1">
				<Label htmlFor={`kf-property-${itemId}`} className="text-xs">
					{t("ui.editor.keyframes.property")}
				</Label>
				<select
					id={`kf-property-${itemId}`}
					value={target}
					disabled={!canEdit}
					className={SELECT_CLASS}
					onChange={(event) => {
						const next = event.target.value;
						if (isProperty(next)) {
							setProperty(next);
							setValueText("");
							setAddError(null);
						}
					}}
				>
					{offered.map((name) => (
						<option key={name} value={name}>
							{t(`ui.editor.keyframes.prop.${name}`)}
						</option>
					))}
				</select>
				<Label htmlFor={`kf-value-${itemId}`} className="text-xs">
					{t("ui.editor.keyframes.value")}
				</Label>
				<Input
					id={`kf-value-${itemId}`}
					value={valueText}
					placeholder={current}
					inputMode="decimal"
					autoComplete="off"
					disabled={!canEdit}
					className="h-8 px-2 font-mono text-sm"
					aria-invalid={addError === null ? undefined : true}
					aria-describedby={
						addError === null ? undefined : `kf-add-${itemId}-message`
					}
					onChange={(event) => {
						setValueText(event.target.value);
						setAddError(null);
					}}
				/>
				<Label htmlFor={`kf-easing-${itemId}`} className="text-xs">
					{t("ui.editor.keyframes.easing")}
				</Label>
				<select
					id={`kf-easing-${itemId}`}
					value={easing}
					disabled={!canEdit}
					className={SELECT_CLASS}
					onChange={(event) => {
						const next = event.target.value;
						if (isNamedEasing(next)) setEasing(next);
					}}
				>
					{KEYFRAME_NAMED_EASINGS.map((name) => (
						<option key={name} value={name}>
							{t(`ui.editor.keyframes.easing_name.${name}`)}
						</option>
					))}
				</select>
			</div>
			<div className="flex flex-wrap items-center gap-2">
				<Button
					type="button"
					size="sm"
					disabled={!canAdd}
					onClick={() => {
						void send(
							keyframeSetOperations({
								itemId,
								property: target,
								frame: playhead,
								value,
								easing,
							}),
						).then((result) => {
							if (result.ok) {
								setAddError(null);
								setValueText("");
							} else setAddError(result.error);
						});
					}}
				>
					{t("ui.editor.keyframes.add")}
				</Button>
				{!inside && (
					<span className="text-xs text-muted-foreground">
						{t("ui.editor.keyframes.outside")}
					</span>
				)}
				{animated.length > 0 && (
					<Button
						type="button"
						size="sm"
						variant="outline"
						disabled={!canEdit}
						onClick={() => void send(keyframeClearOperations(itemId))}
					>
						{t("ui.editor.keyframes.clear_all")}
					</Button>
				)}
			</div>
			<FieldMessage
				id={`kf-add-${itemId}-message`}
				state={{ error: addError, remoteActor: null }}
			/>

			{animated.map((name) => {
				const keys = item.keyframes?.[name] ?? [];
				const label = t(`ui.editor.keyframes.prop.${name}`);
				return (
					<div
						key={name}
						data-keyframe-group={name}
						className="flex flex-col gap-1"
					>
						<div className="flex items-center justify-between">
							<h4 className="text-xs font-semibold">{label}</h4>
							<Button
								type="button"
								size="xs"
								variant="ghost"
								disabled={!canEdit}
								aria-label={t("ui.editor.keyframes.clear_of", {
									property: label,
								})}
								onClick={() => void send(keyframeClearOperations(itemId, name))}
							>
								{t("ui.editor.keyframes.clear")}
							</Button>
						</div>
						<ul className="flex flex-col gap-1">
							{keys.map((key) => {
								const time = formatTime(key.t, fps, readTimeDisplay());
								const frame = keyframeFrame(item.start, key.t);
								const isPicked =
									picked?.item === itemId &&
									picked.property === name &&
									picked.t === key.t;
								const rowId = `${name}:${key.t}`;
								const curve = curveOf(key.bezier);
								// A new value keeps the easing of the keyframe, a curve included.
								const resend = (next: {
									value: number;
								}): Promise<CommitResult> =>
									send(
										keyframeSetOperations({
											itemId,
											property: name,
											frame,
											value: next.value,
											easing: key.easing ?? "linear",
											...(key.bezier === undefined
												? {}
												: { bezier: key.bezier }),
										}),
									);
								// A named easing, or the soft curve as the start of a custom one.
								const setEasingOf = (next: KeyframeEasing): void => {
									if (next === key.easing) return;
									void send(
										keyframeEasingOperations({
											itemId,
											property: name,
											frame,
											...(isNamedEasing(next)
												? { easing: next }
												: { preset: "in_out" as const }),
										}),
									).then((result) => {
										if (result.ok && next === "bezier") setCurveOpen(rowId);
									});
								};
								return (
									<li
										key={key.t}
										data-keyframe-row={rowId}
										data-selected={isPicked ? "" : undefined}
										className={cn(
											"flex flex-wrap items-start gap-1 rounded-md px-1",
											isPicked && "bg-accent",
										)}
									>
										<Button
											type="button"
											size="xs"
											variant="ghost"
											aria-pressed={isPicked}
											aria-label={t("ui.editor.keyframes.select_of", {
												property: label,
												time,
											})}
											className="font-mono"
											onClick={() =>
												runtime.uiWriter.selectKeyframe({
													item: itemId,
													property: name,
													t: key.t,
												})
											}
										>
											{time}
										</Button>
										<ValueCell
											label={t("ui.editor.keyframes.value_of", {
												property: label,
												time,
											})}
											stored={round3(key.value)}
											disabled={!canEdit}
											commit={async (draft) => {
												const next = Number(draft);
												if (draft.trim() === "" || !Number.isFinite(next)) {
													return {
														ok: false,
														error: { code: "E_VALIDATION", message: "" },
													};
												}
												return resend({ value: next });
											}}
										/>
										<select
											aria-label={t("ui.editor.keyframes.easing_of", {
												property: label,
												time,
											})}
											value={key.easing ?? "linear"}
											disabled={!canEdit}
											className={cn(SELECT_CLASS, "h-7 w-28 text-xs")}
											onChange={(event) => {
												const next = event.target.value;
												if (isEasing(next)) setEasingOf(next);
											}}
										>
											{KEYFRAME_EASINGS.map((easingName) => (
												<option key={easingName} value={easingName}>
													{t(`ui.editor.keyframes.easing_name.${easingName}`)}
												</option>
											))}
										</select>
										{curve !== null && (
											<Button
												type="button"
												size="xs"
												variant="ghost"
												aria-expanded={curveOpen === rowId}
												aria-label={t(
													curveOpen === rowId
														? "ui.editor.keyframes.curve_close"
														: "ui.editor.keyframes.curve_edit",
													{ property: label, time },
												)}
												onClick={() =>
													setCurveOpen(curveOpen === rowId ? null : rowId)
												}
											>
												{t("ui.editor.keyframes.curve")}
											</Button>
										)}
										<Button
											type="button"
											size="xs"
											variant="ghost"
											disabled={!canEdit}
											aria-label={t("ui.editor.keyframes.remove_of", {
												property: label,
												time,
											})}
											onClick={() =>
												void send(keyframeRemoveOperations(itemId, name, frame))
											}
										>
											{t("ui.editor.keyframes.remove")}
										</Button>
										{curve !== null && curveOpen === rowId && (
											<BezierEditor
												itemId={itemId}
												property={name}
												frame={frame}
												label={label}
												time={time}
												stored={curve}
											/>
										)}
									</li>
								);
							})}
						</ul>
					</div>
				);
			})}
		</Section>
	);
}
