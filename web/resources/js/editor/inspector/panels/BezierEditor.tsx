import {
	type KeyboardEvent,
	type PointerEvent,
	type ReactNode,
	useEffect,
	useRef,
	useState,
} from "react";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import {
	type AnyKeyframeProperty,
	BEZIER_PRESETS,
	type BezierPreset,
	keyframeEasingOperations,
} from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import type { CommitResult, FieldError } from "../fields/commitField";
import { FieldMessage } from "../fields/FieldFrame";
import { useCommitField } from "../fields/useCommitField";
import { useFieldCommit } from "../useFieldCommit";
import {
	type Curve,
	curvePath,
	fromViewX,
	fromViewY,
	moveHandle,
	SIZE,
	sameCurve,
	setNumber,
	toViewX,
	toViewY,
} from "./bezierModel";

const STEP = 0.01;
const BIG_STEP = 0.1;
const NUMBER_NAMES = ["x1", "y1", "x2", "y2"] as const;

const ARROWS: Record<string, readonly [number, number]> = {
	ArrowLeft: [-1, 0],
	ArrowRight: [1, 0],
	ArrowUp: [0, 1],
	ArrowDown: [0, -1],
};

const SELECT_CLASS =
	"h-8 min-w-0 rounded-lg border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30";

const isPreset = (name: string): name is BezierPreset =>
	(BEZIER_PRESETS as readonly string[]).includes(name);

// One of the four numbers, typed: Enter or blur sends the whole curve with that
// number changed (`keyframe.set_easing {bezier}`).
function NumberCell({
	label,
	stored,
	disabled,
	commit,
}: {
	label: string;
	stored: number;
	disabled: boolean;
	commit(draft: string): Promise<CommitResult>;
}): ReactNode {
	const binding = useCommitField(String(stored), commit);
	return (
		<div className="flex min-w-14 flex-1 flex-col gap-0.5">
			<Label className="text-xs">
				{label}
				<Input
					{...binding.input}
					disabled={disabled}
					inputMode="decimal"
					autoComplete="off"
					spellCheck={false}
					className="mt-0.5 h-7 min-w-14 px-1 font-mono text-xs"
				/>
			</Label>
		</div>
	);
}

