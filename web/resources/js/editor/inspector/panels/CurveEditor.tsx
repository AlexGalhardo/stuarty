import {
	type KeyboardEvent,
	type PointerEvent,
	type ReactNode,
	useEffect,
	useRef,
	useState,
} from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import {
	CURVE_CHANNELS,
	type CurveChannel,
	type CurvePointValue,
	colorOperations,
	colorResetChanges,
} from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import type { FieldError } from "../fields/commitField";
import { FieldMessage } from "../fields/FieldFrame";
import { useFieldCommit } from "../useFieldCommit";
import ColorGroup, { ChoiceRow } from "./ColorGroup";
import {
	addPoint,
	addPointInWidestGap,
	MAX_CURVE_POINTS,
	MIN_CURVE_POINTS,
	movePoint,
	NEUTRAL_CURVE,
	removePoint,
	sameCurve,
} from "./curveModel";

const SIZE = 200;
const PAD = 12;
const SPAN = SIZE - 2 * PAD;
const STEP = 0.01;
const BIG_STEP = 0.05;

const toX = (x: number): number => PAD + x * SPAN;
const toY = (y: number): number => SIZE - PAD - y * SPAN;
const fromSvg = (value: number): number => (value - PAD) / SPAN;

const CHANNEL_COLOUR: Record<CurveChannel, string> = {
	master: "currentColor",
	red: "#e5484d",
	green: "#30a46c",
	blue: "#3e63dd",
};

const ARROWS: Record<string, readonly [number, number]> = {
	ArrowLeft: [-1, 0],
	ArrowRight: [1, 0],
	ArrowUp: [0, 1],
	ArrowDown: [0, -1],
};

// shortcut-ok: Delete and Backspace remove the focused point of the curve; they are keys of the widget, not editor shortcuts.
const isRemovePointKey = (key: string): boolean =>
	key === "Delete" || key === "Backspace";

