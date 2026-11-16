import { Plus, Trash2 } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import {
	type CurvePoint,
	constantSpeedOperations,
	insertCurvePoint,
	speedCurveOperations,
} from "../../commands/builders";
import { ratioValue } from "../../commands/units";
import { useCanEdit } from "../../commands/useActions";
import type { CommitResult } from "../fields/commitField";
import { FieldMessage } from "../fields/FieldFrame";
import NumberField from "../fields/NumberField";
import { useFieldCommit } from "../useFieldCommit";

const MIN_POINTS = 2;
const MAX_POINTS = 8;
const MIN_SPEED = 0.25;
const MAX_SPEED = 4;
const CHART = { width: 160, height: 48 } as const;

interface Ratio {
	readonly num: number;
	readonly den: number;
}

type StoredCurve = readonly { readonly t: number; readonly speed: Ratio }[];

export function storedCurve(curve: StoredCurve | undefined): CurvePoint[] {
	return (curve ?? []).map((point) => ({
		t: point.t,
		speed: ratioValue(point.speed),
	}));
}

const text = (value: number): string => String(Number(value.toFixed(3)));

// The shape of the ramp: the points joined by straight lines, as the core ramps
// the speed between them (SDD-004 section 3.11). Drawn from the points only.
function Chart({
	points,
	label,
}: {
	points: CurvePoint[];
	label: string;
}): ReactNode {
	const y = (speed: number): number =>
		CHART.height -
		((Math.min(Math.max(speed, MIN_SPEED), MAX_SPEED) - MIN_SPEED) /
			(MAX_SPEED - MIN_SPEED)) *
			CHART.height;
	const path = points
		.map((p) => `${(p.t * CHART.width).toFixed(1)},${y(p.speed).toFixed(1)}`)
		.join(" ");
	return (
		<svg
			role="img"
			aria-label={label}
			viewBox={`0 0 ${CHART.width} ${CHART.height}`}
			className="h-12 w-full rounded border border-input"
			data-speed-chart=""
		>
			<polyline
				points={path}
				fill="none"
				stroke="currentColor"
				strokeWidth="2"
				vectorEffect="non-scaling-stroke"
			/>
		</svg>
	);
}

// The speed curve of SDD-012 section 12.1: 2 to 8 points, each a position in the
// item (percent) and a speed (`x`). The points are a draft until Apply sends
// them as one `clip.set_speed` with `curve`; the core checks the order and the
// bounds, and the preview and the export read the result.
export default function SpeedCurve({
	itemId,
	curve,
	speed,
	ripple,
}: {
	itemId: string;
	curve: StoredCurve | undefined;
	speed: Ratio | undefined;
	ripple: boolean;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const stored = storedCurve(curve);
	const key = JSON.stringify(curve ?? null);
	const [draft, setDraft] = useState<CurvePoint[] | null>(null);
	const [failure, setFailure] = useState<CommitResult | null>(null);
	// biome-ignore lint/correctness/useExhaustiveDependencies: a new stored curve (from any actor) replaces the draft.
	useEffect(() => {
		setDraft(null);
		setFailure(null);
	}, [key]);

	const points = draft ?? stored;
	const editing = draft !== null;
	const hasCurve = stored.length > 0;
	const mean = speed === undefined ? 1 : ratioValue(speed);
	const change = (next: CurvePoint[]): void => {
		setDraft(next);
		setFailure(null);
	};
	const apply = async (): Promise<void> => {
		setFailure(await send(speedCurveOperations(itemId, points, ripple)));
	};
	const start = (): void =>
		change([
			{ t: 0, speed: mean },
			{ t: 1, speed: mean },
		]);
	const add = (): void => change(insertCurvePoint(points));
	const setPoint = (index: number, changes: Partial<CurvePoint>): void =>
		change(points.map((p, i) => (i === index ? { ...p, ...changes } : p)));

	if (!hasCurve && !editing) {
		return (
			<div>
				<Button
					type="button"
					size="sm"
					variant="outline"
					disabled={!canEdit}
					data-speed-curve-start=""
					onClick={start}
				>
					{t("ui.editor.clip.curve_start")}
				</Button>
			</div>
		);
	}
	return (
		<div data-speed-curve="" className="flex flex-col gap-2">
			<p className="text-xs font-medium">{t("ui.editor.clip.curve_title")}</p>
			<Chart points={points} label={t("ui.editor.clip.curve_chart")} />
			{points.map((point, index) => {
				const edge = index === 0 || index === points.length - 1;
				return (
					<div
						// biome-ignore lint/suspicious/noArrayIndexKey: the rows are the positions of a short list.
						key={index}
						data-curve-point={index}
						className="flex items-start gap-1"
					>
						<div className="min-w-0 flex-1">
							<NumberField
								label={t("ui.editor.clip.curve_time", { n: index + 1 })}
								stored={text(point.t * 100)}
								disabled={!canEdit || edge}
								step={1}
								min={0}
								max={100}
								suffix="%"
								commit={async (value) => {
									setPoint(index, { t: value / 100 });
									return { ok: true };
								}}
							/>
							<NumberField
								label={t("ui.editor.clip.curve_speed", { n: index + 1 })}
								stored={text(point.speed)}
								disabled={!canEdit}
								step={0.25}
								min={MIN_SPEED}
								max={MAX_SPEED}
								suffix="x"
								commit={async (value) => {
									setPoint(index, { speed: value });
									return { ok: true };
								}}
							/>
						</div>
						<Button
							type="button"
							size="icon-sm"
							variant="ghost"
							aria-label={t("ui.editor.clip.curve_remove", { n: index + 1 })}
							disabled={!canEdit || edge || points.length <= MIN_POINTS}
							onClick={() => change(points.filter((_, i) => i !== index))}
						>
							<Trash2 aria-hidden="true" />
						</Button>
					</div>
				);
			})}
			<div className="flex flex-wrap gap-1">
				<Button
					type="button"
					size="sm"
					variant="outline"
					disabled={!canEdit || points.length >= MAX_POINTS}
					onClick={add}
				>
					<Plus aria-hidden="true" />
					{t("ui.editor.clip.curve_add")}
				</Button>
				<Button
					type="button"
					size="sm"
					disabled={!canEdit || !editing}
					onClick={() => void apply()}
				>
					{t("ui.editor.clip.curve_apply")}
				</Button>
				<Button
					type="button"
					size="sm"
					variant="outline"
					disabled={!canEdit}
					onClick={() => {
						if (hasCurve && speed !== undefined) {
							void send(constantSpeedOperations(itemId, speed));
						} else {
							setDraft(null);
						}
					}}
				>
					{t("ui.editor.clip.curve_remove_all")}
				</Button>
			</div>
			{hasCurve && (
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.clip.curve_mean", { speed: text(mean) })}
				</p>
			)}
			<FieldMessage
				id={`curve-message-${itemId}`}
				state={{
					error: failure !== null && !failure.ok ? failure.error : null,
					remoteActor: null,
				}}
			/>
		</div>
	);
}