// The curve editor of one keyframe segment (SDD-012 section 12.3, RF-080): the
// two handles move with the pointer or the arrow keys (Shift for a larger step), a
// named curve is picked from a list and the four numbers can be typed. The
// gesture ends, the four numbers go to the core as `keyframe.set_easing`. The
// plot draws the curve through the numbers; the value of every frame is the
// core's.
export default function BezierEditor({
	itemId,
	property,
	frame,
	label,
	time,
	stored,
}: {
	itemId: string;
	property: AnyKeyframeProperty;
	frame: number;
	label: string;
	time: string;
	stored: Curve;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const [draft, setDraft] = useState<Curve | null>(null);
	const [active, setActive] = useState<0 | 1 | null>(null);
	const [error, setError] = useState<FieldError | null>(null);
	const plot = useRef<SVGSVGElement | null>(null);
	const dragging = useRef<0 | 1 | null>(null);
	const latest = useRef<Curve>(stored);
	const storedKey = stored.join(",");

	// A change that came from the document replaces the draft.
	// biome-ignore lint/correctness/useExhaustiveDependencies: keyed on the stored numbers
	useEffect(() => {
		setDraft(null);
	}, [storedKey]);

	const curve = draft ?? stored;
	if (draft === null) latest.current = stored;

	const report = (result: CommitResult): CommitResult => {
		setError(result.ok ? null : result.error);
		if (!result.ok) setDraft(null);
		return result;
	};
	const commit = (next: Curve): Promise<CommitResult> => {
		if (sameCurve(next, stored)) {
			setDraft(null);
			return Promise.resolve({ ok: true });
		}
		return send(
			keyframeEasingOperations({ itemId, property, frame, bezier: next }),
		).then(report);
	};
	const change = (next: Curve): void => {
		latest.current = next;
		setDraft(next);
	};

	const spotOf = (
		event: PointerEvent<SVGElement>,
	): { x: number; y: number } | null => {
		const rect = plot.current?.getBoundingClientRect();
		if (rect === undefined || rect.width === 0) return null;
		const scale = SIZE / rect.width;
		return {
			x: fromViewX((event.clientX - rect.left) * scale),
			y: fromViewY((event.clientY - rect.top) * scale),
		};
	};

	const onKey = (event: KeyboardEvent<SVGElement>, handle: 0 | 1): void => {
		const arrow = ARROWS[event.key];
		if (arrow === undefined) return;
		event.preventDefault();
		const step = event.shiftKey ? BIG_STEP : STEP;
		const at = latest.current;
		change(
			moveHandle(
				at,
				handle,
				(at[handle * 2] ?? 0) + arrow[0] * step,
				(at[handle * 2 + 1] ?? 0) + arrow[1] * step,
			),
		);
	};

	const [x1, y1, x2, y2] = curve;
	const heading = `${label} ${time}`;

	return (
		<div
			data-bezier-editor={`${property}`}
			className="flex basis-full flex-col gap-2 rounded-md border border-input p-2"
		>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.keyframes.curve_hint")}
			</p>
			<svg
				ref={plot}
				viewBox={`0 0 ${SIZE} ${SIZE}`}
				aria-label={t("ui.editor.keyframes.curve_plot", { property: heading })}
				className="aspect-square w-full max-w-60 touch-none rounded-md border border-input bg-muted/30"
			>
				<line
					x1={toViewX(0)}
					y1={toViewY(0)}
					x2={toViewX(1)}
					y2={toViewY(1)}
					className="stroke-border"
					strokeDasharray="3 3"
				/>
				<line
					x1={toViewX(0)}
					y1={toViewY(0)}
					x2={toViewX(x1)}
					y2={toViewY(y1)}
					className="stroke-muted-foreground"
				/>
				<line
					x1={toViewX(1)}
					y1={toViewY(1)}
					x2={toViewX(x2)}
					y2={toViewY(y2)}
					className="stroke-muted-foreground"
				/>
				<path
					d={curvePath(curve)}
					fill="none"
					stroke="currentColor"
					strokeWidth={2}
				/>
				{([0, 1] as const).map((handle) => {
					const hx = curve[handle * 2] ?? 0;
					const hy = curve[handle * 2 + 1] ?? 0;
					return (
						<circle
							key={handle}
							role="slider"
							data-bezier-handle={handle + 1}
							tabIndex={canEdit ? 0 : -1}
							aria-label={t("ui.editor.keyframes.curve_handle", {
								n: handle + 1,
							})}
							aria-valuemin={-2}
							aria-valuemax={3}
							aria-valuenow={hy}
							aria-valuetext={t("ui.editor.keyframes.curve_handle_value", {
								x: hx,
								y: hy,
							})}
							aria-disabled={!canEdit}
							cx={toViewX(hx)}
							cy={toViewY(hy)}
							r={active === handle ? 7 : 5}
							fill="currentColor"
							className="cursor-grab stroke-background outline-none focus-visible:stroke-ring"
							strokeWidth={2}
							onFocus={() => setActive(handle)}
							onBlur={() => setActive(null)}
							onKeyDown={(event) => {
								if (canEdit) onKey(event, handle);
							}}
							onKeyUp={(event) => {
								if (event.key in ARROWS) void commit(latest.current);
							}}
							onPointerDown={(event) => {
								if (!canEdit) return;
								event.preventDefault();
								event.currentTarget.setPointerCapture(event.pointerId);
								event.currentTarget.focus();
								dragging.current = handle;
							}}
							onPointerMove={(event) => {
								if (dragging.current !== handle) return;
								const spot = spotOf(event);
								if (spot !== null) {
									change(moveHandle(latest.current, handle, spot.x, spot.y));
								}
							}}
							onPointerUp={() => {
								if (dragging.current !== handle) return;
								dragging.current = null;
								void commit(latest.current);
							}}
						/>
					);
				})}
			</svg>
			<div className="flex flex-wrap items-center gap-2">
				<Label className="text-xs">
					{t("ui.editor.keyframes.curve_preset")}
					<select
						aria-label={t("ui.editor.keyframes.curve_preset_of", {
							property: heading,
						})}
						value=""
						disabled={!canEdit}
						className={`${SELECT_CLASS} ml-2`}
						onChange={(event) => {
							const next = event.target.value;
							if (isPreset(next)) {
								void send(
									keyframeEasingOperations({
										itemId,
										property,
										frame,
										preset: next,
									}),
								).then(report);
							}
						}}
					>
						<option value="">
							{t("ui.editor.keyframes.curve_preset_none")}
						</option>
						{BEZIER_PRESETS.map((name) => (
							<option key={name} value={name}>
								{t(`ui.editor.keyframes.curve_preset_name.${name}`)}
							</option>
						))}
					</select>
				</Label>
			</div>
			<div className="flex flex-wrap gap-2">
				{NUMBER_NAMES.map((name, index) => (
					<NumberCell
						key={name}
						label={t("ui.editor.keyframes.curve_number", {
							name,
							property: heading,
						})}
						stored={curve[index] ?? 0}
						disabled={!canEdit}
						commit={async (text) => {
							const value = Number(text);
							if (text.trim() === "" || !Number.isFinite(value)) {
								return {
									ok: false,
									error: { code: "E_VALIDATION", message: "" },
								};
							}
							return commit(setNumber(latest.current, index, value));
						}}
					/>
				))}
			</div>
			<FieldMessage
				id={`bezier-${itemId}-${property}-${frame}-message`}
				state={{ error, remoteActor: null }}
			/>
		</div>
	);
}