// The tone curves (SDD-012 section 12.1): one editor for the channel the
// person picks. A point moves with the pointer or the arrow keys (Shift for a
// larger step), Delete removes it, a click on the plot adds one, and the
// buttons add and remove without a pointer. The draft is sent once the gesture
// ends, as the whole list in `clip.set_color {curves}`. The editor draws the
// points the person chose and the straight lines between them; the graded
// pixels are the core's.
export default function CurveEditor({ itemId }: { itemId: string }): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	const [channel, setChannel] = useState<CurveChannel>("master");
	const [draft, setDraft] = useState<CurvePointValue[] | null>(null);
	const [active, setActive] = useState<number | null>(null);
	const [error, setError] = useState<FieldError | null>(null);
	const plot = useRef<SVGSVGElement | null>(null);
	const dragging = useRef<number | null>(null);
	const latest = useRef<CurvePointValue[]>([]);

	const stored: readonly CurvePointValue[] =
		item?.curves?.[channel] ?? NEUTRAL_CURVE;
	const storedKey = JSON.stringify(stored);
	// A change that came from the document, or another channel, replaces the draft.
	// biome-ignore lint/correctness/useExhaustiveDependencies: keyed on the stored curve and the channel
	useEffect(() => {
		setDraft(null);
		setActive(null);
	}, [storedKey, channel]);

	if (item === undefined) return null;
	const points = draft ?? stored;
	if (draft === null) latest.current = [...stored];
	const storedChannels = CURVE_CHANNELS.filter(
		(name) => item.curves?.[name] !== undefined,
	);

	const commit = (next: readonly CurvePointValue[]): void => {
		if (sameCurve(next, stored)) {
			setDraft(null);
			return;
		}
		void send(
			colorOperations(itemId, { curves: { [channel]: [...next] } }),
		).then((result) => {
			setError(result.ok ? null : result.error);
			if (!result.ok) setDraft(null);
		});
	};
	const change = (next: CurvePointValue[]): void => {
		latest.current = next;
		setDraft(next);
	};

	const spotOf = (
		event: PointerEvent<SVGSVGElement | SVGCircleElement>,
	): { x: number; y: number } | null => {
		const rect = plot.current?.getBoundingClientRect();
		if (rect === undefined || rect.width === 0) return null;
		const scale = SIZE / rect.width;
		return {
			x: fromSvg((event.clientX - rect.left) * scale),
			y: 1 - fromSvg((event.clientY - rect.top) * scale),
		};
	};

	const onKey = (
		event: KeyboardEvent<SVGCircleElement>,
		index: number,
	): void => {
		const point = latest.current[index];
		if (point === undefined) return;
		const arrow = ARROWS[event.key];
		if (arrow !== undefined) {
			event.preventDefault();
			const step = event.shiftKey ? BIG_STEP : STEP;
			change(
				movePoint(
					latest.current,
					index,
					point.x + arrow[0] * step,
					point.y + arrow[1] * step,
				),
			);
		} else if (isRemovePointKey(event.key)) {
			event.preventDefault();
			setActive(null);
			commit(removePoint(latest.current, index));
		}
	};

	const path = points
		.map(
			(point, index) =>
				`${index === 0 ? "M" : "L"}${toX(point.x)} ${toY(point.y)}`,
		)
		.join(" ");
	const label = t(`ui.editor.clip.curve_channel_name.${channel}`);

	return (
		<ColorGroup
			itemId={itemId}
			title={t("ui.editor.clip.curves_title")}
			resetLabel={t("ui.editor.clip.color_reset")}
			canReset={storedChannels.length > 0}
			reset={() =>
				colorResetChanges("curves", {
					hsl: [],
					wheels: [],
					curves: storedChannels,
				})
			}
		>
			<ChoiceRow<CurveChannel>
				label={t("ui.editor.clip.curve_channel")}
				value={channel}
				options={CURVE_CHANNELS.map((name) => ({
					value: name,
					label: t(`ui.editor.clip.curve_channel_name.${name}`),
				}))}
				onChange={setChannel}
			/>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.clip.curve_hint")}
			</p>
			<svg
				ref={plot}
				viewBox={`0 0 ${SIZE} ${SIZE}`}
				aria-label={t("ui.editor.clip.curve_plot", { channel: label })}
				data-curve-plot={channel}
				className="aspect-square w-full max-w-60 touch-none rounded-md border border-input bg-muted/30"
				onPointerDown={(event) => {
					if (!canEdit || event.target !== event.currentTarget) return;
					const spot = spotOf(event);
					if (spot === null) return;
					const next = addPoint(latest.current, spot.x, spot.y);
					if (next.length !== latest.current.length) commit(next);
				}}
			>
				<line
					x1={toX(0)}
					y1={toY(0)}
					x2={toX(1)}
					y2={toY(1)}
					className="stroke-border"
					strokeDasharray="3 3"
				/>
				<path
					d={path}
					fill="none"
					stroke={CHANNEL_COLOUR[channel]}
					strokeWidth={2}
				/>
				{points.map((point, index) => (
					<circle
						// biome-ignore lint/suspicious/noArrayIndexKey: the points have no id, their order is their identity
						key={index}
						role="slider"
						tabIndex={canEdit ? 0 : -1}
						aria-label={t("ui.editor.clip.curve_point", {
							channel: label,
							n: index + 1,
						})}
						aria-orientation="vertical"
						aria-valuemin={0}
						aria-valuemax={1}
						aria-valuenow={point.y}
						aria-valuetext={t("ui.editor.clip.curve_point_value", {
							x: point.x,
							y: point.y,
						})}
						aria-disabled={!canEdit}
						cx={toX(point.x)}
						cy={toY(point.y)}
						r={active === index ? 7 : 5}
						fill={CHANNEL_COLOUR[channel]}
						className="cursor-grab stroke-background outline-none focus-visible:stroke-ring"
						strokeWidth={2}
						onFocus={() => setActive(index)}
						onKeyDown={(event) => {
							if (canEdit) onKey(event, index);
						}}
						onKeyUp={(event) => {
							if (event.key in ARROWS) commit(latest.current);
						}}
						onPointerDown={(event) => {
							if (!canEdit) return;
							event.preventDefault();
							event.currentTarget.setPointerCapture(event.pointerId);
							event.currentTarget.focus();
							dragging.current = index;
						}}
						onPointerMove={(event) => {
							if (dragging.current !== index) return;
							const spot = spotOf(event);
							if (spot !== null) {
								change(movePoint(latest.current, index, spot.x, spot.y));
							}
						}}
						onPointerUp={() => {
							if (dragging.current !== index) return;
							dragging.current = null;
							commit(latest.current);
						}}
					/>
				))}
			</svg>
			<div className="flex flex-wrap gap-2">
				<Button
					type="button"
					size="sm"
					variant="outline"
					disabled={!canEdit || points.length >= MAX_CURVE_POINTS}
					onClick={() => commit(addPointInWidestGap(latest.current))}
				>
					{t("ui.editor.clip.curve_add_point")}
				</Button>
				<Button
					type="button"
					size="sm"
					variant="outline"
					disabled={
						!canEdit || active === null || points.length <= MIN_CURVE_POINTS
					}
					onClick={() => {
						if (active === null) return;
						setActive(null);
						commit(removePoint(latest.current, active));
					}}
				>
					{t("ui.editor.clip.curve_remove_point")}
				</Button>
			</div>
			<FieldMessage
				id={`curve-message-${itemId}`}
				state={{ error, remoteActor: null }}
			/>
		</ColorGroup>
	);
}
